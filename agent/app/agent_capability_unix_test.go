// +build linux,unit

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

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	mock_pause "github.com/aws/amazon-ecs-agent/agent/eni/pause/mocks"

	app_mocks "github.com/aws/amazon-ecs-agent/agent/app/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	"github.com/aws/amazon-ecs-agent/agent/gpu"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	mock_mobypkgwrapper "github.com/aws/amazon-ecs-agent/agent/utils/mobypkgwrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
	aws_credentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestVolumeDriverCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
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

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
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
			[]string{"coolvolumedriver", "volumedriver:latest"}, nil),
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
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + "docker-plugin.fancyvolumedriver"),
			},
			{
				Name: aws.String(attributePrefix + "docker-plugin.coolvolumedriver"),
			},
			{
				Name: aws.String(attributePrefix + "docker-plugin.volumedriver"),
			},
			{
				Name: aws.String(attributePrefix + "docker-plugin.volumedriver.latest"),
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
		pauseLoader:        mockPauseLoader,
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

func TestNvidiaDriverCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
		GPUSupportEnabled:  true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			// linux specific capabilities
			{
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + capabilityPrivateRegistryAuthASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverSSM),
			},
			// nvidia driver version capability
			{
				Name: aws.String(attributePrefix + "nvidia-driver-version.396.44"),
			},
		}...)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
		resourceFields: &taskresource.ResourceFields{
			NvidiaGPUManager: &gpu.NvidiaGPUManager{
				DriverVersion: "396.44",
			},
		},
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	for i, expected := range expectedCapabilities {
		assert.Equal(t, aws.StringValue(expected.Name), aws.StringValue(capabilities[i].Name))
		assert.Equal(t, aws.StringValue(expected.Value), aws.StringValue(capabilities[i].Value))
	}
}

func TestEmptyNvidiaDriverCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
		GPUSupportEnabled:  true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			// linux specific capabilities

			{
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + capabilityPrivateRegistryAuthASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverSSM),
			},
		}...)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
		resourceFields: &taskresource.ResourceFields{
			NvidiaGPUManager: &gpu.NvidiaGPUManager{
				DriverVersion: "",
			},
		},
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	for i, expected := range expectedCapabilities {
		assert.Equal(t, aws.StringValue(expected.Name), aws.StringValue(capabilities[i].Name))
		assert.Equal(t, aws.StringValue(expected.Value), aws.StringValue(capabilities[i].Value))
	}
}

func TestENITrunkingCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
		TaskENIEnabled:     true,
		ENITrunkingEnabled: true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		cniClient.EXPECT().Version(ecscni.ECSENIPluginName).Return("v1", nil),
		cniClient.EXPECT().Version(ecscni.ECSBranchENIPluginName).Return("v2", nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
		attributePrefix + taskENIAttributeSuffix,
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			// linux specific capabilities
			{
				Name:  aws.String(attributePrefix + cniPluginVersionSuffix),
				Value: aws.String("v1"),
			},
			{
				Name: aws.String(attributePrefix + taskENITrunkingAttributeSuffix),
			},
			{
				Name:  aws.String(attributePrefix + branchCNIPluginVersionSuffix),
				Value: aws.String("v2"),
			},
			{
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + capabilityPrivateRegistryAuthASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverSSM),
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
		pauseLoader:        mockPauseLoader,
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

func TestNoENITrunkingCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
		TaskENIEnabled:     true,
		ENITrunkingEnabled: false,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		cniClient.EXPECT().Version(ecscni.ECSENIPluginName).Return("v1", nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
		attributePrefix + taskENIAttributeSuffix,
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			// linux specific capabilities
			{
				Name:  aws.String(attributePrefix + cniPluginVersionSuffix),
				Value: aws.String("v1"),
			},
			{
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + capabilityPrivateRegistryAuthASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverSSM),
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
		pauseLoader:        mockPauseLoader,
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

func TestPIDAndIPCNamespaceSharingCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			// linux specific capabilities
			{
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + capabilityPrivateRegistryAuthASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilityECREndpoint),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverASM),
			},
			{
				Name: aws.String(attributePrefix + capabilityContainerOrdering),
			},
			{
				Name: aws.String(attributePrefix + capabilityFullTaskSync),
			},
			{
				Name: aws.String(attributePrefix + capabilityEnvFilesS3),
			},
			{
				Name: aws.String(attributePrefix + capabiltyPIDAndIPCNamespaceSharing),
			},
		}...)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
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

func TestPIDAndIPCNamespaceSharingCapabilitiesNoPauseContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, errors.New("mock error"))
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			// linux specific capabilities
			{
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + capabilityPrivateRegistryAuthASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilityECREndpoint),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverASM),
			},
			{
				Name: aws.String(attributePrefix + capabilityContainerOrdering),
			},
			{
				Name: aws.String(attributePrefix + capabilityFullTaskSync),
			},
			{
				Name: aws.String(attributePrefix + capabilityEnvFilesS3),
			},
		}...)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
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

func TestAppMeshCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			// linux specific capabilities
			{
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + capabilityPrivateRegistryAuthASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilityECREndpoint),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverASM),
			},
			{
				Name: aws.String(attributePrefix + capabilityContainerOrdering),
			},
			{
				Name: aws.String(attributePrefix + capabilityFullTaskSync),
			},
			{
				Name: aws.String(attributePrefix + capabilityEnvFilesS3),
			},
			{
				Name: aws.String(attributePrefix + capabiltyPIDAndIPCNamespaceSharing),
			},
			{
				Name: aws.String(attributePrefix + appMeshAttributeSuffix),
			},
		}...)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
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

func TestTaskEIACapabilitiesNoOptimizedCPU(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	utils.OpenFile = func(path string) (*os.File, error) {
		return os.Open(filepath.Join(".", "testdata", "test_cpu_info_fail"))
	}
	defer resetOpenFile()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)
	assert.Contains(t, capabilities, &ecs.Attribute{Name: aws.String(attributePrefix + taskEIAAttributeSuffix)})
	assert.NotContains(t, capabilities, &ecs.Attribute{Name: aws.String(attributePrefix + taskEIAWithOptimizedCPU)})
}

func TestTaskEIACapabilitiesWithOptimizedCPU(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)

	conf := &config.Config{
		PrivilegedDisabled: true,
	}

	utils.OpenFile = func(path string) (*os.File, error) {
		return os.Open(filepath.Join(".", "testdata", "test_cpu_info"))
	}
	defer resetOpenFile()

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)
	assert.Contains(t, capabilities, &ecs.Attribute{Name: aws.String(attributePrefix + taskEIAWithOptimizedCPU)})
}

func resetOpenFile() {
	utils.OpenFile = os.Open
}

func TestCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled:       true,
		VolumePluginCapabilities: []string{capabilityEFSAuth},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	expectedCapabilityNames := []string{
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
	}

	var expectedCapabilities []*ecs.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			&ecs.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]*ecs.Attribute{
			// linux specific capabilities
			{
				Name: aws.String("ecs.capability.docker-plugin.local"),
			},
			{
				Name: aws.String(attributePrefix + capabilityPrivateRegistryAuthASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverSSM),
			},
			{
				Name: aws.String(attributePrefix + capabilityECREndpoint),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretEnvASM),
			},
			{
				Name: aws.String(attributePrefix + capabilitySecretLogDriverASM),
			},
			{
				Name: aws.String(attributePrefix + capabilityContainerOrdering),
			},
			{
				Name: aws.String(attributePrefix + capabiltyPIDAndIPCNamespaceSharing),
			},
			{
				Name: aws.String(attributePrefix + appMeshAttributeSuffix),
			},
			{
				Name: aws.String(attributePrefix + taskEIAAttributeSuffix),
			},
			{
				Name: aws.String(attributePrefix + capabilityFirelensFluentd),
			},
			{
				Name: aws.String(attributePrefix + capabilityFirelensFluentbit),
			},
			{
				Name: aws.String(attributePrefix + capabilityEFS),
			},
			{
				Name: aws.String(attributePrefix + capabilityEFSAuth),
			},
			{
				Name: aws.String(capabilityPrefix + capabilityFirelensLoggingDriver),
			},
			{
				Name: aws.String(attributePrefix + capabilityEnvFilesS3),
			},
		}...)
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	for _, expected := range expectedCapabilities {
		assert.Contains(t, capabilities, &ecs.Attribute{
			Name:  expected.Name,
			Value: expected.Value,
		})
	}
}

func TestFirelensConfigCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_pause.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
		}),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		client.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                conf,
		dockerClient:       client,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	assert.Contains(t, capabilities, &ecs.Attribute{Name: aws.String(attributePrefix + capabilityFirelensConfigFile)})
	assert.Contains(t, capabilities, &ecs.Attribute{Name: aws.String(attributePrefix + capabilityFirelensConfigS3)})
}

func TestAppendGMSACapabilities(t *testing.T) {
	var inputCapabilities []*ecs.Attribute

	agent := &ecsAgent{}

	capabilities := agent.appendGMSACapabilities(inputCapabilities)
	assert.Equal(t, len(inputCapabilities), len(capabilities))
	assert.EqualValues(t, capabilities, inputCapabilities)
}
