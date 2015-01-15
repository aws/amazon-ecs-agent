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
)

const (
	DEFAULT_TIMEOUT_SECONDS uint = 30

	DOCKER_ENDPOINT_ENV_VARIABLE = "DOCKER_HOST"
	DOCKER_DEFAULT_ENDPOINT      = "unix:///var/run/docker.sock"
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

// updateTaskState updates the given task's status based on its container's status.
// For example, if an essential container stops, it will set the task to
// stopped.
// It returns a TaskStatus indicating what change occured or TaskStatusNone if
// there was no change
func updateTaskState(task *api.Task) api.TaskStatus {
	//The task is the minimum status of all its essential containers unless the
	//status is terminal in which case it's that status
	log.Debug("Updating task", "task", task)

	// minContainerStatus is the minimum status of all essential containers
	minContainerStatus := api.ContainerDead + 1
	// minContainerStatus is the minimum status of all containers to be used in
	// the edge case of no essential containers
	absoluteMinContainerStatus := minContainerStatus
	for _, cont := range task.Containers {
		log.Debug("On container", "cont", cont)
		if cont.KnownStatus < absoluteMinContainerStatus {
			absoluteMinContainerStatus = cont.KnownStatus
		}
		if !cont.Essential {
			continue
		}

		// Terminal states
		if cont.KnownStatus == api.ContainerStopped {
			if task.KnownStatus < api.TaskStopped {
				task.KnownStatus = api.TaskStopped
				return task.KnownStatus
			}
		} else if cont.KnownStatus == api.ContainerDead {
			if task.KnownStatus < api.TaskDead {
				task.KnownStatus = api.TaskDead
				return task.KnownStatus
			}
		}
		// Non-terminal
		if cont.KnownStatus < minContainerStatus {
			minContainerStatus = cont.KnownStatus
		}
	}

	if minContainerStatus == api.ContainerDead+1 {
		log.Warn("Task with no essential containers; all properly formed tasks should have at least one essential container", "task", task)

		// If there's no essential containers, let's just assume the container
		// with the earliest status is essential and proceed.
		minContainerStatus = absoluteMinContainerStatus
	}

	log.Info("MinContainerStatus is " + minContainerStatus.String())

	if minContainerStatus == api.ContainerCreated {
		if task.KnownStatus < api.TaskCreated {
			task.KnownStatus = api.TaskCreated
			return task.KnownStatus
		}
	} else if minContainerStatus == api.ContainerRunning {
		if task.KnownStatus < api.TaskRunning {
			task.KnownStatus = api.TaskRunning
			return task.KnownStatus
		}
	} else if minContainerStatus == api.ContainerStopped {
		if task.KnownStatus < api.TaskStopped {
			task.KnownStatus = api.TaskStopped
			return task.KnownStatus
		}
	} else if minContainerStatus == api.ContainerDead {
		if task.KnownStatus < api.TaskDead {
			task.KnownStatus = api.TaskDead
			return task.KnownStatus
		}
	}
	return api.TaskStatusNone
}

// emitEvent passes a given event up through the container_event channel.
// It also will update the task's knownStatus to match the container's
// knownStatus
func (engine *DockerTaskEngine) emitEvent(task *api.Task, container *api.DockerContainer, reason string) {
	err := engine.updateContainerMetadata(task, container)
	// Collect additional info we need for our StateChanges
	if err != nil {
		log.Crit("Error updating container metadata", "err", err)
	}
	cont := container.Container
	event := api.ContainerStateChange{
		TaskArn:       task.Arn,
		ContainerName: cont.Name,
		Status:        cont.KnownStatus,
		ExitCode:      cont.KnownExitCode,
		PortBindings:  cont.KnownPortBindings,
		Reason:        reason,
	}
	if task_change := updateTaskState(task); task_change != api.TaskStatusNone {
		log.Info("Task change event", "state", task_change)
		event.TaskStatus = task_change
	}
	log.Info("Container change event", "event", event)
	engine.container_events <- event

	// Every time something changes, make sure the state for the thing that
	// changed is known about and move forwards if this change allows us to
	engine.ApplyTaskState(task)
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
	llog := log.New("task", task, "container", container)
	switch container.Container.KnownStatus {
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

	engine.ApplyTaskState(task)
}

func (engine *DockerTaskEngine) ApplyContainerState(task *api.Task, container *api.Container) {
	container.StatusLock.Lock()
	defer container.StatusLock.Unlock()

	clog := log.New("task", task, "container", container)
	if container.KnownStatus == container.DesiredStatus {
		clog.Info("Container at desired status", "desired", container.DesiredStatus)
		return
	}
	if container.AppliedStatus >= container.DesiredStatus {
		clog.Info("Container already working towards desired status", "desired", container.DesiredStatus)
		return
	}
	if container.KnownStatus > container.DesiredStatus {
		clog.Info("Container past desired status")
		return
	}
	if !dependencygraph.DependenciesAreResolved(container, task.Containers) {
		clog.Info("Can't proceed this one, dependencies not met")
		return
	}
	// If we got here, the KnownStatus < DesiredStatus and we haven't applied
	// DesiredStatus yet; appliy a step towards it now
	var err error

	// Terminal cases are special. If our desired status is terminal, then
	// immediately go there with no regard to creating or starting the container
	if container.DesiredTerminal() {
		if container.AppliedStatus < api.ContainerStopped {
			err = engine.StopContainer(task, container)
			container.AppliedStatus = api.ContainerStopped
		} else if container.AppliedStatus < api.ContainerDead {
			err = engine.KillContainer(task, container)
			container.AppliedStatus = api.ContainerDead
		}
	} else if container.AppliedStatus < api.ContainerPulled {
		// PullImage is a special case since it has no event on the stream that
		// we can reliably resolve back to the container. Always do something
		// after PullContainer in an attempt to not get stuck pulled, but with
		// no dockerid
		err = engine.PullContainer(task, container)
		if err == nil {
			container.AppliedStatus = api.ContainerPulled
			container.KnownStatus = api.ContainerPulled
		}
	}

	if !container.DesiredTerminal() {
		if container.AppliedStatus < api.ContainerCreated {
			err = engine.CreateContainer(task, container)
			container.AppliedStatus = api.ContainerCreated
		} else if container.AppliedStatus < api.ContainerRunning {
			err = engine.StartContainer(task, container)
			container.AppliedStatus = api.ContainerRunning
		} else if container.AppliedStatus < api.ContainerStopped {
			// Terminal and shouldn't happen unless PullContainer decides this
			// can't proceed and should just be removed; can't happen now, but
			// might soon
			err = engine.StopContainer(task, container)
			container.AppliedStatus = api.ContainerStopped
		}
	}

	engine.saver.Save()

	if err != nil {
		clog.Warn("Error processing container", "err", err)
	}
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
	config, err := container.DockerConfig()
	if err != nil {
		return err
	}

	err = func() error {
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
	if err != nil {
		return err
	}
	return nil
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

func (engine *DockerTaskEngine) KillContainer(task *api.Task, container *api.Container) error {
	log.Info("Killing container", "task", task, "container", container)
	// TODO, add a cleanup trigger here so we know to delete this container
	// soon. This should also occur at some time after stop
	return nil
}

// State is a function primarily meant for testing usage; it is explicitly not
// part of the TaskEngine interface and should not be relied upon.
// It returns an internal representation of the state of this DockerTaskEngine.
func (engine *DockerTaskEngine) State() *dockerstate.DockerTaskEngineState {
	return engine.state
}
