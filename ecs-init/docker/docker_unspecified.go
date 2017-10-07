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
	"github.com/aws/amazon-ecs-init/ecs-init/config"
	godocker "github.com/fsouza/go-dockerclient"
)

// getPlatformSpecificEnvVariables gets a map of environment variable key-value
// pairs to set in the Agent's container config
// The ECS_ENABLE_TASK_ENI flag is only set for Amazon Linux AMI
func getPlatformSpecificEnvVariables() map[string]string {
	return map[string]string{
		"ECS_ENABLE_TASK_ENI": "true",
	}
}

// createHostConfig creates the host config for the ECS Agent container
// It mounts dhclient executable, leases and pid file directories when built
// for Amazon Linux AMI
func createHostConfig(binds []string) *godocker.HostConfig {
	binds = append(binds,
		config.ProcFS+":"+hostProcDir+readOnly,
		config.AgentDHClientLeasesDirectory()+":"+dhclientLeasesLocation,
		dhclientLibDir+":"+dhclientLibDir+readOnly,
		dhclientExecutableDir+":"+dhclientExecutableDir+readOnly)

	return &godocker.HostConfig{
		Binds:       binds,
		NetworkMode: networkMode,
		UsernsMode:  usernsMode,
		CapAdd:      []string{CapNetAdmin, CapSysAdmin},
		Init:        true,
	}
}
