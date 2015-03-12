// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// Package credentialprovider provides a credential provider as defined in
// Kubernetes's 'credentialprovider' package.
// See https://github.com/GoogleCloudPlatform/kubernetes/tree/d5e0054eb0/pkg/credentialprovider
package ecs_credentials

import (
	"encoding/json"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/credentialprovider"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/logger"
)

var log = logger.ForModule("ecs credentials")

type ecsCredentials struct {
	*config.Config
}

// singleton instance
var ecsCredentialsInstance = &ecsCredentials{}

func init() {
	credentialprovider.RegisterCredentialProvider("ecs-config", ecsCredentialsInstance)
}

func SetConfig(cfg *config.Config) {
	ecsCredentialsInstance.Config = cfg
}

func (creds *ecsCredentials) Enabled() bool {
	return true
}

func (creds *ecsCredentials) Provide() credentialprovider.DockerConfig {
	providedCreds := credentialprovider.DockerConfig{}

	switch creds.EngineAuthType {
	case "dockercfg":
		fallthrough
	case "docker":
		err := json.Unmarshal(creds.EngineAuthData, &providedCreds)
		if err != nil {
			log.Error("Unable to decode provided docker credentials", "type", creds.EngineAuthType)
		}
	default:
		log.Error("Unrecognized AuthType type")
	}

	return providedCreds
}
