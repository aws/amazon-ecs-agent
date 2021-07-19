// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package engine contains the core logic for managing tasks
package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/metrics"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	utilsync "github.com/aws/amazon-ecs-agent/agent/utils/sync"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	dockercontainer "github.com/docker/docker/api/types/container"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

const (
	//DockerEndpointEnvVariable is the environment variable that can override the Docker endpoint
	DockerEndpointEnvVariable = "DOCKER_HOST"
	// DockerDefaultEndpoint is the default value for the Docker endpoint
	DockerDefaultEndpoint              = "unix:///var/run/docker.sock"
	labelPrefix                        = "com.amazonaws.ecs."
	labelTaskARN                       = labelPrefix + "task-arn"
	labelContainerName                 = labelPrefix + "container-name"
	labelTaskDefinitionFamily          = labelPrefix + "task-definition-family"
	labelTaskDefinitionVersion         = labelPrefix + "task-definition-version"
	labelCluster                       = labelPrefix + "cluster"
	minGetIPBridgeTimeout              = time.Second
	maxGetIPBridgeTimeout              = 10 * time.Second
	getIPBridgeRetryJitterMultiplier   = 0.2
	getIPBridgeRetryDelayMultiplier    = 2
	ipamCleanupTmeout                  = 5 * time.Second
	minEngineConnectRetryDelay         = 2 * time.Second
	maxEngineConnectRetryDelay         = 200 * time.Second
	engineConnectRetryJitterMultiplier = 0.20
	engineConnectRetryDelayMultiplier  = 1.5
	// logDriverTypeFirelens is the log driver type for containers that want to use the firelens container to send logs.
	logDriverTypeFirelens       = "awsfirelens"
	logDriverTypeFluentd        = "fluentd"
	logDriverTag                = "tag"
	logDriverFluentdAddress     = "fluentd-address"
	dataLogDriverPath           = "/data/firelens/"
	logDriverAsyncConnect       = "fluentd-async-connect"
	logDriverSubSecondPrecision = "fluentd-sub-second-precision"
	logDriverBufferLimit        = "fluentd-buffer-limit"
	dataLogDriverSocketPath     = "/socket/fluent.sock"
	socketPathPrefix            = "unix://"

	// fluentTagDockerFormat is the format for the log tag, which is "containerName-firelens-taskID"
	fluentTagDockerFormat = "%s-firelens-%s"

	// Environment variables are needed for firelens
	fluentNetworkHost      = "FLUENT_HOST"
	fluentNetworkPort      = "FLUENT_PORT"
	FluentNetworkPortValue = "24224"
	FluentAWSVPCHostValue  = "127.0.0.1"

	defaultMonitorExecAgentsInterval = 15 * time.Minute

	defaultStopContainerBackoffMin = time.Second
	defaultStopContainerBackoffMax = time.Second * 5
	stopContainerBackoffJitter     = 0.2
	stopContainerBackoffMultiplier = 1.3
	stopContainerMaxRetryCount     = 5
)

var newExponentialBackoff = retry.NewExponentialBackoff

// DockerTaskEngine is a state machine for managing a task and its containers
// in ECS.
//
// DockerTaskEngine implements an abstraction over the DockerGoClient so that
// it does not have to know about tasks, only containers
// The DockerTaskEngine interacts with Docker to implement a TaskEngine
type DockerTaskEngine struct {
	// implements TaskEngine

	cfg *config.Config

	ctx          context.Context
	initialized  bool
	mustInitLock sync.Mutex

	// state stores all tasks this task engine is aware of, including their
	// current state and mappings to/from dockerId and name.
	// This is used to checkpoint state to disk so tasks may survive agent
	// failures or updates
	state        dockerstate.TaskEngineState
	managedTasks map[string]*managedTask

	taskStopGroup *utilsync.SequentialWaitGroup

	events            <-chan dockerapi.DockerContainerChangeEvent
	stateChangeEvents chan statechange.Event

	client     dockerapi.DockerClient
	dataClient data.Client
	cniClient  ecscni.CNIClient

	containerChangeEventStream *eventstream.EventStream

	stopEngine context.CancelFunc

	// tasksLock is a mutex that the task engine must acquire before changing
	// any task's state which it manages. Since this is a lock that encompasses
	// all tasks, it must not acquire it for any significant duration
	// The write mutex should be taken when adding and removing tasks from managedTasks.
	tasksLock sync.RWMutex

	credentialsManager                  credentials.Manager
	_time                               ttime.Time
	_timeOnce                           sync.Once
	imageManager                        ImageManager
	containerStatusToTransitionFunction map[apicontainerstatus.ContainerStatus]transitionApplyFunc
	metadataManager                     containermetadata.Manager

	// taskSteadyStatePollInterval is the duration that a managed task waits
	// once the task gets into steady state before polling the state of all of
	// the task's containers to re-evaluate if the task is still in steady state
	// This is set to defaultTaskSteadyStatePollInterval in production code.
	// This can be used by tests that are looking to ensure that the steady state
	// verification logic gets executed to set it to a low interval
	taskSteadyStatePollInterval       time.Duration
	taskSteadyStatePollIntervalJitter time.Duration

	resourceFields *taskresource.ResourceFields

	// handleDelay is a function used to delay cleanup. Implementation is
	// swappable for testing
	handleDelay               func(duration time.Duration)
	monitorExecAgentsTicker   *time.Ticker
	execCmdMgr                execcmd.Manager
	monitorExecAgentsInterval time.Duration
	stopContainerBackoffMin   time.Duration
	stopContainerBackoffMax   time.Duration
	namespaceHelper           ecscni.NamespaceHelper
}

// NewDockerTaskEngine returns a created, but uninitialized, DockerTaskEngine.
// The distinction between created and initialized is that when created it may
// be serialized/deserialized, but it will not communicate with docker until it
// is also initialized.
func NewDockerTaskEngine(cfg *config.Config,
	client dockerapi.DockerClient,
	credentialsManager credentials.Manager,
	containerChangeEventStream *eventstream.EventStream,
	imageManager ImageManager,
	state dockerstate.TaskEngineState,
	metadataManager containermetadata.Manager,
	resourceFields *taskresource.ResourceFields,
	execCmdMgr execcmd.Manager) *DockerTaskEngine {
	dockerTaskEngine := &DockerTaskEngine{
		cfg:        cfg,
		client:     client,
		dataClient: data.NewNoopClient(),

		state:             state,
		managedTasks:      make(map[string]*managedTask),
		taskStopGroup:     utilsync.NewSequentialWaitGroup(),
		stateChangeEvents: make(chan statechange.Event),

		credentialsManager: credentialsManager,

		containerChangeEventStream: containerChangeEventStream,
		imageManager:               imageManager,
		cniClient:                  ecscni.NewClient(cfg.CNIPluginsPath),

		metadataManager:                   metadataManager,
		taskSteadyStatePollInterval:       defaultTaskSteadyStatePollInterval,
		taskSteadyStatePollIntervalJitter: defaultTaskSteadyStatePollIntervalJitter,
		resourceFields:                    resourceFields,
		handleDelay:                       time.Sleep,
		execCmdMgr:                        execCmdMgr,
		monitorExecAgentsInterval:         defaultMonitorExecAgentsInterval,
		stopContainerBackoffMin:           defaultStopContainerBackoffMin,
		stopContainerBackoffMax:           defaultStopContainerBackoffMax,
		namespaceHelper:                   ecscni.NewNamespaceHelper(client),
	}

	dockerTaskEngine.initializeContainerStatusToTransitionFunction()

	return dockerTaskEngine
}

func (engine *DockerTaskEngine) initializeContainerStatusToTransitionFunction() {
	containerStatusToTransitionFunction := map[apicontainerstatus.ContainerStatus]transitionApplyFunc{
		apicontainerstatus.ContainerPulled:               engine.pullContainer,
		apicontainerstatus.ContainerCreated:              engine.createContainer,
		apicontainerstatus.ContainerRunning:              engine.startContainer,
		apicontainerstatus.ContainerResourcesProvisioned: engine.provisionContainerResources,
		apicontainerstatus.ContainerStopped:              engine.stopContainer,
	}
	engine.containerStatusToTransitionFunction = containerStatusToTransitionFunction
}

// ImagePullDeleteLock ensures that pulls and deletes do not run at the same time and pulls can be run at the same time for docker >= 1.11.1
// Pulls are serialized as a temporary workaround for a devicemapper issue. (see https://github.com/docker/docker/issues/9718)
// Deletes must not run at the same time as pulls to prevent deletion of images that are being used to launch new tasks.
var ImagePullDeleteLock sync.RWMutex

// UnmarshalJSON restores a previously marshaled task-engine state from json
func (engine *DockerTaskEngine) UnmarshalJSON(data []byte) error {
	return engine.state.UnmarshalJSON(data)
}

// MarshalJSON marshals into state directly
func (engine *DockerTaskEngine) MarshalJSON() ([]byte, error) {
	return engine.state.MarshalJSON()
}

// Init initializes a DockerTaskEngine such that it may communicate with docker
// and operate normally.
// This function must be called before any other function, except serializing and deserializing, can succeed without error.
func (engine *DockerTaskEngine) Init(ctx context.Context) error {
	derivedCtx, cancel := context.WithCancel(ctx)
	engine.stopEngine = cancel
	engine.ctx = derivedCtx

	// Open the event stream before we sync state so that e.g. if a container
	// goes from running to stopped after we sync with it as "running" we still
	// have the "went to stopped" event pending so we can be up to date.
	err := engine.openEventstream(derivedCtx)
	if err != nil {
		return err
	}
	engine.synchronizeState()
	// Now catch up and start processing new events per normal
	go engine.handleDockerEvents(derivedCtx)
	engine.initialized = true
	go engine.startPeriodicExecAgentsMonitoring(derivedCtx)
	return nil
}

func (engine *DockerTaskEngine) startPeriodicExecAgentsMonitoring(ctx context.Context) {
	engine.monitorExecAgentsTicker = time.NewTicker(engine.monitorExecAgentsInterval)
	for {
		select {
		case <-engine.monitorExecAgentsTicker.C:
			go engine.monitorExecAgentProcesses(ctx)
		case <-ctx.Done():
			engine.monitorExecAgentsTicker.Stop()
			return
		}
	}
}

func (engine *DockerTaskEngine) monitorExecAgentProcesses(ctx context.Context) {
	// TODO: [ecs-exec]add jitter between containers to not overload docker with top calls
	engine.tasksLock.RLock()
	defer engine.tasksLock.RUnlock()
	for _, mTask := range engine.managedTasks {
		task := mTask.Task

		if task.GetKnownStatus() != apitaskstatus.TaskRunning {
			continue
		}
		for _, c := range task.Containers {
			if execcmd.IsExecEnabledContainer(c) {
				if ma, _ := c.GetManagedAgentByName(execcmd.ExecuteCommandAgentName); !ma.InitFailed {
					go engine.monitorExecAgentRunning(ctx, mTask, c)
				}
			}
		}
	}
}

func (engine *DockerTaskEngine) monitorExecAgentRunning(ctx context.Context,
	mTask *managedTask, c *apicontainer.Container) {
	if !c.IsRunning() {
		return
	}
	task := mTask.Task
	dockerID, err := engine.getDockerID(task, c)
	if err != nil {
		seelog.Errorf("Task engine [%s]: Could not retrieve docker id for container", task.Arn)
		return
	}
	// Sleeping here so that all the containers do not call inspect/start exec agent process
	// at the same time.
	// The max sleep is 50% of the monitor interval to allow enough buffer time
	// to finish monitoring.
	// This is inspired from containers streaming stats from Docker.
	time.Sleep(retry.AddJitter(time.Nanosecond, engine.monitorExecAgentsInterval/2))
	status, err := engine.execCmdMgr.RestartAgentIfStopped(ctx, engine.client, task, c, dockerID)
	if err != nil {
		seelog.Errorf("Task engine [%s]: Failed to restart ExecCommandAgent Process for container [%s]: %v", task.Arn, dockerID, err)
		mTask.emitManagedAgentEvent(mTask.Task, c, execcmd.ExecuteCommandAgentName, "ExecuteCommandAgent cannot be restarted")
	}
	if status == execcmd.Restarted {
		mTask.emitManagedAgentEvent(mTask.Task, c, execcmd.ExecuteCommandAgentName, "ExecuteCommandAgent restarted")
	}

}

// MustInit blocks and retries until an engine can be initialized.
func (engine *DockerTaskEngine) MustInit(ctx context.Context) {
	if engine.initialized {
		return
	}
	engine.mustInitLock.Lock()
	defer engine.mustInitLock.Unlock()

	errorOnce := sync.Once{}
	taskEngineConnectBackoff := retry.NewExponentialBackoff(minEngineConnectRetryDelay, maxEngineConnectRetryDelay,
		engineConnectRetryJitterMultiplier, engineConnectRetryDelayMultiplier)
	retry.RetryWithBackoff(taskEngineConnectBackoff, func() error {
		if engine.initialized {
			return nil
		}
		err := engine.Init(ctx)
		if err != nil {
			errorOnce.Do(func() {
				seelog.Errorf("Task engine: could not connect to docker daemon: %v", err)
			})
		}
		return err
	})
}

// SetDataClient sets the saver that is used by the DockerTaskEngine.
func (engine *DockerTaskEngine) SetDataClient(client data.Client) {
	engine.dataClient = client
}

// Shutdown makes a best-effort attempt to cleanup after the task engine.
// This should not be relied on for anything more complicated than testing.
func (engine *DockerTaskEngine) Shutdown() {
	engine.stopEngine()
	engine.Disable()
}

// Disable prevents this engine from managing any additional tasks.
func (engine *DockerTaskEngine) Disable() {
	engine.tasksLock.Lock()
}

// isTaskManaged checks if task for the corresponding arn is present
func (engine *DockerTaskEngine) isTaskManaged(arn string) bool {
	engine.tasksLock.RLock()
	defer engine.tasksLock.RUnlock()
	_, ok := engine.managedTasks[arn]
	return ok
}

// synchronizeState explicitly goes through each docker container stored in
// "state" and updates its KnownStatus appropriately, as well as queueing up
// events to push upstream. It also initializes some fields of task resources and eni attachments that won't be populated
// from loading state file.
func (engine *DockerTaskEngine) synchronizeState() {
	engine.tasksLock.Lock()
	defer engine.tasksLock.Unlock()
	imageStates := engine.state.AllImageStates()
	if len(imageStates) != 0 {
		engine.imageManager.AddAllImageStates(imageStates)
	}
	eniAttachments := engine.state.AllENIAttachments()
	for _, eniAttachment := range eniAttachments {
		timeoutFunc := func() {
			eniAttachment, ok := engine.state.ENIByMac(eniAttachment.MACAddress)
			if !ok {
				seelog.Warnf("Ignoring unmanaged ENI attachment with MAC address: %s", eniAttachment.MACAddress)
				return
			}
			if !eniAttachment.IsSent() {
				seelog.Warnf("Timed out waiting for ENI ack; removing ENI attachment record %s", eniAttachment.String())
				engine.removeENIAttachmentData(eniAttachment.MACAddress)
				engine.state.RemoveENIAttachment(eniAttachment.MACAddress)
			}
		}
		err := eniAttachment.Initialize(timeoutFunc)
		if err != nil {
			// The only case where we get an error from Initialize is that the attachment has expired. In that case, remove the expired
			// attachment from state.
			seelog.Warnf("ENI attachment has expired. Removing it from state. %s", eniAttachment.String())
			engine.removeENIAttachmentData(eniAttachment.MACAddress)
			engine.state.RemoveENIAttachment(eniAttachment.MACAddress)
		}
	}

	tasks := engine.state.AllTasks()
	tasksToStart := engine.filterTasksToStartUnsafe(tasks)
	for _, task := range tasks {
		task.InitializeResources(engine.resourceFields)
		engine.saveTaskData(task)
	}

	for _, task := range tasksToStart {
		engine.startTask(task)
	}
}

// filterTasksToStartUnsafe filters only the tasks that need to be started after
// the agent has been restarted. It also synchronizes states of all of the containers
// in tasks that need to be started.
func (engine *DockerTaskEngine) filterTasksToStartUnsafe(tasks []*apitask.Task) []*apitask.Task {
	var tasksToStart []*apitask.Task
	for _, task := range tasks {
		conts, ok := engine.state.ContainerMapByArn(task.Arn)
		if !ok {
			// task hasn't started processing, no need to check container status
			tasksToStart = append(tasksToStart, task)
			continue
		}

		for _, cont := range conts {
			engine.synchronizeContainerStatus(cont, task)
			engine.saveDockerContainerData(cont) // persist the container with the updated information.
		}

		tasksToStart = append(tasksToStart, task)

		// Put tasks that are stopped by acs but hasn't been stopped in wait group
		if task.GetDesiredStatus().Terminal() && task.GetStopSequenceNumber() != 0 {
			engine.taskStopGroup.Add(task.GetStopSequenceNumber(), 1)
		}
	}

	return tasksToStart
}

// updateContainerMetadata sets the container metadata from the docker inspect
func updateContainerMetadata(metadata *dockerapi.DockerContainerMetadata, container *apicontainer.Container, task *apitask.Task) {
	container.SetCreatedAt(metadata.CreatedAt)
	container.SetStartedAt(metadata.StartedAt)
	container.SetFinishedAt(metadata.FinishedAt)

	// Set the labels if it's not set
	if len(metadata.Labels) != 0 && len(container.GetLabels()) == 0 {
		container.SetLabels(metadata.Labels)
	}

	// Update volume for empty volume container
	if metadata.Volumes != nil {
		if container.IsInternal() {
			task.UpdateMountPoints(container, metadata.Volumes)
		} else {
			container.SetVolumes(metadata.Volumes)
		}
	}

	// Set Exitcode if it's not set
	if metadata.ExitCode != nil {
		container.SetKnownExitCode(metadata.ExitCode)
	}

	// Set port mappings
	if len(metadata.PortBindings) != 0 && len(container.GetKnownPortBindings()) == 0 {
		container.SetKnownPortBindings(metadata.PortBindings)
	}
	// update the container health information
	if container.HealthStatusShouldBeReported() {
		container.SetHealthStatus(metadata.Health)
	}
	container.SetNetworkMode(metadata.NetworkMode)
	container.SetNetworkSettings(metadata.NetworkSettings)
}

// synchronizeContainerStatus checks and updates the container status with docker
func (engine *DockerTaskEngine) synchronizeContainerStatus(container *apicontainer.DockerContainer, task *apitask.Task) {
	if container.DockerID == "" {
		seelog.Debugf("Task engine [%s]: found container potentially created while we were down: %s",
			task.Arn, container.DockerName)
		// Figure out the dockerid
		describedContainer, err := engine.client.InspectContainer(engine.ctx,
			container.DockerName, dockerclient.InspectContainerTimeout)
		if err != nil {
			seelog.Warnf("Task engine [%s]: could not find matching container for expected name [%s]: %v",
				task.Arn, container.DockerName, err)
		} else {
			// update the container metadata in case the container was created during agent restart
			metadata := dockerapi.MetadataFromContainer(describedContainer)
			updateContainerMetadata(&metadata, container.Container, task)
			container.DockerID = describedContainer.ID

			container.Container.SetKnownStatus(dockerapi.DockerStateToState(describedContainer.State))
			// update mappings that need dockerid
			engine.state.AddContainer(container, task)
			err := engine.imageManager.RecordContainerReference(container.Container)
			if err != nil {
				seelog.Warnf("Task engine [%s]: unable to add container reference to image state: %v",
					task.Arn, err)
			}
		}
		return
	}

	currentState, metadata := engine.client.DescribeContainer(engine.ctx, container.DockerID)
	if metadata.Error != nil {
		currentState = apicontainerstatus.ContainerStopped
		// If this is a Docker API error
		if metadata.Error.ErrorName() == dockerapi.CannotDescribeContainerErrorName {
			seelog.Warnf("Task engine [%s]: could not describe previously known container [id=%s; name=%s]; assuming dead: %v",
				task.Arn, container.DockerID, container.DockerName, metadata.Error)
			if !container.Container.KnownTerminal() {
				container.Container.ApplyingError = apierrors.NewNamedError(&ContainerVanishedError{})
				err := engine.imageManager.RemoveContainerReferenceFromImageState(container.Container)
				if err != nil {
					seelog.Warnf("Task engine [%s]: could not remove container reference for image state %s: %v",
						container.Container.Image, err)
				}
			}
		} else {
			// If this is a container state error
			updateContainerMetadata(&metadata, container.Container, task)
			container.Container.ApplyingError = apierrors.NewNamedError(metadata.Error)
		}
	} else {
		// update the container metadata in case the container status/metadata changed during agent restart
		updateContainerMetadata(&metadata, container.Container, task)
		err := engine.imageManager.RecordContainerReference(container.Container)
		if err != nil {
			seelog.Warnf("Task engine [%s]: unable to add container reference to image state: %v",
				task.Arn, err)
		}
		if engine.cfg.ContainerMetadataEnabled.Enabled() && !container.Container.IsMetadataFileUpdated() {
			go engine.updateMetadataFile(task, container)
		}
	}
	if currentState > container.Container.GetKnownStatus() {
		// update the container known status
		container.Container.SetKnownStatus(currentState)
	}
	// Update task ExecutionStoppedAt timestamp
	task.RecordExecutionStoppedAt(container.Container)
}

// checkTaskState inspects the state of all containers within a task and writes
// their state to the managed task's container channel.
func (engine *DockerTaskEngine) checkTaskState(task *apitask.Task) {
	defer metrics.MetricsEngineGlobal.RecordTaskEngineMetric("CHECK_TASK_STATE")()
	for _, container := range task.Containers {
		dockerID, err := engine.getDockerID(task, container)
		if err != nil {
			continue
		}
		status, metadata := engine.client.DescribeContainer(engine.ctx, dockerID)
		engine.tasksLock.RLock()
		managedTask, ok := engine.managedTasks[task.Arn]
		engine.tasksLock.RUnlock()

		if ok {
			managedTask.emitDockerContainerChange(dockerContainerChange{
				container: container,
				event: dockerapi.DockerContainerChangeEvent{
					Status:                  status,
					DockerContainerMetadata: metadata,
				},
			})
		}
	}
}

// sweepTask deletes all the containers associated with a task
func (engine *DockerTaskEngine) sweepTask(task *apitask.Task) {
	for _, cont := range task.Containers {
		err := engine.removeContainer(task, cont)
		if err != nil {
			seelog.Infof("Task engine [%s]: unable to remove old container [%s]: %v",
				task.Arn, cont.Name, err)
		}
		// Internal container(created by ecs-agent) state isn't recorded
		if cont.IsInternal() {
			continue
		}
		err = engine.imageManager.RemoveContainerReferenceFromImageState(cont)
		if err != nil {
			seelog.Errorf("Task engine [%s]: Unable to remove container [%s] reference from image state: %v",
				task.Arn, cont.Name, err)
		}
	}

	// Clean metadata directory for task
	if engine.cfg.ContainerMetadataEnabled.Enabled() {
		err := engine.metadataManager.Clean(task.Arn)
		if err != nil {
			seelog.Warnf("Task engine [%s]: clean task metadata failed: %v", task.Arn, err)
		}
	}
}

var removeAll = os.RemoveAll

func (engine *DockerTaskEngine) deleteTask(task *apitask.Task) {
	for _, resource := range task.GetResources() {
		err := resource.Cleanup()
		if err != nil {
			seelog.Warnf("Task engine [%s]: unable to cleanup resource %s: %v",
				task.Arn, resource.GetName(), err)
		} else {
			seelog.Infof("Task engine [%s]: resource %s cleanup complete", task.Arn,
				resource.GetName())
		}
	}

	if execcmd.IsExecEnabledTask(task) {
		// cleanup host exec agent log dirs
		if tID, err := task.GetID(); err != nil {
			seelog.Warnf("Task Engine[%s]: error getting task ID for ExecAgent logs cleanup: %v", task.Arn, err)
		} else {
			if err := removeAll(filepath.Join(execcmd.ECSAgentExecLogDir, tID)); err != nil {
				seelog.Warnf("Task Engine[%s]: unable to remove ExecAgent host logs for task: %v", task.Arn, err)
			}
		}
	}

	// Now remove ourselves from the global state and cleanup channels
	engine.tasksLock.Lock()
	engine.state.RemoveTask(task)

	taskENIs := task.GetTaskENIs()
	for _, taskENI := range taskENIs {
		// ENIs that exist only as logical associations on another interface do not have
		// attachments that need to be removed.
		if taskENI.IsStandardENI() {
			seelog.Debugf("Task engine [%s]: removing eni %s from agent state",
				task.Arn, taskENI.ID)
			engine.removeENIAttachmentData(taskENI.MacAddress)
			engine.state.RemoveENIAttachment(taskENI.MacAddress)
		} else {
			seelog.Debugf("Task engine [%s]: skipping removing logical eni %s from agent state",
				task.Arn, taskENI.ID)
		}
	}

	// Remove task and container data from database.
	engine.removeTaskData(task)

	seelog.Infof("Task engine [%s]: finished removing task data, removing task from managed tasks", task.Arn)
	delete(engine.managedTasks, task.Arn)
	engine.tasksLock.Unlock()
}

func (engine *DockerTaskEngine) emitTaskEvent(task *apitask.Task, reason string) {
	event, err := api.NewTaskStateChangeEvent(task, reason)
	if err != nil {
		seelog.Infof("Task engine [%s]: unable to create task state change event: %v", task.Arn, err)
		return
	}

	seelog.Infof("Task engine [%s]: Task engine: sending change event [%s]", task.Arn, event.String())
	engine.stateChangeEvents <- event
}

// startTask creates a managedTask construct to track the task and then begins
// pushing it towards its desired state when allowed startTask is protected by
// the tasksLock lock of 'AddTask'. It should not be called from anywhere
// else and should exit quickly to allow AddTask to do more work.
func (engine *DockerTaskEngine) startTask(task *apitask.Task) {
	// Create a channel that may be used to communicate with this task, survey
	// what tasks need to be waited for for this one to start, and then spin off
	// a goroutine to oversee this task

	thisTask := engine.newManagedTask(task)
	thisTask._time = engine.time()

	go thisTask.overseeTask()
}

func (engine *DockerTaskEngine) time() ttime.Time {
	engine._timeOnce.Do(func() {
		if engine._time == nil {
			engine._time = &ttime.DefaultTime{}
		}
	})
	return engine._time
}

// openEventstream opens, but does not consume, the docker event stream
func (engine *DockerTaskEngine) openEventstream(ctx context.Context) error {
	events, err := engine.client.ContainerEvents(ctx)
	if err != nil {
		return err
	}
	engine.events = events
	return nil
}

// handleDockerEvents must be called after openEventstream; it processes each
// event that it reads from the docker eventstream
func (engine *DockerTaskEngine) handleDockerEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-engine.events:
			engine.handleDockerEvent(event)
		}
	}
}

// handleDockerEvent is the entrypoint for task modifications originating with
// events occurring through Docker, outside the task engine itself.
// handleDockerEvent is responsible for taking an event that correlates to a
// container and placing it in the context of the task to which that container
// belongs.
func (engine *DockerTaskEngine) handleDockerEvent(event dockerapi.DockerContainerChangeEvent) {
	seelog.Debugf("Task engine: handling a docker event: %s", event.String())

	task, ok := engine.state.TaskByID(event.DockerID)
	if !ok {
		seelog.Debugf("Task engine: event for container [%s] not managed, unable to map container id to task",
			event.DockerID)
		return
	}
	cont, ok := engine.state.ContainerByID(event.DockerID)
	if !ok {
		seelog.Debugf("Task engine: event for container [%s] not managed, unable to map container id to container",
			event.DockerID)
		return
	}

	// Container health status change does not affect the container status
	// no need to process this in task manager
	if event.Type == apicontainer.ContainerHealthEvent {
		if cont.Container.HealthStatusShouldBeReported() {
			seelog.Debugf("Task engine: updating container [%s(%s)] health status: %v",
				cont.Container.Name, cont.DockerID, event.DockerContainerMetadata.Health)
			cont.Container.SetHealthStatus(event.DockerContainerMetadata.Health)
		}
		return
	}

	engine.tasksLock.RLock()
	managedTask, ok := engine.managedTasks[task.Arn]
	engine.tasksLock.RUnlock()
	if !ok {
		seelog.Criticalf("Task engine: could not find managed task [%s] corresponding to a docker event: %s",
			task.Arn, event.String())
		return
	}
	seelog.Debugf("Task engine [%s]: writing docker event to the task: %s",
		task.Arn, event.String())
	managedTask.emitDockerContainerChange(dockerContainerChange{container: cont.Container, event: event})
	seelog.Debugf("Task engine [%s]: wrote docker event to the task: %s",
		task.Arn, event.String())
}

// StateChangeEvents returns channels to read task and container state changes. These
// changes should be read as soon as possible as them not being read will block
// processing the task referenced by the event.
func (engine *DockerTaskEngine) StateChangeEvents() chan statechange.Event {
	return engine.stateChangeEvents
}

// AddTask starts tracking a task
func (engine *DockerTaskEngine) AddTask(task *apitask.Task) {
	defer metrics.MetricsEngineGlobal.RecordTaskEngineMetric("ADD_TASK")()
	err := task.PostUnmarshalTask(engine.cfg, engine.credentialsManager,
		engine.resourceFields, engine.client, engine.ctx)
	if err != nil {
		seelog.Errorf("Task engine [%s]: unable to add task to the engine: %v", task.Arn, err)
		task.SetKnownStatus(apitaskstatus.TaskStopped)
		task.SetDesiredStatus(apitaskstatus.TaskStopped)
		engine.emitTaskEvent(task, err.Error())
		return
	}

	engine.tasksLock.Lock()
	defer engine.tasksLock.Unlock()

	existingTask, exists := engine.state.TaskByArn(task.Arn)
	if !exists {
		// This will update the container desired status
		task.UpdateDesiredStatus()

		// This will update any dependencies for awsvpc network mode before the task is started.
		engine.updateTaskENIDependencies(task)

		engine.state.AddTask(task)
		if dependencygraph.ValidDependencies(task, engine.cfg) {
			engine.startTask(task)
		} else {
			seelog.Errorf("Task engine [%s]: unable to progress task with circular dependencies", task.Arn)
			task.SetKnownStatus(apitaskstatus.TaskStopped)
			task.SetDesiredStatus(apitaskstatus.TaskStopped)
			err := TaskDependencyError{task.Arn}
			engine.emitTaskEvent(task, err.Error())
		}
		return
	}

	// Update task
	engine.updateTaskUnsafe(existingTask, task)
}

// ListTasks returns the tasks currently managed by the DockerTaskEngine
func (engine *DockerTaskEngine) ListTasks() ([]*apitask.Task, error) {
	return engine.state.AllTasks(), nil
}

// GetTaskByArn returns the task identified by that ARN
func (engine *DockerTaskEngine) GetTaskByArn(arn string) (*apitask.Task, bool) {
	return engine.state.TaskByArn(arn)
}

func (engine *DockerTaskEngine) pullContainer(task *apitask.Task, container *apicontainer.Container) dockerapi.DockerContainerMetadata {
	switch container.Type {
	case apicontainer.ContainerCNIPause, apicontainer.ContainerNamespacePause:
		// pause images are managed at startup
		return dockerapi.DockerContainerMetadata{}
	}

	if engine.imagePullRequired(engine.cfg.ImagePullBehavior, container, task.Arn) {
		// Record the pullStoppedAt timestamp
		defer func() {
			timestamp := engine.time().Now()
			task.SetPullStoppedAt(timestamp)
		}()

		seelog.Infof("Task engine [%s]: pulling image %s for container %s concurrently", task.Arn, container.Image, container.Name)
		return engine.concurrentPull(task, container)

	}

	// No pull image is required, the cached image will be used.
	// Add the container that uses the cached image to the pulled container state.
	dockerContainer := &apicontainer.DockerContainer{
		Container: container,
	}
	engine.state.AddPulledContainer(dockerContainer, task)

	// No pull image is required, just update container reference and use cached image.
	engine.updateContainerReference(false, container, task.Arn)
	// Return the metadata without any error
	return dockerapi.DockerContainerMetadata{Error: nil}
}

// imagePullRequired returns true if pulling image is required, or return false if local image cache
// should be used, by inspecting the agent pull behavior variable defined in config. The caller has
// to make sure the container passed in is not an internal container.
func (engine *DockerTaskEngine) imagePullRequired(imagePullBehavior config.ImagePullBehaviorType,
	container *apicontainer.Container,
	taskArn string) bool {
	switch imagePullBehavior {
	case config.ImagePullOnceBehavior:
		// If this image has been pulled successfully before, don't pull the image,
		// otherwise pull the image as usual, regardless whether the image exists or not
		// (the image can be prepopulated with the AMI and never be pulled).
		imageState, ok := engine.imageManager.GetImageStateFromImageName(container.Image)
		if ok && imageState.GetPullSucceeded() {
			seelog.Infof("Task engine [%s]: image %s for container %s has been pulled once, not pulling it again",
				taskArn, container.Image, container.Name)
			return false
		}
		return true
	case config.ImagePullPreferCachedBehavior:
		// If the behavior is prefer cached, don't pull if we found cached image
		// by inspecting the image.
		_, err := engine.client.InspectImage(container.Image)
		if err != nil {
			return true
		}
		seelog.Infof("Task engine [%s]: found cached image %s, use it directly for container %s",
			taskArn, container.Image, container.Name)
		return false
	default:
		// Need to pull the image for always and default agent pull behavior
		return true
	}
}

func (engine *DockerTaskEngine) concurrentPull(task *apitask.Task, container *apicontainer.Container) dockerapi.DockerContainerMetadata {
	seelog.Debugf("Task engine [%s]: attempting to obtain ImagePullDeleteLock to pull image %s for container %s",
		task.Arn, container.Image, container.Name)
	ImagePullDeleteLock.RLock()
	seelog.Debugf("Task engine [%s]: acquired ImagePullDeleteLock, start pulling image %s for container %s",
		task.Arn, container.Image, container.Name)
	defer seelog.Debugf("Task engine [%s]: released ImagePullDeleteLock after pulling image %s for container %s",
		task.Arn, container.Image, container.Name)
	defer ImagePullDeleteLock.RUnlock()

	// Record the task pull_started_at timestamp
	pullStart := engine.time().Now()
	ok := task.SetPullStartedAt(pullStart)
	if ok {
		seelog.Infof("Task engine [%s]: recording timestamp for starting image pulltime: %s",
			task.Arn, pullStart)
	}
	metadata := engine.pullAndUpdateContainerReference(task, container)
	if metadata.Error == nil {
		seelog.Infof("Task engine [%s]: finished pulling image %s for container %s in %s",
			task.Arn, container.Image, container.Name, time.Since(pullStart).String())
	} else {
		seelog.Errorf("Task engine [%s]: failed to pull image %s for container %s: %v",
			task.Arn, container.Image, container.Name, metadata.Error)
	}
	return metadata
}

func (engine *DockerTaskEngine) pullAndUpdateContainerReference(task *apitask.Task, container *apicontainer.Container) dockerapi.DockerContainerMetadata {
	// If a task is blocked here for some time, and before it starts pulling image,
	// the task's desired status is set to stopped, then don't pull the image
	if task.GetDesiredStatus() == apitaskstatus.TaskStopped {
		seelog.Infof("Task engine [%s]: task's desired status is stopped, skipping pulling image %s for container %s",
			task.Arn, container.Image, container.Name)
		container.SetDesiredStatus(apicontainerstatus.ContainerStopped)
		return dockerapi.DockerContainerMetadata{Error: TaskStoppedBeforePullBeginError{task.Arn}}
	}

	// Set the credentials for pull from ECR if necessary
	if container.ShouldPullWithExecutionRole() {
		executionCredentials, ok := engine.credentialsManager.GetTaskCredentials(task.GetExecutionCredentialsID())
		if !ok {
			seelog.Errorf("Task engine [%s]: unable to acquire ECR credentials for image %s for container %s",
				task.Arn, container.Image, container.Name)
			return dockerapi.DockerContainerMetadata{
				Error: dockerapi.CannotPullECRContainerError{
					FromError: errors.New("engine ecr credentials: not found"),
				},
			}
		}

		iamCredentials := executionCredentials.GetIAMRoleCredentials()
		container.SetRegistryAuthCredentials(iamCredentials)
		// Clean up the ECR pull credentials after pulling
		defer container.SetRegistryAuthCredentials(credentials.IAMRoleCredentials{})
	}

	// Apply registry auth data from ASM if required
	if container.ShouldPullWithASMAuth() {
		if err := task.PopulateASMAuthData(container); err != nil {
			seelog.Errorf("Task engine [%s]: unable to acquire Docker registry credentials for image %s for container %s",
				task.Arn, container.Image, container.Name)
			return dockerapi.DockerContainerMetadata{
				Error: dockerapi.CannotPullContainerAuthError{
					FromError: errors.New("engine docker private registry credentials: not found"),
				},
			}
		}
		defer container.SetASMDockerAuthConfig(types.AuthConfig{})
	}

	metadata := engine.client.PullImage(engine.ctx, container.Image, container.RegistryAuthentication, engine.cfg.ImagePullTimeout)

	// Don't add internal images(created by ecs-agent) into imagemanger state
	if container.IsInternal() {
		return metadata
	}
	pullSucceeded := metadata.Error == nil
	findCachedImage := false
	if !pullSucceeded {
		// If Agent failed to pull an image when
		// 1. DependentContainersPullUpfront is enabled
		// 2. ImagePullBehavior is not set to always
		// search the image in local cached images
		if engine.cfg.DependentContainersPullUpfront.Enabled() && engine.cfg.ImagePullBehavior != config.ImagePullAlwaysBehavior {
			if _, err := engine.client.InspectImage(container.Image); err != nil {
				seelog.Errorf("Task engine [%s]: failed to find cached image %s for container %s",
					task.Arn, container.Image, container.Name)
				// Stop the task if the container is an essential container,
				// and the image is not available in both remote and local caches
				if container.IsEssential() {
					task.SetDesiredStatus(apitaskstatus.TaskStopped)
					engine.emitTaskEvent(task, fmt.Sprintf("%s: %s", metadata.Error.ErrorName(), metadata.Error.Error()))
				}
				return dockerapi.DockerContainerMetadata{Error: metadata.Error}
			}
			seelog.Infof("Task engine [%s]: found cached image %s, use it directly for container %s",
				task.Arn, container.Image, container.Name)
			findCachedImage = true
		}
	}

	if pullSucceeded || findCachedImage {
		dockerContainer := &apicontainer.DockerContainer{
			Container: container,
		}
		engine.state.AddPulledContainer(dockerContainer, task)
	}

	engine.updateContainerReference(pullSucceeded, container, task.Arn)
	return metadata
}

func (engine *DockerTaskEngine) updateContainerReference(pullSucceeded bool, container *apicontainer.Container, taskArn string) {
	err := engine.imageManager.RecordContainerReference(container)
	if err != nil {
		seelog.Errorf("Task engine [%s]: unable to add container reference to image state: %v",
			taskArn, err)
	}
	imageState, ok := engine.imageManager.GetImageStateFromImageName(container.Image)
	if ok && pullSucceeded {
		// Only need to update the pullSucceeded flag of the image state when its not yet set to true.
		if !imageState.GetPullSucceeded() {
			imageState.SetPullSucceeded(true)
			err = engine.dataClient.SaveImageState(imageState)
			if err != nil {
				seelog.Warnf("Task engine [%s]: unable to save image state: %v",
					taskArn, err)
			}
		}
	}
	engine.state.AddImageState(imageState)
}

func (engine *DockerTaskEngine) createContainer(task *apitask.Task, container *apicontainer.Container) dockerapi.DockerContainerMetadata {
	seelog.Infof("Task engine [%s]: creating container: %s", task.Arn, container.Name)
	client := engine.client
	if container.DockerConfig.Version != nil {
		client = client.WithVersion(dockerclient.DockerVersion(*container.DockerConfig.Version))
	}

	dockerContainerName := ""
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		containerMap = make(map[string]*apicontainer.DockerContainer)
	} else {
		// looking for container that has docker name but not created
		for _, v := range containerMap {
			if v.Container.Name == container.Name {
				dockerContainerName = v.DockerName
				break
			}
		}
	}

	// Resolve HostConfig
	// we have to do this in create, not start, because docker no longer handles
	// merging create config with start hostconfig the same; e.g. memory limits
	// get lost
	dockerClientVersion, versionErr := client.APIVersion()
	if versionErr != nil {
		return dockerapi.DockerContainerMetadata{Error: CannotGetDockerClientVersionError{versionErr}}
	}
	hostConfig, hcerr := task.DockerHostConfig(container, containerMap, dockerClientVersion, engine.cfg)
	if hcerr != nil {
		return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(hcerr)}
	}

	if container.AWSLogAuthExecutionRole() {
		err := task.ApplyExecutionRoleLogsAuth(hostConfig, engine.credentialsManager)
		if err != nil {
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(err)}
		}
	}

	firelensConfig := container.GetFirelensConfig()
	if firelensConfig != nil {
		err := task.AddFirelensContainerBindMounts(firelensConfig, hostConfig, engine.cfg)
		if err != nil {
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(err)}
		}

		cerr := task.PopulateSecretLogOptionsToFirelensContainer(container)
		if cerr != nil {
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(cerr)}
		}

		if firelensConfig.Type == firelens.FirelensConfigTypeFluentd {
			// For fluentd router, needs to specify FLUENT_UID to root in order for the fluentd process to access
			// the socket created by Docker.
			container.MergeEnvironmentVariables(map[string]string{
				"FLUENT_UID": "0",
			})
		}
	}

	// If the container is using a special log driver type "awsfirelens", it means the container wants to use
	// the firelens container to send logs. In this case, override the log driver type to be fluentd
	// and specify appropriate tag and fluentd-address, so that the logs are sent to and routed by the firelens container.
	// Update the environment variables FLUENT_HOST and FLUENT_PORT depending on the supported network modes - bridge
	// and awsvpc. For reference - https://docs.docker.com/config/containers/logging/fluentd/.
	if hostConfig.LogConfig.Type == logDriverTypeFirelens {
		hostConfig.LogConfig = getFirelensLogConfig(task, container, hostConfig, engine.cfg)
		if task.IsNetworkModeAWSVPC() {
			container.MergeEnvironmentVariables(map[string]string{
				fluentNetworkHost: FluentAWSVPCHostValue,
				fluentNetworkPort: FluentNetworkPortValue,
			})
		} else if container.GetNetworkModeFromHostConfig() == "" || container.GetNetworkModeFromHostConfig() == apitask.BridgeNetworkMode {
			ipAddress, ok := getContainerHostIP(task.GetFirelensContainer().GetNetworkSettings())
			if !ok {
				err := apierrors.DockerClientConfigError{Msg: "unable to get BridgeIP for task in bridge  mode"}
				return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(&err)}
			}
			container.MergeEnvironmentVariables(map[string]string{
				fluentNetworkHost: ipAddress,
				fluentNetworkPort: FluentNetworkPortValue,
			})
		}
	}

	//Apply the log driver secret into container's LogConfig and Env secrets to container.Environment
	hasSecretAsEnvOrLogDriver := func(s apicontainer.Secret) bool {
		return s.Type == apicontainer.SecretTypeEnv || s.Target == apicontainer.SecretTargetLogDriver
	}
	if container.HasSecret(hasSecretAsEnvOrLogDriver) {
		err := task.PopulateSecrets(hostConfig, container)

		if err != nil {
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(err)}
		}
	}

	// Populate credentialspec resource
	if container.RequiresCredentialSpec() {
		seelog.Debugf("Obtained container %s with credentialspec resource requirement for task %s.", container.Name, task.Arn)
		var credSpecResource *credentialspec.CredentialSpecResource
		resource, ok := task.GetCredentialSpecResource()
		if !ok || len(resource) <= 0 {
			resMissingErr := &apierrors.DockerClientConfigError{Msg: "unable to fetch task resource credentialspec"}
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(resMissingErr)}
		}
		credSpecResource = resource[0].(*credentialspec.CredentialSpecResource)

		containerCredSpec, err := container.GetCredentialSpec()
		if err == nil && containerCredSpec != "" {
			// CredentialSpec mapping: input := credentialspec:file://test.json, output := credentialspec=file://test.json
			desiredCredSpecInjection, err := credSpecResource.GetTargetMapping(containerCredSpec)
			if err != nil || desiredCredSpecInjection == "" {
				missingErr := &apierrors.DockerClientConfigError{Msg: "unable to fetch valid credentialspec mapping"}
				return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(missingErr)}
			}

			// Inject containers' hostConfig.SecurityOpt with the credentialspec resource
			seelog.Infof("Injecting container %s with credentialspec %s.", container.Name, desiredCredSpecInjection)
			if len(hostConfig.SecurityOpt) == 0 {
				hostConfig.SecurityOpt = []string{desiredCredSpecInjection}
			} else {
				for idx, opt := range hostConfig.SecurityOpt {
					if strings.HasPrefix(opt, "credentialspec:") {
						hostConfig.SecurityOpt[idx] = desiredCredSpecInjection
					}
				}
			}

		} else {
			emptyErr := &apierrors.DockerClientConfigError{Msg: "unable to fetch valid credentialspec: " + err.Error()}
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(emptyErr)}
		}
	}

	if container.ShouldCreateWithEnvFiles() {
		err := task.MergeEnvVarsFromEnvfiles(container)
		if err != nil {
			seelog.Errorf("Error populating environment variables from specified files into container %s", container.Name)
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(err)}
		}
	}

	if execcmd.IsExecEnabledContainer(container) {
		tID, err := task.GetID()
		if err != nil {
			herr := &apierrors.HostConfigError{Msg: err.Error()}
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(herr)}
		}
		err = engine.execCmdMgr.InitializeContainer(tID, container, hostConfig)
		if err != nil {
			seelog.Warnf("Exec Agent initialization: %v . Continuing to start container without enabling exec feature.", err)

			// Emit a managedagent state chnage event if exec agent initialization fails
			engine.tasksLock.RLock()
			mTask, ok := engine.managedTasks[task.Arn]
			engine.tasksLock.RUnlock()
			if ok {
				mTask.emitManagedAgentEvent(mTask.Task, container, execcmd.ExecuteCommandAgentName, fmt.Sprintf("ExecuteCommandAgent Initialization failed - %v", err))
			} else {
				seelog.Errorf("Task engine [%s]: Failed to update status of ExecCommandAgent Process for container [%s]: managed task not found", task.Arn, container.Name)
			}
		}
	}

	config, err := task.DockerConfig(container, dockerClientVersion)
	if err != nil {
		return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(err)}
	}

	// Augment labels with some metadata from the agent. Explicitly do this last
	// such that it will always override duplicates in the provided raw config
	// data.
	config.Labels[labelTaskARN] = task.Arn
	config.Labels[labelContainerName] = container.Name
	config.Labels[labelTaskDefinitionFamily] = task.Family
	config.Labels[labelTaskDefinitionVersion] = task.Version
	config.Labels[labelCluster] = engine.cfg.Cluster

	if dockerContainerName == "" {
		// only alphanumeric and hyphen characters are allowed
		reInvalidChars := regexp.MustCompile("[^A-Za-z0-9-]+")
		name := reInvalidChars.ReplaceAllString(container.Name, "")

		dockerContainerName = "ecs-" + task.Family + "-" + task.Version + "-" + name + "-" + utils.RandHex()

		// Pre-add the container in case we stop before the next, more useful,
		// AddContainer call. This ensures we have a way to get the container if
		// we die before 'createContainer' returns because we can inspect by
		// name
		engine.state.AddContainer(&apicontainer.DockerContainer{
			DockerName: dockerContainerName,
			Container:  container,
		}, task)
		seelog.Infof("Task engine [%s]: created container name mapping for task:  %s -> %s",
			task.Arn, container.Name, dockerContainerName)
	}

	// Create metadata directory and file then populate it with common metadata of all containers of this task
	// Afterwards add this directory to the container's mounts if file creation was successful
	if engine.cfg.ContainerMetadataEnabled.Enabled() && !container.IsInternal() {
		info, infoErr := engine.client.Info(engine.ctx, dockerclient.InfoTimeout)
		if infoErr != nil {
			seelog.Warnf("Task engine [%s]: unable to get docker info : %v",
				task.Arn, infoErr)
		}
		mderr := engine.metadataManager.Create(config, hostConfig, task, container.Name, info.SecurityOptions)
		if mderr != nil {
			seelog.Warnf("Task engine [%s]: unable to create metadata for container %s: %v",
				task.Arn, container.Name, mderr)
		}
	}

	createContainerBegin := time.Now()
	metadata := client.CreateContainer(engine.ctx, config, hostConfig,
		dockerContainerName, engine.cfg.ContainerCreateTimeout)
	if metadata.DockerID != "" {
		seelog.Infof("Task engine [%s]: created docker container for task: %s -> %s",
			task.Arn, container.Name, metadata.DockerID)
		dockerContainer := &apicontainer.DockerContainer{DockerID: metadata.DockerID,
			DockerName: dockerContainerName,
			Container:  container}
		engine.state.AddContainer(dockerContainer, task)
		engine.saveDockerContainerData(dockerContainer)
	}
	container.SetLabels(config.Labels)
	seelog.Infof("Task engine [%s]: created docker container for task: %s -> %s, took %s",
		task.Arn, container.Name, metadata.DockerID, time.Since(createContainerBegin))
	container.SetRuntimeID(metadata.DockerID)
	return metadata
}

func getFirelensLogConfig(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig, cfg *config.Config) dockercontainer.LogConfig {
	fields := strings.Split(task.Arn, "/")
	taskID := fields[len(fields)-1]
	tag := fmt.Sprintf(fluentTagDockerFormat, container.Name, taskID)
	fluentd := socketPathPrefix + filepath.Join(cfg.DataDirOnHost, dataLogDriverPath, taskID, dataLogDriverSocketPath)
	logConfig := hostConfig.LogConfig
	bufferLimit, bufferLimitExists := logConfig.Config[apitask.FirelensLogDriverBufferLimitOption]
	logConfig.Type = logDriverTypeFluentd
	logConfig.Config = make(map[string]string)
	logConfig.Config[logDriverTag] = tag
	logConfig.Config[logDriverFluentdAddress] = fluentd
	logConfig.Config[logDriverAsyncConnect] = strconv.FormatBool(true)
	logConfig.Config[logDriverSubSecondPrecision] = strconv.FormatBool(true)
	if bufferLimitExists {
		logConfig.Config[logDriverBufferLimit] = bufferLimit
	}
	seelog.Debugf("Applying firelens log config for container %s: %v", container.Name, logConfig)
	return logConfig
}

func (engine *DockerTaskEngine) startContainer(task *apitask.Task, container *apicontainer.Container) dockerapi.DockerContainerMetadata {
	seelog.Infof("Task engine [%s]: starting container: %s (Runtime ID: %s)", task.Arn, container.Name, container.GetRuntimeID())
	client := engine.client
	if container.DockerConfig.Version != nil {
		client = client.WithVersion(dockerclient.DockerVersion(*container.DockerConfig.Version))
	}

	dockerID, err := engine.getDockerID(task, container)
	if err != nil {
		return dockerapi.DockerContainerMetadata{
			Error: dockerapi.CannotStartContainerError{
				FromError: err,
			},
		}
	}

	startContainerBegin := time.Now()
	dockerContainerMD := client.StartContainer(engine.ctx, dockerID, engine.cfg.ContainerStartTimeout)
	if dockerContainerMD.Error != nil {
		return dockerContainerMD
	}

	seelog.Infof("Task engine [%s]: started docker container for task: %s -> %s, took %s",
		task.Arn, container.Name, dockerContainerMD.DockerID, time.Since(startContainerBegin))

	// Get metadata through container inspection and available task information then write this to the metadata file
	// Performs this in the background to avoid delaying container start
	// TODO: Add a state to the apicontainer.Container for the status of the metadata file (Whether it needs update) and
	// add logic to engine state restoration to do a metadata update for containers that are running after the agent was restarted
	if engine.cfg.ContainerMetadataEnabled.Enabled() && !container.IsInternal() {
		go func() {
			err := engine.metadataManager.Update(engine.ctx, dockerID, task, container.Name)
			if err != nil {
				seelog.Warnf("Task engine [%s]: failed to update metadata file for container %s: %v",
					task.Arn, container.Name, err)
				return
			}
			container.SetMetadataFileUpdated()
			seelog.Debugf("Task engine [%s]: updated metadata file for container %s",
				task.Arn, container.Name)
		}()
	}

	// If container is a firelens container, fluent host is needed to be added to the environment variable for the task.
	// For the supported network mode - bridge and awsvpc, the awsvpc take the host 127.0.0.1 but in bridge mode,
	// there is a need to wait for the IP to be present before the container using the firelens can be created.
	if container.GetFirelensConfig() != nil {
		if !task.IsNetworkModeAWSVPC() && (container.GetNetworkModeFromHostConfig() == "" || container.GetNetworkModeFromHostConfig() == apitask.BridgeNetworkMode) {
			_, gotContainerIP := getContainerHostIP(dockerContainerMD.NetworkSettings)
			if !gotContainerIP {
				getIPBridgeBackoff := retry.NewExponentialBackoff(minGetIPBridgeTimeout, maxGetIPBridgeTimeout, getIPBridgeRetryJitterMultiplier, getIPBridgeRetryDelayMultiplier)
				contextWithTimeout, cancel := context.WithTimeout(engine.ctx, time.Minute)
				defer cancel()
				err := retry.RetryWithBackoffCtx(contextWithTimeout, getIPBridgeBackoff, func() error {
					inspectOutput, err := engine.client.InspectContainer(engine.ctx, dockerContainerMD.DockerID,
						dockerclient.InspectContainerTimeout)
					if err != nil {
						return err
					}
					_, gotIPBridge := getContainerHostIP(inspectOutput.NetworkSettings)
					if gotIPBridge {
						dockerContainerMD.NetworkSettings = inspectOutput.NetworkSettings
						return nil
					} else {
						return errors.New("Bridge IP not available to use for firelens")
					}
				})
				if err != nil {
					return dockerapi.DockerContainerMetadata{
						Error: dockerapi.CannotStartContainerError{FromError: err},
					}
				}
			}

		}
	}
	if execcmd.IsExecEnabledContainer(container) {
		if ma, _ := container.GetManagedAgentByName(execcmd.ExecuteCommandAgentName); !ma.InitFailed {
			reason := "ExecuteCommandAgent started"
			if err := engine.execCmdMgr.StartAgent(engine.ctx, engine.client, task, container, dockerID); err != nil {
				reason = err.Error()
				seelog.Errorf("Task engine [%s]: Failed to start ExecCommandAgent Process for container [%s]: %v", task.Arn, container.Name, err)
			}

			engine.tasksLock.RLock()
			mTask, ok := engine.managedTasks[task.Arn]
			engine.tasksLock.RUnlock()
			// whether we started or failed to start, we'll want to emit a state change event
			// redundant state change events like RUNNING->RUNNING are allowed
			if ok {
				mTask.emitManagedAgentEvent(mTask.Task, container, execcmd.ExecuteCommandAgentName, reason)
			} else {
				seelog.Errorf("Task engine [%s]: Failed to update status of ExecCommandAgent Process for container [%s]: managed task not found", task.Arn, container.Name)
			}
		}
	}

	// On Windows, we need to invoke CNI plugins for all containers
	// invokePluginsForContainer will return nil for other platforms
	if dockerContainerMD.Error == nil && task.IsNetworkModeAWSVPC() && !container.IsInternal() {
		err := engine.invokePluginsForContainer(task, container)
		if err != nil {
			return dockerapi.DockerContainerMetadata{
				Error: ContainerNetworkingError{
					fromError: errors.Wrapf(err, "startContainer: cni plugin invocation failed"),
				},
			}
		}
	}

	return dockerContainerMD
}

func (engine *DockerTaskEngine) provisionContainerResources(task *apitask.Task, container *apicontainer.Container) dockerapi.DockerContainerMetadata {
	seelog.Infof("Task engine [%s]: setting up container resources for container [%s]",
		task.Arn, container.Name)
	containerInspectOutput, err := engine.inspectContainer(task, container)
	if err != nil {
		return dockerapi.DockerContainerMetadata{
			Error: ContainerNetworkingError{
				fromError: errors.Wrap(err,
					"container resource provisioning: cannot setup task network namespace due to error inspecting pause container"),
			},
		}
	}

	task.SetPausePIDInVolumeResources(strconv.Itoa(containerInspectOutput.State.Pid))

	cniConfig, err := engine.buildCNIConfigFromTaskContainer(task, containerInspectOutput, true)
	if err != nil {
		return dockerapi.DockerContainerMetadata{
			Error: ContainerNetworkingError{
				fromError: errors.Wrap(err,
					"container resource provisioning: unable to build cni configuration"),
			},
		}
	}

	// Invoke the libcni to config the network namespace for the container
	result, err := engine.cniClient.SetupNS(engine.ctx, cniConfig, cniSetupTimeout)
	if err != nil {
		seelog.Errorf("Task engine [%s]: unable to configure pause container namespace: %v",
			task.Arn, err)
		return dockerapi.DockerContainerMetadata{
			DockerID: cniConfig.ContainerID,
			Error: ContainerNetworkingError{errors.Wrap(err,
				"container resource provisioning: failed to setup network namespace")},
		}
	}

	// This is the IP of the task assigned on the bridge for IAM Task roles
	taskIP := result.IPs[0].Address.IP.String()
	seelog.Infof("Task engine [%s]: associated with ip address '%s'", task.Arn, taskIP)
	engine.state.AddTaskIPAddress(taskIP, task.Arn)
	task.SetLocalIPAddress(taskIP)
	engine.saveTaskData(task)

	// Invoke additional commands required to configure the task namespace routing.
	err = engine.namespaceHelper.ConfigureTaskNamespaceRouting(engine.ctx, task.GetPrimaryENI(), cniConfig, result)
	if err != nil {
		seelog.Errorf("Task engine [%s]: unable to configure pause container namespace: %v",
			task.Arn, err)
		return dockerapi.DockerContainerMetadata{
			DockerID: cniConfig.ContainerID,
			Error: ContainerNetworkingError{errors.Wrapf(err,
				"container resource provisioning: failed to setup network namespace")},
		}
	}

	return dockerapi.DockerContainerMetadata{
		DockerID: cniConfig.ContainerID,
	}
}

// checkTearDownPauseContainer idempotently tears down the pause container network when the pause container's known
//or desired status is stopped.
func (engine *DockerTaskEngine) checkTearDownPauseContainer(task *apitask.Task) {
	if !task.IsNetworkModeAWSVPC() {
		return
	}
	for _, container := range task.Containers {
		// Cleanup the pause container network namespace before stop the container
		if container.Type == apicontainer.ContainerCNIPause {
			// Clean up if the pause container has stopped or will stop
			if container.KnownTerminal() || container.DesiredTerminal() {
				err := engine.cleanupPauseContainerNetwork(task, container)
				if err != nil {
					seelog.Errorf("Task engine [%s]: unable to cleanup pause container network namespace: %v", task.Arn, err)
				}
			}
			return
		}
	}
}

// cleanupPauseContainerNetwork will clean up the network namespace of pause container
func (engine *DockerTaskEngine) cleanupPauseContainerNetwork(task *apitask.Task, container *apicontainer.Container) error {
	// This operation is idempotent
	if container.IsContainerTornDown() {
		return nil
	}
	delay := time.Duration(engine.cfg.ENIPauseContainerCleanupDelaySeconds) * time.Second
	if engine.handleDelay != nil && delay > 0 {
		seelog.Infof("Task engine [%s]: waiting %s before cleaning up pause container.", task.Arn, delay)
		engine.handleDelay(delay)
	}
	containerInspectOutput, err := engine.inspectContainer(task, container)
	if err != nil {
		return errors.Wrap(err, "engine: cannot cleanup task network namespace due to error inspecting pause container")
	}

	seelog.Infof("Task engine [%s]: cleaning up the network namespace", task.Arn)
	cniConfig, err := engine.buildCNIConfigFromTaskContainer(task, containerInspectOutput, false)
	if err != nil {
		return errors.Wrapf(err,
			"engine: failed cleanup task network namespace, task: %s", task.String())
	}

	err = engine.cniClient.CleanupNS(engine.ctx, cniConfig, cniCleanupTimeout)
	if err != nil {
		return err
	}

	container.SetContainerTornDown(true)
	seelog.Infof("Task engine [%s]: cleaned pause container network namespace", task.Arn)
	return nil
}

// buildCNIConfigFromTaskContainer builds a CNI config for the task and container.
func (engine *DockerTaskEngine) buildCNIConfigFromTaskContainer(
	task *apitask.Task,
	containerInspectOutput *types.ContainerJSON,
	includeIPAMConfig bool) (*ecscni.Config, error) {
	cniConfig := &ecscni.Config{
		BlockInstanceMetadata:    engine.cfg.AWSVPCBlockInstanceMetdata.Enabled(),
		MinSupportedCNIVersion:   config.DefaultMinSupportedCNIVersion,
		InstanceENIDNSServerList: engine.cfg.InstanceENIDNSServerList,
	}
	if engine.cfg.OverrideAWSVPCLocalIPv4Address != nil &&
		len(engine.cfg.OverrideAWSVPCLocalIPv4Address.IP) != 0 &&
		len(engine.cfg.OverrideAWSVPCLocalIPv4Address.Mask) != 0 {
		cniConfig.IPAMV4Address = engine.cfg.OverrideAWSVPCLocalIPv4Address
	}
	if len(engine.cfg.AWSVPCAdditionalLocalRoutes) != 0 {
		cniConfig.AdditionalLocalRoutes = engine.cfg.AWSVPCAdditionalLocalRoutes
	}

	cniConfig.ContainerPID = strconv.Itoa(containerInspectOutput.State.Pid)
	cniConfig.ContainerID = containerInspectOutput.ID
	cniConfig.ContainerNetNS = ""

	// For pause containers, NetNS would be none
	// For other containers, NetNS would be of format container:<pause_container_ID>
	if containerInspectOutput.HostConfig.NetworkMode.IsNone() {
		cniConfig.ContainerNetNS = containerInspectOutput.HostConfig.NetworkMode.NetworkName()
	} else if containerInspectOutput.HostConfig.NetworkMode.IsContainer() {
		cniConfig.ContainerNetNS = fmt.Sprintf("container:%s", containerInspectOutput.HostConfig.NetworkMode.ConnectedContainer())
	} else {
		return nil, errors.New("engine: failed to build cni configuration from the task due to invalid container network namespace")
	}

	cniConfig, err := task.BuildCNIConfig(includeIPAMConfig, cniConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "engine: failed to build cni configuration from task")
	}

	return cniConfig, nil
}

func (engine *DockerTaskEngine) inspectContainer(task *apitask.Task, container *apicontainer.Container) (*types.ContainerJSON, error) {
	dockerID, err := engine.getDockerID(task, container)
	if err != nil {
		return nil, err
	}

	return engine.client.InspectContainer(engine.ctx, dockerID, dockerclient.InspectContainerTimeout)
}

func (engine *DockerTaskEngine) stopContainer(task *apitask.Task, container *apicontainer.Container) dockerapi.DockerContainerMetadata {
	seelog.Infof("Task engine [%s]: stopping container [%s]", task.Arn, container.Name)

	dockerID, err := engine.getDockerID(task, container)
	if err != nil {
		return dockerapi.DockerContainerMetadata{
			Error: dockerapi.CannotStopContainerError{
				FromError: err,
			},
		}
	}

	// Cleanup the pause container network namespace before stop the container
	if container.Type == apicontainer.ContainerCNIPause {
		err := engine.cleanupPauseContainerNetwork(task, container)
		if err != nil {
			seelog.Errorf("Task engine [%s]: unable to cleanup pause container network namespace: %v",
				task.Arn, err)
		}
	}

	apiTimeoutStopContainer := container.GetStopTimeout()
	if apiTimeoutStopContainer <= 0 {
		apiTimeoutStopContainer = engine.cfg.DockerStopTimeout
	}

	return engine.stopDockerContainer(dockerID, container.Name, apiTimeoutStopContainer)
}

// stopDockerContainer attempts to stop the container, retrying only in case of time out errors.
// If the maximum number of retries is reached, the container is marked as stopped. This is because docker sometimes
// deadlocks when trying to stop a container but the actual container process is stopped.
// for more information, see: https://github.com/moby/moby/issues/41587
func (engine *DockerTaskEngine) stopDockerContainer(dockerID, containerName string, apiTimeoutStopContainer time.Duration) dockerapi.DockerContainerMetadata {
	var md dockerapi.DockerContainerMetadata
	backoff := newExponentialBackoff(engine.stopContainerBackoffMin, engine.stopContainerBackoffMax, stopContainerBackoffJitter, stopContainerBackoffMultiplier)
	for i := 0; i < stopContainerMaxRetryCount; i++ {
		md = engine.client.StopContainer(engine.ctx, dockerID, apiTimeoutStopContainer)
		if md.Error == nil {
			return md
		}
		cannotStopContainerError, ok := md.Error.(cannotStopContainerError)
		if ok && !cannotStopContainerError.IsRetriableError() {
			return md
		}

		if i < stopContainerMaxRetryCount-1 {
			retryIn := backoff.Duration()
			logger.Warn(fmt.Sprintf("Error stopping container, retrying in %v", retryIn), logger.Fields{
				field.Container: containerName,
				field.RuntimeID: dockerID,
				field.Error:     md.Error,
				"attempt":       i + 1,
			})
			time.Sleep(retryIn)
		}
	}
	return md
}

func (engine *DockerTaskEngine) removeContainer(task *apitask.Task, container *apicontainer.Container) error {
	seelog.Infof("Task engine [%s]: removing container: %s", task.Arn, container.Name)

	dockerID, err := engine.getDockerID(task, container)
	if err != nil {
		return err
	}

	return engine.client.RemoveContainer(engine.ctx, dockerID, dockerclient.RemoveContainerTimeout)
}

// updateTaskUnsafe determines if a new transition needs to be applied to the
// referenced task, and if needed applies it. It should not be called anywhere
// but from 'AddTask' and is protected by the tasksLock lock there.
func (engine *DockerTaskEngine) updateTaskUnsafe(task *apitask.Task, update *apitask.Task) {
	managedTask, ok := engine.managedTasks[task.Arn]
	if !ok {
		seelog.Criticalf("Task engine [%s]: ACS message for a task we thought we managed, but don't!  Aborting.",
			task.Arn)
		return
	}
	// Keep the lock because sequence numbers cannot be correct unless they are
	// also read in the order addtask was called
	// This does block the engine's ability to ingest any new events (including
	// stops for past tasks, ack!), but this is necessary for correctness
	updateDesiredStatus := update.GetDesiredStatus()
	seelog.Debugf("Task engine [%s]: putting update on the acs channel: [%s] with seqnum [%d]",
		task.Arn, updateDesiredStatus.String(), update.StopSequenceNumber)
	managedTask.emitACSTransition(acsTransition{
		desiredStatus: updateDesiredStatus,
		seqnum:        update.StopSequenceNumber,
	})
	seelog.Debugf("Task engine [%s]: update taken off the acs channel: [%s] with seqnum [%d]",
		task.Arn, updateDesiredStatus.String(), update.StopSequenceNumber)
}

// transitionContainer calls applyContainerState, and then notifies the managed
// task of the change. transitionContainer is called by progressTask and
// by handleStoppedToRunningContainerTransition.
func (engine *DockerTaskEngine) transitionContainer(task *apitask.Task, container *apicontainer.Container, to apicontainerstatus.ContainerStatus) {
	// Let docker events operate async so that we can continue to handle ACS / other requests
	// This is safe because 'applyContainerState' will not mutate the task
	metadata := engine.applyContainerState(task, container, to)

	engine.tasksLock.RLock()
	managedTask, ok := engine.managedTasks[task.Arn]
	engine.tasksLock.RUnlock()
	if ok {
		managedTask.emitDockerContainerChange(dockerContainerChange{
			container: container,
			event: dockerapi.DockerContainerChangeEvent{
				Status:                  to,
				DockerContainerMetadata: metadata,
			},
		})
	}
}

// applyContainerState moves the container to the given state by calling the
// function defined in the transitionFunctionMap for the state
func (engine *DockerTaskEngine) applyContainerState(task *apitask.Task, container *apicontainer.Container, nextState apicontainerstatus.ContainerStatus) dockerapi.DockerContainerMetadata {
	transitionFunction, ok := engine.transitionFunctionMap()[nextState]
	if !ok {
		seelog.Criticalf("Task engine [%s]: unsupported desired state transition for container [%s]: %s",
			task.Arn, container.Name, nextState.String())
		return dockerapi.DockerContainerMetadata{Error: &impossibleTransitionError{nextState}}
	}
	metadata := transitionFunction(task, container)
	if metadata.Error != nil {
		seelog.Infof("Task engine [%s]: error transitioning container [%s (Runtime ID: %s)] to [%s]: %v",
			task.Arn, container.Name, container.GetRuntimeID(), nextState.String(), metadata.Error)
	} else {
		seelog.Debugf("Task engine [%s]: transitioned container [%s (Runtime ID: %s)] to [%s]",
			task.Arn, container.Name, container.GetRuntimeID(), nextState.String())
	}
	return metadata
}

// transitionFunctionMap provides the logic for the simple state machine of the
// DockerTaskEngine. Each desired state maps to a function that can be called
// to try and move the task to that desired state.
func (engine *DockerTaskEngine) transitionFunctionMap() map[apicontainerstatus.ContainerStatus]transitionApplyFunc {
	return engine.containerStatusToTransitionFunction
}

type transitionApplyFunc (func(*apitask.Task, *apicontainer.Container) dockerapi.DockerContainerMetadata)

// State is a function primarily meant for testing usage; it is explicitly not
// part of the TaskEngine interface and should not be relied upon.
// It returns an internal representation of the state of this DockerTaskEngine.
func (engine *DockerTaskEngine) State() dockerstate.TaskEngineState {
	return engine.state
}

// Version returns the underlying docker version.
func (engine *DockerTaskEngine) Version() (string, error) {
	return engine.client.Version(engine.ctx, dockerclient.VersionTimeout)
}

func (engine *DockerTaskEngine) updateMetadataFile(task *apitask.Task, cont *apicontainer.DockerContainer) {
	err := engine.metadataManager.Update(engine.ctx, cont.DockerID, task, cont.Container.Name)
	if err != nil {
		seelog.Errorf("Task engine [%s]: failed to update metadata file for container %s: %v",
			task.Arn, cont.Container.Name, err)
	} else {
		cont.Container.SetMetadataFileUpdated()
		seelog.Debugf("Task engine [%s]: updated metadata file for container %s",
			task.Arn, cont.Container.Name)
	}
}

func getContainerHostIP(networkSettings *types.NetworkSettings) (string, bool) {
	if networkSettings == nil {
		return "", false
	} else if networkSettings.IPAddress != "" {
		return networkSettings.IPAddress, true
	} else if len(networkSettings.Networks) > 0 {
		for mode, network := range networkSettings.Networks {
			if mode == apitask.BridgeNetworkMode && network.IPAddress != "" {
				return network.IPAddress, true
			}
		}
	}
	return "", false
}

func (engine *DockerTaskEngine) getDockerID(task *apitask.Task, container *apicontainer.Container) (string, error) {
	runtimeID := container.GetRuntimeID()
	if runtimeID != "" {
		return runtimeID, nil
	}
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		return "", errors.Errorf("container name=%s belongs to unrecognized task taskArn=%s", container.Name, task.Arn)
	}

	dockerContainer, ok := containerMap[container.Name]
	if !ok {
		return "", errors.Errorf("container name=%s not recognized by agent", container.Name)
	}

	if dockerContainer.DockerID == "" {
		return dockerContainer.DockerName, nil
	}
	return dockerContainer.DockerID, nil
}
