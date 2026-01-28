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

package v4

import (
	"errors"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_stats "github.com/aws/amazon-ecs-agent/agent/stats/mock"
	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestSortContainersCNIPauseFirst(t *testing.T) {
	// Local constants for container types used in tests
	const (
		cniPause        = "CNI_PAUSE"
		normal          = "NORMAL"
		emptyHostVolume = "EMPTY_HOST_VOLUME"
	)

	testCases := []struct {
		name  string
		input []string // Container types
		want  []string // Expected order of types
	}{
		{
			name:  "CNI_PAUSE containers appear first",
			input: []string{normal, cniPause, normal, cniPause},
			want:  []string{cniPause, cniPause, normal, normal},
		},
		{
			name:  "only CNI_PAUSE containers",
			input: []string{cniPause, cniPause},
			want:  []string{cniPause, cniPause},
		},
		{
			name:  "only normal containers",
			input: []string{normal, normal},
			want:  []string{normal, normal},
		},
		{
			name:  "empty list",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "mixed types",
			input: []string{normal, cniPause, emptyHostVolume},
			want:  []string{cniPause, normal, emptyHostVolume},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert input to ContainerResponse slice
			containers := make([]tmdsv4.ContainerResponse, len(tc.input))
			for i, containerType := range tc.input {
				containers[i] = tmdsv4.ContainerResponse{
					ContainerResponse: &v2.ContainerResponse{Type: containerType},
				}
			}

			// Sort the containers
			sortContainersCNIPauseFirst(containers)

			// Extract types from result
			result := make([]string, len(containers))
			for i, container := range containers {
				result[i] = container.ContainerResponse.Type
			}

			// Verify the result
			assert.Equal(t, tc.want, result)
		})
	}
}

// TestGetTaskMetadataWithTags verifies that GetTaskMetadataWithTags correctly retrieves
// task metadata with tags included.
func TestGetTaskMetadataWithTags(t *testing.T) {
	testCases := []struct {
		name                   string
		v3EndpointID           string
		taskARN                string
		containerInstanceARN   string
		expectedErrorSubstring string
	}{
		{
			name:                   "successful metadata retrieval with tags",
			v3EndpointID:           "test-endpoint-id",
			taskARN:                "arn:aws:ecs:us-west-2:123456789:task/test-task",
			containerInstanceARN:   "arn:aws:ecs:us-west-2:123456789:container-instance/test",
			expectedErrorSubstring: "",
		},
		{
			name:                   "task not found by endpoint ID",
			v3EndpointID:           "non-existent-endpoint",
			expectedErrorSubstring: "unable to get task arn from request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
			mockStatsEngine := mock_stats.NewMockEngine(ctrl)
			mockECSClient := mock_ecs.NewMockECSClient(ctrl)

			// Create TMDSAgentState
			agentState := &TMDSAgentState{
				state:                mockState,
				statsEngine:          mockStatsEngine,
				ecsClient:            mockECSClient,
				cluster:              "test-cluster",
				availabilityZone:     "us-west-2a",
				availabilityZoneID:   "usw2-az2",
				vpcID:                "vpc-12345",
				containerInstanceARN: "arn:aws:ecs:us-west-2:123456789:container-instance/test",
			}

			expectedTags := []ecstypes.Tag{
				{Key: aws.String("Environment"), Value: aws.String("test")},
			}

			// Setup mocks
			mockState.EXPECT().TaskARNByV3EndpointID(tc.v3EndpointID).Return(tc.taskARN, tc.taskARN != "")
			mockState.EXPECT().ContainerNameByV3EndpointID(tc.v3EndpointID).Return("test-container", true).AnyTimes()
			mockState.EXPECT().ContainerMapByArn(tc.taskARN).Return(map[string]*apicontainer.DockerContainer{}, true).AnyTimes()
			mockState.EXPECT().PulledContainerMapByArn(tc.taskARN).Return(map[string]*apicontainer.DockerContainer{}, true).AnyTimes()
			if tc.taskARN != "" {
				mockTask := &apitask.Task{
					Arn: tc.taskARN,
				}
				mockState.EXPECT().TaskByArn(tc.taskARN).Return(mockTask, true).AnyTimes()
				mockECSClient.EXPECT().GetResourceTags(gomock.Any(), tc.taskARN).Return(
					expectedTags, nil).AnyTimes()
			}
			if tc.containerInstanceARN != "" {
				mockECSClient.EXPECT().GetResourceTags(gomock.Any(), tc.containerInstanceARN).Return(
					expectedTags, nil).AnyTimes()
			}

			response, err := agentState.GetTaskMetadataWithTags(tc.v3EndpointID)

			// Verify results
			if tc.expectedErrorSubstring != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrorSubstring)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, tc.taskARN, response.TaskARN)
				assert.Equal(t, map[string]string{"Environment": "test"}, response.TaskTags)
			}
		})
	}
}

func TestGetTasksMetadata_NotSupported(t *testing.T) {
	state := &TMDSAgentState{}
	_, err := state.GetTasksMetadata("test-container-id")

	var metadataErr *tmdsv4.ErrorMetadataFetchFailure
	assert.True(t, errors.As(err, &metadataErr))
	assert.Contains(t, err.Error(), "not supported")
}

func TestGetTasksMetadataWithTags_NotSupported(t *testing.T) {
	state := &TMDSAgentState{}
	_, err := state.GetTasksMetadataWithTags("test-container-id")

	var metadataErr *tmdsv4.ErrorMetadataFetchFailure
	assert.True(t, errors.As(err, &metadataErr))
	assert.Contains(t, err.Error(), "not supported")
}

func TestGetTasksStats_NotSupported(t *testing.T) {
	state := &TMDSAgentState{}
	_, err := state.GetTasksStats("test")

	var statsErr *tmdsv4.ErrorStatsFetchFailure
	assert.True(t, errors.As(err, &statsErr))
	assert.Contains(t, err.Error(), "not supported")
}
