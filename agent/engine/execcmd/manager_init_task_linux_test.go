//go:build linux && unit
// +build linux,unit

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
	"io/ioutil"
	"os"
	"testing"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
)

func TestInitializeContainer(t *testing.T) {
	defer func() {
		GetExecAgentConfigFileName = getAgentConfigFileName
		newUUID = uuid.New
		ioUtilReadDir = ioutil.ReadDir
		osStat = os.Stat
		GetExecAgentLogConfigFile = getAgentLogConfigFile
	}()

	newUUID = func() string {
		return "test-UUID"
	}

	const (
		containerNameOnlyHyphens = "--"
		latestVersion            = "3.0.236.0"
		previousVersion          = "2.0.0.0"
	)
	var tt = []struct {
		name                              string
		managedAgentName                  string
		getExecAgentConfigFileNameError   error
		getExecAgentLogConfigError        error
		retrieveBinVersionsError          error
		simulateNoValidVersion            bool
		simulateEmptyVersionDir           bool
		simulateFallbackToPreviousVersion bool
		expectedVersionUsed               string
		expectedError                     error
	}{
		{
			name:             "no ExecuteCommandAgent in container",
			managedAgentName: "randomAgent",
			expectedError:    errExecCommandManagedAgentNotFound,
		},
		{
			name:                            "simulate config file name generation error",
			managedAgentName:                ExecuteCommandAgentName,
			getExecAgentConfigFileNameError: errors.New("mockError"),
			expectedError:                   fmt.Errorf("could not generate ExecAgent Config File: %v", errors.New("mockError")),
		},
		{
			name:                     "simulate error when reading versions in bin dir",
			managedAgentName:         ExecuteCommandAgentName,
			retrieveBinVersionsError: errors.New("mockError"),
			expectedError:            errors.New("mockError"),
		},
		{
			name:                   "simulate no valid versions exist in bin dir",
			managedAgentName:       ExecuteCommandAgentName,
			simulateNoValidVersion: true,
			expectedError:          fmt.Errorf("no valid versions were found in %s", HostBinDir),
		},
		{
			name:                    "simulate valid version exists but dir is empty",
			managedAgentName:        ExecuteCommandAgentName,
			simulateEmptyVersionDir: true,
			expectedError:           fmt.Errorf("no valid versions were found in %s", HostBinDir),
		},
		{
			name:                              "can fallback to previous version if latest is empty",
			managedAgentName:                  ExecuteCommandAgentName,
			simulateFallbackToPreviousVersion: true,
			expectedVersionUsed:               previousVersion,
		},
		{
			name:                "happy path",
			managedAgentName:    ExecuteCommandAgentName,
			expectedVersionUsed: latestVersion,
		},
		{
			name:                       "simulate log config file generation error",
			managedAgentName:           ExecuteCommandAgentName,
			getExecAgentLogConfigError: errors.New("mockError"),
			expectedError:              fmt.Errorf("could not generate ExecAgent LogConfig file: %v", errors.New("mockError")),
		},
	}
	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			containers := []*apicontainer.Container{
				{
					Name:                "container-name",
					ManagedAgentsUnsafe: []apicontainer.ManagedAgent{{Name: test.managedAgentName}},
				},
				{
					Name:                "--container-name",
					ManagedAgentsUnsafe: []apicontainer.ManagedAgent{{Name: test.managedAgentName}},
				},
				{
					Name:                containerNameOnlyHyphens, // Container that will end up in an empty directory name (trailing hyphens are removed)
					ManagedAgentsUnsafe: []apicontainer.ManagedAgent{{Name: test.managedAgentName}},
				},
			}

			execCmdMgr := newTestManager()

			GetExecAgentConfigFileName = func(s int) (string, error) {
				return "amazon-ssm-agent.json", test.getExecAgentConfigFileNameError
			}

			GetExecAgentLogConfigFile = func() (string, error) {
				return "seelog.xml", test.getExecAgentLogConfigError
			}

			ioUtilReadDir = func(dirname string) ([]os.FileInfo, error) { // Needed to simulate retrieving agent versions
				latestVersionIsDir := true
				previousVersionIsDir := true
				if test.simulateNoValidVersion {
					// A valid version is only considered if it's a dir, so to simulate invalid version we just say it's
					// NOT a dir.
					latestVersionIsDir = false
					previousVersionIsDir = false
				}

				if test.simulateFallbackToPreviousVersion {
					// making latest version invalid and previous one valid
					latestVersionIsDir = false
					previousVersionIsDir = true
				}

				return []os.FileInfo{
					&mockFileInfo{name: latestVersion, isDir: latestVersionIsDir},
					&mockFileInfo{name: previousVersion, isDir: previousVersionIsDir},
				}, test.retrieveBinVersionsError
			}

			osStat = func(name string) (os.FileInfo, error) {
				var err error
				if test.simulateEmptyVersionDir {
					err = errors.New("mock error to simulate file doesn't exist")
				}
				return &mockFileInfo{name: "", isDir: false}, err
			}

			for _, container := range containers {
				hc := &dockercontainer.HostConfig{}
				err := execCmdMgr.InitializeContainer("task-id", container, hc)

				assert.Equal(t, test.expectedError, err)

				if test.expectedError != nil {
					assert.Empty(t, hc.Binds)
					if ma, ok := container.GetManagedAgentByName(ExecuteCommandAgentName); ok {
						assert.Equal(t, "", ma.ID)
						assert.True(t, ma.InitFailed)
						assert.Equal(t, apicontainerstatus.ManagedAgentStopped, ma.Status)
					}
					continue
				}

				assert.Len(t, hc.Binds, 7)

				assert.Subset(t, hc.Binds, []string{
					"/var/lib/ecs/deps/execute-command/bin/" + test.expectedVersionUsed + "/amazon-ssm-agent:" +
						"/ecs-execute-command-test-UUID/amazon-ssm-agent:ro"})

				assert.Subset(t, hc.Binds, []string{
					"/var/lib/ecs/deps/execute-command/bin/" + test.expectedVersionUsed + "/ssm-agent-worker:" +
						"/ecs-execute-command-test-UUID/ssm-agent-worker:ro"})

				assert.Subset(t, hc.Binds, []string{
					"/var/lib/ecs/deps/execute-command/bin/" + test.expectedVersionUsed + "/ssm-session-worker:" +
						"/ecs-execute-command-test-UUID/ssm-session-worker:ro"})

				assert.Subset(t, hc.Binds, []string{
					"/var/lib/ecs/deps/execute-command/config/amazon-ssm-agent.json:" +
						"/ecs-execute-command-test-UUID/configuration/amazon-ssm-agent.json:ro"})

				assert.Subset(t, hc.Binds, []string{
					"/var/lib/ecs/deps/execute-command/certs/tls-ca-bundle.pem:" +
						"/ecs-execute-command-test-UUID/certs/amazon-ssm-agent.crt:ro"})

				assert.Subset(t, hc.Binds, []string{
					"/var/lib/ecs/deps/execute-command/config/seelog.xml:" +
						"/ecs-execute-command-test-UUID/configuration/seelog.xml:ro"})

				expectedHostLogDir := "/var/log/ecs/exec/task-id/container-name:"
				if container.Name == containerNameOnlyHyphens {
					expectedHostLogDir = "/var/log/ecs/exec/task-id/nameless-container-test-UUID:"
				}
				assert.Subset(t, hc.Binds, []string{
					expectedHostLogDir +
						"/var/log/amazon/ssm"})
			}
		})
	}
}

func TestGetExecAgentConfigFileName(t *testing.T) {
	execAgentConfig := `{
	"Mgs": {
		"Region": "",
		"Endpoint": "",
		"StopTimeoutMillis": 20000,
		"SessionWorkersLimit": 2
	},
	"Agent": {
		"Region": "",
		"OrchestrationRootDir": "",
		"ContainerMode": true
	}
}`
	sha := getExecAgentConfigHash(execAgentConfig)
	configFileName := fmt.Sprintf("amazon-ssm-agent-%s.json", sha)
	var tests = []struct {
		fileExists             bool
		fileIsDir              bool
		removeDirErr           error
		createConfigFileErr    error
		expectedConfigFileName string
		expectedError          error
	}{
		{
			fileExists:             true,
			fileIsDir:              false,
			removeDirErr:           nil,
			createConfigFileErr:    nil,
			expectedConfigFileName: configFileName,
			expectedError:          nil,
		},
		{
			fileExists:             false,
			fileIsDir:              false,
			removeDirErr:           nil,
			createConfigFileErr:    nil,
			expectedConfigFileName: configFileName,
			expectedError:          nil,
		},
		{
			fileExists:             false,
			fileIsDir:              false,
			removeDirErr:           nil,
			createConfigFileErr:    errors.New("cannot create config"),
			expectedConfigFileName: "",
			expectedError:          errors.New("cannot create config"),
		},
		{
			fileExists:             true,
			fileIsDir:              true,
			removeDirErr:           nil,
			createConfigFileErr:    nil,
			expectedConfigFileName: configFileName,
			expectedError:          nil,
		},
		{
			fileExists:             true,
			fileIsDir:              true,
			removeDirErr:           errors.New("remove dir error"),
			createConfigFileErr:    nil,
			expectedConfigFileName: "",
			expectedError:          errors.New("remove dir error"),
		},
	}
	defer func() {
		osStat = os.Stat
		createNewExecAgentConfigFile = createNewConfigFile
		removeAll = os.RemoveAll
	}()
	for _, tc := range tests {
		osStat = func(name string) (os.FileInfo, error) {
			return &mockFileInfo{
				name:  "whatever",
				isDir: tc.fileIsDir,
			}, nil
		}

		removeAll = func(name string) error {
			return tc.removeDirErr
		}

		createNewExecAgentConfigFile = func(c, f string) error {
			return tc.createConfigFileErr
		}
		fileName, err := GetExecAgentConfigFileName(2)
		assert.Equal(t, tc.expectedConfigFileName, fileName, "incorrect config file name")
		assert.Equal(t, tc.expectedError, err)
	}
}

func TestGetExecAgentLogConfigFile(t *testing.T) {
	hash := getExecAgentConfigHash(execAgentLogConfigTemplate)
	var tests = []struct {
		expectedFile             string
		expectedError            error
		execAgentConfigFileExist bool
		fileIsDir                bool
		removeDirErr             error
		existingLogConfigReadErr error
		existingLogConfig        string
		createNewConfigError     error
	}{
		{
			expectedFile:             fmt.Sprintf("seelog-%s.xml", hash),
			expectedError:            nil,
			execAgentConfigFileExist: true,
			fileIsDir:                false,
			removeDirErr:             nil,
			existingLogConfigReadErr: nil,
			existingLogConfig:        execAgentLogConfigTemplate,
			createNewConfigError:     nil,
		},
		{
			expectedFile:             fmt.Sprintf("seelog-%s.xml", hash),
			expectedError:            nil,
			execAgentConfigFileExist: false,
			fileIsDir:                false,
			removeDirErr:             nil,
			existingLogConfigReadErr: nil,
			existingLogConfig:        execAgentLogConfigTemplate,
			createNewConfigError:     nil,
		},
		{
			expectedFile:             fmt.Sprintf("seelog-%s.xml", hash),
			expectedError:            nil,
			execAgentConfigFileExist: true,
			fileIsDir:                false,
			removeDirErr:             nil,
			existingLogConfigReadErr: nil,
			existingLogConfig:        "junk",
			createNewConfigError:     nil,
		},
		{
			expectedFile:             fmt.Sprintf("seelog-%s.xml", hash),
			expectedError:            nil,
			execAgentConfigFileExist: true,
			fileIsDir:                false,
			removeDirErr:             nil,
			existingLogConfigReadErr: errors.New("read file error"),
			existingLogConfig:        "",
			createNewConfigError:     nil,
		},
		{
			expectedFile:             "",
			expectedError:            errors.New("create file error"),
			execAgentConfigFileExist: false,
			fileIsDir:                false,
			removeDirErr:             nil,
			existingLogConfigReadErr: nil,
			existingLogConfig:        "",
			createNewConfigError:     errors.New("create file error"),
		},
		{
			expectedFile:             fmt.Sprintf("seelog-%s.xml", hash),
			expectedError:            nil,
			execAgentConfigFileExist: true,
			fileIsDir:                true,
			removeDirErr:             nil,
			existingLogConfigReadErr: nil,
			existingLogConfig:        execAgentLogConfigTemplate,
			createNewConfigError:     nil,
		},
		{
			expectedFile:             "",
			expectedError:            errors.New("remove dir error"),
			execAgentConfigFileExist: true,
			fileIsDir:                true,
			removeDirErr:             errors.New("remove dir error"),
			existingLogConfigReadErr: nil,
			existingLogConfig:        execAgentLogConfigTemplate,
			createNewConfigError:     nil,
		},
	}
	defer func() {
		osStat = os.Stat
		getFileContent = readFileContent
		createNewExecAgentConfigFile = createNewConfigFile
		removeAll = os.RemoveAll
	}()
	for _, tc := range tests {
		osStat = func(name string) (os.FileInfo, error) {
			if tc.execAgentConfigFileExist {
				return &mockFileInfo{name: "", isDir: tc.fileIsDir}, nil
			}
			return &mockFileInfo{}, errors.New("no such file")
		}
		removeAll = func(name string) error {
			return tc.removeDirErr
		}
		getFileContent = func(path string) ([]byte, error) {
			return []byte(tc.existingLogConfig), tc.existingLogConfigReadErr
		}
		createNewExecAgentConfigFile = func(c, f string) error {
			return tc.createNewConfigError
		}
		configFile, err := GetExecAgentLogConfigFile()
		assert.Equal(t, tc.expectedFile, configFile)
		assert.Equal(t, tc.expectedError, err)
	}
}

func TestGetValidConfigExists(t *testing.T) {
	var tests = []struct {
		isValid                  bool
		existingLogConfigReadErr error
		existingLogConfig        string
	}{
		{
			isValid:                  true,
			existingLogConfigReadErr: nil,
			existingLogConfig:        execAgentLogConfigTemplate,
		},
		{
			isValid:                  false,
			existingLogConfigReadErr: nil,
			existingLogConfig:        "junk",
		},
		{
			isValid:                  false,
			existingLogConfigReadErr: errors.New("read file error"),
			existingLogConfig:        "",
		},
	}
	defer func() {
		getFileContent = readFileContent
	}()
	for _, tc := range tests {
		getFileContent = func(path string) ([]byte, error) {
			return []byte(tc.existingLogConfig), tc.existingLogConfigReadErr
		}
		assert.Equal(t, tc.isValid, validConfigExists("configpath", getExecAgentConfigHash(execAgentLogConfigTemplate)))
	}
}
