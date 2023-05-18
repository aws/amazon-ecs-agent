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
	"fmt"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"

	tmdsresponse "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
)

// parseMetadataAtContainerCreate gathers metadata from task and cluster configurations
// then packages it for JSON Marshaling. We use this version to get data
// available prior to container creation
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this or the above stage.
func (manager *metadataManager) parseMetadataAtContainerCreate(task *apitask.Task, containerName string) Metadata {
	return Metadata{
		cluster: manager.cluster,
		taskMetadata: TaskMetadata{
			containerName:          containerName,
			taskARN:                task.Arn,
			taskDefinitionFamily:   task.Family,
			taskDefinitionRevision: task.Version,
		},
		containerInstanceARN:   manager.containerInstanceARN,
		metadataStatus:         MetadataInitial,
		availabilityZone:       manager.availabilityZone,
		hostPrivateIPv4Address: manager.hostPrivateIPv4Address,
		hostPublicIPv4Address:  manager.hostPublicIPv4Address,
	}
}

// parseMetadata gathers metadata from a docker container, and task
// configuration and data then packages it for JSON Marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this or the above stage.
func (manager *metadataManager) parseMetadata(dockerContainer *types.ContainerJSON, task *apitask.Task, containerName string) Metadata {
	dockerMD := parseDockerContainerMetadata(task.Arn, containerName, dockerContainer)
	return Metadata{
		cluster: manager.cluster,
		taskMetadata: TaskMetadata{
			containerName:          containerName,
			taskARN:                task.Arn,
			taskDefinitionFamily:   task.Family,
			taskDefinitionRevision: task.Version,
		},
		dockerContainerMetadata: dockerMD,
		containerInstanceARN:    manager.containerInstanceARN,
		metadataStatus:          MetadataReady,
		availabilityZone:        manager.availabilityZone,
		hostPrivateIPv4Address:  manager.hostPrivateIPv4Address,
		hostPublicIPv4Address:   manager.hostPublicIPv4Address,
	}
}

// parseDockerContainerMetadata parses the metadata in a docker container
// and packages this data for JSON marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this stage.
func parseDockerContainerMetadata(taskARN string, containerName string, dockerContainer *types.ContainerJSON) DockerContainerMetadata {
	if dockerContainer == nil {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: container metadata not available or does not exist", taskARN, containerName)
		return DockerContainerMetadata{}
	}

	// In most cases a container should never lack a config but we check regardless to avoid
	// nil pointer exceptions (Could occur if there is some error in the docker api call, if the
	// container we receive has incomplete information)
	imageNameFromConfig := ""
	if dockerContainer.Config != nil {
		imageNameFromConfig = dockerContainer.Config.Image
	} else {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: container has no configuration", taskARN, containerName)
	}

	if dockerContainer.ContainerJSONBase == nil {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: container has no host configuration", taskARN, containerName)
		return DockerContainerMetadata{
			imageName: imageNameFromConfig,
		}
	}
	networkMetadata, err := parseNetworkMetadata(dockerContainer.NetworkSettings, dockerContainer.HostConfig)

	if err != nil {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: %v", taskARN, containerName, err)
	}

	// Get Port bindings from NetworkSettings
	var ports []apicontainer.PortBinding
	ports, err = apicontainer.PortBindingFromDockerPortBinding(dockerContainer.NetworkSettings.Ports)
	if err != nil {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: %v", taskARN, containerName, err)
	}

	return DockerContainerMetadata{
		containerID:         dockerContainer.ID,
		dockerContainerName: dockerContainer.Name,
		imageID:             dockerContainer.Image,
		imageName:           imageNameFromConfig,
		ports:               ports,
		networkInfo:         networkMetadata,
	}
}

// parseNetworkMetadata parses the docker.NetworkSettings struct and
// packages the desired metadata for JSON marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this stage.
func parseNetworkMetadata(settings *types.NetworkSettings, hostConfig *dockercontainer.HostConfig) (NetworkMetadata, error) {
	// Network settings and Host configuration should not be missing except due to errors
	if settings == nil {
		err := fmt.Errorf("parse network metadata: could not find network settings")
		return NetworkMetadata{}, err
	}

	if hostConfig == nil {
		err := fmt.Errorf("parse network metadata: could not find host configuration")
		return NetworkMetadata{}, err
	}

	// This metadata is the information provided in older versions of the API
	// We get the NetworkMode (Network interface name) from the HostConfig because this
	// this is the network with which the container is created
	ipv4AddressFromSettings := settings.IPAddress
	networkModeFromHostConfig := string(hostConfig.NetworkMode)

	// Extensive Network information is not available for Docker API versions 1.17-1.20
	// Instead we only get the details of the first network
	networkList := make([]tmdsresponse.Network, 0)
	if len(settings.Networks) > 0 {
		for modeFromSettings, containerNetwork := range settings.Networks {
			networkMode := modeFromSettings
			ipv4Addresses := []string{containerNetwork.IPAddress}
			network := tmdsresponse.Network{NetworkMode: networkMode, IPv4Addresses: ipv4Addresses}
			networkList = append(networkList, network)
		}
	} else {
		ipv4Addresses := []string{ipv4AddressFromSettings}
		network := tmdsresponse.Network{NetworkMode: networkModeFromHostConfig, IPv4Addresses: ipv4Addresses}
		networkList = append(networkList, network)
	}

	return NetworkMetadata{
		networks: networkList,
	}, nil
}
