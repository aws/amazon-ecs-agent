// +build !integration, windows
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
	"testing"

	"github.com/golang/mock/gomock"
)

// TestWriteOpenFileFail checks case where open file fails and does not return a NotExist error
func TestWriteOpenFileFail(t *testing.T) {
	_, mockOS, _, done := writeSetup(t)
	defer done()

	mockData := []byte("")
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockDataDir := dataDir
	mockOpenErr := errors.New("does exist")

	gomock.InOrder(
		mockOS.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, mockOpenErr),
		mockOS.EXPECT().IsNotExist(mockOpenErr).Return(false),
	)

	err := writeToMetadataFile(nil, mockIOUtil, mockData, mockTaskARN, mockContainerName, mockDataDir)
	if err == nil {
		t.Error("Expected error to be returned")
	}
	if err.Error() != "does exist" {
		t.Error("Got unexpected error: " + err.Error())
	}
}

// TestWriteFileWrtieFail checks case where we fail to write to file
func TestWriteFileWriteFail(t *testing.T) {
	_, mockOS, mockFile, done := writeSetup(t)
	defer done()

	mockData := []byte("")
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockDataDir := dataDir

	gomock.InOrder(
		mockOS.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockFile, nil),
		mockFile.EXPECT().Write(mockData).Return(errors.New("write fail")),
		mockFile.EXPECT().Close(),
	)

	err := writeToMetadataFile(nil, mockIOUtil, mockData, mockTaskARN, mockContainerName, mockDataDir)
	if err == nil {
		t.Error("Expected error to be returned")
	}
	if err.Error() != "write fail" {
		t.Error("Got unexpected error: " + err.Error())
	}
}
