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

// Package dockerauth handles storing auth configuration information for Docker
// registries
package dockerauth

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/credentialprovider"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerauth/ecs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	docker "github.com/fsouza/go-dockerclient"
)

var log = logger.ForModule("docker auth")

var keyring credentialprovider.DockerKeyring

// SetConfig loads credentials from a config
func SetConfig(conf *config.Config) {
	ecs_credentials.SetConfig(conf)
	keyring = credentialprovider.NewDockerKeyring()
}

// GetAuthconfig retrieves the correct auth configuration for the given image
func GetAuthconfig(image string) docker.AuthConfiguration {
	authConfig, ok := keyring.Lookup(image)
	if ok {
		return authConfig
	}
	return docker.AuthConfiguration{}
}
