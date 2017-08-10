// +build !integration
// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"errors"
	"fmt"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/containermetadata/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
)

const (
	containerInstanceARN = "a6348116-0ba6-43b5-87c9-8a7e10294b75"
	dockerID             = "888888888887"
	invalidTaskARN       = "invalidARN"
	validTaskARN         = "arn:aws:ecs:region:account-id:task/task-id"
	containerName        = "container"
	dataDir              = "ecs_mockdata"
)

func setup(t *testing.T) (*mock_containermetadata.MockDockerMetadataClient, *mock_ioutilwrapper.MockIOUtil, *mock_oswrapper.MockOS, *mock_oswrapper.MockFile, func()) {
	ctrl := gomock.NewController(t)
	mockDockerMetadataClient := mock_containermetadata.NewMockDockerMetadataClient(ctrl)
	mockIOUtil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	mockOS := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	return mockDockerMetadataClient, mockIOUtil, mockOS, mockFile, ctrl.Finish
}

// TestSetContainerInstanceARN checks whether the container instance ARN is set correctly.
func TestSetContainerInstanceARN(t *testing.T) {
	_, _, _, _, done := setup(t)
	defer done()

	mockARN := containerInstanceARN

	newManager := &metadataManager{}
	newManager.SetContainerInstanceARN(mockARN)
	if newManager.containerInstanceARN != mockARN {
		t.Error("Got unexpected container instance ARN: " + newManager.containerInstanceARN)
	}
}

// TestCreateMalformedFilepath checks case when taskARN is invalid resulting in an invalid file path
func TestCreateMalformedFilepath(t *testing.T) {
	_, _, _, _, done := setup(t)
	defer done()

	mockTaskARN := invalidTaskARN
	mockContainerName := containerName

	newManager := &metadataManager{}
	err := newManager.Create(nil, nil, mockTaskARN, mockContainerName)
	expectErrorMessage := fmt.Sprintf("container metadata create for task %s container %s: get metdata file path of task %s container %s: get task ARN: invalid TaskARN %s", mockTaskARN, mockContainerName, mockTaskARN, mockContainerName, mockTaskARN)

	if err.Error() != expectErrorMessage {
		t.Error("Got unexpected error: " + err.Error())
	}
}

// TestCreateMkdirAllFail checks case when MkdirAll call fails
func TestCreateMkdirAllFail(t *testing.T) {
	_, _, mockOS, _, done := setup(t)
	defer done()

	mockTaskARN := validTaskARN
	mockContainerName := containerName

	gomock.InOrder(
		mockOS.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(errors.New("err")),
	)

	newManager := &metadataManager{
		osWrap: mockOS,
	}
	err := newManager.Create(nil, nil, mockTaskARN, mockContainerName)
	expectErrorMessage := fmt.Sprintf("creating metadata directory for task %s: err", mockTaskARN)

	if err.Error() != expectErrorMessage {
		t.Error("Got unexpected error: " + err.Error())
	}
}

// TestUpdateInspectFail checks case when Inspect call fails
func TestUpdateInspectFail(t *testing.T) {
	mockClient, _, _, _, done := setup(t)
	defer done()

	mockDockerID := dockerID
	mockTaskARN := validTaskARN
	mockContainerName := containerName

	newManager := &metadataManager{
		client: mockClient,
	}

	mockClient.EXPECT().InspectContainer(mockDockerID, inspectContainerTimeout).Return(nil, errors.New("Inspect fail"))
	err := newManager.Update(mockDockerID, mockTaskARN, mockContainerName)

	if err == nil {
		t.Error("Expected inspect error to result in update fail")
	} else if err.Error() != "Inspect fail" {
		t.Error("Got unexpected error: " + err.Error())
	}
}

// TestUpdateNotRunningFail checks case where container is not running
func TestUpdateNotRunningFail(t *testing.T) {
	mockClient, _, _, _, done := setup(t)
	defer done()

	mockDockerID := dockerID
	mockTaskARN := validTaskARN
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

	mockClient.EXPECT().InspectContainer(mockDockerID, inspectContainerTimeout).Return(mockContainer, nil)
	err := newManager.Update(mockDockerID, mockTaskARN, mockContainerName)
	expectErrorMessage := fmt.Sprintf("container metadata update for task %s container %s: container not running or invalid", mockTaskARN, mockContainerName)

	if err.Error() != expectErrorMessage {
		t.Error("Got unexpected error: " + err.Error())
	}
}

// TestCleanMalformedFilepath checks case where ARN is invalid
func TestCleanMalformedFilepath(t *testing.T) {
	_, _, _, _, done := setup(t)
	defer done()

	mockTaskARN := invalidTaskARN

	newManager := &metadataManager{}
	err := newManager.Clean(mockTaskARN)
	expectErrorMessage := fmt.Sprintf("clean task %s: get task ARN: invalid TaskARN invalidARN", mockTaskARN)

	if err.Error() != expectErrorMessage {
		t.Error("Got unexpected error: " + err.Error())
	}
}

// TestClean is the mainline case for metadata create
func TestClean(t *testing.T) {
	_, _, mockOS, _, done := setup(t)
	defer done()

	mockTaskARN := validTaskARN

	newManager := &metadataManager{
		osWrap: mockOS,
	}

	gomock.InOrder(
		mockOS.EXPECT().RemoveAll(gomock.Any()).Return(nil),
	)
	err := newManager.Clean(mockTaskARN)
	if err != nil {
		t.Error("Got unexpected error: " + err.Error())
	}
}
