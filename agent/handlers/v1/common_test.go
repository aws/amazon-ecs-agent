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
	"time"

	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	"github.com/docker/docker/api/types"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
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
	containerTime    = time.Now()
	containerTimeUTC = containerTime.UTC()
)

func expectedPortResponse() response.PortResponse {
	return response.PortResponse{
		ContainerPort: port,
		Protocol:      protocol,
		HostPort:      port,
	}
}

func expectedNetworkResponse() response.Network {
	return response.Network{
		NetworkMode:   networkMode,
		IPv4Addresses: []string{eniIPv4Address},
	}
}

func expectedVolumeResponse() response.VolumeResponse {
	return response.VolumeResponse{
		DockerName:  volName,
		Source:      volSource,
		Destination: volDestination,
	}
}

func expectedContainerResponse() v1.ContainerResponse {
	return v1.ContainerResponse{
		DockerID:   containerID,
		DockerName: containerName,
		Name:       containerName,
		Image:      imageName,
		ImageID:    imageID,
		CreatedAt:  &containerTimeUTC,
		StartedAt:  &containerTimeUTC,
		Ports: []response.PortResponse{
			expectedPortResponse(),
		},
		Networks: []response.Network{
			expectedNetworkResponse(),
		},
		Volumes: []response.VolumeResponse{
			expectedVolumeResponse(),
		},
	}
}

func expectedTaskResponse() v1.TaskResponse {
	return v1.TaskResponse{
		Arn:           taskARN,
		DesiredStatus: status,
		KnownStatus:   status,
		Family:        family,
		Version:       version,
		Containers: []v1.ContainerResponse{
			expectedContainerResponse(),
		},
	}
}

func expectedTaskWithoutContainersResponse() v1.TaskResponse {
	return v1.TaskResponse{
		Arn:           taskARN,
		DesiredStatus: status,
		KnownStatus:   status,
		Family:        family,
		Version:       version,
		Containers:    []v1.ContainerResponse{},
	}
}

func testTask() *apitask.Task {
	return &apitask.Task{
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
}

func testContainer() *apicontainer.Container {
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

	return container
}

func testDockerContainer(container *apicontainer.Container) *apicontainer.DockerContainer {
	return &apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: containerName,
		Container:  container,
	}
}

func testContainerMap(container *apicontainer.Container) map[string]*apicontainer.DockerContainer {
	return map[string]*apicontainer.DockerContainer{
		taskARN: testDockerContainer(container),
	}
}
