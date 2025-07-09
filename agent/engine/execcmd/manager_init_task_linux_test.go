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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
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

			GetExecAgentConfigFileName = func(s int, cfg *config.Config, task *apitask.Task) (string, error) {
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
				task := &apitask.Task{Arn: "arn:aws:ecs:us-west-2:abc:task/my-cluster/task-id"}
				err := execCmdMgr.InitializeContainer(task, container, hc)

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
	},
	"Ssm": {
		"Endpoint": ""
	},
	"Mds": {
		"Endpoint": ""
	},
	"S3": {
		"Endpoint": ""
	},
	"Kms": {
		"Endpoint": ""
	},
	"CloudWatch": {
		"Endpoint": ""
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
		agentCfg := &config.Config{}
		task := &apitask.Task{}
		fileName, err := GetExecAgentConfigFileName(2, agentCfg, task)
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

func TestFormatSSMAgentConfig(t *testing.T) {
	// Define a struct that matches the SSM agent config structure
	type SSMAgentConfig struct {
		Mgs struct {
			Region              string `json:"Region"`
			Endpoint            string `json:"Endpoint"`
			StopTimeoutMillis   int    `json:"StopTimeoutMillis"`
			SessionWorkersLimit int    `json:"SessionWorkersLimit"`
		} `json:"Mgs"`
		Agent struct {
			Region               string `json:"Region"`
			OrchestrationRootDir string `json:"OrchestrationRootDir"`
			ContainerMode        bool   `json:"ContainerMode"`
		} `json:"Agent"`
		Ssm struct {
			Endpoint string `json:"Endpoint"`
		} `json:"Ssm"`
		Mds struct {
			Endpoint string `json:"Endpoint"`
		} `json:"Mds"`
		S3 struct {
			Endpoint string `json:"Endpoint"`
		} `json:"S3"`
		Kms struct {
			Endpoint string `json:"Endpoint"`
		} `json:"Kms"`
		CloudWatch struct {
			Endpoint string `json:"Endpoint"`
		} `json:"CloudWatch"`
	}

	type expectedEndpoints struct {
		mgs        string
		ssm        string
		mds        string
		s3         string
		kms        string
		cloudWatch string
	}

	// Create mock ENIs for testing
	ipv6OnlyENI := &ni.NetworkInterface{
		IPV6Addresses: []*ni.IPV6Address{{Address: "2001:db8::1", Primary: true}},
	}
	dualStackENI := &ni.NetworkInterface{
		IPV4Addresses: []*ni.IPV4Address{{Address: "192.168.1.1", Primary: true}},
		IPV6Addresses: []*ni.IPV6Address{{Address: "2001:db8::1", Primary: true}},
	}

	// Create mock tasks for testing
	ipv6OnlyTask := &apitask.Task{
		NetworkMode: apitask.AWSVPCNetworkMode,
		ENIs:        []*ni.NetworkInterface{ipv6OnlyENI},
	}
	dualStackTask := &apitask.Task{
		NetworkMode: apitask.AWSVPCNetworkMode,
		ENIs:        []*ni.NetworkInterface{dualStackENI},
	}
	bridgeTask := &apitask.Task{
		NetworkMode: apitask.BridgeNetworkMode,
	}

	expectedDualstackEndpointsIAD := expectedEndpoints{
		mgs:        "https://ssmmessages.us-east-1.api.aws",
		ssm:        "https://ssm.us-east-1.api.aws",
		mds:        "https://ec2messages.us-east-1.api.aws",
		s3:         "https://s3.dualstack.us-east-1.amazonaws.com",
		kms:        "https://kms.us-east-1.api.aws",
		cloudWatch: "https://logs.us-east-1.api.aws",
	}

	testCases := []struct {
		name              string
		sessionLimit      int
		cfg               *config.Config
		task              *apitask.Task
		expectedEndpoints expectedEndpoints
		expectError       bool
	}{
		{
			name:         "non-IPv6-only environment with bridge mode task",
			sessionLimit: 5,
			cfg: &config.Config{
				InstanceIPCompatibility: ipcompatibility.NewIPv4OnlyCompatibility(),
				AWSRegion:               "us-west-2",
			},
			task:        bridgeTask,
			expectError: false,
		},
		{
			name:         "IPv6-only environment with bridge mode task",
			sessionLimit: 8,
			cfg: &config.Config{
				InstanceIPCompatibility: ipcompatibility.NewIPv6OnlyCompatibility(),
				AWSRegion:               "us-east-1",
			},
			task:              bridgeTask,
			expectError:       false,
			expectedEndpoints: expectedDualstackEndpointsIAD,
		},
		{
			name:         "IPv6-only task ENI with AWSVPC mode",
			sessionLimit: 15,
			cfg: &config.Config{
				InstanceIPCompatibility: ipcompatibility.NewIPv4OnlyCompatibility(),
				AWSRegion:               "us-east-1",
			},
			task:              ipv6OnlyTask,
			expectError:       false,
			expectedEndpoints: expectedDualstackEndpointsIAD,
		},
		{
			name:         "Dual-stack task ENI with AWSVPC mode",
			sessionLimit: 15,
			cfg: &config.Config{
				InstanceIPCompatibility: ipcompatibility.NewIPv6OnlyCompatibility(),
				AWSRegion:               "us-east-1",
			},
			task:        dualStackTask,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := formatSSMAgentConfig(tc.sessionLimit, tc.cfg, tc.task)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Parse the JSON result into our struct
			var config SSMAgentConfig
			err = json.Unmarshal([]byte(result), &config)
			require.NoError(t, err, "Result should be valid JSON")

			// Check session workers limit is set correctly
			assert.Equal(t, tc.sessionLimit, config.Mgs.SessionWorkersLimit, "Session workers limit should match")

			// Verify container mode is true
			assert.True(t, config.Agent.ContainerMode, "Agent.ContainerMode should be true")

			// Assert each endpoint directly with its expected value from the test case
			assert.Equal(t, tc.expectedEndpoints.mgs, config.Mgs.Endpoint)
			assert.Equal(t, tc.expectedEndpoints.ssm, config.Ssm.Endpoint)
			assert.Equal(t, tc.expectedEndpoints.mds, config.Mds.Endpoint)
			assert.Equal(t, tc.expectedEndpoints.s3, config.S3.Endpoint)
			assert.Equal(t, tc.expectedEndpoints.kms, config.Kms.Endpoint)
			assert.Equal(t, tc.expectedEndpoints.cloudWatch, config.CloudWatch.Endpoint)
		})
	}
}
