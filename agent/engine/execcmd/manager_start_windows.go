package execcmd

import (
	"context"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
)

// Note: exec cmd agent is a linux/windows feature, thus implemented here as a no-op.
func (m *manager) RestartAgentIfStopped(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) (RestartStatus, error) {
	return NotRestarted, nil
}

// Note: exec cmd agent is a linux/windows feature, thus implemented here as a no-op.
func (m *manager) StartAgent(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) error {
	return nil
}
