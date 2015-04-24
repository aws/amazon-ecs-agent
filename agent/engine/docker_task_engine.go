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
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerauth"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	utilsync "github.com/aws/amazon-ecs-agent/agent/utils/sync"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
)

const (
	DEFAULT_TIMEOUT_SECONDS uint = 30

	DOCKER_ENDPOINT_ENV_VARIABLE = "DOCKER_HOST"
	DOCKER_DEFAULT_ENDPOINT      = "unix:///var/run/docker.sock"
)

const (
	taskStoppedDuration           = 3 * time.Hour
	steadyStateTaskVerifyInterval = 5 * time.Minute
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

type managedTask struct {
	*api.Task

	acsMessages    chan acsTransition
	dockerMessages chan dockerContainerChange
}

func (engine *DockerTaskEngine) newManagedTask(task *api.Task) *managedTask {
	t := &managedTask{
		Task:           task,
		acsMessages:    make(chan acsTransition),
		dockerMessages: make(chan dockerContainerChange),
	}
	engine.managedTasks[task.Arn] = t
	return t
}

// The DockerTaskEngine interacts with docker to implement a task
// engine
type DockerTaskEngine struct {
	// implements TaskEngine

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

	client DockerClient

	stopEngine context.CancelFunc

	// processTasks is a mutex that the task engine must aquire before changing
	// any task's state which it manages. Since this is a lock that encompasses
	// all tasks, it must not aquire it for any significant duration
	processTasks sync.Mutex
}

// NewDockerTaskEngine returns a created, but uninitialized, DockerTaskEngine.
// The distinction between created and initialized is that when created it may
// be serialized/deserialized, but it will not communicate with docker until it
// is also initialized.
func NewDockerTaskEngine(cfg *config.Config) *DockerTaskEngine {
	dockerTaskEngine := &DockerTaskEngine{
		client: nil,
		saver:  statemanager.NewNoopStateManager(),

		state:         dockerstate.NewDockerTaskEngineState(),
		managedTasks:  make(map[string]*managedTask),
		taskStopGroup: utilsync.NewSequentialWaitGroup(),

		containerEvents: make(chan api.ContainerStateChange),
		taskEvents:      make(chan api.TaskStateChange),
	}
	dockerauth.SetConfig(cfg)

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

	// TODO, pass ina a context from main from background so that other things can stop us, not just the tests
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

	return nil
}

func (engine *DockerTaskEngine) initDockerClient() error {
	if engine.client == nil {
		client, err := NewDockerGoClient()
		if err != nil {
			return err
		}
		engine.client = client
	}
	return nil
}

// MustInit blocks and retries until an engine can be initialized.
func (engine *DockerTaskEngine) MustInit() {
	if engine.client != nil {
		return
	}

	errorOnce := sync.Once{}
	taskEngineConnectBackoff := utils.NewSimpleBackoff(200*time.Millisecond, 2*time.Second, 0.20, 1.5)
	utils.RetryWithBackoff(taskEngineConnectBackoff, func() error {
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
	tasks := engine.state.AllTasks()
	for _, task := range tasks {
		conts, ok := engine.state.ContainerMapByArn(task.Arn)
		if !ok {
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
				currentState, err := engine.client.DescribeContainer(cont.DockerId)
				if err != nil {
					currentState = api.ContainerStopped
					if !cont.Container.KnownTerminal() {
						// TODO error type
						cont.Container.ApplyingError = api.NewApplyingError(errors.New("Docker did not recognize container id after an ECS Agent restart"))
						log.Warn("Could not describe previously known container; assuming dead", "err", err)
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
				continue
			}
			engine.processTasks.Lock()
			managedTask, ok := engine.managedTasks[task.Arn]
			engine.processTasks.Unlock()
			if !ok {
				log.Crit("Could not find managed task corresponding to a docker event", "event", event, "task", task)
			}
			managedTask.dockerMessages <- dockerContainerChange{container: cont.Container, event: event}
		}
	}
}

// TaskEvents returns channels to read task and container state changes. These
// changes should be read as soon as possible as them not being read will block
// processing the task referenced by the event.
func (engine *DockerTaskEngine) TaskEvents() (<-chan api.TaskStateChange, <-chan api.ContainerStateChange) {
	return engine.taskEvents, engine.containerEvents
}

// startTask creates a taskState construct to track the task and then begins
// pushing it towards its desired state when allowed startTask is protected by
// the processTasks lock of 'AddTask'. It should not be called from anywhere
// else and should exit quickly to allow AddTask to do more work.
func (engine *DockerTaskEngine) startTask(task *api.Task) {
	// Create a channel that may be used to communicate with this task, survey
	// what tasks need to be waited for for this one to start, and then spin off
	// a goroutine to oversee this task

	thisTask := engine.newManagedTask(task)

	go func(task *managedTask) {
		llog := log.New("task", task)
		// This goroutine now owns the lifecycle of this managedTask. No other
		// thread should access this task or any of its members.
		if task.StartSequenceNumber != 0 {
			llog.Debug("Waiting for any previous stops to complete", "seqnum", task.StartSequenceNumber)
			engine.taskStopGroup.Wait(task.StartSequenceNumber)
			llog.Debug("Wait succeeded; ready to start")
		}

		// Do a single updatestatus at the beginning to create the container
		// 'desiredstatus'es which are a construct of the engine used only here,
		// not present on the backend
		task.UpdateStatus()
		for {
			// First check for new transitions and let them 'skip to the head of the line' so to speak
			// This lets a desiredStopped preempt moving to running / created
			handleAcsMessage := func(desiredStatus api.TaskStatus, seqnum int64) {
				// Handle acs message changes this task's desired status to whatever
				// acs says it should be if it is compatible
				llog.Debug("New acs transition", "status", desiredStatus.String(), "seqnum", seqnum, "taskSeqnum", task.StopSequenceNumber)
				if desiredStatus == api.TaskStopped && seqnum != 0 && task.StopSequenceNumber == 0 {
					llog.Debug("Task moving to stopped, adding to stopgroup", "seqnum", seqnum)
					task.StopSequenceNumber = seqnum
					engine.taskStopGroup.Add(seqnum, 1)
				}
				task.DesiredStatus = desiredStatus
				task.UpdateDesiredStatus()
			}
			handleContainerChange := func(containerChange dockerContainerChange) {
				// Handle container change updates a container's known status.
				// In addition, if the change mentions interesting information (like
				// exit codes or ports) this propegates them.
				container := containerChange.container
				found := false
				for _, c := range task.Containers {
					if container == c {
						found = true
					}
				}
				if !found {
					llog.Crit("State error; task manager called with another task's container!", "container", container)
					return
				}
				event := containerChange.event

				if event.Status <= container.KnownStatus {
					llog.Info("Redundant status change; ignoring", "current", container.KnownStatus.String(), "change", event.Status.String())
					return
				}
				if event.Error != nil {
					if event.Status == api.ContainerStopped {
						// If we were trying to transition to stopped and had an error, we
						// clearly can't just continue trying to transition it to stopped
						// again and again... In this case, assume it's stopped (or close
						// enough) and get on with it
						// This actually happens a lot for the case of stopping something that was not running.
						llog.Info("Error for 'docker stop' of container; assuming it's stopped anyways")
						container.KnownStatus = api.ContainerStopped
						container.DesiredStatus = api.ContainerStopped
					} else if event.Status == api.ContainerPulled {
						// Another special case; a failure to pull might not be fatal if e.g. the image already exists.
					} else {
						llog.Warn("Error with docker; stopping container", "container", container, "err", event.Error)
						container.DesiredStatus = api.ContainerStopped
					}
					if container.ApplyingError == nil {
						container.ApplyingError = api.NewApplyingError(event.Error)
					}
				}
				container.KnownStatus = event.Status

				if event.ExitCode != nil && event.ExitCode != container.KnownExitCode {
					container.KnownExitCode = event.ExitCode
				}
				if event.PortBindings != nil {
					container.KnownPortBindings = event.PortBindings
				}
				if event.Volumes != nil {
					// No need to emit an event for this; this information is not propogated up yet
					task.UpdateMountPoints(container, event.Volumes)
				}

				engine.emitContainerEvent(task.Task, container, "")
				if task.UpdateStatus() {
					llog.Debug("Container change also resulted in task change")
					// If knownStatus changed, let it be known
					engine.emitTaskEvent(task.Task, "")
				}
			}

			// If it's steadyState, just spin until we need to do work
			for task.KnownStatus == api.TaskRunning && task.KnownStatus >= task.DesiredStatus {
				llog.Debug("Task at steady state", "state", task.KnownStatus.String())
				maxWait := time.NewTimer(steadyStateTaskVerifyInterval)

				select {
				case acsTransition := <-task.acsMessages:
					handleAcsMessage(acsTransition.desiredStatus, acsTransition.seqnum)
				case dockerChange := <-task.dockerMessages:
					handleContainerChange(dockerChange)
				case <-maxWait.C:
					llog.Debug("No task events in wait time; inspecting just in case")
					//engine.verifyTaskStatus(task) // TODO
				}
				maxWait.Stop()
			}

			if task.KnownStatus != api.TaskStopped {
				// If our task is not steady state we should be able to move forwards one or more containers
				anyChanged := false
				for _, cont := range task.Containers {
					newStatus, metadata, changed := engine.applyContainerState(task.Task, cont)
					handleContainerChange(dockerContainerChange{
						container: cont,
						event: DockerContainerChangeEvent{
							Status:                  newStatus,
							DockerContainerMetadata: metadata,
						},
					})
					if changed {
						anyChanged = true
					}
				}

				if !anyChanged {
					llog.Crit("Task in a bad state; it's not steadystate but no containers want to transition")
					if task.DesiredStatus == api.TaskStopped {
						// Ack, really bad. We want it to stop but the containers don't think
						// that's possible... let's just break out and hope for the best!
						llog.Crit("The state is so bad that we're just giving up on it")
						task.KnownStatus = api.TaskStopped
						engine.emitTaskEvent(task.Task, "TaskStateError: Agent could not progress task's state to stopped")
					} else {
						llog.Crit("Moving task to stopped due to bad state")
						task.DesiredStatus = api.TaskStopped
					}
				}
			}

			if task.KnownStatus == api.TaskStopped {
				break
			}
		}
		// We only break out of the above if this task is known to be stopped. Do
		// onetime cleanup here, including removing the task after a timeout
		llog.Debug("Task has reached stopped. We're just waiting and removing containers now")
		if task.StopSequenceNumber != 0 {
			llog.Debug("Marking done for this sequence", "seqnum", task.StopSequenceNumber)
			engine.taskStopGroup.Done(task.StopSequenceNumber)
		}
		cleanupTime := ttime.After(task.KnownTime.Add(taskStoppedDuration).Sub(ttime.Now()))

	ContinueCleanup:
		for {
			select {
			case <-task.dockerMessages:
			case <-task.acsMessages:
				llog.Debug("ACS message recieved for already stopped task")
			case <-cleanupTime:
				llog.Debug("Cleaning up task's containers and data")
				break ContinueCleanup
			}
		}

		// First make an attempt to cleanup resources
		engine.sweepTask(task.Task)
		engine.state.RemoveTask(task.Task)
		// Now remove ourselves from the global state and cleanup channels
		engine.processTasks.Lock()
		delete(engine.managedTasks, task.Arn)
	FinishCleanup:
		for {
			// Cleanup any leftover messages before closing. No new messages possible
			// because we deleted ourselves from managedTasks, so this removes all stale ones
			select {
			case <-task.dockerMessages:
			case <-task.acsMessages:
			default:
				break FinishCleanup
			}
		}
		engine.processTasks.Unlock()

		close(task.dockerMessages)
		close(task.acsMessages)
	}(thisTask)
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

func (engine *DockerTaskEngine) pullContainer(task *api.Task, container *api.Container) DockerContainerMetadata {
	log.Info("Pulling container", "task", task, "container", container)

	return engine.client.PullImage(container.Image)
}

func (engine *DockerTaskEngine) createContainer(task *api.Task, container *api.Container) DockerContainerMetadata {
	log.Info("Creating container", "task", task, "container", container)
	config, err := task.DockerConfig(container)
	if err != nil {
		return DockerContainerMetadata{Error: err}
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

	metadata := engine.client.CreateContainer(config, containerName)
	if metadata.Error != nil {
		return metadata
	}
	engine.state.AddContainer(&api.DockerContainer{DockerId: metadata.DockerId, DockerName: containerName, Container: container}, task)
	log.Info("Created container successfully", "task", task, "container", container)
	return metadata
}

func (engine *DockerTaskEngine) startContainer(task *api.Task, container *api.Container) DockerContainerMetadata {
	log.Info("Starting container", "task", task, "container", container)
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		// TODO error type
		return DockerContainerMetadata{Error: errors.New("No such task: " + task.Arn)}
	}

	dockerContainer, ok := containerMap[container.Name]
	if !ok {
		// TODO type
		return DockerContainerMetadata{Error: errors.New("No container named '" + container.Name + "' created in " + task.Arn)}
	}

	hostConfig, err := task.DockerHostConfig(container, containerMap)
	if err != nil {
		// TODO hostconfig function neesd an error tyep
		return DockerContainerMetadata{Error: err}
	}

	return engine.client.StartContainer(dockerContainer.DockerId, hostConfig)
}

func (engine *DockerTaskEngine) stopContainer(task *api.Task, container *api.Container) DockerContainerMetadata {
	log.Info("Stopping container", "task", task, "container", container)
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		// TODO error type
		return DockerContainerMetadata{Error: errors.New("No such task: " + task.Arn)}
	}

	dockerContainer, ok := containerMap[container.Name]
	if !ok {
		// TODO type
		return DockerContainerMetadata{Error: errors.New("No container named '" + container.Name + "' created in " + task.Arn)}
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

// State is a function primarily meant for testing usage; it is explicitly not
// part of the TaskEngine interface and should not be relied upon.
// It returns an internal representation of the state of this DockerTaskEngine.
func (engine *DockerTaskEngine) State() *dockerstate.DockerTaskEngineState {
	return engine.state
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

func (engine *DockerTaskEngine) transitionFunctionMap() map[api.ContainerStatus]transitionApplyFunc {
	return map[api.ContainerStatus]transitionApplyFunc{
		api.ContainerPulled:  engine.pullContainer,
		api.ContainerCreated: engine.createContainer,
		api.ContainerRunning: engine.startContainer,
		api.ContainerStopped: engine.stopContainer,
	}
}

func (engine *DockerTaskEngine) applyContainerState(task *api.Task, container *api.Container) (api.ContainerStatus, DockerContainerMetadata, bool) {
	clog := log.New("task", task, "container", container)
	if container.KnownStatus == container.DesiredStatus {
		clog.Debug("Container at desired status", "desired", container.DesiredStatus)
		return api.ContainerStatusNone, DockerContainerMetadata{}, false
	}
	if container.KnownStatus > container.DesiredStatus {
		clog.Debug("Container past desired status")
		return api.ContainerStatusNone, DockerContainerMetadata{}, false
	}
	if !dependencygraph.DependenciesAreResolved(container, task.Containers) {
		clog.Debug("Can't apply state to container yet; dependencies unresolved", "state", container.DesiredStatus)
		return api.ContainerStatusNone, DockerContainerMetadata{}, false
	}
	// If we got here, the KnownStatus < DesiredStatus

	var nextState api.ContainerStatus
	if container.DesiredTerminal() {
		nextState = api.ContainerStopped
	} else {
		nextState = container.KnownStatus + 1
	}

	transitionFunction, ok := engine.transitionFunctionMap()[nextState]
	if !ok {
		clog.Crit("Container desired to transition to an unsupported state", "state", nextState.String())
		// TODO THIS NEEDS TO BE A REAL UNRETRIABLE ERROR
		return api.ContainerStatusNone, DockerContainerMetadata{Error: errors.New("Things went to crap :(")}, false
	}

	metadata := tryApplyTransition(task, container, nextState, transitionFunction)
	if metadata.Error != nil {
		clog.Info("Error transitioning container", "state", nextState.String())
	} else {
		clog.Debug("Transitioned container", "state", nextState.String())
		engine.saver.Save()
	}
	return nextState, metadata, true
}
