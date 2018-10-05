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
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	invalidSplitTaskARN = "arn:aws:ecs:region:account-id:task"
)

// TestGetTaskIDFailDueToInvalidID checks the case where task ARN has 6 parts but last section is invalid
func TestGetTaskIDFailDueToInvalidID(t *testing.T) {
	mockTaskARN := invalidSplitTaskARN

	_, err := getTaskIDfromARN(mockTaskARN)
	expectErrorMessage := fmt.Sprintf("get task ARN: cannot find TaskID for TaskARN %s", mockTaskARN)

	assert.Equal(t, expectErrorMessage, err.Error())
}

func TestGetMetadataFilePathFailDueToInvalidID(t *testing.T) {
	mockTaskARN := invalidSplitTaskARN
	mockContainerName := containerName
	mockDataDir := dataDir

	_, err := getMetadataFilePath(mockTaskARN, mockContainerName, mockDataDir)
	expectErrorMessage := fmt.Sprintf(
		"get metdata file path of task %s container %s: get task ARN: cannot find TaskID for TaskARN %s",
		mockTaskARN, mockContainerName, mockTaskARN)

	assert.Equal(t, expectErrorMessage, err.Error())
}

func TestGetMetadataFilePathSuccess(t *testing.T) {
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockDataDir := dataDir

	path, err := getMetadataFilePath(mockTaskARN, mockContainerName, mockDataDir)
	expectedPath := filepath.Join(mockDataDir, metadataJoinSuffix, "task-id", mockContainerName)

	assert.Equal(t, expectedPath, path)
	assert.NoError(t, err)
}

func TestGetTaskMetadataDirFailDueToInvalidID(t *testing.T) {
	mockTaskARN := invalidSplitTaskARN
	mockDataDir := dataDir

	_, err := getTaskMetadataDir(mockTaskARN, mockDataDir)
	expectErrorMessage := fmt.Sprintf(
		"get task metadata directory: get task ARN: cannot find TaskID for TaskARN %s", mockTaskARN)

	assert.Equal(t, expectErrorMessage, err.Error())
}

func TestGetTaskMetadataDirSuccess(t *testing.T) {
	mockTaskARN := validTaskARN
	mockDataDir := dataDir

	path, err := getTaskMetadataDir(mockTaskARN, mockDataDir)
	expectedPath := filepath.Join(mockDataDir, metadataJoinSuffix, "task-id")

	assert.Equal(t, expectedPath, path)
	assert.NoError(t, err)
}
