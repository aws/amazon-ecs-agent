//go:build windows && unit
// +build windows,unit

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
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds"

	"github.com/hectane/go-acl/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDefault(t *testing.T) {
	defer setTestRegion()()
	os.Unsetenv("ECS_HOST_DATA_DIR")

	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	require.NoError(t, err)

	assert.Equal(t, "npipe:////./pipe/docker_engine", cfg.DockerEndpoint, "Default docker endpoint set incorrectly")
	assert.Equal(t, `C:\ProgramData\Amazon\ECS\data`, cfg.DataDir, "Default datadir set incorrectly")
	assert.False(t, cfg.DisableMetrics.Enabled(), "Default disablemetrics set incorrectly")
	assert.Equal(t, 11, len(cfg.ReservedPorts), "Default reserved ports set incorrectly")
	assert.Equal(t, uint16(0), cfg.ReservedMemory, "Default reserved memory set incorrectly")
	assert.Equal(t, 30*time.Second, cfg.DockerStopTimeout, "Default docker stop container timeout set incorrectly")
	assert.Equal(t, 8*time.Minute, cfg.ContainerStartTimeout, "Default docker start container timeout set incorrectly")
	assert.Equal(t, 4*time.Minute, cfg.ContainerCreateTimeout, "Default docker create container timeout set incorrectly")
	assert.False(t, cfg.PrivilegedDisabled.Enabled(), "Default PrivilegedDisabled set incorrectly")
	assert.Equal(t, []dockerclient.LoggingDriver{dockerclient.JSONFileDriver, dockerclient.NoneDriver, dockerclient.AWSLogsDriver},
		cfg.AvailableLoggingDrivers, "Default logging drivers set incorrectly")
	assert.Equal(t, 3*time.Hour, cfg.TaskCleanupWaitDuration, "Default task cleanup wait duration set incorrectly")
	assert.False(t, cfg.TaskIAMRoleEnabled.Enabled(), "TaskIAMRoleEnabled set incorrectly")
	assert.False(t, cfg.TaskIAMRoleEnabledForNetworkHost, "TaskIAMRoleEnabledForNetworkHost set incorrectly")
	assert.False(t, cfg.CredentialsAuditLogDisabled, "CredentialsAuditLogDisabled set incorrectly")
	assert.Equal(t, `C:\ProgramData\Amazon\ECS\log\audit.log`, cfg.CredentialsAuditLogFile, "CredentialsAuditLogFile is set incorrectly")
	assert.False(t, cfg.ImageCleanupDisabled.Enabled(), "ImageCleanupDisabled default is set incorrectly")
	assert.Equal(t, DefaultImageDeletionAge, cfg.MinimumImageDeletionAge, "MinimumImageDeletionAge default is set incorrectly")
	assert.Equal(t, DefaultNonECSImageDeletionAge, cfg.NonECSMinimumImageDeletionAge, "NonECSMinimumImageDeletionAge default is set incorrectly")
	assert.Equal(t, DefaultImageCleanupTimeInterval, cfg.ImageCleanupInterval, "ImageCleanupInterval default is set incorrectly")
	assert.Equal(t, DefaultNumImagesToDeletePerCycle, cfg.NumImagesToDeletePerCycle, "NumImagesToDeletePerCycle default is set incorrectly")
	assert.Equal(t, `C:\ProgramData\Amazon\ECS\data`, cfg.DataDirOnHost, "Default DataDirOnHost set incorrectly")
	assert.False(t, cfg.PlatformVariables.CPUUnbounded.Enabled(), "CPUUnbounded should be false by default")
	assert.Equal(t, DefaultTaskMetadataSteadyStateRate, cfg.TaskMetadataSteadyStateRate,
		"Default TaskMetadataSteadyStateRate is set incorrectly")
	assert.Equal(t, DefaultTaskMetadataBurstRate, cfg.TaskMetadataBurstRate,
		"Default TaskMetadataBurstRate is set incorrectly")
	assert.False(t, cfg.SharedVolumeMatchFullConfig.Enabled(), "Default SharedVolumeMatchFullConfig set incorrectly")
	assert.Equal(t, DefaultImagePullTimeout, cfg.ImagePullTimeout, "Default ImagePullTimeout set incorrectly")
	assert.False(t, cfg.DependentContainersPullUpfront.Enabled(), "Default DependentContainersPullUpfront set incorrectly")
	assert.False(t, cfg.EnableRuntimeStats.Enabled(), "Default EnableRuntimeStats set incorrectly")
	assert.True(t, cfg.ShouldExcludeIPv6PortBinding.Enabled(), "Default ShouldExcludeIPv6PortBinding set incorrectly")
	assert.True(t, cfg.FSxWindowsFileServerCapable.Enabled(), "Default FSxWindowsFileServerCapable set incorrectly")
	assert.Equal(t, "C:\\ProgramData\\Amazon\\ECS\\ebs-csi-driver\\csi-driver.sock", cfg.CSIDriverSocketPath, "Default CSIDriverSocketPath set incorrectly")
}

func TestConfigIAMTaskRolesReserves80(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_TASK_IAM_ROLE", "true")()

	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, []uint16{
		DockerReservedPort,
		DockerReservedSSLPort,
		AgentIntrospectionPort,
		tmds.Port,
		rdpPort,
		rpcPort,
		smbPort,
		winRMPortHTTP,
		winRMPortHTTPS,
		dnsPort,
		netBIOSPort,
		httpPort,
	}, cfg.ReservedPorts)

	defer setTestEnv("ECS_RESERVED_PORTS", "[1]")()
	cfg, err = NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, []uint16{1, httpPort}, cfg.ReservedPorts)
}

func TestTaskResourceLimitPlatformOverrideDisabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_TASK_CPU_MEM_LIMIT", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.platformOverrides()
	assert.NoError(t, err)
	assert.False(t, cfg.TaskCPUMemLimit.Enabled())
}

func TestCPUUnboundedSet(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_CPU_UNBOUNDED_WINDOWS_WORKAROUND", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.platformOverrides()
	assert.NoError(t, err)
	assert.True(t, cfg.PlatformVariables.CPUUnbounded.Enabled())
}

func TestCPUUnboundedWindowsDisabled(t *testing.T) {
	defer setTestRegion()()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.platformOverrides()
	assert.NoError(t, err)
	assert.False(t, cfg.PlatformVariables.CPUUnbounded.Enabled())
}

func TestMemoryUnboundedSet(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_MEMORY_UNBOUNDED_WINDOWS_WORKAROUND", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.platformOverrides()
	assert.NoError(t, err)
	assert.True(t, cfg.PlatformVariables.MemoryUnbounded.Enabled())
}

func TestMemoryUnboundedWindowsDisabled(t *testing.T) {
	defer setTestRegion()()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.platformOverrides()
	assert.NoError(t, err)
	assert.False(t, cfg.PlatformVariables.MemoryUnbounded.Enabled())
}

func TestGetConfigFileName(t *testing.T) {
	configFileName := "/foo/bar/config.json"
	testCases := []struct {
		name             string
		envVarVal        string
		expectedFileName string
		expectedError    error
		cfgFileSid       string
	}{
		{
			name:             "config file via env var, no errors",
			envVarVal:        configFileName,
			expectedFileName: configFileName,
			expectedError:    nil,
			cfgFileSid:       "",
		},
		{
			name:             "default config file without env var, no errors",
			envVarVal:        "",
			expectedFileName: defaultConfigFileName,
			expectedError:    nil,
			cfgFileSid:       adminSid,
		},
		{
			name:             "unable to validate cfg file error",
			envVarVal:        "",
			expectedFileName: "",
			expectedError:    errors.New("Unable to validate cfg file"),
			cfgFileSid:       "random-sid",
		},
		{
			name:             "invalid cfg file error",
			envVarVal:        "",
			expectedFileName: "",
			expectedError:    errors.New("Invalid cfg file"),
			cfgFileSid:       "S-1-5-7",
		},
	}
	defer func() {
		osStat = os.Stat
		getNamedSecurityInfo = api.GetNamedSecurityInfo
	}()

	for _, tc := range testCases {
		os.Setenv("ECS_AGENT_CONFIG_FILE_PATH", tc.envVarVal)
		defer os.Unsetenv("ECS_AGENT_CONFIG_FILE_PATH")

		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		getNamedSecurityInfo = func(fileName string, fileType int32, secInfo uint32, owner,
			group **windows.SID, dacl, sacl, secDesc *windows.Handle) error {
			*owner, _ = windows.StringToSid(tc.cfgFileSid)
			return tc.expectedError
		}

		fileName, err := getConfigFileName()
		assert.Equal(t, tc.expectedFileName, fileName)
		assert.Equal(t, tc.expectedError, err)
	}
}
