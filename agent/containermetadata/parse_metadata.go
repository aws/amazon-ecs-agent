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
	dockercontainer "github.com/moby/moby/api/types/container"
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
		hostIPv6Address:        manager.hostIPv6Address,
	}
}

// parseMetadata gathers metadata from a docker container, and task
// configuration and data then packages it for JSON Marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this or the above stage.
func (manager *metadataManager) parseMetadata(dockerContainer *dockercontainer.InspectResponse, task *apitask.Task, containerName string) Metadata {
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
		hostIPv6Address:         manager.hostIPv6Address,
	}
}

// parseDockerContainerMetadata parses the metadata in a docker container
// and packages this data for JSON marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this stage.
func parseDockerContainerMetadata(taskARN string, containerName string, dockerContainer *dockercontainer.InspectResponse) DockerContainerMetadata {
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

	if dockerContainer.HostConfig == nil {
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
func parseNetworkMetadata(settings *dockercontainer.NetworkSettings, hostConfig *dockercontainer.HostConfig) (NetworkMetadata, error) {
	// Network settings and Host configuration should not be missing except due to errors
	if settings == nil {
		err := fmt.Errorf("parse network metadata: could not find network settings")
		return NetworkMetadata{}, err
	}

	if hostConfig == nil {
		err := fmt.Errorf("parse network metadata: could not find host configuration")
		return NetworkMetadata{}, err
	}

	// We get the NetworkMode (Network interface name) from the HostConfig because
	// this is the network with which the container is created.
	networkModeFromHostConfig := string(hostConfig.NetworkMode)

	// moby v29 exposes per-network IP addresses under NetworkSettings.Networks
	// (as netip.Addr); the legacy top-level IPAddress/GlobalIPv6Address fields
	// were removed.
	networkList := make([]tmdsresponse.Network, 0)
	if len(settings.Networks) > 0 {
		for modeFromSettings, containerNetwork := range settings.Networks {
			networkMode := modeFromSettings
			var ipv4Addresses []string
			if containerNetwork.IPAddress.IsValid() {
				ipv4Addresses = []string{containerNetwork.IPAddress.String()}
			}
			var ipv6Addresses []string
			if containerNetwork.GlobalIPv6Address.IsValid() {
				ipv6Addresses = []string{containerNetwork.GlobalIPv6Address.String()}
			}
			network := tmdsresponse.Network{
				NetworkMode:   networkMode,
				IPv4Addresses: ipv4Addresses,
				IPv6Addresses: ipv6Addresses,
			}
			networkList = append(networkList, network)
		}
	} else {
		network := tmdsresponse.Network{
			NetworkMode: networkModeFromHostConfig,
		}
		networkList = append(networkList, network)
	}

	return NetworkMetadata{
		networks: networkList,
	}, nil
}
