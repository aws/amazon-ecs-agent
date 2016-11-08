// +build !windows
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

import "github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"

const (
	// defaultAuditLogFile specifies the default audit log filename
	defaultCredentialsAuditLogFile = "/log/audit.log"
)

// DefaultConfig returns the default configuration for Linux
func DefaultConfig() Config {
	return Config{
		DockerEndpoint:              "unix:///var/run/docker.sock",
		ReservedPorts:               []uint16{SSHPort, DockerReservedPort, DockerReservedSSLPort, AgentIntrospectionPort, AgentCredentialsPort},
		ReservedPortsUDP:            []uint16{},
		DataDir:                     "/data/",
		DisableMetrics:              false,
		ReservedMemory:              0,
		AvailableLoggingDrivers:     []dockerclient.LoggingDriver{dockerclient.JsonFileDriver},
		TaskCleanupWaitDuration:     DefaultTaskCleanupWaitDuration,
		DockerStopTimeout:           DefaultDockerStopTimeout,
		CredentialsAuditLogFile:     defaultCredentialsAuditLogFile,
		CredentialsAuditLogDisabled: false,
		ImageCleanupDisabled:        false,
		MinimumImageDeletionAge:     DefaultImageDeletionAge,
		ImageCleanupInterval:        DefaultImageCleanupTimeInterval,
		NumImagesToDeletePerCycle:   DefaultNumImagesToDeletePerCycle,
	}
}

func (config *Config) platformOverrides() {}
