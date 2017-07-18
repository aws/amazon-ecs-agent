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
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"

	"github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

// acquireNetworkMetadata parses the docker.NetworkSettings struct and
// packages the desired metadata for JSON marshaling
func acquireNetworkMetadata(settings *docker.NetworkSettings) NetworkMetadata {
	if settings == nil {
		seelog.Errorf("Failed to acquire network metadata: Network metadata not available or does not exist")
		return NetworkMetadata{}
	}

	// This metadata is available in two different places in NetworkSettings
	ipv4Address := settings.IPAddress
	ipv4Gateway := settings.Gateway
	ipv6Address := settings.GlobalIPv6Address
	ipv6Gateway := settings.IPv6Gateway

	// Network mode is not available for Docker API versions 1.17-1.20
	networkModeFromContainer := ""
	if len(settings.Networks) == 1 {
		for modeFromSettings, containerNetwork := range settings.Networks {
			networkModeFromContainer = modeFromSettings
			ipv4Address = containerNetwork.IPAddress
			ipv4Gateway = containerNetwork.Gateway
			ipv6Address = containerNetwork.GlobalIPv6Address
			ipv6Gateway = containerNetwork.IPv6Gateway
		}
	} else {
		seelog.Debugf("No network mode found due to old Docker Client version")
	}
	networkMD := NetworkMetadata{
		networkMode: networkModeFromContainer,
		ipv4Address: ipv4Address,
		ipv4Gateway: ipv4Gateway,
		ipv6Address: ipv6Address,
		ipv6Gateway: ipv6Gateway,
	}
	return networkMD
}

// acquireDockerContainerMetadata parses the metadata in a docker container
// and packages this data for JSON marshaling
func acquireDockerContainerMetadata(container *docker.Container) DockerContainerMD {
	if container == nil {
		seelog.Errorf("Failed to acquire container metadata: Container metadata not available or does not exist")
		return DockerContainerMD{}
	}

	imageNameFromConfig := ""
	// A real world container should never lack a config but we check regardless to avoid
	// nil pointer exceptions (Could occur if there is some error in the docker api call)
	if container.Config != nil {
		imageNameFromConfig = container.Config.Image
	} else {
		seelog.Errorf("Failed to acquire container metadata: container has no configuration")
		return DockerContainerMD{}
	}

	// Get Port bindings from docker configurations
	var ports []api.PortBinding
	var err error
	// A real world container should never lack a host config but we check just in case
	if container.HostConfig != nil {
		ports, err = api.PortBindingFromDockerPortBinding(container.HostConfig.PortBindings)
		if err != nil {
			seelog.Errorf("Failed to acquire port binding metadata: %v", err)
		}
	} else {
		seelog.Errorf("Failed ot acquire container metadata: container has no host configuration")
		return DockerContainerMD{}
	}

	networkMD := acquireNetworkMetadata(container.NetworkSettings)

	dockerContainerMD := DockerContainerMD{
		status:        container.State.StateString(),
		containerID:   container.ID,
		containerName: container.Name,
		imageID:       container.Image,
		imageName:     imageNameFromConfig,
		ports:         ports,
		networkInfo:   networkMD,
	}
	return dockerContainerMD
}

// acquireTaskMetadata parses metadata in the AWS configuration and task
// and packages this data for JSON marshaling
func acquireTaskMetadata(client dockerDummyClient, cfg *config.Config, task *api.Task) TaskMetadata {
	// Get docker version from client. May block metadata file updates so the file changes
	// should be made in a goroutine as this does a docker client call
	version, err := client.Version()
	if err != nil {
		version = ""
		seelog.Errorf("Failed to get docker version number: %v", err)
	}

	clusterArnFromConfig := ""
	if cfg != nil {
		clusterArnFromConfig = cfg.Cluster
	} else {
		// This error should not happen in real world use cases (Or occurs somewhere else long
		// before this should matter)
		seelog.Errorf("Failed to get cluster Arn: Invalid configuration")
	}

	taskArnFromConfig := ""
	if task != nil {
		taskArnFromConfig = task.Arn
	} else {
		// This error should not happen in real world use cases (Or occurs somewhere else long
		// before this should matter)
		seelog.Errorf("Failed to get task Arn: Invalid task")
	}

	return TaskMetadata{
		version:    version,
		clusterArn: clusterArnFromConfig,
		taskArn:    taskArnFromConfig,
	}
}

// acquireMetadata gathers metadata from a docker container, and task
// configuration and data then packages it for JSON Marshaling
func acquireMetadata(client dockerDummyClient, container *docker.Container, cfg *config.Config, task *api.Task) Metadata {
	taskMD := acquireTaskMetadata(client, cfg, task)
	dockerMD := acquireDockerContainerMetadata(container)
	return Metadata{
		Version:       taskMD.version,
		Status:        dockerMD.status,
		ContainerID:   dockerMD.containerID,
		ContainerName: dockerMD.containerName,
		ImageID:       dockerMD.imageID,
		ImageName:     dockerMD.imageName,
		ClusterArn:    taskMD.clusterArn,
		TaskArn:       taskMD.taskArn,
		Ports:         dockerMD.ports,
		NetworkMode:   dockerMD.networkInfo.networkMode,
		IPv4Address:   dockerMD.networkInfo.ipv4Address,
		IPv4Gateway:   dockerMD.networkInfo.ipv4Gateway,
		IPv6Address:   dockerMD.networkInfo.ipv6Address,
		IPv6Gateway:   dockerMD.networkInfo.ipv6Gateway,
	}
}

// acquireMetadataAtContainerCreate gathers metadata from task and cluster configurations
// then packages it for JSON Marshaling. We use this version to get data
// available prior to container creation
func acquireMetadataAtContainerCreate(client dockerDummyClient, cfg *config.Config, task *api.Task) Metadata {
	taskMD := acquireTaskMetadata(client, cfg, task)
	return Metadata{
		Version:    taskMD.version,
		ClusterArn: taskMD.clusterArn,
		TaskArn:    taskMD.taskArn,
	}
}
