//go:build !linux && unit
// +build !linux,unit

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

package handlers

import (
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	agentV4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	mock_stats "github.com/aws/amazon-ecs-agent/agent/stats/mock"
	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	v4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestV4GetTaskMetadataWithTaskNetworkConfig(t *testing.T) {

	tcs := []struct {
		name                      string
		setStateExpectations      func(state *mock_dockerstate.MockTaskEngineState)
		expectedTaskNetworkConfig *v4.TaskNetworkConfig
	}{
		{
			name: "happy case with awsvpc mode",
			setStateExpectations: func(state *mock_dockerstate.MockTaskEngineState) {
				task := standardTask()
				task.EnableFaultInjection = true
				task.NetworkNamespace = networkNamespace
				task.DefaultIfname = defaultIfname
				gomock.InOrder(
					state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
					state.EXPECT().TaskByArn(taskARN).Return(task, true).Times(2),
					state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true).AnyTimes(),
					state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
					state.EXPECT().TaskByArn(taskARN).Return(task, true),
					state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true).AnyTimes(),
					state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
				)
			},
			expectedTaskNetworkConfig: state.NewTaskNetworkConfig(apitask.AWSVPCNetworkMode,
				networkNamespace,
				[]*state.NetworkInterface{{
					DeviceName:    defaultIfname,
					IPV4Addresses: []string{eniIPv4Address},
				}},
			),
		},
		{
			name: "happy case with host mode",
			setStateExpectations: func(state *mock_dockerstate.MockTaskEngineState) {
				hostTask := standardHostTask()
				hostTask.EnableFaultInjection = true
				hostTask.NetworkNamespace = networkNamespace
				hostTask.DefaultIfname = defaultIfname
				gomock.InOrder(
					state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
					state.EXPECT().TaskByArn(taskARN).Return(hostTask, true).Times(2),
					state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
					state.EXPECT().ContainerByID(containerID).Return(nil, false).AnyTimes(),
					state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
					state.EXPECT().ContainerByID(containerID).Return(nil, false).AnyTimes(),
				)
			},
			expectedTaskNetworkConfig: state.NewTaskNetworkConfig(apitask.HostNetworkMode,
				hostNetworkNamespace,
				[]*state.NetworkInterface{{DeviceName: defaultIfname}},
			),
		},
		{
			name: "happy bridge mode",
			setStateExpectations: func(state *mock_dockerstate.MockTaskEngineState) {
				gomock.InOrder(
					state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
					state.EXPECT().TaskByArn(taskARN).Return(bridgeTask, true).Times(2),
					state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToBridgeContainer, true),
					state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true).AnyTimes(),
					state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
					state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true).AnyTimes(),
				)
			},
			expectedTaskNetworkConfig: state.NewTaskNetworkConfig(bridgeMode, "", nil),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			state := mock_dockerstate.NewMockTaskEngineState(ctrl)
			statsEngine := mock_stats.NewMockEngine(ctrl)
			ecsClient := mock_ecs.NewMockECSClient(ctrl)

			if tc.setStateExpectations != nil {
				tc.setStateExpectations(state)
			}
			tmdsAgentState := agentV4.NewTMDSAgentState(state, statsEngine, ecsClient, clusterName, availabilityzone, vpcID, containerInstanceArn)
			actualTaskResponse, err := tmdsAgentState.GetTaskMetadataWithTaskNetworkConfig(v3EndpointID, nil)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTaskNetworkConfig, actualTaskResponse.TaskNetworkConfig)
		})
	}
}
