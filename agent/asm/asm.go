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
	"encoding/json"
	"fmt"

	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

// AuthDataValue is the schema for
// the SecretStringValue returned by ASM
type AuthDataValue struct {
	Username *string
	Password *string
}

type ASMError struct {
	FromError error
}

func (err ASMError) Error() string {
	return err.FromError.Error()
}

func (err ASMError) ErrorName() string {
	return "ASMError"
}

func (err ASMError) Constructor() func(string) apierrors.NamedError {
	return func(msg string) apierrors.NamedError {
		return ASMError{errors.New(msg)}
	}
}

func wrapError(secretID string, origError error) error {
	ctx := apierrors.ErrorContext{SecretID: secretID}
	return apierrors.AugmentNamedErrMsg(ASMError{FromError: origError}, ctx)
}

// GetDockerAuthFromASM makes the api call to the AWS Secrets Manager service to
// retrieve the docker auth data
func GetDockerAuthFromASM(secretID string, client secretsmanageriface.SecretsManagerAPI) (types.AuthConfig, error) {
	in := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	}

	out, err := client.GetSecretValue(in)
	if err != nil {
		return types.AuthConfig{}, wrapError(secretID, err)
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

func GetSecretFromASMWithInput(input *secretsmanager.GetSecretValueInput,
	client secretsmanageriface.SecretsManagerAPI, jsonKey string) (string, error) {
	secretID := *input.SecretId
	out, err := client.GetSecretValue(input)
	if err != nil {
		return "", wrapError(secretID, errors.Wrapf(err, "secret %s", secretID))
	}

	if jsonKey == "" {
		return aws.StringValue(out.SecretString), nil
	}

	secretMap := make(map[string]interface{})
	jsonErr := json.Unmarshal([]byte(*out.SecretString), &secretMap)
	if jsonErr != nil {
		seelog.Warnf("Error when treating retrieved secret value with secret id %s as JSON and calling unmarshal.", *input.SecretId)
		return "", jsonErr
	}

	secretValue, ok := secretMap[jsonKey]
	if !ok {
		err = errors.New(fmt.Sprintf("retrieved secret from Secrets Manager did not contain json key %s", jsonKey))
		return "", err
	}

	return fmt.Sprintf("%v", secretValue), nil
}

// GetSecretFromASM makes the api call to the AWS Secrets Manager service to
// retrieve the secret value
func GetSecretFromASM(secretID string, client secretsmanageriface.SecretsManagerAPI) (string, error) {
	in := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	}

	out, err := client.GetSecretValue(in)
	if err != nil {
		return "", wrapError(secretID, errors.Wrapf(err, "secret %s", secretID))
	}

	return aws.StringValue(out.SecretString), nil
}
