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

package v1

import (
	"fmt"

	handlerutils "github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	agentversion "github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection"
)

// AgentStateImpl is an implementation of the AgentState interface in the introspection package.
// This struct supplies the introspection server with the necessary data to construct its responses.
type AgentStateImpl struct {
	ContainerInstanceArn *string
	ClusterName          string
	TaskEngine           handlerutils.DockerStateResolver
}

var licenseProvider = utils.NewLicenseProvider()

// GetLicenseText returns the agent's license text as a string with an error if the license cannot be retrieved.
func (as *AgentStateImpl) GetLicenseText() (string, error) {
	return licenseProvider.GetText()
}

// GetAgentMetadata returns agent metadata in v1 format.
func (as *AgentStateImpl) GetAgentMetadata() (*introspection.AgentMetadataResponse, error) {
	return &introspection.AgentMetadataResponse{
		Cluster:              as.ClusterName,
		ContainerInstanceArn: as.ContainerInstanceArn,
		Version:              agentversion.String(),
	}, nil
}

// GetTasksMetadata returns task metadata in v1 format for all tasks on the host with an error if the metadata
// cannot be retrieved.
func (as *AgentStateImpl) GetTasksMetadata() (*introspection.TasksResponse, error) {
	agentState := as.TaskEngine.State()
	allTasks := agentState.AllExternalTasks()
	taskResponses := make([]*introspection.TaskResponse, len(allTasks))
	for ndx, task := range allTasks {
		containerMap, ok := agentState.ContainerMapByArn(task.Arn)
		if !ok {
			return nil, introspection.NewErrorNotFound(fmt.Sprintf("container map for task %s not found", task.Arn))
		}
		taskResponses[ndx] = NewTaskResponse(task, containerMap)
	}
	return &introspection.TasksResponse{Tasks: taskResponses}, nil
}

// GetTaskMetadataByArn returns task metadata in v1 format for the task with a matching task Arn, with an error
// if the metadata cannot be retrieved.
func (as *AgentStateImpl) GetTaskMetadataByArn(taskArn string) (*introspection.TaskResponse, error) {
	agentState := as.TaskEngine.State()
	task, found := agentState.TaskByArn(taskArn)
	if !found {
		return nil, introspection.NewErrorNotFound(fmt.Sprintf("task %s not found", taskArn))
	}
	containerMap, ok := agentState.ContainerMapByArn(task.Arn)
	if !ok {
		return nil, introspection.NewErrorNotFound(fmt.Sprintf("container map for task %s not found", taskArn))
	}
	return NewTaskResponse(task, containerMap), nil
}

// GetTaskMetadataByID returns task metadata in v1 format for the task with a matching docker ID, with an error
// if the metadata cannot be retieved.
func (as *AgentStateImpl) GetTaskMetadataByID(dockerID string) (*introspection.TaskResponse, error) {
	agentState := as.TaskEngine.State()
	task, found := agentState.TaskByID(dockerID)
	if !found {
		return nil, introspection.NewErrorNotFound(fmt.Sprintf("task %s not found", dockerID))
	}
	containerMap, ok := agentState.ContainerMapByArn(task.Arn)
	if !ok {
		return nil, introspection.NewErrorNotFound(fmt.Sprintf("container map for task %s not found", task.Arn))
	}
	return NewTaskResponse(task, containerMap), nil
}

// GetTaskMetadataByShortID returns task metadata in v1 format for the task with a matching short docker ID, with
// an error if the metadata cannot be retrieved.
func (as *AgentStateImpl) GetTaskMetadataByShortID(shortDockerID string) (*introspection.TaskResponse, error) {
	agentState := as.TaskEngine.State()
	tasks, found := agentState.TaskByShortID(shortDockerID)

	if !found {
		return nil, introspection.NewErrorNotFound(fmt.Sprintf("task %s not found", shortDockerID))
	}
	if len(tasks) > 1 {
		return nil, introspection.NewErrorBadRequest(fmt.Sprintf("multiple tasks found with short id %s", shortDockerID))
	}
	containerMap, ok := agentState.ContainerMapByArn(tasks[0].Arn)
	if !ok {
		return nil, introspection.NewErrorNotFound(fmt.Sprintf("container map for task %s not found", tasks[0].Arn))
	}
	return NewTaskResponse(tasks[0], containerMap), nil
}
