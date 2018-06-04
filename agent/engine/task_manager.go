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
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	utilsync "github.com/aws/amazon-ecs-agent/agent/utils/sync"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// waitForPullCredentialsTimeout is the timeout agent trying to wait for pull
	// credentials from acs, after the timeout it will check the credentials manager
	// and start processing the task or start another round of waiting
	waitForPullCredentialsTimeout         = 1 * time.Minute
	defaultTaskSteadyStatePollInterval    = 5 * time.Minute
	stoppedSentWaitInterval               = 30 * time.Second
	maxStoppedWaitTimes                   = 72 * time.Hour / stoppedSentWaitInterval
	taskUnableToTransitionToStoppedReason = "TaskStateError: Agent could not progress task's state to stopped"
	taskUnableToCreatePlatformResources   = "TaskStateError: Agent could not create task's platform resources"
)

var (
	_stoppedSentWaitInterval       = stoppedSentWaitInterval
	_maxStoppedWaitTimes           = int(maxStoppedWaitTimes)
	taskNotWaitForSteadyStateError = errors.New("managed task: steady state check context is nil")
)

type acsTaskUpdate struct {
	apitask.TaskStatus
}

type dockerContainerChange struct {
	container *apicontainer.Container
	event     dockerapi.DockerContainerChangeEvent
}

// resourceStateChange represents the required status change after resource transition
type resourceStateChange struct {
	resource  taskresource.TaskResource
	nextState taskresource.ResourceStatus
	err       error
}

type acsTransition struct {
	seqnum        int64
	desiredStatus apitask.TaskStatus
}

// containerTransition defines the struct for a container to transition
type containerTransition struct {
	nextState      apicontainer.ContainerStatus
	actionRequired bool
	reason         error
}

// resourceTransition defines the struct for a resource to transition.
type resourceTransition struct {
	// nextState represents the next known status that the resource can move to
	nextState taskresource.ResourceStatus
	// status is the string value of nextState
	status string
	// actionRequired indicates if the transition function needs to be called for
	// the transition to be complete
	actionRequired bool
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
	*apitask.Task
	ctx    context.Context
	cancel context.CancelFunc

	engine             *DockerTaskEngine
	cfg                *config.Config
	saver              statemanager.Saver
	credentialsManager credentials.Manager
	cniClient          ecscni.CNIClient
	taskStopWG         *utilsync.SequentialWaitGroup

	acsMessages                chan acsTransition
	dockerMessages             chan dockerContainerChange
	resourceStateChangeEvent   chan resourceStateChange
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

	// steadyStatePollInterval is the duration that a managed task waits
	// once the task gets into steady state before polling the state of all of
	// the task's containers to re-evaluate if the task is still in steady state
	// This is set to defaultTaskSteadyStatePollInterval in production code.
	// This can be used by tests that are looking to ensure that the steady state
	// verification logic gets executed to set it to a low interval
	steadyStatePollInterval time.Duration
}

// newManagedTask is a method on DockerTaskEngine to create a new managedTask.
// This method must only be called when the engine.processTasks write lock is
// already held.
func (engine *DockerTaskEngine) newManagedTask(task *apitask.Task) *managedTask {
	ctx, cancel := context.WithCancel(engine.ctx)
	t := &managedTask{
		ctx:                      ctx,
		cancel:                   cancel,
		Task:                     task,
		acsMessages:              make(chan acsTransition),
		dockerMessages:           make(chan dockerContainerChange),
		resourceStateChangeEvent: make(chan resourceStateChange),
		engine:                     engine,
		cfg:                        engine.cfg,
		stateChangeEvents:          engine.stateChangeEvents,
		containerChangeEventStream: engine.containerChangeEventStream,
		saver:                   engine.saver,
		credentialsManager:      engine.credentialsManager,
		cniClient:               engine.cniClient,
		taskStopWG:              engine.taskStopGroup,
		steadyStatePollInterval: engine.taskSteadyStatePollInterval,
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
		select {
		case <-mtask.ctx.Done():
			seelog.Infof("Managed task [%s]: parent context cancelled, exit", mtask.Arn)
			return
		default:
		}

		// If it's steadyState, just spin until we need to do work
		for mtask.steadyState() {
			mtask.waitSteady()
		}

		if !mtask.GetKnownStatus().Terminal() {
			// If we aren't terminal and we aren't steady state, we should be
			// able to move some containers along.
			seelog.Debugf("Managed task [%s]: task not steady state or terminal; progressing it",
				mtask.Arn)

			mtask.progressTask()
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
	seelog.Debugf("Managed task [%s]: task has reached stopped. Waiting for container cleanup", mtask.Arn)
	mtask.cleanupCredentials()
	if mtask.StopSequenceNumber != 0 {
		seelog.Debugf("Managed task [%s]: marking done for this sequence: %d",
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

	othersStoppedCtx, cancel := context.WithCancel(mtask.ctx)
	defer cancel()

	go func() {
		mtask.taskStopWG.Wait(mtask.StartSequenceNumber)
		cancel()
	}()

	for !mtask.waitEvent(othersStoppedCtx.Done()) {
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
	seelog.Infof("Managed task [%s]: task at steady state: %s", mtask.Arn, mtask.GetKnownStatus().String())

	timeoutCtx, cancel := context.WithTimeout(mtask.ctx, mtask.steadyStatePollInterval)
	defer cancel()
	timedOut := mtask.waitEvent(timeoutCtx.Done())

	if timedOut {
		seelog.Debugf("Managed task [%s]: checking to make sure it's still at steadystate", mtask.Arn)
		go mtask.engine.checkTaskState(mtask.Task)
	}
}

// steadyState returns if the task is in a steady state. Steady state is when task's desired
// and known status are both RUNNING
func (mtask *managedTask) steadyState() bool {
	taskKnownStatus := mtask.GetKnownStatus()
	return taskKnownStatus == apitask.TaskRunning && taskKnownStatus >= mtask.GetDesiredStatus()
}

// cleanupCredentials removes credentials for a stopped task
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
	case resChange := <-mtask.resourceStateChangeEvent:
		res := resChange.resource
		seelog.Debugf("Managed task [%s]: got resource [%s] event: [%s]",
			mtask.Arn, res.GetName(), res.StatusString(resChange.nextState))
		mtask.handleResourceStateChange(resChange)
		return false
	case <-stopWaiting:
		seelog.Debugf("Managed task [%s]: no longer waiting", mtask.Arn)
		return true
	}
}

// handleDesiredStatusChange updates the desired status on the task. Updates
// only occur if the new desired status is "compatible" (farther along than the
// current desired state); "redundant" (less-than or equal desired states) are
// ignored and dropped.
func (mtask *managedTask) handleDesiredStatusChange(desiredStatus apitask.TaskStatus, seqnum int64) {
	// Handle acs message changes this task's desired status to whatever
	// acs says it should be if it is compatible
	seelog.Debugf("Managed task [%s]: new acs transition to: %s; sequence number: %d; task stop sequence number: %d",
		mtask.Arn, desiredStatus.String(), seqnum, mtask.StopSequenceNumber)
	if desiredStatus <= mtask.GetDesiredStatus() {
		seelog.Debugf("Managed task [%s]: redundant task transition from [%s] to [%s], ignoring",
			mtask.Arn, mtask.GetDesiredStatus().String(), desiredStatus.String())
		return
	}
	if desiredStatus == apitask.TaskStopped && seqnum != 0 && mtask.GetStopSequenceNumber() == 0 {
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

// handleResourceStateChange attempts to update resource's known status depending on
// the current status and errors during transition
func (mtask *managedTask) handleResourceStateChange(resChange resourceStateChange) {
	// locate the resource
	res := resChange.resource
	if !mtask.isResourceFound(res) {
		seelog.Criticalf("Managed task [%s]: state error; invoked with another task's resource [%s]",
			mtask.Arn, res.GetName())
		return
	}

	status := resChange.nextState
	err := resChange.err
	currentKnownStatus := res.GetKnownStatus()

	if status <= currentKnownStatus {
		seelog.Infof("Managed task [%s]: redundant resource state change. %s to %s, but already %s",
			mtask.Arn, res.GetName(), res.StatusString(status), res.StatusString(currentKnownStatus))
		return
	}

	if err == nil {
		res.SetKnownStatus(status)
		mtask.engine.saver.Save()
		return
	}
	seelog.Debugf("Managed task [%s]: error while transitioning resource %s to %s: %v",
		mtask.Arn, res.GetName(), res.StatusString(status), err)
	if status == res.SteadyState() {
		seelog.Errorf("Managed task [%s]: error while creating resource %s, setting the task's desired status to STOPPED",
			mtask.Arn, res.GetName())
		mtask.SetDesiredStatus(apitask.TaskStopped)
		mtask.engine.saver.Save()
	}
}

func (mtask *managedTask) emitResourceChange(change resourceStateChange) {
	if mtask.ctx.Err() != nil {
		seelog.Infof("Managed task [%s]: unable to emit resource state change due to closed context: %v",
			mtask.Arn, mtask.ctx.Err())
	}
	mtask.resourceStateChangeEvent <- change
}

func (mtask *managedTask) emitTaskEvent(task *apitask.Task, reason string) {
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
func (mtask *managedTask) emitContainerEvent(task *apitask.Task, cont *apicontainer.Container, reason string) {
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

func (mtask *managedTask) emitDockerContainerChange(change dockerContainerChange) {
	if mtask.ctx.Err() != nil {
		seelog.Infof("Managed task [%s]: unable to emit docker container change due to closed context: %v",
			mtask.Arn, mtask.ctx.Err())
	}
	mtask.dockerMessages <- change
}

func (mtask *managedTask) emitACSTransition(transition acsTransition) {
	if mtask.ctx.Err() != nil {
		seelog.Infof("Managed task [%s]: unable to emit acs transition due to closed context: %v",
			mtask.Arn, mtask.ctx.Err())
	}
	mtask.acsMessages <- transition
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
func (mtask *managedTask) handleStoppedToRunningContainerTransition(status apicontainer.ContainerStatus, container *apicontainer.Container) {
	containerKnownStatus := container.GetKnownStatus()
	if status > containerKnownStatus {
		// Event status is greater than container's known status.
		// This is not a backward transition, return
		return
	}
	if containerKnownStatus != apicontainer.ContainerStopped {
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
		go mtask.engine.transitionContainer(mtask.Task, container, apicontainer.ContainerStopped)
		// This will not proceed afterwards because status <= knownstatus below
	})
}

// handleEventError handles a container change event error and decides whether
// we should proceed to transition the container
func (mtask *managedTask) handleEventError(containerChange dockerContainerChange, currentKnownStatus apicontainer.ContainerStatus) bool {
	container := containerChange.container
	event := containerChange.event
	if container.ApplyingError == nil {
		container.ApplyingError = apierrors.NewNamedError(event.Error)
	}
	switch event.Status {
	// event.Status is the desired container transition from container's known status
	// (* -> event.Status)
	case apicontainer.ContainerPulled:
		// If the agent pull behavior is always or once, we receive the error because
		// the image pull fails, the task should fail. If we don't fail task here,
		// then the cached image will probably be used for creating container, and we
		// don't want to use cached image for both cases.
		if mtask.cfg.ImagePullBehavior == config.ImagePullAlwaysBehavior ||
			mtask.cfg.ImagePullBehavior == config.ImagePullOnceBehavior {
			seelog.Errorf("Managed task [%s]: error while pulling image %s for container %s , moving task to STOPPED: %v",
				mtask.Arn, container.Image, container.Name, event.Error)
			// The task should be stopped regardless of whether this container is
			// essential or non-essential.
			mtask.SetDesiredStatus(apitask.TaskStopped)
			return false
		}
		// If the agent pull behavior is prefer-cached, we receive the error because
		// the image pull fails and there is no cached image in local, we don't make
		// the task fail here, will let create container handle it instead.
		// If the agent pull behavior is default, use local image cache directly,
		// assuming it exists.
		seelog.Errorf("Managed task [%s]: error while pulling container %s and image %s, will try to run anyway: %v",
			mtask.Arn, container.Name, container.Image, event.Error)
		// proceed anyway
		return true
	case apicontainer.ContainerStopped:
		// Container's desired transition was to 'STOPPED'
		return mtask.handleContainerStoppedTransitionError(event, container, currentKnownStatus)
	case apicontainer.ContainerStatusNone:
		fallthrough
	case apicontainer.ContainerCreated:
		// No need to explicitly stop containers if this is a * -> NONE/CREATED transition
		seelog.Warnf("Managed task [%s]: error creating container [%s]; marking its desired status as STOPPED: %v",
			mtask.Arn, container.Name, event.Error)
		container.SetKnownStatus(currentKnownStatus)
		container.SetDesiredStatus(apicontainer.ContainerStopped)
		return false
	default:
		// If this is a * -> RUNNING / RESOURCES_PROVISIONED transition, we need to stop
		// the container.
		seelog.Warnf("Managed task [%s]: error starting/provisioning container[%s]; marking its desired status as STOPPED: %v",
			mtask.Arn, container.Name, event.Error)
		container.SetKnownStatus(currentKnownStatus)
		container.SetDesiredStatus(apicontainer.ContainerStopped)
		errorName := event.Error.ErrorName()
		if errorName == dockerapi.DockerTimeoutErrorName || errorName == dockerapi.CannotInspectContainerErrorName {
			// If there's an error with inspecting the container or in case of timeout error,
			// we'll also assume that the container has transitioned to RUNNING and issue
			// a stop. See #1043 for details
			seelog.Warnf("Managed task [%s]: forcing container [%s] to stop",
				mtask.Arn, container.Name)
			go mtask.engine.transitionContainer(mtask.Task, container, apicontainer.ContainerStopped)
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
	currentKnownStatus apicontainer.ContainerStatus) bool {
	// If we were trying to transition to stopped and had a timeout error
	// from docker, reset the known status to the current status and return
	// This ensures that we don't emit a containerstopped event; a
	// terminal container event from docker event stream will instead be
	// responsible for the transition. Alternatively, the steadyState check
	// could also trigger the progress and have another go at stopping the
	// container
	if event.Error.ErrorName() == dockerapi.DockerTimeoutErrorName {
		seelog.Infof("Managed task [%s]: '%s' error stopping container [%s]. Ignoring state change: %v",
			mtask.Arn, dockerapi.DockerTimeoutErrorName, container.Name, event.Error.Error())
		container.SetKnownStatus(currentKnownStatus)
		return false
	}
	// If docker returned a transient error while trying to stop a container,
	// reset the known status to the current status and return
	cannotStopContainerError, ok := event.Error.(cannotStopContainerError)
	if ok && cannotStopContainerError.IsRetriableError() {
		seelog.Infof("Managed task [%s]: error stopping the container [%s]. Ignoring state change: %v",
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
	container.SetKnownStatus(apicontainer.ContainerStopped)
	container.SetDesiredStatus(apicontainer.ContainerStopped)
	return true
}

// progressTask tries to step forwards all containers and resources that are able to be
// transitioned in the task's current state.
// It will continue listening to events from all channels while it does so, but
// none of those changes will be acted upon until this set of requests to
// docker completes.
// Container changes may also prompt the task status to change as well.
func (mtask *managedTask) progressTask() {
	seelog.Debugf("Managed task [%s]: progressing containers and resources in task", mtask.Arn)
	// max number of transitions length to ensure writes will never block on
	// these and if we exit early transitions can exit the goroutine and it'll
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
		func(resource taskresource.TaskResource, nextStatus taskresource.ResourceStatus) {
			mtask.transitionResource(resource, nextStatus)
			transitionChange <- struct{}{}
			transitionChangeEntity <- resource.GetName()
		})

	anyContainerTransition, contTransitions, reasons := mtask.startContainerTransitions(
		func(container *apicontainer.Container, nextStatus apicontainer.ContainerStatus) {
			mtask.engine.transitionContainer(mtask.Task, container, nextStatus)
			transitionChange <- struct{}{}
			transitionChangeEntity <- container.Name
		})

	if !anyContainerTransition && !anyResourceTransition {
		if !mtask.waitForExecutionCredentialsFromACS(reasons) {
			mtask.onContainersUnableToTransitionState()
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

	// We've kicked off one or more transitions, wait for them to
	// complete, but keep reading events as we do. in fact, we have to for
	// transitions to complete
	mtask.waitForTransition(transitions, transitionChange, transitionChangeEntity)
	// update the task status
	changed := mtask.UpdateStatus()
	if changed {
		seelog.Debugf("Managed task [%s]: container or resource change also resulted in task change", mtask.Arn)
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

			timeoutCtx, timeoutCancel := context.WithTimeout(mtask.ctx, waitForPullCredentialsTimeout)
			defer timeoutCancel()

			timedOut := mtask.waitEvent(timeoutCtx.Done())
			if timedOut {
				seelog.Debugf("Managed task [%s]: timed out waiting for acs credentials message", mtask.Arn)
			}
			return true
		}
	}
	return false
}

// startContainerTransitions steps through each container in the task and calls
// the passed transition function when a transition should occur.
func (mtask *managedTask) startContainerTransitions(transitionFunc containerTransitionFunc) (bool, map[string]apicontainer.ContainerStatus, []error) {
	anyCanTransition := false
	var reasons []error
	transitions := make(map[string]apicontainer.ContainerStatus)
	for _, cont := range mtask.Containers {
		transition := mtask.containerNextState(cont)
		if transition.reason != nil {
			// container can't be transitioned
			reasons = append(reasons, transition.reason)
			continue
		}

		// If the container is already in a transition, skip
		if transition.actionRequired && !cont.SetAppliedStatus(transition.nextState) {
			// At least one container is able to be moved forwards, so we're not deadlocked
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
			go func(cont *apicontainer.Container, status apicontainer.ContainerStatus) {
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

	return anyCanTransition, transitions, reasons
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
			seelog.Debugf("Managed task [%s]: resource [%s] has already transitioned to or beyond the desired status %s; current known is %s",
				mtask.Arn, res.GetName(), res.StatusString(desiredStatus), res.StatusString(knownStatus))
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
	to taskresource.ResourceStatus) {
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
	nextState taskresource.ResourceStatus) error {
	resName := resource.GetName()
	resStatus := resource.StatusString(nextState)
	err := resource.ApplyTransition(nextState)
	if err != nil {
		seelog.Infof("Managed task [%s]: error transitioning resource [%s] to [%s]: %v",
			mtask.Arn, resName, resStatus, err)
		return err
	}
	seelog.Debugf("Managed task [%s]: transitioned resource [%s] to [%s]",
		mtask.Arn, resName, resStatus)
	return nil
}

type containerTransitionFunc func(container *apicontainer.Container, nextStatus apicontainer.ContainerStatus)

type resourceTransitionFunc func(resource taskresource.TaskResource, nextStatus taskresource.ResourceStatus)

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
func (mtask *managedTask) containerNextState(container *apicontainer.Container) *containerTransition {
	containerKnownStatus := container.GetKnownStatus()
	containerDesiredStatus := container.GetDesiredStatus()

	if containerKnownStatus == containerDesiredStatus {
		seelog.Debugf("Managed task [%s]: container [%s] at desired status: %s",
			mtask.Arn, container.Name, containerDesiredStatus.String())
		return &containerTransition{
			nextState:      apicontainer.ContainerStatusNone,
			actionRequired: false,
			reason:         dependencygraph.ContainerPastDesiredStatusErr,
		}
	}

	if containerKnownStatus > containerDesiredStatus {
		seelog.Debugf("Managed task [%s]: container [%s] has already transitioned beyond desired status(%s): %s",
			mtask.Arn, container.Name, containerKnownStatus.String(), containerDesiredStatus.String())
		return &containerTransition{
			nextState:      apicontainer.ContainerStatusNone,
			actionRequired: false,
			reason:         dependencygraph.ContainerPastDesiredStatusErr,
		}
	}
	if err := dependencygraph.DependenciesAreResolved(container, mtask.Containers,
		mtask.Task.GetExecutionCredentialsID(), mtask.credentialsManager, mtask.GetResources()); err != nil {
		seelog.Debugf("Managed task [%s]: can't apply state to container [%s] yet due to unresolved dependencies: %v",
			mtask.Arn, container.Name, err)
		return &containerTransition{
			nextState:      apicontainer.ContainerStatusNone,
			actionRequired: false,
			reason:         err,
		}
	}

	var nextState apicontainer.ContainerStatus
	if container.DesiredTerminal() {
		nextState = apicontainer.ContainerStopped
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

func (mtask *managedTask) resourceNextState(resource taskresource.TaskResource) *resourceTransition {
	if resource.DesiredTerminal() {
		nextState := resource.TerminalStatus()
		return &resourceTransition{
			nextState:      nextState,
			status:         resource.StatusString(nextState),
			actionRequired: false, // since resource cleanup will be done while sweeping task
		}
	}
	nextState := resource.NextKnownState()
	return &resourceTransition{
		nextState:      nextState,
		status:         resource.StatusString(nextState),
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
		mtask.SetKnownStatus(apitask.TaskStopped)
		mtask.emitTaskEvent(mtask.Task, taskUnableToTransitionToStoppedReason)
		// TODO we should probably panic here
	} else {
		seelog.Criticalf("Managed task [%s]: moving task to stopped due to bad state", mtask.Arn)
		mtask.handleDesiredStatusChange(apitask.TaskStopped, 0)
	}
}

func (mtask *managedTask) waitForTransition(transitions map[string]string,
	transition <-chan struct{},
	transitionChangeEntity <-chan string) {
	// There could be multiple transitions, but we just need to wait for one of them
	// to ensure that there is at least one container or resource can be processed in the next
	// progressTask call. This is done by waiting for one transition/acs/docker message.
	if !mtask.waitEvent(transition) {
		seelog.Debugf("Managed task [%s]: received non-transition events", mtask.Arn)
		return
	}
	transitionedEntity := <-transitionChangeEntity
	seelog.Debugf("Managed task [%s]: transition for [%s] finished",
		mtask.Arn, transitionedEntity)
	delete(transitions, transitionedEntity)
	seelog.Debugf("Managed task [%s]: still waiting for: %v", mtask.Arn, transitions)
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
	cleanupTimeBool := make(chan struct{})
	go func() {
		<-cleanupTime
		cleanupTimeBool <- struct{}{}
		close(cleanupTimeBool)
	}()
	// wait for the cleanup time to elapse, signalled by cleanupTimeBool
	for !mtask.waitEvent(cleanupTimeBool) {
	}

	// wait for apitask.TaskStopped to be sent
	ok := mtask.waitForStopReported()
	if !ok {
		seelog.Errorf("Managed task [%s]: aborting cleanup for task as it is not reported as stopped. SentStatus: %s",
			mtask.Arn, mtask.GetSentStatus().String())
		return
	}

	seelog.Infof("Managed task [%s]: cleaning up task's containers and data", mtask.Arn)

	// For the duration of this, simply discard any task events; this ensures the
	// speedy processing of other events for other tasks
	// discard events while the task is being removed from engine state
	go mtask.discardEvents()
	mtask.engine.sweepTask(mtask.Task)
	mtask.engine.deleteTask(mtask.Task)

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
			// The task has been cancelled. No need to process any more
			// events
			close(mtask.dockerMessages)
			close(mtask.acsMessages)
			close(mtask.resourceStateChangeEvent)
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
			// ensure that we block until apitask.TaskStopped is actually sent
			sentStatus := mtask.GetSentStatus()
			if sentStatus >= apitask.TaskStopped {
				taskStopped = true
				break
			}
			seelog.Warnf("Managed task [%s]: blocking cleanup until the task has been reported stopped. SentStatus: %s (%d/%d)",
				mtask.Arn, sentStatus.String(), i+1, _maxStoppedWaitTimes)
			mtask._time.Sleep(_stoppedSentWaitInterval)
		}
		stoppedSentBool <- struct{}{}
		close(stoppedSentBool)
	}()
	// wait for apitask.TaskStopped to be sent
	for !mtask.waitEvent(stoppedSentBool) {
	}
	return taskStopped
}
