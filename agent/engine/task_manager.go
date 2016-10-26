// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/cihub/seelog"
)

const (
	steadyStateTaskVerifyInterval = 10 * time.Minute
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
	engine *DockerTaskEngine

	acsMessages    chan acsTransition
	dockerMessages chan dockerContainerChange

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
}

// newManagedTask is a method on DockerTaskEngine to create a new managedTask.
// This method must only be called when the engine.processTasks write lock is
// already held.
func (engine *DockerTaskEngine) newManagedTask(task *api.Task) *managedTask {
	t := &managedTask{
		Task:           task,
		acsMessages:    make(chan acsTransition),
		dockerMessages: make(chan dockerContainerChange),
		engine:         engine,
	}
	engine.managedTasks[task.Arn] = t
	return t
}

func (mtask *managedTask) overseeTask() {
	llog := log.New("task", mtask)

	// Do a single updatestatus at the beginning to create the container
	// 'desiredstatus'es which are a construct of the engine used only here,
	// not present on the backend
	mtask.UpdateStatus()
	// If this was a 'state restore', send all unsent statuses
	mtask.emitCurrentStatus()

	if mtask.StartSequenceNumber != 0 && !mtask.GetDesiredStatus().Terminal() {
		llog.Debug("Waiting for any previous stops to complete", "seqnum", mtask.StartSequenceNumber)
		othersStopped := make(chan bool, 1)
		go func() {
			mtask.engine.taskStopGroup.Wait(mtask.StartSequenceNumber)
			othersStopped <- true
		}()
		for !mtask.waitEvent(othersStopped) {
			if mtask.GetDesiredStatus().Terminal() {
				// If we end up here, that means we recieved a start then stop for this
				// task before a task that was expected to stop before it could
				// actually stop
				break
			}
		}
		llog.Debug("Wait over; ready to move towards status: " + mtask.GetDesiredStatus().String())
	}
	for {
		// If it's steadyState, just spin until we need to do work
		for mtask.steadyState() {
			llog.Debug("Task at steady state", "state", mtask.KnownStatus.String())
			maxWait := make(chan bool, 1)
			timer := mtask.time().After(steadyStateTaskVerifyInterval)
			go func() {
				<-timer
				maxWait <- true
			}()
			timedOut := mtask.waitEvent(maxWait)

			if timedOut {
				llog.Debug("Checking task to make sure it's still at steadystate")
				go mtask.engine.CheckTaskState(mtask.Task)
			}
		}

		if !mtask.KnownStatus.Terminal() {
			// If we aren't terminal and we aren't steady state, we should be able to move some containers along
			llog.Debug("Task not steady state or terminal; progressing it")
			mtask.progressContainers()
		}

		// If we reach this point, we've changed the task in some way.
		// Conversely, for it to spin in steady state it will have to have been
		// loaded in steady state or progressed through here, so saving here should
		// be sufficient to capture state changes.
		err := mtask.engine.saver.Save()
		if err != nil {
			llog.Warn("Error checkpointing task's states to disk", "err", err)
		}
		if mtask.KnownStatus.Terminal() {
			break
		}
	}
	// We only break out of the above if this task is known to be stopped. Do
	// onetime cleanup here, including removing the task after a timeout
	llog.Debug("Task has reached stopped. We're just waiting and removing containers now")
	taskCredentialsID := mtask.GetCredentialsId()
	if taskCredentialsID != "" {
		mtask.engine.credentialsManager.RemoveCredentials(taskCredentialsID)
	}
	if mtask.StopSequenceNumber != 0 {
		llog.Debug("Marking done for this sequence", "seqnum", mtask.StopSequenceNumber)
		mtask.engine.taskStopGroup.Done(mtask.StopSequenceNumber)
	}
	mtask.cleanupTask(mtask.engine.cfg.TaskCleanupWaitDuration)
}

func (mtask *managedTask) emitCurrentStatus() {
	for _, container := range mtask.Containers {
		mtask.engine.emitContainerEvent(mtask.Task, container, "")
	}
	mtask.engine.emitTaskEvent(mtask.Task, "")
}

func (mtask *managedTask) handleDesiredStatusChange(desiredStatus api.TaskStatus, seqnum int64) {
	llog := log.New("task", mtask.Task)
	// Handle acs message changes this task's desired status to whatever
	// acs says it should be if it is compatible
	llog.Debug("New acs transition", "status", desiredStatus.String(), "seqnum", seqnum, "taskSeqnum", mtask.StopSequenceNumber)
	if desiredStatus <= mtask.GetDesiredStatus() {
		llog.Debug("Redundant task transition; ignoring", "old", mtask.GetDesiredStatus().String(), "new", desiredStatus.String())
		return
	}
	if desiredStatus == api.TaskStopped && seqnum != 0 && mtask.StopSequenceNumber == 0 {
		llog.Debug("Task moving to stopped, adding to stopgroup", "seqnum", seqnum)
		mtask.StopSequenceNumber = seqnum
		mtask.engine.taskStopGroup.Add(seqnum, 1)
	}
	mtask.SetDesiredStatus(desiredStatus)
	mtask.UpdateDesiredStatus()
}

func (mtask *managedTask) handleContainerChange(containerChange dockerContainerChange) {
	llog := log.New("task", mtask.Task)
	// Handle container change updates a container's known status.
	// In addition, if the change mentions interesting information (like
	// exit codes or ports) this propagates them.
	container := containerChange.container
	found := false
	for _, c := range mtask.Containers {
		if container == c {
			found = true
		}
	}
	if !found {
		llog.Crit("State error; task manager called with another task's container!", "container", container)
		return
	}
	event := containerChange.event
	llog.Debug("Handling container change", "change", containerChange)

	// Cases: If this is a forward transition (else) update the container to be known to be at that status.
	// If this is a backwards transition stopped->running, the first time set it
	// to be known running so it will be stopped. Subsequently ignore these backward transitions
	containerKnownStatus := container.GetKnownStatus()
	if event.Status <= containerKnownStatus && containerKnownStatus == api.ContainerStopped {
		if event.Status == api.ContainerRunning {
			// If the container becomes running after we've stopped it (possibly
			// because we got an error running it and it ran anyways), the first time
			// update it to 'known running' so that it will be driven back to stopped
			mtask.unexpectedStart.Do(func() {
				llog.Warn("Container that we thought was stopped came back; re-stopping it once")
				go mtask.engine.transitionContainer(mtask.Task, container, api.ContainerStopped)
				// This will not proceed afterwards because status <= knownstatus below
			})
		}
	}
	if event.Status <= containerKnownStatus {
		seelog.Infof("Redundant container state change for task %s: %s to %s, but already %s", mtask.Task, container, event.Status, containerKnownStatus)
		return
	}
	currentKnownStatus := containerKnownStatus
	container.SetKnownStatus(event.Status)

	if event.Error != nil {
		if container.ApplyingError == nil {
			container.ApplyingError = api.NewNamedError(event.Error)
		}
		if event.Status == api.ContainerStopped {
			// If we were trying to transition to stopped and had a timeout error
			// from docker, reset the known status to the current status and return
			// This ensures that we don't emit a containerstopped event; a
			// terminal container event from docker event stream will instead be
			// responsible for the transition. Alternatively, the steadyState check
			// could also trigger the progress and have another go at stopping the
			// container
			if event.Error.ErrorName() == dockerTimeoutErrorName {
				seelog.Infof("%s for 'docker stop' of container; ignoring state change;  task: %v, container: %v, error: %v", dockerTimeoutErrorName, mtask.Task, container, event.Error.Error())
				container.SetKnownStatus(currentKnownStatus)
				return
			}
			// If we were trying to transition to stopped and had an error, we
			// clearly can't just continue trying to transition it to stopped
			// again and again... In this case, assume it's stopped (or close
			// enough) and get on with it
			// This actually happens a lot for the case of stopping something that was not running.
			llog.Info("Error for 'docker stop' of container; assuming it's stopped anyways")
			container.SetKnownStatus(api.ContainerStopped)
			container.SetDesiredStatus(api.ContainerStopped)
		} else if event.Status == api.ContainerPulled {
			// Another special case; a failure to pull might not be fatal if e.g. the image already exists.
			llog.Info("Error while pulling container; will try to run anyways", "err", event.Error)
		} else {
			llog.Warn("Error with docker; stopping container", "container", container, "err", event.Error)
			// Leave the container known status as it is when encountered transition error,
			// as we are not sure if the container status changed or not, we will get the actual
			// status change from the docker event stream
			container.SetKnownStatus(currentKnownStatus)
			container.SetDesiredStatus(api.ContainerStopped)
			// Container known status not changed, no need for further processing
			return
		}
	}

	seelog.Debugf("Sending container change event to tcs, container: %s, status: %s", event.DockerID, event.Status)
	err := mtask.engine.containerChangeEventStream.WriteToEventStream(event)
	if err != nil {
		seelog.Warnf("Failed to write container change event to event stream, err %v", err)
	}

	if event.ExitCode != nil && event.ExitCode != container.KnownExitCode {
		container.KnownExitCode = event.ExitCode
	}
	if event.PortBindings != nil {
		container.KnownPortBindings = event.PortBindings
	}
	if event.Volumes != nil {
		mtask.UpdateMountPoints(container, event.Volumes)
	}

	mtask.engine.emitContainerEvent(mtask.Task, container, "")
	if mtask.UpdateStatus() {
		llog.Debug("Container change also resulted in task change")
		// If knownStatus changed, let it be known
		mtask.engine.emitTaskEvent(mtask.Task, "")
	}
}

func (mtask *managedTask) steadyState() bool {
	taskKnownStatus := mtask.GetKnownStatus()
	return taskKnownStatus == api.TaskRunning && taskKnownStatus >= mtask.GetDesiredStatus()
}

// waitEvent waits for any event to occur. If the event is the passed in
// channel, it will return the value written to the channel, otherwise it will
// return false
func (mtask *managedTask) waitEvent(stopWaiting <-chan bool) bool {
	log.Debug("Waiting for event for task", "task", mtask.Task)
	select {
	case acsTransition := <-mtask.acsMessages:
		log.Debug("Got acs event for task", "task", mtask.Task)
		mtask.handleDesiredStatusChange(acsTransition.desiredStatus, acsTransition.seqnum)
		return false
	case dockerChange := <-mtask.dockerMessages:
		log.Debug("Got container event for task", "task", mtask.Task)
		mtask.handleContainerChange(dockerChange)
		return false
	case b := <-stopWaiting:
		log.Debug("No longer waiting", "task", mtask.Task)
		return b
	}
}

// containerNextState determines the next state a container should go to.
// It returns: The state it should transition to, a bool indicating whether any
// action is required, and a bool indicating whether a known status change is
// possible.
// 'Stopped, false, true' -> "You can move it to known stopped, but you don't have to call a transition function"
// 'Running, true, true' -> "You can move it to running and you need to call the transition function"
// 'None, false, false' -> "This should not be moved; it has unresolved dependencies or is complete; no knownstatus change"
func (mtask *managedTask) containerNextState(container *api.Container) (api.ContainerStatus, bool, bool) {
	clog := log.New("task", mtask.Task, "container", container)
	containerKnownStatus := container.GetKnownStatus()
	containerDesiredStatus := container.GetDesiredStatus()
	if containerKnownStatus == containerDesiredStatus {
		clog.Debug("Container at desired status", "desired", containerDesiredStatus)
		return api.ContainerStatusNone, false, false
	}
	if containerKnownStatus > containerDesiredStatus {
		clog.Debug("Container past desired status")
		return api.ContainerStatusNone, false, false
	}
	if !dependencygraph.DependenciesAreResolved(container, mtask.Containers) {
		clog.Debug("Can't apply state to container yet; dependencies unresolved", "state", containerDesiredStatus)
		return api.ContainerStatusNone, false, false
	}

	var nextState api.ContainerStatus
	if container.DesiredTerminal() {
		nextState = api.ContainerStopped
		if containerKnownStatus != api.ContainerRunning {
			// If it's not currently running we do not need to do anything to make it become stopped.
			return nextState, false, true
		}
	} else {
		nextState = containerKnownStatus + 1
	}
	return nextState, true, true
}

// progressContainers tries to step forwards all containers that are able to be
// transitioned in the task's current state.
// It will continue listening to events from all channels while it does so, but
// none of those changes will be acted upon until this set of requests to
// docker completes.
func (mtask *managedTask) progressContainers() {
	log.Debug("Progressing task", "task", mtask.Task)
	// max number of transitions length to ensure writes will never block on
	// these and if we exit early transitions can exit the goroutine and it'll
	// get GC'd eventually
	transitionChange := make(chan bool, len(mtask.Containers))
	transitionChangeContainer := make(chan string, len(mtask.Containers))

	// Map of containerName -> applyingTransition
	transitionsMap := make(map[string]api.ContainerStatus)

	anyCanTransition := false
	for _, cont := range mtask.Containers {
		nextState, shouldCallTransitionFunc, canTransition := mtask.containerNextState(cont)
		if !canTransition {
			continue
		}
		// At least one container is able to be moved forwards, so we're not deadlocked
		anyCanTransition = true

		if !shouldCallTransitionFunc {
			mtask.handleContainerChange(dockerContainerChange{cont, DockerContainerChangeEvent{Status: nextState}})
			continue
		}
		transitionsMap[cont.Name] = nextState
		go func(container *api.Container, nextStatus api.ContainerStatus) {
			mtask.engine.transitionContainer(mtask.Task, container, nextStatus)
			transitionChange <- true
			transitionChangeContainer <- container.Name
		}(cont, nextState)
	}

	if !anyCanTransition {
		log.Crit("Task in a bad state; it's not steadystate but no containers want to transition", "task", mtask.Task)
		if mtask.GetDesiredStatus().Terminal() {
			// Ack, really bad. We want it to stop but the containers don't think
			// that's possible... let's just break out and hope for the best!
			log.Crit("The state is so bad that we're just giving up on it")
			mtask.UpdateKnownStatusAndTime(api.TaskStopped)
			mtask.engine.emitTaskEvent(mtask.Task, "TaskStateError: Agent could not progress task's state to stopped")
		} else {
			log.Crit("Moving task to stopped due to bad state", "task", mtask.Task)
			mtask.handleDesiredStatusChange(api.TaskStopped, 0)
		}
		return
	}

	// We've kicked off one or more transitions, wait for them to
	// complete, but keep reading events as we do.. in fact, we have to for
	// transitions to complete
	for len(transitionsMap) > 0 {
		if mtask.waitEvent(transitionChange) {
			changedContainer := <-transitionChangeContainer
			log.Debug("Transition for container finished", "task", mtask.Task, "container", changedContainer)
			delete(transitionsMap, changedContainer)
			log.Debug("Still waiting for", "map", transitionsMap)
		}
		if mtask.GetDesiredStatus().Terminal() || mtask.GetKnownStatus().Terminal() {
			allWaitingOnPulled := true
			for _, desired := range transitionsMap {
				if desired != api.ContainerPulled {
					allWaitingOnPulled = false
				}
			}
			if allWaitingOnPulled {
				// We don't actually care to wait for 'pull' transitions to finish if
				// we're just heading to stopped since those resources aren't
				// inherently linked to this task anyways for e.g. gc and so on.
				log.Debug("All waiting is for pulled transition; exiting early", "map", transitionsMap, "task", mtask.Task)
				break
			}
		}
	}
	log.Debug("Done transitioning all containers for task", "task", mtask.Task)

	if mtask.UpdateStatus() {
		log.Debug("Container change also resulted in task change")
		// If knownStatus changed, let it be known
		mtask.engine.emitTaskEvent(mtask.Task, "")
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
		log.Debug("Task Cleanup Duration is too short. Resetting to " + config.DefaultTaskCleanupWaitDuration.String())
		cleanupTimeDuration = config.DefaultTaskCleanupWaitDuration
	}
	cleanupTime := mtask.time().After(cleanupTimeDuration)
	cleanupTimeBool := make(chan bool)
	go func() {
		<-cleanupTime
		cleanupTimeBool <- true
		close(cleanupTimeBool)
	}()
	for !mtask.waitEvent(cleanupTimeBool) {
	}
	log.Info("Cleaning up task's containers and data", "task", mtask.Task)

	// For the duration of this, simply discard any task events; this ensures the
	// speedy processing of other events for other tasks
	handleCleanupDone := make(chan struct{})
	go func() {
		mtask.engine.sweepTask(mtask.Task)
		mtask.engine.state.RemoveTask(mtask.Task)
		handleCleanupDone <- struct{}{}
	}()
	// discard events while the task is being removed from engine state
	mtask.discardEventsUntil(handleCleanupDone)
	log.Debug("Finished removing task data; removing from state no longer managing", "task", mtask.Task)
	// Now remove ourselves from the global state and cleanup channels
	go mtask.discardEventsUntil(handleCleanupDone) // keep discarding events until the task is fully gone
	mtask.engine.processTasks.Lock()
	delete(mtask.engine.managedTasks, mtask.Arn)
	handleCleanupDone <- struct{}{}
	mtask.engine.processTasks.Unlock()
	mtask.engine.saver.Save()

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
