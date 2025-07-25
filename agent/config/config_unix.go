//go:build !windows
// +build !windows

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

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	netutils "github.com/aws/amazon-ecs-agent/ecs-agent/utils/net"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds"
)

const (
	// AgentCredentialsAddress is used to serve the credentials for tasks.
	AgentCredentialsAddress = "" // this is left blank right now for net=bridge
	// defaultAuditLogFile specifies the default audit log filename
	defaultCredentialsAuditLogFile = "/log/audit.log"

	// defaultRuntimeStatsLogFile stores the path where the golang runtime stats are periodically logged
	defaultRuntimeStatsLogFile = `/log/agent-runtime-stats.log`

	// DefaultTaskCgroupV1Prefix is default cgroup v1 prefix for ECS tasks
	DefaultTaskCgroupV1Prefix = "/ecs"
	// DefaultTaskCgroupV2Prefix is default cgroup v2 prefix for ECS tasks
	// ecstasks is used because this creates a systemd "slice", and using just
	// ecs would create a confusing name conflict with the ecs systemd service.
	// (we would have both ecs.service and ecs.slice in /sys/fs/cgroup).
	DefaultTaskCgroupV2Prefix = "ecstasks"

	// Default cgroup memory system root path, this is the default used if the
	// path has not been configured through ECS_CGROUP_PATH
	defaultCgroupPath = "/sys/fs/cgroup"
	// minimumManifestPullTimeout is the minimum timeout allowed for manifest pulls
	minimumManifestPullTimeout = 30 * time.Second
	// defaultManifestPullTimeout is the default timeout for manifest pulls
	defaultManifestPullTimeout = 1 * time.Minute
	// defaultContainerStartTimeout specifies the value for container start timeout duration
	defaultContainerStartTimeout = 3 * time.Minute
	// minimumContainerStartTimeout specifies the minimum value for starting a container
	minimumContainerStartTimeout = 45 * time.Second
	// defaultContainerCreateTimeout specifies the value for container create timeout duration
	defaultContainerCreateTimeout = 4 * time.Minute
	// minimumContainerCreateTimeout specifies the minimum value for creating a container
	minimumContainerCreateTimeout = 1 * time.Minute
	// default docker inactivity time is extra time needed on container extraction
	defaultImagePullInactivityTimeout = 1 * time.Minute
	// default socket filepath is "/var/run/ecs/ebs-csi-driver/csi-driver.sock"
	defaultCSIDriverSocketPath = "/var/run/ecs/ebs-csi-driver/csi-driver.sock"
	// nodeStageTimeout is the deafult timeout for staging an EBS TA volume
	nodeStageTimeout = 2 * time.Second
	// nodeUnstageTimeout is the deafult timeout for unstaging an EBS TA volume
	nodeUnstageTimeout = 30 * time.Second
)

var (
	nlWrapper = netlinkwrapper.New()
)

// DefaultConfig returns the default configuration for Linux
func DefaultConfig(instanceIPCompatibility ipcompatibility.IPCompatibility) Config {
	shouldExcludeIPv6PortBinding := BooleanDefaultTrue{Value: ExplicitlyEnabled}
	if instanceIPCompatibility.IsIPv6Only() {
		// If the instance is IPv6-only then IPv6 port bindings should be included by default
		shouldExcludeIPv6PortBinding = BooleanDefaultTrue{Value: ExplicitlyDisabled}
	}
	return Config{
		DockerEndpoint:                      "unix:///var/run/docker.sock",
		ReservedPorts:                       []uint16{SSHPort, DockerReservedPort, DockerReservedSSLPort, AgentIntrospectionPort, tmds.Port},
		ReservedPortsUDP:                    []uint16{},
		DataDir:                             "/data/",
		DataDirOnHost:                       "/var/lib/ecs",
		DisableMetrics:                      BooleanDefaultFalse{Value: ExplicitlyDisabled},
		ReservedMemory:                      0,
		AvailableLoggingDrivers:             []dockerclient.LoggingDriver{dockerclient.JSONFileDriver, dockerclient.NoneDriver},
		TaskCleanupWaitDuration:             DefaultTaskCleanupWaitDuration,
		ManifestPullTimeout:                 defaultManifestPullTimeout,
		DockerStopTimeout:                   defaultDockerStopTimeout,
		ContainerStartTimeout:               defaultContainerStartTimeout,
		ContainerCreateTimeout:              defaultContainerCreateTimeout,
		DependentContainersPullUpfront:      BooleanDefaultFalse{Value: ExplicitlyDisabled},
		CredentialsAuditLogFile:             defaultCredentialsAuditLogFile,
		CredentialsAuditLogDisabled:         false,
		ImageCleanupDisabled:                BooleanDefaultFalse{Value: ExplicitlyDisabled},
		MinimumImageDeletionAge:             DefaultImageDeletionAge,
		NonECSMinimumImageDeletionAge:       DefaultNonECSImageDeletionAge,
		ImageCleanupInterval:                DefaultImageCleanupTimeInterval,
		ImagePullInactivityTimeout:          defaultImagePullInactivityTimeout,
		ImagePullTimeout:                    DefaultImagePullTimeout,
		NumImagesToDeletePerCycle:           DefaultNumImagesToDeletePerCycle,
		NumNonECSContainersToDeletePerCycle: DefaultNumNonECSContainersToDeletePerCycle,
		CNIPluginsPath:                      defaultCNIPluginsPath,
		PauseContainerTarballPath:           pauseContainerTarballPath,
		PauseContainerImageName:             DefaultPauseContainerImageName,
		PauseContainerTag:                   DefaultPauseContainerTag,
		AWSVPCBlockInstanceMetdata:          BooleanDefaultFalse{Value: ExplicitlyDisabled},
		ContainerMetadataEnabled:            BooleanDefaultFalse{Value: ExplicitlyDisabled},
		TaskCPUMemLimit:                     BooleanDefaultTrue{Value: NotSet},
		CgroupPath:                          defaultCgroupPath,
		TaskMetadataSteadyStateRate:         DefaultTaskMetadataSteadyStateRate,
		TaskMetadataBurstRate:               DefaultTaskMetadataBurstRate,
		SharedVolumeMatchFullConfig:         BooleanDefaultFalse{Value: ExplicitlyDisabled}, // only requiring shared volumes to match on name, which is default docker behavior
		ContainerInstancePropagateTagsFrom:  ContainerInstancePropagateTagsFromNoneType,
		PrometheusMetricsEnabled:            false,
		PollMetrics:                         BooleanDefaultFalse{Value: NotSet},
		PollingMetricsWaitDuration:          DefaultPollingMetricsWaitDuration,
		NvidiaRuntime:                       DefaultNvidiaRuntime,
		CgroupCPUPeriod:                     defaultCgroupCPUPeriod,
		GMSACapable:                         parseGMSACapability(),
		GMSADomainlessCapable:               parseGMSADomainlessCapability(),
		FSxWindowsFileServerCapable:         BooleanDefaultTrue{Value: ExplicitlyDisabled},
		RuntimeStatsLogFile:                 defaultRuntimeStatsLogFile,
		EnableRuntimeStats:                  BooleanDefaultFalse{Value: NotSet},
		ShouldExcludeIPv6PortBinding:        shouldExcludeIPv6PortBinding,
		CSIDriverSocketPath:                 defaultCSIDriverSocketPath,
		NodeStageTimeout:                    nodeStageTimeout,
		NodeUnstageTimeout:                  nodeUnstageTimeout,
		FirelensAsyncEnabled:                BooleanDefaultTrue{Value: ExplicitlyEnabled},
	}
}

func (cfg *Config) platformOverrides() {
	cfg.PrometheusMetricsEnabled = utils.ParseBool(os.Getenv("ECS_ENABLE_PROMETHEUS_METRICS"), false)
	if cfg.PrometheusMetricsEnabled {
		cfg.ReservedPorts = append(cfg.ReservedPorts, AgentPrometheusExpositionPort)
	}

	if cfg.TaskENIEnabled.Enabled() { // when task networking is enabled, eni trunking is enabled by default
		cfg.ENITrunkingEnabled = parseBooleanDefaultTrueConfig("ECS_ENABLE_HIGH_DENSITY_ENI")
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

func getConfigFileName() (string, error) {
	return utils.DefaultIfBlank(os.Getenv("ECS_AGENT_CONFIG_FILE_PATH"), defaultConfigFileName), nil
}

// Determines and sets IP Compatibility of the container instance.
//
// Fails back to IPv4-only instance IP compatibility status in case there was an issue
// determining the IP compatibility of the container instance.
// This is a fallback to help with graceful adoption of Agent in IPv6-only environments
// without disrupting existing environments.
//
// TODO feat:IPv6-only - Remove lint rule below
//
//lint:ignore U1000 Function will be used in the future
func (c *Config) determineIPCompatibility() {
	var err error
	c.InstanceIPCompatibility, err = netutils.DetermineIPCompatibility(nlWrapper, "")
	if err != nil {
		logger.Warn("Failed to determine instance IP compatibility."+
			" Failing back instance IP compatibility to IPv4-only.",
			logger.Fields{field.Error: err})
		c.InstanceIPCompatibility = ipcompatibility.NewIPv4OnlyCompatibility()
		return
	}

	logger.Info("Successfully determined IP compatibilty of the container instance", logger.Fields{
		"IPv4": c.InstanceIPCompatibility.IsIPv4Compatible(),
		"IPv6": c.InstanceIPCompatibility.IsIPv6Compatible(),
	})
}
