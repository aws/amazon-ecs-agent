// +build linux,unit

// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package monitor

import (
	"context"
	"errors"
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler/os/mock"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient/mocks"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory/mocks"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	prometheusTarballPath   = "/images/amazon-ecs-prometheus-monitor.tar"
	prometheusName          = "amazon/amazon-ecs-prometheus-monitor"
	prometheusTag           = "0.1.0"
	prometheusVolume        = "amazon-ecs-prometheus-persistance-data"
	prometheusContainerName = "Prometheus_Monitor"
)

func getDefaultConfig() config.Config {
	defaultConfig := config.DefaultConfig()
	defaultConfig.PrometheusMetricsEnabled = true
	return defaultConfig
}

// Test if Volume was created if it does not exist
func TestPrometheusVolumeSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	defaultConfig := getDefaultConfig()

	client, err := dockerapi.NewDockerGoClient(sdkFactory, &defaultConfig, ctx)
	assert.NoError(t, err)
	gomock.InOrder(
		mockDockerSDK.EXPECT().VolumeInspect(gomock.Any(), prometheusVolume).Return(types.Volume{}, errors.New("Volume not found")),
		mockDockerSDK.EXPECT().VolumeRemove(gomock.Any(), prometheusVolume, gomock.Any()).Return(nil),
		mockDockerSDK.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Return(types.Volume{}, nil),
	)
	prometheusVolumeSetup(client, &defaultConfig, ctx)
}

//Test if Volume is created even if it already exists
func TestPrometheusVolumeSetupUnnecessary(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	defaultConfig := getDefaultConfig()

	client, err := dockerapi.NewDockerGoClient(sdkFactory, &defaultConfig, ctx)
	assert.NoError(t, err)

	gomock.InOrder(
		mockDockerSDK.EXPECT().VolumeInspect(gomock.Any(), prometheusVolume).Return(types.Volume{}, nil),
		mockDockerSDK.EXPECT().VolumeRemove(gomock.Any(), gomock.Any(), gomock.Any()).Times(0),
		mockDockerSDK.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Times(0),
	)
	prometheusVolumeSetup(client, &defaultConfig, ctx)
}

func TestCreatePrometheusContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	defaultConfig := getDefaultConfig()
	mockfs := mock_os.NewMockFileSystem(ctrl)

	gomock.InOrder(
		mockfs.EXPECT().Open(prometheusTarballPath).Return(nil, nil),
		client.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()),
		client.EXPECT().InspectImage(prometheusName+":"+prometheusTag),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), prometheusContainerName, gomock.Any()),
	)

	_, err := createPrometheusContainer(client, &defaultConfig, ctx, mockfs)
	assert.NoError(t, err)
}

func TestInitPrometheusContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_dockerapi.NewMockDockerClient(ctrl)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	defaultConfig := getDefaultConfig()
	defaultConfig.PrometheusMonitorContainerID = "TestContainer" // Trigger InspectContainer mock call for test

	mockfs := mock_os.NewMockFileSystem(ctrl)

	gomock.InOrder(
		client.EXPECT().InspectVolume(gomock.Any(), prometheusVolume, gomock.Any()).Return(
			dockerapi.SDKVolumeResponse{Error: errors.New("Volume not created yet")}),
		client.EXPECT().RemoveVolume(gomock.Any(), prometheusVolume, gomock.Any()),
		client.EXPECT().CreateVolume(gomock.Any(), prometheusVolume, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()),
		client.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			&types.ContainerJSON{}, errors.New("Container not found")),
		mockfs.EXPECT().Open(prometheusTarballPath).Return(nil, nil),
		client.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()),
		client.EXPECT().InspectImage(prometheusName+":"+prometheusTag),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), prometheusContainerName, gomock.Any()),
		client.EXPECT().StartContainer(gomock.Any(), gomock.Any(), gomock.Any()),
	)
	InitPrometheusContainer(ctx, &defaultConfig, client, mockfs)
}
