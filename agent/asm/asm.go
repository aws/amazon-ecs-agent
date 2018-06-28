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
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

// AuthDataValue is the schema for
// the SecretStringValue returned by ASM
type AuthDataValue struct {
	Username *string
	Password *string
}

func GetDockerAuthFromASM(secretID string, client secretsmanageriface.SecretsManagerAPI) (docker.AuthConfiguration, error) {
	in := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	}

	out, err := client.GetSecretValue(in)
	if err != nil {
		return docker.AuthConfiguration{}, errors.Wrapf(err,
			"asm fetching secret from the service for %s", secretID)
	}

	return extractASMValue(out)
}

func extractASMValue(out *secretsmanager.GetSecretValueOutput) (docker.AuthConfiguration, error) {
	if out == nil {
		return docker.AuthConfiguration{}, fmt.Errorf(
			"asm fetching authorization data: empty response")
	}

	secretValue := aws.StringValue(out.SecretString)
	if secretValue == "" {
		return docker.AuthConfiguration{}, fmt.Errorf(
			"asm fetching authorization data: empty secrets value")
	}

	authDataValue := AuthDataValue{}
	err := json.Unmarshal([]byte(secretValue), &authDataValue)
	if err != nil {
		// could  not unmarshal, incorrect secret value schema
		return docker.AuthConfiguration{}, errors.Wrapf(err,
			"asm fetching authorization data: unable to unmarshal secret value")
	}

	username := aws.StringValue(authDataValue.Username)
	password := aws.StringValue(authDataValue.Password)

	if username == "" {
		return docker.AuthConfiguration{}, fmt.Errorf(
			"asm fetching username: AuthorizationData is malformed, emmpty field")
	}

	if password == "" {
		return docker.AuthConfiguration{}, fmt.Errorf(
			"asm fetching password: AuthorizationData is malformed, emmpty field")
	}

	dac := docker.AuthConfiguration{
		Username: username,
		Password: password,
	}

	return dac, nil
}
