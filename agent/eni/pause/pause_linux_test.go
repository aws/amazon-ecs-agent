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

package pause

import (
	"context"
	"errors"
	"os"
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
	pauseTarballPath = "/path/to/pause.tar"
	pauseName        = "pause"
	pauseTag         = "tag"
)

var defaultConfig = config.DefaultConfig()

func mockOpen() func() {
	open = func(name string) (*os.File, error) {
		return nil, nil
	}
	return func() {
		open = os.Open
	}
}

// TestLoadFromFileWithReaderError tests loadFromFile with reader error
func TestLoadFromFileWithReaderError(t *testing.T) {
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

	open = func(name string) (*os.File, error) {
		return nil, errors.New("Dummy Reader Error")
	}
	defer func() {
		open = os.Open
	}()

	err = loadFromFile(ctx, pauseTarballPath, client)
	assert.Error(t, err)
}

// TestLoadFromFileHappyPath tests loadFromFile against happy path
func TestLoadFromFileHappyPath(t *testing.T) {
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
	mockDockerSDK.EXPECT().ImageLoad(gomock.Any(), gomock.Any(), false).Return(types.ImageLoadResponse{}, nil)
	defer mockOpen()()

	err = loadFromFile(ctx, pauseTarballPath, client)
	assert.NoError(t, err)
}

// TestLoadFromFileDockerLoadImageError tests loadFromFile against error
// from Docker clients LoadImage
func TestLoadFromFileDockerLoadImageError(t *testing.T) {
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
	mockDockerSDK.EXPECT().ImageLoad(gomock.Any(), gomock.Any(), false).Return(types.ImageLoadResponse{},
		errors.New("Dummy Load Image Error"))

	defer mockOpen()()

	err = loadFromFile(ctx, pauseTarballPath, client)
	assert.Error(t, err)
}

func TestGetPauseContainerImageInspectImageError(t *testing.T) {
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
	mockDockerSDK.EXPECT().ImageInspectWithRaw(gomock.Any(), pauseName+":"+pauseTag).Return(
		types.ImageInspect{}, nil, errors.New("error"))

	_, err = getPauseContainerImage(pauseName, pauseTag, client)
	assert.Error(t, err)
}

func TestGetPauseContainerHappyPath(t *testing.T) {
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
	mockDockerSDK.EXPECT().ImageInspectWithRaw(gomock.Any(), pauseName+":"+pauseTag).Return(types.ImageInspect{}, nil, nil)

	_, err = getPauseContainerImage(pauseName, pauseTag, client)
	assert.NoError(t, err)
}

func TestIsLoadedHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	pauseLoader := New()
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

	isLoaded, err := pauseLoader.IsLoaded(client)
	assert.NoError(t, err)
	assert.True(t, isLoaded)
}

func TestIsLoadedNotLoaded(t *testing.T) {
	ctrl := gomock.NewController(t)
	pauseLoader := New()
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

	isLoaded, err := pauseLoader.IsLoaded(client)
	assert.NoError(t, err)
	assert.False(t, isLoaded)
}

func TestIsLoadedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	pauseLoader := New()
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

	isLoaded, err := pauseLoader.IsLoaded(client)
	assert.Error(t, err)
	assert.False(t, isLoaded)
}
