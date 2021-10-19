//go:build windows

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
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/cihub/seelog"
	"github.com/hectane/go-acl/api"
)

const (
	// AgentCredentialsAddress is used to serve the credentials for tasks.
	AgentCredentialsAddress = "127.0.0.1"

	// defaultAuditLogFile specifies the default audit log filename
	defaultCredentialsAuditLogFile = `log\audit.log`

	// defaultRuntimeStatsLogFile stores the path where the golang runtime stats are periodically logged
	defaultRuntimeStatsLogFile = `log\agent-runtime-stats.log`

	// When using IAM roles for tasks on Windows, the credential proxy consumes port 80
	httpPort = 80
	// Remote Desktop / Terminal Services
	rdpPort = 3389
	// RPC client
	rpcPort = 135
	// Server Message Block (SMB) over TCP
	smbPort = 445
	// HTTP port for Windows Remote Management (WinRM) listener
	winRMPortHTTP = 5985
	// HTTPS port for Windows Remote Management (WinRM) listener
	winRMPortHTTPS = 5986
	// DNS client
	dnsPort = 53
	// NetBIOS over TCP/IP
	netBIOSPort = 139
	// defaultContainerStartTimeout specifies the value for container start timeout duration
	defaultContainerStartTimeout = 8 * time.Minute
	// minimumContainerStartTimeout specifies the minimum value for starting a container
	minimumContainerStartTimeout = 2 * time.Minute
	// defaultContainerCreateTimeout specifies the value for container create timeout duration
	defaultContainerCreateTimeout = 4 * time.Minute
	// minimumContainerCreateTimeout specifies the minimum value for creating a container
	minimumContainerCreateTimeout = 1 * time.Minute
	// default image pull inactivity time is extra time needed on container extraction
	defaultImagePullInactivityTimeout = 3 * time.Minute
	// adminSid is the security ID for the admin group on Windows
	// Reference: https://docs.microsoft.com/en-us/troubleshoot/windows-server/identity/security-identifiers-in-windows
	adminSid = "S-1-5-32-544"
	// default directory name of CNI Plugins
	defaultCNIPluginDirName = "cni"
)

var (
	envProgramFiles = utils.DefaultIfBlank(os.Getenv("ProgramFiles"), `C:\Program Files`)
	envProgramData  = utils.DefaultIfBlank(os.Getenv("ProgramData"), `C:\ProgramData`)

	AmazonProgramFiles = filepath.Join(envProgramFiles, "Amazon")
	AmazonProgramData  = filepath.Join(envProgramData, "Amazon")

	AmazonECSProgramFiles = filepath.Join(envProgramFiles, "Amazon", "ECS")
	AmazonECSProgramData  = filepath.Join(AmazonProgramData, "ECS")
)

// DefaultConfig returns the default configuration for Windows
func DefaultConfig() Config {
	programData := utils.DefaultIfBlank(os.Getenv("ProgramData"), `C:\ProgramData`)
	ecsRoot := filepath.Join(programData, "Amazon", "ECS")
	dataDir := filepath.Join(ecsRoot, "data")

	programFiles := utils.DefaultIfBlank(os.Getenv("ProgramFiles"), `C:\Program Files`)
	ecsBinaryDir := filepath.Join(programFiles, "Amazon", "ECS")

	platformVariables := PlatformVariables{
		CPUUnbounded:    BooleanDefaultFalse{Value: ExplicitlyDisabled},
		MemoryUnbounded: BooleanDefaultFalse{Value: ExplicitlyDisabled},
	}
	return Config{
		DockerEndpoint: "npipe:////./pipe/docker_engine",
		ReservedPorts: []uint16{
			DockerReservedPort,
			DockerReservedSSLPort,
			AgentIntrospectionPort,
			AgentCredentialsPort,
			rdpPort,
			rpcPort,
			smbPort,
			winRMPortHTTP,
			winRMPortHTTPS,
			dnsPort,
			netBIOSPort,
		},
		ReservedPortsUDP: []uint16{},
		DataDir:          dataDir,
		// DataDirOnHost is identical to DataDir for Windows because we do not
		// run as a container
		DataDirOnHost:                       dataDir,
		ReservedMemory:                      0,
		AvailableLoggingDrivers:             []dockerclient.LoggingDriver{dockerclient.JSONFileDriver, dockerclient.NoneDriver, dockerclient.AWSLogsDriver},
		TaskCleanupWaitDuration:             DefaultTaskCleanupWaitDuration,
		DockerStopTimeout:                   defaultDockerStopTimeout,
		ContainerStartTimeout:               defaultContainerStartTimeout,
		ContainerCreateTimeout:              defaultContainerCreateTimeout,
		DependentContainersPullUpfront:      BooleanDefaultFalse{Value: ExplicitlyDisabled},
		ImagePullInactivityTimeout:          defaultImagePullInactivityTimeout,
		ImagePullTimeout:                    DefaultImagePullTimeout,
		CredentialsAuditLogFile:             filepath.Join(ecsRoot, defaultCredentialsAuditLogFile),
		CredentialsAuditLogDisabled:         false,
		ImageCleanupDisabled:                BooleanDefaultFalse{Value: ExplicitlyDisabled},
		MinimumImageDeletionAge:             DefaultImageDeletionAge,
		NonECSMinimumImageDeletionAge:       DefaultNonECSImageDeletionAge,
		ImageCleanupInterval:                DefaultImageCleanupTimeInterval,
		NumImagesToDeletePerCycle:           DefaultNumImagesToDeletePerCycle,
		NumNonECSContainersToDeletePerCycle: DefaultNumNonECSContainersToDeletePerCycle,
		ContainerMetadataEnabled:            BooleanDefaultFalse{Value: ExplicitlyDisabled},
		TaskCPUMemLimit:                     BooleanDefaultTrue{Value: ExplicitlyDisabled},
		PlatformVariables:                   platformVariables,
		TaskMetadataSteadyStateRate:         DefaultTaskMetadataSteadyStateRate,
		TaskMetadataBurstRate:               DefaultTaskMetadataBurstRate,
		SharedVolumeMatchFullConfig:         BooleanDefaultFalse{Value: ExplicitlyDisabled}, //only requiring shared volumes to match on name, which is default docker behavior
		PollMetrics:                         BooleanDefaultFalse{Value: NotSet},
		PollingMetricsWaitDuration:          DefaultPollingMetricsWaitDuration,
		GMSACapable:                         true,
		FSxWindowsFileServerCapable:         true,
		PauseContainerImageName:             DefaultPauseContainerImageName,
		PauseContainerTag:                   DefaultPauseContainerTag,
		CNIPluginsPath:                      filepath.Join(ecsBinaryDir, defaultCNIPluginDirName),
		RuntimeStatsLogFile:                 filepath.Join(ecsRoot, defaultRuntimeStatsLogFile),
		EnableRuntimeStats:                  BooleanDefaultFalse{Value: NotSet},
		ShouldExcludeIPv6PortBinding:        BooleanDefaultTrue{Value: ExplicitlyEnabled},
	}
}

func (cfg *Config) platformOverrides() {
	// Enabling task IAM roles for Windows requires the credential proxy to run on port 80,
	// so we reserve this port by default when that happens.
	if cfg.TaskIAMRoleEnabled.Enabled() {
		if cfg.ReservedPorts == nil {
			cfg.ReservedPorts = []uint16{}
		}
		cfg.ReservedPorts = append(cfg.ReservedPorts, httpPort)
	}

	// ensure TaskResourceLimit is disabled
	cfg.TaskCPUMemLimit.Value = ExplicitlyDisabled

	cpuUnbounded := parseBooleanDefaultFalseConfig("ECS_ENABLE_CPU_UNBOUNDED_WINDOWS_WORKAROUND")
	memoryUnbounded := parseBooleanDefaultFalseConfig("ECS_ENABLE_MEMORY_UNBOUNDED_WINDOWS_WORKAROUND")

	platformVariables := PlatformVariables{
		CPUUnbounded:    cpuUnbounded,
		MemoryUnbounded: memoryUnbounded,
	}
	cfg.PlatformVariables = platformVariables
}

// platformString returns platform-specific config data that can be serialized
// to string for debugging
func (cfg *Config) platformString() string {
	return ""
}

var getNamedSecurityInfo = api.GetNamedSecurityInfo

// validateConfigFile checks if the config file owner is an admin
// Reference: https://github.com/hectane/go-acl#using-the-api-directly
func validateConfigFile(configFileName string) (bool, error) {
	var (
		Sid    *windows.SID
		handle windows.Handle
	)
	err := getNamedSecurityInfo(
		configFileName,
		api.SE_FILE_OBJECT,
		api.OWNER_SECURITY_INFORMATION,
		&Sid,
		nil,
		nil,
		nil,
		&handle,
	)
	if err != nil {
		return false, err
	}
	defer windows.LocalFree(handle)

	id, err := Sid.String()
	if err != nil {
		return false, err
	}

	if id == adminSid {
		return true, nil
	}
	seelog.Debugf("Non-admin cfg file owner with SID: %v, skip merging into agent config", id)
	return false, nil
}

var osStat = os.Stat

func getConfigFileName() (string, error) {
	fileName := os.Getenv("ECS_AGENT_CONFIG_FILE_PATH")
	// validate the config file only if above env var is not set
	if len(fileName) == 0 {
		fileName = defaultConfigFileName
		// check if the default config file exists before validating it
		_, err := osStat(fileName)
		if err != nil {
			return "", err
		}

		isValidFile, err := validateConfigFile(fileName)
		if err != nil {
			seelog.Errorf("Unable to validate cfg file: %v, err: %v", fileName, err)
			return "", err
		}
		if !isValidFile {
			seelog.Error("Invalid cfg file")
			return "", err
		}
	}
	return fileName, nil
}
