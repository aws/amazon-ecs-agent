//go:build windows
// +build windows

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

package handler

import (
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
)

// setDomainlessGMSATaskExecutionRoleCredentials sets the taskExecutionRoleCredentials to a Windows Registry Key so that
// the domainless gMSA plugin can use these credentials to retrieve the customer Active Directory credential
func checkAndSetDomainlessGMSATaskExecutionRoleCredentials(iamRoleCredentials credentials.IAMRoleCredentials, task *task.Task) error {
	// exit early if the task does not need domainless gMSA
	if !task.RequiresDomainlessCredentialSpecResource() {
		return nil
	}
	return credentialspec.SetTaskExecutionCredentialsRegKeys(iamRoleCredentials, task.Arn)
}
