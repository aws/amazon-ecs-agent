//go:build linux && unit
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

	app_mocks "github.com/aws/amazon-ecs-agent/agent/app/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	mock_serviceconnect "github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect/mock"
	"github.com/aws/amazon-ecs-agent/agent/gpu"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	mock_loader "github.com/aws/amazon-ecs-agent/agent/utils/loader/mocks"
	mock_mobypkgwrapper "github.com/aws/amazon-ecs-agent/agent/utils/mobypkgwrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
	aws_credentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func init() {
	mockPathExists(false)
}

func TestVolumeDriverCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		AvailableLoggingDrivers: []dockerclient.LoggingDriver{
			dockerclient.JSONFileDriver,
			dockerclient.SyslogDriver,
			dockerclient.JournaldDriver,
			dockerclient.GelfDriver,
			dockerclient.FluentdDriver,
		},
		PrivilegedDisabled:         config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled},
		SELinuxCapable:             config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		AppArmorCapable:            config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		TaskENIEnabled:             config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		AWSVPCBlockInstanceMetdata: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		TaskCleanupWaitDuration:    config.DefaultConfig().TaskCleanupWaitDuration,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "privileged-container",
		capabilityPrefix + "docker-remote-api.1.17",
		capabilityPrefix + "docker-remote-api.1.18",
		capabilityPrefix + "logging-driver.json-file",
		capabilityPrefix + "logging-driver.syslog",
		capabilityPrefix + "logging-driver.journald",
		capabilityPrefix + "selinux",
		capabilityPrefix + "apparmor",
		attributePrefix + "docker-plugin.local",
		attributePrefix + "docker-plugin.fancyvolumedriver",
		attributePrefix + "docker-plugin.coolvolumedriver",
		attributePrefix + "docker-plugin.volumedriver",
		attributePrefix + "docker-plugin.volumedriver.latest",
		attributePrefix + taskENIBlockInstanceMetadataAttributeSuffix,
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
		}...)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		cniClient:             cniClient,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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

func TestNvidiaDriverCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		GPUSupportEnabled:  true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "docker-remote-api.1.17",
		attributePrefix + "docker-plugin.local",
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
		// nvidia driver version capability
		attributePrefix + "nvidia-driver-version.396.44",
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
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
		resourceFields: &taskresource.ResourceFields{
			NvidiaGPUManager: &gpu.NvidiaGPUManager{
				DriverVersion: "396.44",
			},
		},
		serviceconnectManager: mockServiceConnectManager,
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

func TestEmptyNvidiaDriverCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		GPUSupportEnabled:  true,
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "docker-remote-api.1.17",
		attributePrefix + "docker-plugin.local",
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
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
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
		resourceFields: &taskresource.ResourceFields{
			NvidiaGPUManager: &gpu.NvidiaGPUManager{
				DriverVersion: "",
			},
		},
		serviceconnectManager: mockServiceConnectManager,
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

func TestENITrunkingCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		TaskENIEnabled:     config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		ENITrunkingEnabled: config.BooleanDefaultTrue{Value: config.ExplicitlyEnabled},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "docker-remote-api.1.17",
		attributePrefix + "docker-plugin.local",
		attributePrefix + taskENIAttributeSuffix,
		attributePrefix + taskENIIPv6AttributeSuffix,
		attributePrefix + taskENITrunkingAttributeSuffix,
		attributePrefix + taskENITrunkingAttributeSuffix,
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
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
				Name:  aws.String(attributePrefix + branchCNIPluginVersionSuffix),
				Value: aws.String("v2"),
			},
		}...)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		cniClient:             cniClient,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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

func TestNoENITrunkingCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		TaskENIEnabled:     config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		ENITrunkingEnabled: config.BooleanDefaultTrue{Value: config.ExplicitlyDisabled},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "docker-remote-api.1.17",
		attributePrefix + "docker-plugin.local",
		attributePrefix + taskENIAttributeSuffix,
		attributePrefix + taskENIIPv6AttributeSuffix,
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
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
		}...)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		cniClient:             cniClient,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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

func TestPIDAndIPCNamespaceSharingCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "docker-remote-api.1.17",
		attributePrefix + "docker-plugin.local",
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
		attributePrefix + capabilityECREndpoint,
		attributePrefix + capabilitySecretEnvASM,
		attributePrefix + capabilitySecretLogDriverASM,
		attributePrefix + capabilityContainerOrdering,
		attributePrefix + capabilityFullTaskSync,
		attributePrefix + capabilityEnvFilesS3,
		attributePrefix + capabiltyPIDAndIPCNamespaceSharing,
		attributePrefix + capabilityContainerPortRange,
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
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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

func TestPIDAndIPCNamespaceSharingCapabilitiesNoPauseContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, errors.New("mock error"))
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "docker-remote-api.1.17",
		attributePrefix + "docker-plugin.local",
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
		attributePrefix + capabilityECREndpoint,
		attributePrefix + capabilitySecretEnvASM,
		attributePrefix + capabilitySecretLogDriverASM,
		attributePrefix + capabilityContainerOrdering,
		attributePrefix + capabilityFullTaskSync,
		attributePrefix + capabilityEnvFilesS3,
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
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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

func TestAppMeshCapabilitiesUnix(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "docker-remote-api.1.17",
		attributePrefix + "docker-plugin.local",
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
		attributePrefix + capabilityECREndpoint,
		attributePrefix + capabilitySecretEnvASM,
		attributePrefix + capabilitySecretLogDriverASM,
		attributePrefix + capabilityContainerOrdering,
		attributePrefix + capabilityFullTaskSync,
		attributePrefix + capabilityEnvFilesS3,
		attributePrefix + capabiltyPIDAndIPCNamespaceSharing,
		attributePrefix + appMeshAttributeSuffix,
		attributePrefix + capabilityContainerPortRange,
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
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
	}

	utils.OpenFile = func(path string) (*os.File, error) {
		return os.Open(filepath.Join(".", "testdata", "test_cpu_info"))
	}
	defer resetOpenFile()

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled:       config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		VolumePluginCapabilities: []string{capabilityEFSAuth},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		capabilityPrefix + "docker-remote-api.1.17",
		attributePrefix + "docker-plugin.local",
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
		attributePrefix + capabilityECREndpoint,
		attributePrefix + capabilitySecretEnvASM,
		attributePrefix + capabilitySecretLogDriverASM,
		attributePrefix + capabilityContainerOrdering,
		attributePrefix + capabiltyPIDAndIPCNamespaceSharing,
		attributePrefix + appMeshAttributeSuffix,
		attributePrefix + taskEIAAttributeSuffix,
		attributePrefix + capabilityFirelensFluentd,
		attributePrefix + capabilityFirelensFluentbit,
		attributePrefix + capabilityEFS,
		attributePrefix + capabilityEFSAuth,
		capabilityPrefix + capabilityFirelensLoggingDriver,
		attributePrefix + capabilityFirelensLoggingDriver + capabilityFireLensLoggingDriverConfigBufferLimitSuffix,
		attributePrefix + capabilityEnvFilesS3,
		attributePrefix + capabilityContainerPortRange,
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
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
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
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	conf := &config.Config{
		PrivilegedDisabled: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
	}

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(true, nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
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
		ctx:                   ctx,
		cfg:                   conf,
		dockerClient:          client,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	assert.Contains(t, capabilities, &ecs.Attribute{Name: aws.String(attributePrefix + capabilityFirelensConfigFile)})
	assert.Contains(t, capabilities, &ecs.Attribute{Name: aws.String(attributePrefix + capabilityFirelensConfigS3)})
}

func TestAppendFSxWindowsFileServerCapabilities(t *testing.T) {
	var inputCapabilities []*ecs.Attribute

	agent := &ecsAgent{}

	capabilities := agent.appendFSxWindowsFileServerCapabilities(inputCapabilities)
	assert.Equal(t, len(inputCapabilities), len(capabilities))
	assert.EqualValues(t, capabilities, inputCapabilities)
}
