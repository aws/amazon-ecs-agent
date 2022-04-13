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

package containermetadata

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_containermetadata "github.com/aws/amazon-ecs-agent/agent/containermetadata/mocks"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/docker/docker/api/types"
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
	availabilityZone       = "us-west-2b"
	hostPrivateIPv4Address = "127.0.0.1"
	hostPublicIPv4Address  = "127.0.0.1"
)

func managerSetup(t *testing.T) (*mock_containermetadata.MockDockerMetadataClient, oswrapper.File, func()) {
	ctrl := gomock.NewController(t)
	mockDockerMetadataClient := mock_containermetadata.NewMockDockerMetadataClient(ctrl)
	mockFile := mock_oswrapper.NewMockFile()
	return mockDockerMetadataClient, mockFile, ctrl.Finish
}

// TestSetContainerInstanceARN checks whether the container instance ARN is set correctly.
func TestSetContainerInstanceARN(t *testing.T) {
	_, _, done := managerSetup(t)
	defer done()

	mockARN := containerInstanceARN

	newManager := &metadataManager{}
	newManager.SetContainerInstanceARN(mockARN)
	assert.Equal(t, mockARN, newManager.containerInstanceARN)
}

// TestAvailabilityZone checks whether the container availabilityZone is set correctly.
func TestSetAvailabilityZone(t *testing.T) {
	_, _, done := managerSetup(t)
	defer done()
	mockAvailabilityZone := availabilityZone
	newManager := &metadataManager{}
	newManager.SetAvailabilityZone(mockAvailabilityZone)
	assert.Equal(t, mockAvailabilityZone, newManager.availabilityZone)
}

// TestSetHostPrivateIPv4Address checks whether the container hostPublicIPv4Address is set correctly.
func TestSetHostPrivateIPv4Address(t *testing.T) {
	_, _, done := managerSetup(t)
	defer done()
	newManager := &metadataManager{}
	newManager.SetHostPrivateIPv4Address(hostPrivateIPv4Address)
	assert.Equal(t, hostPrivateIPv4Address, newManager.hostPrivateIPv4Address)
}

// TestSetHostPublicIPv4Address checks whether the container hostPublicIPv4Address is set correctly.
func TestSetHostPublicIPv4Address(t *testing.T) {
	_, _, done := managerSetup(t)
	defer done()
	newManager := &metadataManager{}
	newManager.SetHostPublicIPv4Address(hostPublicIPv4Address)
	assert.Equal(t, hostPublicIPv4Address, newManager.hostPublicIPv4Address)
}

// TestCreateMalformedFilepath checks case when taskARN is invalid resulting in an invalid file path
func TestCreateMalformedFilepath(t *testing.T) {
	_, _, done := managerSetup(t)
	defer done()

	mockTaskARN := invalidTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockDockerSecurityOptions := types.Info{SecurityOptions: make([]string, 0)}.SecurityOptions

	newManager := &metadataManager{}
	err := newManager.Create(nil, nil, mockTask, mockContainerName, mockDockerSecurityOptions)
	assert.Error(t, err)
}

// TestCreateMkdirAllFail checks case when MkdirAll call fails
func TestCreateMkdirAllFail(t *testing.T) {
	_, _, done := managerSetup(t)
	defer done()

	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockDockerSecurityOptions := types.Info{SecurityOptions: make([]string, 0)}.SecurityOptions

	mkdirAll = func(path string, perm os.FileMode) error {
		return errors.New("err")
	}
	defer func() {
		mkdirAll = os.MkdirAll
	}()

	newManager := &metadataManager{}
	err := newManager.Create(nil, nil, mockTask, mockContainerName, mockDockerSecurityOptions)
	assert.Error(t, err)
}

// TestUpdateInspectFail checks case when Inspect call fails
func TestUpdateInspectFail(t *testing.T) {
	mockClient, _, done := managerSetup(t)
	defer done()

	mockDockerID := dockerID
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName

	newManager := &metadataManager{
		client: mockClient,
	}

	mockClient.EXPECT().InspectContainer(gomock.Any(), mockDockerID, dockerclient.InspectContainerTimeout).Return(nil, errors.New("Inspect fail"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := newManager.Update(ctx, mockDockerID, mockTask, mockContainerName)

	assert.Error(t, err, "Expected inspect error to result in update fail")
}

// TestUpdateNotRunningFail checks case where container is not running
func TestUpdateNotRunningFail(t *testing.T) {
	mockClient, _, done := managerSetup(t)
	defer done()

	mockDockerID := dockerID
	mockTaskARN := validTaskARN
	mockTask := &apitask.Task{Arn: mockTaskARN}
	mockContainerName := containerName
	mockState := types.ContainerState{
		Running: false,
	}
	mockContainer := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			State: &mockState,
		},
	}

	newManager := &metadataManager{
		client: mockClient,
	}

	mockClient.EXPECT().InspectContainer(gomock.Any(), mockDockerID, dockerclient.InspectContainerTimeout).Return(&mockContainer, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := newManager.Update(ctx, mockDockerID, mockTask, mockContainerName)
	assert.Error(t, err)
}

// TestMalformedFilepath checks case where ARN is invalid
func TestMalformedFilepath(t *testing.T) {
	_, _, done := managerSetup(t)
	defer done()

	mockTaskARN := invalidTaskARN

	newManager := &metadataManager{}
	err := newManager.Clean(mockTaskARN)
	assert.Error(t, err)
}

// TestHappyPath is the mainline case for metadata create
func TestHappyPath(t *testing.T) {
	_, _, done := managerSetup(t)
	defer done()

	mockTaskARN := validTaskARN

	removeAll = func(path string) error {
		return nil
	}
	defer func() {
		removeAll = os.RemoveAll
	}()

	newManager := &metadataManager{}
	err := newManager.Clean(mockTaskARN)
	assert.NoError(t, err)
}
