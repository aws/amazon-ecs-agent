// Copyright 2014 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/logger"
)

var log = logger.ForModule("dockerstate")

// dockerTaskEngineState keeps track of all mappings between tasks we know about
// and containers docker runs
// It contains a mutex that can be used to ensure out-of-date state cannot be
// accessed before an update comes and to ensure multiple goroutines can safely
// work with it.
//
// The methods on it will only aquire the read lock and it is up to the user to
// aquire the write lock (via Lock()) before modifying data via one of the Add*
// methods
//
// Some information is duplicated in the interest of having efficient lookups
type DockerTaskEngineState struct {
	lock sync.RWMutex

	tasks             map[string]*api.Task                       // taskarn -> api.Task
	idToTask          map[string]string                          // DockerId -> taskarn
	taskToId          map[string]map[string]*api.DockerContainer // taskarn -> (containername -> api.DockerContainer)
	idToContainer     map[string]*api.DockerContainer            // DockerId -> api.DockerContainer
	imageToContainers map[string][]*api.DockerContainer          // image -> api.DockerContainers
}

func NewDockerTaskEngineState() *DockerTaskEngineState {
	return &DockerTaskEngineState{
		tasks:             make(map[string]*api.Task),
		idToTask:          make(map[string]string),
		taskToId:          make(map[string]map[string]*api.DockerContainer),
		idToContainer:     make(map[string]*api.DockerContainer),
		imageToContainers: make(map[string][]*api.DockerContainer),
	}
}

func (state *DockerTaskEngineState) ContainerById(id string) (*api.DockerContainer, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	c, ok := state.idToContainer[id]
	return c, ok
}

func (state *DockerTaskEngineState) ContainerMapByArn(arn string) (map[string]*api.DockerContainer, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	ret, ok := state.taskToId[arn]
	return ret, ok
}

func (state *DockerTaskEngineState) ContainersByImage(image string) []*api.DockerContainer {
	state.lock.RLock()
	defer state.lock.RUnlock()
	conts, ok := state.imageToContainers[image]
	if !ok {
		return []*api.DockerContainer{}
	}
	return conts
}

func (state *DockerTaskEngineState) TaskById(cid string) (*api.Task, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	arn, found := state.idToTask[cid]
	if !found {
		return nil, false
	}
	return state.TaskByArn(arn)
}

func (state *DockerTaskEngineState) Lock() {
	log.Debug("State taking lock")
	state.lock.Lock()
}

func (state *DockerTaskEngineState) Unlock() {
	log.Debug("State releasing lock")
	state.lock.Unlock()
}

func (state *DockerTaskEngineState) AddOrUpdateTask(task *api.Task) *api.Task {
	state.Lock()
	defer state.Unlock()

	current, exists := state.tasks[task.Arn]
	if !exists {
		state.AddTask(task)
		return task
	}

	// Update
	if task.DesiredStatus > current.DesiredStatus {
		current.DesiredStatus = task.DesiredStatus
	}

	return current
}

// Users should aquire a lock before using this
func (state *DockerTaskEngineState) AddTask(task *api.Task) {
	state.tasks[task.Arn] = task
}

// Users should aquire a lock before using this
func (state *DockerTaskEngineState) AddContainer(container *api.DockerContainer, task *api.Task) {
	if task == nil || container == nil {
		log.Crit("Addtask called with nil task/container! This should not happen")
		return
	}

	_, exists := state.tasks[task.Arn]
	if !exists {
		state.AddTask(task)
	}

	state.idToTask[container.DockerId] = task.Arn
	existingMap, exists := state.taskToId[task.Arn]
	if !exists {
		existingMap = make(map[string]*api.DockerContainer, len(task.Containers))
		state.taskToId[task.Arn] = existingMap
	}
	existingMap[container.Container.Name] = container

	state.idToContainer[container.DockerId] = container

	existingContainers, exists := state.imageToContainers[container.Container.Image]
	if !exists {
		state.imageToContainers[container.Container.Image] = make([]*api.DockerContainer, 0)
	}
	state.imageToContainers[container.Container.Image] = append(existingContainers, container)
}

func (state *DockerTaskEngineState) TaskByArn(arn string) (*api.Task, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	t, ok := state.tasks[arn]
	return t, ok
}

func (state *DockerTaskEngineState) AllTasks() []*api.Task {
	state.lock.RLock()
	defer state.lock.RUnlock()

	ret := make([]*api.Task, len(state.tasks))
	ndx := 0
	for _, task := range state.tasks {
		ret[ndx] = task
		ndx += 1
	}
	return ret
}
