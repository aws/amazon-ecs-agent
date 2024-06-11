//go:build unit
// +build unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package asm

import (
	"fmt"
	"testing"

	mocks "github.com/aws/amazon-ecs-agent/agent/asm/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	versionID                = "versionId"
	versionStage             = "versionStage"
	jsonKey                  = "jsonKey"
	valueFrom                = "arn:aws:secretsmanager:region:account-id:secret:secretId"
	secretValue              = "secretValue"
	jsonSecretValue          = "{\"" + jsonKey + "\": \"" + secretValue + "\",\"some-other-key\": \"secret2\"}"
	malformedJsonSecretValue = "{\"" + jsonKey + "\": \"" + secretValue
)

func TestASMGetAuthConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cases := []struct {
		Name        string
		Resp        *secretsmanager.GetSecretValueOutput
		ShouldError bool
	}{
		{
			Name: "SuccessWithValidResponse",
			Resp: &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"usr","password":"pwd"}`),
			},
			ShouldError: false,
		},
		{
			Name: "MissingUsername",
			Resp: &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"password":"pwd"}`),
			},
			ShouldError: true,
		},
		{
			Name: "MissingPassword",
			Resp: &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"usr"}`),
			},
			ShouldError: true,
		},
		{
			Name: "EmptyUsername",
			Resp: &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"","password":"pwd"}`),
			},
			ShouldError: true,
		},
		{
			Name: "EmptyPassword",
			Resp: &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"usr","password":""}`),
			},
			ShouldError: true,
		},
		{
			Name: "MalformedJson",
			Resp: &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"usr"`),
			},
			ShouldError: true,
		},
		{
			Name:        "MissingSecretString",
			Resp:        &secretsmanager.GetSecretValueOutput{},
			ShouldError: true,
		},
		{
			Name: "EmptyJsonStruct",
			Resp: &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{}`),
			},
			ShouldError: true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
			mockSecretsManager.EXPECT().
				GetSecretValue(gomock.Any()).
				Return(c.Resp, nil)

			_, err := GetDockerAuthFromASM("secret-value-id", mockSecretsManager)

			if c.ShouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetSecretFromASM(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
	mockSecretsManager.EXPECT().
		GetSecretValue(gomock.Any()).
		Return(&secretsmanager.GetSecretValueOutput{
			SecretString: aws.String(secretValue),
		}, nil)

	_, err := GetSecretFromASM("secretName", mockSecretsManager)
	assert.NoError(t, err)
}

func TestGetSecretFromASMWithJsonKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
	mockSecretsManager.EXPECT().
		GetSecretValue(gomock.Any()).
		Return(&secretsmanager.GetSecretValueOutput{
			SecretString: aws.String(jsonSecretValue),
		}, nil)

	secretValueInput := createSecretValueInput(toPtr(valueFrom), nil, nil)
	outSecretValue, _ := GetSecretFromASMWithInput(secretValueInput, mockSecretsManager, jsonKey)
	assert.Equal(t, secretValue, outSecretValue)
}

func TestGetSecretFromASMWithMalformedJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
	mockSecretsManager.EXPECT().
		GetSecretValue(gomock.Any()).
		Return(&secretsmanager.GetSecretValueOutput{
			SecretString: aws.String(malformedJsonSecretValue),
		}, nil)

	secretValueInput := createSecretValueInput(toPtr(valueFrom), nil, nil)
	outSecretValue, err := GetSecretFromASMWithInput(secretValueInput, mockSecretsManager, jsonKey)
	require.Error(t, err)
	assert.Equal(t, "", outSecretValue)
}

func TestGetSecretFromASMWithJSONKeyNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
	mockSecretsManager.EXPECT().
		GetSecretValue(gomock.Any()).
		Return(&secretsmanager.GetSecretValueOutput{
			SecretString: aws.String(jsonSecretValue),
		}, nil)

	secretValueInput := createSecretValueInput(toPtr(valueFrom), nil, nil)
	nonExistentKey := "nonExistentKey"
	_, err := GetSecretFromASMWithInput(secretValueInput, mockSecretsManager, nonExistentKey)
	assert.Error(t, err)
}

func TestGetSecretFromASMWithVersionID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
	mockSecretsManager.EXPECT().
		GetSecretValue(gomock.Any()).
		Return(&secretsmanager.GetSecretValueOutput{
			SecretString: aws.String(secretValue),
		}, nil)

	secretValueInput := createSecretValueInput(toPtr(valueFrom), toPtr(versionID), nil)
	outSecretValue, err := GetSecretFromASMWithInput(secretValueInput, mockSecretsManager, "")
	require.NoError(t, err)
	assert.Equal(t, secretValue, outSecretValue)
}

func TestGetSecretFromASMWithVersionIDAndStage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
	mockSecretsManager.EXPECT().
		GetSecretValue(gomock.Any()).
		Return(&secretsmanager.GetSecretValueOutput{
			SecretString: aws.String(secretValue),
		}, nil)

	secretValueInput := createSecretValueInput(toPtr(valueFrom), toPtr(versionID), toPtr(versionStage))
	outSecretValue, err := GetSecretFromASMWithInput(secretValueInput, mockSecretsManager, "")
	require.NoError(t, err)
	assert.Equal(t, secretValue, outSecretValue)
}

func toPtr(input string) *string {
	if input == "" {
		return nil
	}
	return &input
}

func createSecretValueInput(secretID *string, versionID *string, versionStage *string) *secretsmanager.GetSecretValueInput {
	return &secretsmanager.GetSecretValueInput{
		SecretId:     secretID,
		VersionId:    versionID,
		VersionStage: versionStage,
	}
}

func TestGetSecretFromASMWithInputErrorMessageKnownError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
	mockSecretsManager.EXPECT().
		GetSecretValue(gomock.Any()).
		Return(nil, awserr.New(secretsmanager.ErrCodeResourceNotFoundException, "Secrets Manager can't find the specified secret.", nil))

	secretValueInput := createSecretValueInput(toPtr(valueFrom), toPtr(versionID), nil)
	_, err := GetSecretFromASMWithInput(secretValueInput, mockSecretsManager, jsonKey)

	assert.Error(t, err)
	aerr, ok := errors.Cause(err).(awserr.Error)
	require.True(t, ok, "error is not of type awserr.Error")
	assert.Equal(t, secretsmanager.ErrCodeResourceNotFoundException, aerr.Code())
	assert.Contains(t, err.Error(), fmt.Sprintf("ResourceNotFoundException: The task can't retrieve the secret with ARN '%s' from AWS Secrets Manager. Check whether the secret exists in the specified Region", valueFrom))
}

func TestGetSecretFromASMWithInputErrorMessageUnknownError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretsManager := mocks.NewMockSecretsManagerAPI(ctrl)
	mockSecretsManager.EXPECT().
		GetSecretValue(gomock.Any()).
		Return(nil, awserr.New(secretsmanager.ErrCodeInternalServiceError, "uhoh", nil))

	secretValueInput := createSecretValueInput(toPtr(valueFrom), toPtr(versionID), nil)
	_, err := GetSecretFromASMWithInput(secretValueInput, mockSecretsManager, jsonKey)

	assert.Error(t, err)
	aerr, ok := errors.Cause(err).(awserr.Error)
	require.True(t, ok, "error is not of type awserr.Error")
	assert.Equal(t, secretsmanager.ErrCodeInternalServiceError, aerr.Code())
	assert.Contains(t, err.Error(), fmt.Sprintf("secret %s: InternalServiceError: uhoh", valueFrom))
}
