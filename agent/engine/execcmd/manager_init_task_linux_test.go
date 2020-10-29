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
	"strconv"
	"testing"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
)

func TestInitializeContainer(t *testing.T) {
	const containerNameOnlyHyphens = "--"
	var tt = []struct {
		managedAgentName                string
		getExecAgentConfigFileNameError error
	}{
		{
			managedAgentName: "randomAgent",
		},
		{
			managedAgentName:                ExecuteCommandAgentName,
			getExecAgentConfigFileNameError: errors.New("mockError"),
		},
		{
			managedAgentName: ExecuteCommandAgentName,
		},
	}
	defer func() {
		GetExecAgentConfigFileName = getAgentConfigFileName
		newUUID = uuid.New
	}()
	for _, test := range tt {
		task := &apitask.Task{
			Arn: "arn:aws:iam::123456789012:user/Test",
			Containers: []*apicontainer.Container{
				{
					Name: "container-name",
				},
				{
					Name: "--container-name",
				},
				{
					Name: containerNameOnlyHyphens, // Container that will end up in an empty directory name (trailing hyphens are removed)
				},
			},
		}

		// This makes task exec-enabled
		for _, c := range task.Containers {
			c.ManagedAgentsUnsafe = []apicontainer.ManagedAgent{
				{
					Name: test.managedAgentName,
				},
			}
		}
		execCmdMgr := newTestManager()
		GetExecAgentConfigFileName = func(s int) (string, error) {
			return "amazon-ssm-agent.json", test.getExecAgentConfigFileNameError
		}
		newUUID = func() string {
			return "test-UUID"
		}
		for _, container := range task.Containers {
			hc := &dockercontainer.HostConfig{}
			err := execCmdMgr.InitializeContainer("task-id", container, hc)
			var expectedError error
			if test.managedAgentName != ExecuteCommandAgentName {
				expectedError = errExecCommandManagedAgentNotFound
			}
			if test.getExecAgentConfigFileNameError != nil {
				expectedError = fmt.Errorf("could not generate ExecAgent Config File: %v", test.getExecAgentConfigFileNameError)
			}

			assert.Equal(t, expectedError, err)

			if expectedError != nil {
				assert.Empty(t, hc.Binds)
				continue
			}

			assert.Len(t, hc.Binds, 6)

			assert.Subset(t, hc.Binds, []string{
				"/var/lib/ecs/deps/execute-command/bin/3.0.236.0/amazon-ssm-agent:" +
					"/ecs-execute-command-test-UUID/amazon-ssm-agent:ro"})

			assert.Subset(t, hc.Binds, []string{
				"/var/lib/ecs/deps/execute-command/bin/3.0.236.0/ssm-agent-worker:" +
					"/ecs-execute-command-test-UUID/ssm-agent-worker:ro"})

			assert.Subset(t, hc.Binds, []string{
				"/var/lib/ecs/deps/execute-command/bin/3.0.236.0/ssm-session-worker:" +
					"/ecs-execute-command-test-UUID/ssm-session-worker:ro"})

			assert.Subset(t, hc.Binds, []string{
				"/var/lib/ecs/deps/execute-command/config/amazon-ssm-agent.json:" +
					"/ecs-execute-command-test-UUID/configuration/amazon-ssm-agent.json:ro"})

			assert.Subset(t, hc.Binds, []string{
				"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem:" +
					"/ecs-execute-command-test-UUID/certs/amazon-ssm-agent.crt:ro"})

			expectedHostLogDir := "/var/log/ecs/exec/task-id/container-name:"
			if container.Name == containerNameOnlyHyphens {
				expectedHostLogDir = "/var/log/ecs/exec/task-id/nameless-container-test-UUID:"
			}
			assert.Subset(t, hc.Binds, []string{
				expectedHostLogDir +
					"/var/log/amazon/ssm"})
		}
	}
}

func TestGetExecAgentConfigFileName(t *testing.T) {
	execAgentConfig := `{
	"Agent": {
		"Region": "",
		"OrchestrationRootDir": "",
		"ContainerMode": true,
		"Mgs": {
			"Region": "",
			"Endpoint": "",
			"StopTimeoutMillis": 20000,
			"SessionWorkersLimit": 2
		}
	}
}`
	sha := getExecAgentConfigHash(execAgentConfig)
	configFileName := fmt.Sprintf("amazon-ssm-agent-%s.json", sha)
	var tests = []struct {
		fileExists             bool
		createConfigFileErr    error
		expectedConfigFileName string
		expectedError          error
	}{
		{
			fileExists:             true,
			createConfigFileErr:    nil,
			expectedConfigFileName: configFileName,
			expectedError:          nil,
		},
		{
			fileExists:             false,
			createConfigFileErr:    nil,
			expectedConfigFileName: configFileName,
			expectedError:          nil,
		},
		{
			fileExists:             false,
			createConfigFileErr:    errors.New("cannot create config"),
			expectedConfigFileName: "",
			expectedError:          errors.New("cannot create config"),
		},
	}
	defer func() {
		execAgentConfigFileExists = configFileExists
		createNewExecAgentConfigFile = createNewConfigFile
	}()
	for _, tc := range tests {
		execAgentConfigFileExists = func(file string) bool {
			return tc.fileExists
		}
		createNewExecAgentConfigFile = func(c, f string) error {
			return tc.createConfigFileErr
		}
		fileName, err := GetExecAgentConfigFileName(2)
		assert.Equal(t, tc.expectedConfigFileName, fileName, "incorrect config file name")
		assert.Equal(t, tc.expectedError, err)
	}
}

func TestGetSessionWorkersLimit(t *testing.T) {
	var tests = []struct {
		sessionLimit  int
		expectedLimit int
	}{
		{
			sessionLimit:  -2,
			expectedLimit: 2,
		},
		{
			sessionLimit:  0,
			expectedLimit: 2,
		},
		{
			sessionLimit:  1,
			expectedLimit: 1,
		},
		{
			sessionLimit:  2,
			expectedLimit: 2,
		},
	}
	for _, tc := range tests {
		ma := apicontainer.ManagedAgent{
			Properties: map[string]string{
				"SessionWorkersLimit": strconv.Itoa(tc.sessionLimit),
			},
		}
		limit := getSessionWorkersLimit(ma)
		assert.Equal(t, tc.expectedLimit, limit)
	}
}
