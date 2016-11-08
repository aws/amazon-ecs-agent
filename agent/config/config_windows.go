// +build windows
// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package config

import (
	"os"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

const (
	defaultCredentialsAuditLogFile = `log\audit.log`
	httpPort                       = 80
)

// DefaultConfig returns the default configuration for Windows
func DefaultConfig() Config {
	programData := utils.DefaultIfBlank(os.Getenv("ProgramData"), `C:\ProgramData`)
	ecsRoot := filepath.Join(programData, "Amazon ECS")
	return Config{
		DockerEndpoint:              "npipe:////./pipe/docker_engine",
		ReservedPorts:               []uint16{DockerReservedPort, DockerReservedSSLPort, AgentIntrospectionPort, AgentCredentialsPort},
		ReservedPortsUDP:            []uint16{},
		DataDir:                     filepath.Join(ecsRoot, "data"),
		DisableMetrics:              false,
		ReservedMemory:              0,
		AvailableLoggingDrivers:     []dockerclient.LoggingDriver{dockerclient.JsonFileDriver},
		TaskCleanupWaitDuration:     DefaultTaskCleanupWaitDuration,
		DockerStopTimeout:           DefaultDockerStopTimeout,
		CredentialsAuditLogFile:     filepath.Join(ecsRoot, defaultCredentialsAuditLogFile),
		CredentialsAuditLogDisabled: false,
		ImageCleanupDisabled:        false,
		MinimumImageDeletionAge:     DefaultImageDeletionAge,
		ImageCleanupInterval:        DefaultImageCleanupTimeInterval,
		NumImagesToDeletePerCycle:   DefaultNumImagesToDeletePerCycle,
	}
}

func (config *Config) platformOverrides() {
	// Enabling task IAM roles for Windows requires the credential proxy to run on port 80,
	// so we reserve this port by default when that happens.
	if config.TaskIAMRoleEnabled {
		if config.ReservedPorts == nil {
			config.ReservedPorts = []uint16{}
		}
		config.ReservedPorts = append(config.ReservedPorts, httpPort)
	}
}
