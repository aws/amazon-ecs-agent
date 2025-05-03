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

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
)

const (
	cluster = "us-west2"
)

type testFixture struct {
	task           *apitask.Task
	container      *types.ContainerJSON
	manager        *metadataManager
	expectedStatus string
}

func newBasicFixture() *testFixture {
	mockTask := &apitask.Task{Arn: validTaskARN}

	manager := &metadataManager{
		cluster:                cluster,
		containerInstanceARN:   containerInstanceARN,
		availabilityZone:       availabilityZone,
		hostPrivateIPv4Address: hostPrivateIPv4Address,
		hostPublicIPv4Address:  hostPublicIPv4Address,
	}

	return &testFixture{
		task:           mockTask,
		manager:        manager,
		expectedStatus: string(MetadataReady),
	}
}

// adds a container with a basic Config and an empty NetworkingSettings
func (f *testFixture) withContainer() *testFixture {
	f.container = &types.ContainerJSON{
		Config:          &dockercontainer.Config{Image: "image"},
		NetworkSettings: &types.NetworkSettings{},
	}
	return f
}

// adds a host config to the container
func (f *testFixture) withHostConfig() *testFixture {
	if f.container == nil {
		f.withContainer()
	}
	f.container.ContainerJSONBase = &types.ContainerJSONBase{HostConfig: &dockercontainer.HostConfig{NetworkMode: "bridge"}}
	return f
}

// ensures container and host config are properly initialized
func (f *testFixture) initContainerAndHostConfig() *testFixture {
	if f.container == nil {
		f.withContainer()
	}

	if f.container.ContainerJSONBase == nil {
		f.withHostConfig()
	}

	return f
}

// adds default network settings to the container NetworkSettings
func (f *testFixture) withDefaultNetworkSettings(ipv4 string, ipv6 string) *testFixture {
	f.initContainerAndHostConfig()
	f.container.NetworkSettings.DefaultNetworkSettings = types.DefaultNetworkSettings{
		IPAddress:         ipv4,
		GlobalIPv6Address: ipv6,
	}

	return f
}

// adds specified networks to the container NetworkSettings
func (f *testFixture) withNetworks(networks map[string]*network.EndpointSettings) *testFixture {
	f.initContainerAndHostConfig()
	f.container.NetworkSettings.Networks = networks
	return f
}

// withPorts adds port bindings to the container NetworkSettings
func (f *testFixture) withPorts(ports nat.PortMap) *testFixture {
	f.initContainerAndHostConfig()
	f.container.NetworkSettings.Ports = ports
	return f
}

// Helper function for standard validations
func validateBasicMetadata(t *testing.T, metadata *Metadata, fixture *testFixture) {
	assert.Equal(t, fixture.manager.cluster, metadata.cluster, "Expected cluster "+fixture.manager.cluster)
	assert.Equal(t, containerName, metadata.taskMetadata.containerName, "Expected container name "+containerName)
	assert.Equal(t, fixture.task.Arn, metadata.taskMetadata.taskARN, "Expected task ARN "+fixture.task.Arn)
	assert.Equal(t, fixture.manager.containerInstanceARN, metadata.containerInstanceARN, "Expected container instance ARN "+fixture.manager.containerInstanceARN)
	assert.Equal(t, fixture.manager.availabilityZone, metadata.availabilityZone, "Expected availabilityZone "+fixture.manager.availabilityZone)
	assert.Equal(t, fixture.manager.hostPrivateIPv4Address, metadata.hostPrivateIPv4Address, "Expected hostPrivateIPv4Address "+fixture.manager.hostPrivateIPv4Address)
	assert.Equal(t, fixture.manager.hostPublicIPv4Address, metadata.hostPublicIPv4Address, "Expected hostPublicIPv4Address "+fixture.manager.hostPublicIPv4Address)
	assert.Equal(t, fixture.expectedStatus, string(metadata.metadataStatus), "Expected status "+fixture.expectedStatus)
}

// TestParseContainerCreate checks case when parsing is done at metadata creation
func TestParseContainerCreate(t *testing.T) {
	fixture := newBasicFixture()
	fixture.task = &apitask.Task{
		Arn:     validTaskARN,
		Family:  taskDefinitionFamily,
		Version: taskDefinitionRevision,
	}
	fixture.expectedStatus = string(MetadataInitial)
	metadata := fixture.manager.parseMetadataAtContainerCreate(fixture.task, containerName)

	validateBasicMetadata(t, &metadata, fixture)
	assert.Equal(t, taskDefinitionFamily, metadata.taskMetadata.taskDefinitionFamily, "Expected task definition family "+taskDefinitionFamily)
	assert.Equal(t, taskDefinitionRevision, metadata.taskMetadata.taskDefinitionRevision, "Expected task definition revision "+taskDefinitionRevision)
}

func TestParseHasNoContainer(t *testing.T) {
	fixture := newBasicFixture()
	metadata := fixture.manager.parseMetadata(nil, fixture.task, containerName)

	validateBasicMetadata(t, &metadata, fixture)
	assert.Equal(t, "", metadata.dockerContainerMetadata.containerID, "Expected empty container metadata")
	assert.Equal(t, "", metadata.dockerContainerMetadata.dockerContainerName, "Expected empty container metadata")
	assert.Equal(t, "", metadata.dockerContainerMetadata.imageID, "Expected empty container metadata")
	assert.Equal(t, "", metadata.dockerContainerMetadata.imageName, "Expected empty container metadata")
}

func TestParseHasConfig(t *testing.T) {
	mockNetworks := map[string]*network.EndpointSettings{}

	fixture := newBasicFixture().withContainer().withNetworks(mockNetworks)
	metadata := fixture.manager.parseMetadata(fixture.container, fixture.task, containerName)

	validateBasicMetadata(t, &metadata, fixture)
	assert.Equal(t, "image", metadata.dockerContainerMetadata.imageName, "Expected nonempty imageID")
}

func TestParseHasNetworkSettingsPortBindings(t *testing.T) {
	mockNetworks := map[string]*network.EndpointSettings{}
	mockNetworks["bridge"] = &network.EndpointSettings{}
	mockNetworks["network0"] = &network.EndpointSettings{}

	mockPorts := nat.PortMap{}
	mockPortBinding := make([]nat.PortBinding, 0)
	mockPortBinding = append(mockPortBinding, nat.PortBinding{HostIP: "0.0.0.0", HostPort: "8080"})
	mockPorts["80/tcp"] = mockPortBinding

	fixture := newBasicFixture().withNetworks(mockNetworks).withPorts(mockPorts)
	metadata := fixture.manager.parseMetadata(fixture.container, fixture.task, containerName)

	validateBasicMetadata(t, &metadata, fixture)
	assert.Equal(t, 2, len(metadata.dockerContainerMetadata.networkInfo.networks), "Expected two networks")
	assert.Equal(t, 1, len(metadata.dockerContainerMetadata.ports), "Expected nonempty list of ports")
	assert.Equal(t, uint16(80), metadata.dockerContainerMetadata.ports[0].ContainerPort, "Expected nonempty ContainerPort field")
	assert.Equal(t, uint16(8080), metadata.dockerContainerMetadata.ports[0].HostPort, "Expected nonempty HostPort field")
	assert.Equal(t, "0.0.0.0", metadata.dockerContainerMetadata.ports[0].BindIP, "Expected nonempty HostIP field")
}

func TestParseHasNetworkSettingsNetworksEmpty(t *testing.T) {
	fixture := newBasicFixture().withDefaultNetworkSettings("1.2.3.4", "5:6:7:8::")
	metadata := fixture.manager.parseMetadata(fixture.container, fixture.task, containerName)

	validateBasicMetadata(t, &metadata, fixture)
	// Networks assertions
	networks := metadata.dockerContainerMetadata.networkInfo.networks
	assert.Equal(t, len(networks), 1, "Expected one networks")
	assert.Equal(t, "bridge", networks[0].NetworkMode)
	assert.Equal(t, "1.2.3.4", networks[0].IPv4Addresses[0])
	assert.Equal(t, "5:6:7:8::", networks[0].IPv6Addresses[0])
}

func TestParseHasNetworkSettingsNetworksNonEmpty(t *testing.T) {
	mockNetworks := map[string]*network.EndpointSettings{}
	mockNetworks["bridge"] = &network.EndpointSettings{IPAddress: "1.2.3.4", GlobalIPv6Address: "5:6:7:8::"}
	mockNetworks["network0"] = &network.EndpointSettings{}

	fixture := newBasicFixture().withNetworks(mockNetworks)
	metadata := fixture.manager.parseMetadata(fixture.container, fixture.task, containerName)

	validateBasicMetadata(t, &metadata, fixture)
	// Networks assertions
	networks := metadata.dockerContainerMetadata.networkInfo.networks
	assert.Equal(t, len(networks), 2, "Expected two networks")
	assert.Equal(t, "bridge", networks[0].NetworkMode)
	assert.Equal(t, "1.2.3.4", networks[0].IPv4Addresses[0])
	assert.Equal(t, "5:6:7:8::", networks[0].IPv6Addresses[0])
}

func TestParseHasNoContainerJSONBase(t *testing.T) {
	fixture := newBasicFixture().withDefaultNetworkSettings("0.0.0.0", "")
	fixture.container.ContainerJSONBase = nil
	metadata := fixture.manager.parseMetadata(fixture.container, fixture.task, containerName)

	validateBasicMetadata(t, &metadata, fixture)
	// nil ContainerJSONBase means that hostConfig will be nil too; thus expect no networks
	assert.Equal(t, 0, len(metadata.dockerContainerMetadata.networkInfo.networks), "Expected zero networks")
	assert.Equal(t, "image", metadata.dockerContainerMetadata.imageName)
}

func TestParseTaskDefinitionSettings(t *testing.T) {
	mockNetworkSettings := &types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{
			LinkLocalIPv6Address: "0.0.0.0",
		},
	}

	fixture := newBasicFixture().withHostConfig()
	fixture.container.NetworkSettings = mockNetworkSettings
	metadata := fixture.manager.parseMetadata(fixture.container, fixture.task, containerName)

	validateBasicMetadata(t, &metadata, fixture)
	assert.Equal(t, "", metadata.taskMetadata.taskDefinitionFamily, "Expected no task definition family")
	assert.Equal(t, "", metadata.taskMetadata.taskDefinitionRevision, "Expected no task definition revision")

	// now add the task definition details
	mockTask := &apitask.Task{Arn: validTaskARN, Family: taskDefinitionFamily, Version: taskDefinitionRevision}
	metadata = fixture.manager.parseMetadata(fixture.container, mockTask, containerName)
	assert.Equal(t, taskDefinitionFamily, metadata.taskMetadata.taskDefinitionFamily, "Expected task definition family")
	assert.Equal(t, taskDefinitionRevision, metadata.taskMetadata.taskDefinitionRevision, "Expected task definition revision")
}
