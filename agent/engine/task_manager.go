// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/resources"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	utilsync "github.com/aws/amazon-ecs-agent/agent/utils/sync"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"

	"github.com/cihub/seelog"
)

const (
	// waitForPullCredentialsTimeout is the timeout agent trying to wait for pull
	// credentials from acs, after the timeout it will check the credentials manager
	// and start processing the task or start another round of waiting
	waitForPullCredentialsTimeout         = 1 * time.Minute
	steadyStateTaskVerifyInterval         = 5 * time.Minute
	stoppedSentWaitInterval               = 30 * time.Second
	maxStoppedWaitTimes                   = 72 * time.Hour / stoppedSentWaitInterval
	taskUnableToTransitionToStoppedReason = "TaskStateError: Agent could not progress task's state to stopped"
	taskUnableToCreatePlatformResources   = "TaskStateError: Agent could not create task's platform resources"
)

var (
	_stoppedSentWaitInterval = stoppedSentWaitInterval
	_maxStoppedWaitTimes     = int(maxStoppedWaitTimes)
)

type acsTaskUpdate struct {
	api.TaskStatus
}

type dockerContainerChange struct {
	container *api.Container
	event     DockerContainerChangeEvent
}

type acsTransition struct {
	seqnum        int64
	desiredStatus api.TaskStatus
}

// containerTransition defines the struct for a container to transition
type containerTransition struct {
	nextState      api.ContainerStatus
	actionRequired bool
	reason         error
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
// block and it is expected that the managedTask listen to those channels
// almost constantly.
// The general operation should be:
//  1) Listen to the channels
//  2) On an event, update the status of the task and containers (known/desired)
//  3) Figure out if any action needs to be done. If so, do it
//  4) GOTO 1
// Item '3' obviously might lead to some duration where you are not listening
// to the channels. However, this can be solved by kicking off '3' as a
// goroutine and then only communicating the result back via the channels
// (obviously once you kick off a goroutine you give up the right to write the
// task's statuses yourself)
type managedTask struct {
	*api.Task
	ctx                context.Context
	engine             *DockerTaskEngine
	cfg                *config.Config
	saver              statemanager.Saver
	credentialsManager credentials.Manager
	cniClient          ecscni.CNIClient
	taskStopWG         *utilsync.SequentialWaitGroup

	acsMessages                chan acsTransition
	dockerMessages             chan dockerContainerChange
	stateChangeEvents          chan statechange.Event
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

	resource resources.Resource
}

// newManagedTask is a method on DockerTaskEngine to create a new managedTask.
// This method must only be called when the engine.processTasks write lock is
// already held.
func (engine *DockerTaskEngine) newManagedTask(task *api.Task) *managedTask {
	t := &managedTask{
		ctx:                        engine.ctx,
		Task:                       task,
		acsMessages:                make(chan acsTransition),
		dockerMessages:             make(chan dockerContainerChange),
		engine:                     engine,
		resource:                   engine.resource,
		cfg:                        engine.cfg,
		stateChangeEvents:          engine.stateChangeEvents,
		containerChangeEventStream: engine.containerChangeEventStream,
		saver:              engine.saver,
		credentialsManager: engine.credentialsManager,
		cniClient:          engine.cniClient,
		taskStopWG:         engine.taskStopGroup,
	}
	engine.managedTasks[task.Arn] = t
	return t
}

// overseeTask is the main goroutine of the managedTask. It runs an infinite
// loop of receiving messages and attempting to take action based on those
// messages.
func (mtask *managedTask) overseeTask() {
	// Do a single updatestatus at the beginning to create the container
	// `desiredstatus`es which are a construct of the engine used only here,
	// not present on the backend
	mtask.UpdateStatus()
	// If this was a 'state restore', send all unsent statuses
	mtask.emitCurrentStatus()

	// Wait for host resources required by this task to become available
	mtask.waitForHostResources()

	// Main infinite loop. This is where we receive messages and dispatch work.
	for {
		// If it's steadyState, just spin until we need to do work
		for mtask.steadyState() {
			mtask.waitSteady()
		}

		if !mtask.GetKnownStatus().Terminal() {
			// If we aren't terminal and we aren't steady state, we should be
			// able to move some containers along.
			seelog.Debugf("Managed task [%s]: task not steady state or terminal; progressing it",
				mtask.Arn)

			// TODO: Add new resource provisioned state ?
			if mtask.cfg.TaskCPUMemLimit.Enabled() {
				err := mtask.resource.Setup(mtask.Task)
				if err != nil {
					seelog.Criticalf("Managed task [%s]: unable to setup platform resources: %v",
						mtask.Arn, err)
					mtask.SetDesiredStatus(api.TaskStopped)
					mtask.emitTaskEvent(mtask.Task, taskUnableToCreatePlatformResources)
				}
				seelog.Infof("Managed task [%s]: Cgroup resource set up for task complete", mtask.Arn)
			}
			mtask.progressContainers()
		}

		// If we reach this point, we've changed the task in some way.
		// Conversely, for it to spin in steady state it will have to have been
		// loaded in steady state or progressed through here, so saving here should
		// be sufficient to capture state changes.
		err := mtask.saver.Save()
		if err != nil {
			seelog.Warnf("Managed task [%s]: unable to checkpoint task's states to disk: %v",
				mtask.Arn, err)
		}

		if mtask.GetKnownStatus().Terminal() {
			break
		}
	}
	// We only break out of the above if this task is known to be stopped. Do
	// onetime cleanup here, including removing the task after a timeout
	seelog.Debugf("Managed task [%s]: Task has reached stopped. Waiting for container cleanup", mtask.Arn)
	mtask.cleanupCredentials()
	if mtask.StopSequenceNumber != 0 {
		seelog.Debugf("Managed task [%s]: Marking done for this sequence: %d",
			mtask.Arn, mtask.StopSequenceNumber)
		mtask.taskStopWG.Done(mtask.StopSequenceNumber)
	}
	// TODO: make this idempotent on agent restart
	go mtask.releaseIPInIPAM()
	mtask.cleanupTask(mtask.cfg.TaskCleanupWaitDuration)
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
// the task. This involves waiting for previous stops to complete so the
// resources become free.
func (mtask *managedTask) waitForHostResources() {
	if mtask.StartSequenceNumber == 0 {
		// This is the first transition on this host. No need to wait
		return
	}
	if mtask.GetDesiredStatus().Terminal() {
		// Task's desired status is STOPPED. No need to wait in this case either
		return
	}

	seelog.Infof("Managed task [%s]: waiting for any previous stops to complete. Sequence number: %d",
		mtask.Arn, mtask.StartSequenceNumber)
	othersStopped := make(chan bool, 1)
	go func() {
		mtask.taskStopWG.Wait(mtask.StartSequenceNumber)
		othersStopped <- true
	}()
	for !mtask.waitEvent(othersStopped) {
		if mtask.GetDesiredStatus().Terminal() {
			// If we end up here, that means we received a start then stop for this
			// task before a task that was expected to stop before it could
			// actually stop
			break
		}
	}
	seelog.Infof("Managed task [%s]: wait over; ready to move towards status: %s",
		mtask.Arn, mtask.GetDesiredStatus().String())
}

// waitSteady waits for a task to leave steady-state by waiting for a new
// event, or a timeout.
func (mtask *managedTask) waitSteady() {
	seelog.Debugf("Managed task [%s]: task at steady state: %s", mtask.Arn, mtask.GetKnownStatus().String())

	maxWait := make(chan bool, 1)
	timer := mtask.time().After(steadyStateTaskVerifyInterval)
	go func() {
		// TODO: Wire in context here
		<-timer
		maxWait <- true
	}()
	timedOut := mtask.waitEvent(maxWait)

	if timedOut {
		seelog.Debugf("Managed task [%s]: checking to make sure it's still at steadystate", mtask.Arn)
		go mtask.engine.CheckTaskState(mtask.Task)
	}
}

// steadyState returns if the task is in a steady state. Steady state is when task's desired
// and known status are both RUNNING
func (mtask *managedTask) steadyState() bool {
	taskKnownStatus := mtask.GetKnownStatus()
	return taskKnownStatus == api.TaskRunning && taskKnownStatus >= mtask.GetDesiredStatus()
}

// cleanupCredentials removes credentials for a stopped task
func (mtask *managedTask) cleanupCredentials() {
	taskCredentialsID := mtask.GetCredentialsID()
	if taskCredentialsID != "" {
		mtask.credentialsManager.RemoveCredentials(taskCredentialsID)
	}
}

// waitEvent waits for any event to occur. If an event occurs, the appropriate
// handler is called. If the event is the passed in channel, it will return the
// value written to the channel, otherwise it will return false.
// TODO: Wire in context here
func (mtask *managedTask) waitEvent(stopWaiting <-chan bool) bool {
	seelog.Debugf("Managed task [%s]: waiting for event for task", mtask.Arn)
	select {
	case acsTransition := <-mtask.acsMessages:
		seelog.Debugf("Managed task [%s]: got acs event", mtask.Arn)
		mtask.handleDesiredStatusChange(acsTransition.desiredStatus, acsTransition.seqnum)
		return false
	case dockerChange := <-mtask.dockerMessages:
		seelog.Debugf("Managed task [%s]: got container [%s] event: [%s]",
			mtask.Arn, dockerChange.container.Name, dockerChange.event.Status.String())
		mtask.handleContainerChange(dockerChange)
		return false
	case b := <-stopWaiting:
		seelog.Debugf("Managed task [%s]: no longer waiting", mtask.Arn)
		return b
	}
}

// handleDesiredStatusChange updates the desired status on the task. Updates
// only occur if the new desired status is "compatible" (farther along than the
// current desired state); "redundant" (less-than or equal desired states) are
// ignored and dropped.
func (mtask *managedTask) handleDesiredStatusChange(desiredStatus api.TaskStatus, seqnum int64) {
	// Handle acs message changes this task's desired status to whatever
	// acs says it should be if it is compatible
	seelog.Debugf("Managed task [%s]: new acs transition to: %s; sequence number: %d; task stop sequence number: %d",
		mtask.Arn, desiredStatus.String(), seqnum, mtask.StopSequenceNumber)
	if desiredStatus <= mtask.GetDesiredStatus() {
		seelog.Debugf("Managed task [%s]: redundant task transition from [%s] to [%s], ignoring",
			mtask.Arn, mtask.GetDesiredStatus().String(), desiredStatus.String())
		return
	}
	if desiredStatus == api.TaskStopped && seqnum != 0 && mtask.GetStopSequenceNumber() == 0 {
		seelog.Debugf("Managed task [%s]: task moving to stopped, adding to stopgroup with sequence number: %d",
			mtask.Arn, seqnum)
		mtask.SetStopSequenceNumber(seqnum)
		mtask.taskStopWG.Add(seqnum, 1)
	}
	mtask.SetDesiredStatus(desiredStatus)
	mtask.UpdateDesiredStatus()
}

// handleContainerChange updates a container's known status. If the message
// contains any interesting information (like exit codes or ports), they are
// propagated.
func (mtask *managedTask) handleContainerChange(containerChange dockerContainerChange) {
	// locate the container
	container := containerChange.container
	found := mtask.isContainerFound(container)
	if !found {
		seelog.Criticalf("Managed task [%s]: state error; invoked with another task's container [%s]!",
			mtask.Arn, container.Name)
		return
	}

	event := containerChange.event
	seelog.Debugf("Managed task [%s]: handling container change [%v] for container [%s]",
		mtask.Arn, event, container.Name)

	// If this is a backwards transition stopped->running, the first time set it
	// to be known running so it will be stopped. Subsequently ignore these backward transitions
	containerKnownStatus := container.GetKnownStatus()
	mtask.handleStoppedToRunningContainerTransition(event.Status, container)
	if event.Status <= containerKnownStatus {
		seelog.Infof("Managed task [%s]: redundant container state change. %s to %s, but already %s",
			mtask.Arn, container.Name, event.Status.String(), containerKnownStatus.String())
		return
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

	// Update the container health status
	if container.HealthStatusShouldBeReported() {
		container.SetHealthStatus(event.Health)
	}

	mtask.RecordExecutionStoppedAt(container)
	seelog.Debugf("Managed task [%s]: sending container change event to tcs, container: [%s(%s)], status: %s",
		mtask.Arn, container.Name, event.DockerID, event.Status.String())
	err := mtask.containerChangeEventStream.WriteToEventStream(event)
	if err != nil {
		seelog.Warnf("Managed task [%s]: failed to write container [%s] change event to tcs event stream: %v",
			mtask.Arn, container.Name, err)
	}

	mtask.emitContainerEvent(mtask.Task, container, "")
	if mtask.UpdateStatus() {
		seelog.Debugf("Managed task [%s]: container change also resulted in task change [%s]: [%s]",
			mtask.Arn, container.Name, mtask.GetDesiredStatus().String())
		// If knownStatus changed, let it be known
		mtask.emitTaskEvent(mtask.Task, "")
	}
	seelog.Debugf("Managed task [%s]: container change also resulted in task change [%s]: [%s]",
		mtask.Arn, container.Name, mtask.GetDesiredStatus().String())
}

func (mtask *managedTask) emitTaskEvent(task *api.Task, reason string) {
	event, err := api.NewTaskStateChangeEvent(task, reason)
	if err != nil {
		seelog.Infof("Managed task [%s]: unable to create task state change event [%s]: %v",
			task.Arn, reason, err)
		return
	}

	seelog.Infof("Managed task [%s]: sending task change event [%s]", mtask.Arn, event.String())
	mtask.stateChangeEvents <- event
	seelog.Infof("Managed task [%s]: sent task change event [%s]", mtask.Arn, event.String())
}

// emitContainerEvent passes a given event up through the containerEvents channel if necessary.
// It will omit events the backend would not process and will perform best-effort deduplication of events.
func (mtask *managedTask) emitContainerEvent(task *api.Task, cont *api.Container, reason string) {
	event, err := api.NewContainerStateChangeEvent(task, cont, reason)
	if err != nil {
		seelog.Debugf("Managed task [%s]: unable to create state change event for container [%s]: %v",
			task.Arn, cont.Name, err)
		return
	}

	seelog.Infof("Managed task [%s]: sending container change event [%s]: %s",
		mtask.Arn, cont.Name, event.String())
	mtask.stateChangeEvents <- event
	seelog.Infof("Managed task [%s]: sent container change event [%s]: %s",
		mtask.Arn, cont.Name, event.String())
}

func (mtask *managedTask) isContainerFound(container *api.Container) bool {
	found := false
	for _, c := range mtask.Containers {
		if container == c {
			found = true
			break
		}
	}
	return found
}

// releaseIPInIPAM releases the ip used by the task for awsvpc
func (mtask *managedTask) releaseIPInIPAM() {
	if mtask.ENI == nil {
		return
	}
	seelog.Infof("Managed task [%s]: IPAM releasing ip for task eni", mtask.Arn)

	cfg, err := mtask.BuildCNIConfig()
	if err != nil {
		seelog.Warnf("Managed task [%s]: failed to release ip; unable to build cni configuration: %v",
			mtask.Arn, err)
		return
	}
	err = mtask.cniClient.ReleaseIPResource(cfg)
	if err != nil {
		seelog.Warnf("Managed task [%s]: failed to release ip; IPAM error: %v",
			mtask.Arn, err)
		return
	}
}

// handleStoppedToRunningContainerTransition detects a "backwards" container
// transition where a known-stopped container is found to be running again and
// handles it.
func (mtask *managedTask) handleStoppedToRunningContainerTransition(status api.ContainerStatus, container *api.Container) {
	containerKnownStatus := container.GetKnownStatus()
	if status > containerKnownStatus {
		// Event status is greater than container's known status.
		// This is not a backward transition, return
		return
	}
	if containerKnownStatus != api.ContainerStopped {
		// Container's known status is not STOPPED. Nothing to do here.
		return
	}
	if !status.IsRunning() {
		// Container's 'to' transition was not either of RUNNING or RESOURCES_PROVISIONED
		// states. Nothing to do in this case as well
		return
	}
	// If the container becomes running after we've stopped it (possibly
	// because we got an error running it and it ran anyways), the first time
	// update it to 'known running' so that it will be driven back to stopped
	mtask.unexpectedStart.Do(func() {
		seelog.Warnf("Managed task [%s]: stopped container [%s] came back; re-stopping it once",
			mtask.Arn, container.Name)
		go mtask.engine.transitionContainer(mtask.Task, container, api.ContainerStopped)
		// This will not proceed afterwards because status <= knownstatus below
	})
}

// handleEventError handles a container change event error and decides whether
// we should proceed to transition the container
func (mtask *managedTask) handleEventError(containerChange dockerContainerChange, currentKnownStatus api.ContainerStatus) bool {
	container := containerChange.container
	event := containerChange.event
	if container.ApplyingError == nil {
		container.ApplyingError = api.NewNamedError(event.Error)
	}
	switch event.Status {
	// event.Status is the desired container transition from container's known status
	// (* -> event.Status)
	case api.ContainerPulled:
		// Container's desired transition was to 'PULLED'. A failure to pull might
		// not be fatal if e.g. the image already exists.
		seelog.Errorf("Managed task [%s]: Error while pulling container %s, will try to run anyway: %v",
			mtask.Arn, container.Name, event.Error)
		// proceed anyway
		return true
	case api.ContainerStopped:
		// Container's desired transition was to 'STOPPED'
		return mtask.handleContainerStoppedTransitionError(event, container, currentKnownStatus)
	case api.ContainerStatusNone:
		fallthrough
	case api.ContainerCreated:
		// No need to explicitly stop containers if this is a * -> NONE/CREATED transition
		seelog.Warnf("Managed task [%s]: Error creating container [%s]; marking its desired status as STOPPED: %v",
			mtask.Arn, container.Name, event.Error)
		container.SetKnownStatus(currentKnownStatus)
		container.SetDesiredStatus(api.ContainerStopped)
		return false
	default:
		// If this is a * -> RUNNING / RESOURCES_PROVISIONED transition, we need to stop
		// the container.
		seelog.Warnf("Managed task [%s]: Error starting/provisioning container[%s]; marking its desired status as STOPPED: %v",
			mtask.Arn, container.Name, event.Error)
		container.SetKnownStatus(currentKnownStatus)
		container.SetDesiredStatus(api.ContainerStopped)
		errorName := event.Error.ErrorName()
		if errorName == dockerTimeoutErrorName || errorName == cannotInspectContainerErrorName {
			// If there's an error with inspecting the container or in case of timeout error,
			// we'll also assume that the container has transitioned to RUNNING and issue
			// a stop. See #1043 for details
			seelog.Warnf("Managed task [%s]: Forcing container [%s] to stop",
				mtask.Arn, container.Name)
			go mtask.engine.transitionContainer(mtask.Task, container, api.ContainerStopped)
		}
		// Container known status not changed, no need for further processing
		return false
	}
}

// handleContainerStoppedTransitionError handles an error when transitioning a container to
// STOPPED. It returns a boolean indicating whether the tak can continue with updating its
// state
func (mtask *managedTask) handleContainerStoppedTransitionError(event DockerContainerChangeEvent,
	container *api.Container,
	currentKnownStatus api.ContainerStatus) bool {
	// If we were trying to transition to stopped and had a timeout error
	// from docker, reset the known status to the current status and return
	// This ensures that we don't emit a containerstopped event; a
	// terminal container event from docker event stream will instead be
	// responsible for the transition. Alternatively, the steadyState check
	// could also trigger the progress and have another go at stopping the
	// container
	if event.Error.ErrorName() == dockerTimeoutErrorName {
		seelog.Infof("Managed task [%s]: '%s' error stopping container [%s]. Ignoring state change: %v",
			mtask.Arn, dockerTimeoutErrorName, container.Name, event.Error.Error())
		container.SetKnownStatus(currentKnownStatus)
		return false
	}
	// If docker returned a transient error while trying to stop a container,
	// reset the known status to the current status and return
	cannotStopContainerError, ok := event.Error.(cannotStopContainerError)
	if ok && cannotStopContainerError.IsRetriableError() {
		seelog.Infof("Managed task [%s]: Error stopping the container [%s]. Ignoring state change: %v",
			mtask.Arn, container.Name, cannotStopContainerError.Error())
		container.SetKnownStatus(currentKnownStatus)
		return false
	}

	// If we were trying to transition to stopped and had an error, we
	// clearly can't just continue trying to transition it to stopped
	// again and again. In this case, assume it's stopped (or close
	// enough) and get on with it
	// This can happen in cases where the container we tried to stop
	// was already stopped or did not exist at all.
	seelog.Warnf("Managed task [%s]: 'docker stop' for container [%s] returned %s: %s",
		mtask.Arn, container.Name, event.Error.ErrorName(), event.Error.Error())
	container.SetKnownStatus(api.ContainerStopped)
	container.SetDesiredStatus(api.ContainerStopped)
	return true
}

// progressContainers tries to step forwards all containers that are able to be
// transitioned in the task's current state.
// It will continue listening to events from all channels while it does so, but
// none of those changes will be acted upon until this set of requests to
// docker completes.
// Container changes may also prompt the task status to change as well.
func (mtask *managedTask) progressContainers() {
	seelog.Debugf("Managed task [%s]: progressing containers in task", mtask.Arn)
	// max number of transitions length to ensure writes will never block on
	// these and if we exit early transitions can exit the goroutine and it'll
	// get GC'd eventually
	transitionChange := make(chan bool, len(mtask.Containers))
	transitionChangeContainer := make(chan string, len(mtask.Containers))

	anyCanTransition, transitions, reasons := mtask.startContainerTransitions(
		func(container *api.Container, nextStatus api.ContainerStatus) {
			mtask.engine.transitionContainer(mtask.Task, container, nextStatus)
			transitionChange <- true
			transitionChangeContainer <- container.Name
		})

	if !anyCanTransition {
		if !mtask.waitForExecutionCredentialsFromACS(reasons) {
			mtask.onContainersUnableToTransitionState()
		}
		return
	}

	// We've kicked off one or more transitions, wait for them to
	// complete, but keep reading events as we do. in fact, we have to for
	// transitions to complete
	mtask.waitForContainerTransitions(transitions, transitionChange, transitionChangeContainer)
	seelog.Debugf("Managed task [%s]: done transitioning all containers", mtask.Arn)

	// update the task status
	changed := mtask.UpdateStatus()
	if changed {
		seelog.Debugf("Managed task [%s]: container change also resulted in task change", mtask.Arn)
		// If knownStatus changed, let it be known
		mtask.emitTaskEvent(mtask.Task, "")
	}
}

// waitForExecutionCredentialsFromACS checks if the container that can't be transitioned
// was caused by waiting for credentials and start waiting
func (mtask *managedTask) waitForExecutionCredentialsFromACS(reasons []error) bool {
	for _, reason := range reasons {
		if reason == dependencygraph.CredentialsNotResolvedErr {
			seelog.Debugf("Managed task [%s]: waiting for credentials to pull from ECR", mtask.Arn)

			maxWait := make(chan bool, 1)
			timer := mtask.time().AfterFunc(waitForPullCredentialsTimeout, func() {
				maxWait <- true
			})
			// Have a timeout in case we missed the acs message but the credentials
			// were already in the credentials manager
			if !mtask.waitEvent(maxWait) {
				timer.Stop()
			}
			return true
		}
	}
	return false
}

// startContainerTransitions steps through each container in the task and calls
// the passed transition function when a transition should occur.
func (mtask *managedTask) startContainerTransitions(transitionFunc containerTransitionFunc) (bool, map[string]api.ContainerStatus, []error) {
	anyCanTransition := false
	var reasons []error
	transitions := make(map[string]api.ContainerStatus)
	for _, cont := range mtask.Containers {
		transition := mtask.containerNextState(cont)
		if transition.reason != nil {
			// container can't be transitioned
			reasons = append(reasons, transition.reason)
			continue
		}
		// At least one container is able to be moved forwards, so we're not deadlocked
		anyCanTransition = true

		if !transition.actionRequired {
			mtask.handleContainerChange(dockerContainerChange{
				container: cont,
				event: DockerContainerChangeEvent{
					Status: transition.nextState,
				},
			})
			continue
		}
		transitions[cont.Name] = transition.nextState
		go transitionFunc(cont, transition.nextState)
	}

	return anyCanTransition, transitions, reasons
}

type containerTransitionFunc func(container *api.Container, nextStatus api.ContainerStatus)

// containerNextState determines the next state a container should go to.
// It returns a transition struct including the information:
// * container state it should transition to,
// * a bool indicating whether any action is required
// * an error indicating why this transition can't happen
//
// 'Stopped, false, ""' -> "You can move it to known stopped, but you don't have to call a transition function"
// 'Running, true, ""' -> "You can move it to running and you need to call the transition function"
// 'None, false, containerDependencyNotResolved' -> "This should not be moved; it has unresolved dependencies;"
//
// Next status is determined by whether the known and desired statuses are
// equal, the next numerically greater (iota-wise) status, and whether
// dependencies are fully resolved.
func (mtask *managedTask) containerNextState(container *api.Container) *containerTransition {
	containerKnownStatus := container.GetKnownStatus()
	containerDesiredStatus := container.GetDesiredStatus()

	if containerKnownStatus == containerDesiredStatus {
		seelog.Debugf("Managed task [%s]: container [%s] at desired status: %s",
			mtask.Arn, container.Name, containerDesiredStatus.String())
		return &containerTransition{
			nextState:      api.ContainerStatusNone,
			actionRequired: false,
			reason:         dependencygraph.ContainerPastDesiredStatusErr,
		}
	}

	if containerKnownStatus > containerDesiredStatus {
		seelog.Debugf("Managed task [%s]: container [%s] has already transitioned beyond desired status(%s): %s",
			mtask.Arn, container.Name, containerKnownStatus.String(), containerDesiredStatus.String())
		return &containerTransition{
			nextState:      api.ContainerStatusNone,
			actionRequired: false,
			reason:         dependencygraph.ContainerPastDesiredStatusErr,
		}
	}
	if err := dependencygraph.DependenciesAreResolved(container, mtask.Containers,
		mtask.Task.GetExecutionCredentialsID(), mtask.credentialsManager); err != nil {
		seelog.Debugf("Managed task [%s]: can't apply state to container [%s] yet due to unresolved dependencies: %v",
			mtask.Arn, container.Name, err)
		return &containerTransition{
			nextState:      api.ContainerStatusNone,
			actionRequired: false,
			reason:         err,
		}
	}

	var nextState api.ContainerStatus
	if container.DesiredTerminal() {
		nextState = api.ContainerStopped
		// It's not enough to just check if container is in steady state here
		// we should really check if >= RUNNING <= STOPPED
		if !container.IsRunning() {
			// If it's not currently running we do not need to do anything to make it become stopped.
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

func (mtask *managedTask) onContainersUnableToTransitionState() {
	seelog.Criticalf("Managed task [%s]: task in a bad state; it's not steadystate but no containers want to transition",
		mtask.Arn)
	if mtask.GetDesiredStatus().Terminal() {
		// Ack, really bad. We want it to stop but the containers don't think
		// that's possible. let's just break out and hope for the best!
		seelog.Criticalf("Managed task [%s]: The state is so bad that we're just giving up on it", mtask.Arn)
		mtask.SetKnownStatus(api.TaskStopped)
		mtask.emitTaskEvent(mtask.Task, taskUnableToTransitionToStoppedReason)
		// TODO we should probably panic here
	} else {
		seelog.Criticalf("Managed task [%s]: voving task to stopped due to bad state", mtask.Arn)
		mtask.handleDesiredStatusChange(api.TaskStopped, 0)
	}
}

func (mtask *managedTask) waitForContainerTransitions(transitions map[string]api.ContainerStatus,
	transitionChange <-chan bool,
	transitionChangeContainer <-chan string) {
	for len(transitions) > 0 {
		if mtask.waitEvent(transitionChange) {
			changedContainer := <-transitionChangeContainer
			seelog.Debugf("Managed task [%s]: transition for container[%s] finished",
				mtask.Arn, changedContainer)
			delete(transitions, changedContainer)
			seelog.Debugf("Managed task [%s]: still waiting for: %v", mtask.Arn, transitions)
		}
		if mtask.GetDesiredStatus().Terminal() || mtask.GetKnownStatus().Terminal() {
			allWaitingOnPulled := true
			for _, desired := range transitions {
				if desired != api.ContainerPulled {
					allWaitingOnPulled = false
				}
			}
			if allWaitingOnPulled {
				// We don't actually care to wait for 'pull' transitions to finish if
				// we're just heading to stopped since those resources aren't
				// inherently linked to this task anyways for e.g. gc and so on.
				seelog.Debugf("Managed task [%s]: all containers are waiting for pulled transition; exiting early: %v",
					mtask.Arn, transitions)
				break
			}
		}
	}
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
	cleanupTimeDuration := mtask.GetKnownStatusTime().Add(taskStoppedDuration).Sub(ttime.Now())
	// There is a potential deadlock here if cleanupTime is negative. Ignore the computed
	// value in this case in favor of the default config value.
	if cleanupTimeDuration < 0 {
		seelog.Debugf("Managed task [%s]: Cleanup Duration is too short. Resetting to: %s",
			mtask.Arn, config.DefaultTaskCleanupWaitDuration.String())
		cleanupTimeDuration = config.DefaultTaskCleanupWaitDuration
	}
	cleanupTime := mtask.time().After(cleanupTimeDuration)
	cleanupTimeBool := make(chan bool)
	go func() {
		<-cleanupTime
		cleanupTimeBool <- true
		close(cleanupTimeBool)
	}()
	// wait for the cleanup time to elapse, signalled by cleanupTimeBool
	for !mtask.waitEvent(cleanupTimeBool) {
	}

	// wait for api.TaskStopped to be sent
	ok := mtask.waitForStopReported()
	if !ok {
		seelog.Errorf("Managed task [%s]: Aborting cleanup for task as it is not reported as stopped. SentStatus: %s",
			mtask.Arn, mtask.GetSentStatus().String())
		return
	}

	seelog.Infof("Managed task [%s]: cleaning up task's containers and data", mtask.Arn)

	// For the duration of this, simply discard any task events; this ensures the
	// speedy processing of other events for other tasks
	handleCleanupDone := make(chan struct{})
	// discard events while the task is being removed from engine state
	go mtask.discardEventsUntil(handleCleanupDone)
	mtask.engine.sweepTask(mtask.Task)
	mtask.engine.deleteTask(mtask.Task, handleCleanupDone)

	// Cleanup any leftover messages before closing their channels. No new
	// messages possible because we deleted ourselves from managedTasks, so this
	// removes all stale ones
	mtask.discardPendingMessages()

	close(mtask.dockerMessages)
	close(mtask.acsMessages)
}

func (mtask *managedTask) discardEventsUntil(done chan struct{}) {
	for {
		select {
		case <-mtask.dockerMessages:
		case <-mtask.acsMessages:
		case <-done:
			return
		}
	}
}

func (mtask *managedTask) discardPendingMessages() {
	for {
		select {
		case <-mtask.dockerMessages:
		case <-mtask.acsMessages:
		default:
			return
		}
	}
}

// waitForStopReported will wait for the task to be reported stopped and return true, or will time-out and return false.
// Messages on the mtask.dockerMessages and mtask.acsMessages channels will be handled while this function is waiting.
func (mtask *managedTask) waitForStopReported() bool {
	stoppedSentBool := make(chan bool)
	taskStopped := false
	go func() {
		for i := 0; i < _maxStoppedWaitTimes; i++ {
			// ensure that we block until api.TaskStopped is actually sent
			sentStatus := mtask.GetSentStatus()
			if sentStatus >= api.TaskStopped {
				taskStopped = true
				break
			}
			seelog.Warnf("Managed task [%s]: Blocking cleanup until the task has been reported stopped. SentStatus: %s (%d/%d)",
				mtask.Arn, sentStatus.String(), i+1, _maxStoppedWaitTimes)
			mtask._time.Sleep(_stoppedSentWaitInterval)
		}
		stoppedSentBool <- true
		close(stoppedSentBool)
	}()
	// wait for api.TaskStopped to be sent
	for !mtask.waitEvent(stoppedSentBool) {
	}
	return taskStopped
}
