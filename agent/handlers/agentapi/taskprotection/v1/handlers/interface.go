package handlers

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
)

type TaskProtectionClientFactoryInterface interface {
	newTaskProtectionClient(credentialsManager credentials.Manager, task *apitask.Task) (api.ECSTaskProtectionSDK, int, string, error)
}
