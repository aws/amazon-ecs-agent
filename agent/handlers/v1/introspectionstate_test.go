//go:build unit
// +build unit

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
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_utils "github.com/aws/amazon-ecs-agent/agent/handlers/mocks"
	agentversion "github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	clusterName = "test-cluster"
)

var containerInstanceArn = aws.String("arn/test")

func prepareTestState(t *testing.T) (*gomock.Controller, *AgentStateImpl) {
	ctrl := gomock.NewController(t)

	mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)

	agentState := &AgentStateImpl{
		ContainerInstanceArn: containerInstanceArn,
		ClusterName:          clusterName,
		TaskEngine:           mockDockerState,
	}

	return ctrl, agentState
}

func TestGetAgentMetadata(t *testing.T) {
	agentState := &AgentStateImpl{
		ContainerInstanceArn: containerInstanceArn,
		ClusterName:          clusterName,
	}
	response, _ := agentState.GetAgentMetadata()

	assert.Equal(t, response.Cluster, clusterName)
	assert.Equal(t, response.ContainerInstanceArn, containerInstanceArn)
	assert.Equal(t, response.Version, agentversion.String())
}

func TestGetTasksMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)

	task := testTask()
	container := testContainer()
	task.Containers = append(task.Containers, container)
	tasks := []*apitask.Task{
		task,
	}
	containerMap := testContainerMap(container)

	mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
	mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockTaskEngine.EXPECT().AllExternalTasks().Return(tasks)
	mockTaskEngine.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true)

	mockDockerState.EXPECT().State().Return(mockTaskEngine)

	agentState := &AgentStateImpl{
		ContainerInstanceArn: containerInstanceArn,
		ClusterName:          clusterName,
		TaskEngine:           mockDockerState,
	}
	response, _ := agentState.GetTasksMetadata()

	assert.Equal(t, &introspection.TasksResponse{
		Tasks: []*introspection.TaskResponse{
			&expectedTaskResponse,
		},
	}, response)
}

func TestGetTaskMetadataByArn(t *testing.T) {
	t.Run("happy case", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		task := testTask()
		container := testContainer()
		containerMap := testContainerMap(container)

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByArn(taskARN).Return(task, true)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)
		mockTaskEngine.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}
		response, _ := agentState.GetTaskMetadataByArn(taskARN)

		assert.Equal(t, &expectedTaskResponse, response)
	})
	t.Run("task not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByArn(taskARN).Return(nil, false)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}
		response, err := agentState.GetTaskMetadataByArn(taskARN)

		assert.Nil(t, response)
		assert.Equal(t, introspection.NewErrorNotFound(fmt.Sprintf("Task %s not found", taskARN)), err)
	})

	t.Run("container map not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		task := testTask()

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByArn(taskARN).Return(task, true)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)
		mockTaskEngine.EXPECT().ContainerMapByArn(taskARN).Return(nil, false)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}

		response, err := agentState.GetTaskMetadataByArn(taskARN)

		assert.Nil(t, response)
		assert.Equal(t, introspection.NewErrorNotFound(fmt.Sprintf("Container map for task %s not found", taskARN)), err)
	})
}

func TestGetTaskMetadataByID(t *testing.T) {
	t.Run("happy case", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		task := testTask()
		container := testContainer()
		containerMap := testContainerMap(container)

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByID(containerID).Return(task, true)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)
		mockTaskEngine.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}
		response, _ := agentState.GetTaskMetadataByID(containerID)

		assert.Equal(t, &expectedTaskResponse, response)
	})
	t.Run("task not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByID(containerID).Return(nil, false)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}
		response, err := agentState.GetTaskMetadataByID(containerID)

		assert.Nil(t, response)
		assert.Equal(t, introspection.NewErrorNotFound(fmt.Sprintf("Task %s not found", containerID)), err)
	})

	t.Run("container map not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		task := testTask()

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByID(containerID).Return(task, true)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)
		mockTaskEngine.EXPECT().ContainerMapByArn(taskARN).Return(nil, false)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}

		response, err := agentState.GetTaskMetadataByID(containerID)

		assert.Nil(t, response)
		assert.Equal(t, introspection.NewErrorNotFound(fmt.Sprintf("Container map for task %s not found", taskARN)), err)
	})
}

func TestGetTaskMetadataByShortID(t *testing.T) {
	t.Run("happy case", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		task := testTask()
		container := testContainer()
		containerMap := testContainerMap(container)

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByShortID(containerID).Return([]*apitask.Task{task}, true)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)
		mockTaskEngine.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}
		response, _ := agentState.GetTaskMetadataByShortID(containerID)

		assert.Equal(t, &expectedTaskResponse, response)
	})

	t.Run("task not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByShortID(containerID).Return([]*apitask.Task{}, false)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}
		response, err := agentState.GetTaskMetadataByShortID(containerID)

		assert.Nil(t, response)
		assert.Equal(t, introspection.NewErrorNotFound(fmt.Sprintf("Task %s not found", containerID)), err)
	})

	t.Run("multiple found", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		task := testTask()

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByShortID(containerID).Return([]*apitask.Task{task, task}, false)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}
		response, err := agentState.GetTaskMetadataByShortID(containerID)

		assert.Nil(t, response)
		assert.Equal(t, introspection.NewErrorBadRequest(fmt.Sprintf("Multiple tasks found with short id %s", containerID)), err)
	})

	t.Run("container map not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		task := testTask()

		mockDockerState := mock_utils.NewMockDockerStateResolver(ctrl)
		mockTaskEngine := mock_dockerstate.NewMockTaskEngineState(ctrl)
		mockTaskEngine.EXPECT().TaskByShortID(containerID).Return([]*apitask.Task{task}, true)

		mockDockerState.EXPECT().State().Return(mockTaskEngine)
		mockTaskEngine.EXPECT().ContainerMapByArn(taskARN).Return(nil, false)

		agentState := &AgentStateImpl{
			ContainerInstanceArn: containerInstanceArn,
			ClusterName:          clusterName,
			TaskEngine:           mockDockerState,
		}

		response, err := agentState.GetTaskMetadataByShortID(containerID)

		assert.Nil(t, response)
		assert.Equal(t, introspection.NewErrorNotFound(fmt.Sprintf("Container map for task %s not found", taskARN)), err)
	})
}
