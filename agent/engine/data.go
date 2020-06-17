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
	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/cihub/seelog"
)

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
