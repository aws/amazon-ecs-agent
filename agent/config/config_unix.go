// +build !windows
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

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

const (
	// AgentCredentialsAddress is used to serve the credentials for tasks.
	AgentCredentialsAddress = "" // this is left blank right now for net=bridge
	// defaultAuditLogFile specifies the default audit log filename
	defaultCredentialsAuditLogFile = "/log/audit.log"
	// Default cgroup prefix for ECS tasks
	DefaultTaskCgroupPrefix = "/ecs"

	// Default cgroup memory system root path, this is the default used if the
	// path has not been configured through ECS_CGROUP_PATH
	defaultCgroupPath = "/sys/fs/cgroup"
	// defaultContainerStartTimeout specifies the value for container start timeout duration
	defaultContainerStartTimeout = 3 * time.Minute
	// minimumContainerStartTimeout specifies the minimum value for starting a container
	minimumContainerStartTimeout = 45 * time.Second
	// default docker inactivity time is extra time needed on container extraction
	defaultImagePullInactivityTimeout = 1 * time.Minute
)

// DefaultConfig returns the default configuration for Linux
func DefaultConfig() Config {
	return Config{
		DockerEndpoint:                      "unix:///var/run/docker.sock",
		ReservedPorts:                       []uint16{SSHPort, DockerReservedPort, DockerReservedSSLPort, AgentIntrospectionPort, AgentCredentialsPort},
		ReservedPortsUDP:                    []uint16{},
		DataDir:                             "/data/",
		DataDirOnHost:                       "/var/lib/ecs",
		DisableMetrics:                      false,
		ReservedMemory:                      0,
		AvailableLoggingDrivers:             []dockerclient.LoggingDriver{dockerclient.JSONFileDriver, dockerclient.NoneDriver},
		TaskCleanupWaitDuration:             DefaultTaskCleanupWaitDuration,
		DockerStopTimeout:                   defaultDockerStopTimeout,
		ContainerStartTimeout:               defaultContainerStartTimeout,
		CredentialsAuditLogFile:             defaultCredentialsAuditLogFile,
		CredentialsAuditLogDisabled:         false,
		ImageCleanupDisabled:                false,
		MinimumImageDeletionAge:             DefaultImageDeletionAge,
		ImageCleanupInterval:                DefaultImageCleanupTimeInterval,
		ImagePullInactivityTimeout:          defaultImagePullInactivityTimeout,
		NumImagesToDeletePerCycle:           DefaultNumImagesToDeletePerCycle,
		NumNonECSContainersToDeletePerCycle: DefaultNumNonECSContainersToDeletePerCycle,
		CNIPluginsPath:                      defaultCNIPluginsPath,
		PauseContainerTarballPath:           pauseContainerTarballPath,
		PauseContainerImageName:             DefaultPauseContainerImageName,
		PauseContainerTag:                   DefaultPauseContainerTag,
		AWSVPCBlockInstanceMetdata:          false,
		ContainerMetadataEnabled:            false,
		TaskCPUMemLimit:                     DefaultEnabled,
		CgroupPath:                          defaultCgroupPath,
		TaskMetadataSteadyStateRate:         DefaultTaskMetadataSteadyStateRate,
		TaskMetadataBurstRate:               DefaultTaskMetadataBurstRate,
		SharedVolumeMatchFullConfig:         false, // only requiring shared volumes to match on name, which is default docker behavior
		ContainerInstancePropagateTagsFrom:  ContainerInstancePropagateTagsFromNoneType,
		PrometheusMetricsEnabled:            false,
		PollMetrics:                         false,
		PollingMetricsWaitDuration:          DefaultPollingMetricsWaitDuration,
		NvidiaRuntime:                       DefaultNvidiaRuntime,
	}
}

func (cfg *Config) platformOverrides() {
	cfg.PrometheusMetricsEnabled = utils.ParseBool(os.Getenv("ECS_ENABLE_PROMETHEUS_METRICS"), false)
	if cfg.PrometheusMetricsEnabled {
		cfg.ReservedPorts = append(cfg.ReservedPorts, AgentPrometheusExpositionPort)
	}
}

// platformString returns platform-specific config data that can be serialized
// to string for debugging
func (cfg *Config) platformString() string {
	// Returns a string if the default image name/tag of the Pause container has
	// been overridden
	if cfg.PauseContainerImageName == DefaultPauseContainerImageName &&
		cfg.PauseContainerTag == DefaultPauseContainerTag {
		return fmt.Sprintf(", PauseContainerImageName: %s, PauseContainerTag: %s",
			cfg.PauseContainerImageName, cfg.PauseContainerTag)
	}
	return ""
}
