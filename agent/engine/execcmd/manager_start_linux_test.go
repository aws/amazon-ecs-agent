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
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	errors2 "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"

	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestStartAgent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	const (
		testPid1               = 9876
		testContainerRuntimeId = "123abc"
		testDockerExecId       = "mockDockerExecID"
	)

	var (
		mockError      = errors.New("mock error")
		mockStartError = StartError{error: mockError, retryable: true}
		testContainers = []*apicontainer.Container{{
			RuntimeID: testContainerRuntimeId,
		}}
	)

	tt := []struct {
		execEnabled                bool
		containers                 []*apicontainer.Container
		expectCreateContainerExec  bool
		createContainerExecRes     *types.IDResponse
		createContainerExecErr     error
		expectStartContainerExec   bool
		startContainerExecErr      error
		expectInspectContainerExec bool
		inspectContainerExecRes    *types.ContainerExecInspect
		inspectContainerExecErr    error
		expectedError              error
	}{
		{
			execEnabled: false,
			containers:  testContainers,
		},
		{
			execEnabled:               true,
			containers:                testContainers,
			expectCreateContainerExec: true,
			createContainerExecErr:    mockError,
			expectedError:             mockStartError,
		},
		{
			execEnabled:               true,
			containers:                testContainers,
			expectCreateContainerExec: true,
			createContainerExecRes: &types.IDResponse{
				ID: testDockerExecId,
			},
			expectStartContainerExec: true,
			startContainerExecErr:    mockError,
			expectedError:            mockStartError,
		},
		{
			execEnabled:               true,
			containers:                testContainers,
			expectCreateContainerExec: true,
			createContainerExecRes: &types.IDResponse{
				ID: testDockerExecId,
			},
			expectStartContainerExec:   true,
			startContainerExecErr:      nil, // Simulate StartContainerExec succeeds
			expectInspectContainerExec: true,
			inspectContainerExecErr:    mockError,
			expectedError:              mockStartError,
		},
		{
			execEnabled:               true,
			containers:                testContainers,
			expectCreateContainerExec: true,
			createContainerExecRes: &types.IDResponse{
				ID: testDockerExecId,
			},
			expectStartContainerExec:   true,
			startContainerExecErr:      nil, // Simulate StartContainerExec succeeds
			expectInspectContainerExec: true,
			inspectContainerExecRes: &types.ContainerExecInspect{
				ExecID:  testDockerExecId,
				Pid:     testPid1,
				Running: true,
			},
		},
	}
	for _, test := range tt {
		testTask := &apitask.Task{
			Arn:                     "taskArn:aws:ecs:region:account-id:task/test-task-taskArn",
			Containers:              test.containers,
			ExecCommandAgentEnabled: test.execEnabled,
		}

		times := maxRetries
		retryableErr, isRetryable := test.expectedError.(errors2.RetriableError)
		if test.expectedError == nil || (isRetryable && !retryableErr.Retry()) {
			times = 1
		}
		if test.expectCreateContainerExec {
			execCfg := types.ExecConfig{
				Detach: true,
				Cmd:    []string{filepath.Join(ContainerBinDir, BinName)},
			}
			client.EXPECT().CreateContainerExec(gomock.Any(), testTask.Containers[0].RuntimeID, execCfg, dockerclient.ContainerExecCreateTimeout).
				Return(test.createContainerExecRes, test.createContainerExecErr).
				Times(times)
		}

		if test.expectStartContainerExec {
			client.EXPECT().StartContainerExec(gomock.Any(), testDockerExecId, dockerclient.ContainerExecStartTimeout).
				Return(test.startContainerExecErr).
				Times(times)
		}

		if test.expectInspectContainerExec {
			client.EXPECT().InspectContainerExec(gomock.Any(), testDockerExecId, dockerclient.ContainerExecInspectTimeout).
				Return(test.inspectContainerExecRes, test.inspectContainerExecErr).
				Times(times)
		}

		mgr := newTestManager()
		err := mgr.StartAgent(context.TODO(), client, testTask, testTask.Containers[0], testTask.Containers[0].RuntimeID)
		if test.expectedError != nil {
			assert.Equal(t, test.expectedError, err, "Wrong error returned")
			// When there's an error, ExecCommandAgentMetadata should not be modified
			assert.Equal(t, test.containers[0].GetExecCommandAgentMetadata(), testTask.Containers[0].ExecCommandAgentMetadata)
		} else { // No error case
			assert.NoError(t, err, "No error was expected")
			if !test.execEnabled {
				assert.Nil(t, testTask.Containers[0].GetExecCommandAgentMetadata())
			} else {
				assert.Equal(t, &apicontainer.ExecCommandAgentMetadata{
					PID:          strconv.Itoa(testPid1),
					DockerExecID: testDockerExecId,
					CMD:          filepath.Join(ContainerBinDir, BinName),
				}, testTask.Containers[0].GetExecCommandAgentMetadata())
			}
		}
	}
}

func TestIdempotentStartAgent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	const (
		testDockerExecId = "abc"
		testPid          = 111
	)
	var (
		testPidStr     = strconv.Itoa(testPid)
		testCmd        = filepath.Join(ContainerBinDir, BinName)
		expectedExecMD = &apicontainer.ExecCommandAgentMetadata{
			PID:          testPidStr,
			DockerExecID: testDockerExecId,
			CMD:          testCmd,
		}
	)

	testTask := &apitask.Task{
		Arn: "taskArn:aws:ecs:region:account-id:task/test-task-taskArn",
		Containers: []*apicontainer.Container{{
			RuntimeID: "123",
		}},
		ExecCommandAgentEnabled: true,
	}

	execCfg := types.ExecConfig{
		Detach: true,
		Cmd:    []string{filepath.Join(ContainerBinDir, BinName)},
	}
	client.EXPECT().CreateContainerExec(gomock.Any(), testTask.Containers[0].RuntimeID, execCfg, dockerclient.ContainerExecCreateTimeout).
		Return(&types.IDResponse{ID: testDockerExecId}, nil).
		Times(1)

	client.EXPECT().StartContainerExec(gomock.Any(), testDockerExecId, dockerclient.ContainerExecStartTimeout).
		Return(nil).
		Times(1)

	client.EXPECT().InspectContainerExec(gomock.Any(), testDockerExecId, dockerclient.ContainerExecInspectTimeout).
		Return(&types.ContainerExecInspect{
			ExecID:  testDockerExecId,
			Pid:     testPid,
			Running: true,
		}, nil).
		Times(2)

	mgr := newTestManager()
	err := mgr.StartAgent(context.TODO(), client, testTask, testTask.Containers[0], testTask.Containers[0].RuntimeID)
	assert.NoError(t, err)
	assert.Equal(t, expectedExecMD, testTask.Containers[0].GetExecCommandAgentMetadata())

	// Second call to start. The mock's expected call times is 1 (except for inspect); the absence of "too many calls"
	// along with unchanged metadata guarantee idempotency
	err = mgr.StartAgent(context.TODO(), client, testTask, testTask.Containers[0], testTask.Containers[0].RuntimeID)
	assert.NoError(t, err)
	assert.Equal(t, expectedExecMD, testTask.Containers[0].GetExecCommandAgentMetadata())
}

func TestRestartAgentIfStopped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)

	const (
		testContainerId     = "123"
		testNewDockerExecID = "newDockerExecId"
		testNewPID          = 111
	)
	var (
		mockError        = errors.New("mock error")
		dockerTimeoutErr = &dockerapi.DockerTimeoutError{}
		testExecCmdMD    = &apicontainer.ExecCommandAgentMetadata{
			PID:          "456",
			DockerExecID: "789",
			CMD:          "amazon-ssm-agent",
		}
	)

	tt := []struct {
		execEnabled             bool
		expectedRestartStatus   RestartStatus
		execCmdMD               *apicontainer.ExecCommandAgentMetadata
		containerExecInspectRes *types.ContainerExecInspect
		expectedInspectErr      error
		expectedRestartErr      error
	}{
		{
			execEnabled:           false,
			expectedRestartStatus: NotRestarted,
		},
		{
			execEnabled:           true, // exec enabled but no exec cmd metadata
			expectedRestartStatus: NotRestarted,
		},
		{
			execEnabled:           true,
			execCmdMD:             testExecCmdMD,
			expectedInspectErr:    dockerTimeoutErr,
			expectedRestartErr:    nil,
			expectedRestartStatus: Unknown,
		},
		{
			execEnabled:           true,
			execCmdMD:             testExecCmdMD,
			expectedInspectErr:    mockError,
			expectedRestartErr:    nil,
			expectedRestartStatus: Unknown,
		},
		{
			execEnabled: true,
			execCmdMD:   testExecCmdMD,
			containerExecInspectRes: &types.ContainerExecInspect{
				Running: true,
			},
			expectedRestartStatus: NotRestarted,
		},
		{
			execEnabled: true,
			execCmdMD:   testExecCmdMD,
			containerExecInspectRes: &types.ContainerExecInspect{
				Running: false,
			},
			expectedRestartStatus: Restarted,
		},
	}

	for _, test := range tt {
		testTask := &apitask.Task{
			Arn: "taskArn:aws:ecs:region:account-id:task/test-task-taskArn",
			Containers: []*apicontainer.Container{{
				RuntimeID:                testContainerId,
				ExecCommandAgentMetadata: test.execCmdMD,
			}},
			ExecCommandAgentEnabled: test.execEnabled,
		}

		if test.execEnabled && test.execCmdMD != nil {
			times := 1
			if test.expectedInspectErr == mockError {
				times = maxRetries
			}
			client.EXPECT().InspectContainerExec(gomock.Any(), test.execCmdMD.DockerExecID, dockerclient.ContainerExecInspectTimeout).
				Return(test.containerExecInspectRes, test.expectedInspectErr).Times(times)
		}

		// Expect calls made by Start()
		if test.containerExecInspectRes != nil && !test.containerExecInspectRes.Running {
			client.EXPECT().CreateContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&types.IDResponse{ID: testNewDockerExecID}, nil).
				Times(1)

			client.EXPECT().StartContainerExec(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil).
				Times(1)

			client.EXPECT().InspectContainerExec(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&types.ContainerExecInspect{
					ExecID:  testNewDockerExecID,
					Pid:     testNewPID,
					Running: true,
				}, nil).
				Times(1)
		}

		mgr := newTestManager()
		restarted, err := mgr.RestartAgentIfStopped(context.TODO(), client, testTask, testTask.Containers[0], testTask.Containers[0].RuntimeID)
		assert.Equal(t, test.expectedRestartErr, err)
		assert.Equal(t, test.expectedRestartStatus, restarted, "expected: %s, actual: %s", test.expectedRestartStatus, restarted)

		if test.expectedRestartStatus != Restarted {
			assert.Equal(t, test.execCmdMD, testTask.Containers[0].GetExecCommandAgentMetadata(), "ExecCommandAgentMetadata was incorrectly modified")
		} else {
			assert.Equal(t, &apicontainer.ExecCommandAgentMetadata{
				PID:          strconv.Itoa(testNewPID),
				DockerExecID: testNewDockerExecID,
				CMD:          filepath.Join(ContainerBinDir, BinName),
			}, testTask.Containers[0].GetExecCommandAgentMetadata(), "ExecCommandAgentMetadata is not the newest after restart")
		}
	}
}

func newTestManager() *manager {
	m := NewManager()
	m.retryMaxDelay = time.Millisecond * 30
	m.retryMinDelay = time.Millisecond * 1
	m.startRetryTimeout = time.Second * 2
	m.inspectRetryTimeout = time.Second
	return m
}
