//go:build windows && unit

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

package execcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitializeContainer(t *testing.T) {}

func TestGetExecAgentConfigDir(t *testing.T) {
	hash := getExecAgentConfigHash(fmt.Sprintf(execAgentConfigTemplate, 2) + execAgentLogConfigTemplate)

	var tests = []struct {
		expectedDir                string
		expectedError              error
		execAgentConfigDirExist    bool
		configDirIsFile            bool
		removeFileErr              error
		existingLogConfigReadErr   error
		existingLogConfig          string
		logConfigIsDir             bool
		existingAgentConfigReadErr error
		existingAgentConfig        string
		agentConfigIsDir           bool
		createNewConfigDirError    error
		createNewConfigFileError   error
	}{
		{
			expectedDir:             hash,
			expectedError:           nil,
			execAgentConfigDirExist: true,
			existingLogConfig:       execAgentLogConfigTemplate,
			existingAgentConfig:     fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			expectedDir:             hash,
			expectedError:           nil,
			execAgentConfigDirExist: false,
			existingLogConfig:       execAgentLogConfigTemplate,
			existingAgentConfig:     fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			expectedDir:             hash,
			expectedError:           nil,
			execAgentConfigDirExist: true,
			existingLogConfig:       "junk",
			existingAgentConfig:     fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			expectedDir:              hash,
			expectedError:            nil,
			execAgentConfigDirExist:  true,
			existingLogConfigReadErr: errors.New("read file error"),
			existingLogConfig:        "",
			existingAgentConfig:      fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			expectedDir:                hash,
			expectedError:              nil,
			execAgentConfigDirExist:    true,
			existingLogConfig:          execAgentLogConfigTemplate,
			existingAgentConfigReadErr: errors.New("read file error"),
			existingAgentConfig:        "",
		},
		{
			expectedDir:             hash,
			expectedError:           nil,
			execAgentConfigDirExist: true,
			configDirIsFile:         true,
			existingLogConfig:       execAgentLogConfigTemplate,
			existingAgentConfig:     fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			expectedDir:              "",
			expectedError:            errors.New("create file error"),
			execAgentConfigDirExist:  false,
			createNewConfigFileError: errors.New("create file error"),
		},
		{
			expectedDir:             "",
			expectedError:           errors.New("create dir error"),
			execAgentConfigDirExist: false,
			createNewConfigDirError: errors.New("create dir error"),
		},
		// todo: need to fix
		{
			expectedDir:             "",
			expectedError:           errors.New("remove file error"),
			execAgentConfigDirExist: true,
			configDirIsFile:         true,
			removeFileErr:           errors.New("remove file error"),
		},

		// todo: add the is_dir flags
	}
	defer func() {
		osStat = os.Stat
		getFileContent = readFileContent
		createNewExecAgentConfigFile = createNewConfigFile
		removeAll = os.RemoveAll
		mkdirAll = os.MkdirAll
	}()

	for _, tc := range tests {
		osStat = func(name string) (os.FileInfo, error) {
			if filepath.Base(name) == "amazon-ssm-agent.json" {
				return &mockFileInfo{name: "", isDir: tc.agentConfigIsDir}, nil
			} else if filepath.Base(name) == "seelog.xml" {
				return &mockFileInfo{name: "", isDir: tc.logConfigIsDir}, nil
			} else { // this is the case for the top level \config\hash folder
				if tc.execAgentConfigDirExist {
					return &mockFileInfo{name: "", isDir: !tc.configDirIsFile}, nil
				}
				return &mockFileInfo{}, errors.New("no such file")
			}

		}
		removeAll = func(name string) error {
			return tc.removeFileErr
		}
		getFileContent = func(path string) ([]byte, error) {
			if filepath.Base(path) == "amazon-ssm-agent.json" {
				return []byte(tc.existingAgentConfig), tc.existingAgentConfigReadErr
			}

			if filepath.Base(path) == "seelog.xml" {
				return []byte(tc.existingLogConfig), tc.existingLogConfigReadErr
			}
			return nil, nil
		}
		createNewExecAgentConfigFile = func(c, f string) error {
			return tc.createNewConfigFileError
		}

		mkdirAll = func(path string, perm os.FileMode) error {
			return tc.createNewConfigDirError
		}
		configDir, err := GetExecAgentConfigDir(2)
		assert.Equal(t, tc.expectedDir, configDir)
		assert.Equal(t, tc.expectedError, err)
	}

}

func TestGetValidConfigDirExists(t *testing.T) {
	var tests = []struct {
		isValid                    bool
		existingLogConfigReadErr   error
		existingLogConfig          string
		existingLogConfigIsDir     bool
		existingAgentConfigReadErr error
		existingAgentConfig        string
		existingAgentConfigIsDir   bool
	}{
		{
			isValid:                    true,
			existingLogConfigReadErr:   nil,
			existingLogConfig:          execAgentLogConfigTemplate,
			existingAgentConfigReadErr: nil,
			existingAgentConfig:        fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			isValid:                    false,
			existingLogConfigReadErr:   nil,
			existingLogConfig:          execAgentLogConfigTemplate,
			existingAgentConfigReadErr: nil,
			existingAgentConfig:        fmt.Sprintf(execAgentConfigTemplate, 3),
		},
		{
			isValid:                    false,
			existingLogConfigReadErr:   nil,
			existingLogConfig:          "junk",
			existingAgentConfigReadErr: nil,
			existingAgentConfig:        fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			isValid:                    false,
			existingLogConfigReadErr:   errors.New("read file error"),
			existingLogConfig:          "",
			existingAgentConfigReadErr: nil,
			existingAgentConfig:        fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			isValid:                    false,
			existingLogConfigReadErr:   nil,
			existingLogConfig:          execAgentLogConfigTemplate,
			existingAgentConfigReadErr: errors.New("read file error"),
			existingAgentConfig:        "",
		},
		{
			isValid:                    false,
			existingLogConfigReadErr:   nil,
			existingLogConfig:          execAgentLogConfigTemplate,
			existingLogConfigIsDir:     true,
			existingAgentConfigReadErr: nil,
			existingAgentConfig:        fmt.Sprintf(execAgentConfigTemplate, 2),
		},
		{
			isValid:                    false,
			existingLogConfigReadErr:   nil,
			existingLogConfig:          execAgentLogConfigTemplate,
			existingAgentConfigReadErr: nil,
			existingAgentConfig:        fmt.Sprintf(execAgentConfigTemplate, 2),
			existingAgentConfigIsDir:   true,
		},
	}
	defer func() {
		getFileContent = readFileContent
		osStat = os.Stat
	}()
	configDirPath := "C:\\configpath"
	for _, tc := range tests {
		getFileContent = func(path string) ([]byte, error) {
			// amazon-ssm-agent.json
			if path == filepath.Join(configDirPath, containerConfigFileName) {
				return []byte(tc.existingAgentConfig), tc.existingAgentConfigReadErr
			}

			// seelog.xml
			if path == filepath.Join(configDirPath, ExecAgentLogConfigFileName) {
				return []byte(tc.existingLogConfig), tc.existingLogConfigReadErr
			}

			return nil, nil
		}

		osStat = func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == "amazon-ssm-agent.json" {
				return &mockFileInfo{name: "", isDir: tc.existingAgentConfigIsDir}, nil
			}

			if filepath.Base(path) == "seelog.xml" {
				return &mockFileInfo{name: "", isDir: tc.existingLogConfigIsDir}, nil
			}

			return &mockFileInfo{}, errors.New("no such file")
		}
		assert.Equal(t, tc.isValid, validConfigDirExists(configDirPath, getExecAgentConfigHash(fmt.Sprintf(execAgentConfigTemplate, 2)+execAgentLogConfigTemplate)))
	}
}
