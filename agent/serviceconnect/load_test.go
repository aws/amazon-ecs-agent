//go:build unit
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

package serviceconnect

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_sdkclient "github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient/mocks"
	mock_sdkclientfactory "github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory/mocks"

	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	agentImageName = "appnet-agent:testtag"
	agentName      = "appnet-agent"
	agentTag       = "testtag"
)

var defaultConfig = config.DefaultConfig()

func TestGetAppnetAgentContainerImageInspectImageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := dockerapi.NewDockerGoClient(sdkFactory, &defaultConfig, ctx)
	assert.NoError(t, err)
	mockDockerSDK.EXPECT().ImageInspectWithRaw(gomock.Any(), agentImageName).Return(
		types.ImageInspect{}, nil, errors.New("error"))

	_, err = getAgentContainerImage(agentImageName, client)
	assert.Error(t, err)
}

func TestGetAgentContainerHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := dockerapi.NewDockerGoClient(sdkFactory, &defaultConfig, ctx)
	assert.NoError(t, err)
	mockDockerSDK.EXPECT().ImageInspectWithRaw(gomock.Any(), agentImageName).Return(types.ImageInspect{}, nil, nil)

	_, err = getAgentContainerImage(agentImageName, client)
	assert.NoError(t, err)
}

func TestIsImageLoadedHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := dockerapi.NewDockerGoClient(sdkFactory, &defaultConfig, ctx)
	assert.NoError(t, err)
	mockDockerSDK.EXPECT().ImageInspectWithRaw(gomock.Any(), gomock.Any()).Return(types.ImageInspect{ID: "test123"}, nil, nil)

	isLoaded, err := (&loader{
		AgentContainerImageName: agentName,
		AgentContainerTag:       agentTag,
	}).isImageLoaded(client)
	assert.NoError(t, err)
	assert.True(t, isLoaded)
}

func TestIsImageLoadedNotLoaded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := dockerapi.NewDockerGoClient(sdkFactory, &defaultConfig, ctx)
	assert.NoError(t, err)
	mockDockerSDK.EXPECT().ImageInspectWithRaw(gomock.Any(), gomock.Any()).Return(types.ImageInspect{}, nil, nil)

	isLoaded, err := (&loader{
		AgentContainerImageName: agentName,
		AgentContainerTag:       agentTag,
	}).isImageLoaded(client)
	assert.NoError(t, err)
	assert.False(t, isLoaded)
}

func TestIsImageLoadedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := dockerapi.NewDockerGoClient(sdkFactory, &defaultConfig, ctx)
	assert.NoError(t, err)
	mockDockerSDK.EXPECT().ImageInspectWithRaw(gomock.Any(), gomock.Any()).Return(
		types.ImageInspect{}, nil, errors.New("error"))

	isLoaded, err := (&loader{
		AgentContainerImageName: agentName,
		AgentContainerTag:       agentTag,
	}).isImageLoaded(client)
	assert.Error(t, err)
	assert.False(t, isLoaded)
}
