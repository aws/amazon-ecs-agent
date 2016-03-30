// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// The DockerTaskEngine is an abstraction over the DockerGoClient so that
// it does not have to know about tasks, only containers
package engine

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	utilsync "github.com/aws/amazon-ecs-agent/agent/utils/sync"
	"github.com/cihub/seelog"
)

const (
	DOCKER_ENDPOINT_ENV_VARIABLE = "DOCKER_HOST"
	DOCKER_DEFAULT_ENDPOINT      = "unix:///var/run/docker.sock"
	capabilityPrefix             = "com.amazonaws.ecs.capability."
)

// The DockerTaskEngine interacts with docker to implement a task
// engine
type DockerTaskEngine struct {
	// implements TaskEngine

	cfg                *config.Config
	acceptInsecureCert bool

	initialized  bool
	mustInitLock sync.Mutex

	// state stores all tasks this task engine is aware of, including their
	// current state and mappings to/from dockerId and name.
	// This is used to checkpoint state to disk so tasks may survive agent
	// failures or updates
	state        *dockerstate.DockerTaskEngineState
	managedTasks map[string]*managedTask

	taskStopGroup *utilsync.SequentialWaitGroup

	events          <-chan DockerContainerChangeEvent
	containerEvents chan api.ContainerStateChange
	taskEvents      chan api.TaskStateChange
	saver           statemanager.Saver

	client     DockerClient
	clientLock sync.Mutex

	stopEngine context.CancelFunc

	// processTasks is a mutex that the task engine must aquire before changing
	// any task's state which it manages. Since this is a lock that encompasses
	// all tasks, it must not aquire it for any significant duration
	// The write mutex should be taken when adding and removing tasks from managedTasks.
	processTasks sync.RWMutex
}

// NewDockerTaskEngine returns a created, but uninitialized, DockerTaskEngine.
// The distinction between created and initialized is that when created it may
// be serialized/deserialized, but it will not communicate with docker until it
// is also initialized.
func NewDockerTaskEngine(cfg *config.Config, acceptInsecureCert bool) *DockerTaskEngine {
	dockerTaskEngine := &DockerTaskEngine{
		cfg:                cfg,
		acceptInsecureCert: acceptInsecureCert,
		client:             nil,
		saver:              statemanager.NewNoopStateManager(),

		state:         dockerstate.NewDockerTaskEngineState(),
		managedTasks:  make(map[string]*managedTask),
		taskStopGroup: utilsync.NewSequentialWaitGroup(),

		containerEvents: make(chan api.ContainerStateChange),
		taskEvents:      make(chan api.TaskStateChange),
	}

	return dockerTaskEngine
}

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
func (engine *DockerTaskEngine) Init() error {
	err := engine.initDockerClient()
	if err != nil {
		return err
	}

	// TODO, pass in a a context from main from background so that other things can stop us, not just the tests
	ctx, cancel := context.WithCancel(context.TODO())
	engine.stopEngine = cancel
	// Open the event stream before we sync state so that e.g. if a container
	// goes from running to stopped after we sync with it as "running" we still
	// have the "went to stopped" event pending so we can be up to date.
	err = engine.openEventstream(ctx)
	if err != nil {
		return err
	}
	engine.synchronizeState()
	// Now catch up and start processing new events per normal
	go engine.handleDockerEvents(ctx)
	engine.initialized = true
	return nil
}

func (engine *DockerTaskEngine) initDockerClient() error {
	if engine.client != nil {
		return nil
	}

	engine.clientLock.Lock()
	defer engine.clientLock.Unlock()
	if engine.client != nil {
		return nil
	}
	client, err := NewDockerGoClient(nil, engine.cfg.EngineAuthType, engine.cfg.EngineAuthData, engine.acceptInsecureCert)
	if err != nil {
		return err
	}
	engine.client = client

	return nil
}

// SetDockerClient provides a way to override the client used for communication with docker as a testing hook.
func (engine *DockerTaskEngine) SetDockerClient(client DockerClient) {
	engine.clientLock.Lock()
	engine.clientLock.Unlock()
	engine.client = client
}

// MustInit blocks and retries until an engine can be initialized.
func (engine *DockerTaskEngine) MustInit() {
	if engine.initialized {
		return
	}
	engine.mustInitLock.Lock()
	defer engine.mustInitLock.Unlock()

	errorOnce := sync.Once{}
	taskEngineConnectBackoff := utils.NewSimpleBackoff(200*time.Millisecond, 2*time.Second, 0.20, 1.5)
	utils.RetryWithBackoff(taskEngineConnectBackoff, func() error {
		if engine.initialized {
			return nil
		}
		err := engine.Init()
		if err != nil {
			errorOnce.Do(func() {
				log.Error("Could not connect to docker daemon", "err", err)
			})
		}
		return err
	})
}

func (engine *DockerTaskEngine) SetSaver(saver statemanager.Saver) {
	engine.saver = saver
}

// Shutdown makes a best-effort attempt to cleanup after the task engine.
// This should not be relied on for anything more complicated than testing.
func (engine *DockerTaskEngine) Shutdown() {
	engine.stopEngine()
	engine.Disable()
}

// Disable prevents this engine from managing any additional tasks.
func (engine *DockerTaskEngine) Disable() {
	engine.processTasks.Lock()
}

// synchronizeState explicitly goes through each docker container stored in
// "state" and updates its KnownStatus appropriately, as well as queueing up
// events to push upstream.
func (engine *DockerTaskEngine) synchronizeState() {
	engine.processTasks.Lock()
	defer engine.processTasks.Unlock()

	tasks := engine.state.AllTasks()
	for _, task := range tasks {
		conts, ok := engine.state.ContainerMapByArn(task.Arn)
		if !ok {
			engine.startTask(task)
			continue
		}
		for _, cont := range conts {
			if cont.DockerId == "" {
				log.Debug("Found container potentially created while we were down", "name", cont.DockerName)
				// Figure out the dockerid
				describedCont, err := engine.client.InspectContainer(cont.DockerName)
				if err != nil {
					log.Warn("Could not find matching container for expected", "name", cont.DockerName)
				} else {
					cont.DockerId = describedCont.ID
					// update mappings that need dockerid
					engine.state.AddContainer(cont, task)
				}
			}
			if cont.DockerId != "" {
				currentState, metadata := engine.client.DescribeContainer(cont.DockerId)
				if metadata.Error != nil {
					currentState = api.ContainerStopped
					if !cont.Container.KnownTerminal() {
						cont.Container.ApplyingError = api.NewNamedError(&ContainerVanishedError{})
						log.Warn("Could not describe previously known container; assuming dead", "err", metadata.Error, "id", cont.DockerId, "name", cont.DockerName)
					}
				}
				if currentState > cont.Container.KnownStatus {
					cont.Container.KnownStatus = currentState
				}
			}
		}
		engine.startTask(task)
	}
	engine.saver.Save()
}

// CheckTaskState inspects the state of all containers within a task and writes
// their state to the managed task's container channel.
func (engine *DockerTaskEngine) CheckTaskState(task *api.Task) {
	taskContainers, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		log.Warn("Could not check task state for task; no task in state", "task", task)
		return
	}
	for _, container := range task.Containers {
		dockerContainer, ok := taskContainers[container.Name]
		if !ok {
			continue
		}
		status, metadata := engine.client.DescribeContainer(dockerContainer.DockerId)
		engine.processTasks.RLock()
		managedTask, ok := engine.managedTasks[task.Arn]
		engine.processTasks.RUnlock()

		if ok {
			managedTask.dockerMessages <- dockerContainerChange{
				container: container,
				event: DockerContainerChangeEvent{
					Status:                  status,
					DockerContainerMetadata: metadata,
				},
			}
		}
	}
}

// sweepTask deletes all the containers associated with a task
func (engine *DockerTaskEngine) sweepTask(task *api.Task) {
	for _, cont := range task.Containers {
		err := engine.removeContainer(task, cont)
		if err != nil {
			log.Debug("Unable to remove old container", "err", err, "task", task, "cont", cont)
		}
	}
}

func (engine *DockerTaskEngine) emitTaskEvent(task *api.Task, reason string) {
	if !task.KnownStatus.BackendRecognized() {
		return
	}
	if task.SentStatus >= task.KnownStatus {
		log.Debug("Already sent task event; no need to re-send", "task", task.Arn, "event", task.KnownStatus.String())
		return
	}
	event := api.TaskStateChange{
		TaskArn:    task.Arn,
		Status:     task.KnownStatus,
		Reason:     reason,
		SentStatus: &task.SentStatus,
	}
	log.Info("Task change event", "event", event)
	engine.taskEvents <- event
}

// startTask creates a managedTask construct to track the task and then begins
// pushing it towards its desired state when allowed startTask is protected by
// the processTasks lock of 'AddTask'. It should not be called from anywhere
// else and should exit quickly to allow AddTask to do more work.
func (engine *DockerTaskEngine) startTask(task *api.Task) {
	// Create a channel that may be used to communicate with this task, survey
	// what tasks need to be waited for for this one to start, and then spin off
	// a goroutine to oversee this task

	thisTask := engine.newManagedTask(task)

	go thisTask.overseeTask()
}

// emitContainerEvent passes a given event up through the containerEvents channel if necessary.
// It will omit events the backend would not process and will perform best-effort deduplication of events.
func (engine *DockerTaskEngine) emitContainerEvent(task *api.Task, cont *api.Container, reason string) {
	if !cont.KnownStatus.BackendRecognized() {
		return
	}
	if cont.IsInternal {
		return
	}
	if cont.SentStatus >= cont.KnownStatus {
		log.Debug("Already sent container event; no need to re-send", "task", task.Arn, "container", cont.Name, "event", cont.KnownStatus.String())
		return
	}

	if reason == "" && cont.ApplyingError != nil {
		reason = cont.ApplyingError.Error()
	}
	event := api.ContainerStateChange{
		TaskArn:       task.Arn,
		ContainerName: cont.Name,
		Status:        cont.KnownStatus,
		ExitCode:      cont.KnownExitCode,
		PortBindings:  cont.KnownPortBindings,
		Reason:        reason,
		SentStatus:    &cont.SentStatus,
	}
	log.Debug("Container change event", "event", event)
	engine.containerEvents <- event
	log.Debug("Container change event passed on", "event", event)
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
			log.Debug("Handling a docker event", "event", event)

			task, task_found := engine.state.TaskById(event.DockerId)
			cont, container_found := engine.state.ContainerById(event.DockerId)
			if !task_found || !container_found {
				log.Debug("Event for container not managed", "dockerId", event.DockerId)
				break
			}
			engine.processTasks.RLock()
			managedTask, ok := engine.managedTasks[task.Arn]
			engine.processTasks.RUnlock()
			if !ok {
				log.Crit("Could not find managed task corresponding to a docker event", "event", event, "task", task)
				continue
			}
			log.Debug("Writing docker event to the associated task", "task", task, "event", event)
			managedTask.dockerMessages <- dockerContainerChange{container: cont.Container, event: event}
			log.Debug("Wrote docker event to the associated task", "task", task, "event", event)
		}
	}
}

// TaskEvents returns channels to read task and container state changes. These
// changes should be read as soon as possible as them not being read will block
// processing the task referenced by the event.
func (engine *DockerTaskEngine) TaskEvents() (<-chan api.TaskStateChange, <-chan api.ContainerStateChange) {
	return engine.taskEvents, engine.containerEvents
}

func (engine *DockerTaskEngine) AddTask(task *api.Task) error {
	task.PostUnmarshalTask()

	engine.processTasks.Lock()
	defer engine.processTasks.Unlock()

	existingTask, exists := engine.state.TaskByArn(task.Arn)
	if !exists {
		engine.state.AddTask(task)
		engine.startTask(task)
	} else {
		engine.updateTask(existingTask, task)
	}

	return nil
}

type transitionApplyFunc (func(*api.Task, *api.Container) DockerContainerMetadata)

func tryApplyTransition(task *api.Task, container *api.Container, to api.ContainerStatus, f transitionApplyFunc) DockerContainerMetadata {
	return f(task, container)
}

func (engine *DockerTaskEngine) ListTasks() ([]*api.Task, error) {
	return engine.state.AllTasks(), nil
}

func (engine *DockerTaskEngine) GetTaskByArn(arn string) (*api.Task, bool) {
	return engine.state.TaskByArn(arn)
}

func (engine *DockerTaskEngine) pullContainer(task *api.Task, container *api.Container) DockerContainerMetadata {
	log.Info("Pulling container", "task", task, "container", container)
	return engine.client.PullImage(container.Image, container.RegistryAuthentication)
}

func (engine *DockerTaskEngine) createContainer(task *api.Task, container *api.Container) DockerContainerMetadata {
	log.Info("Creating container", "task", task, "container", container)
	client := engine.client
	if container.DockerConfig.Version != nil {
		client = client.WithVersion(dockerclient.DockerVersion(*container.DockerConfig.Version))
	}

	// Resolve HostConfig
	// we have to do this in create, not start, because docker no longer handles
	// merging create config with start hostconfig the same; e.g. memory limits
	// get lost
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		containerMap = make(map[string]*api.DockerContainer)
	}

	hostConfig, hcerr := task.DockerHostConfig(container, containerMap)
	if hcerr != nil {
		return DockerContainerMetadata{Error: api.NamedError(hcerr)}
	}

	config, err := task.DockerConfig(container)
	if err != nil {
		return DockerContainerMetadata{Error: api.NamedError(err)}
	}

	name := ""
	for i := 0; i < len(container.Name); i++ {
		c := container.Name[i]
		if !((c <= '9' && c >= '0') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c == '-')) {
			continue
		}
		name += string(c)
	}

	containerName := "ecs-" + task.Family + "-" + task.Version + "-" + name + "-" + utils.RandHex()

	// Pre-add the container in case we stop before the next, more useful,
	// AddContainer call. This ensures we have a way to get the container if
	// we die before 'createContainer' returns because we can inspect by
	// name
	engine.state.AddContainer(&api.DockerContainer{DockerName: containerName, Container: container}, task)
	seelog.Infof("Created container name mapping for task %s - %s -> %s", task, container, containerName)
	engine.saver.ForceSave()

	metadata := client.CreateContainer(config, hostConfig, containerName)
	if metadata.DockerId != "" {
		engine.state.AddContainer(&api.DockerContainer{DockerId: metadata.DockerId, DockerName: containerName, Container: container}, task)
	}
	seelog.Infof("Created docker container for task %s: %s -> %s", task, container, metadata.DockerId)
	return metadata
}

func (engine *DockerTaskEngine) startContainer(task *api.Task, container *api.Container) DockerContainerMetadata {
	log.Info("Starting container", "task", task, "container", container)
	client := engine.client
	if container.DockerConfig.Version != nil {
		client = client.WithVersion(dockerclient.DockerVersion(*container.DockerConfig.Version))
	}

	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		return DockerContainerMetadata{Error: CannotXContainerError{"Start", "Container belongs to unrecognized task " + task.Arn}}
	}

	dockerContainer, ok := containerMap[container.Name]
	if !ok {
		return DockerContainerMetadata{Error: CannotXContainerError{"Start", "Container not recorded as created"}}
	}
	return client.StartContainer(dockerContainer.DockerId)
}

func (engine *DockerTaskEngine) stopContainer(task *api.Task, container *api.Container) DockerContainerMetadata {
	log.Info("Stopping container", "task", task, "container", container)
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		return DockerContainerMetadata{Error: CannotXContainerError{"Stop", "Container belongs to unrecognized task " + task.Arn}}
	}

	dockerContainer, ok := containerMap[container.Name]
	if !ok {
		return DockerContainerMetadata{Error: CannotXContainerError{"Stop", "Container not recorded as created"}}
	}

	return engine.client.StopContainer(dockerContainer.DockerId)
}

func (engine *DockerTaskEngine) removeContainer(task *api.Task, container *api.Container) error {
	log.Info("Removing container", "task", task, "container", container)
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)

	if !ok {
		return errors.New("No such task: " + task.Arn)
	}

	dockerContainer, ok := containerMap[container.Name]
	if !ok {
		return errors.New("No container named '" + container.Name + "' created in " + task.Arn)
	}

	return engine.client.RemoveContainer(dockerContainer.DockerId)
}

// updateTask determines if a new transition needs to be applied to the
// referenced task, and if needed applies it. It should not be called anywhere
// but from 'AddTask' and is protected by the processTasks lock there.
func (engine *DockerTaskEngine) updateTask(task *api.Task, update *api.Task) {
	managedTask, ok := engine.managedTasks[task.Arn]
	if !ok {
		log.Crit("ACS message for a task we thought we managed, but don't!", "arn", task.Arn)
		// Is this the right thing to do?
		// Calling startTask should overwrite our bad 'state' data with the new
		// task which we do manage.. but this is still scary and shouldn't have happened
		engine.startTask(update)
		return
	}
	// Keep the lock because sequence numbers cannot be correct unless they are
	// also read in the order addtask was called
	// This does block the engine's ability to ingest any new events (including
	// stops for past tasks, ack!), but this is necessary for correctness
	log.Debug("Putting update on the acs channel", "task", task.Arn, "status", update.DesiredStatus, "seqnum", update.StopSequenceNumber)
	transition := acsTransition{desiredStatus: update.DesiredStatus}
	transition.seqnum = update.StopSequenceNumber
	managedTask.acsMessages <- transition
	log.Debug("Update was taken off the acs channel", "task", task.Arn, "status", update.DesiredStatus)
}

func (engine *DockerTaskEngine) transitionFunctionMap() map[api.ContainerStatus]transitionApplyFunc {
	return map[api.ContainerStatus]transitionApplyFunc{
		api.ContainerPulled:  engine.pullContainer,
		api.ContainerCreated: engine.createContainer,
		api.ContainerRunning: engine.startContainer,
		api.ContainerStopped: engine.stopContainer,
	}
}

// applyContainerState moves the container to the given state
func (engine *DockerTaskEngine) applyContainerState(task *api.Task, container *api.Container, nextState api.ContainerStatus) DockerContainerMetadata {
	clog := log.New("task", task, "container", container)
	transitionFunction, ok := engine.transitionFunctionMap()[nextState]
	if !ok {
		clog.Crit("Container desired to transition to an unsupported state", "state", nextState.String())
		return DockerContainerMetadata{Error: &impossibleTransitionError{nextState}}
	}

	metadata := tryApplyTransition(task, container, nextState, transitionFunction)
	if metadata.Error != nil {
		clog.Info("Error transitioning container", "state", nextState.String())
	} else {
		clog.Debug("Transitioned container", "state", nextState.String())
		engine.saver.Save()
	}
	return metadata
}

func (engine *DockerTaskEngine) transitionContainer(task *api.Task, container *api.Container, to api.ContainerStatus) {
	// Let docker events operate async so that we can continue to handle ACS / other requests
	// This is safe because 'applyContainerState' will not mutate the task
	metadata := engine.applyContainerState(task, container, to)

	engine.processTasks.RLock()
	managedTask, ok := engine.managedTasks[task.Arn]
	engine.processTasks.RUnlock()
	if ok {
		managedTask.dockerMessages <- dockerContainerChange{
			container: container,
			event: DockerContainerChangeEvent{
				Status:                  to,
				DockerContainerMetadata: metadata,
			},
		}
	}
}

// State is a function primarily meant for testing usage; it is explicitly not
// part of the TaskEngine interface and should not be relied upon.
// It returns an internal representation of the state of this DockerTaskEngine.
func (engine *DockerTaskEngine) State() *dockerstate.DockerTaskEngineState {
	return engine.state
}

// Capabilities returns the supported capabilities of this agent / docker-client pair.
// Currently, the following capabilities are possible:
//
//    com.amazonaws.ecs.capability.privileged-container
//    com.amazonaws.ecs.capability.docker-remote-api.1.17
//    com.amazonaws.ecs.capability.docker-remote-api.1.18
//    com.amazonaws.ecs.capability.docker-remote-api.1.19
//    com.amazonaws.ecs.capability.docker-remote-api.1.20
//    com.amazonaws.ecs.capability.logging-driver.json-file
//    com.amazonaws.ecs.capability.logging-driver.syslog
//    com.amazonaws.ecs.capability.logging-driver.fluentd
//    com.amazonaws.ecs.capability.logging-driver.journald
//    com.amazonaws.ecs.capability.logging-driver.gelf
//    com.amazonaws.ecs.capability.selinux
//    com.amazonaws.ecs.capability.apparmor
func (engine *DockerTaskEngine) Capabilities() []string {
	err := engine.initDockerClient()
	if err != nil {
		return nil
	}
	capabilities := []string{}
	if !engine.cfg.PrivilegedDisabled {
		capabilities = append(capabilities, capabilityPrefix+"privileged-container")
	}
	versions := make(map[dockerclient.DockerVersion]bool)
	for _, version := range engine.client.SupportedVersions() {
		capabilities = append(capabilities, capabilityPrefix+"docker-remote-api."+string(version))
		versions[version] = true
	}

	for _, loggingDriver := range engine.cfg.AvailableLoggingDrivers {
		requiredVersion := dockerclient.LoggingDriverMinimumVersion[loggingDriver]
		if _, ok := versions[requiredVersion]; ok {
			capabilities = append(capabilities, capabilityPrefix+"logging-driver."+string(loggingDriver))
		}
	}

	if engine.cfg.SELinuxCapable {
		capabilities = append(capabilities, capabilityPrefix+"selinux")
	}
	if engine.cfg.AppArmorCapable {
		capabilities = append(capabilities, capabilityPrefix+"apparmor")
	}

	if _, ok := versions[dockerclient.Version_1_19]; ok {
		capabilities = append(capabilities, capabilityPrefix+"ecr-auth")
	}

	return capabilities
}

// Version returns the underlying docker version.
func (engine *DockerTaskEngine) Version() (string, error) {
	// Must be able to be called before Init()
	err := engine.initDockerClient()
	if err != nil {
		return "", err
	}
	return engine.client.Version()
}
