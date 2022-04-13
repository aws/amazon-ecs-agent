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
	"errors"
	"os"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"

	"github.com/stretchr/testify/assert"
)

// TestWriteOpenFileFail checks case where open file fails and does not return a NotExist error
func TestWriteOpenFileFail(t *testing.T) {
	_, done := writeSetup(t)
	defer done()

	mockData := []byte("")
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockDataDir := dataDir
	mockOpenErr := errors.New("does exist")

	tempOpenFile := openFile
	openFile = func(name string, flag int, perm os.FileMode) (oswrapper.File, error) {
		return nil, mockOpenErr
	}
	defer func() {
		openFile = tempOpenFile
	}()

	err := writeToMetadataFile(mockData, mockTaskARN, mockContainerName, mockDataDir)

	expectErrorMessage := "does exist"

	assert.Error(t, err)
	assert.Equal(t, expectErrorMessage, err.Error())
}

// TestWriteFileWrtieFail checks case where we fail to write to file
func TestWriteFileWriteFail(t *testing.T) {
	mockFile, done := writeSetup(t)
	defer done()

	mockData := []byte("")
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockDataDir := dataDir

	mockFile.(*mock_oswrapper.MockFile).WriteImpl = func(bytes []byte) (i int, e error) {
		return 0, errors.New("write fail")
	}
	tempOpenFile := openFile
	openFile = func(name string, flag int, perm os.FileMode) (oswrapper.File, error) {
		return mockFile, nil
	}
	defer func() {
		openFile = tempOpenFile
	}()

	err := writeToMetadataFile(mockData, mockTaskARN, mockContainerName, mockDataDir)

	expectErrorMessage := "write fail"

	assert.Error(t, err)
	assert.Equal(t, expectErrorMessage, err.Error())
}
