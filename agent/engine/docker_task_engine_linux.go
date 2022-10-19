//go:build linux
// +build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package engine

import (
	"strings"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	// Constants for CNI timeout during setup and cleanup.
	cniSetupTimeout   = 1 * time.Minute
	cniCleanupTimeout = 30 * time.Second

	defaultKerberosTicketBindPath = "/var/credentials-fetcher/krbdir"
	readOnly                      = ":ro"
)

// updateTaskENIDependencies updates the task's dependencies for awsvpc networking mode.
// This method is used only on Windows platform.
func (engine *DockerTaskEngine) updateTaskENIDependencies(task *apitask.Task) {
}

// invokePluginForContainer is used to invoke the CNI plugin for the given container
// On non-windows platform, we will not invoke CNI plugins for non-pause containers
func (engine *DockerTaskEngine) invokePluginsForContainer(task *apitask.Task, container *apicontainer.Container) error {
	return nil
}

// updateCredentialSpecMapping is used to map the bind location of kerberos ticket to the target location on the application container
func (engine *DockerTaskEngine) updateCredentialSpecMapping(desiredCredSpecInjection string, hostConfig *dockercontainer.HostConfig) {
	// Inject containers' hostConfig.BindMount with the kerberos ticket location
	bindMountKerberosTickets := desiredCredSpecInjection + ":" + defaultKerberosTicketBindPath + readOnly
	if len(hostConfig.Binds) == 0 {
		hostConfig.Binds = []string{bindMountKerberosTickets}
	} else {
		hostConfig.Binds = append(hostConfig.Binds, bindMountKerberosTickets)
	}

	if len(hostConfig.SecurityOpt) != 0 {
		for idx, opt := range hostConfig.SecurityOpt {
			// credentialspec security opt is not supported by docker on linux
			if strings.HasPrefix(opt, "credentialspec:") {
				hostConfig.SecurityOpt = remove(hostConfig.SecurityOpt, idx)
			}
		}
	}
}
