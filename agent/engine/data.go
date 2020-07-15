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
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
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
	}
	return nil
}

func (engine *DockerTaskEngine) loadContainers() error {
	containers, err := engine.dataClient.GetContainers()
	if err != nil {
		return err
	}

	for _, container := range containers {
		task, ok := engine.state.TaskByArn(container.Container.TaskARN)
		if !ok {
			// A task is saved to task table before its containers saved to container table. It is not expected
			// that we have a container from container table whose task is not in the task table.
			return errors.Errorf("did not find the task of container %s: %s", container.Container.Name,
				container.Container.TaskARN)
		}
		engine.state.AddContainer(container, task)
	}

	// Update containers in task from data in container table. It stores more updated data of container
	// than the task table.
	tasks := engine.state.AllTasks()
	for _, task := range tasks {
		dockerContainers, ok := engine.state.ContainerMapByArn(task.Arn)
		if !ok {
			// A task's container map contains containers loaded from container table. It is ok
			// that there isn't any container in the map, because we first save a container to container table
			// when finished a transition on it, and before that the container is just stored as part of the task.
			continue
		}

		for idx, container := range task.Containers {
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
