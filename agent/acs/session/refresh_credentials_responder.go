// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package session

import (
	"fmt"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acs"
	"github.com/pkg/errors"
)

var (
	// For ease of unit testing
	checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl = checkAndSetDomainlessGMSATaskExecutionRoleCredentials
)

// credentialsMetadataSetter struct implements CredentialsMetadataSetter interface defined in ecs-agent module.
type credentialsMetadataSetter struct {
	taskEngine engine.TaskEngine
}

// NewCredentialsMetadataSetter creates a new credentialsMetadataSetter.
func NewCredentialsMetadataSetter(taskEngine engine.TaskEngine) *credentialsMetadataSetter {
	return &credentialsMetadataSetter{
		taskEngine: taskEngine,
	}
}

func (cmSetter *credentialsMetadataSetter) SetTaskRoleCredentialsMetadata(
	message *acs.RefreshTaskIAMRoleCredentialsInput) error {
	task, err := cmSetter.getCredentialsMessageTask(message)
	if err != nil {
		return err
	}
	task.SetCredentialsID(aws.ToString(message.RoleCredentials.CredentialsId))
	return nil
}

func (cmSetter *credentialsMetadataSetter) SetExecRoleCredentialsMetadata(
	message *acs.RefreshTaskIAMRoleCredentialsInput) error {
	task, err := cmSetter.getCredentialsMessageTask(message)
	if err != nil {
		return errors.Wrap(err, "unable to get credentials message's task")
	}
	task.SetExecutionRoleCredentialsID(aws.ToString(message.RoleCredentials.CredentialsId))

	// Refresh domainless gMSA plugin credentials if needed.
	err = checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl(credentials.IAMRoleCredentialsFromACS(
		message.RoleCredentials, string(message.RoleType)), task)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("unable to set %s for task with ARN %s",
			"DomainlessGMSATaskExecutionRoleCredentials", aws.ToString(message.TaskArn)))
	}

	return nil
}

func (cmSetter *credentialsMetadataSetter) getCredentialsMessageTask(
	message *acs.RefreshTaskIAMRoleCredentialsInput) (*apitask.Task, error) {
	taskARN := aws.ToString(message.TaskArn)
	messageID := aws.ToString(message.MessageId)
	task, ok := cmSetter.taskEngine.GetTaskByArn(taskARN)
	if !ok {
		return nil, errors.Errorf(
			"Task not found in the task engine for task ARN %s from credentials message with message ID %s",
			taskARN, messageID)
	}
	return task, nil
}
