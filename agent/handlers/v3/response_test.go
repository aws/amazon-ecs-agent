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

package v3

import (
	"encoding/json"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	taskARN             = "taskARN"
	dockerID            = "dockerID"
	dockerName          = "dockerName"
	containerName       = "containerName"
	associationType     = "elastic-inference"
	associationName     = "dev1"
	associationEncoding = "base64"
	associationValue    = "val"
)

var (
	container = &apicontainer.Container{
		Name: containerName,
	}

	dockerContainer = &apicontainer.DockerContainer{
		DockerID:   dockerID,
		DockerName: dockerName,
		Container:  container,
	}

	association = apitask.Association{
		Containers: []string{containerName},
		Content: apitask.EncodedString{
			Encoding: associationEncoding,
			Value:    associationValue,
		},
		Name: associationName,
		Type: associationType,
	}

	task = &apitask.Task{
		Arn:          taskARN,
		Containers:   []*apicontainer.Container{container},
		Associations: []apitask.Association{association},
	}
)

func TestContainerAssociationsResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	state := mock_dockerstate.NewMockTaskEngineState(ctrl)

	expectedAssociationsResponseMap := map[string]interface{}{
		"Associations": []interface{}{associationName},
	}

	gomock.InOrder(
		state.EXPECT().ContainerByID(dockerID).Return(dockerContainer, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)

	associationsResponse, err := NewAssociationsResponse(dockerID, taskARN, associationType, state)
	assert.NoError(t, err)

	associationsResponseJSON, err := json.Marshal(associationsResponse)
	assert.NoError(t, err)

	associationsResponseMap := make(map[string]interface{})
	json.Unmarshal(associationsResponseJSON, &associationsResponseMap)
	assert.Equal(t, expectedAssociationsResponseMap, associationsResponseMap)
}

func TestContainerAssociationResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	state := mock_dockerstate.NewMockTaskEngineState(ctrl)

	state.EXPECT().TaskByArn(taskARN).Return(task, true)

	associationResponse, err := NewAssociationResponse(taskARN, associationType, associationName, state)
	assert.NoError(t, err)

	// the response is expected to be the same as the association value
	assert.Equal(t, associationResponse, associationValue)
}
