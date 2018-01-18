// +build !windows,!integration

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
package engine

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/resources/mock_resources"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
)

func TestPullEmptyVolumeImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, _, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	saver := mock_statemanager.NewMockStateManager(ctrl)
	taskEngine.SetSaver(saver)

	imageName := "image"
	container := &api.Container{
		Type:  api.ContainerEmptyHostVolume,
		Image: imageName,
	}
	task := &api.Task{
		Containers: []*api.Container{container},
	}

	assert.True(t, emptyvolume.LocalImage, "empty volume image is local")
	client.EXPECT().ImportLocalEmptyVolumeImage()

	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	task := &api.Task{
		ENI: &api.ENI{
			MacAddress: mac,
		},
	}

	cfg := &defaultConfig
	cfg.TaskCPUMemLimit = config.ExplicitlyEnabled
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockSaver := mock_statemanager.NewMockStateManager(ctrl)
	mockResource := mock_resources.NewMockResource(ctrl)
	taskEngine := &DockerTaskEngine{
		state:    mockState,
		saver:    mockSaver,
		cfg:      &defaultConfig,
		resource: mockResource,
	}

	gomock.InOrder(
		mockResource.EXPECT().Cleanup(task).Return(errors.New("error")),
		mockState.EXPECT().RemoveTask(task),
		mockState.EXPECT().RemoveENIAttachment(mac),
		mockSaver.EXPECT().Save(),
	)

	var cleanupDone sync.WaitGroup
	handleCleanupDone := make(chan struct{})
	cleanupDone.Add(1)
	go func() {
		<-handleCleanupDone
		cleanupDone.Done()
	}()
	taskEngine.deleteTask(task, handleCleanupDone)
	cleanupDone.Wait()
}
