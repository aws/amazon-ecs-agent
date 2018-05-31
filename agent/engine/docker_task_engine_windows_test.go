// +build windows,unit

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
package engine

import (
	"context"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
)

const (
	// dockerVersionCheckDuringInit specifies if Docker client's Version()
	// API needs to be mocked in engine tests
	//
	// isParallelPullCompatible is not invoked during engin intialization
	// on windows. No need for mock Docker client's Version() call
	dockerVersionCheckDuringInit = false
)

func TestPullEmptyVolumeImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, _, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	saver := mock_statemanager.NewMockStateManager(ctrl)
	taskEngine.SetSaver(saver)
	taskEngine._time = nil

	imageName := "image"
	container := &apicontainer.Container{
		Type:  apicontainer.ContainerEmptyHostVolume,
		Image: imageName,
	}
	task := &apitask.Task{
		Containers: []*apicontainer.Container{container},
	}

	assert.False(t, emptyvolume.LocalImage, "Windows empty volume image is not local")
	client.EXPECT().PullImage(imageName, nil)

	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, dockerapi.DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	task := &apitask.Task{}

	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockSaver := mock_statemanager.NewMockStateManager(ctrl)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	taskEngine := &DockerTaskEngine{
		state: mockState,
		saver: mockSaver,
		cfg:   &defaultConfig,
		ctx:   ctx,
	}

	gomock.InOrder(
		mockState.EXPECT().RemoveTask(task),
		mockSaver.EXPECT().Save(),
	)

	taskEngine.deleteTask(task)
}
