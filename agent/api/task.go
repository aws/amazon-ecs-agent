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

package api

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/private/protocol/json/jsonutil"
	"github.com/cihub/seelog"
	"github.com/fsouza/go-dockerclient"
)

const (
	emptyHostVolumeName = "~internal~ecs-emptyvolume-source"

	// awsSDKCredentialsRelativeURIPathEnvironmentVariableName defines the name of the environment
	// variable containers' config, which will be used by the AWS SDK to fetch
	// credentials.
	awsSDKCredentialsRelativeURIPathEnvironmentVariableName = "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"
)

// PostUnmarshalTask is run after a task has been unmarshalled, but before it has been
// run. It is possible it will be subsequently called after that and should be
// able to handle such an occurrence appropriately (e.g. behave idempotently).
func (task *Task) PostUnmarshalTask(credentialsManager credentials.Manager) {
	// TODO, add rudimentary plugin support and call any plugins that want to
	// hook into this
	task.adjustForPlatform()
	task.initializeEmptyVolumes()
	task.initializeCredentialsEndpoint(credentialsManager)
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
			// BUG(samuelkarp) On Windows, volumes with names that differ only by case will collide
			containerPath := getCanonicalPath(emptyvolume.ContainerPathPrefix + volume)
			mountPoints[i] = MountPoint{SourceVolume: volume, ContainerPath: containerPath}
		}
		sourceContainer := &Container{
			Name:          emptyHostVolumeName,
			Image:         emptyvolume.Image + ":" + emptyvolume.Tag,
			Command:       []string{emptyvolume.Command}, // Command required, but this only gets created so N/A
			MountPoints:   mountPoints,
			Essential:     false,
			IsInternal:    true,
			DesiredStatus: ContainerRunning,
		}
		task.Containers = append(task.Containers, sourceContainer)
	}

}

// initializeCredentialsEndpoint sets the credentials endpoint for all containers in a task if needed.
func (task *Task) initializeCredentialsEndpoint(credentialsManager credentials.Manager) {
	id := task.GetCredentialsId()
	if id == "" {
		// No credentials set for the task. Do not inject the endpoint environment variable.
		return
	}
	taskCredentials, ok := credentialsManager.GetTaskCredentials(id)
	if !ok {
		// Task has credentials id set, but credentials manager is unaware of
		// the id. This should never happen as the payload handler sets
		// credentialsId for the task after adding credentials to the
		// credentials manager
		seelog.Errorf("Unable to get credentials for task: %s", task.Arn)
		return
	}

	credentialsEndpointRelativeURI := taskCredentials.IAMRoleCredentials.GenerateCredentialsEndpointRelativeURI()
	for _, container := range task.Containers {
		// container.Environment map would not be initialized if there are
		// no environment variables to be set or overridden in the container
		// config. Check if that's the case and initilialize if needed
		if container.Environment == nil {
			container.Environment = make(map[string]string)
		}
		container.Environment[awsSDKCredentialsRelativeURIPathEnvironmentVariableName] = credentialsEndpointRelativeURI
	}

}

// ContainerByName returns the *Container for the given name
func (task *Task) ContainerByName(name string) (*Container, bool) {
	for _, container := range task.Containers {
		if container.Name == name {
			return container, true
		}
	}
	return nil, false
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

// UpdateMountPoints updates the mount points of volumes that were created
// without specifying a host path.  This is used as part of the empty host
// volume feature.
func (task *Task) UpdateMountPoints(cont *Container, vols map[string]string) {
	for _, mountPoint := range cont.MountPoints {
		containerPath := getCanonicalPath(mountPoint.ContainerPath)
		hostPath, ok := vols[containerPath]
		if !ok {
			// /path/ -> /path or \path\ -> \path
			hostPath, ok = vols[strings.TrimRight(containerPath, string(filepath.Separator))]
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
		taskDesiredStatus := task.GetDesiredStatus()
		if c.GetDesiredStatus() < taskDesiredStatus.ContainerStatus() {
			c.SetDesiredStatus(taskDesiredStatus.ContainerStatus())
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
	essentialContainerStopped := false
	for _, cont := range task.Containers {
		contKnownStatus := cont.GetKnownStatus()
		if contKnownStatus == ContainerStopped && cont.Essential {
			essentialContainerStopped = true
		}
		if contKnownStatus < earliestStatus {
			earliestStatus = contKnownStatus
		}
	}

	// If the essential container is stopped while other containers may be running
	// don't update the task status until the other containers are stopped.
	if earliestStatus == ContainerRunning && essentialContainerStopped {
		llog.Debug("Essential container is stopped while other containers are running, not update task status")
		return TaskStatusNone
	}
	llog.Debug("Earliest status is " + earliestStatus.String())
	if task.GetKnownStatus() < earliestStatus.TaskStatus() {
		task.UpdateKnownStatusAndTime(earliestStatus.TaskStatus())
		return task.GetKnownStatus()
	}
	return TaskStatusNone
}

// Overridden returns a copy of the task with all container's overridden and
// itself overridden as well
func (task *Task) Overridden() *Task {
	result := *task
	// Task has no overrides currently, just do the containers

	// Shallow copy, take care of the deeper bits too
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
		CPUShares:    task.dockerCpuShares(container.CPU),
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
			dockerPortMap[dockerPort] = append(currentMappings, docker.PortBinding{HostIP: portBindingHostIP, HostPort: strconv.Itoa(int(portBinding.HostPort))})
		} else {
			dockerPortMap[dockerPort] = []docker.PortBinding{{HostIP: portBindingHostIP, HostPort: strconv.Itoa(int(portBinding.HostPort))}}
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

// TaskFromACS translates ecsacs.Task to api.Task by first marshaling the recieved
// ecsacs.Task to json and unmrashaling it as api.Task
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

	if task.GetDesiredStatus() == TaskRunning && envelope.SeqNum != nil {
		task.StartSequenceNumber = *envelope.SeqNum
	} else if task.GetDesiredStatus() == TaskStopped && envelope.SeqNum != nil {
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
		if cont.Essential && (cont.KnownTerminal() || cont.DesiredTerminal()) {
			llog.Debug("Updating task desired status to stopped", "container", cont.Name)
			task.SetDesiredStatus(TaskStopped)
		}
	}
}

// UpdateStatus updates a task's known and desired statuses to be compatible
// with all of its containers
// It will return a bool indicating if there was a change
func (task *Task) UpdateStatus() bool {
	change := task.updateTaskKnownStatus()
	// DesiredStatus can change based on a new known status
	task.UpdateDesiredStatus()
	return change != TaskStatusNone
}

func (task *Task) UpdateDesiredStatus() {
	task.updateTaskDesiredStatus()
	task.updateContainerDesiredStatus()
}

func (task *Task) SetKnownStatus(status TaskStatus) {
	task.setKnownStatus(status)
	task.updateKnownStatusTime()
}

// UpdateKnownStatusAndTime updates the KnownStatus and KnownStatusTime
// of the task
func (task *Task) UpdateKnownStatusAndTime(status TaskStatus) {
	task.setKnownStatus(status)
	task.updateKnownStatusTime()
}

// GetKnownStatus gets the KnownStatus of the task
func (task *Task) GetKnownStatus() TaskStatus {
	task.knownStatusLock.RLock()
	defer task.knownStatusLock.RUnlock()

	return task.KnownStatus
}

// GetKnownStatusTime gets the KnownStatusTime of the task
func (task *Task) GetKnownStatusTime() time.Time {
	task.knownStatusTimeLock.RLock()
	defer task.knownStatusTimeLock.RUnlock()

	return task.KnownStatusTime
}

func (task *Task) setKnownStatus(status TaskStatus) {
	task.knownStatusLock.Lock()
	defer task.knownStatusLock.Unlock()

	task.KnownStatus = status
}

func (task *Task) updateKnownStatusTime() {
	task.knownStatusTimeLock.Lock()
	defer task.knownStatusTimeLock.Unlock()

	task.KnownStatusTime = ttime.Now()
}

func (task *Task) SetCredentialsId(id string) {
	task.credentialsIDLock.Lock()
	defer task.credentialsIDLock.Unlock()

	task.credentialsID = id
}

func (task *Task) GetCredentialsId() string {
	task.credentialsIDLock.RLock()
	defer task.credentialsIDLock.RUnlock()

	return task.credentialsID
}

func (task *Task) GetDesiredStatus() TaskStatus {
	task.desiredStatusLock.RLock()
	defer task.desiredStatusLock.RUnlock()

	return task.DesiredStatus
}

func (task *Task) SetDesiredStatus(status TaskStatus) {
	task.desiredStatusLock.Lock()
	defer task.desiredStatusLock.Unlock()

	task.DesiredStatus = status
}

// GetSentStatus safely returns the SentStatus of the task
func (task *Task) GetSentStatus() TaskStatus {
	task.sentStatusLock.RLock()
	defer task.sentStatusLock.RUnlock()

	return task.SentStatus
}

// SetSentStatus safely sets the SentStatus of the task
func (task *Task) SetSentStatus(status TaskStatus) {
	task.sentStatusLock.Lock()
	defer task.sentStatusLock.Unlock()

	task.SentStatus = status
}
