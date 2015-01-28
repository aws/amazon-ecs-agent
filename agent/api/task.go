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

package api

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/fsouza/go-dockerclient"
)

func (task *Task) _containersByName() map[string]*Container {
	task.containersByNameLock.Lock()
	defer task.containersByNameLock.Unlock()

	if task.containersByName != nil {
		return task.containersByName
	}
	task.containersByName = make(map[string]*Container)
	for _, container := range task.Containers {
		task.containersByName[container.Name] = container
	}
	return task.containersByName
}

func (task *Task) ContainerByName(name string) (*Container, bool) {
	container, ok := task._containersByName()[name]
	return container, ok
}

// InferContainerDesiredStatus ensures that all container's desired statuses are
// compatible with whatever status the task desires to be at or is at.
// This is used both to initialize container statuses of new tasks and to force
// auxilery containers into terminal states (e.g. the essential containers died
// already)
func (task *Task) InferContainerDesiredStatus() {
	for _, c := range task.Containers {
		c.DesiredStatus = task.maxStatus().ContainerStatus()
	}
}

func (task *Task) maxStatus() *TaskStatus {
	if task.KnownStatus > task.DesiredStatus {
		return &task.KnownStatus
	}
	return &task.DesiredStatus
}

// UpdateTaskState updates the given task's status based on its container's status.
// For example, if an essential container stops, it will set the task to
// stopped.
// It returns a TaskStatus indicating what change occured or TaskStatusNone if
// there was no change
func (task *Task) UpdateTaskStatus() (newStatus TaskStatus) {
	//The task is the minimum status of all its essential containers unless the
	//status is terminal in which case it's that status
	log.Debug("Updating task", "task", task)
	defer func() {
		if newStatus != TaskStatusNone {
			task.KnownTime = time.Now()
		}
	}()

	// Set to a large 'impossible' status that can't be the min
	essentialContainersEarliestStatus := ContainerZombie
	allContainersEarliestStatus := ContainerZombie
	for _, cont := range task.Containers {
		log.Debug("On container", "cont", cont)
		if cont.KnownStatus < allContainersEarliestStatus {
			allContainersEarliestStatus = cont.KnownStatus
		}
		if !cont.Essential {
			continue
		}

		if cont.KnownStatus.Terminal() && !task.KnownStatus.Terminal() {
			// Any essential & terminal container brings down the whole task
			task.KnownStatus = TaskStopped
			return task.KnownStatus
		}
		// Non-terminal
		if cont.KnownStatus < essentialContainersEarliestStatus {
			essentialContainersEarliestStatus = cont.KnownStatus
		}
	}

	if essentialContainersEarliestStatus == ContainerZombie {
		log.Warn("Task with no essential containers; all properly formed tasks should have at least one essential container", "task", task)

		// If there are no essential containers, assume the container with the
		// earliest status is essential and proceed.
		essentialContainersEarliestStatus = allContainersEarliestStatus
	}

	log.Debug("Earliest essential status is " + essentialContainersEarliestStatus.String())

	if essentialContainersEarliestStatus == ContainerCreated {
		if task.KnownStatus < TaskCreated {
			task.KnownStatus = TaskCreated
			return task.KnownStatus
		}
	} else if essentialContainersEarliestStatus == ContainerRunning {
		if task.KnownStatus < TaskRunning {
			task.KnownStatus = TaskRunning
			return task.KnownStatus
		}
	} else if essentialContainersEarliestStatus == ContainerStopped {
		if task.KnownStatus < TaskStopped {
			task.KnownStatus = TaskStopped
			return task.KnownStatus
		}
	} else if essentialContainersEarliestStatus == ContainerDead {
		if task.KnownStatus < TaskDead {
			task.KnownStatus = TaskDead
			return task.KnownStatus
		}
	}
	return TaskStatusNone
}

// Overridden returns a copy of the task with all container's overridden and
// itself overridden as well
func (task *Task) Overridden() *Task {
	result := *task
	// Task has no overrides currently, just do the containers

	// Shallow copy, take care of the deeper bits too
	result.containersByNameLock.Lock()
	result.containersByName = make(map[string]*Container)
	result.containersByNameLock.Unlock()

	result.Containers = make([]*Container, len(result.Containers))
	for i, cont := range task.Containers {
		result.Containers[i] = cont.Overridden()
	}
	return &result
}

func (task *Task) DockerHostConfig(container *Container, dockerContainerMap map[string]*DockerContainer) (*docker.HostConfig, error) {
	return task.Overridden().dockerHostConfig(container.Overridden(), dockerContainerMap)
}

func (task *Task) dockerHostConfig(container *Container, dockerContainerMap map[string]*DockerContainer) (*docker.HostConfig, error) {
	dockerLinkArr := make([]string, 0, len(container.Links))
	for _, link := range container.Links {
		linkParts := strings.Split(link, ":")
		if len(linkParts) > 2 {
			return nil, errors.New("Invalid link format")
		}
		linkName := linkParts[0]
		var linkAlias string

		if len(linkParts) == 2 {
			linkAlias = linkParts[1]
		} else {
			log.Warn("Warning, link with linkalias", "linkName", linkName, "task", task, "container", container)
			linkAlias = linkName
		}

		targetContainer, ok := dockerContainerMap[linkName]
		if !ok {
			return nil, errors.New("Link target not available: " + linkName)
		}
		fixedLink := targetContainer.DockerName + ":" + linkAlias
		dockerLinkArr = append(dockerLinkArr, fixedLink)
	}

	dockerPortMap := make(map[docker.Port][]docker.PortBinding)

	for _, portBinding := range container.Ports {
		dockerPort := docker.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/tcp")
		currentMappings, existing := dockerPortMap[dockerPort]
		if existing {
			dockerPortMap[dockerPort] = append(currentMappings, docker.PortBinding{HostIP: "0.0.0.0", HostPort: strconv.Itoa(int(portBinding.HostPort))})
		} else {
			dockerPortMap[dockerPort] = []docker.PortBinding{docker.PortBinding{HostIP: "0.0.0.0", HostPort: strconv.Itoa(int(portBinding.HostPort))}}
		}
	}

	hostConfig := &docker.HostConfig{Links: dockerLinkArr, Binds: container.BindMounts, PortBindings: dockerPortMap}
	return hostConfig, nil
}
