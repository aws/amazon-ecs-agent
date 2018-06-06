// +build unit

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

package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"

	"context"

	app_mocks "github.com/aws/amazon-ecs-agent/agent/app/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/mobypkgwrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
	aws_credentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilities(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	conf := &config.Config{
		AvailableLoggingDrivers: []dockerclient.LoggingDriver{
			dockerclient.JSONFileDriver,
			dockerclient.SyslogDriver,
			dockerclient.JournaldDriver,
			dockerclient.GelfDriver,
			dockerclient.FluentdDriver,
		},
		PrivilegedDisabled:         false,
		SELinuxCapable:             true,
		AppArmorCapable:            true,
		TaskENIEnabled:             true,
		AWSVPCBlockInstanceMetdata: true,
		TaskCleanupWaitDuration:    config.DefaultConfig().TaskCleanupWaitDuration,
	}

	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
			dockerclient.Version_1_18,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
			dockerclient.Version_1_18,
			dockerclient.Version_1_19,
		}),
		cniClient.EXPECT().Version(ecscni.ECSENIPluginName).Return("v1", nil),
		mockMobyPlugins.EXPECT().Scan().Return([]string{"fancyvolumedriver"}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return(
			[]string{"coolvolumedriver"}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.privileged-container",
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
		"com.amazonaws.ecs.capability.docker-remote-api.1.18",
		"com.amazonaws.ecs.capability.logging-driver.json-file",
		"com.amazonaws.ecs.capability.logging-driver.syslog",
		"com.amazonaws.ecs.capability.logging-driver.journald",
		"com.amazonaws.ecs.capability.selinux",
		"com.amazonaws.ecs.capability.apparmor",
		attributePrefix + taskENIAttributeSuffix,
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			{
				Name:  aws.String(attributePrefix + cniPluginVersionSuffix),
				Value: aws.String("v1"),
			},
			{
				Name: aws.String(attributePrefix + taskENIBlockInstanceMetadataAttributeSuffix),
			},
			{
				Name: aws.String(attributePrefix + "docker-volume-driver.fancyvolumedriver"),
			},
			{
				Name: aws.String(attributePrefix + "docker-volume-driver.coolvolumedriver"),
			},
		}...)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		cniClient:          cniClient,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	for i, expected := range expectedCapabilities {
		assert.Equal(t, aws.StringValue(expected.Name), aws.StringValue(capabilities[i].Name))
		assert.Equal(t, aws.StringValue(expected.Value), aws.StringValue(capabilities[i].Value))
	}
}

func TestCapabilitiesECR(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conf := &config.Config{}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)
	mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil)
	client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]string{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	_, ok := capMap["com.amazonaws.ecs.capability.ecr-auth"]
	assert.True(t, ok, "Could not find ECR capability when expected; got capabilities %v", capabilities)

	_, ok = capMap["ecs.capability.execution-role-ecr-pull"]
	assert.True(t, ok, "Could not find ECR execution pull capability when expected; got capabilities %v", capabilities)
}

func TestCapabilitiesTaskIAMRoleForSupportedDockerVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conf := &config.Config{
		TaskIAMRoleEnabled: true,
	}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)
	mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil)
	client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]string{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	ok := capMap["com.amazonaws.ecs.capability.task-iam-role"]
	assert.True(t, ok, "Could not find iam capability when expected; got capabilities %v", capabilities)
}

func TestCapabilitiesTaskIAMRoleForUnSupportedDockerVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	conf := &config.Config{
		TaskIAMRoleEnabled: true,
	}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_18,
	})
	client.EXPECT().KnownVersions().Return(nil)
	mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil)
	client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]string{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}

	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	_, ok := capMap["com.amazonaws.ecs.capability.task-iam-role"]
	assert.False(t, ok, "task-iam-role capability set for unsupported docker version")
}

func TestCapabilitiesTaskIAMRoleNetworkHostForSupportedDockerVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	conf := &config.Config{
		TaskIAMRoleEnabledForNetworkHost: true,
	}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)
	mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil)
	client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]string{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}

	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	_, ok := capMap["com.amazonaws.ecs.capability.task-iam-role-network-host"]
	assert.True(t, ok, "Could not find iam capability when expected; got capabilities %v", capabilities)
}

func TestCapabilitiesTaskIAMRoleNetworkHostForUnSupportedDockerVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	conf := &config.Config{
		TaskIAMRoleEnabledForNetworkHost: true,
	}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_18,
	})
	client.EXPECT().KnownVersions().Return(nil)
	mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil)
	client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]string{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}

	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	_, ok := capMap["com.amazonaws.ecs.capability.task-iam-role-network-host"]
	assert.False(t, ok, "task-iam-role capability set for unsupported docker version")
}

func TestAWSVPCBlockInstanceMetadataWhenTaskENIIsDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	conf := &config.Config{
		AvailableLoggingDrivers: []dockerclient.LoggingDriver{
			dockerclient.JSONFileDriver,
		},
		TaskENIEnabled:             false,
		AWSVPCBlockInstanceMetdata: true,
	}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
			dockerclient.Version_1_18,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
			dockerclient.Version_1_18,
			dockerclient.Version_1_19,
		}),
		mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.privileged-container",
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
		"com.amazonaws.ecs.capability.docker-remote-api.1.18",
		"com.amazonaws.ecs.capability.logging-driver.json-file",
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		cniClient:          cniClient,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	for i, expected := range expectedCapabilities {
		assert.Equal(t, aws.StringValue(expected.Name), aws.StringValue(capabilities[i].Name))
		assert.Equal(t, aws.StringValue(expected.Value), aws.StringValue(capabilities[i].Value))
	}

	for _, capability := range capabilities {
		if aws.StringValue(capability.Name) == "ecs.capability.task-eni-block-instance-metadata" {
			t.Errorf("%s capability found when Task ENI is disabled in the config", taskENIBlockInstanceMetadataAttributeSuffix)
		}
	}
}

func TestCapabilitiesExecutionRoleAWSLogs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	conf := &config.Config{
		OverrideAWSLogsExecutionRole: true,
		TaskENIEnabled:               true,
	}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_17,
	})
	client.EXPECT().KnownVersions().Return(nil)
	cniClient.EXPECT().Version(ecscni.ECSENIPluginName).Return("v1", errors.New("some error happened"))
	mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil)
	client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]string{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
		cniClient:    cniClient,
		mobyPlugins:  mockMobyPlugins,
	}

	capabilities, err := agent.capabilities()
	require.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	_, ok := capMap["ecs.capability.execution-role-awslogs"]
	assert.True(t, ok, "Could not find AWSLogs execution role capability when expected; got capabilities %v", capabilities)
}

func TestCapabilitiesTaskResourceLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conf := &config.Config{TaskCPUMemLimit: config.ExplicitlyEnabled}

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	versionList := []dockerclient.DockerVersion{dockerclient.Version_1_22}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return(versionList),
		client.EXPECT().KnownVersions().Return(versionList),
		mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return([]string{}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}

	expectedCapability := attributePrefix + capabilityTaskCPUMemLimit

	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	_, ok := capMap[expectedCapability]
	assert.True(t, ok, "Should contain: "+expectedCapability)
	assert.True(t, agent.cfg.TaskCPUMemLimit.Enabled(), "TaskCPUMemLimit should remain true")
}

func TestCapabilitesTaskResourceLimitDisabledByMissingDockerVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conf := &config.Config{TaskCPUMemLimit: config.DefaultEnabled}

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	versionList := []dockerclient.DockerVersion{dockerclient.Version_1_19}
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return(versionList),
		client.EXPECT().KnownVersions().Return(versionList),
		mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return([]string{}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}

	unexpectedCapability := attributePrefix + capabilityTaskCPUMemLimit
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	_, ok := capMap[unexpectedCapability]

	assert.False(t, ok, "Docker 1.22 is required for task resource limits. Should be disabled")
	assert.False(t, conf.TaskCPUMemLimit.Enabled(), "TaskCPUMemLimit should be made false when we can't find the right docker.")
}

func TestCapabilitesTaskResourceLimitErrorCase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conf := &config.Config{TaskCPUMemLimit: config.ExplicitlyEnabled}

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	versionList := []dockerclient.DockerVersion{dockerclient.Version_1_19}
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return(versionList),
		client.EXPECT().KnownVersions().Return(versionList),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
	}

	capabilities, err := agent.capabilities()
	assert.Nil(t, capabilities)
	assert.Error(t, err, "An error should be thrown when TaskCPUMemLimit is explicitly enabled")
}

func TestCapabilitiesContainerHealth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_24,
	})
	client.EXPECT().KnownVersions().Return(nil)
	mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil)
	client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]string{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          &config.Config{},
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}

	capabilities, err := agent.capabilities()
	require.NoError(t, err)

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[aws.StringValue(capability.Name)] = true
	}

	_, ok := capMap["ecs.capability.container-health-check"]
	assert.True(t, ok, "Could not find container health check capability when expected; got capabilities %v", capabilities)
}

func TestCapabilitesListPluginsErrorCase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	versionList := []dockerclient.DockerVersion{dockerclient.Version_1_19}
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return(versionList),
		client.EXPECT().KnownVersions().Return(versionList),
		mockMobyPlugins.EXPECT().Scan().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return(nil, errors.New("listPlugins error happened")),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          &config.Config{},
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}

	capabilities, err := agent.capabilities()
	require.NoError(t, err)

	for _, capability := range capabilities {
		assert.False(t, strings.HasPrefix(aws.StringValue(capability.Name), "ecs.capability.docker-volume-driver"))
	}
}

func TestCapabilitesScanPluginsErrorCase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	versionList := []dockerclient.DockerVersion{dockerclient.Version_1_19}
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return(versionList),
		client.EXPECT().KnownVersions().Return(versionList),
		mockMobyPlugins.EXPECT().Scan().Return(nil, errors.New("Scan plugins error happened")),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return([]string{}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          &config.Config{},
		dockerClient: client,
		mobyPlugins:  mockMobyPlugins,
	}

	capabilities, err := agent.capabilities()
	require.NoError(t, err)

	for _, capability := range capabilities {
		assert.False(t, strings.HasPrefix(aws.StringValue(capability.Name), "ecs.capability.docker-volume-driver"))
	}
}
