// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package asm

import (
	"encoding/json"
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	log "github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"

	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
)

// asmAuthDataValue is the schema for the SecretStringValue returned by ASM
type asmAuthDataValue struct {
	Username *string
	Password *string
}

func NewASMClient(region string, credential credentials.IAMRoleCredentials) secretsmanageriface.SecretsManagerAPI {
	creds := awscreds.NewStaticCredentials(credential.AccessKeyID, credential.SecretAccessKey, credential.SessionToken)
	cfg := aws.NewConfig().WithRegion(region).WithCredentials(creds)
	sess := session.Must(session.NewSession(cfg))
	return secretsmanager.New(sess)
}

func GetDockerAuthFromASM(secretID string, client secretsmanageriface.SecretsManagerAPI) (docker.AuthConfiguration, error) {

	in := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	}

	log.Debugf("Calling ASM.GetSecretValue for %s", secretID)
	out, err := client.GetSecretValue(in)
	if err != nil {
		return docker.AuthConfiguration{}, err
	}

	dac, err := extractASMValue(out)
	if err != nil {
		return docker.AuthConfiguration{}, err
	}

	return dac, nil
}

func extractASMValue(out *secretsmanager.GetSecretValueOutput) (docker.AuthConfiguration, error) {

	if out == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("missing AuthorizationData in ASM response")
	}

	if out.SecretString == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("AWS Secrets Manager response missing SecretString")
	}

	authDataValue := &asmAuthDataValue{}

	secretstring := aws.StringValue(out.SecretString)
	err := json.Unmarshal([]byte(secretstring), authDataValue)
	if err != nil {
		// could not unmarshal, incorrect secret value schema
		return docker.AuthConfiguration{}, fmt.Errorf("AuthorizationData is malformed")
	}

	if authDataValue.Username == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("AuthorizationData is malformed, username field missing")
	}

	if authDataValue.Password == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("AuthorizationData is malformed, password field missing")
	}

	usernameValue := aws.StringValue(authDataValue.Username)
	passwordValue := aws.StringValue(authDataValue.Password)

	if usernameValue == "" {
		return docker.AuthConfiguration{}, fmt.Errorf("AuthorizationData is malformed, username field cannot be empty")
	}

	if passwordValue == "" {
		return docker.AuthConfiguration{}, fmt.Errorf("AuthorizationData is malformed, password field cannot be empty")
	}

	dac := docker.AuthConfiguration{
		Username: usernameValue,
		Password: passwordValue,
	}

	return dac, nil
}
