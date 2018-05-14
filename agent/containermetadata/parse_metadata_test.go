// +build unit

// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	docker "github.com/fsouza/go-dockerclient"
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

	expectedStatus := string(MetadataInitial)

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadataAtContainerCreate(mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
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

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(nil, mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
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

	mockConfig := &docker.Config{Image: "image"}

	mockNetworks := make(map[string]docker.ContainerNetwork)
	mockNetworkSettings := &docker.NetworkSettings{Networks: mockNetworks}

	mockContainer := &docker.Container{Config: mockConfig, NetworkSettings: mockNetworkSettings}

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
	assert.Equal(t, metadata.dockerContainerMetadata.imageName, "image", "Expected nonempty imageID")
}

func TestParseHasNetworkSettingsPortBindings(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockPorts := make(map[docker.Port][]docker.PortBinding)
	mockPortBinding := make([]docker.PortBinding, 0)
	mockPortBinding = append(mockPortBinding, docker.PortBinding{HostIP: "0.0.0.0", HostPort: "8080"})
	mockPorts["80/tcp"] = mockPortBinding

	mockHostConfig := &docker.HostConfig{NetworkMode: "bridge"}
	mockNetworks := make(map[string]docker.ContainerNetwork)
	mockNetworks["bridge"] = docker.ContainerNetwork{}
	mockNetworks["network0"] = docker.ContainerNetwork{}
	mockNetworkSettings := &docker.NetworkSettings{Networks: mockNetworks, Ports: mockPorts}
	mockContainer := &docker.Container{HostConfig: mockHostConfig, NetworkSettings: mockNetworkSettings}

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
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 2, "Expected two networks")

	assert.Equal(t, len(metadata.dockerContainerMetadata.ports), 1, "Expected nonempty list of ports")
	assert.Equal(t, uint16(80), metadata.dockerContainerMetadata.ports[0].ContainerPort, "Expected nonempty ContainerPort field")
	assert.Equal(t, uint16(8080), metadata.dockerContainerMetadata.ports[0].HostPort, "Expected nonempty HostPort field")
	assert.Equal(t, "0.0.0.0", metadata.dockerContainerMetadata.ports[0].BindIP, "Expected nonempty HostIP field")
}

func TestParseHasNetworkSettingsNetworksEmpty(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockHostConfig := &docker.HostConfig{NetworkMode: "bridge"}
	mockNetworkSettings := &docker.NetworkSettings{IPAddress: "0.0.0.0"}
	mockContainer := &docker.Container{HostConfig: mockHostConfig, NetworkSettings: mockNetworkSettings}

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
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 1, "Expected one network")
}

func TestParseHasNetworkSettingsNetworksNonEmpty(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockHostConfig := &docker.HostConfig{NetworkMode: "bridge"}
	mockNetworks := make(map[string]docker.ContainerNetwork)
	mockNetworks["bridge"] = docker.ContainerNetwork{}
	mockNetworks["network0"] = docker.ContainerNetwork{}
	mockNetworkSettings := &docker.NetworkSettings{Networks: mockNetworks}
	mockContainer := &docker.Container{HostConfig: mockHostConfig, NetworkSettings: mockNetworkSettings}

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
	assert.Equal(t, len(metadata.dockerContainerMetadata.networkInfo.networks), 2, "Expected two networks")
}

func TestParseTaskDefinitionSettings(t *testing.T) {
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockCluster := cluster
	mockContainerInstanceARN := containerInstanceARN

	mockHostConfig := &docker.HostConfig{NetworkMode: "bridge"}
	mockConfig := &docker.Config{Image: "image"}
	mockNetworkSettings := &docker.NetworkSettings{IPAddress: "0.0.0.0"}
	mockContainer := &docker.Container{HostConfig: mockHostConfig, Config: mockConfig, NetworkSettings: mockNetworkSettings}

	expectedStatus := string(MetadataReady)

	newManager := &metadataManager{
		cluster:              mockCluster,
		containerInstanceARN: mockContainerInstanceARN,
	}

	metadata := newManager.parseMetadata(mockContainer, mockTask, mockContainerName)
	assert.Equal(t, metadata.cluster, mockCluster, "Expected cluster "+mockCluster)
	assert.Equal(t, metadata.taskMetadata.containerName, mockContainerName, "Expected container name "+mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskARN, mockTaskARN, "Expected task ARN "+mockTaskARN)
	assert.Equal(t, metadata.taskMetadata.taskDefinitionFamily, "", "Expected no task definition family")
	assert.Equal(t, metadata.taskMetadata.taskDefinitionRevision, "", "Expected no task definition revision")
	assert.Equal(t, metadata.containerInstanceARN, mockContainerInstanceARN, "Expected container instance ARN "+mockContainerInstanceARN)
	assert.Equal(t, string(metadata.metadataStatus), expectedStatus, "Expected status "+expectedStatus)

	// now add the task definition details
	mockTaskDefinitionFamily := taskDefinitionFamily
	mockTaskDefinitionRevision := taskDefinitionRevision
	mockTask = &apitask.Task{Arn: mockTaskARN, Family: mockTaskDefinitionFamily, Version: mockTaskDefinitionRevision}
	metadata = newManager.parseMetadata(nil, mockTask, mockContainerName)
	assert.Equal(t, metadata.taskMetadata.taskDefinitionFamily, mockTaskDefinitionFamily, "Expected task definition family "+mockTaskDefinitionFamily)
	assert.Equal(t, metadata.taskMetadata.taskDefinitionRevision, mockTaskDefinitionRevision, "Expected task definition revision "+mockTaskDefinitionRevision)
}
