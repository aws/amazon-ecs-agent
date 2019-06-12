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

package asmsecret

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/asm/factory/mocks"
	mock_secretsmanageriface "github.com/aws/amazon-ecs-agent/agent/asm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
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
	region1                = "us-west-2"
	region2                = "us-east-1"
	secretName1            = "db_username_1"
	secretName2            = "db_username_2"
	valueFrom1             = "secret-name"
	secretKeyWest1         = "secret-name_us-west-2"
	secretKeyEast1         = "secret-name_us-east-1"
	secretValue            = "secret-value"
	taskARN                = "task1"
)

func TestCreateWithMultipleASMCall(t *testing.T) {
	requiredSecretData := map[string]apicontainer.Secret{
		secretKeyWest1: {
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "asm",
		},
		secretKeyEast1: {
			Name:      secretName2,
			ValueFrom: valueFrom1,
			Region:    region2,
			Provider:  "asm",
		},
	}

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
		SecretString: aws.String(secretValue),
	}

	credentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true)
	asmClientCreator.EXPECT().NewASMClient(region1, iamRoleCreds).Return(mockASMClient)
	asmClientCreator.EXPECT().NewASMClient(region2, iamRoleCreds).Return(mockASMClient)
	mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Do(func(in *secretsmanager.GetSecretValueInput) {
		assert.Equal(t, aws.StringValue(in.SecretId), valueFrom1)
	}).Return(asmSecretValue, nil).Times(2)

	asmRes := &ASMSecretResource{
		executionCredentialsID: executionCredentialsID,
		requiredSecrets:        requiredSecretData,
		credentialsManager:     credentialsManager,
		asmClientCreator:       asmClientCreator,
	}
	require.NoError(t, asmRes.Create())

	value1, ok := asmRes.GetCachedSecretValue(secretKeyWest1)
	require.True(t, ok)
	assert.Equal(t, secretValue, value1)

	value2, ok := asmRes.GetCachedSecretValue(secretKeyEast1)
	require.True(t, ok)
	assert.Equal(t, secretValue, value2)
}

func TestCreateReturnMultipleErrors(t *testing.T) {

	requiredSecretData := map[string]apicontainer.Secret{
		secretKeyWest1: {
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "asm",
		},
		secretKeyEast1: {
			Name:      secretName2,
			ValueFrom: valueFrom1,
			Region:    region2,
			Provider:  "asm",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)
	mockASMClient := mock_secretsmanageriface.NewMockSecretsManagerAPI(ctrl)

	iamRoleCreds := credentials.IAMRoleCredentials{}
	creds := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: iamRoleCreds,
	}

	asmSecretValue := &secretsmanager.GetSecretValueOutput{}

	credentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true)
	asmClientCreator.EXPECT().NewASMClient(region1, iamRoleCreds).Return(mockASMClient)
	asmClientCreator.EXPECT().NewASMClient(region2, iamRoleCreds).Return(mockASMClient)
	mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Do(func(in *secretsmanager.GetSecretValueInput) {
		assert.Equal(t, aws.StringValue(in.SecretId), valueFrom1)
	}).Return(asmSecretValue, errors.New("error response")).Times(2)

	asmRes := &ASMSecretResource{
		executionCredentialsID: executionCredentialsID,
		requiredSecrets:        requiredSecretData,
		credentialsManager:     credentialsManager,
		asmClientCreator:       asmClientCreator,
	}

	assert.Error(t, asmRes.Create())
	expectedError1 := fmt.Sprintf("fetching secret data from AWS Secrets Manager in region %s: secret %s: error response", region1, valueFrom1)
	expectedError2 := fmt.Sprintf("fetching secret data from AWS Secrets Manager in region %s: secret %s: error response", region2, valueFrom1)
	assert.Contains(t, asmRes.GetTerminalReason(), expectedError1)
	assert.Contains(t, asmRes.GetTerminalReason(), expectedError2)
}

func TestCreateReturnError(t *testing.T) {
	requiredSecretData := map[string]apicontainer.Secret{
		secretKeyWest1: {
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "asm",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)
	mockASMClient := mock_secretsmanageriface.NewMockSecretsManagerAPI(ctrl)

	iamRoleCreds := credentials.IAMRoleCredentials{}
	creds := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: iamRoleCreds,
	}

	asmSecretValue := &secretsmanager.GetSecretValueOutput{}

	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		asmClientCreator.EXPECT().NewASMClient(region1, iamRoleCreds).Return(mockASMClient),
		mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Do(func(in *secretsmanager.GetSecretValueInput) {
			assert.Equal(t, aws.StringValue(in.SecretId), valueFrom1)
		}).Return(asmSecretValue, errors.New("error response")),
	)
	asmRes := &ASMSecretResource{
		executionCredentialsID: executionCredentialsID,
		requiredSecrets:        requiredSecretData,
		credentialsManager:     credentialsManager,
		asmClientCreator:       asmClientCreator,
	}

	assert.Error(t, asmRes.Create())
	expectedError := fmt.Sprintf("fetching secret data from AWS Secrets Manager in region %s: secret %s: error response", region1, valueFrom1)
	assert.Equal(t, expectedError, asmRes.GetTerminalReason())
}

func TestMarshalUnmarshalJSON(t *testing.T) {
	requiredSecretData := map[string]apicontainer.Secret{
		secretKeyWest1: {
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "asm",
		},
	}

	asmResIn := &ASMSecretResource{
		taskARN:                taskARN,
		executionCredentialsID: executionCredentialsID,
		createdAt:              time.Now(),
		knownStatusUnsafe:      resourcestatus.ResourceCreated,
		desiredStatusUnsafe:    resourcestatus.ResourceCreated,
		requiredSecrets:        requiredSecretData,
	}

	bytes, err := json.Marshal(asmResIn)
	require.NoError(t, err)

	asmResOut := &ASMSecretResource{}
	err = json.Unmarshal(bytes, asmResOut)
	require.NoError(t, err)
	assert.Equal(t, asmResIn.taskARN, asmResOut.taskARN)
	assert.WithinDuration(t, asmResIn.createdAt, asmResOut.createdAt, time.Microsecond)
	assert.Equal(t, asmResIn.desiredStatusUnsafe, asmResOut.desiredStatusUnsafe)
	assert.Equal(t, asmResIn.knownStatusUnsafe, asmResOut.knownStatusUnsafe)
	assert.Equal(t, asmResIn.executionCredentialsID, asmResOut.executionCredentialsID)
	assert.Equal(t, len(asmResIn.requiredSecrets), len(asmResOut.requiredSecrets))
	assert.Equal(t, asmResIn.requiredSecrets[secretKeyWest1], asmResOut.requiredSecrets[secretKeyWest1])
}

func TestInitialize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	asmClientCreator := mock_factory.NewMockClientCreator(ctrl)
	asmRes := &ASMSecretResource{
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

func TestClearASMSecretValue(t *testing.T) {
	secretValues := map[string]string{
		"db_name":     "db_value",
		"secret_name": "secret_value",
	}

	asmRes := &ASMSecretResource{
		secretData: secretValues,
	}
	asmRes.clearASMSecretValue()
	assert.Equal(t, 0, len(asmRes.secretData))
}
