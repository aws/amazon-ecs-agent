// +build unit

// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package asmauth

import (
	"encoding/json"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/asm"
	"github.com/aws/amazon-ecs-agent/agent/asm/factory/mocks"
	mock_secretsmanageriface "github.com/aws/amazon-ecs-agent/agent/asm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	executionCredentialsID = "exec-creds-id"
	region                 = "us-west-2"
	secretID               = "meaning-of-life"
	username               = "irene"
	password               = "sher"
	taskARN                = "task1"
)

var (
	asmAuthDataVal       string
	requiredASMResources []*apicontainer.ASMAuthData
)

func init() {
	asmAuthDataBytes, _ := json.Marshal(&asm.AuthDataValue{
		Username: aws.String(username),
		Password: aws.String(password),
	})
	asmAuthDataVal = string(asmAuthDataBytes)
	requiredASMResources = []*apicontainer.ASMAuthData{
		{
			CredentialsParameter: secretID,
			Region:               region,
		},
	}
}

func TestCreateAndGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)
	mockASMClient := mock_secretsmanageriface.NewMockSecretsManagerAPI(ctrl)

	iamRoleCreds := credentials.IAMRoleCredentials{}
	creds := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: iamRoleCreds,
	}
	asmSecretValue := &secretsmanager.GetSecretValueOutput{
		SecretString: aws.String(asmAuthDataVal),
	}
	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		asmClientCreator.EXPECT().NewASMClient(region, iamRoleCreds).Return(mockASMClient),
		mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Do(func(in *secretsmanager.GetSecretValueInput) {
			assert.Equal(t, aws.StringValue(in.SecretId), secretID)
		}).Return(asmSecretValue, nil),
	)
	asmRes := &ASMAuthResource{
		executionCredentialsID: executionCredentialsID,
		requiredASMResources:   requiredASMResources,
		credentialsManager:     credentialsManager,
		asmClientCreator:       asmClientCreator,
	}
	require.NoError(t, asmRes.Create())
	dac, ok := asmRes.GetASMDockerAuthConfig(secretID)
	require.True(t, ok)
	assert.Equal(t, dac.Username, username)
	assert.Equal(t, dac.Password, password)
}

func TestMarshalUnmarshalJSON(t *testing.T) {
	asmResIn := &ASMAuthResource{
		taskARN:                taskARN,
		executionCredentialsID: executionCredentialsID,
		createdAt:              time.Now(),
		knownStatusUnsafe:      resourcestatus.ResourceCreated,
		desiredStatusUnsafe:    resourcestatus.ResourceCreated,
		requiredASMResources:   requiredASMResources,
	}

	bytes, err := json.Marshal(asmResIn)
	require.NoError(t, err)

	asmResOut := &ASMAuthResource{}
	err = json.Unmarshal(bytes, asmResOut)
	require.NoError(t, err)
	assert.Equal(t, asmResIn.taskARN, asmResOut.taskARN)
	assert.WithinDuration(t, asmResIn.createdAt, asmResOut.createdAt, time.Microsecond)
	assert.Equal(t, asmResIn.desiredStatusUnsafe, asmResOut.desiredStatusUnsafe)
	assert.Equal(t, asmResIn.knownStatusUnsafe, asmResOut.knownStatusUnsafe)
	assert.Equal(t, asmResIn.executionCredentialsID, asmResOut.executionCredentialsID)
	assert.Equal(t, len(asmResIn.requiredASMResources), len(asmResOut.requiredASMResources))
	assert.Equal(t, asmResIn.requiredASMResources[0], asmResOut.requiredASMResources[0])
}

func TestInitialize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)
	asmRes := &ASMAuthResource{
		knownStatusUnsafe:   resourcestatus.ResourceCreated,
		desiredStatusUnsafe: resourcestatus.ResourceCreated,
	}
	asmRes.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			ASMClientCreator:   asmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)
	assert.Equal(t, resourcestatus.ResourceStatusNone, asmRes.GetKnownStatus())
	assert.Equal(t, resourcestatus.ResourceCreated, asmRes.GetDesiredStatus())

}
