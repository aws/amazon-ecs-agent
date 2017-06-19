package metadataservice

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	docker "github.com/fsouza/go-dockerclient"
)

//Main metadata gathering function.
func AcquireMetadata(container *docker.Container, cfg *config.Config, task *api.Task) *Metadata {
	dockermd := acquireDockerMetadata(container)
	awsmd := acquireAWSMetadata(cfg, task)
	return &Metadata {
		Status        : dockermd.status,
		ContainerID   : dockermd.containerID,
		ContainerName : dockermd.containerName,
		ImageID       : dockermd.imageID,
		ImageName     : dockermd.imageName,
		ClusterArn    : awsmd.clusterArn,
		TaskArn       : awsmd.taskArn,
	}
}

func acquireDockerMetadata(container *docker.Container) DockerMetadata {
	return DockerMetadata {
		status        : container.State.String(),
		containerID   : container.ID,
		containerName : container.Name,
		imageID       : container.Image,
		imageName     : container.Config.Image,
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
