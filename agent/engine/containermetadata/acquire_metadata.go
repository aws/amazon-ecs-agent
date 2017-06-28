package containermetadata

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	docker "github.com/fsouza/go-dockerclient"
)

// acquireNetworkMetadata parses the docker.NetworkSettings struct and
// packages the desired metadata for JSON marshaling
func acquireNetworkMetadata(settings *docker.NetworkSettings) *NetworkMetadata {
	if settings == nil {
		return nil
	}
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
	networkModeFromContainer := ""
	gateway := settings.Gateway
	iPAddress := settings.IPAddress
	iPv6Gateway := settings.IPv6Gateway
	// Assume there is at most one network mode (And if none, network is "None")
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
	return &NetworkMetadata{
		ports:       portMapping,
		networkMode: networkModeFromContainer,
		gateway:     gateway,
		iPAddress:   iPAddress,
		iPv6Gateway: iPv6Gateway,
	}
}

// acquireDockerContainerMetadata parses the metadata in a docker container
// and packages this data for JSON marshaling
func acquireDockerContainerMetadata(container *docker.Container) DockerContainerMetadata {
	if container == nil {
		return DockerContainerMetadata{}
	}
	imageNameFromConfig := ""
	if container.Config != nil {
		imageNameFromConfig = container.Config.Image
	}
	return DockerContainerMetadata{
		status:        container.State.StateString(),
		containerID:   container.ID,
		containerName: container.Name,
		imageID:       container.Image,
		imageName:     imageNameFromConfig,
		networkInfo:   acquireNetworkMetadata(container.NetworkSettings),
	}
}

// acquireTaskStaticMetadata parses metadata in the AWS configuration and task
// and packages this data for JSON marshaling
func acquireTaskStaticMetadata(cfg *config.Config, task *api.Task) TaskStaticMetadata {
	clusterArnFromConfig := ""
	if cfg != nil {
		clusterArnFromConfig = cfg.Cluster
	}
	taskArnFromConfig := ""
	if task != nil {
		taskArnFromConfig = task.Arn
	}
	return TaskStaticMetadata{
		clusterArn: clusterArnFromConfig,
		taskArn:    taskArnFromConfig,
	}
}

// AcquireStaticMetadata gets the initial metadata that is available before
// container creation, i.e. AWS generated information
func acquireStaticMetadata(cfg *config.Config, task *api.Task) *Metadata {
	awsMD := acquireTaskStaticMetadata(cfg, task)
	return &Metadata{
		clusterArn: awsMD.clusterArn,
		taskArn:    awsMD.taskArn,
	}
}

// AcquireMetadata gathers metadata from a docker container, and task
// configuration and data then packages it for JSON Marshaling
func AcquireMetadata(container *docker.Container, cfg *config.Config, task *api.Task) *Metadata {
	dockerMD := acquireDockerContainerMetadata(container)
	taskMD := acquireTaskStaticMetadata(cfg, task)
	return &Metadata{
		status:        dockerMD.status,
		containerID:   dockerMD.containerID,
		containerName: dockerMD.containerName,
		imageID:       dockerMD.imageID,
		imageName:     dockerMD.imageName,
		clusterArn:    taskMD.clusterArn,
		taskArn:       taskMD.taskArn,
		network:       dockerMD.networkInfo,
	}
}
