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
	"encoding/json"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	taskARN                  = "t1"
	cluster                  = "default"
	family                   = "sleep"
	version                  = "1"
	containerID              = "cid"
	containerName            = "sleepy"
	imageName                = "busybox"
	imageID                  = "bUsYbOx"
	cpu                      = 1024
	memory                   = 512
	eniIPv4Address           = "192.168.0.5"
	subnetGatewayIPV4Address = "192.168.0.1/24"
	volName                  = "volume1"
	volSource                = "/var/lib/volume1"
	volDestination           = "/volume"
	availabilityZone         = "us-west-2b"
	containerInstanceArn     = "containerInstance-test"
)

func TestNewTaskContainerResponses(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	now := time.Now()
	task := &apitask.Task{
		Arn:                 taskARN,
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		ENIs: []*apieni.ENI{
			{
				IPV4Addresses: []*apieni.ENIIPV4Address{
					{
						Address: eniIPv4Address,
					},
				},
				SubnetGatewayIPV4Address: subnetGatewayIPV4Address,
			},
		},
		CPU:                      cpu,
		Memory:                   memory,
		PullStartedAtUnsafe:      now,
		PullStoppedAtUnsafe:      now,
		ExecutionStoppedAtUnsafe: now,
	}
	container := &apicontainer.Container{
		Name:                containerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		Ports: []apicontainer.PortBinding{
			{
				ContainerPort: 80,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
		VolumesUnsafe: []types.MountPoint{
			{
				Name:        volName,
				Source:      volSource,
				Destination: volDestination,
			},
		},
	}
	created := time.Now()
	container.SetCreatedAt(created)
	labels := map[string]string{
		"foo": "bar",
	}
	container.SetLabels(labels)
	dockerContainer := &apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: containerName,
		Container:  container,
	}
	containerNameToDockerContainer := map[string]*apicontainer.DockerContainer{
		taskARN: dockerContainer,
	}
	gomock.InOrder(
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
	)

	taskResponse, err := NewTaskResponse(taskARN, state, ecsClient, cluster, availabilityZone, containerInstanceArn, false)
	require.NoError(t, err)
	_, err = json.Marshal(taskResponse)
	require.NoError(t, err)
	assert.Equal(t, created.UTC().String(), taskResponse.Containers[0].CreatedAt.String())
	assert.Equal(t, "192.168.0.0/24", taskResponse.Containers[0].Networks[0].IPV4SubnetCIDRBlock)
	assert.Equal(t, subnetGatewayIPV4Address, taskResponse.Containers[0].Networks[0].SubnetGatewayIPV4Address)

	gomock.InOrder(
		state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true),
		state.EXPECT().TaskByID(containerID).Return(task, true).Times(2),
	)
	containerResponse, err := NewContainerResponse(containerID, state)
	require.NoError(t, err)
	_, err = json.Marshal(containerResponse)
	require.NoError(t, err)
	assert.Equal(t, created.UTC().String(), containerResponse.CreatedAt.String())
	assert.Equal(t, "192.168.0.0/24", containerResponse.Networks[0].IPV4SubnetCIDRBlock)
	assert.Equal(t, subnetGatewayIPV4Address, containerResponse.Networks[0].SubnetGatewayIPV4Address)
}
