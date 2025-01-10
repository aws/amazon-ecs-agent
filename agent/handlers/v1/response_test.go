//go:build unit
// +build unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package v1

import (
	"encoding/json"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/container/restart"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
)

func TestTaskResponse(t *testing.T) {
	task := testTask()
	container := testContainer()
	containerNameToDockerContainer := testContainerMap(container)

	taskResponse := NewTaskResponse(task, containerNameToDockerContainer)

	_, err := json.Marshal(taskResponse)
	assert.NoError(t, err)

	assert.Equal(t, expectedTaskResponse, *taskResponse)
}

func TestContainerResponse(t *testing.T) {
	container := testContainer()

	dockerContainer := testDockerContainer(container)

	eni := &ni.NetworkInterface{
		IPV4Addresses: []*ni.IPV4Address{
			{
				Address: eniIPv4Address,
			},
		},
	}

	containerResponse := NewContainerResponse(dockerContainer, eni)

	_, err := json.Marshal(containerResponse)
	assert.NoError(t, err)

	assert.Equal(t, expectedContainerResponse, containerResponse)
}

func TestContainerResponseWithRestartPolicy(t *testing.T) {
	restartPolicy := &restart.RestartPolicy{
		Enabled: true,
	}
	container := testContainer()

	container.RestartPolicy = restartPolicy
	container.RestartTracker = restart.NewRestartTracker(*restartPolicy)
	container.RestartTracker.RecordRestart()

	dockerContainer := testDockerContainer(container)

	eni := &ni.NetworkInterface{
		IPV4Addresses: []*ni.IPV4Address{
			{
				Address: eniIPv4Address,
			},
		},
	}

	containerResponse := NewContainerResponse(dockerContainer, eni)

	_, err := json.Marshal(containerResponse)
	assert.NoError(t, err)

	expectedContainerResponseWithRestartPolicy := expectedContainerResponse
	rc := int(1)
	expectedContainerResponseWithRestartPolicy.RestartCount = &rc

	assert.Equal(t, expectedContainerResponseWithRestartPolicy, containerResponse)
}

func TestPortBindingsResponse(t *testing.T) {
	container := &apicontainer.Container{
		Name: containerName,
		Ports: []apicontainer.PortBinding{
			{
				ContainerPort: 80,
				HostPort:      80,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}

	dockerContainer := &apicontainer.DockerContainer{
		Container: container,
	}

	PortBindingsResponse := NewPortBindingsResponse(dockerContainer, nil)
	assert.Equal(t, expectedPortResponse, PortBindingsResponse[0])
}

func TestVolumesResponse(t *testing.T) {
	container := &apicontainer.Container{
		Name: containerName,
		VolumesUnsafe: []types.MountPoint{
			{
				Name:        volName,
				Source:      volSource,
				Destination: volDestination,
			},
		},
	}

	dockerContainer := &apicontainer.DockerContainer{
		Container: container,
	}

	VolumesResponse := NewVolumesResponse(dockerContainer)
	assert.Equal(t, expectedVolumeResponse, VolumesResponse[0])
}
