//go:build unit

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
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/pborman/uuid"

	"github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	errors2 "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"

	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func getAgentMetadata(container *container.Container) AgentMetadata {
	ma, _ := container.GetManagedAgentByName(ExecuteCommandAgentName)
	return MapToAgentMetadata(ma.Metadata)
}

func TestStartAgent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	const (
		testPid1               = 9876
		testContainerRuntimeId = "123abc"
		testDockerExecId       = "mockDockerExecID"
	)
	nowTime := time.Now()
	zeroTime := time.Time{}
	var (
		mockError      = errors.New("mock error")
		testContainers = []*apicontainer.Container{{
			RuntimeID: testContainerRuntimeId,
		}}
	)

	tt := []struct {
		name                       string
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
		expectedStatus             apicontainerstatus.ManagedAgentStatus
		expectedStartTime          time.Time
	}{
		{
			name:        "test exec disabled",
			execEnabled: false,
			containers:  testContainers,
		},
		{
			name:                      "test with create container error",
			execEnabled:               true,
			containers:                testContainers,
			expectCreateContainerExec: true,
			expectedStatus:            apicontainerstatus.ManagedAgentStopped,
			createContainerExecErr:    mockError,
			expectedError:             StartError{error: fmt.Errorf("unable to start ExecuteCommandAgent [create]: %v", mockError), retryable: true},
			expectedStartTime:         zeroTime,
		},
		{
			name:                      "test with start container error",
			execEnabled:               true,
			containers:                testContainers,
			expectCreateContainerExec: true,
			createContainerExecRes: &types.IDResponse{
				ID: testDockerExecId,
			},
			expectStartContainerExec: true,
			expectedStatus:           apicontainerstatus.ManagedAgentStopped,
			startContainerExecErr:    mockError,
			expectedError:            StartError{error: fmt.Errorf("unable to start ExecuteCommandAgent [pre-start]: %v", mockError), retryable: true},
			expectedStartTime:        zeroTime,
		},
		{
			name:                      "test with inspect container error",
			execEnabled:               true,
			containers:                testContainers,
			expectCreateContainerExec: true,
			createContainerExecRes: &types.IDResponse{
				ID: testDockerExecId,
			},
			expectStartContainerExec:   true,
			startContainerExecErr:      nil, // Simulate StartContainerExec succeeds
			expectInspectContainerExec: true,
			expectedStatus:             apicontainerstatus.ManagedAgentStopped,
			inspectContainerExecErr:    mockError,
			expectedError:              StartError{error: fmt.Errorf("unable to start ExecuteCommandAgent [inspect]: %v", mockError), retryable: true},
			expectedStartTime:          zeroTime,
		},
		{
			name:                      "test happy path",
			execEnabled:               true,
			containers:                testContainers,
			expectCreateContainerExec: true,
			createContainerExecRes: &types.IDResponse{
				ID: testDockerExecId,
			},
			expectStartContainerExec:   true,
			startContainerExecErr:      nil, // Simulate StartContainerExec succeeds
			expectInspectContainerExec: true,
			expectedStatus:             apicontainerstatus.ManagedAgentRunning,
			expectedStartTime:          nowTime,
			inspectContainerExecRes: &types.ContainerExecInspect{
				ExecID:  testDockerExecId,
				Pid:     testPid1,
				Running: true,
			},
		},
	}
	testUUID := "test-uid"
	defer func() {
		newUUID = uuid.New
	}()
	newUUID = func() string {
		return testUUID
	}
	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {

			testTask := &apitask.Task{
				Arn:        "taskArn:aws:ecs:region:account-id:task/test-task-taskArn",
				Containers: test.containers,
			}
			if test.execEnabled {
				for _, c := range testTask.Containers {
					c.ManagedAgentsUnsafe = []apicontainer.ManagedAgent{
						{
							Name: ExecuteCommandAgentName,
							ManagedAgentState: apicontainer.ManagedAgentState{
								ID: testUUID,
							},
						},
					}
				}
			}

			times := maxRetries
			retryableErr, isRetryable := test.expectedError.(errors2.RetriableError)
			if test.expectedError == nil || (isRetryable && !retryableErr.Retry()) {
				times = 1
			}
			if test.expectCreateContainerExec {
				execCfg := types.ExecConfig{
					User:   specUser,
					Detach: true,
					Cmd:    []string{specTestCmd},
				}
				client.EXPECT().CreateContainerExec(gomock.Any(), testTask.Containers[0].RuntimeID, execCfg, dockerclient.ContainerExecCreateTimeout).
					Return(test.createContainerExecRes, test.createContainerExecErr).
					Times(times)
			}

			if test.expectStartContainerExec {
				client.EXPECT().StartContainerExec(gomock.Any(), testDockerExecId, gomock.Any(), dockerclient.ContainerExecStartTimeout).
					Return(test.startContainerExecErr).
					Times(times)
			}

			if test.expectInspectContainerExec {
				client.EXPECT().InspectContainerExec(gomock.Any(), testDockerExecId, dockerclient.ContainerExecInspectTimeout).
					Return(test.inspectContainerExecRes, test.inspectContainerExecErr).
					Times(times)
			}

			mgr := newTestManager()
			prevMetadata := getAgentMetadata(test.containers[0])
			err := mgr.StartAgent(context.TODO(), client, testTask, testTask.Containers[0], testTask.Containers[0].RuntimeID)
			if test.expectedError != nil {
				assert.Equal(t, test.expectedError, err, "Wrong error returned")
				// When there's an error, ExecCommandAgentMetadata should not be modified
				newMetadata := getAgentMetadata(test.containers[0])
				assert.Equal(t, prevMetadata, newMetadata)
			} else { // No error case
				assert.NoError(t, err, "No error was expected")
				if !test.execEnabled {
					_, ok := test.containers[0].GetManagedAgentByName(ExecuteCommandAgentName)
					assert.False(t, ok)
				} else {
					ma, _ := test.containers[0].GetManagedAgentByName(ExecuteCommandAgentName)
					execMD := getAgentMetadata(test.containers[0])
					assert.Equal(t, strconv.Itoa(testPid1), execMD.PID, "PID not equal")
					assert.Equal(t, testDockerExecId, execMD.DockerExecID, "DockerExecId not equal")
					assert.Equal(t, specTestCmd, execMD.CMD)
					assert.Equal(t, test.expectedStatus, ma.Status, "Exec status not equal")
					assert.WithinDuration(t, test.expectedStartTime, ma.LastStartedAt, 5*time.Second, "StartedAt not equal")
				}
			}
		})
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
		testPidStr = strconv.Itoa(testPid)
		testCmd    = specTestCmd
	)
	testUUID := "test-uid"
	defer func() {
		newUUID = uuid.New
	}()
	newUUID = func() string {
		return testUUID
	}

	testTask := &apitask.Task{
		Arn: "taskArn:aws:ecs:region:account-id:task/test-task-taskArn",
		Containers: []*apicontainer.Container{{
			RuntimeID: "123",
			ManagedAgentsUnsafe: []apicontainer.ManagedAgent{
				{
					Name: ExecuteCommandAgentName,
					ManagedAgentState: apicontainer.ManagedAgentState{
						ID: testUUID,
					},
				},
			},
		}},
	}

	execCfg := types.ExecConfig{
		User:   specUser,
		Detach: true,
		Cmd:    []string{specTestCmd},
	}
	client.EXPECT().CreateContainerExec(gomock.Any(), testTask.Containers[0].RuntimeID, execCfg, dockerclient.ContainerExecCreateTimeout).
		Return(&types.IDResponse{ID: testDockerExecId}, nil).
		Times(1)

	client.EXPECT().StartContainerExec(gomock.Any(), testDockerExecId, gomock.Any(), dockerclient.ContainerExecStartTimeout).
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

	ma, _ := testTask.Containers[0].GetManagedAgentByName(ExecuteCommandAgentName)
	execMD := getAgentMetadata(testTask.Containers[0])
	firstStart := ma.LastStartedAt
	assert.NotNil(t, ma.LastStartedAt)
	assert.Equal(t, testPidStr, execMD.PID)
	assert.Equal(t, testDockerExecId, execMD.DockerExecID)
	assert.Equal(t, testCmd, execMD.CMD)
	assert.Equal(t, apicontainerstatus.ManagedAgentRunning, ma.Status)

	// Second call to start. The mock's expected call times is 1 (except for inspect); the absence of "too many calls"
	// along with unchanged metadata guarantees idempotency
	err = mgr.StartAgent(context.TODO(), client, testTask, testTask.Containers[0], testTask.Containers[0].RuntimeID)
	assert.NoError(t, err)

	ma, _ = testTask.Containers[0].GetManagedAgentByName(ExecuteCommandAgentName)
	execMD = getAgentMetadata(testTask.Containers[0])
	// check StartedAt is not Zero and hasn't been updated
	assert.NotNil(t, ma.LastStartedAt)
	assert.False(t, ma.LastStartedAt.IsZero())
	assert.Equal(t, ma.LastStartedAt, firstStart)
	assert.Equal(t, testPidStr, execMD.PID)
	assert.Equal(t, testDockerExecId, execMD.DockerExecID)
	assert.Equal(t, testCmd, execMD.CMD)
	assert.Equal(t, apicontainerstatus.ManagedAgentRunning, ma.Status)
}

func TestRestartAgentIfStopped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	nowTime := time.Now()

	const (
		testContainerId     = "123"
		testNewDockerExecID = "newDockerExecId"
		testNewPID          = 111
	)
	testUUID := "test-uid"
	defer func() {
		newUUID = uuid.New
	}()
	newUUID = func() string {
		return testUUID
	}
	var (
		mockError          = errors.New("mock error")
		dockerTimeoutErr   = &dockerapi.DockerTimeoutError{}
		testExecAgentState = apicontainer.ManagedAgentState{
			ID:            testUUID,
			LastStartedAt: nowTime,
			Metadata: map[string]interface{}{
				"PID":          "456",
				"DockerExecID": "789",
				"CMD":          "amazon-ssm-agent",
			},
		}
	)
	tt := []struct {
		name                    string
		execEnabled             bool
		expectedRestartStatus   RestartStatus
		execAgentState          apicontainer.ManagedAgentState
		containerExecInspectRes *types.ContainerExecInspect
		expectedInspectErr      error
		expectedRestartErr      error
		expectedExecAgentStatus apicontainerstatus.ManagedAgentStatus
	}{
		{
			name:                  "test with exec agent disabled",
			execEnabled:           false,
			expectedRestartStatus: NotRestarted,
		},
		{
			name:                    "test with inspect timeout error",
			execEnabled:             true,
			execAgentState:          testExecAgentState,
			expectedInspectErr:      dockerTimeoutErr,
			expectedRestartErr:      nil,
			expectedRestartStatus:   Unknown,
			expectedExecAgentStatus: apicontainerstatus.ManagedAgentStopped,
		},
		{
			name:                    "test with other inspect error",
			execEnabled:             true,
			execAgentState:          testExecAgentState,
			expectedInspectErr:      mockError,
			expectedRestartErr:      nil,
			expectedRestartStatus:   Unknown,
			expectedExecAgentStatus: apicontainerstatus.ManagedAgentStopped,
		},
		{
			name:           "test with exec command still running",
			execEnabled:    true,
			execAgentState: testExecAgentState,
			containerExecInspectRes: &types.ContainerExecInspect{
				Running: true,
			},
			expectedRestartStatus:   NotRestarted,
			expectedExecAgentStatus: apicontainerstatus.ManagedAgentRunning,
		},
		{
			name:           "test with exec command stopped",
			execEnabled:    true,
			execAgentState: testExecAgentState,
			containerExecInspectRes: &types.ContainerExecInspect{
				Running: false,
			},
			expectedRestartStatus:   Restarted,
			expectedExecAgentStatus: apicontainerstatus.ManagedAgentRunning,
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			testTask := &apitask.Task{
				Arn: "taskArn:aws:ecs:region:account-id:task/test-task-taskArn",
				Containers: []*apicontainer.Container{{
					RuntimeID: testContainerId,
				}},
			}

			if test.execEnabled {
				testTask.Containers[0].ManagedAgentsUnsafe = []apicontainer.ManagedAgent{
					{
						Name:              ExecuteCommandAgentName,
						ManagedAgentState: test.execAgentState,
					},
				}
				times := 1
				if test.expectedInspectErr == mockError {
					times = maxRetries
				}
				execMD := getAgentMetadata(testTask.Containers[0])
				client.EXPECT().InspectContainerExec(gomock.Any(), execMD.DockerExecID, dockerclient.ContainerExecInspectTimeout).
					Return(test.containerExecInspectRes, test.expectedInspectErr).Times(times)
			}

			// Expect calls made by Start()
			if test.containerExecInspectRes != nil && !test.containerExecInspectRes.Running {
				client.EXPECT().CreateContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&types.IDResponse{ID: testNewDockerExecID}, nil).
					Times(1)

				client.EXPECT().StartContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
				ma, _ := testTask.Containers[0].GetManagedAgentByName(ExecuteCommandAgentName)
				actualState := ma.ManagedAgentState
				assert.Equal(t, test.execAgentState, actualState, "ExecCommandAgentMetadata was incorrectly modified")
			} else {
				ma, _ := testTask.Containers[0].GetManagedAgentByName(ExecuteCommandAgentName)
				actualState := ma.ManagedAgentState
				execMD := getAgentMetadata(testTask.Containers[0])
				assert.Equal(t, strconv.Itoa(testNewPID), execMD.PID,
					"ExecCommandAgentMetadata.PID is not the newest after restart")
				assert.Equal(t, testNewDockerExecID, execMD.DockerExecID,
					"ExecCommandAgentMetadata.ExecID is not the newest after restart")
				assert.Equal(t, specTestCmd, execMD.CMD,
					"ExecCommandAgentMetadata.CMD is not the newest after restart")
				assert.Equal(t, test.expectedExecAgentStatus, actualState.Status,
					"ExecCommandAgentStatus is not correct after restart")
			}
		})
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
