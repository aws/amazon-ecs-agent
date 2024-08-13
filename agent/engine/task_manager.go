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

package engine

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/csiclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime"
)

const (
	// waitForPullCredentialsTimeout is the timeout agent trying to wait for pull
	// credentials from acs, after the timeout it will check the credentials manager
	// and start processing the task or start another round of waiting
	waitForPullCredentialsTimeout            = 1 * time.Minute
	systemPingTimeout                        = 5 * time.Second
	defaultTaskSteadyStatePollInterval       = 5 * time.Minute
	defaultTaskSteadyStatePollIntervalJitter = 30 * time.Second
	transitionPollTime                       = 5 * time.Second
	stoppedSentWaitInterval                  = 30 * time.Second
	maxStoppedWaitTimes                      = 72 * time.Hour / stoppedSentWaitInterval
	taskUnableToTransitionToStoppedReason    = "TaskStateError: Agent could not progress task's state to stopped"
	// unstage retries are ultimately limited by successful unstage or by the unstageVolumeTimeout
	unstageVolumeTimeout = 30 * time.Second
	// substantial min/max accommodate a csi-driver outage
	unstageBackoffMin      = 5 * time.Second
	unstageBackoffMax      = 10 * time.Second
	unstageBackoffJitter   = 0.0
	unstageBackoffMultiple = 1.5
	unstageRetryAttempts   = 5
)

var (
	_stoppedSentWaitInterval = stoppedSentWaitInterval
	_maxStoppedWaitTimes     = int(maxStoppedWaitTimes)
)

type dockerContainerChange struct {
	container *apicontainer.Container
	event     dockerapi.DockerContainerChangeEvent
}

// resourceStateChange represents the required status change after resource transition
type resourceStateChange struct {
	resource  taskresource.TaskResource
	nextState resourcestatus.ResourceStatus
	err       error
}

type acsTransition struct {
	seqnum        int64
	desiredStatus apitaskstatus.TaskStatus
}

// containerTransition defines the struct for a container to transition
type containerTransition struct {
	nextState      apicontainerstatus.ContainerStatus
	actionRequired bool
	blockedOn      *apicontainer.DependsOn
	reason         dependencygraph.DependencyError
}

// resourceTransition defines the struct for a resource to transition.
type resourceTransition struct {
	// nextState represents the next known status that the resource can move to
	nextState resourcestatus.ResourceStatus
	// status is the string value of nextState
	status string
	// actionRequired indicates if the transition function needs to be called for
	// the transition to be complete
	actionRequired bool
	// reason represents the error blocks transition
	reason error
}

// managedTask is a type that is meant to manage the lifecycle of a task.
// There should be only one managed task construct for a given task arn and the
// managed task should be the only thing to modify the task's known or desired statuses.
//
// The managedTask should run serially in a single goroutine in which it reads
// messages from the two given channels and acts upon them.
// This design is chosen to allow a safe level if isolation and avoid any race
// conditions around the state of a task.
// The data sources (e.g. docker, acs) that write to the task's channels may
// block, and it is expected that the managedTask listen to those channels
// almost constantly.
// The general operation should be:
//  1. Listen to the channels
//  2. On an event, update the status of the task and containers (known/desired)
//  3. Figure out if any action needs to be done. If so, do it
//  4. GOTO 1
//
// Item '3' obviously might lead to some duration where you are not listening
// to the channels. However, this can be solved by kicking off '3' as a
// goroutine and then only communicating the result back via the channels
// (obviously once you kick off a goroutine you give up the right to write the
// task's statuses yourself)
type managedTask struct {
	*apitask.Task
	ctx    context.Context
	cancel context.CancelFunc

	engine             *DockerTaskEngine
	cfg                *config.Config
	credentialsManager credentials.Manager
	cniClient          ecscni.CNIClient
	dockerClient       dockerapi.DockerClient

	acsMessages                chan acsTransition
	dockerMessages             chan dockerContainerChange
	resourceStateChangeEvent   chan resourceStateChange
	stateChangeEvents          chan statechange.Event
	consumedHostResourceEvent  chan struct{}
	containerChangeEventStream *eventstream.EventStream

	// unexpectedStart is a once that controls stopping a container that
	// unexpectedly started one time.
	// This exists because a 'start' after a container is meant to be stopped is
	// possible under some circumstances (e.g. a timeout). However, if it
	// continues to 'start' when we aren't asking it to, let it go through in
	// case it's a user trying to debug it or in case we're fighting with another
	// thing managing the container.
	unexpectedStart sync.Once

	_time     ttime.Time
	_timeOnce sync.Once

	// steadyStatePollInterval is the duration that a managed task waits
	// once the task gets into steady state before polling the state of all
	// the task's containers to re-evaluate if the task is still in steady state
	// This is set to defaultTaskSteadyStatePollInterval in production code.
	// This can be used by tests that are looking to ensure that the steady state
	// verification logic gets executed to set it to a low interval
	steadyStatePollInterval       time.Duration
	steadyStatePollIntervalJitter time.Duration
}

// newManagedTask is a method on DockerTaskEngine to create a new managedTask.
// This method must only be called when the engine.processTasks write lock is
// already held.
func (engine *DockerTaskEngine) newManagedTask(task *apitask.Task) *managedTask {
	ctx, cancel := context.WithCancel(engine.ctx)
	t := &managedTask{
		ctx:                           ctx,
		cancel:                        cancel,
		Task:                          task,
		acsMessages:                   make(chan acsTransition),
		dockerMessages:                make(chan dockerContainerChange),
		resourceStateChangeEvent:      make(chan resourceStateChange),
		consumedHostResourceEvent:     make(chan struct{}, 1),
		engine:                        engine,
		cfg:                           engine.cfg,
		stateChangeEvents:             engine.stateChangeEvents,
		containerChangeEventStream:    engine.containerChangeEventStream,
		credentialsManager:            engine.credentialsManager,
		cniClient:                     engine.cniClient,
		dockerClient:                  engine.client,
		steadyStatePollInterval:       engine.taskSteadyStatePollInterval,
		steadyStatePollIntervalJitter: engine.taskSteadyStatePollIntervalJitter,
	}
	engine.managedTasks[task.Arn] = t
	return t
}

// overseeTask is the main goroutine of the managedTask. It runs an infinite
// loop of receiving messages and attempting to take action based on those
// messages.
func (mtask *managedTask) overseeTask() {
	// Do a single UpdateStatus at the beginning to create the container
	// `DesiredStatus`es which are a construct of the engine used only here,
	// not present on the backend
	mtask.UpdateStatus()

	// Wait here until enough resources are available on host for the task to progress
	// - Waits until host resource manager successfully 'consume's task resources and returns
	// - For tasks which have crossed this stage before (on agent restarts), resources are pre-consumed - returns immediately
	// - If the task is already stopped (knownStatus is STOPPED), does not attempt to consume resources - returns immediately
	// - If an ACS StopTask arrives, host resources manager returns immediately. Host resource manager does not consume resources
	// (resources are later 'release'd on Stopped task emitTaskEvent call)
	mtask.waitForHostResources()

	// If this was a 'state restore', send all unsent statuses
	mtask.emitCurrentStatus()

	// Main infinite loop. This is where we receive messages and dispatch work.
	for {
		if mtask.shouldExit() {
			return
		}

		// If it's steadyState, just spin until we need to do work
		for mtask.steadyState() {
			mtask.waitSteady()
		}

		if mtask.shouldExit() {
			return
		}

		if !mtask.GetKnownStatus().Terminal() {
			// If we aren't terminal, and we aren't steady state, we should be
			// able to move some containers along.
			logger.Debug("Task not steady state or terminal; progressing it", logger.Fields{
				field.TaskID: mtask.GetID(),
			})

			mtask.progressTask()
		}

		if mtask.GetKnownStatus().Terminal() {
			break
		}
	}
	// We only break out of the above if this task is known to be stopped. Do
	// onetime cleanup here, including removing the task after a timeout
	logger.Info("Managed task has reached stopped; waiting for container cleanup", logger.Fields{
		field.TaskID: mtask.GetID(),
	})
	mtask.engine.checkTearDownPauseContainer(mtask.Task)
	// TODO [SC]: We need to also tear down pause containets in bridge mode for SC-enabled tasks
	mtask.cleanupCredentials()
	// Send event to monitor queue task routine to check for any pending tasks to progress
	mtask.engine.wakeUpTaskQueueMonitor()
	// TODO: make this idempotent on agent restart
	go mtask.releaseIPInIPAM()

	if mtask.Task.IsEBSTaskAttachEnabled() {
		csiClient := csiclient.NewCSIClient(filepath.Join(csiclient.DefaultSocketHostPath, csiclient.DefaultImageName,
			csiclient.DefaultSocketName))
		errors := mtask.UnstageVolumes(&csiClient)
		for _, err := range errors {
			logger.Error(fmt.Sprintf("Unable to unstage volumes: %v", err))
		}
	}

	mtask.cleanupTask(retry.AddJitter(mtask.cfg.TaskCleanupWaitDuration, mtask.cfg.TaskCleanupWaitDurationJitter))
}

// shouldExit checks if the task manager should exit, as the agent is exiting.
func (mtask *managedTask) shouldExit() bool {
	select {
	case <-mtask.ctx.Done():
		return true
	default:
		return false
	}
}

// emitCurrentStatus emits a container event for every container and a task
// event for the task
func (mtask *managedTask) emitCurrentStatus() {
	for _, container := range mtask.Containers {
		mtask.emitContainerEvent(mtask.Task, container, "")
	}
	mtask.emitTaskEvent(mtask.Task, "")
}

// waitForHostResources waits for host resources to become available to start
// the task. It will wait for event on this task's consumedHostResourceEvent
// channel from monitorQueuedTasks routine to wake up
func (mtask *managedTask) waitForHostResources() {
	if mtask.GetKnownStatus().Terminal() {
		// Task's known status is STOPPED. No need to wait in this case and proceed to clean up
		// This is relevant when agent restarts and a task has stopped - do not attempt
		// to consume resources in host resource manager
		return
	}

	if !mtask.IsLaunchTypeFargate() && !mtask.IsInternal && !mtask.engine.hostResourceManager.checkTaskConsumed(mtask.Arn) {
		// Internal tasks are started right away as their resources are not accounted for
		// Fargate (1.3.0) - rely on backend instance placement and skip resource accounting
		mtask.engine.enqueueTask(mtask)
		for !mtask.waitEvent(mtask.consumedHostResourceEvent) {
			if mtask.GetDesiredStatus().Terminal() {
				// If we end up here, that means we received a start then stop for this
				// task before a task that was expected to stop before it could
				// actually stop
				break
			}
		}
	}
}

// waitSteady waits for a task to leave steady-state by waiting for a new
// event, or a timeout.
func (mtask *managedTask) waitSteady() {
	logger.Debug("Managed task at steady state", logger.Fields{
		field.TaskID:      mtask.GetID(),
		field.KnownStatus: mtask.GetKnownStatus().String(),
	})

	timeoutCtx, cancel := context.WithTimeout(mtask.ctx, retry.AddJitter(mtask.steadyStatePollInterval,
		mtask.steadyStatePollIntervalJitter))
	defer cancel()
	timedOut := mtask.waitEvent(timeoutCtx.Done())
	if mtask.shouldExit() {
		return
	}

	if timedOut {
		logger.Debug("Checking to verify it's still at steady state", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
		go mtask.engine.checkTaskState(mtask.Task)
	}
}

// steadyState returns if the task is in a steady state. Steady state is when task's desired
// and known status are both RUNNING
func (mtask *managedTask) steadyState() bool {
	select {
	case <-mtask.ctx.Done():
		logger.Info("Task manager exiting", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
		return false
	default:
		taskKnownStatus := mtask.GetKnownStatus()
		return taskKnownStatus == apitaskstatus.TaskRunning && taskKnownStatus >= mtask.GetDesiredStatus()
	}
}

// cleanupCredentials removes credentials for a stopped task (execution credentials are removed in cleanupTask
// due to its potential usage in the later phase of the task cleanup such as sending logs)
func (mtask *managedTask) cleanupCredentials() {
	taskCredentialsID := mtask.GetCredentialsID()
	if taskCredentialsID != "" {
		mtask.credentialsManager.RemoveCredentials(taskCredentialsID)
	}
}

// waitEvent waits for any event to occur. If an event occurs, the appropriate
// handler is called. Generally the stopWaiting arg is the context's Done
// channel. When the Done channel is signalled by the context, waitEvent will
// return true.
func (mtask *managedTask) waitEvent(stopWaiting <-chan struct{}) bool {
	logger.Debug("Waiting for task event", logger.Fields{
		field.TaskID: mtask.GetID(),
	})
	select {
	case acsTransition := <-mtask.acsMessages:
		logger.Info("Managed task got acs event", logger.Fields{
			field.TaskID:        mtask.GetID(),
			field.DesiredStatus: acsTransition.desiredStatus.String(),
			field.Sequence:      acsTransition.seqnum,
		})
		mtask.handleDesiredStatusChange(acsTransition.desiredStatus, acsTransition.seqnum)
		return false
	case dockerChange := <-mtask.dockerMessages:
		mtask.handleContainerChange(dockerChange)
		return false
	case resChange := <-mtask.resourceStateChangeEvent:
		res := resChange.resource
		logger.Info("Managed task got resource", logger.Fields{
			field.TaskID:   mtask.GetID(),
			field.Resource: res.GetName(),
			field.Status:   res.StatusString(resChange.nextState),
		})
		mtask.handleResourceStateChange(resChange)
		return false
	case <-stopWaiting:
		return true
	}
}

// handleDesiredStatusChange updates the desired status on the task. Updates
// only occur if the new desired status is "compatible" (farther along than the
// current desired state); "redundant" (less-than or equal desired states) are
// ignored and dropped.
func (mtask *managedTask) handleDesiredStatusChange(desiredStatus apitaskstatus.TaskStatus, seqnum int64) {
	// Handle acs message changes this task's desired status to whatever
	// acs says it should be if it is compatible

	// Isolate change of desired status updates from monitorQueuedTasks processing to prevent
	// unexpected updates to host resource manager when tasks are being processed by monitorQueuedTasks
	// For example when ACS StopTask event updates arrives and simultaneously monitorQueuedTasks
	// could be processing
	mtask.engine.monitorQueuedTasksLock.Lock()
	defer mtask.engine.monitorQueuedTasksLock.Unlock()

	logger.Info("New acs transition", logger.Fields{
		field.TaskID:        mtask.GetID(),
		field.DesiredStatus: desiredStatus.String(),
		field.Sequence:      seqnum,
	})
	if desiredStatus <= mtask.GetDesiredStatus() {
		logger.Debug("Redundant task transition; ignoring", logger.Fields{
			field.TaskID:        mtask.GetID(),
			field.DesiredStatus: desiredStatus.String(),
			field.Sequence:      seqnum,
		})
		return
	}
	mtask.SetDesiredStatus(desiredStatus)
	mtask.UpdateDesiredStatus()
	mtask.engine.saveTaskData(mtask.Task)
}

// handleContainerChange updates a container's known status. If the message
// contains any interesting information (like exit codes or ports), they are
// propagated.
func (mtask *managedTask) handleContainerChange(containerChange dockerContainerChange) {
	// locate the container
	container := containerChange.container
	runtimeID := container.GetRuntimeID()
	event := containerChange.event
	containerKnownStatus := container.GetKnownStatus()

	eventLogFields := logger.Fields{
		field.TaskID:        mtask.GetID(),
		field.Container:     container.Name,
		field.RuntimeID:     runtimeID,
		"changeEventStatus": event.Status.String(),
		field.KnownStatus:   containerKnownStatus.String(),
		field.DesiredStatus: container.GetDesiredStatus().String(),
	}

	if event.Status.String() == apicontainerstatus.ContainerStatusNone.String() ||
		(event.Status == apicontainerstatus.ContainerRunning &&
			containerKnownStatus == apicontainerstatus.ContainerResourcesProvisioned) {
		logger.Debug("Handling container change event", eventLogFields)
	} else if event.Status != containerKnownStatus { // This prevents noisy docker event logs for pause container
		logger.Info("Handling container change event", eventLogFields)
	}
	found := mtask.isContainerFound(container)
	if !found {
		logger.Critical("State error; invoked with another task's container!", eventLogFields)
		return
	}

	// If this is a backwards transition stopped->running, the first time set it
	// to be known running so it will be stopped. Subsequently, ignore these backward transitions
	mtask.handleStoppedToRunningContainerTransition(event.Status, container)
	if event.Status <= containerKnownStatus {
		logger.Debug("Container change is redundant", eventLogFields)

		// Only update container metadata when status stays RUNNING
		if event.Status == containerKnownStatus && event.Status == apicontainerstatus.ContainerRunning {
			updateContainerMetadata(&event.DockerContainerMetadata, container, mtask.Task)
		}
		return
	}

	// Container has progressed its status if we reach here. Make sure to save it to database.
	defer mtask.engine.saveContainerData(container)

	// If container is transitioning to STOPPED, first check if we should short-circuit
	// the stop workflow and restart the container.
	if event.Status == apicontainerstatus.ContainerStopped && container.RestartPolicyEnabled() {
		exitCode := event.DockerContainerMetadata.ExitCode
		shouldRestart, reason := container.RestartTracker.ShouldRestart(exitCode, container.GetStartedAt(),
			container.GetDesiredStatus())
		if shouldRestart {
			container.RestartTracker.RecordRestart()
			resp := mtask.engine.startContainer(mtask.Task, container)
			if resp.Error == nil {
				logger.Info("Restarted container", eventLogFields,
					logger.Fields{
						"restartCount":  container.RestartTracker.GetRestartCount(),
						"lastRestartAt": container.RestartTracker.GetLastRestartAt().UTC().Format(time.RFC3339),
					})
				// return here because we have now restarted the container, and we don't
				// want to complete the rest of the "container stop" workflow
				return
			}
			logger.Error("Restart container failed", eventLogFields,
				logger.Fields{
					"restartCount":  container.RestartTracker.GetRestartCount(),
					"lastRestartAt": container.RestartTracker.GetLastRestartAt().UTC().Format(time.RFC3339),
					field.Error:     resp.Error,
				})
		} else {
			logger.Info("Not Restarting container", eventLogFields,
				logger.Fields{
					"restartCount":  container.RestartTracker.GetRestartCount(),
					"lastRestartAt": container.RestartTracker.GetLastRestartAt().UTC().Format(time.RFC3339),
					"reason":        reason,
				})
		}
	}

	// Update the container to be known
	currentKnownStatus := containerKnownStatus
	container.SetKnownStatus(event.Status)
	updateContainerMetadata(&event.DockerContainerMetadata, container, mtask.Task)

	if event.Error != nil {
		proceedAnyway := mtask.handleEventError(containerChange, currentKnownStatus)
		if !proceedAnyway {
			return
		}
	}

	if execcmd.IsExecEnabledContainer(container) && container.GetKnownStatus() == apicontainerstatus.ContainerStopped {
		// if this is an execute-command-enabled container STOPPED event, we should emit a corresponding managedAgent event
		mtask.handleManagedAgentStoppedTransition(container, execcmd.ExecuteCommandAgentName)
	}

	mtask.RecordExecutionStoppedAt(container)
	logger.Debug("Sending container change event to tcs", eventLogFields)
	err := mtask.containerChangeEventStream.WriteToEventStream(event)
	if err != nil {
		logger.Warn("Failed to write container change event to tcs event stream",
			eventLogFields,
			logger.Fields{
				field.Error: err,
			})
	}

	mtask.emitContainerEvent(mtask.Task, container, "")
	if mtask.UpdateStatus() {
		// If knownStatus changed, let it be known
		var taskStateChangeReason string
		if mtask.GetKnownStatus().Terminal() {
			taskStateChangeReason = mtask.Task.GetTerminalReason()
		}
		mtask.emitTaskEvent(mtask.Task, taskStateChangeReason)
		// Save the new task status to database.
		mtask.engine.saveTaskData(mtask.Task)
	}
}

// handleResourceStateChange attempts to update resource's known status depending on
// the current status and errors during transition
func (mtask *managedTask) handleResourceStateChange(resChange resourceStateChange) {
	// locate the resource
	res := resChange.resource
	if !mtask.isResourceFound(res) {
		logger.Critical("State error; invoked with another task's resource",
			logger.Fields{
				field.TaskID:   mtask.GetID(),
				field.Resource: res.GetName(),
			})
		return
	}

	status := resChange.nextState
	err := resChange.err
	currentKnownStatus := res.GetKnownStatus()

	if status <= currentKnownStatus {
		logger.Info("Redundant resource state change", logger.Fields{
			field.TaskID:      mtask.GetID(),
			field.Resource:    res.GetName(),
			field.Status:      res.StatusString(status),
			field.KnownStatus: res.StatusString(currentKnownStatus),
		})
		return
	}

	// There is a resource state change. Resource is stored as part of the task, so make sure to save the task
	// at the end.
	defer mtask.engine.saveTaskData(mtask.Task)

	// Set known status regardless of error so the applied status can be cleared. If there is error,
	// the known status might be set again below (but that won't affect the applied status anymore).
	// This follows how container state change is handled.
	res.SetKnownStatus(status)
	if err == nil {
		return
	}

	if status == res.SteadyState() { // Failed to create resource.
		logger.Error("Failed to create task resource", logger.Fields{
			field.TaskID:   mtask.GetID(),
			field.Resource: res.GetName(),
			field.Error:    err,
		})
		res.SetKnownStatus(currentKnownStatus) // Set status back to None.

		logger.Info("Marking task desired status to STOPPED", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
		mtask.SetDesiredStatus(apitaskstatus.TaskStopped)
		mtask.Task.SetTerminalReason(res.GetTerminalReason())
	}
}

func (mtask *managedTask) emitResourceChange(change resourceStateChange) {
	select {
	case <-mtask.ctx.Done():
		logger.Info("Unable to emit resource state change due to exit", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
	case mtask.resourceStateChangeEvent <- change:
	}
}

func getContainerEventLogFields(c api.ContainerStateChange) logger.Fields {
	f := logger.Fields{
		"ContainerName": c.ContainerName,
	}
	if c.ExitCode != nil {
		f["Exit"] = strconv.Itoa(*c.ExitCode)
	}
	if c.Reason != "" {
		f["Reason"] = c.Reason
	}
	if len(c.PortBindings) != 0 {
		f["Ports"] = fmt.Sprintf("%v", c.PortBindings)
	}
	if c.Container != nil {
		f["KnownSent"] = c.Container.GetSentStatus().String()
	}
	f["KnownStatus"] = c.Container.GetKnownStatus()
	return f
}

func (mtask *managedTask) emitTaskEvent(task *apitask.Task, reason string) {
	taskKnownStatus := task.GetKnownStatus()
	// Always do (idempotent) release host resources whenever state change with
	// known status == STOPPED is done to ensure sync between tasks and host resource manager
	if taskKnownStatus.Terminal() {
		resourcesToRelease := mtask.ToHostResources()
		err := mtask.engine.hostResourceManager.release(mtask.Arn, resourcesToRelease)
		if err != nil {
			logger.Critical("Failed to release resources after task stopped",
				logger.Fields{field.TaskARN: mtask.Arn})
		}
	}
	if taskKnownStatus != apitaskstatus.TaskManifestPulled && !taskKnownStatus.BackendRecognized() {
		logger.Debug("Skipping event emission for task", logger.Fields{
			field.TaskID:      mtask.GetID(),
			field.Error:       "status not recognized by ECS",
			field.KnownStatus: taskKnownStatus.String(),
		})
		return
	}
	event, err := api.NewTaskStateChangeEvent(task, reason)
	if err != nil {
		if _, ok := err.(api.ErrShouldNotSendEvent); ok {
			logger.Debug(err.Error(), logger.Fields{field.TaskID: mtask.GetID()})
		} else {
			logger.Error("Skipping emitting event for task due to error", logger.Fields{
				field.TaskID: mtask.GetID(),
				field.Reason: reason,
				field.Error:  err,
			})
		}
		return
	}
	logger.Debug("Sending task change event", logger.Fields{
		field.TaskID:     mtask.GetID(),
		field.Status:     event.Status.String(),
		field.SentStatus: task.GetSentStatus().String(),
		field.Event:      event.String(),
	})
	select {
	case <-mtask.ctx.Done():
		logger.Info("Unable to send task change event due to exit", logger.Fields{
			field.TaskID:     mtask.GetID(),
			field.Status:     event.Status.String(),
			field.SentStatus: task.GetSentStatus().String(),
			field.Event:      event.String(),
		})
	case mtask.stateChangeEvents <- event:
	}
	logger.Debug("Sent task change event", logger.Fields{
		field.TaskID: mtask.GetID(),
		field.Event:  event.String(),
	})
}

// emitManagedAgentEvent passes a special task event up through the taskEvents channel if there are managed
// agent changes in the container passed as parameter.
// It will omit events the backend would not process
func (mtask *managedTask) emitManagedAgentEvent(task *apitask.Task, cont *apicontainer.Container,
	managedAgentName string, reason string) {
	event, err := api.NewManagedAgentChangeEvent(task, cont, managedAgentName, reason)
	if err != nil {
		logger.Error("Skipping emitting ManagedAgent event for task", logger.Fields{
			field.TaskID: mtask.GetID(),
			field.Reason: reason,
			field.Error:  err,
		})
		return
	}
	logger.Info("Sending ManagedAgent event", logger.Fields{
		field.TaskID: mtask.GetID(),
		field.Event:  event.String(),
	})
	select {
	case <-mtask.ctx.Done():
		logger.Info("Unable to send managed agent event due to exit", logger.Fields{
			field.TaskID: mtask.GetID(),
			field.Event:  event.String(),
		})
	case mtask.stateChangeEvents <- event:
	}
	logger.Info("Sent managed agent event [%s]", logger.Fields{
		field.TaskID: mtask.GetID(),
		field.Event:  event.String(),
	})
}

// emitContainerEvent passes a given event up through the containerEvents channel if necessary.
// It will omit events the backend would not process and will perform best-effort deduplication of events.
func (mtask *managedTask) emitContainerEvent(task *apitask.Task, cont *apicontainer.Container, reason string) {
	event, err := api.NewContainerStateChangeEvent(task, cont, reason)
	if err != nil {
		if _, ok := err.(api.ErrShouldNotSendEvent); ok {
			logger.Debug(err.Error(), logger.Fields{
				field.TaskID:    mtask.GetID(),
				field.Container: cont.Name,
			})
		} else {
			logger.Error("Skipping emitting event for container due to error", logger.Fields{
				field.TaskID:    mtask.GetID(),
				field.Container: cont.Name,
				field.Error:     err,
			})
		}
		return
	}
	mtask.doEmitContainerEvent(event)
}

func (mtask *managedTask) doEmitContainerEvent(event api.ContainerStateChange) {
	logger.Debug("Sending container change event", getContainerEventLogFields(event), logger.Fields{
		field.TaskID: mtask.GetID(),
	})
	select {
	case <-mtask.ctx.Done():
		logger.Info("Unable to send container change event due to exit", getContainerEventLogFields(event),
			logger.Fields{
				field.TaskID: mtask.GetID(),
			})
	case mtask.stateChangeEvents <- event:
	}
	logger.Debug("Sent container change event", getContainerEventLogFields(event), logger.Fields{
		field.TaskID: mtask.GetID(),
	})
}

func (mtask *managedTask) emitDockerContainerChange(change dockerContainerChange) {
	select {
	case <-mtask.ctx.Done():
		logger.Info("Unable to emit docker container change due to exit", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
	case mtask.dockerMessages <- change:
	}
}

func (mtask *managedTask) emitACSTransition(transition acsTransition) {
	select {
	case <-mtask.ctx.Done():
		logger.Info("Unable to emit docker container change due to exit", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
	case mtask.acsMessages <- transition:
	}
}

func (mtask *managedTask) isContainerFound(container *apicontainer.Container) bool {
	found := false
	for _, c := range mtask.Containers {
		if container == c {
			found = true
			break
		}
	}
	return found
}

func (mtask *managedTask) isResourceFound(res taskresource.TaskResource) bool {
	for _, r := range mtask.GetResources() {
		if res.GetName() == r.GetName() {
			return true
		}
	}
	return false
}

// releaseIPInIPAM releases the IP address used by the task in awsvpc mode.
func (mtask *managedTask) releaseIPInIPAM() {
	if !mtask.IsNetworkModeAWSVPC() {
		return
	}
	logger.Info("IPAM releasing ip for task eni", logger.Fields{
		field.TaskID: mtask.GetID(),
	})

	cfg, err := mtask.BuildCNIConfigAwsvpc(true, &ecscni.Config{
		MinSupportedCNIVersion: config.DefaultMinSupportedCNIVersion,
	})
	if err != nil {
		logger.Error("Failed to release ip; unable to build cni configuration", logger.Fields{
			field.TaskID: mtask.GetID(),
			field.Error:  err,
		})
		return
	}
	err = mtask.cniClient.ReleaseIPResource(mtask.ctx, cfg, ipamCleanupTmeout)
	if err != nil {
		logger.Error("Failed to release ip; IPAM error", logger.Fields{
			field.TaskID: mtask.GetID(),
			field.Error:  err,
		})
		return
	}
}

// handleStoppedToRunningContainerTransition detects a "backwards" container
// transition where a known-stopped container is found to be running again and
// handles it.
func (mtask *managedTask) handleStoppedToRunningContainerTransition(status apicontainerstatus.ContainerStatus,
	container *apicontainer.Container) {
	containerKnownStatus := container.GetKnownStatus()
	if status > containerKnownStatus {
		// Event status is greater than container's known status.
		// This is not a backward transition, return
		return
	}
	if containerKnownStatus != apicontainerstatus.ContainerStopped {
		// Container's known status is not STOPPED. Nothing to do here.
		return
	}
	if !status.IsRunning() {
		// Container's 'to' transition was not either of RUNNING or RESOURCES_PROVISIONED
		// states. Nothing to do in this case as well
		return
	}
	// If the container becomes running after we've stopped it (possibly
	// because we got an error running it, and it ran anyway), the first time
	// update it to 'known running' so that it will be driven back to stopped
	mtask.unexpectedStart.Do(func() {
		logger.Warn("Stopped container came back; re-stopping it once", logger.Fields{
			field.TaskID:    mtask.GetID(),
			field.Container: container.Name,
		})
		go mtask.engine.transitionContainer(mtask.Task, container, apicontainerstatus.ContainerStopped)
		// This will not proceed afterwards because status <= knownstatus below
	})
}

// handleManagedAgentStoppedTransition handles a container change event which has a managed agent status
// we should emit ManagedAgent events for certain container events.
func (mtask *managedTask) handleManagedAgentStoppedTransition(container *apicontainer.Container, managedAgentName string) {
	//for now, we only have the ExecuteCommandAgent
	switch managedAgentName {
	case execcmd.ExecuteCommandAgentName:
		if !container.UpdateManagedAgentStatus(managedAgentName, apicontainerstatus.ManagedAgentStopped) {
			logger.Warn("Cannot find ManagedAgent for container", logger.Fields{
				field.TaskID:       mtask.GetID(),
				field.Container:    container.Name,
				field.ManagedAgent: managedAgentName,
			})

		}
		mtask.emitManagedAgentEvent(mtask.Task, container, managedAgentName, "Received Container Stopped event")
	default:
		logger.Warn("Unexpected ManagedAgent in container; unable to process ManagedAgent transition event",
			logger.Fields{
				field.TaskID:       mtask.GetID(),
				field.Container:    container.Name,
				field.ManagedAgent: managedAgentName,
			})
	}
}

// handleEventError handles a container change event error and decides whether
// we should proceed to transition the container
func (mtask *managedTask) handleEventError(containerChange dockerContainerChange,
	currentKnownStatus apicontainerstatus.ContainerStatus) bool {
	container := containerChange.container
	event := containerChange.event
	if container.ApplyingError == nil {
		container.ApplyingError = apierrors.NewNamedError(event.Error)
	}
	switch event.Status {
	// event.Status is the desired container transition from container's known status
	// (* -> event.Status)
	case apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerPulled:
		// If the agent pull behavior is always or once, we receive the error because
		// the image or manifest pull fails, the task should fail. If we don't fail task here,
		// then the cached image will probably be used for creating container, and we
		// don't want to use cached image for both cases.
		if mtask.cfg.ImagePullBehavior == config.ImagePullAlwaysBehavior ||
			mtask.cfg.ImagePullBehavior == config.ImagePullOnceBehavior {
			logger.Error("Error while pulling image or its manifest; moving task to STOPPED", logger.Fields{
				field.TaskID:    mtask.GetID(),
				field.Image:     container.Image,
				field.Container: container.Name,
				field.Error:     event.Error,
				field.Status:    event.Status,
			})
			// The task should be stopped regardless of whether this container is
			// essential or non-essential.
			mtask.SetDesiredStatus(apitaskstatus.TaskStopped)
			return false
		}
		// If the agent pull behavior is prefer-cached, we receive the error because
		// the image or manifest pull fails and there is no cached image in local, we don't make
		// the task fail here, will let create container handle it instead.
		// If the agent pull behavior is default, use local image cache directly,
		// assuming it exists.
		logger.Error("Error while pulling image or its manifest; will try to run anyway", logger.Fields{
			field.TaskID:    mtask.GetID(),
			field.Image:     container.Image,
			field.Container: container.Name,
			field.Error:     event.Error,
			field.Status:    event.Status,
		})
		// proceed anyway
		return true
	case apicontainerstatus.ContainerStopped:
		// Container's desired transition was to 'STOPPED'
		return mtask.handleContainerStoppedTransitionError(event, container, currentKnownStatus)
	case apicontainerstatus.ContainerStatusNone:
		fallthrough
	case apicontainerstatus.ContainerCreated:
		// No need to explicitly stop containers if this is a * -> NONE/CREATED transition
		logger.Warn("Error creating container; marking its desired status as STOPPED", logger.Fields{
			field.TaskID:    mtask.GetID(),
			field.Container: container.Name,
			field.Error:     event.Error,
		})
		container.SetKnownStatus(currentKnownStatus)
		container.SetDesiredStatus(apicontainerstatus.ContainerStopped)
		return false
	default:
		// If this is a * -> RUNNING / RESOURCES_PROVISIONED transition, we need to stop
		// the container.
		logger.Warn("Error starting/provisioning container[%s (Runtime ID: %s)];", logger.Fields{
			field.TaskID:    mtask.GetID(),
			field.Container: container.Name,
			field.RuntimeID: container.GetRuntimeID(),
			field.Error:     event.Error,
		})
		container.SetKnownStatus(currentKnownStatus)
		container.SetDesiredStatus(apicontainerstatus.ContainerStopped)
		errorName := event.Error.ErrorName()
		errorStr := event.Error.Error()
		shouldForceStop := false
		if errorName == dockerapi.DockerTimeoutErrorName || errorName == dockerapi.CannotInspectContainerErrorName {
			// If there's an error with inspecting the container or in case of timeout error,
			// we'll assume that the container has transitioned to RUNNING and issue
			// a stop. See #1043 for details
			shouldForceStop = true
		} else if errorName == dockerapi.CannotStartContainerErrorName && strings.HasSuffix(errorStr, io.EOF.Error()) {
			// If we get an EOF error from Docker when starting the container, we don't really know whether the
			// container is started anyway. So issuing a stop here as well. See #1708.
			shouldForceStop = true
		}

		if shouldForceStop {
			logger.Warn("Forcing container to stop", logger.Fields{
				field.TaskID:    mtask.GetID(),
				field.Container: container.Name,
				field.RuntimeID: container.GetRuntimeID(),
			})
			go mtask.engine.transitionContainer(mtask.Task, container, apicontainerstatus.ContainerStopped)
		}
		// Container known status not changed, no need for further processing
		return false
	}
}

// handleContainerStoppedTransitionError handles an error when transitioning a container to
// STOPPED. It returns a boolean indicating whether the tak can continue with updating its
// state
func (mtask *managedTask) handleContainerStoppedTransitionError(event dockerapi.DockerContainerChangeEvent,
	container *apicontainer.Container,
	currentKnownStatus apicontainerstatus.ContainerStatus) bool {

	pr := mtask.dockerClient.SystemPing(mtask.ctx, systemPingTimeout)
	if pr.Error != nil {
		logger.Info("Error stopping the container, but docker seems to be unresponsive; ignoring state change",
			logger.Fields{
				field.TaskID:      mtask.GetID(),
				field.Container:   container.Name,
				field.RuntimeID:   container.GetRuntimeID(),
				"ErrorName":       event.Error.ErrorName(),
				field.Error:       event.Error.Error(),
				"SystemPingError": pr.Error,
			})
		container.SetKnownStatus(currentKnownStatus)
		return false
	}

	// If we were trying to transition to stopped and had an error, we
	// clearly can't just continue trying to transition it to stopped
	// again and again. In this case, assume it's stopped (or close
	// enough) and get on with it
	// This can happen in cases where the container we tried to stop
	// was already stopped or did not exist at all.
	logger.Warn("Error stopping the container; marking it as stopped anyway", logger.Fields{
		field.TaskID:    mtask.GetID(),
		field.Container: container.Name,
		field.RuntimeID: container.GetRuntimeID(),
		"ErrorName":     event.Error.ErrorName(),
		field.Error:     event.Error.Error(),
	})
	container.SetKnownStatus(apicontainerstatus.ContainerStopped)
	container.SetDesiredStatus(apicontainerstatus.ContainerStopped)
	return true
}

// progressTask tries to step forwards all containers and resources that are able to be
// transitioned in the task's current state.
// It will continue listening to events from all channels while it does so, but
// none of those changes will be acted upon until this set of requests to
// docker completes.
// Container changes may also prompt the task status to change as well.
func (mtask *managedTask) progressTask() {
	logger.Debug("Progressing containers and resources in task", logger.Fields{
		field.TaskID: mtask.GetID(),
	})
	// max number of transitions length to ensure writes will never block on
	// these and if we exit early transitions can exit the goroutine, and it'll
	// get GC'd eventually
	resources := mtask.GetResources()
	transitionChange := make(chan struct{}, len(mtask.Containers)+len(resources))
	transitionChangeEntity := make(chan string, len(mtask.Containers)+len(resources))

	// startResourceTransitions should always be called before startContainerTransitions,
	// else it might result in a state where none of the containers can transition and
	// task may be moved to stopped.
	// anyResourceTransition is set to true when transition function needs to be called or
	// known status can be changed
	anyResourceTransition, resTransitions := mtask.startResourceTransitions(
		func(resource taskresource.TaskResource, nextStatus resourcestatus.ResourceStatus) {
			mtask.transitionResource(resource, nextStatus)
			transitionChange <- struct{}{}
			transitionChangeEntity <- resource.GetName()
		})

	anyContainerTransition, blockedDependencies, contTransitions, reasons := mtask.startContainerTransitions(
		func(container *apicontainer.Container, nextStatus apicontainerstatus.ContainerStatus) {
			mtask.engine.transitionContainer(mtask.Task, container, nextStatus)
			transitionChange <- struct{}{}
			transitionChangeEntity <- container.Name
		})

	atLeastOneTransitionStarted := anyResourceTransition || anyContainerTransition

	blockedByOrderingDependencies := len(blockedDependencies) > 0

	// If no transitions happened, and we aren't blocked by ordering dependencies, then we are possibly in a state where
	// its impossible for containers to move forward. We will do an additional check to see if we are waiting for ACS
	// execution credentials. If not, then we will abort the task progression.
	if !atLeastOneTransitionStarted && !blockedByOrderingDependencies {
		if !mtask.isWaitingForACSExecutionCredentials(reasons) {
			mtask.handleContainersUnableToTransitionState()
		}
		return
	}

	// If no containers are starting and we are blocked on ordering dependencies, we should watch for the task to change
	// over time. This will update the containers if they become healthy or stop, which makes it possible for the
	// conditions "HEALTHY" and "SUCCESS" to succeed.
	if !atLeastOneTransitionStarted && blockedByOrderingDependencies {
		go mtask.engine.checkTaskState(mtask.Task)
		ctx, cancel := context.WithTimeout(context.Background(), transitionPollTime)
		defer cancel()
		for timeout := mtask.waitEvent(ctx.Done()); !timeout; {
			timeout = mtask.waitEvent(ctx.Done())
		}
		return
	}

	// combine the resource and container transitions
	transitions := make(map[string]string)
	for k, v := range resTransitions {
		transitions[k] = v
	}
	for k, v := range contTransitions {
		transitions[k] = v.String()
	}

	// We've kicked off one or more transitions, wait for them to complete, but keep reading events as we do.
	// In fact, we have to for transitions to complete.
	mtask.waitForTransition(transitions, transitionChange, transitionChangeEntity)
	// update the task status
	if mtask.UpdateStatus() {
		logger.Info("Container or resource change also resulted in task change", logger.Fields{
			field.TaskID: mtask.GetID(),
		})

		// If knownStatus changed, let it be known
		var taskStateChangeReason string
		if mtask.GetKnownStatus().Terminal() {
			taskStateChangeReason = mtask.Task.GetTerminalReason()
		}
		mtask.emitTaskEvent(mtask.Task, taskStateChangeReason)
	}
}

// isWaitingForACSExecutionCredentials checks if the container that can't be transitioned
// was caused by waiting for credentials and start waiting
func (mtask *managedTask) isWaitingForACSExecutionCredentials(reasons []error) bool {
	for _, reason := range reasons {
		if reason == dependencygraph.CredentialsNotResolvedErr {
			logger.Info("Waiting for credentials to pull from ECR", logger.Fields{
				field.TaskID: mtask.GetID(),
			})

			timeoutCtx, timeoutCancel := context.WithTimeout(mtask.ctx, waitForPullCredentialsTimeout)
			defer timeoutCancel()

			timedOut := mtask.waitEvent(timeoutCtx.Done())
			if timedOut {
				logger.Info("Timed out waiting for acs credentials message", logger.Fields{
					field.TaskID: mtask.GetID(),
				})
			}
			return true
		}
	}
	return false
}

// startContainerTransitions steps through each container in the task and calls
// the passed transition function when a transition should occur.
func (mtask *managedTask) startContainerTransitions(transitionFunc containerTransitionFunc) (bool,
	map[string]apicontainer.DependsOn, map[string]apicontainerstatus.ContainerStatus, []error) {
	anyCanTransition := false
	var reasons []error
	blocked := make(map[string]apicontainer.DependsOn)
	transitions := make(map[string]apicontainerstatus.ContainerStatus)
	for _, cont := range mtask.Containers {
		transition := mtask.containerNextState(cont)
		if transition.reason != nil {
			if transition.reason.IsTerminal() {
				mtask.handleTerminalDependencyError(cont, transition.reason)
			}
			// container can't be transitioned
			reasons = append(reasons, transition.reason)
			if transition.blockedOn != nil {
				blocked[cont.Name] = *transition.blockedOn
			}
			continue
		}

		// If the container is already in a transition, skip
		if transition.actionRequired && !cont.SetAppliedStatus(transition.nextState) {
			// At least one container is able to be moved forward, so we're not deadlocked
			anyCanTransition = true
			continue
		}

		// At least one container is able to be moved forwards, so we're not deadlocked
		anyCanTransition = true

		if !transition.actionRequired {
			// Updating the container status without calling any docker API, send in
			// a goroutine so that it won't block here before the waitForContainerTransition
			// was called after this function. And all the events sent to mtask.dockerMessages
			// will be handled by handleContainerChange.
			go func(cont *apicontainer.Container, status apicontainerstatus.ContainerStatus) {
				mtask.dockerMessages <- dockerContainerChange{
					container: cont,
					event: dockerapi.DockerContainerChangeEvent{
						Status: status,
					},
				}
			}(cont, transition.nextState)
			continue
		}
		transitions[cont.Name] = transition.nextState
		go transitionFunc(cont, transition.nextState)
	}

	return anyCanTransition, blocked, transitions, reasons
}

func (mtask *managedTask) handleTerminalDependencyError(container *apicontainer.Container,
	error dependencygraph.DependencyError) {
	logger.Error("Terminal error detected during transition; marking container as stopped", logger.Fields{
		field.Container: container.Name,
		field.Error:     error.Error(),
	})
	container.SetDesiredStatus(apicontainerstatus.ContainerStopped)
	exitCode := 143
	container.SetKnownExitCode(&exitCode)
	// Change container status to STOPPED with exit code 143. This exit code is what docker reports when
	// a container receives SIGTERM. In this case it's technically not true that we send SIGTERM because the
	// container didn't even start, but we have to report an error and 143 seems the most appropriate.
	go func(cont *apicontainer.Container) {
		mtask.dockerMessages <- dockerContainerChange{
			container: cont,
			event: dockerapi.DockerContainerChangeEvent{
				Status: apicontainerstatus.ContainerStopped,
				DockerContainerMetadata: dockerapi.DockerContainerMetadata{
					Error:    dockerapi.CannotStartContainerError{FromError: error},
					ExitCode: &exitCode,
				},
			},
		}
	}(container)
}

// startResourceTransitions steps through each resource in the task and calls
// the passed transition function when a transition should occur
func (mtask *managedTask) startResourceTransitions(transitionFunc resourceTransitionFunc) (bool, map[string]string) {
	anyCanTransition := false
	transitions := make(map[string]string)
	for _, res := range mtask.GetResources() {
		knownStatus := res.GetKnownStatus()
		desiredStatus := res.GetDesiredStatus()
		if knownStatus >= desiredStatus {
			logger.Debug("Resource has already transitioned to or beyond the desired status",
				logger.Fields{
					field.TaskID:        mtask.GetID(),
					field.Resource:      res.GetName(),
					field.KnownStatus:   res.StatusString(knownStatus),
					field.DesiredStatus: res.StatusString(desiredStatus),
				})
			continue
		}
		anyCanTransition = true
		transition := mtask.resourceNextState(res)
		// If the resource is already in a transition, skip
		if transition.actionRequired && !res.SetAppliedStatus(transition.nextState) {
			// At least one resource is able to be moved forwards, so we're not deadlocked
			continue
		}
		if !transition.actionRequired {
			// no action is required for the transition, just set the known status without
			// calling any transition function
			go mtask.emitResourceChange(resourceStateChange{
				resource:  res,
				nextState: transition.nextState,
				err:       nil,
			})
			continue
		}
		// At least one resource is able to be move forwards, so we're not deadlocked
		transitions[res.GetName()] = transition.status
		go transitionFunc(res, transition.nextState)
	}
	return anyCanTransition, transitions
}

// transitionResource calls applyResourceState, and then notifies the managed
// task of the change. transitionResource is called by progressTask
func (mtask *managedTask) transitionResource(resource taskresource.TaskResource,
	to resourcestatus.ResourceStatus) {
	err := mtask.applyResourceState(resource, to)

	if mtask.engine.isTaskManaged(mtask.Arn) {
		mtask.emitResourceChange(resourceStateChange{
			resource:  resource,
			nextState: to,
			err:       err,
		})
	}
}

// applyResourceState moves the resource to the given state by calling the
// function defined in the transitionFunctionMap for the state
func (mtask *managedTask) applyResourceState(resource taskresource.TaskResource,
	nextState resourcestatus.ResourceStatus) error {
	resName := resource.GetName()
	resStatus := resource.StatusString(nextState)
	err := resource.ApplyTransition(nextState)
	if err != nil {
		logger.Info("Error transitioning resource", logger.Fields{
			field.TaskID:       mtask.GetID(),
			field.Resource:     resName,
			field.FailedStatus: resStatus,
			field.Error:        err,
		})

		return err
	}
	logger.Info("Transitioned resource", logger.Fields{
		field.TaskID:   mtask.GetID(),
		field.Resource: resName,
		field.Status:   resStatus,
	})
	return nil
}

type containerTransitionFunc func(container *apicontainer.Container, nextStatus apicontainerstatus.ContainerStatus)

type resourceTransitionFunc func(resource taskresource.TaskResource, nextStatus resourcestatus.ResourceStatus)

// containerNextState determines the next state a container should go to.
// It returns a transition struct including the information:
// * container state it should transition to,
// * a bool indicating whether any action is required
// * an error indicating why this transition can't happen
//
// 'Stopped, false, ""' -> "You can move it to known stopped, but you don't have to call a transition function"
// 'Running, true, ""' -> "You can move it to running, and you need to call the transition function"
// 'None, false, containerDependencyNotResolved' -> "This should not be moved; it has unresolved dependencies;"
//
// Next status is determined by whether the known and desired statuses are
// equal, the next numerically greater (iota-wise) status, and whether
// dependencies are fully resolved.
func (mtask *managedTask) containerNextState(container *apicontainer.Container) *containerTransition {
	containerKnownStatus := container.GetKnownStatus()
	containerDesiredStatus := container.GetDesiredStatus()

	if containerKnownStatus == containerDesiredStatus {
		logger.Debug("Container at desired status", logger.Fields{
			field.TaskID:        mtask.GetID(),
			field.Container:     container.Name,
			field.RuntimeID:     container.GetRuntimeID(),
			field.DesiredStatus: containerDesiredStatus.String(),
		})
		return &containerTransition{
			nextState:      apicontainerstatus.ContainerStatusNone,
			actionRequired: false,
			reason:         dependencygraph.ContainerPastDesiredStatusErr,
		}
	}

	if containerKnownStatus > containerDesiredStatus {
		logger.Debug("Container has already transitioned beyond desired status", logger.Fields{
			field.TaskID:        mtask.GetID(),
			field.Container:     container.Name,
			field.RuntimeID:     container.GetRuntimeID(),
			field.KnownStatus:   containerKnownStatus.String(),
			field.DesiredStatus: containerDesiredStatus.String(),
		})
		return &containerTransition{
			nextState:      apicontainerstatus.ContainerStatusNone,
			actionRequired: false,
			reason:         dependencygraph.ContainerPastDesiredStatusErr,
		}
	}
	if blocked, err := dependencygraph.DependenciesAreResolved(container, mtask.Containers,
		mtask.Task.GetExecutionCredentialsID(), mtask.credentialsManager, mtask.GetResources(), mtask.cfg); err != nil {
		logger.Debug("Can't apply state to container yet due to unresolved dependencies", logger.Fields{
			field.TaskID:    mtask.GetID(),
			field.Container: container.Name,
			field.RuntimeID: container.GetRuntimeID(),
			field.Error:     err,
		})
		return &containerTransition{
			nextState:      apicontainerstatus.ContainerStatusNone,
			actionRequired: false,
			reason:         err,
			blockedOn:      blocked,
		}
	}

	var nextState apicontainerstatus.ContainerStatus
	if container.DesiredTerminal() {
		nextState = apicontainerstatus.ContainerStopped
		// It's not enough to just check if container is in steady state here
		// we should really check if >= RUNNING <= STOPPED
		if !container.IsRunning() {
			// If the container's AppliedStatus is running, it means the StartContainer
			// api call has already been scheduled, we should not mark it as stopped
			// directly, because when the stopped container comes back, we will end up
			// with either:
			// 1. The task is not cleaned up, the handleStoppedToRunningContainerTransition
			// function will handle this case, but only once. If there are some
			// other stopped containers come back, they will not be stopped by
			// Agent.
			// 2. The task has already been cleaned up, in this case any stopped container
			// will not be stopped by Agent when they come back.
			if container.GetAppliedStatus() == apicontainerstatus.ContainerRunning {
				nextState = apicontainerstatus.ContainerStatusNone
			}

			return &containerTransition{
				nextState:      nextState,
				actionRequired: false,
			}
		}
	} else {
		nextState = container.GetNextKnownStateProgression()
	}
	return &containerTransition{
		nextState:      nextState,
		actionRequired: true,
	}
}

// resourceNextState determines the next state a resource should go to.
// It returns a transition struct including the information:
// * resource state it should transition to,
// * string presentation of the resource state
// * a bool indicating whether any action is required
// * an error indicating why this transition can't happen
//
// Next status is determined by whether the known and desired statuses are
// equal, the next numerically greater (iota-wise) status, and whether
// dependencies are fully resolved.
func (mtask *managedTask) resourceNextState(resource taskresource.TaskResource) *resourceTransition {
	resKnownStatus := resource.GetKnownStatus()
	resDesiredStatus := resource.GetDesiredStatus()

	if resKnownStatus >= resDesiredStatus {
		logger.Debug("Task resource has already transitioned to or beyond desired status", logger.Fields{
			field.TaskID:        mtask.GetID(),
			field.Resource:      resource.GetName(),
			field.KnownStatus:   resource.StatusString(resKnownStatus),
			field.DesiredStatus: resource.StatusString(resDesiredStatus),
		})
		return &resourceTransition{
			nextState:      resourcestatus.ResourceStatusNone,
			status:         resource.StatusString(resourcestatus.ResourceStatusNone),
			actionRequired: false,
			reason:         dependencygraph.ResourcePastDesiredStatusErr,
		}
	}
	if err := dependencygraph.TaskResourceDependenciesAreResolved(resource, mtask.Containers); err != nil {
		logger.Debug("Can't apply state to resource yet due to unresolved dependencies",
			logger.Fields{
				field.TaskID:   mtask.GetID(),
				field.Resource: resource.GetName(),
				field.Error:    err,
			})
		return &resourceTransition{
			nextState:      resourcestatus.ResourceStatusNone,
			status:         resource.StatusString(resourcestatus.ResourceStatusNone),
			actionRequired: false,
			reason:         err,
		}
	}

	var nextState resourcestatus.ResourceStatus
	if resource.DesiredTerminal() {
		nextState := resource.TerminalStatus()
		return &resourceTransition{
			nextState:      nextState,
			status:         resource.StatusString(nextState),
			actionRequired: false, // Resource cleanup is done while cleaning up task, so not doing anything here.
		}
	}
	nextState = resource.NextKnownState()
	return &resourceTransition{
		nextState:      nextState,
		status:         resource.StatusString(nextState),
		actionRequired: true,
	}
}

func (mtask *managedTask) handleContainersUnableToTransitionState() {
	if mtask.GetDesiredStatus().Terminal() {
		// Ack, really bad. We want it to stop but the containers don't think
		// that's possible. let's just break out and hope for the best!
		logger.Critical("The state is so bad that we're just giving up on it", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
		mtask.SetKnownStatus(apitaskstatus.TaskStopped)
		mtask.emitTaskEvent(mtask.Task, taskUnableToTransitionToStoppedReason)
		// TODO we should probably panic here
	} else {
		// If we end up here, it means containers are not able to transition anymore; maybe because of dependencies that
		// are unable to start. Therefore, if there are essential containers that haven't started yet, we need to
		// stop the task since they are not going to start.
		stopTask := false
		for _, c := range mtask.Containers {
			if c.IsEssential() && !c.IsKnownSteadyState() {
				stopTask = true
				break
			}
		}

		if stopTask {
			logger.Critical("Task in a bad state; it's not steady state but no containers want to transition",
				logger.Fields{
					field.TaskID: mtask.GetID(),
				})
			mtask.handleDesiredStatusChange(apitaskstatus.TaskStopped, 0)
		}
	}
}

func (mtask *managedTask) waitForTransition(transitions map[string]string,
	transition <-chan struct{},
	transitionChangeEntity <-chan string) {
	// There could be multiple transitions, but we just need to wait for one of them
	// to ensure that there is at least one container or resource can be processed in the next
	// progressTask call. This is done by waiting for one transition/acs/docker message.
	if !mtask.waitEvent(transition) {
		logger.Debug("Received non-transition events", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
		return
	}
	transitionedEntity := <-transitionChangeEntity
	logger.Debug("Transition finished", logger.Fields{
		field.TaskID:         mtask.GetID(),
		"TransitionedEntity": transitionedEntity,
	})
	delete(transitions, transitionedEntity)
	logger.Debug("Task still waiting for: %v", logger.Fields{
		field.TaskID:         mtask.GetID(),
		"TransitionedEntity": fmt.Sprintf("%v", transitions),
	})
}

func (mtask *managedTask) time() ttime.Time {
	mtask._timeOnce.Do(func() {
		if mtask._time == nil {
			mtask._time = &ttime.DefaultTime{}
		}
	})
	return mtask._time
}

func (mtask *managedTask) cleanupTask(taskStoppedDuration time.Duration) {
	taskExecutionCredentialsID := mtask.GetExecutionCredentialsID()
	cleanupTimeDuration := mtask.GetKnownStatusTime().Add(taskStoppedDuration).Sub(ttime.Now())
	cleanupTime := make(<-chan time.Time)
	if cleanupTimeDuration < 0 {
		logger.Info("Cleanup Duration has been exceeded; starting cleanup now", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
		cleanupTime = mtask.time().After(time.Nanosecond)
	} else {
		cleanupTime = mtask.time().After(cleanupTimeDuration)
	}
	cleanupTimeBool := make(chan struct{})
	go func() {
		<-cleanupTime
		close(cleanupTimeBool)
	}()
	// wait for the cleanup time to elapse, signalled by cleanupTimeBool
	for !mtask.waitEvent(cleanupTimeBool) {
	}

	// wait for apitaskstatus.TaskStopped to be sent
	ok := mtask.waitForStopReported()
	if !ok {
		logger.Error("Aborting cleanup for task as it is not reported as stopped", logger.Fields{
			field.TaskID:     mtask.GetID(),
			field.SentStatus: mtask.GetSentStatus().String(),
		})
		return
	}

	logger.Info("Cleaning up task's containers and data", logger.Fields{
		field.TaskID: mtask.GetID(),
	})

	// For the duration of this, simply discard any task events; this ensures the
	// speedy processing of other events for other tasks
	// discard events while the task is being removed from engine state
	go mtask.discardEvents()
	mtask.engine.sweepTask(mtask.Task)
	mtask.engine.deleteTask(mtask.Task)

	// Remove TaskExecutionCredentials from credentialsManager
	if taskExecutionCredentialsID != "" {
		logger.Info("Cleaning up task's execution credentials", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
		mtask.credentialsManager.RemoveCredentials(taskExecutionCredentialsID)
	}

	// The last thing to do here is to cancel the context, which should cancel
	// all outstanding go routines associated with this managed task.
	mtask.cancel()
}

func (mtask *managedTask) discardEvents() {
	for {
		select {
		case <-mtask.dockerMessages:
		case <-mtask.acsMessages:
		case <-mtask.resourceStateChangeEvent:
		case <-mtask.ctx.Done():
			return
		}
	}
}

// waitForStopReported will wait for the task to be reported stopped and return true, or will time-out and return false.
// Messages on the mtask.dockerMessages and mtask.acsMessages channels will be handled while this function is waiting.
func (mtask *managedTask) waitForStopReported() bool {
	stoppedSentBool := make(chan struct{})
	taskStopped := false
	go func() {
		for i := 0; i < _maxStoppedWaitTimes; i++ {
			// ensure that we block until apitaskstatus.TaskStopped is actually sent
			sentStatus := mtask.GetSentStatus()
			if sentStatus >= apitaskstatus.TaskStopped {
				taskStopped = true
				break
			}
			logger.Warn("Blocking cleanup until the task has been reported stopped", logger.Fields{
				field.TaskID:     mtask.GetID(),
				field.SentStatus: sentStatus.String(),
				"Attempt":        i + 1,
				"MaxAttempts":    _maxStoppedWaitTimes,
			})
			mtask._time.Sleep(_stoppedSentWaitInterval)
		}
		stoppedSentBool <- struct{}{}
		close(stoppedSentBool)
	}()
	// wait for apitaskstatus.TaskStopped to be sent
	for !mtask.waitEvent(stoppedSentBool) {
	}
	return taskStopped
}

func (mtask *managedTask) UnstageVolumes(csiClient csiclient.CSIClient) []error {
	errors := make([]error, 0)
	task := mtask.Task
	if task == nil {
		errors = append(errors, fmt.Errorf("managed task is nil"))
		return errors
	}
	if !task.IsEBSTaskAttachEnabled() {
		logger.Debug("Task is not EBS-backed. Skip NodeUnstageVolume.")
		return nil
	}
	for _, tv := range task.Volumes {
		switch tv.Volume.(type) {
		case *taskresourcevolume.EBSTaskVolumeConfig:
			ebsCfg := tv.Volume.(*taskresourcevolume.EBSTaskVolumeConfig)
			volumeId := ebsCfg.VolumeId
			hostPath := ebsCfg.Source()
			err := mtask.unstageVolumeWithRetriesAndTimeout(csiClient, volumeId, hostPath)
			if err != nil {
				errors = append(errors, fmt.Errorf("%w; unable to unstage volume for task %s", err, task.String()))
				continue
			}
		}
	}
	return errors
}

func (mtask *managedTask) unstageVolumeWithRetriesAndTimeout(csiClient csiclient.CSIClient, volumeId, hostPath string) error {
	derivedCtx, cancel := context.WithTimeout(mtask.ctx, unstageVolumeTimeout)
	defer cancel()
	backoff := retry.NewExponentialBackoff(unstageBackoffMin, unstageBackoffMax, unstageBackoffJitter, unstageBackoffMultiple)
	err := retry.RetryNWithBackoff(backoff, unstageRetryAttempts, func() error {
		logger.Debug("attempting CSI unstage with retries", logger.Fields{
			field.TaskID: mtask.GetID(),
		})
		return csiClient.NodeUnstageVolume(derivedCtx, volumeId, hostPath)
	})
	return err
}
