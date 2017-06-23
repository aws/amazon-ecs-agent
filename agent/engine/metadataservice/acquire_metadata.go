package metadataservice

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	docker "github.com/fsouza/go-dockerclient"
)

func acquireNetworkMetadata(settings *docker.NetworkSettings) *NetworkMetadata {
	if settings == nil {
		return nil
	}
	ports := make(map[string]PortMapping)
	for pt, bind := range settings.Ports {
		containerPort := pt.Port()
		protocol := pt.Proto()
		//Assume have a single port bound to container port
		if len(bind) > 1 {
			continue
		}
		hostIP := bind[0].HostIP
		hostPort := bind[0].HostPort
		portMap := PortMapping {
			ContainerPort : containerPort,
			HostPort      : hostPort,
			BindIP        : hostIP,
			Protocol      : protocol,
		}
		ports[containerPort] = portMap
	}
	return &NetworkMetadata {
		ports       : ports,
		gateway     : settings.Gateway,
		iPAddress   : settings.IPAddress,
		iPv6Gateway : settings.IPv6Gateway,
	}
}

func acquireDockerMetadata(container *docker.Container) DockerMetadata {
	if container == nil {
		return DockerMetadata{}
	}
	imName := ""
	if (container.Config != nil) {
		imName = container.Config.Image
	}
//TODO: Add network metadata nil pointer handler and add it to dockermetadata	
	return DockerMetadata {
		status        : container.State.StateString(),
		containerID   : container.ID,
		containerName : container.Name,
		imageID       : container.Image,
		imageName     : imName,
	}
}

func acquireAWSMetadata(cfg *config.Config, task *api.Task) AWSMetadata {
	cluster_arn := ""
	if cfg != nil {
		cluster_arn = cfg.Cluster
	}
	task_arn := ""
	if task != nil {
		task_arn = task.Arn
	}
	return AWSMetadata {
		clusterArn : cluster_arn,
		taskArn    : task_arn,
	}
}
//AcquireMetadata gathers metadata from inputs and packages it for JSON Marshaling
func AcquireMetadata(container *docker.Container, cfg *config.Config, task *api.Task) *Metadata {
	dockermd := acquireDockerMetadata(container)
	awsmd := acquireAWSMetadata(cfg, task)
	return &Metadata {
		status        : dockermd.status,
		containerID   : dockermd.containerID,
		containerName : dockermd.containerName,
		imageID       : dockermd.imageID,
		imageName     : dockermd.imageName,
		clusterArn    : awsmd.clusterArn,
		taskArn       : awsmd.taskArn,
		network       : dockermd.networkInfo,
	}
}
