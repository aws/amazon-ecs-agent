//go:build unit && !windows
// +build unit,!windows

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
	"fmt"
	"os"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"

	"github.com/stretchr/testify/assert"
)

// TestWriteTempFileFail checks case where temp file cannot be made
func TestWriteTempFileFail(t *testing.T) {
	_, done := writeSetup(t)
	defer done()

	mockData := []byte("")
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockDataDir := dataDir

	oTempFile := TempFile
	TempFile = func(dir, pattern string) (oswrapper.File, error) {
		return nil, errors.New("temp file fail")
	}
	defer func() {
		TempFile = oTempFile
	}()

	err := writeToMetadataFile(mockData, mockTaskARN, mockContainerName, mockDataDir)
	expectErrorMessage := "temp file fail"

	assert.Error(t, err)
	assert.Equal(t, expectErrorMessage, err.Error())
}

// TestWriteFileWriteFail checks case where write to file fails
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

	oTempFile := TempFile
	TempFile = func(dir, pattern string) (oswrapper.File, error) {
		return mockFile, nil
	}
	defer func() {
		TempFile = oTempFile
	}()

	err := writeToMetadataFile(mockData, mockTaskARN, mockContainerName, mockDataDir)
	expectErrorMessage := "write fail"

	assert.Error(t, err)
	assert.Equal(t, expectErrorMessage, err.Error())
}

// TestWriteChmodFail checks case where chmod fails
func TestWriteChmodFail(t *testing.T) {
	mockFile, done := writeSetup(t)
	defer done()

	mockFile.(*mock_oswrapper.MockFile).ChmodImpl = func(os.FileMode) error {
		return errors.New("chmod fail")
	}

	oTempFile := TempFile
	TempFile = func(dir, pattern string) (oswrapper.File, error) {
		return mockFile, nil
	}
	defer func() {
		TempFile = oTempFile
	}()

	mockData := []byte("")
	mockTaskARN := validTaskARN
	mockContainerName := containerName
	mockDataDir := dataDir

	err := writeToMetadataFile(mockData, mockTaskARN, mockContainerName, mockDataDir)
	expectErrorMessage := "chmod fail"

	assert.Error(t, err)
	assert.Equal(t, expectErrorMessage, err.Error())
}

func TestCreateBindEnv(t *testing.T) {
	mockBinds := []string{}
	mockEnv := []string{}
	mockDataDirOnHost := ""
	mockMetadataDirectoryPath := ""
	expectedBindMode := fmt.Sprintf(`:%s`, bindMode)

	testcases := []struct {
		name            string
		securityOptions []string
		selinuxEnabled  bool
	}{
		{
			name:            "Selinux Enabled Bind Mode",
			securityOptions: []string{"selinux"},
			selinuxEnabled:  true,
		},
		{
			name:            "Selinux Disabled Bind Mode",
			securityOptions: []string{""},
			selinuxEnabled:  false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			binds, _ := createBindsEnv(mockBinds, mockEnv, mockDataDirOnHost, mockMetadataDirectoryPath, tc.securityOptions)
			actualBindMode := binds[0][len(binds[0])-2:]
			if tc.selinuxEnabled {
				assert.Equal(t, expectedBindMode, actualBindMode)
			} else {
				assert.NotEqual(t, expectedBindMode, actualBindMode)
			}
		})
	}
}
