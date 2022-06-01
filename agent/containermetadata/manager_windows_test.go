//go:build unit && windows
// +build unit,windows

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

package containermetadata

import (
	"context"
	"os"
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

var mockMkdirAll = func(path string, perm os.FileMode) error {
	return nil
}

// TestCreate is the mainline case for metadata create
func TestCreate(t *testing.T) {
	_, mockFile, done := managerSetup(t)
	defer done()

	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockConfig := &dockercontainer.Config{Env: make([]string, 0)}
	mockHostConfig := &dockercontainer.HostConfig{Binds: make([]string, 0)}
	mockDockerSecurityOptions := types.Info{SecurityOptions: make([]string, 0)}.SecurityOptions

	tempOpenFile := openFile
	openFile = func(name string, flag int, perm os.FileMode) (oswrapper.File, error) {
		return mockFile, nil
	}
	mkdirAll = mockMkdirAll

	defer func() {
		mkdirAll = os.MkdirAll
		openFile = tempOpenFile
	}()

	newManager := &metadataManager{}
	err := newManager.Create(mockConfig, mockHostConfig, mockTask, mockContainerName, mockDockerSecurityOptions)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(mockConfig.Env), "Unexpected number of environment variables in config")
	assert.Equal(t, 1, len(mockHostConfig.Binds), "Unexpected number of binds in host config")
}

// TestUpdate is happy path case for metadata update
func TestUpdate(t *testing.T) {
	mockClient, mockFile, done := managerSetup(t)
	defer done()

	mockDockerID := dockerID
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockState := types.ContainerState{
		Running: true,
	}

	mockConfig := &dockercontainer.Config{Image: "image"}

	mockNetworks := map[string]*network.EndpointSettings{}
	mockNetworkSettings := types.NetworkSettings{Networks: mockNetworks}

	mockContainer := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			State: &mockState,
		},
		Config:          mockConfig,
		NetworkSettings: &mockNetworkSettings,
	}

	newManager := &metadataManager{
		client: mockClient,
	}

	tempOpenFile := openFile
	openFile = func(name string, flag int, perm os.FileMode) (oswrapper.File, error) {
		return mockFile, nil
	}
	defer func() {
		openFile = tempOpenFile
	}()

	gomock.InOrder(
		mockClient.EXPECT().InspectContainer(gomock.Any(), mockDockerID, dockerclient.InspectContainerTimeout).Return(&mockContainer, nil),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := newManager.Update(ctx, mockDockerID, mockTask, mockContainerName)

	assert.NoError(t, err)
}
