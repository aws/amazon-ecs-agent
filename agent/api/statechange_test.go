//go:build unit
// +build unit

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

package api

import (
	"fmt"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	execcmd "github.com/aws/amazon-ecs-agent/agent/engine/execcmd"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

func TestShouldBeReported(t *testing.T) {
	cases := []struct {
		status          apitaskstatus.TaskStatus
		containerChange []ContainerStateChange
		result          bool
	}{
		{ // Normal task state change to running
			status: apitaskstatus.TaskRunning,
			result: true,
		},
		{ // Normal task state change to stopped
			status: apitaskstatus.TaskStopped,
			result: true,
		},
		{ // Container changed while task is not in steady state
			status: apitaskstatus.TaskCreated,
			containerChange: []ContainerStateChange{
				{TaskArn: "taskarn"},
			},
			result: true,
		},
		{ // No container change and task status not recognized
			status: apitaskstatus.TaskCreated,
			result: false,
		},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("task change status: %s, container change: %t", tc.status, len(tc.containerChange) > 0),
			func(t *testing.T) {
				taskChange := TaskStateChange{
					Status:     tc.status,
					Containers: tc.containerChange,
				}

				assert.Equal(t, tc.result, taskChange.ShouldBeReported())
			})
	}
}

func TestSetTaskTimestamps(t *testing.T) {
	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t2.Add(time.Second)

	change := &TaskStateChange{
		Task: &apitask.Task{
			PullStartedAtUnsafe:      t1,
			PullStoppedAtUnsafe:      t2,
			ExecutionStoppedAtUnsafe: t3,
		},
	}

	change.SetTaskTimestamps()
	assert.Equal(t, t1.UTC().String(), change.PullStartedAt.String())
	assert.Equal(t, t2.UTC().String(), change.PullStoppedAt.String())
	assert.Equal(t, t3.UTC().String(), change.ExecutionStoppedAt.String())
}

func TestSetContainerRuntimeID(t *testing.T) {
	task := &apitask.Task{}
	steadyStateStatus := apicontainerstatus.ContainerRunning
	Containers := []*apicontainer.Container{
		{
			RuntimeID:               "222",
			KnownStatusUnsafe:       apicontainerstatus.ContainerRunning,
			SentStatusUnsafe:        apicontainerstatus.ContainerStatusNone,
			Type:                    apicontainer.ContainerNormal,
			SteadyStateStatusUnsafe: &steadyStateStatus,
		},
	}

	task.Containers = Containers
	resp, ok := NewContainerStateChangeEvent(task, task.Containers[0], "")

	assert.NoError(t, ok, "error create newContainerStateChangeEvent")
	assert.Equal(t, "222", resp.RuntimeID)
}

func TestSetImageDigest(t *testing.T) {
	task := &apitask.Task{}
	steadyStateStatus := apicontainerstatus.ContainerRunning
	Containers := []*apicontainer.Container{
		{
			ImageDigest:             "sha256:d1c14fcf2e9476ed58ebc4251b211f403f271e96b6c3d9ada0f1c5454ca4d230",
			KnownStatusUnsafe:       apicontainerstatus.ContainerRunning,
			SentStatusUnsafe:        apicontainerstatus.ContainerStatusNone,
			Type:                    apicontainer.ContainerNormal,
			SteadyStateStatusUnsafe: &steadyStateStatus,
		},
	}

	task.Containers = Containers
	resp, ok := NewContainerStateChangeEvent(task, task.Containers[0], "")

	assert.NoError(t, ok, "error create newContainerStateChangeEvent")
	assert.Equal(t, "sha256:d1c14fcf2e9476ed58ebc4251b211f403f271e96b6c3d9ada0f1c5454ca4d230", resp.ImageDigest)
}

func TestNewUncheckedContainerStateChangeEvent(t *testing.T) {
	tests := []struct {
		name          string
		containerType apicontainer.ContainerType
	}{
		{
			name:          "internal container",
			containerType: apicontainer.ContainerEmptyHostVolume,
		},
		{
			name:          "normal container",
			containerType: apicontainer.ContainerNormal,
		},
	}
	steadyStateStatus := apicontainerstatus.ContainerRunning
	exitCode := 1
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := &apitask.Task{
				Arn: "arn:123",
				Containers: []*apicontainer.Container{
					{
						Name:                    "c1",
						RuntimeID:               "222",
						KnownStatusUnsafe:       apicontainerstatus.ContainerRunning,
						SentStatusUnsafe:        apicontainerstatus.ContainerStatusNone,
						Type:                    tc.containerType,
						SteadyStateStatusUnsafe: &steadyStateStatus,
						KnownExitCodeUnsafe:     &exitCode,
						KnownPortBindingsUnsafe: []apicontainer.PortBinding{{
							ContainerPort: aws.Uint16(1),
							HostPort:      2,
							BindIP:        "1.2.3.4",
							Protocol:      3,
						}},
						ImageDigest: "image",
					},
				}}

			expectedEvent := ContainerStateChange{
				TaskArn:       "arn:123",
				ContainerName: "c1",
				RuntimeID:     "222",
				Status:        apicontainerstatus.ContainerRunning,
				ExitCode:      &exitCode,
				PortBindings: []apicontainer.PortBinding{{
					ContainerPort: aws.Uint16(1),
					HostPort:      2,
					BindIP:        "1.2.3.4",
					Protocol:      3,
				}},
				ImageDigest: "image",
				Reason:      "reason",
				Container:   task.Containers[0],
			}

			event, err := newUncheckedContainerStateChangeEvent(task, task.Containers[0], "reason")
			if tc.containerType == apicontainer.ContainerNormal {
				assert.NoError(t, err)
				assert.Equal(t, expectedEvent, event)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestNewUncheckedContainerStateChangeEvent_SCBridge(t *testing.T) {
	testContainerName := "c1"
	tests := []struct {
		name                       string
		addPauseContainer          bool
		pauseContainerName         string
		pauseContainerPortBindings []apicontainer.PortBinding
		err                        error
	}{
		{
			name:                       "should fail to resolve pause container - pause container name doesn't match",
			addPauseContainer:          true,
			pauseContainerName:         "invalid-pause-container-name",
			pauseContainerPortBindings: []apicontainer.PortBinding{{}},
			err:                        fmt.Errorf("error resolving pause container for bridge mode SC container: %s", testContainerName),
		},
		{
			name:               "should use pause container port mapping",
			addPauseContainer:  true,
			pauseContainerName: fmt.Sprintf("%s-%s", apitask.NetworkPauseContainerName, testContainerName),
			pauseContainerPortBindings: []apicontainer.PortBinding{{
				ContainerPort: aws.Uint16(1),
				HostPort:      2,
				BindIP:        "1.2.3.4",
				Protocol:      3,
			}},
			err: nil,
		},
		{
			name:              "should fail to resolve pause container - no pause container available",
			addPauseContainer: false,
			err:               fmt.Errorf("error resolving pause container for bridge mode SC container: %s", testContainerName),
		},
	}
	steadyStateStatus := apicontainerstatus.ContainerRunning
	exitCode := 1
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := &apitask.Task{
				Arn:         "arn:123",
				NetworkMode: apitask.BridgeNetworkMode,
				ServiceConnectConfig: &serviceconnect.Config{
					ContainerName: "service-connect",
				},
				Containers: []*apicontainer.Container{
					{
						Name:                    testContainerName,
						RuntimeID:               "222",
						KnownStatusUnsafe:       apicontainerstatus.ContainerRunning,
						SentStatusUnsafe:        apicontainerstatus.ContainerStatusNone,
						Type:                    apicontainer.ContainerNormal,
						SteadyStateStatusUnsafe: &steadyStateStatus,
						KnownExitCodeUnsafe:     &exitCode,
						KnownPortBindingsUnsafe: []apicontainer.PortBinding{{
							ContainerPort: aws.Uint16(8080), // we get this from task definition
						}},
						ImageDigest: "image",
					},
					{
						Name: "service-connect",
					},
				}}
			if tc.addPauseContainer {
				task.Containers = append(task.Containers, &apicontainer.Container{
					Name:                    tc.pauseContainerName,
					Type:                    apicontainer.ContainerCNIPause,
					KnownPortBindingsUnsafe: tc.pauseContainerPortBindings,
				})
			}

			expectedEvent := ContainerStateChange{
				TaskArn:       "arn:123",
				ContainerName: testContainerName,
				RuntimeID:     "222",
				Status:        apicontainerstatus.ContainerRunning,
				ExitCode:      &exitCode,
				PortBindings:  tc.pauseContainerPortBindings,
				ImageDigest:   "image",
				Reason:        "reason",
				Container:     task.Containers[0],
			}

			event, err := newUncheckedContainerStateChangeEvent(task, task.Containers[0], "reason")
			if tc.err == nil {
				assert.NoError(t, err)
				assert.Equal(t, expectedEvent, event)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tc.err, err)
			}
		})
	}
}

func TestNewManagedAgentChangeEvent(t *testing.T) {
	tests := []struct {
		name          string
		managedAgents []apicontainer.ManagedAgent
		expectError   bool
	}{
		{
			name:        "no managed agents",
			expectError: true,
		},
		{
			name:        "non-existent managed agent",
			expectError: true,
			managedAgents: []apicontainer.ManagedAgent{{
				Name: "nonExistentAgent",
				ManagedAgentState: apicontainer.ManagedAgentState{
					Status: apicontainerstatus.ManagedAgentStatusNone,
					Reason: "reason",
				}}},
		},
		{
			name: "with managed agents that should NOT be reported",
			managedAgents: []apicontainer.ManagedAgent{{
				Name: "dummyAgent",
				ManagedAgentState: apicontainer.ManagedAgentState{
					Status: apicontainerstatus.ManagedAgentStatusNone,
					Reason: "reason",
				}}},
			expectError: true,
		},
		{
			name: "with managed agents that should be reported",
			managedAgents: []apicontainer.ManagedAgent{{
				Name: execcmd.ExecuteCommandAgentName,
				ManagedAgentState: apicontainer.ManagedAgentState{
					Status: apicontainerstatus.ManagedAgentRunning,
					Reason: "reason",
				}}},
			expectError: false,
		},
	}
	for _, tc := range tests {
		testContainer := apicontainer.Container{
			Name:                "c1",
			ManagedAgentsUnsafe: tc.managedAgents,
		}
		testContainers := []*apicontainer.Container{&testContainer}
		t.Run(tc.name, func(t *testing.T) {

			task := &apitask.Task{
				Arn:        "arn:123",
				Containers: testContainers,
			}
			expectedEvent := ManagedAgentStateChange{
				TaskArn:   "arn:123",
				Name:      execcmd.ExecuteCommandAgentName,
				Container: &testContainer,
				Status:    apicontainerstatus.ManagedAgentRunning,
				Reason:    "test",
			}

			event, err := NewManagedAgentChangeEvent(task, task.Containers[0], execcmd.ExecuteCommandAgentName, "test")
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, expectedEvent, event)
			}
		})
	}
}
