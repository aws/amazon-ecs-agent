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

package ssmsecret

import (
	"encoding/json"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ssm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
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
	valueFrom2             = "secret-name-2"
	valueFromARN           = "arn:aws:ssm:us-west-2:123456:parameter/secret-name"
	secretKeyWest1         = "secret-name_us-west-2"
	secretKeyWest2         = "secret-name-2_us-west-2"
	secretKeyEast1         = "secret-name_us-east-1"
	secretValue            = "secret-value"
	taskARN                = "task1"
)

func TestCreateAndGetWithOneCall(t *testing.T) {
	requiredSecretData := make(map[string][]apicontainer.Secret)
	secretsInRegion1 := []apicontainer.Secret{
		{
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "ssm",
		},
		{
			Name:      secretName2,
			ValueFrom: valueFrom2,
			Region:    region1,
			Provider:  "ssm",
		},
	}

	requiredSecretData[region1] = secretsInRegion1

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	mockSSMClient := mock_ssmiface.NewMockSSMAPI(ctrl)

	iamRoleCreds := credentials.IAMRoleCredentials{}
	creds := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: iamRoleCreds,
	}

	ssmOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{},
		Parameters: []*ssm.Parameter{
			&ssm.Parameter{
				Name:  aws.String(valueFrom1),
				Value: aws.String(secretValue),
			},
			&ssm.Parameter{
				Name:  aws.String(valueFrom2),
				Value: aws.String(secretValue),
			},
		},
	}

	allNames := []*string{aws.String(valueFrom1), aws.String(valueFrom2)}

	credentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true)
	ssmClientCreator.EXPECT().NewSSMClient(region1, iamRoleCreds).Return(mockSSMClient)
	mockSSMClient.EXPECT().GetParameters(gomock.Any()).Do(func(in *ssm.GetParametersInput) {
		assert.Equal(t, in.Names, allNames)
	}).Return(ssmOutput, nil).Times(1)

	ssmRes := &SSMSecretResource{
		executionCredentialsID: executionCredentialsID,
		requiredSecrets:        requiredSecretData,
		credentialsManager:     credentialsManager,
		ssmClientCreator:       ssmClientCreator,
	}
	require.NoError(t, ssmRes.Create())

	value1, ok := ssmRes.GetSSMSecretValue(secretKeyWest1)
	require.True(t, ok)
	assert.Equal(t, secretValue, value1)

	value2, ok := ssmRes.GetSSMSecretValue(secretKeyWest2)
	require.True(t, ok)
	assert.Equal(t, secretValue, value2)
}

func TestCreateAndGetWithTwoCalls(t *testing.T) {
	requiredSecretData := make(map[string][]apicontainer.Secret)
	secretsInRegion1 := []apicontainer.Secret{
		{
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "ssm",
		},
	}
	secretsInRegion2 := []apicontainer.Secret{
		{
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region2,
			Provider:  "ssm",
		},
	}
	requiredSecretData[region1] = secretsInRegion1
	requiredSecretData[region2] = secretsInRegion2

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	mockSSMClient := mock_ssmiface.NewMockSSMAPI(ctrl)

	iamRoleCreds := credentials.IAMRoleCredentials{}
	creds := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: iamRoleCreds,
	}

	ssmOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{},
		Parameters: []*ssm.Parameter{
			&ssm.Parameter{
				Name:  aws.String(valueFrom1),
				Value: aws.String(secretValue),
			},
		},
	}

	allNames := []*string{aws.String(valueFrom1)}

	credentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true)
	ssmClientCreator.EXPECT().NewSSMClient(region1, iamRoleCreds).Return(mockSSMClient)
	ssmClientCreator.EXPECT().NewSSMClient(region2, iamRoleCreds).Return(mockSSMClient)
	mockSSMClient.EXPECT().GetParameters(gomock.Any()).Do(func(in *ssm.GetParametersInput) {
		assert.Equal(t, in.Names, allNames)
	}).Return(ssmOutput, nil).Times(2)

	ssmRes := &SSMSecretResource{
		executionCredentialsID: executionCredentialsID,
		requiredSecrets:        requiredSecretData,
		credentialsManager:     credentialsManager,
		ssmClientCreator:       ssmClientCreator,
	}
	require.NoError(t, ssmRes.Create())

	value1, ok := ssmRes.GetSSMSecretValue(secretKeyWest1)
	require.True(t, ok)
	assert.Equal(t, secretValue, value1)

	value2, ok := ssmRes.GetSSMSecretValue(secretKeyEast1)
	require.True(t, ok)
	assert.Equal(t, secretValue, value2)
}

func TestCreateReturnError(t *testing.T) {
	requiredSecretData := make(map[string][]apicontainer.Secret)
	secretsInRegion1 := []apicontainer.Secret{
		{
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "ssm",
		},
	}
	requiredSecretData[region1] = secretsInRegion1

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	mockSSMClient := mock_ssmiface.NewMockSSMAPI(ctrl)

	iamRoleCreds := credentials.IAMRoleCredentials{}
	creds := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: iamRoleCreds,
	}

	ssmOutput := &ssm.GetParametersOutput{
		InvalidParameters: []*string{aws.String(valueFrom1)},
		Parameters:        []*ssm.Parameter{},
	}

	allNames := []*string{aws.String(valueFrom1)}
	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(executionCredentialsID).Return(creds, true),
		ssmClientCreator.EXPECT().NewSSMClient(region1, iamRoleCreds).Return(mockSSMClient),
		mockSSMClient.EXPECT().GetParameters(gomock.Any()).Do(func(in *ssm.GetParametersInput) {
			assert.Equal(t, in.Names, allNames)
		}).Return(ssmOutput, nil),
	)
	ssmRes := &SSMSecretResource{
		executionCredentialsID: executionCredentialsID,
		requiredSecrets:        requiredSecretData,
		credentialsManager:     credentialsManager,
		ssmClientCreator:       ssmClientCreator,
	}

	assert.Error(t, ssmRes.Create())
	expectedError := "fetching secret data from ssm parameter store: invalid parameters: secret-name, "
	assert.Equal(t, expectedError, ssmRes.GetTerminalReason())
}

func TestGetGoRoutineTotalNumTwoRegions(t *testing.T) {
	requiredSecretData := make(map[string][]apicontainer.Secret)
	secretsInRegion1 := []apicontainer.Secret{
		{
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "ssm",
		},
	}
	secretsInRegion2 := []apicontainer.Secret{
		{
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region2,
			Provider:  "ssm",
		},
	}
	requiredSecretData[region1] = secretsInRegion1
	requiredSecretData[region2] = secretsInRegion2

	ssmRes := &SSMSecretResource{
		requiredSecrets: requiredSecretData,
	}

	number := ssmRes.getGoRoutineTotalNum()
	assert.Equal(t, 2, number)
}

func TestGetGoRoutineTotalNumOneRegion(t *testing.T) {
	requiredSecretData := make(map[string][]apicontainer.Secret)
	secretsInRegion1 := []apicontainer.Secret{
		{
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "ssm",
		},
		{
			Name:      secretName2,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "ssm",
		},
	}

	requiredSecretData[region1] = secretsInRegion1

	ssmRes := &SSMSecretResource{
		requiredSecrets: requiredSecretData,
	}

	number := ssmRes.getGoRoutineTotalNum()
	assert.Equal(t, 1, number)
}

func TestExtractNameFromValueFrom(t *testing.T) {
	secretData := apicontainer.Secret{
		Name:      secretName1,
		ValueFrom: valueFromARN,
		Region:    region1,
	}
	ssmRes := &SSMSecretResource{}

	name := ssmRes.extractNameFromValueFrom(secretData)
	assert.Equal(t, valueFrom1, name)
}

func TestMarshalUnmarshalJSON(t *testing.T) {
	requiredSecretData := make(map[string][]apicontainer.Secret)
	secretsInRegion1 := []apicontainer.Secret{
		{
			Name:      secretName1,
			ValueFrom: valueFrom1,
			Region:    region1,
			Provider:  "ssm",
		},
	}
	requiredSecretData[region1] = secretsInRegion1

	ssmResIn := &SSMSecretResource{
		taskARN:                taskARN,
		executionCredentialsID: executionCredentialsID,
		createdAt:              time.Now(),
		knownStatusUnsafe:      resourcestatus.ResourceCreated,
		desiredStatusUnsafe:    resourcestatus.ResourceCreated,
		requiredSecrets:        requiredSecretData,
	}

	bytes, err := json.Marshal(ssmResIn)
	require.NoError(t, err)

	ssmResOut := &SSMSecretResource{}
	err = json.Unmarshal(bytes, ssmResOut)
	require.NoError(t, err)
	assert.Equal(t, ssmResIn.taskARN, ssmResOut.taskARN)
	assert.WithinDuration(t, ssmResIn.createdAt, ssmResOut.createdAt, time.Microsecond)
	assert.Equal(t, ssmResIn.desiredStatusUnsafe, ssmResOut.desiredStatusUnsafe)
	assert.Equal(t, ssmResIn.knownStatusUnsafe, ssmResOut.knownStatusUnsafe)
	assert.Equal(t, ssmResIn.executionCredentialsID, ssmResOut.executionCredentialsID)
	assert.Equal(t, len(ssmResIn.requiredSecrets), len(ssmResOut.requiredSecrets))
	assert.Equal(t, ssmResIn.requiredSecrets[region1], ssmResOut.requiredSecrets[region1])
}

func TestInitialize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_factory.NewMockSSMClientCreator(ctrl)
	ssmRes := &SSMSecretResource{
		knownStatusUnsafe:   resourcestatus.ResourceCreated,
		desiredStatusUnsafe: resourcestatus.ResourceCreated,
	}
	ssmRes.Initialize(&taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
	}, apitaskstatus.TaskStatusNone, apitaskstatus.TaskRunning)
	assert.Equal(t, resourcestatus.ResourceStatusNone, ssmRes.GetKnownStatus())
	assert.Equal(t, resourcestatus.ResourceCreated, ssmRes.GetDesiredStatus())

}
func TestClearSSMSecretValue(t *testing.T) {
	secretValues := map[string]string{
		"db_name": "db_value",
		"secret":  "secret_value",
	}

	ssmRes := &SSMSecretResource{
		secretData: secretValues,
	}
	ssmRes.clearSSMSecretValue()
	assert.Equal(t, 0, len(ssmRes.secretData))
}
