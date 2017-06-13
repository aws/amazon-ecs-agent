// +build !integration
// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
)

const (
	cluster = "us-west2"
)

// TestParseContainerCreate checks case when parsing is done at metadata creation
func TestParseContainerCreate(t *testing.T) {
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadataAtContainerCreate(mockTaskARN, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expcted container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expcted task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expcted container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), "INITIAL", "Expected status "+MetadataInitial)
}

func TestParseHasNoContainer(t *testing.T) {
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(nil, mockTaskARN, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expcted container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expcted task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expcted container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), "READY", "Expected status "+MetadataInitial)
	assert.Equal(t, metadata.dockerContainerMetadata.containerID, "", "Expected empty container metadata")
	assert.Equal(t, metadata.dockerContainerMetadata.dockerContainerName, "", "Expected empty container metadata")
	assert.Equal(t, metadata.dockerContainerMetadata.imageID, "", "Expected empty container metadata")
	assert.Equal(t, metadata.dockerContainerMetadata.imageName, "", "Expected empty container metadata")
}

func TestParseHasConfig(t *testing.T) {
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockConfig := &docker.Config{Image: "image"}
	mockContainer := &docker.Container{Config: mockConfig}

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTaskARN, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expcted container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expcted task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expcted container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), "READY", "Expected status "+MetadataInitial)
	assert.Equal(t, metadata.dockerContainerMetadata.imageName, "image", "Expected nonempty imageID")
}

func TestParseHasHostConfigPortBindsError(t *testing.T) {
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockPorts := make(map[docker.Port][]docker.PortBinding)
	mockPortBinding := make([]docker.PortBinding, 0)
	mockPortBinding = append(mockPortBinding, docker.PortBinding{})
	mockPorts["80/tcp"] = mockPortBinding
	mockHostConfig := &docker.HostConfig{PortBindings: mockPorts}
	mockContainer := &docker.Container{HostConfig: mockHostConfig}

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTaskARN, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expcted container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expcted task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expcted container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), "READY", "Expected status "+MetadataInitial)
	assert.Equal(t, len(metadata.dockerContainerMetadata.ports), 0, "Expected empty list of ports")
}

func TestParseHasHostConfigPortBindsNoError(t *testing.T) {
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockPorts := make(map[docker.Port][]docker.PortBinding)
	mockPortBinding := make([]docker.PortBinding, 0)
	mockPortBinding = append(mockPortBinding, docker.PortBinding{HostIP: "0.0.0.0", HostPort: "80"})
	mockPorts["80/tcp"] = mockPortBinding
	mockHostConfig := &docker.HostConfig{PortBindings: mockPorts}
	mockContainer := &docker.Container{HostConfig: mockHostConfig}

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTaskARN, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expcted container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expcted task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expcted container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), "READY", "Expected status "+MetadataInitial)
	assert.Equal(t, len(metadata.dockerContainerMetadata.ports), 1, "Expected empty list of ports")
}

func TestParseHasNetworkSettingsNetworksEmpty(t *testing.T) {
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockHostConfig := &docker.HostConfig{NetworkMode: "bridge"}
	mockNetworkSettings := &docker.NetworkSettings{IPAddress: "0.0.0.0"}
	mockContainer := &docker.Container{HostConfig: mockHostConfig, NetworkSettings: mockNetworkSettings}

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTaskARN, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expcted container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expcted task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expcted container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), "READY", "Expected status "+MetadataInitial)
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 1, "Expected one network")
}

func TestParseHasNetworkSettingsNetworksNonEmpty(t *testing.T) {
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockHostConfig := &docker.HostConfig{NetworkMode: "bridge"}
	mockNetworks := make(map[string]docker.ContainerNetwork)
	mockNetworks["bridge"] = docker.ContainerNetwork{}
	mockNetworks["network0"] = docker.ContainerNetwork{}
	mockNetworkSettings := &docker.NetworkSettings{Networks: mockNetworks}
	mockContainer := &docker.Container{HostConfig: mockHostConfig, NetworkSettings: mockNetworkSettings}

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTaskARN, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expcted container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expcted task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expcted container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), "READY", "Expected status "+MetadataInitial)
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 2, "Expected two networks")
}
