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

package containermetadata

import (
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
)

const (
	cluster = "us-west2"
)

// TestParseContainerCreate checks case when parsing is done at metadata creation
func TestParseContainerCreate(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTaskDefinitionFamily := taskDefinitionFamily
	mockTaskDefinitionRevision := taskDefinitionRevision
	mockTask := &apitask.Task{Arn: mockTaskARN, Family: mockTaskDefinitionFamily, Version: mockTaskDefinitionRevision}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN
	mockAvailabilityZone := availabilityZone
	mockHostPrivateIPv4Address := hostPrivateIPv4Address
	mockHostPublicIPv4Address := hostPublicIPv4Address

	expectedStatus := string(MetadataInitial)

	newManager := &metadataManager{
		cluster:                mockCluster,
		containerInstanceARN:   mockContainerInstanceARN,
		availabilityZone:       mockAvailabilityZone,
		hostPrivateIPv4Address: mockHostPrivateIPv4Address,
		hostPublicIPv4Address:  mockHostPublicIPv4Address,
	}

	metadata := newManager.parseMetadataAtContainerCreate(mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, metadata.availabilityZone, mockAvailabilityZone, "Expected availabilityZone "+mockAvailabilityZone)
	assert.Equal(t, metadata.hostPrivateIPv4Address, mockHostPrivateIPv4Address, "Expected hostPrivateIPv4Address "+hostPrivateIPv4Address)
	assert.Equal(t, metadata.hostPublicIPv4Address, mockHostPublicIPv4Address, "Expected hostPublicIPv4Address "+hostPublicIPv4Address)
	assert.Equal(t, metadata.taskMetadata.taskDefinitionFamily, mockTaskDefinitionFamily, "Expected task definition family "+mockTaskDefinitionFamily)
	assert.Equal(t, metadata.taskMetadata.taskDefinitionRevision, mockTaskDefinitionRevision, "Expected task definition revision "+mockTaskDefinitionRevision)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)
}

func TestParseHasNoContainer(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN
	mockAvailabilityZone := availabilityZone
	mockHostPrivateIPv4Address := hostPrivateIPv4Address
	mockHostPublicIPv4Address := hostPublicIPv4Address

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:                mockCluster,
		containerInstanceARN:   mockContainerInstanceARN,
		availabilityZone:       mockAvailabilityZone,
		hostPrivateIPv4Address: mockHostPrivateIPv4Address,
		hostPublicIPv4Address:  mockHostPublicIPv4Address,
	}

	metadata := newManager.parseMetadata(nil, mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, metadata.availabilityZone, mockAvailabilityZone, "Expected availabilityZone "+mockAvailabilityZone)
	assert.Equal(t, metadata.hostPrivateIPv4Address, mockHostPrivateIPv4Address, "Expected hostPrivateIPv4Address "+hostPrivateIPv4Address)
	assert.Equal(t, metadata.hostPublicIPv4Address, mockHostPublicIPv4Address, "Expected hostPublicIPv4Address "+hostPublicIPv4Address)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)
	assert.Equal(t, metadata.dockerContainerMetadata.containerID, "", "Expected empty container metadata")
	assert.Equal(t, metadata.dockerContainerMetadata.dockerContainerName, "", "Expected empty container metadata")
	assert.Equal(t, metadata.dockerContainerMetadata.imageID, "", "Expected empty container metadata")
	assert.Equal(t, metadata.dockerContainerMetadata.imageName, "", "Expected empty container metadata")
}

func TestParseHasConfig(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN
	mockAvailabilityZone := availabilityZone
	mockHostPrivateIPv4Address := hostPrivateIPv4Address
	mockHostPublicIPv4Address := hostPublicIPv4Address

	mockConfig := &dockercontainer.Config{Image: "image"}

	mockNetworks := map[string]*network.EndpointSettings{}
	mockNetworkSettings := &types.NetworkSettings{Networks: mockNetworks}

	mockContainer := &types.ContainerJSON{
		Config:          mockConfig,
		NetworkSettings: mockNetworkSettings,
	}

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:                mockCluster,
		containerInstanceARN:   mockContainerInstanceARN,
		availabilityZone:       mockAvailabilityZone,
		hostPrivateIPv4Address: mockHostPrivateIPv4Address,
		hostPublicIPv4Address:  mockHostPublicIPv4Address,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTask, mockContainerName)

	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, metadata.availabilityZone, mockAvailabilityZone, "Expected availabilityZone "+mockAvailabilityZone)
	assert.Equal(t, metadata.hostPrivateIPv4Address, mockHostPrivateIPv4Address, "Expected hostPrivateIPv4Address "+hostPrivateIPv4Address)
	assert.Equal(t, metadata.hostPublicIPv4Address, mockHostPublicIPv4Address, "Expected hostPublicIPv4Address "+hostPublicIPv4Address)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)
	assert.Equal(t, metadata.dockerContainerMetadata.imageName, "image", "Expected nonempty imageID")
}

func TestParseHasNetworkSettingsPortBindings(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN
	mockAvailabilityZone := availabilityZone
	mockHostPrivateIPv4Address := hostPrivateIPv4Address
	mockHostPublicIPv4Address := hostPublicIPv4Address

	mockPorts := nat.PortMap{}
	mockPortBinding := make([]nat.PortBinding, 0)
	mockPortBinding = append(mockPortBinding, nat.PortBinding{HostIP: "0.0.0.0", HostPort: "8080"})
	mockPorts["80/tcp"] = mockPortBinding

	mockHostConfig := &dockercontainer.HostConfig{NetworkMode: "bridge"}
	mockNetworks := map[string]*network.EndpointSettings{}
	mockNetworks["bridge"] = &network.EndpointSettings{}
	mockNetworks["network0"] = &network.EndpointSettings{}
	mockNetworkSettings := &types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{
			Ports: mockPorts,
		},
		Networks: mockNetworks,
	}
	mockContainer := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			HostConfig: mockHostConfig,
		},
		NetworkSettings: mockNetworkSettings,
	}

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:                mockCluster,
		containerInstanceARN:   mockContainerInstanceARN,
		availabilityZone:       mockAvailabilityZone,
		hostPrivateIPv4Address: mockHostPrivateIPv4Address,
		hostPublicIPv4Address:  mockHostPublicIPv4Address,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, metadata.availabilityZone, mockAvailabilityZone, "Expected availabilityZone "+mockAvailabilityZone)
	assert.Equal(t, metadata.hostPrivateIPv4Address, mockHostPrivateIPv4Address, "Expected hostPrivateIPv4Address "+hostPrivateIPv4Address)
	assert.Equal(t, metadata.hostPublicIPv4Address, mockHostPublicIPv4Address, "Expected hostPublicIPv4Address "+hostPublicIPv4Address)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 2, "Expected two networks")

	assert.Equal(t, len(metadata.dockerContainerMetadata.ports), 1, "Expected nonempty list of ports")
	assert.Equal(t, aws.Uint16(80), metadata.dockerContainerMetadata.ports[0].ContainerPort, "Expected nonempty ContainerPort field")
	assert.Equal(t, uint16(8080), metadata.dockerContainerMetadata.ports[0].HostPort, "Expected nonempty HostPort field")
	assert.Equal(t, "0.0.0.0", metadata.dockerContainerMetadata.ports[0].BindIP, "Expected nonempty HostIP field")
}

func TestParseHasNetworkSettingsNetworksEmpty(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN
	mockAvailabilityZone := availabilityZone
	mockHostPrivateIPv4Address := hostPrivateIPv4Address
	mockHostPublicIPv4Address := hostPublicIPv4Address

	mockHostConfig := &dockercontainer.HostConfig{NetworkMode: "bridge"}
	mockNetworkSettings := &types.NetworkSettings{
		DefaultNetworkSettings: types.DefaultNetworkSettings{
			IPAddress: "0.0.0.0",
		}}
	mockContainer := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			HostConfig: mockHostConfig,
		},
		NetworkSettings: mockNetworkSettings,
	}

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:                mockCluster,
		containerInstanceARN:   mockContainerInstanceARN,
		availabilityZone:       mockAvailabilityZone,
		hostPrivateIPv4Address: mockHostPrivateIPv4Address,
		hostPublicIPv4Address:  mockHostPublicIPv4Address,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, metadata.availabilityZone, mockAvailabilityZone, "Expected availabilityZone "+mockAvailabilityZone)
	assert.Equal(t, metadata.hostPrivateIPv4Address, mockHostPrivateIPv4Address, "Expected hostPrivateIPv4Address "+hostPrivateIPv4Address)
	assert.Equal(t, metadata.hostPublicIPv4Address, mockHostPublicIPv4Address, "Expected hostPublicIPv4Address "+hostPublicIPv4Address)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 1, "Expected one network")
}

func TestParseHasNetworkSettingsNetworksNonEmpty(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN
	mockAvailabilityZone := availabilityZone
	mockHostPrivateIPv4Address := hostPrivateIPv4Address
	mockHostPublicIPv4Address := hostPublicIPv4Address

	mockHostConfig := &dockercontainer.HostConfig{NetworkMode: dockercontainer.NetworkMode("bridge")}
	mockNetworks := map[string]*network.EndpointSettings{}
	mockNetworks["bridge"] = &network.EndpointSettings{}
	mockNetworks["network0"] = &network.EndpointSettings{}
	mockNetworkSettings := &types.NetworkSettings{
		Networks: mockNetworks,
	}
	mockContainer := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			HostConfig: mockHostConfig,
		},
		NetworkSettings: mockNetworkSettings,
	}

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:                mockCluster,
		containerInstanceARN:   mockContainerInstanceARN,
		availabilityZone:       mockAvailabilityZone,
		hostPrivateIPv4Address: mockHostPrivateIPv4Address,
		hostPublicIPv4Address:  mockHostPublicIPv4Address,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, metadata.availabilityZone, mockAvailabilityZone, "Expected AvailabilityZone"+mockAvailabilityZone)
	assert.Equal(t, metadata.hostPrivateIPv4Address, mockHostPrivateIPv4Address, "Expected hostPrivateIPv4Address "+hostPrivateIPv4Address)
	assert.Equal(t, metadata.hostPublicIPv4Address, mockHostPublicIPv4Address, "Expected hostPublicIPv4Address "+hostPublicIPv4Address)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 2, "Expected two networks")
}

func TestParseHasNoContainerJSONBase(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockConfig := &dockercontainer.Config{Image: "image"}
	mockNetworkSettings := &types.NetworkSettings{
		DefaultNetworkSettings: types.DefaultNetworkSettings{
			IPAddress: "0.0.0.0",
		}}
	mockContainer := &types.ContainerJSON{
		NetworkSettings: mockNetworkSettings,
		Config:          mockConfig,
	}

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 0, "Expected one network")
	assert.Equal(t, metadata.dockerContainerMetadata.imageName, "image")
}

func TestParseTaskDefinitionSettings(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN
	mockAvailabilityZone := availabilityZone
	mockHostPrivateIPv4Address := hostPrivateIPv4Address
	mockHostPublicIPv4Address := hostPublicIPv4Address

	mockHostConfig := &dockercontainer.HostConfig{NetworkMode: dockercontainer.NetworkMode("bridge")}
	mockConfig := &dockercontainer.Config{Image: "image"}
	mockNetworkSettings := &types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{
			LinkLocalIPv6Address: "0.0.0.0",
		},
	}
	mockContainer := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			HostConfig: mockHostConfig,
		},
		Config:          mockConfig,
		NetworkSettings: mockNetworkSettings,
	}

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:                mockCluster,
		containerInstanceARN:   mockContainerInstanceARN,
		availabilityZone:       mockAvailabilityZone,
		hostPrivateIPv4Address: mockHostPrivateIPv4Address,
		hostPublicIPv4Address:  mockHostPublicIPv4Address,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.taskMetadata.taskDefinitionFamily, "", "Expected no task definition family")
	assert.Equal(t, metadata.taskMetadata.taskDefinitionRevision, "", "Expected no task definition revision")
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, metadata.availabilityZone, mockAvailabilityZone, "Expected availabilityZone "+mockAvailabilityZone)
	assert.Equal(t, metadata.hostPrivateIPv4Address, mockHostPrivateIPv4Address, "Expected hostPrivateIPv4Address "+hostPrivateIPv4Address)
	assert.Equal(t, metadata.hostPublicIPv4Address, mockHostPublicIPv4Address, "Expected hostPublicIPv4Address "+hostPublicIPv4Address)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)

	// now add the task definition details
	mockTaskDefinitionFamily := taskDefinitionFamily
	mockTaskDefinitionRevision := taskDefinitionRevision
	mockTask = &apitask.Task{Arn: mockTaskARN, Family: mockTaskDefinitionFamily, Version: mockTaskDefinitionRevision}
	metadata = newManager.parseMetadata(nil, mockTask, mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskDefinitionFamily, mockTaskDefinitionFamily, "Expected task definition family "+mockTaskDefinitionFamily)
	assert.Equal(t, metadata.taskMetadata.taskDefinitionRevision, mockTaskDefinitionRevision, "Expected task definition revision "+mockTaskDefinitionRevision)
}
