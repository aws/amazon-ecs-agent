// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
