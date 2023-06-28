package handlers

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/api"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
)

type TaskProtectionClientFactoryInterface interface {
	NewTaskProtectionClient(taskRoleCredential credentials.TaskIAMRoleCredentials) api.ECSTaskProtectionSDK
}
