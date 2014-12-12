package engine

import "fmt"
import "github.com/aws/amazon-ecs-agent/agent/api"

type ContainerNotFound struct {
	TaskArn       string
	ContainerName string
}

func (cnferror ContainerNotFound) Error() string {
	return fmt.Sprintf("Could not find container '%s' in task '%s'", cnferror.ContainerName, cnferror.TaskArn)
}

type DockerContainerChangeEvent struct {
	DockerId string
	Image    string
	Status   api.ContainerStatus
}
