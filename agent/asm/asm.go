// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

// AuthDataValue is the schema for
// the SecretStringValue returned by ASM
type AuthDataValue struct {
	Username *string
	Password *string
}

// GetDockerAuthFromASM makes the api call to the AWS Secrets Manager service to
// retrieve the docker auth data
func GetDockerAuthFromASM(secretID string, client secretsmanageriface.SecretsManagerAPI) (types.AuthConfig, error) {
	in := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	}

	out, err := client.GetSecretValue(in)
	if err != nil {
		return types.AuthConfig{}, errors.Wrapf(err,
			"asm fetching secret from the service for %s", secretID)
	}

	return extractASMValue(out)
}

func extractASMValue(out *secretsmanager.GetSecretValueOutput) (types.AuthConfig, error) {
	if out == nil {
		return types.AuthConfig{}, errors.New(
			"asm fetching authorization data: empty response")
	}

	secretValue := aws.StringValue(out.SecretString)
	if secretValue == "" {
		return types.AuthConfig{}, errors.New(
			"asm fetching authorization data: empty secrets value")
	}

	authDataValue := AuthDataValue{}
	err := json.Unmarshal([]byte(secretValue), &authDataValue)
	if err != nil {
		// could  not unmarshal, incorrect secret value schema
		return types.AuthConfig{}, errors.New(
			"asm fetching authorization data: unable to unmarshal secret value, invalid schema")
	}

	username := aws.StringValue(authDataValue.Username)
	password := aws.StringValue(authDataValue.Password)

	if username == "" {
		return types.AuthConfig{}, errors.New(
			"asm fetching username: AuthorizationData is malformed, empty field")
	}

	if password == "" {
		return types.AuthConfig{}, errors.New(
			"asm fetching password: AuthorizationData is malformed, empty field")
	}

	dac := types.AuthConfig{
		Username: username,
		Password: password,
	}

	return dac, nil
}

// GetSecretFromASM makes the api call to the AWS Secrets Manager service to
// retrieve the secret value
func GetSecretFromASM(secretID string, client secretsmanageriface.SecretsManagerAPI) (string, error) {
	in := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	}

	out, err := client.GetSecretValue(in)
	if err != nil {
		return "", errors.Wrapf(err, "secret %s", secretID)
	}

	return aws.StringValue(out.SecretString), nil
}
