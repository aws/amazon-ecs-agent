//go:build linux
// +build linux

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
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
)

func checkAndSetDomainlessGMSATaskExecutionRoleCredentials(iamRoleCredentials credentials.IAMRoleCredentials, task *task.Task) error {
	// exit early if the task does not need domainless gMSA
	if !task.RequiresDomainlessCredentialSpecResource() {
		return nil
	}
	credspecContainerMapping := task.GetAllCredentialSpecRequirements()
	credentialspecResource, err := credentialspec.NewCredentialSpecResource(task.Arn, "", task.ExecutionCredentialsID,
		nil, ssmfactory.NewSSMClientCreator(), s3factory.NewS3ClientCreator(), asmfactory.NewClientCreator(), credspecContainerMapping)
	if err != nil {
		return err
	}

	err = credentialspecResource.HandleDomainlessKerberosTicketRenewal(iamRoleCredentials)
	if err != nil {
		return err
	}
	return nil
}
