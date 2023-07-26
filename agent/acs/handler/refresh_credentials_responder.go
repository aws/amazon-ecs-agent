package handler

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
)

var (
	// For ease of unit testing
	checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl = checkAndSetDomainlessGMSATaskExecutionRoleCredentials
)

// credentialsMetadataSetter implements CredentialsMetadataSetter
type credentialsMetadataSetter struct {
	taskEngine engine.TaskEngine
}

func (cmSetter *credentialsMetadataSetter) SetTaskRoleMetadata(
	message *ecsacs.IAMRoleCredentialsMessage) error {
	task, err := cmSetter.getCredentialsMessageTask(message)
	if err != nil {
		return err
	}
	task.SetCredentialsID(aws.StringValue(message.RoleCredentials.CredentialsId))
	return nil
}

func (cmSetter *credentialsMetadataSetter) SetExecRoleMetadata(
	message *ecsacs.IAMRoleCredentialsMessage) error {
	task, err := cmSetter.getCredentialsMessageTask(message)
	if err != nil {
		return errors.Wrap(err, "unable to get credentials message's task")
	}
	task.SetExecutionRoleCredentialsID(aws.StringValue(message.RoleCredentials.CredentialsId))

	// Refresh domainless gMSA plugin credentials if needed.
	err = checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl(credentials.IAMRoleCredentialsFromACS(
		message.RoleCredentials, aws.StringValue(message.RoleType)), task)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("unable to set %s for task with ARN %s",
			"DomainlessGMSATaskExecutionRoleCredentials", aws.StringValue(message.TaskArn)))
	}

	return nil
}

func (cmSetter *credentialsMetadataSetter) getCredentialsMessageTask(
	message *ecsacs.IAMRoleCredentialsMessage) (*apitask.Task, error) {
	taskARN := aws.StringValue(message.TaskArn)
	messageID := aws.StringValue(message.MessageId)
	task, ok := cmSetter.taskEngine.GetTaskByArn(taskARN)
	if !ok {
		return nil, errors.Errorf(
			"Task not found in the task engine for task ARN %s from credentials message with message ID %s",
			taskARN, messageID)
	}
	return task, nil
}
