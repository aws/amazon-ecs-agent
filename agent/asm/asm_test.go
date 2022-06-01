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
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
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

type mockGetSecretValue struct {
	secretsmanageriface.SecretsManagerAPI
	Resp secretsmanager.GetSecretValueOutput
}

func (m mockGetSecretValue) GetSecretValue(input *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
	return &m.Resp, nil
}

func TestASMGetAuthConfig(t *testing.T) {

	cases := []struct {
		Name        string
		Resp        secretsmanager.GetSecretValueOutput
		ShouldError bool
	}{
		{
			Name: "SuccessWithValidResponse",
			Resp: secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"usr","password":"pwd"}`),
			},
			ShouldError: false,
		},
		{
			Name: "MissingUsername",
			Resp: secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"password":"pwd"}`),
			},
			ShouldError: true,
		},
		{
			Name: "MissingPassword",
			Resp: secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"usr"}`),
			},
			ShouldError: true,
		},
		{
			Name: "EmptyUsername",
			Resp: secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"","password":"pwd"}`),
			},
			ShouldError: true,
		},
		{
			Name: "EmptyPassword",
			Resp: secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"usr","password":""}`),
			},
			ShouldError: true,
		},
		{
			Name: "MalformedJson",
			Resp: secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{"username":"usr"`),
			},
			ShouldError: true,
		},
		{
			Name:        "MissingSecretString",
			Resp:        secretsmanager.GetSecretValueOutput{},
			ShouldError: true,
		},
		{
			Name: "EmptyJsonStruct",
			Resp: secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(`{}`),
			},
			ShouldError: true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			asmClient := mockGetSecretValue{Resp: c.Resp}
			_, err := GetDockerAuthFromASM("secret-value-id", asmClient)

			if c.ShouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetSecretFromASM(t *testing.T) {
	asmClient := createASMInterface(secretValue)
	_, err := GetSecretFromASM("secretName", asmClient)
	assert.NoError(t, err)
}

func TestGetSecretFromASMWithJsonKey(t *testing.T) {
	asmClient := createASMInterface(jsonSecretValue)
	secretValueInput := createSecretValueInput(toPtr(valueFrom), nil, nil)
	outSecretValue, _ := GetSecretFromASMWithInput(secretValueInput, asmClient, jsonKey)
	assert.Equal(t, secretValue, outSecretValue)
}

func TestGetSecretFromASMWithMalformedJSON(t *testing.T) {
	asmClient := createASMInterface(malformedJsonSecretValue)
	secretValueInput := createSecretValueInput(toPtr(valueFrom), nil, nil)
	outSecretValue, err := GetSecretFromASMWithInput(secretValueInput, asmClient, jsonKey)
	require.Error(t, err)
	assert.Equal(t, "", outSecretValue)
}

func TestGetSecretFromASMWithJSONKeyNotFound(t *testing.T) {
	asmClient := createASMInterface(jsonSecretValue)
	secretValueInput := createSecretValueInput(toPtr(valueFrom), nil, nil)
	nonExistentKey := "nonExistentKey"
	_, err := GetSecretFromASMWithInput(secretValueInput, asmClient, nonExistentKey)
	assert.Error(t, err)
}

func TestGetSecretFromASMWithVersionID(t *testing.T) {
	asmClient := createASMInterface(secretValue)
	secretValueInput := createSecretValueInput(toPtr(valueFrom), toPtr(versionID), nil)
	outSecretValue, err := GetSecretFromASMWithInput(secretValueInput, asmClient, "")
	require.NoError(t, err)
	assert.Equal(t, secretValue, outSecretValue)
}

func TestGetSecretFromASMWithVersionIDAndStage(t *testing.T) {
	asmClient := createASMInterface(secretValue)
	secretValueInput := createSecretValueInput(toPtr(valueFrom), toPtr(versionID), toPtr(versionStage))
	outSecretValue, err := GetSecretFromASMWithInput(secretValueInput, asmClient, "")
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

func createASMInterface(secretValue string) mockGetSecretValue {
	return mockGetSecretValue{
		Resp: secretsmanager.GetSecretValueOutput{
			SecretString: aws.String(secretValue),
		},
	}
}
