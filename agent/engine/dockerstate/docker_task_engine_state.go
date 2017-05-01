// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package dockerstate

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	"github.com/aws/amazon-ecs-agent/agent/logger"
)

var log = logger.ForModule("dockerstate")

// TaskEngineState keeps track of all mappings between tasks we know about
// and containers docker runs
type TaskEngineState interface {
	// AllTasks returns all of the tasks
	AllTasks() []*api.Task
	// AllImageStates returns all of the image.ImageStates
	AllImageStates() []*image.ImageState
	// ContainerByID returns an api.DockerContainer for a given container ID
	ContainerByID(id string) (*api.DockerContainer, bool)
	// ContainerMapByArn returns a map of containers belonging to a particular task ARN
	ContainerMapByArn(arn string) (map[string]*api.DockerContainer, bool)
	// TaskByShortID retrieves the task of a given docker short container id
	TaskByShortID(cid string) ([]*api.Task, bool)
	// TaskByID returns an api.Task for a given container ID
	TaskByID(cid string) (*api.Task, bool)
	// TaskByArn returns a task for a given ARN
	TaskByArn(arn string) (*api.Task, bool)
	// AddTask adds a task to the state to be stored
	AddTask(task *api.Task)
	// AddContainer adds a container to the state to be stored for a given task
	AddContainer(container *api.DockerContainer, task *api.Task)
	// AddImageState adds an image.ImageState to be stored
	AddImageState(imageState *image.ImageState)
	// RemoveTask removes a task from the state
	RemoveTask(task *api.Task)
	// RemoveImageState removes an image.ImageState
	RemoveImageState(imageState *image.ImageState)
	json.Marshaler
	json.Unmarshaler
}

// DockerTaskEngineState keeps track of all mappings between tasks we know about
// and containers docker runs
// It contains a mutex that can be used to ensure out-of-date state cannot be
// accessed before an update comes and to ensure multiple goroutines can safely
// work with it.
//
// The methods on it will aquire the read lock, but not all aquire the write
// lock (sometimes it is up to the caller). This is because the write lock for
// containers should encapsulate the creation of the resource as well as adding,
// and creating the resource (docker container) is outside the scope of this
// package. This isn't ideal usage and I'm open to this being reworked/improved.
//
// Some information is duplicated in the interest of having efficient lookups
type DockerTaskEngineState struct {
	lock sync.RWMutex

	tasks         map[string]*api.Task                       // taskarn -> api.Task
	idToTask      map[string]string                          // DockerId -> taskarn
	taskToID      map[string]map[string]*api.DockerContainer // taskarn -> (containername -> api.DockerContainer)
	idToContainer map[string]*api.DockerContainer            // DockerId -> api.DockerContainer
	imageStates   map[string]*image.ImageState
}

// NewTaskEngineState returns a new TaskEngineState
func NewTaskEngineState() TaskEngineState {
	return newDockerTaskEngineState()
}

func newDockerTaskEngineState() *DockerTaskEngineState {
	return &DockerTaskEngineState{
		tasks:         make(map[string]*api.Task),
		idToTask:      make(map[string]string),
		taskToID:      make(map[string]map[string]*api.DockerContainer),
		idToContainer: make(map[string]*api.DockerContainer),
		imageStates:   make(map[string]*image.ImageState),
	}
}

// AllTasks returns all of the tasks
func (state *DockerTaskEngineState) AllTasks() []*api.Task {
	state.lock.RLock()
	defer state.lock.RUnlock()

	return state.allTasks()
}

func (state *DockerTaskEngineState) allTasks() []*api.Task {
	ret := make([]*api.Task, len(state.tasks))
	ndx := 0
	for _, task := range state.tasks {
		ret[ndx] = task
		ndx++
	}
	return ret
}

// AllImageStates returns all of the image.ImageStates
func (state *DockerTaskEngineState) AllImageStates() []*image.ImageState {
	state.lock.RLock()
	defer state.lock.RUnlock()

	return state.allImageStates()
}

func (state *DockerTaskEngineState) allImageStates() []*image.ImageState {
	var allImageStates []*image.ImageState
	for _, imageState := range state.imageStates {
		allImageStates = append(allImageStates, imageState)
	}
	return allImageStates
}

// ContainerByID returns an api.DockerContainer for a given container ID
func (state *DockerTaskEngineState) ContainerByID(id string) (*api.DockerContainer, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	c, ok := state.idToContainer[id]
	return c, ok
}

// ContainerMapByArn returns a map of containers belonging to a particular task ARN
func (state *DockerTaskEngineState) ContainerMapByArn(arn string) (map[string]*api.DockerContainer, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	ret, ok := state.taskToID[arn]
	return ret, ok
}

// TaskByShortID retrieves the task of a given docker short container id
func (state *DockerTaskEngineState) TaskByShortID(cid string) ([]*api.Task, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	var tasks []*api.Task
	for id := range state.idToTask {
		if strings.HasPrefix(id, cid) {
			task, _ := state.TaskByID(id)
			tasks = append(tasks, task)
		}
	}

	return tasks, len(tasks) > 0
}

// TaskByID retrieves the task of a given docker container id
func (state *DockerTaskEngineState) TaskByID(cid string) (*api.Task, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	arn, found := state.idToTask[cid]
	if !found {
		return nil, false
	}
	return state.taskByArn(arn)
}

// TaskByArn returns a task for a given ARN
func (state *DockerTaskEngineState) TaskByArn(arn string) (*api.Task, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	return state.taskByArn(arn)
}

func (state *DockerTaskEngineState) taskByArn(arn string) (*api.Task, bool) {
	t, ok := state.tasks[arn]
	return t, ok
}

// AddTask adds a new task to the state
func (state *DockerTaskEngineState) AddTask(task *api.Task) {
	state.lock.Lock()
	defer state.lock.Unlock()

	state.tasks[task.Arn] = task
}

// AddContainer adds a container to the state.
// If the container has been added with only a name and no docker-id, this
// updates the state to include the docker id
func (state *DockerTaskEngineState) AddContainer(container *api.DockerContainer, task *api.Task) {
	state.lock.Lock()
	defer state.lock.Unlock()
	if task == nil || container == nil {
		log.Crit("Addcontainer called with nil task/container")
		return
	}

	_, exists := state.tasks[task.Arn]
	if !exists {
		log.Debug("AddContainer called with unknown task; adding", "arn", task.Arn)
		state.tasks[task.Arn] = task
	}

	if container.DockerID != "" {
		state.idToTask[container.DockerID] = task.Arn
	}
	existingMap, exists := state.taskToID[task.Arn]
	if !exists {
		existingMap = make(map[string]*api.DockerContainer, len(task.Containers))
		state.taskToID[task.Arn] = existingMap
	}
	existingMap[container.Container.Name] = container

	if container.DockerID != "" {
		state.idToContainer[container.DockerID] = container
	}
}

// AddImageState adds an image.ImageState to be stored
func (state *DockerTaskEngineState) AddImageState(imageState *image.ImageState) {
	if imageState == nil {
		log.Debug("Cannot add empty image state")
		return
	}
	if imageState.Image.ImageID == "" {
		log.Debug("Cannot add image state with empty image id")
		return
	}
	state.lock.Lock()
	defer state.lock.Unlock()

	state.imageStates[imageState.Image.ImageID] = imageState
}

// RemoveTask removes a task from this state. It removes all containers and
// other associated metadata. It does acquire the write lock.
func (state *DockerTaskEngineState) RemoveTask(task *api.Task) {
	state.lock.Lock()
	defer state.lock.Unlock()

	task, ok := state.tasks[task.Arn]
	if !ok {
		return
	}
	delete(state.tasks, task.Arn)
	containerMap, ok := state.taskToID[task.Arn]
	if !ok {
		return
	}
	delete(state.taskToID, task.Arn)

	for _, dockerContainer := range containerMap {
		delete(state.idToTask, dockerContainer.DockerID)
		delete(state.idToContainer, dockerContainer.DockerID)
	}
}

// RemoveImageState removes an image.ImageState
func (state *DockerTaskEngineState) RemoveImageState(imageState *image.ImageState) {
	if imageState == nil {
		log.Debug("Cannot remove empty image state")
		return
	}
	state.lock.Lock()
	defer state.lock.Unlock()

	imageState, ok := state.imageStates[imageState.Image.ImageID]
	if !ok {
		log.Debug("Image State is not found. Cannot be removed")
		return
	}
	delete(state.imageStates, imageState.Image.ImageID)
}
