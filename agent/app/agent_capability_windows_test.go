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

package app

import (
	"context"
	"testing"

	app_mocks "github.com/aws/amazon-ecs-agent/agent/app/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	dm "github.com/aws/amazon-ecs-agent/agent/engine/daemonmanager"
	"github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect"
	mock_mobypkgwrapper "github.com/aws/amazon-ecs-agent/agent/utils/mobypkgwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func init() {
	mockPathExists(false)
}

func TestVolumeDriverCapabilitiesWindows(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockCredentialsProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
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
		TaskCleanupWaitDuration:    config.DefaultConfig(ipcompatibility.NewIPv4OnlyCompatibility()).TaskCleanupWaitDuration,
	}

	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
			dockerclient.Version_1_18,
			dockerclient.Version_1_19,
		}),
		cniClient.EXPECT().Version(ecscni.ECSVPCENIPluginExecutable).Return("v1", nil),
	)

	expectedCapabilityNames := []string{
		capabilityPrefix + "privileged-container",
		capabilityPrefix + "docker-remote-api.1.17",
		capabilityPrefix + "docker-remote-api.1.18",
		capabilityPrefix + "docker-remote-api.1.19",
		capabilityPrefix + "logging-driver.json-file",
		capabilityPrefix + "logging-driver.syslog",
		capabilityPrefix + "logging-driver.journald",
		capabilityPrefix + "selinux",
		capabilityPrefix + "apparmor",
		attributePrefix + "docker-plugin.local",
		attributePrefix + taskENIAttributeSuffix,
		attributePrefix + taskENIBlockInstanceMetadataAttributeSuffix,
	}

	var expectedCapabilities []types.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			types.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]types.Attribute{
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
		credentialsCache:      aws.NewCredentialsCache(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: serviceconnect.NewManager(),
		daemonManagers:        make(map[string]dm.DaemonManager),
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	for _, expected := range expectedCapabilities {
		assert.Contains(t, capabilities, types.Attribute{
			Name:  expected.Name,
			Value: expected.Value,
		})
	}

}

// Test the list of capabilities supported in windows
func TestSupportedCapabilitiesWindows(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockCredentialsProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
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
		TaskCleanupWaitDuration:    config.DefaultConfig(ipcompatibility.NewIPv4OnlyCompatibility()).TaskCleanupWaitDuration,
	}

	gomock.InOrder(
		client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
			dockerclient.Version_1_17,
			dockerclient.Version_1_18,
			dockerclient.Version_1_19,
		}),
		cniClient.EXPECT().Version(ecscni.ECSVPCENIPluginExecutable).Return("v1", nil),
	)

	expectedCapabilityNames := []string{
		capabilityPrefix + "privileged-container",
		capabilityPrefix + "docker-remote-api.1.17",
		capabilityPrefix + "docker-remote-api.1.18",
		capabilityPrefix + "docker-remote-api.1.19",
		capabilityPrefix + "logging-driver.json-file",
		capabilityPrefix + "logging-driver.syslog",
		capabilityPrefix + "logging-driver.journald",
		capabilityPrefix + "selinux",
		capabilityPrefix + "apparmor",
		attributePrefix + "docker-plugin.local",
		attributePrefix + taskENIAttributeSuffix,
		attributePrefix + capabilityPrivateRegistryAuthASM,
		attributePrefix + capabilitySecretEnvSSM,
		attributePrefix + capabilitySecretLogDriverSSM,
		attributePrefix + capabilityECREndpoint,
		attributePrefix + capabilitySecretEnvASM,
		attributePrefix + capabilitySecretLogDriverASM,
		attributePrefix + capabilityContainerOrdering,
		attributePrefix + capabilityFullTaskSync,
		attributePrefix + capabilityEnvFilesS3,
		attributePrefix + taskENIBlockInstanceMetadataAttributeSuffix,
		attributePrefix + capabilityContainerPortRange,
		attributePrefix + capabilityContainerRestartPolicy,
	}

	var expectedCapabilities []types.Attribute
	for _, name := range expectedCapabilityNames {
		expectedCapabilities = append(expectedCapabilities,
			types.Attribute{Name: aws.String(name)})
	}
	expectedCapabilities = append(expectedCapabilities,
		[]types.Attribute{
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
		credentialsCache:      aws.NewCredentialsCache(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: serviceconnect.NewManager(),
		daemonManagers:        make(map[string]dm.DaemonManager),
	}
	capabilities, err := agent.capabilities()
	assert.NoError(t, err)

	for _, expected := range expectedCapabilities {
		assert.Contains(t, capabilities, types.Attribute{
			Name:  expected.Name,
			Value: expected.Value,
		})
	}
}

func TestAppendGMSACapabilitiesFalse(t *testing.T) {
	var inputCapabilities []types.Attribute
	var expectedCapabilities []types.Attribute

	expectedCapabilities = append(expectedCapabilities,
		[]types.Attribute{}...)

	agent := &ecsAgent{
		cfg: &config.Config{
			GMSACapable: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled},
		},
	}

	capabilities := agent.appendGMSACapabilities(inputCapabilities)

	assert.Equal(t, len(expectedCapabilities), len(capabilities))
}

func TestAppendFSxWindowsFileServerCapabilities(t *testing.T) {
	var inputCapabilities []types.Attribute
	var expectedCapabilities []types.Attribute

	expectedCapabilities = append(expectedCapabilities,
		[]types.Attribute{
			{
				Name: aws.String(attributePrefix + capabilityFSxWindowsFileServer),
			},
		}...)

	agent := &ecsAgent{
		cfg: &config.Config{
			FSxWindowsFileServerCapable: config.BooleanDefaultTrue{Value: config.ExplicitlyEnabled},
		},
	}

	capabilities := agent.appendFSxWindowsFileServerCapabilities(inputCapabilities)

	assert.Equal(t, len(expectedCapabilities), len(capabilities))
	for i, expected := range expectedCapabilities {
		assert.Equal(t, aws.ToString(expected.Name), aws.ToString(capabilities[i].Name))
		assert.Equal(t, aws.ToString(expected.Value), aws.ToString(capabilities[i].Value))
	}
}

func TestAppendFSxWindowsFileServerCapabilitiesFalse(t *testing.T) {
	var inputCapabilities []types.Attribute
	var expectedCapabilities []types.Attribute

	expectedCapabilities = append(expectedCapabilities,
		[]types.Attribute{}...)

	agent := &ecsAgent{
		cfg: &config.Config{
			FSxWindowsFileServerCapable: config.BooleanDefaultTrue{Value: config.ExplicitlyDisabled},
		},
	}

	capabilities := agent.appendFSxWindowsFileServerCapabilities(inputCapabilities)

	assert.Equal(t, len(expectedCapabilities), len(capabilities))
}

func TestAppendExecCapabilities(t *testing.T) {
	var inputCapabilities []types.Attribute
	var expectedCapabilities []types.Attribute
	execCapability := types.Attribute{
		Name: aws.String(attributePrefix + capabilityExec),
	}

	expectedCapabilities = append(expectedCapabilities,
		[]types.Attribute{}...)
	testCases := []struct {
		name                     string
		pathExists               func(string, bool) (bool, error)
		getSubDirectories        func(path string) ([]string, error)
		isWindows2016Instance    bool
		shouldHaveExecCapability bool
	}{
		{
			name:                     "execute-command capability should not be added on Win2016 instances",
			pathExists:               func(path string, shouldBeDirectory bool) (bool, error) { return true, nil },
			getSubDirectories:        func(path string) ([]string, error) { return []string{"3.0.236.0"}, nil },
			isWindows2016Instance:    true,
			shouldHaveExecCapability: false,
		},
		{
			name:                     "execute-command capability should be added if not a Win2016 instances",
			pathExists:               func(path string, shouldBeDirectory bool) (bool, error) { return true, nil },
			getSubDirectories:        func(path string) ([]string, error) { return []string{"3.0.236.0"}, nil },
			isWindows2016Instance:    false,
			shouldHaveExecCapability: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isWindows2016 = func() (bool, error) { return tc.isWindows2016Instance, nil }
			pathExists = tc.pathExists
			getSubDirectories = tc.getSubDirectories

			defer func() {
				isWindows2016 = config.IsWindows2016
				pathExists = defaultPathExists
				getSubDirectories = defaultGetSubDirectories
			}()
			agent := &ecsAgent{
				cfg: &config.Config{},
			}

			capabilities, err := agent.appendExecCapabilities(inputCapabilities)

			assert.NoError(t, err)

			if tc.shouldHaveExecCapability {
				assert.Contains(t, capabilities, execCapability)
			} else {
				assert.NotContains(t, capabilities, execCapability)
			}
		})
	}
}
