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
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/api"

	"github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

// parseNetworkMetadata parses the docker.NetworkSettings struct and
// packages the desired metadata for JSON marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this stage.
func parseNetworkMetadata(settings *docker.NetworkSettings, hostConfig *docker.HostConfig) (NetworkMetadata, error) {
	// Network settings and Host configuration should not be missing except due to errors
	if settings == nil || hostConfig == nil {
		err := fmt.Errorf("parse network metadata: could not find network settings or host configuration")
		return NetworkMetadata{}, err
	}

	// This metadata is the information provided in older versions of the API
	// We get the NetworkMode (Network interface name) from the HostConfig because this
	// this is the network with which the container is created
	ipv4AddressFromSettings := settings.IPAddress
	networkModeFromHostConfig := hostConfig.NetworkMode

	// Extensive Network interface information is not available for Docker API versions 1.17-1.20
	// Instead we only get the details of the first network interface
	networkInterfaceList := make([]Network, 0)
	if len(settings.Networks) > 0 {
		for modeFromSettings, containerNetwork := range settings.Networks {
			networkMode := modeFromSettings
			ipv4Address := containerNetwork.IPAddress
			networkInterface := Network{networkMode, ipv4Address}
			networkInterfaceList = append(networkInterfaceList, networkInterface)
		}
	} else {
		networkInterface := Network{networkModeFromHostConfig, ipv4AddressFromSettings}
		networkInterfaceList = append(networkInterfaceList, networkInterface)
	}

	return NetworkMetadata{
		networks: networkInterfaceList,
	}, nil
}

// parseDockerContainerMetadata parses the metadata in a docker container
// and packages this data for JSON marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this stage.
func parseDockerContainerMetadata(task *api.Task, container *api.Container, dockerContainer *docker.Container) DockerContainerMetadata {
	if container == nil {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: container metadata not available or does not exist", task, container)
		return DockerContainerMetadata{}
	}

	// In most cases a container should never lack a config but we check regardless to avoid
	// nil pointer exceptions (Could occur if there is some error in the docker api call, if the
	// container we receive has incomplete information)
	imageNameFromConfig := ""
	if dockerContainer.Config != nil {
		imageNameFromConfig = dockerContainer.Config.Image
	} else {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: container has no configuration", task, container)
	}

	// Get Port bindings from docker configurations
	var ports []api.PortBinding
	var err error
	if dockerContainer.HostConfig != nil {
		ports, err = api.PortBindingFromDockerPortBinding(dockerContainer.HostConfig.PortBindings)
		if err != nil {
			seelog.Warnf("Failed to parse container metadata for task %s container %s: %v", task, container, err)
		}
	} else {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: container has no host configuration", task, container)
	}

	networkMetadata, err := parseNetworkMetadata(dockerContainer.NetworkSettings, dockerContainer.HostConfig)
	if err != nil {
		seelog.Warnf("Failed to parse container metadata for task %s container %s: %v", task, container, err)
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

// parseTaskMetadata parses  in the AWS configuration and task
// and packages this data for JSON marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this stage.
func parseTaskMetadata(task *api.Task, container *api.Container) TaskMetadata {
	containerName := container.Name

	taskARNFromConfig := task.Arn

	return TaskMetadata{
		containerName: containerName,
		taskARN:       taskARNFromConfig,
	}
}

// parseMetadata gathers metadata from a docker container, and task
// configuration and data then packages it for JSON Marshaling
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this or the above stage.
func (manager *metadataManager) parseMetadata(dockerContainer *docker.Container, task *api.Task, container *api.Container) Metadata {
	taskMD := parseTaskMetadata(task, container)
	dockerMD := parseDockerContainerMetadata(task, container, dockerContainer)
	return Metadata{
		cluster:                 manager.cluster,
		taskMetadata:            taskMD,
		dockerContainerMetadata: dockerMD,
		containerInstanceARN:    manager.containerInstanceARN,
		metadataStatus:          "READY",
	}
}

// parseMetadataAtContainerCreate gathers metadata from task and cluster configurations
// then packages it for JSON Marshaling. We use this version to get data
// available prior to container creation
// Since we accept incomplete metadata fields, we should not return
// errors here and handle them at this or the above stage.
func (manager *metadataManager) parseMetadataAtContainerCreate(task *api.Task, container *api.Container) Metadata {
	taskMD := parseTaskMetadata(task, container)
	return Metadata{
		cluster:              manager.cluster,
		taskMetadata:         taskMD,
		containerInstanceARN: manager.containerInstanceARN,
		metadataStatus:       "NOT_READY",
	}
}
