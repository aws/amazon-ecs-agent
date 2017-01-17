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
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ec2/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestMerge(t *testing.T) {
	conf1 := &Config{Cluster: "Foo"}
	conf2 := Config{Cluster: "ignored", APIEndpoint: "Bar"}
	conf3 := Config{AWSRegion: "us-west-2"}

	conf1.Merge(conf2).Merge(conf3)

	if conf1.Cluster != "Foo" {
		t.Error("The cluster should not have been overridden")
	}
	if conf1.APIEndpoint != "Bar" {
		t.Error("The APIEndpoint should have been merged in")
	}
	if conf1.AWSRegion != "us-west-2" {
		t.Error("Incorrect region")
	}
}

func TestBrokenEC2Metadata(t *testing.T) {
	os.Clearenv()
	ctrl := gomock.NewController(t)
	mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockEc2Metadata.EXPECT().InstanceIdentityDocument().Return(nil, errors.New("err"))

	_, err := NewConfig(mockEc2Metadata)
	if err == nil {
		t.Fatal("Expected error when region isn't set and metadata doesn't work")
	}
}

func TestBrokenEC2MetadataEndpoint(t *testing.T) {
	os.Clearenv()
	ctrl := gomock.NewController(t)
	mockEc2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)

	mockEc2Metadata.EXPECT().InstanceIdentityDocument().Return(nil, errors.New("err"))
	os.Setenv("AWS_DEFAULT_REGION", "us-west-2")

	config, err := NewConfig(mockEc2Metadata)
	if err != nil {
		t.Fatal("Expected no error")
	}
	if config.AWSRegion != "us-west-2" {
		t.Fatal("Wrong region: " + config.AWSRegion)
	}
	if config.APIEndpoint != "" {
		t.Fatal("Endpoint env variable not set; endpoint should be blank")
	}
}

func TestEnvironmentConfig(t *testing.T) {
	os.Setenv("ECS_CLUSTER", "myCluster")
	os.Setenv("ECS_RESERVED_PORTS_UDP", "[42,99]")
	os.Setenv("ECS_RESERVED_MEMORY", "20")
	os.Setenv("ECS_CONTAINER_STOP_TIMEOUT", "60s")
	os.Setenv("ECS_AVAILABLE_LOGGING_DRIVERS", "[\""+string(dockerclient.SyslogDriver)+"\"]")
	os.Setenv("ECS_SELINUX_CAPABLE", "true")
	os.Setenv("ECS_APPARMOR_CAPABLE", "true")
	os.Setenv("ECS_DISABLE_PRIVILEGED", "true")
	os.Setenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "90s")
	os.Setenv("ECS_ENABLE_TASK_IAM_ROLE", "true")
	os.Setenv("ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST", "true")
	os.Setenv("ECS_DISABLE_IMAGE_CLEANUP", "true")
	os.Setenv("ECS_IMAGE_CLEANUP_INTERVAL", "2h")
	os.Setenv("ECS_IMAGE_MINIMUM_CLEANUP_AGE", "30m")
	os.Setenv("ECS_NUM_IMAGES_DELETE_PER_CYCLE", "2")
	os.Setenv("ECS_INSTANCE_ATTRIBUTES", "{\"my_attribute\": \"testing\"}")

	conf, err := environmentConfig()
	assert.Nil(t, err)
	assert.Equal(t, "myCluster", conf.Cluster)
	assert.Equal(t, 2, len(conf.ReservedPortsUDP))
	assert.Contains(t, conf.ReservedPortsUDP, uint16(42))
	assert.Contains(t, conf.ReservedPortsUDP, uint16(99))
	assert.Equal(t, uint16(20), conf.ReservedMemory)

	expectedDuration, _ := time.ParseDuration("60s")
	assert.Equal(t, expectedDuration, conf.DockerStopTimeout)

	assert.Equal(t, []dockerclient.LoggingDriver{dockerclient.SyslogDriver}, conf.AvailableLoggingDrivers)

	assert.True(t, conf.PrivilegedDisabled)
	assert.True(t, conf.SELinuxCapable, "Wrong value for SELinuxCapable")
	assert.True(t, conf.AppArmorCapable, "Wrong value for AppArmorCapable")
	assert.True(t, conf.TaskIAMRoleEnabled, "Wrong value for TaskIAMRoleEnabled")
	assert.True(t, conf.TaskIAMRoleEnabledForNetworkHost, "Wrong value for TaskIAMRoleEnabledForNetworkHost")
	assert.True(t, conf.ImageCleanupDisabled, "Wrong value for ImageCleanupDisabled")

	assert.Equal(t, (30 * time.Minute), conf.MinimumImageDeletionAge)
	assert.Equal(t, (2 * time.Hour), conf.ImageCleanupInterval)
	assert.Equal(t, 2, conf.NumImagesToDeletePerCycle)
	assert.Equal(t, "testing", conf.InstanceAttributes["my_attribute"])
	assert.Equal(t, (90 * time.Second), conf.TaskCleanupWaitDuration)
}

func TestTrimWhitespace(t *testing.T) {
	os.Setenv("ECS_CLUSTER", "default \r")
	os.Setenv("ECS_ENGINE_AUTH_TYPE", "dockercfg\r")

	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster != "default" {
		t.Error("Wrong cluster: " + cfg.Cluster)
	}
	if cfg.EngineAuthType != "dockercfg" {
		t.Error("Wrong auth type: " + cfg.EngineAuthType)
	}

	cfg = &Config{
		Cluster:   " asdf ",
		AWSRegion: " us-east-1\r\t",
		DataDir:   "/trailing/space/directory ",
	}
	cfg.trimWhitespace()
	if !reflect.DeepEqual(cfg, &Config{Cluster: "asdf", AWSRegion: "us-east-1", DataDir: "/trailing/space/directory "}) {
		t.Error("Did not match expected", *cfg)
	}
}

func TestConfigBoolean(t *testing.T) {
	os.Setenv("ECS_DISABLE_METRICS", "true")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DisableMetrics {
		t.Error("DisableMetrics not set to true")
	}
}

func TestBadLoggingDriverSerialization(t *testing.T) {
	os.Setenv("ECS_AVAILABLE_LOGGING_DRIVERS", "[\"malformed]")
	defer os.Unsetenv("ECS_AVAILABLE_LOGGING_DRIVERS")

	conf, err := environmentConfig()
	assert.NoError(t, err)
	if len(conf.AvailableLoggingDrivers) != 0 {
		t.Error("Wrong value for AvailableLoggingDrivers", conf.AvailableLoggingDrivers)
	}
}

func TestBadAttributesSerialization(t *testing.T) {
	os.Setenv("ECS_INSTANCE_ATTRIBUTES", "This is not valid JSON")
	defer os.Unsetenv("ECS_INSTANCE_ATTRIBUTES")
	_, err := environmentConfig()
	assert.Error(t, err)
}

func TestInvalidLoggingDriver(t *testing.T) {
	conf := DefaultConfig()
	conf.AWSRegion = "us-west-2"
	conf.AvailableLoggingDrivers = []dockerclient.LoggingDriver{"invalid-logging-driver"}

	err := conf.validateAndOverrideBounds()
	if err == nil {
		t.Error("Should be error with invalid-logging-driver")
	}
}

func TestInvalidFormatDockerStopTimeout(t *testing.T) {
	os.Setenv("ECS_CONTAINER_STOP_TIMEOUT", "invalid")
	conf, err := environmentConfig()
	if err != nil {
		t.Error("environmentConfig() returned unexpected error %v", err)
	}
	if conf.DockerStopTimeout != 0 {
		t.Error("Wrong value for DockerStopTimeout", conf.DockerStopTimeout)
	}
}

func TestInvalideValueDockerStopTimeout(t *testing.T) {
	os.Setenv("ECS_CONTAINER_STOP_TIMEOUT", "-10s")
	conf, err := environmentConfig()
	assert.NoError(t, err)
	assert.Zero(t, conf.DockerStopTimeout)
}

func TestInvalideDockerStopTimeout(t *testing.T) {
	conf := DefaultConfig()
	conf.DockerStopTimeout = -1 * time.Second

	err := conf.validateAndOverrideBounds()
	if err == nil {
		t.Error("Should be error with negative DockerStopTimeout")
	}
}

func TestInvalidFormatParseEnvVariableUint16(t *testing.T) {
	os.Setenv("FOO", "foo")
	var16 := parseEnvVariableUint16("FOO")
	if var16 != 0 {
		t.Error("Expected 0 from parseEnvVariableUint16 for invalid Uint16 format")
	}
}

func TestValidFormatParseEnvVariableUint16(t *testing.T) {
	os.Setenv("FOO", "1")
	var16 := parseEnvVariableUint16("FOO")
	if var16 != 1 {
		t.Errorf("Unexpected value parsed in parseEnvVariableUint16. Expected %d, got %d", 1, var16)
	}
}

func TestInvalidFormatParseEnvVariableDuration(t *testing.T) {
	os.Setenv("FOO", "foo")
	duration := parseEnvVariableDuration("FOO")
	if duration != 0 {
		t.Error("Expected 0 from parseEnvVariableDuration for invalid format")
	}
}

func TestValidFormatParseEnvVariableDuration(t *testing.T) {
	os.Setenv("FOO", "1s")
	duration := parseEnvVariableDuration("FOO")
	if duration != 1*time.Second {
		t.Errorf("Unexpected value parsed in parseEnvVariableDuration. Expected %v, got %v", 1*time.Second, duration)
	}
}

func TestInvalidTaskCleanupTimeout(t *testing.T) {
	os.Setenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "1s")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	// If an invalid value is set, the config should pick up the default value for
	// cleaning up the task.
	if cfg.TaskCleanupWaitDuration != 3*time.Hour {
		t.Error("Defualt task cleanup wait duration set incorrectly")
	}
}

func TestTaskCleanupTimeout(t *testing.T) {
	os.Setenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "10m")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	// If an invalid value is set, the config should pick up the default value for
	// cleaning up the task.
	if cfg.TaskCleanupWaitDuration != 10*time.Minute {
		t.Errorf("Task cleanup wait duration set incorrectly. Expected %v, got %v", 10*time.Minute, cfg.TaskCleanupWaitDuration)
	}
}

func TestInvalidReservedMemory(t *testing.T) {
	os.Setenv("ECS_RESERVED_MEMORY", "-1")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	// If an invalid value is set, the config should pick up the default value for
	// reserved memory, which is 0.
	if cfg.ReservedMemory != 0 {
		t.Error("Wrong value for ReservedMemory", cfg.ReservedMemory)
	}
}

func TestReservedMemory(t *testing.T) {
	os.Setenv("ECS_RESERVED_MEMORY", "1")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	// If an invalid value is set, the config should pick up the default value for
	// reserved memory, which is 0.
	if cfg.ReservedMemory != 1 {
		t.Errorf("Wrong value for ReservedMemory. Expected %d, got %d", 1, cfg.ReservedMemory)
	}
}

func TestTaskIAMRoleEnabled(t *testing.T) {
	os.Setenv("ECS_ENABLE_TASK_IAM_ROLE", "true")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.TaskIAMRoleEnabled {
		t.Errorf("Wrong value for TaskIAMRoleEnabled: %v", cfg.TaskIAMRoleEnabled)
	}
}

func TestTaskIAMRoleForHostNetworkEnabled(t *testing.T) {
	os.Setenv("ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST", "true")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.TaskIAMRoleEnabledForNetworkHost {
		t.Errorf("Wrong value for TaskIAMRoleEnabledForNetworkHost: %v", cfg.TaskIAMRoleEnabledForNetworkHost)
	}
}

func TestCredentialsAuditLogFile(t *testing.T) {
	dummyLocation := "/foo/bar.log"
	os.Setenv("ECS_AUDIT_LOGFILE", dummyLocation)
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	if cfg.CredentialsAuditLogFile != dummyLocation {
		t.Errorf("Wrong value for CredentialsAuditLogFile: %v", cfg.CredentialsAuditLogFile)
	}
}

func TestCredentialsAuditLogDisabled(t *testing.T) {
	os.Setenv("ECS_AUDIT_LOGFILE_DISABLED", "true")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.CredentialsAuditLogDisabled {
		t.Errorf("Wrong value for CredentialsAuditLogDisabled: %v", cfg.CredentialsAuditLogDisabled)
	}
}

func TestImageCleanupMinimumInterval(t *testing.T) {
	os.Setenv("ECS_IMAGE_CLEANUP_INTERVAL", "1m")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ImageCleanupInterval != DefaultImageCleanupTimeInterval {
		t.Errorf("Wrong value for ImageCleanupInterval: %v", cfg.ImageCleanupInterval)
	}
}

func TestImageCleanupMinimumNumImagesToDeletePerCycle(t *testing.T) {
	os.Setenv("ECS_NUM_IMAGES_DELETE_PER_CYCLE", "-1")
	cfg, err := NewConfig(ec2.NewBlackholeEC2MetadataClient())
	if err != nil {
		t.Fatal(err)
	}

	if cfg.NumImagesToDeletePerCycle != DefaultNumImagesToDeletePerCycle {
		t.Errorf("Wrong value for NumImagesToDeletePerCycle: %v", cfg.NumImagesToDeletePerCycle)
	}
}
