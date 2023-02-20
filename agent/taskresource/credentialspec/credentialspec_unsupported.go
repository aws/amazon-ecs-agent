//go:build !linux && !windows
// +build !linux,!windows

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

package credentialspec

import (
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"

	"github.com/pkg/errors"
)

// CredentialSpecResource is the abstraction for credentialspec resources
type CredentialSpecResource struct {
	*CredentialSpecResourceCommon
}

// CredentialSpecResourceJSON is the json representation of the credentialspec resource
type CredentialSpecResourceJSON struct {
	*CredentialSpecResourceJSONCommon
}

func NewCredentialSpecResource(taskARN, region string,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	s3ClientCreator s3factory.S3ClientCreator,
	credentialSpecContainerMap map[string]string) (*CredentialSpecResource, error) {
	return nil, errors.New("not supported")
}

func (cs *CredentialSpecResource) Create() error {
	return errors.New("not supported")
}

func (cs *CredentialSpecResource) Cleanup() error {
	return errors.New("not supported")
}

func (cs *CredentialSpecResource) MarshallPlatformSpecificFields(credentialSpecResourceJSON *CredentialSpecResourceJSON) {
	return
}

func (cs *CredentialSpecResource) UnmarshallPlatformSpecificFields(credentialSpecResourceJSON CredentialSpecResourceJSON) {
	return
}
