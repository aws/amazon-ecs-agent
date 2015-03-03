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

const emptyHostVolumeName = "~internal~ecs-emptyvolume-source"

// PostUnmarshalTask is run after a task has been unmarshalled, but before it has been
// run. It is possible it will be subsequently called after that and should be
// able to handle such an occurrence appropriately (e.g. behave idempotently).
func (task *Task) PostUnmarshalTask() {
	// TODO, add rudimentary plugin support and call any plugins that want to
	// hook into this

	task.initializeEmptyVolumes()
}

func (task *Task) initializeEmptyVolumes() {
	requiredEmptyVolumes := []string{}
	for _, container := range task.Containers {
		for _, mountPoint := range container.MountPoints {
			vol, ok := task.HostVolumeByName(mountPoint.SourceVolume)
			if !ok {
				continue
			}
			if _, ok := vol.(*EmptyHostVolume); ok {
				if container.RunDependencies == nil {
					container.RunDependencies = make([]string, 0)
				}
				container.RunDependencies = append(container.RunDependencies, emptyHostVolumeName)
				requiredEmptyVolumes = append(requiredEmptyVolumes, mountPoint.SourceVolume)
			}
		}
	}

	if len(requiredEmptyVolumes) == 0 {
		// No need to create the auxiliary 'empty-volumes' container
		return
	}

	// If we have required empty volumes, add an 'internal' container that handles all
	// of them
	_, ok := task.ContainerByName(emptyHostVolumeName)
	if !ok {
		mountPoints := make([]MountPoint, len(requiredEmptyVolumes))
		for i, volume := range requiredEmptyVolumes {
			containerPath := "/ecs-empty-volume/" + volume
			mountPoints[i] = MountPoint{SourceVolume: volume, ContainerPath: containerPath}
		}
		sourceContainer := &Container{
			Name:          emptyHostVolumeName,
			Image:         emptyvolume.Image + ":" + emptyvolume.Tag,
			Command:       []string{"not-applicable"}, // Command required, but this only gets created so N/A
			MountPoints:   mountPoints,
			Essential:     false,
			IsInternal:    true,
			DesiredStatus: ContainerRunning,
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
	for _, mountPoint := range cont.MountPoints {
		hostPath, ok := vols[mountPoint.ContainerPath]
		if !ok {
			// /path/ -> /path
			hostPath, ok = vols[strings.TrimRight(mountPoint.ContainerPath, "/")]
		}
		if ok {
			if hostVolume, exists := task.HostVolumeByName(mountPoint.SourceVolume); exists {
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
	dockerVolumes, err := task.dockerConfigVolumes(container)
	if err != nil {
		return nil, err
	}

	dockerEnv := make([]string, 0, len(container.Environment))
	for envKey, envVal := range container.Environment {
		dockerEnv = append(dockerEnv, envKey+"="+envVal)
	}

	// Convert MB to B
	dockerMem := int64(container.Memory * 1024 * 1024)
	if dockerMem != 0 && dockerMem < DOCKER_MINIMUM_MEMORY {
		dockerMem = DOCKER_MINIMUM_MEMORY
	}

	entryPoint := []string{}
	if container.EntryPoint != nil {
		entryPoint = *container.EntryPoint
	}

	config := &docker.Config{
		Image:        container.Image,
		Cmd:          container.Command,
		Entrypoint:   entryPoint,
		ExposedPorts: task.dockerExposedPorts(container),
		Volumes:      dockerVolumes,
		Env:          dockerEnv,
		Memory:       dockerMem,
		CPUShares:    int64(container.Cpu),
	}
	return config, nil
}

func (task *Task) dockerExposedPorts(container *Container) map[docker.Port]struct{} {
	dockerExposedPorts := make(map[docker.Port]struct{})

	for _, portBinding := range container.Ports {
		dockerPort := docker.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/tcp")
		dockerExposedPorts[dockerPort] = struct{}{}
	}
	return dockerExposedPorts
}

func (task *Task) dockerConfigVolumes(container *Container) (map[string]struct{}, error) {
	volumeMap := make(map[string]struct{})
	for _, m := range container.MountPoints {
		vol, exists := task.HostVolumeByName(m.SourceVolume)
		if !exists {
			return nil, errors.New("Container references non-existent volume")
		}
		// you can handle most volume mount types in the HostConfig at run-time;
		// empty mounts are created by docker at create-time (Config) so set
		// them here.
		if container.Name == emptyHostVolumeName && container.IsInternal {
			_, ok := vol.(*EmptyHostVolume)
			if !ok {
				return nil, errors.New("invalid state; internal emptyvolume container with non empty volume")
			}

			volumeMap[m.ContainerPath] = struct{}{}
		}
	}
	return volumeMap, nil
}

func (task *Task) DockerHostConfig(container *Container, dockerContainerMap map[string]*DockerContainer) (*docker.HostConfig, error) {
	return task.Overridden().dockerHostConfig(container.Overridden(), dockerContainerMap)
}

func (task *Task) dockerHostConfig(container *Container, dockerContainerMap map[string]*DockerContainer) (*docker.HostConfig, error) {
	dockerLinkArr, err := task.dockerLinks(container, dockerContainerMap)
	if err != nil {
		return nil, err
	}

	dockerPortMap := task.dockerPortMap(container)

	volumesFrom, err := task.dockerVolumesFrom(container, dockerContainerMap)
	if err != nil {
		return nil, err
	}

	binds, err := task.dockerHostBinds(container)
	if err != nil {
		return nil, err
	}

	hostConfig := &docker.HostConfig{
		Links:        dockerLinkArr,
		Binds:        binds,
		PortBindings: dockerPortMap,
		VolumesFrom:  volumesFrom,
	}
	return hostConfig, nil
}

func (task *Task) dockerLinks(container *Container, dockerContainerMap map[string]*DockerContainer) ([]string, error) {
	dockerLinkArr := make([]string, len(container.Links))
	for i, link := range container.Links {
		linkParts := strings.Split(link, ":")
		if len(linkParts) > 2 {
			return []string{}, errors.New("Invalid link format")
		}
		linkName := linkParts[0]
		var linkAlias string

		if len(linkParts) == 2 {
			linkAlias = linkParts[1]
		} else {
			log.Warn("Warning, link with no linkalias", "linkName", linkName, "task", task, "container", container)
			linkAlias = linkName
		}

		targetContainer, ok := dockerContainerMap[linkName]
		if !ok {
			return []string{}, errors.New("Link target not available: " + linkName)
		}
		dockerLinkArr[i] = targetContainer.DockerName + ":" + linkAlias
	}
	return dockerLinkArr, nil
}

func (task *Task) dockerPortMap(container *Container) map[docker.Port][]docker.PortBinding {
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
	return dockerPortMap
}

func (task *Task) dockerVolumesFrom(container *Container, dockerContainerMap map[string]*DockerContainer) ([]string, error) {
	volumesFrom := make([]string, len(container.VolumesFrom))
	for i, volume := range container.VolumesFrom {
		targetContainer, ok := dockerContainerMap[volume.SourceContainer]
		if !ok {
			return []string{}, errors.New("Volume target not available: " + volume.SourceContainer)
		}
		if volume.ReadOnly {
			volumesFrom[i] = targetContainer.DockerName + ":ro"
		} else {
			volumesFrom[i] = targetContainer.DockerName
		}
	}
	return volumesFrom, nil
}

func (task *Task) dockerHostBinds(container *Container) ([]string, error) {
	if container.Name == emptyHostVolumeName {
		// emptyHostVolumes are handled as a special case in config, not
		// hostConfig
		return []string{}, nil
	}

	binds := make([]string, len(container.MountPoints))
	for i, mountPoint := range container.MountPoints {
		hv, ok := task.HostVolumeByName(mountPoint.SourceVolume)
		if !ok {
			return []string{}, errors.New("Invalid volume referenced: " + mountPoint.SourceVolume)
		}

		if hv.SourcePath() == "" || mountPoint.ContainerPath == "" {
			return []string{}, errors.New("Unable to resolve volume mounts; invalid path")
		}

		bind := hv.SourcePath() + ":" + mountPoint.ContainerPath
		if mountPoint.ReadOnly {
			bind += ":ro"
		}
		binds[i] = bind
	}

	return binds, nil
}
