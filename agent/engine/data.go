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

package engine

import (
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// LoadState populates the task engine state with data in db.
func (engine *DockerTaskEngine) LoadState() error {
	if err := engine.loadTasks(); err != nil {
		return err
	}

	if err := engine.loadContainers(); err != nil {
		return err
	}

	if err := engine.loadImageStates(); err != nil {
		return err
	}

	return engine.loadENIAttachments()
}

func (engine *DockerTaskEngine) loadTasks() error {
	tasks, err := engine.dataClient.GetTasks()
	if err != nil {
		return err
	}

	for _, task := range tasks {
		engine.state.AddTask(task)

		// Populate ip <-> task mapping if task has a local ip. This mapping is needed for serving v2 task metadata.
		if ip := task.GetLocalIPAddress(); ip != "" {
			engine.state.AddTaskIPAddress(ip, task.Arn)
		}
	}
	return nil
}

func (engine *DockerTaskEngine) loadContainers() error {
	containers, err := engine.dataClient.GetContainers()
	if err != nil {
		return err
	}

	for _, container := range containers {
		task, ok := engine.state.TaskByArn(container.Container.GetTaskARN())
		if !ok {
			// A task is saved to task table before its containers saved to container table. It is not expected
			// that we have a container from container table whose task is not in the task table.
			return errors.Errorf("did not find the task of container %s: %s", container.Container.Name,
				container.Container.GetTaskARN())
		}
		if container.Container.GetKnownStatus() == apicontainerstatus.ContainerPulled {
			engine.state.AddPulledContainer(container, task)
		} else {
			engine.state.AddContainer(container, task)
		}
	}

	// Update containers in task from data in container table. It stores more updated data of container
	// than the task table.
	tasks := engine.state.AllTasks()
	for _, task := range tasks {
		// A task's container map contains containers loaded from container table. It is ok
		// that there isn't any container in the map, because we first save a container to container table
		// when finished a transition on it, and before that the container is just stored as part of the task.
		dockerContainers, _ := engine.state.ContainerMapByArn(task.Arn)
		pulledContainers, _ := engine.state.PulledContainerMapByArn(task.Arn)

		for idx, container := range task.Containers {
			for _, pulledContainer := range pulledContainers {
				if container.Name == pulledContainer.Container.Name {
					task.Containers[idx] = pulledContainer.Container
				}
			}
			for _, dockerContainer := range dockerContainers {
				if container.Name == dockerContainer.Container.Name {
					task.Containers[idx] = dockerContainer.Container
				}
			}
		}
	}
	return nil
}

func (engine *DockerTaskEngine) loadImageStates() error {
	images, err := engine.dataClient.GetImageStates()
	if err != nil {
		return err
	}

	for _, image := range images {
		engine.state.AddImageState(image)
	}
	return nil
}

func (engine *DockerTaskEngine) loadENIAttachments() error {
	eniAttachments, err := engine.dataClient.GetENIAttachments()
	if err != nil {
		return err
	}

	for _, eniAttachment := range eniAttachments {
		engine.state.AddENIAttachment(eniAttachment)
	}
	return nil
}

// SaveState saves all the data in task engine state to db.
func (engine *DockerTaskEngine) SaveState() error {
	state := engine.state
	ensureCompatibleWithBoltDB(state)

	for _, task := range state.AllTasks() {
		if err := engine.dataClient.SaveTask(task); err != nil {
			return err
		}
	}

	for _, containerID := range state.GetAllContainerIDs() {
		container, ok := state.ContainerByID(containerID)
		if !ok {
			return errors.Errorf("container not found: %s", containerID)
		}
		if err := engine.dataClient.SaveDockerContainer(container); err != nil {
			return err
		}
	}

	for _, imageState := range state.AllImageStates() {
		if err := engine.dataClient.SaveImageState(imageState); err != nil {
			return err
		}
	}

	for _, eniAttachment := range state.AllENIAttachments() {
		if err := engine.dataClient.SaveENIAttachment(eniAttachment); err != nil {
			return err
		}
	}
	return nil
}

// ensureCompatibleWithBoltDB ensures that the data in the state is compatible with what we need
// in order to save into boltdb. Specifically, we need: (1) container to have its taskARN field populated;
// (2) if task has a local ip address, it's stored in the localIPAddressUnsafe field.
// Task launched with new agent will have those fields populated already, but task loaded
// from state file generated by old agent will not, which is why this method is needed.
func ensureCompatibleWithBoltDB(state dockerstate.TaskEngineState) {
	tasks := state.AllTasks()
	for _, t := range tasks {
		for _, c := range t.Containers {
			c.SetTaskARN(t.Arn)
		}

		addr, ok := state.GetIPAddressByTaskARN(t.Arn)
		if ok {
			t.SetLocalIPAddress(addr)
		}
	}
}

func (engine *DockerTaskEngine) saveTaskData(task *apitask.Task) {
	err := engine.dataClient.SaveTask(task)
	if err != nil {
		seelog.Errorf("Failed to save data for task %s: %v", task.Arn, err)
	}
}

func (engine *DockerTaskEngine) saveContainerData(container *apicontainer.Container) {
	err := engine.dataClient.SaveContainer(container)
	if err != nil {
		seelog.Errorf("Failed to save data for container %s: %v", container.Name, err)
	}
}

func (engine *DockerTaskEngine) saveDockerContainerData(container *apicontainer.DockerContainer) {
	err := engine.dataClient.SaveDockerContainer(container)
	if err != nil {
		seelog.Errorf("Failed to save data for docker container %s: %v", container.Container.Name, err)
	}
}

func (engine *DockerTaskEngine) removeTaskData(task *apitask.Task) {
	id, err := utils.GetTaskID(task.Arn)
	if err != nil {
		seelog.Errorf("Failed to get task id from task ARN %s: %v", task.Arn, err)
		return
	}
	err = engine.dataClient.DeleteTask(id)
	if err != nil {
		seelog.Errorf("Failed to remove data for task %s: %v", task.Arn, err)
	}

	for _, c := range task.Containers {
		id, err := data.GetContainerID(c)
		if err != nil {
			seelog.Errorf("Failed to get container id from container %s: %v", c.Name, err)
			continue
		}
		err = engine.dataClient.DeleteContainer(id)
		if err != nil {
			seelog.Errorf("Failed to remove data for container %s: %v", c.Name, err)
		}
	}
}

func (engine *DockerTaskEngine) removeENIAttachmentData(mac string) {
	attachmentToRemove, ok := engine.state.ENIByMac(mac)
	if !ok {
		seelog.Warnf("Unable to retrieve ENI Attachment for mac address %s: ", mac)
		return
	}
	attachmentId, err := utils.GetENIAttachmentId(attachmentToRemove.AttachmentARN)
	if err != nil {
		seelog.Errorf("Failed to get attachment id for %s: %v", attachmentToRemove.AttachmentARN, err)
	} else {
		err = engine.dataClient.DeleteENIAttachment(attachmentId)
		if err != nil {
			seelog.Errorf("Failed to remove data for eni attachment %s: %v", attachmentId, err)
		}
	}
}

func (imageManager *dockerImageManager) saveImageStateData(imageState *image.ImageState) {
	err := imageManager.dataClient.SaveImageState(imageState)
	if err != nil {
		seelog.Errorf("Failed to save data for image state %s:, %v", imageState.GetImageID(), err)
	}
}

func (imageManager *dockerImageManager) removeImageStateData(imageId string) {
	err := imageManager.dataClient.DeleteImageState(imageId)
	if err != nil {
		seelog.Errorf("Failed to remove data for image state %s:, %v", imageId, err)
	}
}
