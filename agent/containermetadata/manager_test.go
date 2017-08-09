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
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/containermetadata/mocks"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/pborman/uuid"
)

const (
	containerInstanceARN = "a6348116-0ba6-43b5-87c9-8a7e10294b75"
	dockerID             = "888888888887"
	invalidTaskARN       = "invalidARN"
	validTaskARN         = "arn:aws:ecs:region:account-id:task/task-id"
	containerName        = "container"
	dataDirPrefix        = "ecs_mockdata"
)

func setup(t *testing.T) (*mock_containermetadata.MockDockerMetadataClient, string, func()) {
	ctrl := gomock.NewController(t)
	mockDockerMetadataClient := mock_containermetadata.NewMockDockerMetadataClient(ctrl)
	// Generate UUID suffix for mocked dataDir to avoid collision with existing files and directories
	randID := uuid.New()
	mockDataDir := dataDirPrefix + "_" + randID
	return mockDockerMetadataClient, mockDataDir, ctrl.Finish
}

// TestSetContainerInstanceARN checks whether the container instance ARN is set correctly.
func TestSetContainerInstanceARN(t *testing.T) {
	_, _, done := setup(t)
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
	_, _, done := setup(t)
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
	_, mockDataDir, done := setup(t)
	defer done()

	mockTaskARN := validTaskARN
	mockContainerName := containerName

	// Create metadata file directory path as a file instead of directory to cause MkDirAll to fail in Create call
	directoryPath, _ := getTaskMetadataDir(mockTaskARN, mockDataDir)
	os.MkdirAll(directoryPath, os.ModePerm)
	filePath := filepath.Join(directoryPath, mockContainerName)
	os.Create(filePath)

	newManager := &metadataManager{
		dataDir: mockDataDir,
	}
	err := newManager.Create(nil, nil, mockTaskARN, mockContainerName)
	expectErrorMessage := fmt.Sprintf("creating metadata directory for task %s: mkdir %s: not a directory", mockTaskARN, filePath)

	// Remove test artifacts
	os.RemoveAll(mockDataDir)

	if err.Error() != expectErrorMessage {
		t.Error("Got unexpected error: " + err.Error())
	}
}

// TestCreate is the mainline case for metadata create
func TestCreate(t *testing.T) {
	_, mockDataDir, done := setup(t)
	defer done()

	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockConfig := &docker.Config{Env: make([]string, 0)}
	mockHostConfig := &docker.HostConfig{Binds: make([]string, 0)}

	newManager := &metadataManager{
		dataDir: mockDataDir,
	}
	err := newManager.Create(mockConfig, mockHostConfig, mockTaskARN, mockContainerName)

	// Remove test artifacts
	os.RemoveAll(mockDataDir)

	if err != nil {
		t.Error("Got unexpected error: " + err.Error())
	}

	if len(mockConfig.Env) != 1 {
		t.Error("Unexpected number of environment variables in config: ", len(mockConfig.Env))
	}
	if len(mockHostConfig.Binds) != 1 {
		t.Error("Unexpected number of binds in host config: ", len(mockHostConfig.Binds))
	}
}

// TestUpdateInspectFail checks case when Inspect call fails
func TestUpdateInspectFail(t *testing.T) {
	mockClient, _, done := setup(t)
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
	mockClient, _, done := setup(t)
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

// TestUpdate is mainline case for metadata update
func TestUpdate(t *testing.T) {
	mockClient, mockDataDir, done := setup(t)
	defer done()

	mockDockerID := dockerID
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockConfig := &docker.Config{Env: make([]string, 0)}
	mockHostConfig := &docker.HostConfig{Binds: make([]string, 0)}
	mockState := docker.State{
		Running: true,
	}
	mockContainer := &docker.Container{
		State: mockState,
	}

	newManager := &metadataManager{
		client:  mockClient,
		dataDir: mockDataDir,
	}

	err := newManager.Create(mockConfig, mockHostConfig, mockTaskARN, mockContainerName)
	if err != nil {
		os.RemoveAll(mockDataDir)
		t.Error("Got unexpected error: " + err.Error())
	}

	mockClient.EXPECT().InspectContainer(mockDockerID, inspectContainerTimeout).Return(mockContainer, nil)
	err = newManager.Update(mockDockerID, mockTaskARN, mockContainerName)

	// Remove test artifacts
	os.RemoveAll(mockDataDir)

	if err != nil {
		t.Error("Got unexpected error: " + err.Error())
	}
}

// TestCleanMalformedFilepath checks case where ARN is invalid
func TestCleanMalformedFilepath(t *testing.T) {
	_, _, done := setup(t)
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
	_, mockDataDir, done := setup(t)
	defer done()

	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockConfig := &docker.Config{Env: make([]string, 0)}
	mockHostConfig := &docker.HostConfig{Binds: make([]string, 0)}

	newManager := &metadataManager{
		dataDir: mockDataDir,
	}

	err := newManager.Create(mockConfig, mockHostConfig, mockTaskARN, mockContainerName)
	if err != nil {
		os.RemoveAll(mockDataDir)
		t.Error("Got unexpected error: " + err.Error())
	}

	err = newManager.Clean(mockTaskARN)
	if err != nil {
		os.RemoveAll(mockDataDir)
		t.Error("Got unexpected error: " + err.Error())
	}

	// Verify cleanup by checking if file path still remains
	metadataPath, _ := getTaskMetadataDir(mockTaskARN, mockDataDir)
	_, err = os.Stat(metadataPath)
	if err == nil {
		os.RemoveAll(mockDataDir)
		t.Error("Expected file to not exist")
	}
	if !os.IsNotExist(err) {
		os.RemoveAll(mockDataDir)
		t.Error("Got unexpected error: " + err.Error())
	}
	os.RemoveAll(mockDataDir)
}
