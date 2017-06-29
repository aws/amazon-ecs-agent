package containermetadata

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	docker "github.com/fsouza/go-dockerclient"
)

// acquireNetworkMetadata parses the docker.NetworkSettings struct and
// packages the desired metadata for JSON marshaling
func acquireNetworkMetadata(settings *docker.NetworkSettings) NetworkMetadata {
	if settings == nil {
		return NetworkMetadata{}
	}

	// Scan all mapped ports
	portMapping := make([]PortMapping, 0)
	for port, bind := range settings.Ports {
		containerPort := port.Port()
		protocol := port.Proto()
		for index := range bind {
			hostIP := bind[index].HostIP
			hostPort := bind[index].HostPort
			portMap := PortMapping{
				ContainerPort: containerPort,
				HostPort:      hostPort,
				BindIP:        hostIP,
				Protocol:      protocol,
			}
			portMapping = append(portMapping, portMap)
		}
	}

	// This metadata is available in two different places in NetworkSettings
	// Since a container should have only one network mode this metadata
	// should be identical in the two locations but maybe there are other
	// use cases?
	gateway := settings.Gateway
	iPAddress := settings.IPAddress
	iPv6Gateway := settings.IPv6Gateway

	// Assume there is at most one network mode (And if none, network is "none")
	networkModeFromContainer := ""
	if len(settings.Networks) == 1 {
		for modeFromSettings, containerNetwork := range settings.Networks {
			networkModeFromContainer = modeFromSettings
			gateway = containerNetwork.Gateway
			iPAddress = containerNetwork.IPAddress
			iPv6Gateway = containerNetwork.IPv6Gateway
		}
	} else if len(settings.Networks) == 0 {
		networkModeFromContainer = "none"
	}
	return NetworkMetadata{
		ports:       portMapping,
		networkMode: networkModeFromContainer,
		gateway:     gateway,
		iPAddress:   iPAddress,
		iPv6Gateway: iPv6Gateway,
	}
}

// acquireDockerContainerMetadata parses the metadata in a docker container
// and packages this data for JSON marshaling
func acquireDockerContainerMetadata(container *docker.Container) DockerContainerMD {
	if container == nil {
		return DockerContainerMD{}
	}
	imageNameFromConfig := ""
	if container.Config != nil {
		imageNameFromConfig = container.Config.Image
	}
	return DockerContainerMD{
		status:        container.State.StateString(),
		containerID:   container.ID,
		containerName: container.Name,
		imageID:       container.Image,
		imageName:     imageNameFromConfig,
		networkInfo:   acquireNetworkMetadata(container.NetworkSettings),
	}
}

// acquireTaskMetadata parses metadata in the AWS configuration and task
// and packages this data for JSON marshaling
func acquireTaskMetadata(client dockerDummyClient, cfg *config.Config, task *api.Task) TaskMetadata {
	// Get docker version from client. May block metadata file updates so the file changes
	// should be made in a goroutine as this does a docker client call
	version, err := client.Version()
	if err != nil {
		version = ""
	}

	clusterArnFromConfig := ""
	if cfg != nil {
		clusterArnFromConfig = cfg.Cluster
	}
	taskArnFromConfig := ""
	if task != nil {
		taskArnFromConfig = task.Arn
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
	dockerMD := acquireDockerContainerMetadata(container)
	taskMD := acquireTaskMetadata(client, cfg, task)
	return Metadata{
		version:       taskMD.version,
		status:        dockerMD.status,
		containerID:   dockerMD.containerID,
		containerName: dockerMD.containerName,
		imageID:       dockerMD.imageID,
		imageName:     dockerMD.imageName,
		clusterArn:    taskMD.clusterArn,
		taskArn:       taskMD.taskArn,
		ports:         dockerMD.networkInfo.ports,
		networkMode:   dockerMD.networkInfo.networkMode,
		gateway:       dockerMD.networkInfo.gateway,
		iPAddress:     dockerMD.networkInfo.iPAddress,
		iPv6Gateway:   dockerMD.networkInfo.iPv6Gateway,
	}
}

// acquireMetadataAtContainerCreate gathers metadata from task and cluster configurations
// then packages it for JSON Marshaling. We use this version to get data
// available prior to container creation
func acquireMetadataAtContainerCreate(client dockerDummyClient, cfg *config.Config, task *api.Task) Metadata {
	return acquireMetadata(client, nil, cfg, task)
}
