// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"

	"golang.org/x/net/context"

	app_mocks "github.com/aws/amazon-ecs-agent/agent/app/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/aws-sdk-go/aws"
	aws_credentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestCapabilities(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := engine.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
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
	}
	capabilities := agent.capabilities()

	for i, expected := range expectedCapabilities {
		assert.Equal(t, aws.StringValue(expected.Name), aws.StringValue(capabilities[i].Name))
		assert.Equal(t, aws.StringValue(expected.Value), aws.StringValue(capabilities[i].Value))
	}
}

func TestCapabilitiesECR(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	conf := &config.Config{}

	client := engine.NewMockDockerClient(ctrl)
	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
	}
	capabilities := agent.capabilities()

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

	client := engine.NewMockDockerClient(ctrl)
	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
	}
	capabilities := agent.capabilities()
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

	client := engine.NewMockDockerClient(ctrl)
	conf := &config.Config{
		TaskIAMRoleEnabled: true,
	}

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_18,
	})
	client.EXPECT().KnownVersions().Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
	}

	capabilities := agent.capabilities()
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

	client := engine.NewMockDockerClient(ctrl)
	conf := &config.Config{
		TaskIAMRoleEnabledForNetworkHost: true,
	}

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
	}

	capabilities := agent.capabilities()
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

	client := engine.NewMockDockerClient(ctrl)
	conf := &config.Config{
		TaskIAMRoleEnabledForNetworkHost: true,
	}

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_18,
	})
	client.EXPECT().KnownVersions().Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:          ctx,
		cfg:          conf,
		dockerClient: client,
	}

	capabilities := agent.capabilities()
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

	client := engine.NewMockDockerClient(ctrl)
	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	conf := &config.Config{
		AvailableLoggingDrivers: []dockerclient.LoggingDriver{
			dockerclient.JSONFileDriver,
		},
		TaskENIEnabled:             false,
		AWSVPCBlockInstanceMetdata: true,
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
	}
	capabilities := agent.capabilities()

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
