// +build !suse,!ubuntu

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package docker

import (
	godocker "github.com/fsouza/go-dockerclient"
)

// getPlatformSpecificEnvVariables gets a map of environment variable key-value
// pairs to set in the Agent's container config
func getPlatformSpecificEnvVariables() map[string]string {
	return map[string]string{}
}

// createHostConfig creates the host config for the ECS Agent container
func createHostConfig(binds []string) *godocker.HostConfig {
	return &godocker.HostConfig{
		Binds:       binds,
		NetworkMode: networkMode,
		UsernsMode:  usernsMode,
	}
}
