package handlers

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
)

type TaskProtectionClientFactoryInterface interface {
	NewTaskProtectionClient(taskRoleCredential credentials.TaskIAMRoleCredentials) ecs.ECSTaskProtectionSDK
}
