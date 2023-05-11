package handlers

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
)

type TaskProtectionClientFactoryInterface interface {
	newTaskProtectionClient(taskRoleCredential credentials.TaskIAMRoleCredentials) api.ECSTaskProtectionSDK
}
