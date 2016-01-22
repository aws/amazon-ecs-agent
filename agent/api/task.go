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
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/private/protocol/json/jsonutil"
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
					empty.HostPath = hostPath
				}
			}
		}
	}
}

// updateContainerDesiredStatus sets all container's desired status's to the
// task's desired status
func (task *Task) updateContainerDesiredStatus() {
	for _, c := range task.Containers {
		if c.DesiredStatus < task.DesiredStatus.ContainerStatus() {
			c.DesiredStatus = task.DesiredStatus.ContainerStatus()
		}
	}
}

// updateTaskKnownState updates the given task's status based on its container's status.
// It updates to the minimum of all containers no matter what
// It returns a TaskStatus indicating what change occured or TaskStatusNone if
// there was no change
func (task *Task) updateTaskKnownStatus() (newStatus TaskStatus) {
	llog := log.New("task", task)
	llog.Debug("Updating task")

	// Set to a large 'impossible' status that can't be the min
	earliestStatus := ContainerZombie
	for _, cont := range task.Containers {
		if cont.KnownStatus < earliestStatus {
			earliestStatus = cont.KnownStatus
		}
	}

	llog.Debug("Earliest status is " + earliestStatus.String())
	if task.KnownStatus < earliestStatus.TaskStatus() {
		task.SetKnownStatus(earliestStatus.TaskStatus())
		return task.KnownStatus
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
func (task *Task) DockerConfig(container *Container) (*docker.Config, *DockerClientConfigError) {
	return task.Overridden().dockerConfig(container.Overridden())
}

func (task *Task) dockerConfig(container *Container) (*docker.Config, *DockerClientConfigError) {
	dockerVolumes, err := task.dockerConfigVolumes(container)
	if err != nil {
		return nil, &DockerClientConfigError{err.Error()}
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

	var entryPoint []string
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
		CPUShares:    task.dockerCpuShares(container.Cpu),
	}

	if container.DockerConfig.Config != nil {
		err := json.Unmarshal([]byte(*container.DockerConfig.Config), &config)
		if err != nil {
			return nil, &DockerClientConfigError{"Unable decode given docker config: " + err.Error()}
		}
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	// Augment labels with some metadata from the agent. Explicitly do this last
	// such that it will always override duplicates in the provided raw config
	// data.
	config.Labels["com.amazonaws.ecs.task-arn"] = task.Arn
	config.Labels["com.amazonaws.ecs.container-name"] = container.Name
	config.Labels["com.amazonaws.ecs.task-definition-family"] = task.Family
	config.Labels["com.amazonaws.ecs.task-definition-version"] = task.Version

	return config, nil
}

// Docker silently converts 0 to 1024 CPU shares, which is probably not what we
// want.  Instead, we convert 0 to 2 to be closer to expected behavior. The
// reason for 2 over 1 is that 1 is an invalid value (Linux's choice, not
// Docker's).
func (task *Task) dockerCpuShares(containerCpu uint) int64 {
	if containerCpu <= 1 {
		log.Debug("Converting CPU shares to allowed minimum of 2", "task", task.Arn, "cpuShares", containerCpu)
		return 2
	}
	return int64(containerCpu)
}

func (task *Task) dockerExposedPorts(container *Container) map[docker.Port]struct{} {
	dockerExposedPorts := make(map[docker.Port]struct{})

	for _, portBinding := range container.Ports {
		dockerPort := docker.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/" + portBinding.Protocol.String())
		dockerExposedPorts[dockerPort] = struct{}{}
	}
	return dockerExposedPorts
}

func (task *Task) dockerConfigVolumes(container *Container) (map[string]struct{}, error) {
	volumeMap := make(map[string]struct{})
	for _, m := range container.MountPoints {
		vol, exists := task.HostVolumeByName(m.SourceVolume)
		if !exists {
			return nil, &badVolumeError{"Container " + container.Name + " in task " + task.Arn + " references invalid volume " + m.SourceVolume}
		}
		// you can handle most volume mount types in the HostConfig at run-time;
		// empty mounts are created by docker at create-time (Config) so set
		// them here.
		if container.Name == emptyHostVolumeName && container.IsInternal {
			_, ok := vol.(*EmptyHostVolume)
			if !ok {
				return nil, &badVolumeError{"Empty volume container in task " + task.Arn + " was the wrong type"}
			}

			volumeMap[m.ContainerPath] = struct{}{}
		}
	}
	return volumeMap, nil
}

func (task *Task) DockerHostConfig(container *Container, dockerContainerMap map[string]*DockerContainer) (*docker.HostConfig, *HostConfigError) {
	return task.Overridden().dockerHostConfig(container.Overridden(), dockerContainerMap)
}

func (task *Task) dockerHostConfig(container *Container, dockerContainerMap map[string]*DockerContainer) (*docker.HostConfig, *HostConfigError) {
	dockerLinkArr, err := task.dockerLinks(container, dockerContainerMap)
	if err != nil {
		return nil, &HostConfigError{err.Error()}
	}

	dockerPortMap := task.dockerPortMap(container)

	volumesFrom, err := task.dockerVolumesFrom(container, dockerContainerMap)
	if err != nil {
		return nil, &HostConfigError{err.Error()}
	}

	binds, err := task.dockerHostBinds(container)
	if err != nil {
		return nil, &HostConfigError{err.Error()}
	}

	hostConfig := &docker.HostConfig{
		Links:        dockerLinkArr,
		Binds:        binds,
		PortBindings: dockerPortMap,
		VolumesFrom:  volumesFrom,
	}

	if container.DockerConfig.HostConfig != nil {
		err := json.Unmarshal([]byte(*container.DockerConfig.HostConfig), hostConfig)
		if err != nil {
			return nil, &HostConfigError{"Unable to decode given host config: " + err.Error()}
		}
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
		dockerPort := docker.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/" + portBinding.Protocol.String())
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
			log.Error("Unable to resolve volume mounts; invalid path: " + container.Name + " " + mountPoint.SourceVolume + "; " + hv.SourcePath() + " -> " + mountPoint.ContainerPath)
			return []string{}, errors.New("Unable to resolve volume mounts; invalid path: " + container.Name + " " + mountPoint.SourceVolume + "; " + hv.SourcePath() + " -> " + mountPoint.ContainerPath)
		}

		bind := hv.SourcePath() + ":" + mountPoint.ContainerPath
		if mountPoint.ReadOnly {
			bind += ":ro"
		}
		binds[i] = bind
	}

	return binds, nil
}

func TaskFromACS(acsTask *ecsacs.Task, envelope *ecsacs.PayloadMessage) (*Task, error) {
	data, err := jsonutil.BuildJSON(acsTask)
	if err != nil {
		return nil, err
	}
	task := &Task{}
	err = json.Unmarshal(data, task)
	if err != nil {
		return nil, err
	}
	if task.DesiredStatus == TaskRunning && envelope.SeqNum != nil {
		task.StartSequenceNumber = *envelope.SeqNum
	} else if task.DesiredStatus == TaskStopped && envelope.SeqNum != nil {
		task.StopSequenceNumber = *envelope.SeqNum
	}

	return task, nil
}

// updateTaskDesiredStatus determines what status the task should properly be at based on its container's statuses
func (task *Task) updateTaskDesiredStatus() {
	llog := log.New("task", task)
	llog.Debug("Updating task")

	// A task's desired status is stopped if any essential container is stopped
	// Otherwise, the task's desired status is unchanged (typically running, but no need to change)
	for _, cont := range task.Containers {
		if cont.Essential && (cont.KnownStatus.Terminal() || cont.DesiredStatus.Terminal()) {
			llog.Debug("Updating task desired status to stopped", "container", cont.Name)
			task.DesiredStatus = TaskStopped
		}
	}
}

// UpdateStatus updates a task's known and desired statuses to be compatible
// with all of its containers
// It will return a bool indicating if there was a change
func (t *Task) UpdateStatus() bool {
	change := t.updateTaskKnownStatus()
	// DesiredStatus can change based on a new known status
	t.UpdateDesiredStatus()
	return change != TaskStatusNone
}

func (t *Task) UpdateDesiredStatus() {
	t.updateTaskDesiredStatus()
	t.updateContainerDesiredStatus()
}

func (t *Task) SetKnownStatus(status TaskStatus) {
	t.KnownStatus = status
	t.KnownStatusTime = ttime.Now()
}
