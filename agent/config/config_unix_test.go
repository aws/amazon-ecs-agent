//go:build !windows && unit
// +build !windows,unit

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
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"

	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/agent/utils/netlinkwrapper/mocks"
	mock_ec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2/mocks"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	ec2testutil "github.com/aws/amazon-ecs-agent/agent/utils/test/ec2util"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
)

func TestConfigDefault(t *testing.T) {
	defer setTestRegion()()
	os.Unsetenv("ECS_HOST_DATA_DIR")

	cfg, err := NewConfig(ec2testutil.FakeEC2MetadataClient{})
	require.NoError(t, err)

	assert.Equal(t, "unix:///var/run/docker.sock", cfg.DockerEndpoint, "Default docker endpoint set incorrectly")
	assert.Equal(t, "/data/", cfg.DataDir, "Default datadir set incorrectly")
	assert.False(t, cfg.DisableMetrics.Enabled(), "Default disablemetrics set incorrectly")
	assert.Equal(t, 5, len(cfg.ReservedPorts), "Default reserved ports set incorrectly")
	assert.Equal(t, uint16(0), cfg.ReservedMemory, "Default reserved memory set incorrectly")
	assert.Equal(t, 30*time.Second, cfg.DockerStopTimeout, "Default docker stop container timeout set incorrectly")
	assert.Equal(t, 3*time.Minute, cfg.ContainerStartTimeout, "Default docker start container timeout set incorrectly")
	assert.Equal(t, 4*time.Minute, cfg.ContainerCreateTimeout, "Default docker create container timeout set incorrectly")
	assert.Equal(t, 1*time.Minute, cfg.ManifestPullTimeout, "Default manifest pull timeout set incorrectly")
	assert.False(t, cfg.PrivilegedDisabled.Enabled(), "Default PrivilegedDisabled set incorrectly")
	assert.Equal(t, []dockerclient.LoggingDriver{dockerclient.JSONFileDriver, dockerclient.NoneDriver},
		cfg.AvailableLoggingDrivers, "Default logging drivers set incorrectly")
	assert.Equal(t, 3*time.Hour, cfg.TaskCleanupWaitDuration, "Default task cleanup wait duration set incorrectly")
	assert.False(t, cfg.TaskENIEnabled.Enabled(), "TaskENIEnabled set incorrectly")
	assert.False(t, cfg.TaskIAMRoleEnabled.Enabled(), "TaskIAMRoleEnabled set incorrectly")
	assert.False(t, cfg.TaskIAMRoleEnabledForNetworkHost, "TaskIAMRoleEnabledForNetworkHost set incorrectly")
	assert.Equal(t, NotSet, cfg.TaskCPUMemLimit.Value, "TaskCPUMemLimit should be NotSet")
	assert.False(t, cfg.CredentialsAuditLogDisabled, "CredentialsAuditLogDisabled set incorrectly")
	assert.Equal(t, defaultCredentialsAuditLogFile, cfg.CredentialsAuditLogFile, "CredentialsAuditLogFile is set incorrectly")
	assert.False(t, cfg.ImageCleanupDisabled.Enabled(), "ImageCleanupDisabled default is set incorrectly")
	assert.Equal(t, DefaultImageDeletionAge, cfg.MinimumImageDeletionAge, "MinimumImageDeletionAge default is set incorrectly")
	assert.Equal(t, DefaultNonECSImageDeletionAge, cfg.NonECSMinimumImageDeletionAge, "NonECSMinimumImageDeletionAge default is set incorrectly")
	assert.Equal(t, DefaultImageCleanupTimeInterval, cfg.ImageCleanupInterval, "ImageCleanupInterval default is set incorrectly")
	assert.Equal(t, DefaultNumImagesToDeletePerCycle, cfg.NumImagesToDeletePerCycle, "NumImagesToDeletePerCycle default is set incorrectly")
	assert.Equal(t, defaultCNIPluginsPath, cfg.CNIPluginsPath, "CNIPluginsPath default is set incorrectly")
	assert.False(t, cfg.AWSVPCBlockInstanceMetdata.Enabled(), "AWSVPCBlockInstanceMetdata default is incorrectly set")
	assert.Equal(t, "/var/lib/ecs", cfg.DataDirOnHost, "Default DataDirOnHost set incorrectly")
	assert.Equal(t, DefaultTaskMetadataSteadyStateRate, cfg.TaskMetadataSteadyStateRate,
		"Default TaskMetadataSteadyStateRate is set incorrectly")
	assert.Equal(t, DefaultTaskMetadataBurstRate, cfg.TaskMetadataBurstRate,
		"Default TaskMetadataBurstRate is set incorrectly")
	assert.False(t, cfg.SharedVolumeMatchFullConfig.Enabled(), "Default SharedVolumeMatchFullConfig set incorrectly")
	assert.Equal(t, defaultCgroupCPUPeriod, cfg.CgroupCPUPeriod, "CFS cpu period set incorrectly")
	assert.Equal(t, DefaultImagePullTimeout, cfg.ImagePullTimeout, "Default ImagePullTimeout set incorrectly")
	assert.False(t, cfg.DependentContainersPullUpfront.Enabled(), "Default DependentContainersPullUpfront set incorrectly")
	assert.False(t, cfg.PollMetrics.Enabled(), "ECS_POLL_METRICS default should be false")
	assert.False(t, cfg.EnableRuntimeStats.Enabled(), "Default EnableRuntimeStats set incorrectly")
	assert.True(t, cfg.ShouldExcludeIPv6PortBinding.Enabled(), "Default ShouldExcludeIPv6PortBinding set incorrectly")
	assert.False(t, cfg.FSxWindowsFileServerCapable.Enabled(), "Default FSxWindowsFileServerCapable set incorrectly")
	assert.Equal(t, "/var/run/ecs/ebs-csi-driver/csi-driver.sock", cfg.CSIDriverSocketPath, "Default CSIDriverSocketPath set incorrectly")
	assert.Equal(t, 2*time.Second, cfg.NodeStageTimeout, "Default NodeStage timeout set incorrectly")
	assert.Equal(t, 30*time.Second, cfg.NodeUnstageTimeout, "Default NodeUntage timeout set incorrectly")
}

// TestConfigFromFile tests the configuration can be read from file
func TestConfigFromFile(t *testing.T) {
	cluster := "TestCluster"
	dockerAuthType := "dockercfg"
	dockerAuth := `{
  "https://index.docker.io/v1/":{
    "auth":"admin",
    "email":"email"
  }
}`
	testPauseImageName := "pause-image-name"
	testPauseTag := "pause-image-tag"
	content := fmt.Sprintf(`{
  "AWSRegion": "not-real-1",
  "Cluster": "%s",
  "EngineAuthType": "%s",
  "EngineAuthData": %s,
  "DataDir": "/var/run/ecs_agent",
  "TaskIAMRoleEnabled": true,
  "TaskCPUMemLimit": true,
  "InstanceAttributes": {
    "attribute1": "value1"
  },
  "ContainerInstanceTags": {
    "tag1": "value1"
  },
  "PauseContainerImageName":"%s",
  "PauseContainerTag":"%s",
  "AWSVPCAdditionalLocalRoutes":["169.254.172.1/32"]
}`, cluster, dockerAuthType, dockerAuth, testPauseImageName, testPauseTag)

	filePath := setupFileConfiguration(t, content)
	defer os.Remove(filePath)

	defer setTestEnv("ECS_AGENT_CONFIG_FILE_PATH", filePath)()
	defer setTestEnv("AWS_DEFAULT_REGION", "us-west-2")()

	cfg, err := fileConfig()
	assert.NoError(t, err, "reading configuration from file failed")

	assert.Equal(t, cluster, cfg.Cluster, "cluster name not as expected from file")
	assert.Equal(t, dockerAuthType, cfg.EngineAuthType, "docker auth type not as expected from file")
	assert.Equal(t, dockerAuth, string(cfg.EngineAuthData.Contents()), "docker auth data not as expected from file")
	assert.Equal(t, map[string]string{"attribute1": "value1"}, cfg.InstanceAttributes)
	assert.Equal(t, map[string]string{"tag1": "value1"}, cfg.ContainerInstanceTags)
	assert.Equal(t, testPauseImageName, cfg.PauseContainerImageName, "should read PauseContainerImageName")
	assert.Equal(t, testPauseTag, cfg.PauseContainerTag, "should read PauseContainerTag")
	assert.Equal(t, 1, len(cfg.AWSVPCAdditionalLocalRoutes), "should have one additional local route")
	expectedLocalRoute, err := cniTypes.ParseCIDR("169.254.172.1/32")
	assert.NoError(t, err)
	assert.Equal(t, expectedLocalRoute.IP, cfg.AWSVPCAdditionalLocalRoutes[0].IP, "should match expected route IP")
	assert.Equal(t, expectedLocalRoute.Mask, cfg.AWSVPCAdditionalLocalRoutes[0].Mask, "should match expected route Mask")
	assert.Equal(t, ExplicitlyEnabled, cfg.TaskCPUMemLimit.Value, "TaskCPUMemLimit should be explicitly enabled")
}

// TestDockerAuthMergeFromFile tests docker auth read from file correctly after merge
func TestDockerAuthMergeFromFile(t *testing.T) {
	cluster := "myCluster"
	dockerAuthType := "dockercfg"
	dockerAuth := `{
  "https://index.docker.io/v1/":{
    "auth":"admin",
    "email":"email"
  }
}`
	content := fmt.Sprintf(`{
  "AWSRegion": "not-real-1",
  "Cluster": "TestCluster",
  "EngineAuthType": "%s",
  "EngineAuthData": %s,
  "DataDir": "/var/run/ecs_agent",
  "TaskIAMRoleEnabled": true,
  "InstanceAttributes": {
    "attribute1": "value1"
  },
  "ContainerInstanceTags": {
    "tag1": "value1"
  }
}`, dockerAuthType, dockerAuth)

	filePath := setupFileConfiguration(t, content)
	defer os.Remove(filePath)

	defer setTestEnv("ECS_CLUSTER", cluster)()
	defer setTestEnv("ECS_AGENT_CONFIG_FILE_PATH", filePath)()
	defer setTestEnv("AWS_DEFAULT_REGION", "us-west-2")()

	cfg, err := NewConfig(ec2testutil.FakeEC2MetadataClient{})
	assert.NoError(t, err, "create configuration failed")

	assert.Equal(t, cluster, cfg.Cluster, "cluster name not as expected from environment variable")
	assert.Equal(t, dockerAuthType, cfg.EngineAuthType, "docker auth type not as expected from file")
	assert.Equal(t, dockerAuth, string(cfg.EngineAuthData.Contents()), "docker auth data not as expected from file")
	assert.Equal(t, map[string]string{"attribute1": "value1"}, cfg.InstanceAttributes)
	assert.Equal(t, map[string]string{"tag1": "value1"}, cfg.ContainerInstanceTags)
}

func TestBadFileContent(t *testing.T) {
	content := `{
	"AWSRegion": "not-real-1",
	"AWSVPCAdditionalLocalRoutes":["169.254.172.1/32", "300.300.300.300/32", "foo"]
	}`

	filePath := setupFileConfiguration(t, content)
	defer os.Remove(filePath)

	os.Setenv("ECS_AGENT_CONFIG_FILE_PATH", filePath)
	defer os.Unsetenv("ECS_AGENT_CONFIG_FILE_PATH")

	_, err := NewConfig(ec2testutil.FakeEC2MetadataClient{})
	assert.Error(t, err, "create configuration should fail")
}

func TestPrometheusMetricsPlatformOverrides(t *testing.T) {
	defer setTestRegion()()
	cfg, err := NewConfig(ec2testutil.FakeEC2MetadataClient{})
	require.NoError(t, err)

	defer setTestEnv("ECS_ENABLE_PROMETHEUS_METRICS", "true")()
	cfg.platformOverrides()
	assert.True(t, cfg.PrometheusMetricsEnabled, "Prometheus metrics should be enabled")
	assert.Equal(t, 6, len(cfg.ReservedPorts), "Reserved ports should have added Prometheus endpoint")
}

// TestENITrunkingEnabled tests that when task networking is enabled, eni trunking is enabled by default
func TestENITrunkingEnabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_TASK_ENI", "true")()
	cfg, err := NewConfig(ec2testutil.FakeEC2MetadataClient{})
	require.NoError(t, err)

	cfg.platformOverrides()
	assert.True(t, cfg.ENITrunkingEnabled.Enabled(), "ENI trunking should be enabled")
}

// TestENITrunkingDisabled tests that when task networking is enabled, eni trunking can be disabled
func TestENITrunkingDisabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_TASK_ENI", "true")()
	cfg, err := NewConfig(ec2testutil.FakeEC2MetadataClient{})
	require.NoError(t, err)

	defer setTestEnv("ECS_ENABLE_HIGH_DENSITY_ENI", "false")()
	cfg.platformOverrides()
	assert.False(t, cfg.ENITrunkingEnabled.Enabled(), "ENI trunking should be disabled")
}

// setupFileConfiguration create a temp file store the configuration
func setupFileConfiguration(t *testing.T, configContent string) string {
	file, err := ioutil.TempFile("", "ecs-test")
	require.NoError(t, err, "creating temp file for configuration failed")

	_, err = file.Write([]byte(configContent))
	require.NoError(t, err, "writing configuration to file failed")

	return file.Name()
}

func TestEmptyNvidiaRuntime(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_NVIDIA_RUNTIME", "")()
	cfg, err := NewConfig(ec2testutil.FakeEC2MetadataClient{})
	assert.NoError(t, err)
	assert.Equal(t, DefaultNvidiaRuntime, cfg.NvidiaRuntime, "Wrong value for NvidiaRuntime")
}

func TestCPUPeriodSettings(t *testing.T) {
	cases := []struct {
		Name     string
		Env      string
		Response time.Duration
	}{
		{
			Name:     "OverrideDefaultCPUPeriod",
			Env:      "10ms",
			Response: 10 * time.Millisecond,
		},
		{
			Name:     "DefaultCPUPeriod",
			Env:      "",
			Response: defaultCgroupCPUPeriod,
		},
		{
			Name:     "TestCPUPeriodUpperBoundLimit",
			Env:      "110ms",
			Response: defaultCgroupCPUPeriod,
		},
		{
			Name:     "TestCPUPeriodLowerBoundLimit",
			Env:      "7ms",
			Response: defaultCgroupCPUPeriod,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			defer setTestRegion()()
			defer os.Setenv("ECS_CGROUP_CPU_PERIOD", "100ms")

			os.Setenv("ECS_CGROUP_CPU_PERIOD", c.Env)
			conf, err := NewConfig(ec2testutil.FakeEC2MetadataClient{})

			assert.NoError(t, err)
			assert.Equal(t, c.Response, conf.CgroupCPUPeriod, "Wrong value for CgroupCPUPeriod")
		})
	}
}

func TestDetermineIPCompatibility(t *testing.T) {
	ipv6Gw := net.ParseIP("1:2:3:4::")
	require.NotNil(t, ipv6Gw)
	ipv6DefaultRoute := netlink.Route{Gw: ipv6Gw, Dst: nil}

	testCases := []struct {
		name            string
		setExpectations func(*mock_ec2.MockEC2MetadataClient, *mock_netlinkwrapper.MockNetLink)
		expected        ipcompatibility.IPCompatibility
	}{
		{
			name: "IP compatibility determined successfully",
			setExpectations: func(ec2c *mock_ec2.MockEC2MetadataClient, nl *mock_netlinkwrapper.MockNetLink) {
				nl.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return([]netlink.Route{}, nil)
				nl.EXPECT().RouteList(nil, netlink.FAMILY_V6).Return([]netlink.Route{ipv6DefaultRoute}, nil)
			},
			expected: ipcompatibility.NewIPCompatibility(false, true),
		},
		{
			name: "IP compatibility could not be determined",
			setExpectations: func(ec2c *mock_ec2.MockEC2MetadataClient, nl *mock_netlinkwrapper.MockNetLink) {
				nl.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return(nil, errors.New("some error"))
			},
			expected: ipcompatibility.NewIPv4OnlyCompatibility(),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Init mock
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			imdsClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
			mockNLWrapper := mock_netlinkwrapper.NewMockNetLink(ctrl)

			// Set up mock netlink wrapper
			ogNLWrapper := nlWrapper
			defer func() {
				nlWrapper = ogNLWrapper
			}()
			nlWrapper = mockNLWrapper

			// Set up mock expectations
			tc.setExpectations(imdsClient, mockNLWrapper)

			// Run the test
			cfg := Config{}
			cfg.determineIPCompatibility(imdsClient)
			assert.Equal(t, tc.expected, cfg.InstanceIPCompatibility)
		})
	}
}

// Tests that IPCompatibility defaults to IPv4-only when determining IP compatibility of
// the container instance fails due to some error.
func TestIPCompatibilityFallback(t *testing.T) {
	// TODO feat:IPv6-only - Remove skip
	t.Skip("Enable when launching IPv6-only support")
	defer setTestRegion()()
	ctrl := gomock.NewController(t)
	mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)

	mockEc2Metadata.EXPECT().PrimaryENIMAC().Return("invalid", nil) // fails to parse
	mockEc2Metadata.EXPECT().GetUserData()

	config, err := NewConfig(mockEc2Metadata)
	assert.NoError(t, err)
	assert.Equal(t, config.InstanceIPCompatibility.IsIPv4Compatible(), true)
	assert.Equal(t, config.InstanceIPCompatibility.IsIPv6Compatible(), false)
}

func TestShouldExcludeIPv6PortBindingDefault(t *testing.T) {
	t.Run("ipv6-only instance", func(t *testing.T) {
		assert.False(t,
			DefaultConfig(ipcompatibility.NewIPv6OnlyCompatibility()).
				ShouldExcludeIPv6PortBinding.Enabled())
	})
	t.Run("dual-stack instance", func(t *testing.T) {
		assert.True(t,
			DefaultConfig(ipcompatibility.NewDualStackCompatibility()).
				ShouldExcludeIPv6PortBinding.Enabled())
	})
	t.Run("ipv4-only instance", func(t *testing.T) {
		assert.True(t,
			DefaultConfig(ipcompatibility.NewIPv4OnlyCompatibility()).
				ShouldExcludeIPv6PortBinding.Enabled())
	})
	t.Run("no ip compatibility", func(t *testing.T) {
		assert.True(t,
			DefaultConfig(ipcompatibility.NewIPCompatibility(false, false)).
				ShouldExcludeIPv6PortBinding.Enabled())
	})
}
