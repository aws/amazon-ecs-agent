package engine

import "github.com/aws/amazon-ecs-agent/agent/api"

type TaskEngine interface {
	TaskEvents() (<-chan api.ContainerStateChange, <-chan error)

	AddTask(*api.Task)

	ListTasks() ([]*api.Task, error)
}
