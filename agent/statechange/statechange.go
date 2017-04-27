package statechange

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
)

type StateChangeEvent struct {
	TaskEvent      *api.TaskStateChange
	ContainerEvent *api.ContainerStateChange
}
