// +build unit

// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"errors"
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	containerInstanceARN   = "a6348116-0ba6-43b5-87c9-8a7e10294b75"
	dockerID               = "888888888887"
	invalidTaskARN         = "invalidARN"
	validTaskARN           = "arn:aws:ecs:region:account-id:task/task-id"
	taskDefinitionFamily   = "taskdefinitionfamily"
	taskDefinitionRevision = "8"
	containerName          = "container"
	dataDir                = "ecs_mockdata"
)

func managerSetup(t *testing.T) (*mock_containermetadata.MockDockerMetadataClient, *mock_ioutilwrapper.MockIOUtil, *mock_oswrapper.MockOS, *mock_oswrapper.MockFile, func()) {
	ctrl := gomock.NewController(t)
	mockDockerMetadataClient := mock_containermetadata.NewMockDockerMetadataClient(ctrl)
	mockIOUtil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockOS := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	return mockDockerMetadataClient, mockIOUtil, mockOS, mockFile, ctrl.Finish
}

// TestSetContainerInstanceARN checks whether the container instance ARN is set correctly.
func TestSetContainerInstanceARN(t *testing.T) {
	_, _, _, _, done := managerSetup(t)
	defer done()

	mockARN := containerInstanceARN

	newManager := &metadataManager{}
	newManager.SetContainerInstanceARN(mockARN)
	assert.Equal(t, mockARN, newManager.containerInstanceARN)
}

// TestCreateMalformedFilepath checks case when taskARN is invalid resulting in an invalid file path
func TestCreateMalformedFilepath(t *testing.T) {
	_, _, _, _, done := managerSetup(t)
	defer done()

	mockTaskARN := invalidTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName

	newManager := &metadataManager{}
	err := newManager.Create(nil, nil, mockTask, mockContainerName)
	assert.Error(t, err)
}

// TestCreateMkdirAllFail checks case when MkdirAll call fails
func TestCreateMkdirAllFail(t *testing.T) {
	_, _, mockOS, _, done := managerSetup(t)
	defer done()

	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(errors.New("err")),
	)

	newManager := &metadataManager{
		osWrap: mockOS,
	}
	err := newManager.Create(nil, nil, mockTask, mockContainerName)
	assert.Error(t, err)
}

// TestUpdateInspectFail checks case when Inspect call fails
func TestUpdateInspectFail(t *testing.T) {
	mockClient, _, _, _, done := managerSetup(t)
	defer done()

	mockDockerID := dockerID
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName

	newManager := &metadataManager{
		client: mockClient,
	}

	mockClient.EXPECT().InspectContainer(gomock.Any(), mockDockerID, inspectContainerTimeout).Return(nil, errors.New("Inspect fail"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := newManager.Update(ctx, mockDockerID, mockTask, mockContainerName)

	assert.Error(t, err, "Expected inspect error to result in update fail")
}

// TestUpdateNotRunningFail checks case where container is not running
func TestUpdateNotRunningFail(t *testing.T) {
	mockClient, _, _, _, done := managerSetup(t)
	defer done()

	mockDockerID := dockerID
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockState := docker.State{
		Running: false,
	}
	mockContainer := &docker.Container{
		State: mockState,
	}

	newManager := &metadataManager{
		client: mockClient,
	}

	mockClient.EXPECT().InspectContainer(gomock.Any(), mockDockerID, inspectContainerTimeout).Return(mockContainer, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := newManager.Update(ctx, mockDockerID, mockTask, mockContainerName)
	assert.Error(t, err)
}

// TestMalformedFilepath checks case where ARN is invalid
func TestMalformedFilepath(t *testing.T) {
	_, _, _, _, done := managerSetup(t)
	defer done()

	mockTaskARN := invalidTaskARN

	newManager := &metadataManager{}
	err := newManager.Clean(mockTaskARN)
	assert.Error(t, err)
}

// TestHappyPath is the mainline case for metadata create
func TestHappyPath(t *testing.T) {
	_, _, mockOS, _, done := managerSetup(t)
	defer done()

	mockTaskARN := validTaskARN

	newManager := &metadataManager{
		osWrap: mockOS,
	}

	gomock.InOrder(
		mockOS.EXPECT().RemoveAll(gomock.Any()).Return(nil),
	)
	err := newManager.Clean(mockTaskARN)
	assert.NoError(t, err)
}
