// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	"github.com/cihub/seelog"
)

// TaskEngineState keeps track of all mappings between tasks we know about
// and containers docker runs
type TaskEngineState interface {
	// AllTasks returns all of the tasks
	AllTasks() []*apitask.Task
	// AllExternalTasks returns all tasks with IsInternal==false (i.e. customer-initiated tasks).
	// Currently, ServiceConnect AppNet Relay task is the only internal task.
	AllExternalTasks() []*apitask.Task
	// AllENIAttachments returns all of the eni attachments
	AllENIAttachments() []*apieni.ENIAttachment
	// AllImageStates returns all of the image.ImageStates
	AllImageStates() []*image.ImageState
	// GetAllContainerIDs returns all of the Container Ids
	GetAllContainerIDs() []string
	// ContainerByID returns an apicontainer.DockerContainer for a given container ID
	ContainerByID(id string) (*apicontainer.DockerContainer, bool)
	// ContainerMapByArn returns a map of containers belonging to a particular task ARN
	ContainerMapByArn(arn string) (map[string]*apicontainer.DockerContainer, bool)
	// PulledContainerMapByArn returns a map of pulled containers belonging to a particular task ARN
	PulledContainerMapByArn(arn string) (map[string]*apicontainer.DockerContainer, bool)
	// TaskByShortID retrieves the task of a given docker short container id
	TaskByShortID(cid string) ([]*apitask.Task, bool)
	// TaskByID returns an apitask.Task for a given container ID
	TaskByID(cid string) (*apitask.Task, bool)
	// TaskByArn returns a task for a given ARN
	TaskByArn(arn string) (*apitask.Task, bool)
	// AddTask adds a task to the state to be stored
	AddTask(task *apitask.Task)
	// AddPulledContainer adds a pulled container to the state to be stored for a given task
	AddPulledContainer(container *apicontainer.DockerContainer, task *apitask.Task)
	// AddContainer adds a container to the state to be stored for a given task
	AddContainer(container *apicontainer.DockerContainer, task *apitask.Task)
	// AddImageState adds an image.ImageState to be stored
	AddImageState(imageState *image.ImageState)
	// AddENIAttachment adds an eni attachment from acs to be stored
	AddENIAttachment(eni *apieni.ENIAttachment)
	// RemoveENIAttachment removes an eni attachment to stop tracking
	RemoveENIAttachment(mac string)
	// ENIByMac returns the specific ENIAttachment of the given mac address
	ENIByMac(mac string) (*apieni.ENIAttachment, bool)
	// RemoveTask removes a task from the state
	RemoveTask(task *apitask.Task)
	// Reset resets all the fileds in the state
	Reset()
	// RemoveImageState removes an image.ImageState
	RemoveImageState(imageState *image.ImageState)
	// AddTaskIPAddress adds ip adddress for a task arn into the state
	AddTaskIPAddress(addr string, taskARN string)
	// GetTaskByIPAddress gets the task arn for an IP address
	GetTaskByIPAddress(addr string) (string, bool)
	// GetIPAddressByTaskARN gets the local ip address of a task.
	GetIPAddressByTaskARN(taskARN string) (string, bool)
	// DockerIDByV3EndpointID returns a docker ID for a given v3 endpoint ID
	DockerIDByV3EndpointID(v3EndpointID string) (string, bool)
	// TaskARNByV3EndpointID returns a taskARN for a given v3 endpoint ID
	TaskARNByV3EndpointID(v3EndpointID string) (string, bool)

	json.Marshaler
	json.Unmarshaler
}

// DockerTaskEngineState keeps track of all mappings between tasks we know about
// and containers docker runs
// It contains a mutex that can be used to ensure out-of-date state cannot be
// accessed before an update comes and to ensure multiple goroutines can safely
// work with it.
//
// The methods on it will acquire the read lock, but not all acquire the write
// lock (sometimes it is up to the caller). This is because the write lock for
// containers should encapsulate the creation of the resource as well as adding,
// and creating the resource (docker container) is outside the scope of this
// package. This isn't ideal usage and I'm open to this being reworked/improved.
//
// Some information is duplicated in the interest of having efficient lookups
type DockerTaskEngineState struct {
	lock sync.RWMutex

	tasks                  map[string]*apitask.Task                            // taskarn -> apitask.Task
	idToTask               map[string]string                                   // DockerId -> taskarn
	taskToID               map[string]map[string]*apicontainer.DockerContainer // taskarn -> (containername -> c.DockerContainer)
	taskToPulledContainer  map[string]map[string]*apicontainer.DockerContainer // taskarn -> (containername -> c.DockerContainer)
	idToContainer          map[string]*apicontainer.DockerContainer            // DockerId -> c.DockerContainer
	eniAttachments         map[string]*apieni.ENIAttachment                    // ENIMac -> apieni.ENIAttachment
	imageStates            map[string]*image.ImageState
	ipToTask               map[string]string // ip address -> task arn
	v3EndpointIDToTask     map[string]string // container's v3 endpoint id -> taskarn
	v3EndpointIDToDockerID map[string]string // container's v3 endpoint id -> DockerId
}

// NewTaskEngineState returns a new TaskEngineState
func NewTaskEngineState() TaskEngineState {
	return newDockerTaskEngineState()
}

func newDockerTaskEngineState() *DockerTaskEngineState {
	state := &DockerTaskEngineState{}
	state.initializeDockerTaskEngineState()
	return state
}

func (state *DockerTaskEngineState) initializeDockerTaskEngineState() {
	state.lock.Lock()
	defer state.lock.Unlock()

	state.tasks = make(map[string]*apitask.Task)
	state.idToTask = make(map[string]string)
	state.taskToID = make(map[string]map[string]*apicontainer.DockerContainer)
	state.taskToPulledContainer = make(map[string]map[string]*apicontainer.DockerContainer)
	state.idToContainer = make(map[string]*apicontainer.DockerContainer)
	state.imageStates = make(map[string]*image.ImageState)
	state.eniAttachments = make(map[string]*apieni.ENIAttachment)
	state.ipToTask = make(map[string]string)
	state.v3EndpointIDToTask = make(map[string]string)
	state.v3EndpointIDToDockerID = make(map[string]string)
}

// Reset resets all the states
func (state *DockerTaskEngineState) Reset() {
	state.initializeDockerTaskEngineState()
}

// AllTasks returns all of the tasks
func (state *DockerTaskEngineState) AllTasks() []*apitask.Task {
	state.lock.RLock()
	defer state.lock.RUnlock()

	return state.allTasksUnsafe()
}

func (state *DockerTaskEngineState) allTasksUnsafe() []*apitask.Task {
	return state.getFilteredTasksUnsafe(false)
}

// AllExternalTasks returns all tasks with IsInternal==false (i.e. all customer-initiated tasks)
func (state *DockerTaskEngineState) AllExternalTasks() []*apitask.Task {
	state.lock.RLock()
	defer state.lock.RUnlock()

	return state.allExternalTasksUnsafe()
}

func (state *DockerTaskEngineState) allExternalTasksUnsafe() []*apitask.Task {
	return state.getFilteredTasksUnsafe(true)
}

func (state *DockerTaskEngineState) getFilteredTasksUnsafe(excludeInternal bool) []*apitask.Task {
	ret := make([]*apitask.Task, len(state.tasks))
	ndx := 0
	for _, task := range state.tasks {
		if excludeInternal && task.IsInternal {
			continue
		}
		ret[ndx] = task
		ndx++
	}
	return ret[:ndx]
}

// AllImageStates returns all of the image.ImageStates
func (state *DockerTaskEngineState) AllImageStates() []*image.ImageState {
	state.lock.RLock()
	defer state.lock.RUnlock()

	return state.allImageStatesUnsafe()
}

func (state *DockerTaskEngineState) allImageStatesUnsafe() []*image.ImageState {
	var allImageStates []*image.ImageState
	for _, imageState := range state.imageStates {
		allImageStates = append(allImageStates, imageState)
	}
	return allImageStates
}

// AllENIAttachments returns all the enis managed by ecs on the instance
func (state *DockerTaskEngineState) AllENIAttachments() []*apieni.ENIAttachment {
	state.lock.RLock()
	defer state.lock.RUnlock()

	return state.allENIAttachmentsUnsafe()
}

func (state *DockerTaskEngineState) allENIAttachmentsUnsafe() []*apieni.ENIAttachment {
	var allENIAttachments []*apieni.ENIAttachment
	for _, v := range state.eniAttachments {
		allENIAttachments = append(allENIAttachments, v)
	}

	return allENIAttachments
}

// ENIByMac returns the eni object that match the give mac address
func (state *DockerTaskEngineState) ENIByMac(mac string) (*apieni.ENIAttachment, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	eni, ok := state.eniAttachments[mac]
	return eni, ok
}

// AddENIAttachment adds the eni into the state
func (state *DockerTaskEngineState) AddENIAttachment(eniAttachment *apieni.ENIAttachment) {
	if eniAttachment == nil {
		seelog.Debug("Cannot add empty eni attachment information")
		return
	}

	state.lock.Lock()
	defer state.lock.Unlock()

	if _, ok := state.eniAttachments[eniAttachment.MACAddress]; !ok {
		state.eniAttachments[eniAttachment.MACAddress] = eniAttachment
	} else {
		seelog.Debugf("Duplicate eni attachment information: %v", eniAttachment)
	}

}

// RemoveENIAttachment removes the eni from state and stop managing
func (state *DockerTaskEngineState) RemoveENIAttachment(mac string) {
	if mac == "" {
		seelog.Debug("Cannot remove empty eni attachment information")
		return
	}
	state.lock.Lock()
	defer state.lock.Unlock()
	if _, ok := state.eniAttachments[mac]; ok {
		delete(state.eniAttachments, mac)
	} else {
		seelog.Debugf("Delete non-existed eni attachment: %v", mac)
	}
}

// GetAllContainerIDs returns all of the Container Ids
func (state *DockerTaskEngineState) GetAllContainerIDs() []string {
	state.lock.RLock()
	defer state.lock.RUnlock()

	var ids []string
	for id := range state.idToTask {
		ids = append(ids, id)
	}

	return ids
}

// ContainerByID returns an apicontainer.DockerContainer for a given container ID
func (state *DockerTaskEngineState) ContainerByID(id string) (*apicontainer.DockerContainer, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	c, ok := state.idToContainer[id]
	return c, ok
}

// ContainerMapByArn returns a map of containers belonging to a particular task ARN
func (state *DockerTaskEngineState) ContainerMapByArn(arn string) (map[string]*apicontainer.DockerContainer, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	ret, ok := state.taskToID[arn]

	if !ok {
		return ret, ok
	}
	// Copy the map to avoid data race
	mc := make(map[string]*apicontainer.DockerContainer)
	for k, v := range ret {
		mc[k] = v
	}
	return mc, ok
}

// PulledContainerMapByArn returns a map of pulled containers belonging to a particular task ARN
func (state *DockerTaskEngineState) PulledContainerMapByArn(arn string) (map[string]*apicontainer.DockerContainer, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	ret, ok := state.taskToPulledContainer[arn]
	if !ok {
		return ret, ok
	}
	// Copy the map to avoid data race
	mc := make(map[string]*apicontainer.DockerContainer)
	for k, v := range ret {
		mc[k] = v
	}
	return mc, ok
}

// TaskByShortID retrieves the task of a given docker short container id
func (state *DockerTaskEngineState) TaskByShortID(cid string) ([]*apitask.Task, bool) {
	containerIDs := state.GetAllContainerIDs()
	var tasks []*apitask.Task
	for _, id := range containerIDs {
		if strings.HasPrefix(id, cid) {
			if task, ok := state.TaskByID(id); ok {
				tasks = append(tasks, task)
			}
		}
	}
	return tasks, len(tasks) > 0
}

// TaskByID retrieves the task of a given docker container id
func (state *DockerTaskEngineState) TaskByID(cid string) (*apitask.Task, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	arn, found := state.idToTask[cid]
	if !found {
		return nil, false
	}
	return state.taskByArn(arn)
}

// TaskByArn returns a task for a given ARN
func (state *DockerTaskEngineState) TaskByArn(arn string) (*apitask.Task, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	return state.taskByArn(arn)
}

func (state *DockerTaskEngineState) taskByArn(arn string) (*apitask.Task, bool) {
	t, ok := state.tasks[arn]
	return t, ok
}

// AddTask adds a new task to the state
func (state *DockerTaskEngineState) AddTask(task *apitask.Task) {
	state.lock.Lock()
	defer state.lock.Unlock()

	state.tasks[task.Arn] = task
}

// AddPulledContainer adds a pulled container to the state
func (state *DockerTaskEngineState) AddPulledContainer(container *apicontainer.DockerContainer, task *apitask.Task) {
	state.lock.Lock()
	defer state.lock.Unlock()
	if task == nil || container == nil {
		seelog.Critical("AddPulledContainer called with nil task/container")
		return
	}

	_, exists := state.tasks[task.Arn]
	if !exists {
		seelog.Debugf("AddPulledContainer called with unknown task; adding", "arn", task.Arn)
		state.tasks[task.Arn] = task
	}

	existingMap, exists := state.taskToPulledContainer[task.Arn]
	if !exists {
		existingMap = make(map[string]*apicontainer.DockerContainer, len(task.Containers))
		state.taskToPulledContainer[task.Arn] = existingMap
	}
	existingMap[container.Container.Name] = container
}

// AddContainer adds a container to the state.
// If the container has been added with only a name and no docker-id, this
// updates the state to include the docker id
func (state *DockerTaskEngineState) AddContainer(container *apicontainer.DockerContainer, task *apitask.Task) {
	state.lock.Lock()
	defer state.lock.Unlock()
	if task == nil || container == nil {
		seelog.Critical("AddContainer called with nil task/container")
		return
	}

	_, exists := state.tasks[task.Arn]
	if !exists {
		seelog.Debugf("AddContainer called with unknown task; adding", "arn", task.Arn)
		state.tasks[task.Arn] = task
	}

	_, pulledExist := state.taskToPulledContainer[task.Arn]
	if pulledExist {
		seelog.Debugf("Delete a pulled container named %s from the pulled container map associated with the"+
			" task ARN %s since AddContainer is called", container.Container.Name, task.Arn)
		delete(state.taskToPulledContainer[task.Arn], container.Container.Name)
		if len(state.taskToPulledContainer[task.Arn]) == 0 {
			delete(state.taskToPulledContainer, task.Arn)
		}
	}

	state.storeIDToContainerTaskUnsafe(container, task)

	dockerID := container.DockerID
	v3EndpointID := container.Container.V3EndpointID
	// stores the v3EndpointID mappings only if container's dockerID exists and container's v3EndpointID has been generated
	if dockerID != "" && v3EndpointID != "" {
		state.storeV3EndpointIDToTaskUnsafe(v3EndpointID, task.Arn)
		state.storeV3EndpointIDToDockerIDUnsafe(v3EndpointID, dockerID)
	}

	existingMap, exists := state.taskToID[task.Arn]
	if !exists {
		existingMap = make(map[string]*apicontainer.DockerContainer, len(task.Containers))
		state.taskToID[task.Arn] = existingMap
	}
	existingMap[container.Container.Name] = container
}

// AddImageState adds an image.ImageState to be stored
func (state *DockerTaskEngineState) AddImageState(imageState *image.ImageState) {
	if imageState == nil {
		seelog.Debug("Cannot add empty image state")
		return
	}
	if imageState.Image.ImageID == "" {
		seelog.Debug("Cannot add image state with empty image id")
		return
	}
	state.lock.Lock()
	defer state.lock.Unlock()

	state.imageStates[imageState.Image.ImageID] = imageState
}

// RemoveTask removes a task from this state. It removes all containers and
// other associated metadata. It does acquire the write lock.
func (state *DockerTaskEngineState) RemoveTask(task *apitask.Task) {
	state.lock.Lock()
	defer state.lock.Unlock()

	task, ok := state.tasks[task.Arn]
	if !ok {
		seelog.Warnf("Failed to locate task %s for removal from state", task.Arn)
		return
	}
	delete(state.tasks, task.Arn)
	if ip, ok := state.taskToIPUnsafe(task.Arn); ok {
		delete(state.ipToTask, ip)
	}

	containerMap, ok := state.taskToID[task.Arn]
	if !ok {
		seelog.Warnf("Failed to locate containerMap for task %s for removal from state", task.Arn)
		return
	}
	delete(state.taskToID, task.Arn)

	for _, dockerContainer := range containerMap {
		state.removeIDToContainerTaskUnsafe(dockerContainer)
		// remove v3 endpoint mappings
		state.removeV3EndpointIDToTaskContainerUnsafe(dockerContainer.Container.V3EndpointID)
	}
	delete(state.taskToPulledContainer, task.Arn)
}

// taskToIPUnsafe gets the ip address for a given task arn
func (state *DockerTaskEngineState) taskToIPUnsafe(arn string) (string, bool) {
	for ip, taskARN := range state.ipToTask {
		if arn == taskARN {
			return ip, true
		}
	}

	return "", false
}

// storeIDToContainerTaskUnsafe stores the container in the idToContainer and idToTask maps.  The key to the maps is
// either the Docker-generated ID or the agent-generated name (if the ID is not available).  If the container is updated
// with an ID, a subsequent call to this function will update the map to use the ID as the key.
func (state *DockerTaskEngineState) storeIDToContainerTaskUnsafe(container *apicontainer.DockerContainer, task *apitask.Task) {
	if container.DockerID != "" {
		// Update the container id to the state
		state.idToContainer[container.DockerID] = container
		state.idToTask[container.DockerID] = task.Arn

		// Remove the previously added name mapping
		delete(state.idToContainer, container.DockerName)
		delete(state.idToTask, container.DockerName)
	} else if container.DockerName != "" {
		// Update the container name mapping to the state when the ID isn't available
		state.idToContainer[container.DockerName] = container
		state.idToTask[container.DockerName] = task.Arn
	}
}

// removeIDToContainerTaskUnsafe removes the container from the idToContainer and idToTask maps.  They key to the maps
// is either the Docker-generated ID or the agent-generated name (if the ID is not available).  This function assumes
// that the ID takes precedence and will delete by the ID when the ID is available.
func (state *DockerTaskEngineState) removeIDToContainerTaskUnsafe(container *apicontainer.DockerContainer) {
	// The key to these maps is either the Docker ID or agent-generated name.  We use the agent-generated name
	// before a Docker ID is available.
	key := container.DockerID
	if key == "" {
		key = container.DockerName
	}
	delete(state.idToTask, key)
	delete(state.idToContainer, key)
}

// removeV3EndpointIDToTaskContainerUnsafe removes the container from v3EndpointIDToTask and v3EndpointIDToDockerID maps
func (state *DockerTaskEngineState) removeV3EndpointIDToTaskContainerUnsafe(v3EndpointID string) {
	if v3EndpointID != "" {
		delete(state.v3EndpointIDToTask, v3EndpointID)
		delete(state.v3EndpointIDToDockerID, v3EndpointID)
	}
}

// RemoveImageState removes an image.ImageState
func (state *DockerTaskEngineState) RemoveImageState(imageState *image.ImageState) {
	if imageState == nil {
		seelog.Debug("Cannot remove empty image state")
		return
	}
	state.lock.Lock()
	defer state.lock.Unlock()

	imageState, ok := state.imageStates[imageState.Image.ImageID]
	if !ok {
		seelog.Debug("Image State is not found. Cannot be removed")
		return
	}
	delete(state.imageStates, imageState.Image.ImageID)
}

// AddTaskIPAddress adds ip adddress for a task arn into the state
func (state *DockerTaskEngineState) AddTaskIPAddress(addr string, taskARN string) {
	state.lock.Lock()
	defer state.lock.Unlock()

	state.ipToTask[addr] = taskARN
}

// GetTaskByIPAddress gets the task arn for an IP address
func (state *DockerTaskEngineState) GetTaskByIPAddress(addr string) (string, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	taskARN, ok := state.ipToTask[addr]
	return taskARN, ok
}

// GetIPAddressByTaskARN gets the local ip address of a task.
func (state *DockerTaskEngineState) GetIPAddressByTaskARN(taskARN string) (string, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	for addr, arn := range state.ipToTask {
		if arn == taskARN {
			return addr, true
		}
	}
	return "", false
}

// storeV3EndpointIDToTaskUnsafe adds v3EndpointID -> taskARN mapping to state
func (state *DockerTaskEngineState) storeV3EndpointIDToTaskUnsafe(v3EndpointID, taskARN string) {
	state.v3EndpointIDToTask[v3EndpointID] = taskARN
}

// storeV3EndpointIDToContainerNameUnsafe adds v3EndpointID -> dockerID mapping to state
func (state *DockerTaskEngineState) storeV3EndpointIDToDockerIDUnsafe(v3EndpointID, dockerID string) {
	state.v3EndpointIDToDockerID[v3EndpointID] = dockerID
}

// DockerIDByV3EndpointID returns a docker ID for a given v3 endpoint ID
func (state *DockerTaskEngineState) DockerIDByV3EndpointID(v3EndpointID string) (string, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	dockerID, ok := state.v3EndpointIDToDockerID[v3EndpointID]
	return dockerID, ok
}

// TaskARNByV3EndpointID returns a taskARN for a given v3 endpoint ID
func (state *DockerTaskEngineState) TaskARNByV3EndpointID(v3EndpointID string) (string, bool) {
	state.lock.RLock()
	defer state.lock.RUnlock()

	taskArn, ok := state.v3EndpointIDToTask[v3EndpointID]
	return taskArn, ok
}
