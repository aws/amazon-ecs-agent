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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerauth"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/fsouza/go-dockerclient"
)

const (
	DEFAULT_TIMEOUT_SECONDS uint = 30

	DOCKER_ENDPOINT_ENV_VARIABLE = "DOCKER_HOST"
	DOCKER_DEFAULT_ENDPOINT      = "unix:///var/run/docker.sock"
)

const (
	sweepInterval       = 5 * time.Minute
	taskStoppedDuration = 3 * time.Hour
)

// The DockerTaskEngine interacts with docker to implement a task
// engine
type DockerTaskEngine struct {
	// implements TaskEngine

	state *dockerstate.DockerTaskEngineState

	events           <-chan DockerContainerChangeEvent
	container_events chan api.ContainerStateChange
	saver            statemanager.Saver

	client DockerClient
}

// NewDockerTaskEngine returns a created, but uninitialized, DockerTaskEngine.
// The distinction between created and initialized is that when created it may
// be serialized/deserialized, but it will not communicate with docker until it
// is also initialized.
func NewDockerTaskEngine(cfg *config.Config) *DockerTaskEngine {
	dockerTaskEngine := &DockerTaskEngine{
		client: nil,
		saver:  statemanager.NewNoopStateManager(),

		state: dockerstate.NewDockerTaskEngineState(),

		container_events: make(chan api.ContainerStateChange),
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
	if engine.client == nil {
		client, err := NewDockerGoClient()
		if err != nil {
			return err
		}
		engine.client = client
	}

	// Open the event stream before we sync state so that e.g. if a container
	// goes from running to stopped after we sync with it as "running" we still
	// have the "went to stopped" event pending so we can be up to date.
	err := engine.openEventstream()
	if err != nil {
		return err
	}
	engine.synchronizeState()
	// Now catch up and start processing new events per normal
	go engine.handleDockerEvents()

	go engine.sweepTasks()

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
			var reason string
			dockerId := cont.DockerId
			currentState, err := engine.client.DescribeContainer(dockerId)
			if err != nil {
				currentState = api.ContainerDead
				if !cont.Container.KnownTerminal() {
					log.Warn("Could not describe previously known container; assuming dead", "err", err)
					reason = "Docker did not recognize container id after an ECS Agent restart."
				}
			}
			if currentState > cont.Container.KnownStatus {
				cont.Container.KnownStatus = currentState
			}
			// Even if the state didn't change, we send it upstream;
			// there's no truly reliable way to be sure we have or haven't sent
			// it already currently and there's no harm in re-sending an
			// accurate status; TODO, store that we've sent some status so we
			// can at least reduce the number of messages safely
			// We cannot actually emit an event yet because nothing is handling
			// events; just throw it in a goroutine so this doesn't block
			// forever.
			go engine.emitEvent(task, cont, reason)
		}
	}
	engine.saver.Save()
}

// sweepTasks periodically sweeps through all tasks looking for tasks that have
// been in the 'stopped' state for a sufficiently long time. At that time it
// deletes them and removes them from its "state".
func (engine *DockerTaskEngine) sweepTasks() {
	for {
		tasks := engine.state.AllTasks()

		for _, task := range tasks {
			if task.KnownStatus.Terminal() {
				if ttime.Since(task.KnownTime) > taskStoppedDuration {
					engine.sweepTask(task)
					engine.state.RemoveTask(task)
				}
			}
		}

		ttime.Sleep(sweepInterval)
	}
}

// sweepTask deletes all the containers associated with a task
func (engine *DockerTaskEngine) sweepTask(task *api.Task) {
	for _, cont := range task.Containers {
		err := engine.RemoveContainer(task, cont)
		if err != nil {
			log.Debug("Unable to remove old container", "err", err, "task", task, "cont", cont)
		}
	}
}

// emitEvent passes a given event up through the container_event channel.
// It also will update the task's knownStatus to match the container's
// knownStatus
func (engine *DockerTaskEngine) emitEvent(task *api.Task, container *api.DockerContainer, reason string) {
	err := engine.updateContainerMetadata(task, container)

	// Every time something changes, make sure the state for the thing that
	// changed is known about and move forwards if this change allows us to
	defer engine.ApplyTaskState(task)

	// Collect additional info we need for our StateChanges
	if err != nil {
		log.Crit("Error updating container metadata", "err", err)
	}
	cont := container.Container

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
		Task:          task,
		Container:     cont,
	}
	if task_change := task.UpdateTaskStatus(); task_change != api.TaskStatusNone {
		log.Info("Task change event", "state", task_change)
		event.TaskStatus = task_change
	}
	log.Info("Container change event", "event", event)
	if cont.IsInternal {
		return
	}
	engine.container_events <- event
}

// openEventstream opens, but does not consume, the docker event stream
func (engine *DockerTaskEngine) openEventstream() error {
	events, err := engine.client.ContainerEvents()
	if err != nil {
		return err
	}
	engine.events = events
	return nil
}

// handleDockerEvents must be called after openEventstream; it processes each
// event that it reads from the docker eventstream
func (engine *DockerTaskEngine) handleDockerEvents() {
	for event := range engine.events {
		log.Info("Handling an event", "event", event)

		task, task_found := engine.state.TaskById(event.DockerId)
		cont, container_found := engine.state.ContainerById(event.DockerId)
		if !task_found || !container_found {
			log.Debug("Event for container not managed", "dockerId", event.DockerId)
			continue
		}
		// Update the status to what we now know to be the true status
		if cont.Container.KnownStatus < event.Status {
			cont.Container.KnownStatus = event.Status
			engine.emitEvent(task, cont, "")
		} else if cont.Container.KnownStatus == event.Status {
			log.Warn("Redundant docker event; unusual but not critical", "event", event, "cont", cont)
		} else {
			if !cont.Container.KnownTerminal() {
				log.Crit("Docker container went backwards in state! This container will no longer be managed", "cont", cont, "event", event)
			}
		}
	}
	log.Crit("Docker event stream closed unexpectedly")
}

// updateContainerMetadata updates a minor set of metadata about a container
// that cannot be fully determined beforehand. Specifically, it will determine
// the exit code when a container stops, and the portBindings when it is started
// (and thus they are fully resolved).
func (engine *DockerTaskEngine) updateContainerMetadata(task *api.Task, container *api.DockerContainer) error {
	if container.DockerId == "" {
		return nil
	}
	llog := log.New("task", task, "container", container)
	switch container.Container.KnownStatus {
	case api.ContainerCreated:
		containerInfo, err := engine.client.InspectContainer(container.DockerId)
		if err != nil {
			llog.Error("Error inspecting container", "err", err)
			return err
		}

		task.UpdateMountPoints(container.Container, containerInfo.Volumes)
	case api.ContainerRunning:
		containerInfo, err := engine.client.InspectContainer(container.DockerId)
		if err != nil {
			llog.Error("Error inspecting container", "err", err)
			return err
		}

		// Port bindings
		if containerInfo.NetworkSettings != nil {
			// Convert port bindings into the format our container expects
			bindings, err := api.PortBindingFromDockerPortBinding(containerInfo.NetworkSettings.Ports)
			if err != nil {
				return err
			}
			container.Container.KnownPortBindings = bindings
		}
	case api.ContainerStopped:
		fallthrough
	case api.ContainerDead:
		containerInfo, err := engine.client.InspectContainer(container.DockerId)
		if err != nil {
			llog.Error("Error inspecting container", "err", err)
			return err
		}

		// Exit code
		log.Debug("Updating exit code", "exit code", containerInfo.State.ExitCode)
		container.Container.KnownExitCode = &containerInfo.State.ExitCode
	}

	return nil
}

// TaskEvents returns channels to read task and container state changes. These
// changes should be read as soon as possible as them not being read will block
// processing tasks and events.
func (engine *DockerTaskEngine) TaskEvents() <-chan api.ContainerStateChange {
	return engine.container_events
}

// TaskCompleted evaluates if a task is at a steady state; that is that all the
// containers have reached their desired status as well as the task itself
func TaskCompleted(task *api.Task) bool {
	if task.KnownStatus < task.DesiredStatus {
		return false
	}
	for _, container := range task.Containers {
		if container.KnownStatus < container.DesiredStatus {
			return false
		}
	}
	return true
}

func (engine *DockerTaskEngine) AddTask(task *api.Task) {
	task = engine.state.AddOrUpdateTask(task)

	task.PostAddTask()

	engine.ApplyTaskState(task)
}

type transitionApplyFunc (func(*api.Task, *api.Container) error)

func tryApplyTransition(task *api.Task, container *api.Container, to api.ContainerStatus, f transitionApplyFunc) error {
	err := utils.RetryNWithBackoff(utils.NewSimpleBackoff(5*time.Second, 30*time.Second, 0.25, 2), 3, func() error {
		return f(task, container)
	})

	if err == nil {
		container.AppliedStatus = to
	}
	return err
}

func (engine *DockerTaskEngine) ApplyContainerState(task *api.Task, container *api.Container) {
	container.StatusLock.Lock()
	defer container.StatusLock.Unlock()

	clog := log.New("task", task, "container", container)
	if container.IsInternal && container.AppliedStatus >= container.InternalMaxStatus {
		return
	}
	if container.KnownStatus == container.DesiredStatus {
		clog.Debug("Container at desired status", "desired", container.DesiredStatus)
		return
	}
	if container.AppliedStatus >= container.DesiredStatus {
		clog.Debug("Container already working towards desired status", "desired", container.DesiredStatus)
		return
	}
	if container.KnownStatus > container.DesiredStatus {
		clog.Debug("Container past desired status")
		return
	}
	if !dependencygraph.DependenciesAreResolved(container, task.Containers) {
		clog.Info("Can't apply state to container yet; dependencies unresolved", "state", container.DesiredStatus)
		return
	}
	// If we got here, the KnownStatus < DesiredStatus and we haven't applied
	// DesiredStatus yet; appliy a step towards it now

	var err error
	// Terminal cases are special. If our desired status is terminal, then
	// immediately go there with no regard to creating or starting the container
	if container.DesiredTerminal() {
		// Terminal cases are also special in that we do not record any error
		// in applying this state; this is because it could overwrite an error
		// that caused us to stop it and the previous error is more useful to
		// show. This is also the only state where an error results in a
		// state-change submission anyways.
		if container.AppliedStatus < api.ContainerStopped {
			err = tryApplyTransition(task, container, api.ContainerStopped, engine.StopContainer)
			if err != nil {
				clog.Info("Unable to stop container", "err", err)
				// If there was an error, assume we won't get an event in the
				// eventstream and emit it ourselves.
				container.KnownStatus = api.ContainerStopped
				engine.emitEvent(task, &api.DockerContainer{Container: container}, "")
				if _, ok := err.(*docker.NoSuchContainer); ok {
					engine.state.RemoveTask(task)
				}
				err = nil
			}
		}
	} else if container.AppliedStatus < api.ContainerPulled {
		err = tryApplyTransition(task, container, api.ContainerPulled, engine.PullContainer)
		if err != nil {
			clog.Warn("Unable to pull container image", "err", err)
		} else {
			// PullImage is a special case; update KnownStatus because there is
			// no corresponding event from the docker eventstream to update
			// this with.
			container.KnownStatus = api.ContainerPulled
		}
	}

	if !container.DesiredTerminal() {
		if container.AppliedStatus < api.ContainerCreated {
			err = tryApplyTransition(task, container, api.ContainerCreated, engine.CreateContainer)
			if err != nil {
				clog.Warn("Unable to create container", "err", err)
			}
		} else if container.AppliedStatus < api.ContainerRunning {
			err = tryApplyTransition(task, container, api.ContainerRunning, engine.StartContainer)
			if err != nil {
				clog.Warn("Unable to start container", "err", err)
			}
		}
	}

	if err != nil {
		// If we were unable to successfully accomplish a state transition,
		// we should move that container to 'stopped'
		container.ApplyingError = err
		container.DesiredStatus = api.ContainerStopped
		// Because our desired status is now stopped, we should call this
		// function again to actually stop it
		go engine.ApplyContainerState(task, container)
	} else {
		clog.Debug("Successfully applied transition")
	}

	engine.saver.Save()
}

// ApplyTaskState checks if there is any work to be done on a given task or any
// of the containers belonging to it, and if there is work to be done that can
// be done, it does it. This function can be called frequently (and should be
// called anytime a container changes) and will do nothing if the task is at a
// steady state
func (engine *DockerTaskEngine) ApplyTaskState(task *api.Task) {
	llog := log.New("task", task)
	llog.Info("Top of ApplyTaskState")

	task.InferContainerDesiredStatus()

	if !dependencygraph.ValidDependencies(task) {
		llog.Error("Invalid task dependency graph")
		return
	}
	if TaskCompleted(task) {
		llog.Info("Task completed, not acting upon it")
		return
	}

	for _, container := range task.Containers {
		go engine.ApplyContainerState(task, container)
	}
}

func (engine *DockerTaskEngine) ListTasks() ([]*api.Task, error) {
	return engine.state.AllTasks(), nil
}

func (engine *DockerTaskEngine) PullContainer(task *api.Task, container *api.Container) error {
	log.Info("Pulling container", "task", task, "container", container)

	err := engine.client.PullImage(container.Image)
	if err != nil {
		return err
	}
	return nil
}

func (engine *DockerTaskEngine) CreateContainer(task *api.Task, container *api.Container) error {
	log.Info("Creating container", "task", task, "container", container)
	config, err := task.DockerConfig(container)
	if err != nil {
		return err
	}

	err = func() error {
		// Lock state for writing so that handleDockerEvents will block on
		// resolving the 'create' event's dockerid until it is actually in the
		// added to state.
		engine.state.Lock()
		defer engine.state.Unlock()

		containerName := "ecs-" + task.Family + "-" + task.Version + "-" + container.Name + "-" + utils.RandHex()
		containerId, err := engine.client.CreateContainer(config, containerName)
		if err != nil {
			return err
		}
		engine.state.AddContainer(&api.DockerContainer{DockerId: containerId, DockerName: containerName, Container: container}, task)
		log.Info("Created container successfully", "task", task, "container", container)
		return nil
	}()
	return err
}

func (engine *DockerTaskEngine) StartContainer(task *api.Task, container *api.Container) error {
	log.Info("Starting container", "task", task, "container", container)
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		return errors.New("No such task: " + task.Arn)
	}

	dockerContainer, ok := containerMap[container.Name]
	if !ok {
		return errors.New("No container named '" + container.Name + "' created in " + task.Arn)
	}

	hostConfig, err := task.DockerHostConfig(container, containerMap)
	if err != nil {
		return err
	}

	return engine.client.StartContainer(dockerContainer.DockerId, hostConfig)
}

func (engine *DockerTaskEngine) StopContainer(task *api.Task, container *api.Container) error {
	log.Info("Stopping container", "task", task, "container", container)
	containerMap, ok := engine.state.ContainerMapByArn(task.Arn)
	if !ok {
		return errors.New("No such task: " + task.Arn)
	}

	dockerContainer, ok := containerMap[container.Name]
	if !ok {
		return errors.New("No container named '" + container.Name + "' created in " + task.Arn)
	}

	return engine.client.StopContainer(dockerContainer.DockerId)
}

func (engine *DockerTaskEngine) RemoveContainer(task *api.Task, container *api.Container) error {
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
