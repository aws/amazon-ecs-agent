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
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
)

const (
	taskARN        = "t1"
	family         = "sleep"
	version        = "1"
	status         = "RUNNING"
	containerID    = "cid"
	containerName  = "sleepy"
	imageName      = "busybox"
	imageID        = "busyboxID"
	networkMode    = "awsvpc"
	eniIPv4Address = "10.0.0.2"
	port           = 80
	protocol       = "tcp"
	volName        = "volume1"
	volSource      = "/var/lib/volume1"
	volDestination = "/volume"
)

var (
	containerTime        = time.Now()
	containerTimeUTC     = containerTime.UTC()
	expectedPortResponse = response.PortResponse{
		ContainerPort: port,
		Protocol:      protocol,
		HostPort:      port,
	}
	expectedNetworkResponse = response.Network{
		NetworkMode:   networkMode,
		IPv4Addresses: []string{eniIPv4Address},
	}
	expectedVolumeResponse = response.VolumeResponse{
		DockerName:  volName,
		Source:      volSource,
		Destination: volDestination,
	}
	expectedContainerResponse = ContainerResponse{
		DockerID:   containerID,
		DockerName: containerName,
		Name:       containerName,
		Image:      imageName,
		ImageID:    imageID,
		CreatedAt:  &containerTimeUTC,
		StartedAt:  &containerTimeUTC,
		Ports: []response.PortResponse{
			expectedPortResponse,
		},
		Networks: []response.Network{
			expectedNetworkResponse,
		},
		Volumes: []response.VolumeResponse{
			expectedVolumeResponse,
		},
	}
	expectedTaskResponse = TaskResponse{
		Arn:           taskARN,
		DesiredStatus: status,
		KnownStatus:   status,
		Family:        family,
		Version:       version,
		Containers: []ContainerResponse{
			expectedContainerResponse,
		},
	}
)

func TestTaskResponse(t *testing.T) {
	task := &apitask.Task{
		Arn:                 taskARN,
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		ENIs: []*ni.NetworkInterface{
			{
				IPV4Addresses: []*ni.IPV4Address{
					{
						Address: eniIPv4Address,
					},
				},
			},
		},
	}

	container := &apicontainer.Container{
		Name:    containerName,
		Image:   imageName,
		ImageID: imageID,
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

	container.SetCreatedAt(containerTime)
	container.SetStartedAt(containerTime)

	containerNameToDockerContainer := map[string]*apicontainer.DockerContainer{
		taskARN: {
			DockerID:   containerID,
			DockerName: containerName,
			Container:  container,
		},
	}

	taskResponse := NewTaskResponse(task, containerNameToDockerContainer)

	_, err := json.Marshal(taskResponse)
	assert.NoError(t, err)

	assert.Equal(t, expectedTaskResponse, *taskResponse)
}

func TestContainerResponse(t *testing.T) {
	container := &apicontainer.Container{
		Name:    containerName,
		Image:   imageName,
		ImageID: imageID,
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

	container.SetCreatedAt(containerTime)
	container.SetStartedAt(containerTime)

	dockerContainer := &apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: containerName,
		Container:  container,
	}

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
