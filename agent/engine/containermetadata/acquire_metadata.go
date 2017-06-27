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
	return &NetworkMetadata{
		ports:       portMapping,
		gateway:     settings.Gateway,
		iPAddress:   settings.IPAddress,
		iPv6Gateway: settings.IPv6Gateway,
	}
}

// acquireDockerMetadata parses the metadata in a docker container
// and packages this data for JSON marshaling
func acquireDockerMetadata(container *docker.Container) DockerMetadata {
	if container == nil {
		return DockerMetadata{}
	}
	imageNameFromConfig := ""
	if container.Config != nil {
		imageNameFromConfig = container.Config.Image
	}
	return DockerMetadata{
		status:        container.State.StateString(),
		containerID:   container.ID,
		containerName: container.Name,
		imageID:       container.Image,
		imageName:     imageNameFromConfig,
		networkInfo:   acquireNetworkMetadata(container.NetworkSettings),
	}
}

// acquireAWSMetadata parses metadata in the AWS  configuration and task
// and packages this data for JSON marshaling
func acquireAWSMetadata(cfg *config.Config, task *api.Task) AWSMetadata {
	clusterArnFromConfig := ""
	if cfg != nil {
		clusterArnFromConfig = cfg.Cluster
	}
	taskArnFromConfig := ""
	if task != nil {
		taskArnFromConfig = task.Arn
	}
	return AWSMetadata{
		clusterArn: clusterArnFromConfig,
		taskArn:    taskArnFromConfig,
	}
}

// AcquireStaticMetadata gets the initial metadata that is available before
// container creation, i.e. AWS generated information
func acquireStaticMetadata(cfg *config.Config, task *api.Task) *Metadata {
	awsMD := acquireAWSMetadata(cfg, task)
	return &Metadata{
		clusterArn: awsMD.clusterArn,
		taskArn:    awsMD.taskArn,
	}
}

// AcquireMetadata gathers metadata from a docker container, and task
// configuration and data then packages it for JSON Marshaling
func AcquireMetadata(container *docker.Container, cfg *config.Config, task *api.Task) *Metadata {
	dockerMD := acquireDockerMetadata(container)
	awsMD := acquireAWSMetadata(cfg, task)
	return &Metadata{
		status:        dockerMD.status,
		containerID:   dockerMD.containerID,
		containerName: dockerMD.containerName,
		imageID:       dockerMD.imageID,
		imageName:     dockerMD.imageName,
		clusterArn:    awsMD.clusterArn,
		taskArn:       awsMD.taskArn,
		network:       dockerMD.networkInfo,
	}
}
