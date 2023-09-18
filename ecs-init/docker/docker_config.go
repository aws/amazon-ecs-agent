// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-init/apparmor"
	"github.com/aws/amazon-ecs-agent/ecs-init/config"
	ctrdapparmor "github.com/containerd/containerd/pkg/apparmor"
	godocker "github.com/fsouza/go-dockerclient"
)

// getPlatformSpecificEnvVariables gets a map of environment variable key-value
// pairs to set in the Agent's container config
// The ECS_ENABLE_TASK_ENI flag is only set for Amazon Linux AMI
func getPlatformSpecificEnvVariables() map[string]string {
	return map[string]string{
		"ECS_ENABLE_TASK_ENI":                       "true",
		"ECS_ENABLE_AWSLOGS_EXECUTIONROLE_OVERRIDE": "true",
	}
}

// createHostConfig creates the host config for the ECS Agent container
// It mounts leases and pid file directories when built for Amazon Linux AMI
func createHostConfig(binds []string) *godocker.HostConfig {
	binds = append(binds,
		config.ProcFS+":"+hostProcDir+readOnly,
		iptablesUsrLibDir+":"+iptablesUsrLibDir+readOnly,
		iptablesLibDir+":"+iptablesLibDir+readOnly,
		iptablesUsrLib64Dir+":"+iptablesUsrLib64Dir+readOnly,
		iptablesLib64Dir+":"+iptablesLib64Dir+readOnly,
		iptablesExecutableHostDir+":"+iptablesExecutableContainerDir+readOnly,
		iptablesAltDir+":"+iptablesAltDir+readOnly,
		iptablesLegacyDir+":"+iptablesLegacyDir+readOnly,
	)

	logConfig := config.AgentDockerLogDriverConfiguration()

	var caps []string
	if !config.RunningInExternal() {
		// CapNetAdmin and CapSysAdmin are needed for running task in awsvpc network mode.
		// This network mode is (at least currently) not supported in external environment,
		// hence not adding them in that case.
		caps = []string{CapNetAdmin, CapSysAdmin}
	}

	hostConfig := &godocker.HostConfig{
		LogConfig:   logConfig,
		Binds:       binds,
		NetworkMode: networkMode,
		UsernsMode:  usernsMode,
		CapAdd:      caps,
		Init:        true,
	}

	if ctrdapparmor.HostSupports() {
		hostConfig.SecurityOpt = []string{fmt.Sprintf("apparmor:%s", apparmor.ECSDefaultProfileName)}
	}

	if config.RunPrivileged() {
		hostConfig.Privileged = true
	}

	return hostConfig
}
