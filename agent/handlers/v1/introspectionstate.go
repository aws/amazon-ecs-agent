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

	"github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	handlerutils "github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	agentversion "github.com/aws/amazon-ecs-agent/agent/version"
	v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1"
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
func (as *AgentStateImpl) GetAgentMetadata() (*v1.AgentMetadataResponse, error) {
	return &v1.AgentMetadataResponse{
		Cluster:              as.ClusterName,
		ContainerInstanceArn: as.ContainerInstanceArn,
		Version:              agentversion.String(),
	}, nil
}

// GetTasksMetadata returns task metadata in v1 format for all tasks on the host with an error if the metadata
// cannot be retrieved.
func (as *AgentStateImpl) GetTasksMetadata() (*v1.TasksResponse, error) {
	agentState := as.TaskEngine.State()
	allTasks := agentState.AllExternalTasks()
	taskResponses := make([]*v1.TaskResponse, len(allTasks))
	for ndx, task := range allTasks {
		containerMap, err := getContainerMap(agentState, task.Arn)
		if err != nil {
			return nil, err
		}
		taskResponses[ndx] = NewTaskResponse(task, containerMap)
	}
	return &v1.TasksResponse{Tasks: taskResponses}, nil
}

// GetTaskMetadataByArn returns task metadata in v1 format for the task with a matching task Arn, with an error
// if the metadata cannot be retrieved.
func (as *AgentStateImpl) GetTaskMetadataByArn(taskArn string) (*v1.TaskResponse, error) {
	agentState := as.TaskEngine.State()
	task, found := agentState.TaskByArn(taskArn)
	return createTaskResponse(taskArn, agentState, task, found)
}

// GetTaskMetadataByID returns task metadata in v1 format for the task with a matching docker ID, with an error
// if the metadata cannot be retieved.
func (as *AgentStateImpl) GetTaskMetadataByID(dockerID string) (*v1.TaskResponse, error) {
	agentState := as.TaskEngine.State()
	task, found := agentState.TaskByID(dockerID)
	return createTaskResponse(dockerID, agentState, task, found)
}

// GetTaskMetadataByShortID returns task metadata in v1 format for the task with a matching short docker ID, with
// an error if the metadata cannot be retrieved.
func (as *AgentStateImpl) GetTaskMetadataByShortID(shortDockerID string) (*v1.TaskResponse, error) {
	agentState := as.TaskEngine.State()
	tasks, found := agentState.TaskByShortID(shortDockerID)
	if len(tasks) > 1 {
		return nil, v1.NewErrorMultipleTasksFound(fmt.Sprintf("multiple tasks found with short id %s", shortDockerID))
	}
	var task *apitask.Task
	if found {
		task = tasks[0]
	}
	return createTaskResponse(shortDockerID, agentState, task, found)
}

// createTaskResponse looks up a task and returns the task metadata response in v1 format or a not found error
// if the response cannot be constructed.
func createTaskResponse(
	key string,
	agentState dockerstate.TaskEngineState,
	task *apitask.Task, found bool) (*v1.TaskResponse, error) {
	if !found {
		return nil, v1.NewErrorNotFound(fmt.Sprintf("task %s not found", key))
	}
	containerMap, err := getContainerMap(agentState, task.Arn)
	if err != nil {
		return nil, err
	}
	return NewTaskResponse(task, containerMap), nil
}

// getContainerMap returns a map of the containers belonging to a task or a not found error if it cannot.
func getContainerMap(agentState dockerstate.TaskEngineState, taskArn string) (map[string]*container.DockerContainer, error) {
	containers, ok := agentState.ContainerMapByArn(taskArn)
	if !ok {
		return nil, v1.NewErrorFetchFailure(fmt.Sprintf("container map for task %s not found", taskArn))
	}
	return containers, nil
}
