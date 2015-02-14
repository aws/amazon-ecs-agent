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

	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/fsouza/go-dockerclient"
)

const emptyHostVolumeName = "ecs-emptyvolume-source"

// PostAddTask is run after a task has been unmarshalled, but before it has been
// run. It is possible it will be subsequently called after that and should be
// able to handle such an occurrence appropriately (e.g. behave idempotently).
func (task *Task) PostAddTask() {
	// TODO, add rudimentary plugin support and call any plugins that want to
	// hook into this

	usedEmptyVolumes := []string{}
	for _, c := range task.Containers {
		for _, mp := range c.MountPoints {
			vol, ok := task.HostVolumeByName(mp.SourceVolume)
			if !ok {
				continue
			}
			if _, ok := vol.(*EmptyHostVolume); ok {
				if c.CreateDependencies == nil {
					c.CreateDependencies = []string{}
				}
				c.CreateDependencies = append(c.CreateDependencies, emptyHostVolumeName)
				usedEmptyVolumes = append(usedEmptyVolumes, mp.SourceVolume)
			}
		}
	}

	if len(usedEmptyVolumes) == 0 {
		return
	}

	// If we have used empty volumes, add an internal container that handles all
	// of them
	_, ok := task.ContainerByName(emptyHostVolumeName)
	if !ok {
		mountPoints := make([]MountPoint, len(usedEmptyVolumes))
		for i, u := range usedEmptyVolumes {
			containerPath := "/ecs-empty-volume/" + u
			mountPoints[i] = MountPoint{SourceVolume: u, ContainerPath: containerPath}
		}
		sourceContainer := &Container{
			Name:              emptyHostVolumeName,
			Image:             emptyvolume.Image + ":" + emptyvolume.Tag,
			Command:           []string{"na"},
			MountPoints:       mountPoints,
			Essential:         false,
			IsInternal:        true,
			InternalMaxStatus: ContainerCreated,
		}
		task.Containers = append(task.Containers, sourceContainer)
	}
}

func (task *Task) _containersByName() map[string]*Container {
	task.containersByNameLock.Lock()
	defer task.containersByNameLock.Unlock()

	if task.containersByName != nil {
		return task.containersByName
	}
	task.containersByName = make(map[string]*Container)
	for _, container := range task.Containers {
		if container.IsInternal {
			continue
		}
		task.containersByName[container.Name] = container
	}
	return task.containersByName
}

func (task *Task) ContainerByName(name string) (*Container, bool) {
	container, ok := task._containersByName()[name]
	return container, ok
}

// HostVolumeByName returns the task Volume for the given a volume name in that
// task. The second return value indicates the presense of that volume
func (task *Task) HostVolumeByName(name string) (HostVolume, bool) {
	for _, v := range task.Volumes {
		if v.Name == name {
			return v.Volume, true
		}
	}
	return nil, false
}

func (task *Task) UpdateMountPoints(cont *Container, vols map[string]string) {
	for _, m := range cont.MountPoints {
		hostPath, ok := vols[m.ContainerPath]
		if !ok {
			// /path/ -> /path
			hostPath, ok = vols[strings.TrimRight(m.ContainerPath, "/")]
		}
		if ok {
			if hostVolume, exists := task.HostVolumeByName(m.SourceVolume); exists {
				if empty, ok := hostVolume.(*EmptyHostVolume); ok {
					empty.hostPath = hostPath
				}
			}
		}
	}
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

// DockerConfig converts the given container in this task to the format of
// GoDockerClient's 'Config' struct
func (task *Task) DockerConfig(container *Container) (*docker.Config, error) {
	return task.Overridden().dockerConfig(container.Overridden())
}

func (task *Task) dockerConfig(container *Container) (*docker.Config, error) {
	dockerEnv := make([]string, 0, len(container.Environment))
	for envKey, envVal := range container.Environment {
		dockerEnv = append(dockerEnv, envKey+"="+envVal)
	}

	// Convert MB to B
	dockerMem := int64(container.Memory * 1024 * 1024)
	if dockerMem != 0 && dockerMem < DOCKER_MINIMUM_MEMORY {
		dockerMem = DOCKER_MINIMUM_MEMORY
	}
	dockerExposedPorts := make(map[docker.Port]struct{})

	for _, portBinding := range container.Ports {
		dockerPort := docker.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/tcp")
		dockerExposedPorts[dockerPort] = struct{}{}
	}

	volumeMap := make(map[string]struct{})
	for _, m := range container.MountPoints {
		vol, exists := task.HostVolumeByName(m.SourceVolume)
		if !exists {
			return nil, errors.New("Container references non-existent volume")
		}
		// you can handle most volume mount types in the HostConfig; empty
		// mounts are the only type that need to go here
		if container.Name == emptyHostVolumeName && container.IsInternal {
			_, ok := vol.(*EmptyHostVolume)
			if !ok {
				return nil, errors.New("invalid state; internal emptyvolume container with non empty volume")
			}

			volumeMap[m.ContainerPath] = struct{}{}
		}
	}

	entryPoint := []string{}
	if container.EntryPoint != nil {
		entryPoint = *container.EntryPoint
	}
	config := &docker.Config{
		Image:        container.Image,
		Cmd:          container.Command,
		Entrypoint:   entryPoint,
		ExposedPorts: dockerExposedPorts,
		Volumes:      volumeMap,
		Env:          dockerEnv,
		Memory:       dockerMem,
		CPUShares:    int64(container.Cpu),
	}
	return config, nil
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

	volumesFrom := make([]string, len(container.VolumesFrom))
	for i, volume := range container.VolumesFrom {
		targetContainer, ok := dockerContainerMap[volume.SourceContainer]
		if !ok {
			return nil, errors.New("Volume target not available: " + volume.SourceContainer)
		}
		if volume.ReadOnly {
			volumesFrom[i] = targetContainer.DockerName + ":ro"
		} else {
			volumesFrom[i] = targetContainer.DockerName
		}
	}

	binds := []string{}
	for _, m := range container.MountPoints {
		hv, ok := task.HostVolumeByName(m.SourceVolume)
		if !ok {
			return nil, errors.New("Invalid volume referenced: " + m.SourceVolume)
		}

		bind := hv.SourcePath() + ":" + m.ContainerPath
		if m.ReadOnly {
			bind += ":ro"
		}
		binds = append(binds, bind)
	}

	hostConfig := &docker.HostConfig{
		Links:        dockerLinkArr,
		Binds:        binds,
		PortBindings: dockerPortMap,
		VolumesFrom:  volumesFrom,
	}
	return hostConfig, nil
}
