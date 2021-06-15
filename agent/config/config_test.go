// +build unit

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
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	mock_ec2 "github.com/aws/amazon-ecs-agent/agent/ec2/mocks"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	conf1 := &Config{Cluster: "Foo"}
	conf2 := Config{Cluster: "ignored", APIEndpoint: "Bar"}
	conf3 := Config{AWSRegion: "us-west-2"}

	conf1.Merge(conf2).Merge(conf3)

	assert.Equal(t, "Foo", conf1.Cluster, "The cluster should not have been overridden")
	assert.Equal(t, "Bar", conf1.APIEndpoint, "The APIEndpoint should have been merged in")
	assert.Equal(t, "us-west-2", conf1.AWSRegion, "Incorrect region")
}

// TODO: we need to move all boolean configs to use BooleanDefaultTrue/False
// Below TestBooleanMerge* tests pass for one that's been migrated.
func TestBooleanMergeFalseNotOverridden(t *testing.T) {
	conf1 := &Config{Cluster: "Foo"}
	conf2 := Config{Cluster: "ignored", DeleteNonECSImagesEnabled: BooleanDefaultFalse{Value: ExplicitlyDisabled}}
	conf3 := Config{AWSRegion: "us-west-2", DeleteNonECSImagesEnabled: BooleanDefaultFalse{Value: ExplicitlyEnabled}}

	conf1.Merge(conf2).Merge(conf3)

	assert.Equal(t, "Foo", conf1.Cluster, "The cluster should not have been overridden")
	assert.Equal(t, ExplicitlyDisabled, conf1.DeleteNonECSImagesEnabled.Value, "The DeleteNonECSImagesEnabled should not have been overridden")
	assert.Equal(t, "us-west-2", conf1.AWSRegion, "Incorrect region")
}

func TestBooleanMergeNotSetOverridden(t *testing.T) {
	conf1 := &Config{Cluster: "Foo"}
	conf2 := Config{Cluster: "ignored", DeleteNonECSImagesEnabled: BooleanDefaultFalse{Value: NotSet}}
	conf3 := Config{AWSRegion: "us-west-2", DeleteNonECSImagesEnabled: BooleanDefaultFalse{Value: ExplicitlyDisabled}}

	conf1.Merge(conf2).Merge(conf3)

	assert.Equal(t, "Foo", conf1.Cluster, "The cluster should not have been overridden")
	assert.Equal(t, ExplicitlyDisabled, conf1.DeleteNonECSImagesEnabled.Value, "The DeleteNonECSImagesEnabled should have been overridden")
	assert.Equal(t, "us-west-2", conf1.AWSRegion, "Incorrect region")
}

func TestBrokenEC2Metadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockEc2Metadata.EXPECT().InstanceIdentityDocument().Return(ec2metadata.EC2InstanceIdentityDocument{}, errors.New("err"))
	mockEc2Metadata.EXPECT().GetUserData()

	_, err := NewConfig(mockEc2Metadata)
	assert.Error(t, err, "Expected error when region isn't set and metadata doesn't work")
}

func TestBrokenEC2MetadataEndpoint(t *testing.T) {
	defer setTestRegion()()
	ctrl := gomock.NewController(t)
	mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)

	mockEc2Metadata.EXPECT().InstanceIdentityDocument().Return(ec2metadata.EC2InstanceIdentityDocument{}, errors.New("err"))
	mockEc2Metadata.EXPECT().GetUserData()

	config, err := NewConfig(mockEc2Metadata)
	assert.NoError(t, err)
	assert.Equal(t, config.AWSRegion, "us-west-2", "Wrong region")
	assert.Zero(t, config.APIEndpoint, "Endpoint env variable not set; endpoint should be blank")
}

func TestGetRegionWithNoIID(t *testing.T) {
	defer setTestEnv("AWS_DEFAULT_REGION", "")()
	ctrl := gomock.NewController(t)
	mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)

	userDataResponse := `{ "ECSAgentConfiguration":{
		"Cluster":"arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster",
		"APIEndpoint":"https://some-endpoint.com",
		"NoIID":true
	}}`
	mockEc2Metadata.EXPECT().GetUserData().Return(userDataResponse, nil)
	mockEc2Metadata.EXPECT().Region().Return("us-east-1", nil)

	config, err := NewConfig(mockEc2Metadata)
	assert.NoError(t, err)
	assert.Equal(t, "us-east-1", config.AWSRegion, "Wrong region")
	assert.Equal(t, "https://some-endpoint.com", config.APIEndpoint, "Endpoint env variable not set; endpoint should be blank")
}

func TestEnvironmentConfig(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CLUSTER", "myCluster")()
	defer setTestEnv("ECS_RESERVED_PORTS_UDP", "[42,99]")()
	defer setTestEnv("ECS_RESERVED_MEMORY", "20")()
	defer setTestEnv("ECS_CONTAINER_STOP_TIMEOUT", "60s")()
	defer setTestEnv("ECS_CONTAINER_START_TIMEOUT", "5m")()
	defer setTestEnv("ECS_CONTAINER_CREATE_TIMEOUT", "4m")()
	defer setTestEnv("ECS_IMAGE_PULL_INACTIVITY_TIMEOUT", "10m")()
	defer setTestEnv("ECS_AVAILABLE_LOGGING_DRIVERS", "[\""+string(dockerclient.SyslogDriver)+"\"]")()
	defer setTestEnv("ECS_SELINUX_CAPABLE", "true")()
	defer setTestEnv("ECS_APPARMOR_CAPABLE", "true")()
	defer setTestEnv("ECS_DISABLE_PRIVILEGED", "true")()
	defer setTestEnv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "90s")()
	defer setTestEnv("ECS_ENABLE_TASK_IAM_ROLE", "true")()
	defer setTestEnv("ECS_ENABLE_UNTRACKED_IMAGE_CLEANUP", "true")()
	defer setTestEnv("ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST", "true")()
	defer setTestEnv("ECS_DISABLE_IMAGE_CLEANUP", "true")()
	defer setTestEnv("ECS_IMAGE_CLEANUP_INTERVAL", "2h")()
	defer setTestEnv("ECS_IMAGE_MINIMUM_CLEANUP_AGE", "30m")()
	defer setTestEnv("NON_ECS_IMAGE_MINIMUM_CLEANUP_AGE", "30m")()
	defer setTestEnv("ECS_NUM_IMAGES_DELETE_PER_CYCLE", "2")()
	defer setTestEnv("ECS_IMAGE_PULL_BEHAVIOR", "always")()
	defer setTestEnv("ECS_INSTANCE_ATTRIBUTES", "{\"my_attribute\": \"testing\"}")()
	defer setTestEnv("ECS_CONTAINER_INSTANCE_TAGS", `{"my_tag": "testing"}`)()
	defer setTestEnv("ECS_ENABLE_TASK_ENI", "true")()
	defer setTestEnv("ECS_TASK_METADATA_RPS_LIMIT", "1000,1100")()
	defer setTestEnv("ECS_SHARED_VOLUME_MATCH_FULL_CONFIG", "true")()
	defer setTestEnv("ECS_ENABLE_GPU_SUPPORT", "true")()
	defer setTestEnv("ECS_DISABLE_TASK_METADATA_AZ", "true")()
	defer setTestEnv("ECS_NVIDIA_RUNTIME", "nvidia")()
	defer setTestEnv("ECS_POLL_METRICS", "true")()
	defer setTestEnv("ECS_POLLING_METRICS_WAIT_DURATION", "10s")()
	defer setTestEnv("ECS_CGROUP_CPU_PERIOD", "")
	defer setTestEnv("ECS_PULL_DEPENDENT_CONTAINERS_UPFRONT", "true")()
	additionalLocalRoutesJSON := `["1.2.3.4/22","5.6.7.8/32"]`
	setTestEnv("ECS_AWSVPC_ADDITIONAL_LOCAL_ROUTES", additionalLocalRoutesJSON)
	setTestEnv("ECS_ENABLE_CONTAINER_METADATA", "true")
	setTestEnv("ECS_HOST_DATA_DIR", "/etc/ecs/")
	setTestEnv("ECS_CGROUP_CPU_PERIOD", "10ms")
	setTestEnv("ECS_VOLUME_PLUGIN_CAPABILITIES", "[\"efsAuth\"]")

	conf, err := environmentConfig()
	assert.NoError(t, err)
	assert.Equal(t, "myCluster", conf.Cluster)
	assert.Equal(t, 2, len(conf.ReservedPortsUDP))
	assert.Contains(t, conf.ReservedPortsUDP, uint16(42))
	assert.Contains(t, conf.ReservedPortsUDP, uint16(99))
	assert.Equal(t, uint16(20), conf.ReservedMemory)
	expectedDurationDockerStopTimeout, _ := time.ParseDuration("60s")
	assert.Equal(t, expectedDurationDockerStopTimeout, conf.DockerStopTimeout)
	expectedDurationContainerStartTimeout, _ := time.ParseDuration("5m")
	assert.Equal(t, expectedDurationContainerStartTimeout, conf.ContainerStartTimeout)
	expectedDurationContainerCreateTimeout, _ := time.ParseDuration("4m")
	assert.Equal(t, expectedDurationContainerCreateTimeout, conf.ContainerCreateTimeout)
	assert.Equal(t, []dockerclient.LoggingDriver{dockerclient.SyslogDriver}, conf.AvailableLoggingDrivers)
	assert.True(t, conf.PrivilegedDisabled.Enabled())
	assert.True(t, conf.SELinuxCapable.Enabled(), "Wrong value for SELinuxCapable")
	assert.True(t, conf.AppArmorCapable.Enabled(), "Wrong value for AppArmorCapable")
	assert.True(t, conf.TaskIAMRoleEnabled.Enabled(), "Wrong value for TaskIAMRoleEnabled")
	assert.Equal(t, ExplicitlyEnabled, conf.DeleteNonECSImagesEnabled.Value, "Wrong value for DeleteNonECSImagesEnabled")
	assert.True(t, conf.TaskIAMRoleEnabledForNetworkHost, "Wrong value for TaskIAMRoleEnabledForNetworkHost")
	assert.True(t, conf.ImageCleanupDisabled.Enabled(), "Wrong value for ImageCleanupDisabled")
	assert.True(t, conf.PollMetrics.Enabled(), "Wrong value for PollMetrics")
	expectedDurationPollingMetricsWaitDuration, _ := time.ParseDuration("10s")
	assert.Equal(t, expectedDurationPollingMetricsWaitDuration, conf.PollingMetricsWaitDuration)
	assert.True(t, conf.TaskENIEnabled.Enabled(), "Wrong value for TaskNetwork")
	assert.Equal(t, (30 * time.Minute), conf.MinimumImageDeletionAge)
	assert.Equal(t, (30 * time.Minute), conf.NonECSMinimumImageDeletionAge)
	assert.Equal(t, (2 * time.Hour), conf.ImageCleanupInterval)
	assert.Equal(t, 2, conf.NumImagesToDeletePerCycle)
	assert.Equal(t, ImagePullAlwaysBehavior, conf.ImagePullBehavior)
	assert.Equal(t, "testing", conf.InstanceAttributes["my_attribute"])
	assert.Equal(t, "testing", conf.ContainerInstanceTags["my_tag"])
	assert.Equal(t, (90 * time.Second), conf.TaskCleanupWaitDuration)
	serializedAdditionalLocalRoutesJSON, err := json.Marshal(conf.AWSVPCAdditionalLocalRoutes)
	assert.NoError(t, err, "should marshal additional local routes")
	assert.Equal(t, additionalLocalRoutesJSON, string(serializedAdditionalLocalRoutesJSON))
	assert.Equal(t, "/etc/ecs/", conf.DataDirOnHost, "Wrong value for DataDirOnHost")
	assert.True(t, conf.ContainerMetadataEnabled.Enabled(), "Wrong value for ContainerMetadataEnabled")
	assert.Equal(t, 1000, conf.TaskMetadataSteadyStateRate)
	assert.Equal(t, 1100, conf.TaskMetadataBurstRate)
	assert.True(t, conf.SharedVolumeMatchFullConfig.Enabled(), "Wrong value for SharedVolumeMatchFullConfig")
	assert.True(t, conf.GPUSupportEnabled, "Wrong value for GPUSupportEnabled")
	assert.Equal(t, "nvidia", conf.NvidiaRuntime)
	assert.True(t, conf.TaskMetadataAZDisabled, "Wrong value for TaskMetadataAZDisabled")
	assert.Equal(t, 10*time.Millisecond, conf.CgroupCPUPeriod)
	assert.False(t, conf.SpotInstanceDrainingEnabled.Enabled())
	assert.Equal(t, []string{"efsAuth"}, conf.VolumePluginCapabilities)
	assert.True(t, conf.DependentContainersPullUpfront.Enabled(), "Wrong value for DependentContainersPullUpfront")
}

func TestTrimWhitespaceWhenCreating(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CLUSTER", "default \r")()
	defer setTestEnv("ECS_ENGINE_AUTH_TYPE", "dockercfg\r")()

	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, "default", cfg.Cluster, "Wrong cluster")
	assert.Equal(t, "dockercfg", cfg.EngineAuthType, "Wrong auth type")
}

func TestTrimWhitespace(t *testing.T) {
	cfg := &Config{
		Cluster:   " asdf ",
		AWSRegion: " us-east-1\r\t",
		DataDir:   "/trailing/space/directory ",
	}

	cfg.trimWhitespace()
	assert.Equal(t, &Config{Cluster: "asdf", AWSRegion: "us-east-1", DataDir: "/trailing/space/directory "}, cfg)
}

func TestConfigBoolean(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_DISABLE_DOCKER_HEALTH_CHECK", "true")()
	defer setTestEnv("ECS_DISABLE_METRICS", "true")()
	defer setTestEnv("ECS_ENABLE_SPOT_INSTANCE_DRAINING", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.DisableMetrics.Enabled())
	assert.True(t, cfg.DisableDockerHealthCheck.Enabled())
	assert.True(t, cfg.SpotInstanceDrainingEnabled.Enabled())
}

func TestBadLoggingDriverSerialization(t *testing.T) {
	defer setTestEnv("ECS_AVAILABLE_LOGGING_DRIVERS", "[\"malformed]")
	defer setTestRegion()()
	conf, err := environmentConfig()
	assert.NoError(t, err)
	assert.Zero(t, len(conf.AvailableLoggingDrivers), "Wrong value for AvailableLoggingDrivers")
}

func TestBadAttributesSerialization(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_INSTANCE_ATTRIBUTES", "This is not valid JSON")()
	_, err := environmentConfig()
	assert.Error(t, err)
}

func TestBadTagsSerialization(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_INSTANCE_TAGS", "This is not valid JSON")()
	_, err := environmentConfig()
	assert.Error(t, err)
}

func TestInvalidLoggingDriver(t *testing.T) {
	conf := DefaultConfig()
	conf.AWSRegion = "us-west-2"
	conf.AvailableLoggingDrivers = []dockerclient.LoggingDriver{"invalid-logging-driver"}
	assert.Error(t, conf.validateAndOverrideBounds(), "Should be error with invalid-logging-driver")
}

func TestAwsFirelensLoggingDriver(t *testing.T) {
	conf := DefaultConfig()
	conf.AWSRegion = "us-west-2"
	conf.AvailableLoggingDrivers = []dockerclient.LoggingDriver{"awsfirelens"}
	assert.NoError(t, conf.validateAndOverrideBounds(), "awsfirelens is a valid logging driver, no error was expected")
}

func TestDefaultPollMetricsWithoutECSDataDir(t *testing.T) {
	conf, err := environmentConfig()
	assert.NoError(t, err)
	assert.False(t, conf.PollMetrics.Enabled())
}

func TestDefaultCheckpointWithoutECSDataDir(t *testing.T) {
	conf, err := environmentConfig()
	assert.NoError(t, err)
	assert.False(t, conf.Checkpoint.Enabled())
	assert.Equal(t, NotSet, conf.Checkpoint.Value)
}

func TestDefaultCheckpointWithECSDataDir(t *testing.T) {
	defer setTestEnv("ECS_DATADIR", "/some/dir")()
	conf, err := environmentConfig()
	assert.NoError(t, err)
	assert.True(t, conf.Checkpoint.Enabled())
}

func TestCheckpointWithoutECSDataDir(t *testing.T) {
	defer setTestEnv("ECS_CHECKPOINT", "true")()
	conf, err := environmentConfig()
	assert.NoError(t, err)
	assert.True(t, conf.Checkpoint.Enabled())
}

func TestInvalidFormatDockerStopTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_STOP_TIMEOUT", "invalid")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, conf.DockerStopTimeout, defaultDockerStopTimeout, "Wrong value for DockerStopTimeout")
}

func TestZeroValueDockerStopTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_STOP_TIMEOUT", "0s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, defaultDockerStopTimeout, conf.DockerStopTimeout, "Wrong value for DockerStopTimeout")
}

func TestInvalidValueDockerStopTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_STOP_TIMEOUT", "-10s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, minimumDockerStopTimeout, conf.DockerStopTimeout, "Wrong value for DockerStopTimeout")
}

func TestInvalidFormatContainerStartTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_START_TIMEOUT", "invalid")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, defaultContainerStartTimeout, conf.ContainerStartTimeout, "Wrong value for ContainerStartTimeout")
}

func TestInvalidFormatContainerCreateTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_CREATE_TIMEOUT", "invalid")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, defaultContainerCreateTimeout, conf.ContainerCreateTimeout, "Wrong value for ContainerCreateTimeout")
}

func TestInvalidFormatDockerInactivityTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_IMAGE_PULL_INACTIVITY_TIMEOUT", "invalid")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, defaultImagePullInactivityTimeout, conf.ImagePullInactivityTimeout, "Wrong value for ImagePullInactivityTimeout")
}

func TestTooSmallDockerInactivityTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_IMAGE_PULL_INACTIVITY_TIMEOUT", "5s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, minimumImagePullInactivityTimeout, conf.ImagePullInactivityTimeout, "Wrong value for ImagePullInactivityTimeout")
}

func TestNegativeValueDockerInactivityTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_IMAGE_PULL_INACTIVITY_TIMEOUT", "-10s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, minimumImagePullInactivityTimeout, conf.ImagePullInactivityTimeout, "Wrong value for ImagePullInactivityTimeout")
}

// Zero is also how the config api handles 'bad' values... so we get a 'default' and not a minimum
func TestZeroValueContainerStartTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_START_TIMEOUT", "0s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, defaultContainerStartTimeout, conf.ContainerStartTimeout, "Wrong value for ContainerStartTimeout")
}

func TestZeroValueContainerCreateTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_CREATE_TIMEOUT", "0s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, defaultContainerCreateTimeout, conf.ContainerCreateTimeout, "Wrong value for ContainerCreateTimeout")
}

func TestInvalidValueContainerStartTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_START_TIMEOUT", "-10s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, minimumContainerStartTimeout, conf.ContainerStartTimeout, "Wrong value for ContainerStartTimeout")
}

func TestInvalidValueContainerCreateTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_CONTAINER_CREATE_TIMEOUT", "-10s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, minimumContainerCreateTimeout, conf.ContainerCreateTimeout, "Wrong value for ContainerCreataeTimeout")
}

func TestZeroValueDockerPullInactivityTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_DOCKER_PULL_INACTIVITY_TIMEOUT", "0s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, defaultImagePullInactivityTimeout, conf.ImagePullInactivityTimeout, "Wrong value for ImagePullInactivityTimeout")
}

func TestInvalidValueDockerPullInactivityTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_DOCKER_PULL_INACTIVITY_TIMEOUT", "-10s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, defaultImagePullInactivityTimeout, conf.ImagePullInactivityTimeout, "Wrong value for ImagePullInactivityTimeout")
}

func TestInvalidValueMaxPollingMetricsWaitDuration(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_POLL_METRICS", "true")()
	defer setTestEnv("ECS_POLLING_METRICS_WAIT_DURATION", "21s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, maximumPollingMetricsWaitDuration, conf.PollingMetricsWaitDuration, "Wrong value for PollingMetricsWaitDuration")
}

func TestInvalidValueMinPollingMetricsWaitDuration(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_POLL_METRICS", "true")()
	defer setTestEnv("ECS_POLLING_METRICS_WAIT_DURATION", "1s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, minimumPollingMetricsWaitDuration, conf.PollingMetricsWaitDuration, "Wrong value for PollingMetricsWaitDuration")
}

func TestInvalidValuePollingMetricsWaitDuration(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_POLL_METRICS", "true")()
	defer setTestEnv("ECS_POLLING_METRICS_WAIT_DURATION", "0s")()
	conf, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, DefaultPollingMetricsWaitDuration, conf.PollingMetricsWaitDuration, "Wrong value for PollingMetricsWaitDuration")
}

func TestInvalidFormatParseEnvVariableUint16(t *testing.T) {
	defer setTestRegion()()
	setTestEnv("FOO", "foo")
	var16 := parseEnvVariableUint16("FOO")
	assert.Zero(t, var16, "Expected 0 from parseEnvVariableUint16 for invalid Uint16 format")
}

func TestValidFormatParseEnvVariableUint16(t *testing.T) {
	defer setTestRegion()()
	setTestEnv("FOO", "1")
	var16 := parseEnvVariableUint16("FOO")
	assert.Equal(t, uint16(1), var16, "Unexpected value parsed in parseEnvVariableUint16.")
}

func TestInvalidFormatParseEnvVariableDuration(t *testing.T) {
	defer setTestRegion()()
	setTestEnv("FOO", "foo")
	duration := parseEnvVariableDuration("FOO")
	assert.Zero(t, duration, "Expected 0 from parseEnvVariableDuration for invalid format")
}

func TestValidForImagesCleanupExclusion(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_EXCLUDE_UNTRACKED_IMAGE", "amazonlinux:2,amazonlinux:3")()
	imagesNotDelete := parseImageCleanupExclusionList("ECS_EXCLUDE_UNTRACKED_IMAGE")
	expectedImages := []string{"amazonlinux:2", "amazonlinux:3"}
	assert.Equal(t, expectedImages, imagesNotDelete, "unexpected imageCleanupExclusionList")
}

func TestValidFormatParseEnvVariableDuration(t *testing.T) {
	defer setTestRegion()()
	setTestEnv("FOO", "1s")
	duration := parseEnvVariableDuration("FOO")
	assert.Equal(t, 1*time.Second, duration, "Unexpected value parsed in parseEnvVariableDuration.")
}

func TestInvalidTaskCleanupTimeoutOverridesToThreeHours(t *testing.T) {
	defer setTestRegion()()
	setTestEnv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "1s")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)

	// If an invalid value is set, the config should pick up the default value for
	// cleaning up the task.
	assert.Equal(t, 3*time.Hour, cfg.TaskCleanupWaitDuration, "Default task cleanup wait duration set incorrectly")
}

func TestTaskCleanupTimeout(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "10m")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Minute, cfg.TaskCleanupWaitDuration, "Task cleanup wait duration set incorrectly")
}

func TestInvalidReservedMemoryOverridesToZero(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_RESERVED_MEMORY", "-1")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	// If an invalid value is set, the config should pick up the default value for
	// reserved memory, which is 0.
	assert.Zero(t, cfg.ReservedMemory, "Wrong value for ReservedMemory")
}

func TestReservedMemory(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_RESERVED_MEMORY", "1")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, uint16(1), cfg.ReservedMemory, "Wrong value for ReservedMemory.")
}

func TestTaskIAMRoleEnabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_TASK_IAM_ROLE", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.TaskIAMRoleEnabled.Enabled(), "Wrong value for TaskIAMRoleEnabled")
}

func TestDeleteNonECSImagesEnabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_UNTRACKED_IMAGE_CLEANUP", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, ExplicitlyEnabled, cfg.DeleteNonECSImagesEnabled.Value, "Wrong value for DeleteNonECSImagesEnabled")
}

func TestTaskIAMRoleForHostNetworkEnabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.TaskIAMRoleEnabledForNetworkHost, "Wrong value for TaskIAMRoleEnabledForNetworkHost")
}

func TestCredentialsAuditLogFile(t *testing.T) {
	defer setTestRegion()()
	dummyLocation := "/foo/bar.log"
	defer setTestEnv("ECS_AUDIT_LOGFILE", dummyLocation)()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, dummyLocation, cfg.CredentialsAuditLogFile, "Wrong value for CredentialsAuditLogFile")
}

func TestCredentialsAuditLogDisabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_AUDIT_LOGFILE_DISABLED", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.CredentialsAuditLogDisabled, "Wrong value for CredentialsAuditLogDisabled")
}

func TestImageCleanupMinimumInterval(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_IMAGE_CLEANUP_INTERVAL", "1m")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, DefaultImageCleanupTimeInterval, cfg.ImageCleanupInterval, "Wrong value for ImageCleanupInterval")
}

func TestImageCleanupMinimumNumImagesToDeletePerCycle(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_NUM_IMAGES_DELETE_PER_CYCLE", "-1")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, DefaultNumImagesToDeletePerCycle, cfg.NumImagesToDeletePerCycle, "Wrong value for NumImagesToDeletePerCycle")
}

func TestInvalidImagePullBehavior(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_IMAGE_PULL_BEHAVIOR", "invalid")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.Equal(t, ImagePullDefaultBehavior, cfg.ImagePullBehavior, "Wrong value for ImagePullBehavior")
}

func TestSharedVolumeMatchFullConfigEnabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_SHARED_VOLUME_MATCH_FULL_CONFIG", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.SharedVolumeMatchFullConfig.Enabled(), "Wrong value for SharedVolumeMatchFullConfig")
}

func TestParseImagePullBehavior(t *testing.T) {
	testcases := []struct {
		name                      string
		envVarVal                 string
		expectedImagePullBehavior ImagePullBehaviorType
	}{
		{
			name:                      "default agent behavior",
			envVarVal:                 "default",
			expectedImagePullBehavior: ImagePullDefaultBehavior,
		},
		{
			name:                      "always agent behavior",
			envVarVal:                 "always",
			expectedImagePullBehavior: ImagePullAlwaysBehavior,
		},
		{
			name:                      "once agent behavior",
			envVarVal:                 "once",
			expectedImagePullBehavior: ImagePullOnceBehavior,
		},
		{
			name:                      "prefer-cached agent behavior",
			envVarVal:                 "prefer-cached",
			expectedImagePullBehavior: ImagePullPreferCachedBehavior,
		},
		{
			name:                      "invalid agent behavior",
			envVarVal:                 "invalid",
			expectedImagePullBehavior: ImagePullDefaultBehavior,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer setTestRegion()()
			defer setTestEnv("ECS_IMAGE_PULL_BEHAVIOR", tc.envVarVal)()
			assert.Equal(t, parseImagePullBehavior(), tc.expectedImagePullBehavior, "Wrong value for ImagePullBehavior")
		})
	}
}

func TestTaskResourceLimitsOverride(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_TASK_CPU_MEM_LIMIT", "false")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.False(t, cfg.TaskCPUMemLimit.Enabled(), "Task cpu and memory limits should be overridden to false")
	assert.Equal(t, ExplicitlyDisabled, cfg.TaskCPUMemLimit.Value, "Task cpu and memory limits should be explicitly set")
}

func TestAWSVPCBlockInstanceMetadata(t *testing.T) {
	defer setTestEnv("ECS_AWSVPC_BLOCK_IMDS", "true")()
	defer setTestRegion()()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.AWSVPCBlockInstanceMetdata.Enabled())
}

func TestInvalidAWSVPCAdditionalLocalRoutes(t *testing.T) {
	os.Setenv("ECS_AWSVPC_ADDITIONAL_LOCAL_ROUTES", `["300.300.300.300/64"]`)
	defer os.Unsetenv("ECS_AWSVPC_ADDITIONAL_LOCAL_ROUTES")
	_, err := environmentConfig()
	assert.Error(t, err)
}

func TestAWSLogsExecutionRole(t *testing.T) {
	setTestEnv("ECS_ENABLE_AWSLOGS_EXECUTIONROLE_OVERRIDE", "true")
	conf, err := environmentConfig()
	assert.NoError(t, err)
	assert.True(t, conf.OverrideAWSLogsExecutionRole.Enabled())
}

func TestTaskMetadataRPSLimits(t *testing.T) {
	testCases := []struct {
		name                    string
		envVarVal               string
		expectedSteadyStateRate int
		expectedBurstRate       int
	}{
		{
			name:                    "negative limit values",
			envVarVal:               "-10,-10",
			expectedSteadyStateRate: DefaultTaskMetadataSteadyStateRate,
			expectedBurstRate:       DefaultTaskMetadataBurstRate,
		},
		{
			name:                    "negative limit,valid burst",
			envVarVal:               "-10,10",
			expectedSteadyStateRate: DefaultTaskMetadataSteadyStateRate,
			expectedBurstRate:       DefaultTaskMetadataBurstRate,
		},
		{
			name:                    "missing limit,valid burst",
			envVarVal:               " ,10",
			expectedSteadyStateRate: DefaultTaskMetadataSteadyStateRate,
			expectedBurstRate:       DefaultTaskMetadataBurstRate,
		},
		{
			name:                    "valid limit,missing burst",
			envVarVal:               "10,",
			expectedSteadyStateRate: DefaultTaskMetadataSteadyStateRate,
			expectedBurstRate:       DefaultTaskMetadataBurstRate,
		},
		{
			name:                    "empty variable",
			envVarVal:               "",
			expectedSteadyStateRate: DefaultTaskMetadataSteadyStateRate,
			expectedBurstRate:       DefaultTaskMetadataBurstRate,
		},
		{
			name:                    "missing burst",
			envVarVal:               "10",
			expectedSteadyStateRate: DefaultTaskMetadataSteadyStateRate,
			expectedBurstRate:       DefaultTaskMetadataBurstRate,
		},
		{
			name:                    "more than expected values",
			envVarVal:               "10,10,10",
			expectedSteadyStateRate: DefaultTaskMetadataSteadyStateRate,
			expectedBurstRate:       DefaultTaskMetadataBurstRate,
		},
		{
			name:                    "values with spaces",
			envVarVal:               "  10 ,5  ",
			expectedSteadyStateRate: 10,
			expectedBurstRate:       5,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer setTestEnv("ECS_TASK_METADATA_RPS_LIMIT", tc.envVarVal)()
			defer setTestRegion()()
			cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedSteadyStateRate, cfg.TaskMetadataSteadyStateRate)
			assert.Equal(t, tc.expectedBurstRate, cfg.TaskMetadataBurstRate)
		})
	}
}

func TestUserDataConfig(t *testing.T) {
	testcases := []struct {
		name                      string
		userDataResponse          string
		userDataResponseError     error
		expectedConfigCluster     string
		expectedConfigAPIEndpoint string
		shouldFail                bool
	}{
		{
			name: "successful consume userdata config",
			userDataResponse: `{ "ECSAgentConfiguration":{
					"Cluster":"arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster",
					"APIEndpoint":"https://some-endpoint.com"
				}
			}`,
			userDataResponseError:     nil,
			expectedConfigCluster:     "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster",
			expectedConfigAPIEndpoint: "https://some-endpoint.com",
		},
		{
			name:                      "returns errors retrieving ec2 userdata",
			userDataResponse:          "",
			userDataResponseError:     errors.New("failed to get userdata"),
			expectedConfigCluster:     "",
			expectedConfigAPIEndpoint: "",
		},
		{
			name: "returns error, failed to parse json",
			userDataResponse: `{{{ "ECSAgentConfiguration":{
					"Cluster":"arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster",
					"APIEndpoint":"https://some-endpoint.com"
				}
			}`,
			userDataResponseError:     nil,
			expectedConfigCluster:     "",
			expectedConfigAPIEndpoint: "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
			mockEc2Metadata.EXPECT().GetUserData().Return(tc.userDataResponse, tc.userDataResponseError)
			cfg := userDataConfig(mockEc2Metadata)
			assert.Equal(t, tc.expectedConfigAPIEndpoint, cfg.APIEndpoint)
			assert.Equal(t, tc.expectedConfigCluster, cfg.Cluster)
		})
	}
}

func TestContainerInstancePropagateTagsFrom(t *testing.T) {
	testcases := []struct {
		name                                       string
		envVarVal                                  string
		expectedContainerInstancePropagateTagsFrom ContainerInstancePropagateTagsFromType
	}{
		{
			name:      "none container instance propagate tags",
			envVarVal: "none",
			expectedContainerInstancePropagateTagsFrom: ContainerInstancePropagateTagsFromNoneType,
		},
		{
			name:      "ec2_instance container instance propagate tags",
			envVarVal: "ec2_instance",
			expectedContainerInstancePropagateTagsFrom: ContainerInstancePropagateTagsFromEC2InstanceType,
		},
		{
			name:      "invalid container instance propagate tags",
			envVarVal: "none",
			expectedContainerInstancePropagateTagsFrom: ContainerInstancePropagateTagsFromNoneType,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer setTestRegion()()
			defer setTestEnv("ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM", tc.envVarVal)()

			// Test the parse function only.
			assert.Equal(t, parseContainerInstancePropagateTagsFrom(), tc.expectedContainerInstancePropagateTagsFrom,
				"Wrong value from parseContainerInstancePropagateTagsFrom")

			cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
			assert.NoError(t, err)
			assert.Equal(t, cfg.ContainerInstancePropagateTagsFrom, tc.expectedContainerInstancePropagateTagsFrom,
				"Wrong value for ContainerInstancePropagateTagsFrom")
		})
	}
}

func TestGPUSupportEnabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_GPU_SUPPORT", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.GPUSupportEnabled, "Wrong value for GPUSupportEnabled")
}

func TestInferentiaSupportEnabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_ENABLE_INF_SUPPORT", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.InferentiaSupportEnabled, "Wrong value for InferentiaSupportEnabled")
}

func TestTaskMetadataAZDisabled(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_DISABLE_TASK_METADATA_AZ", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.NoError(t, err)
	assert.True(t, cfg.TaskMetadataAZDisabled, "Wrong value for TaskMetadataAZDisabled")
}

func TestExternalConfig(t *testing.T) {
	defer setTestRegion()()
	defer setTestEnv("ECS_EXTERNAL", "true")()
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	require.NoError(t, err)
	assert.True(t, cfg.External.Enabled())
}

func TestExternalConfigMissingRegion(t *testing.T) {
	defer setTestEnv("ECS_EXTERNAL", "true")()
	_, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	assert.Error(t, err)
}

func setTestRegion() func() {
	return setTestEnv("AWS_DEFAULT_REGION", "us-west-2")
}

func setTestEnv(k, v string) func() {
	os.Setenv(k, v)
	return func() {
		os.Unsetenv(k)
	}
}
