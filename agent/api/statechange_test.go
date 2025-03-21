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
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	ecsmodel "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
							ContainerPort: 1,
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
					ContainerPort: 1,
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
				ContainerPort: 1,
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
							ContainerPort: 8080, // we get this from task definition
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

func TestGetNetworkBindings(t *testing.T) {
	testContainerStateChange := getTestContainerStateChange()
	expectedNetworkBindings := []*ecs.NetworkBinding{
		{
			BindIP:        aws.String("0.0.0.0"),
			ContainerPort: aws.Int64(10),
			HostPort:      aws.Int64(10),
			Protocol:      aws.String("tcp"),
		},
		{
			BindIP:        aws.String("1.2.3.4"),
			ContainerPort: aws.Int64(12),
			HostPort:      aws.Int64(12),
			Protocol:      aws.String("udp"),
		},
		{
			BindIP:        aws.String("5.6.7.8"),
			ContainerPort: aws.Int64(15),
			HostPort:      aws.Int64(20),
			Protocol:      aws.String("tcp"),
		},
		{
			BindIP:             aws.String("::"),
			ContainerPortRange: aws.String("21-22"),
			HostPortRange:      aws.String("60001-60002"),
			Protocol:           aws.String("udp"),
		},
		{
			BindIP:             aws.String("0.0.0.0"),
			ContainerPortRange: aws.String("96-97"),
			HostPortRange:      aws.String("47001-47002"),
			Protocol:           aws.String("tcp"),
		},
	}

	networkBindings := getNetworkBindings(testContainerStateChange)
	assert.ElementsMatch(t, expectedNetworkBindings, networkBindings)
}

func getTestContainerStateChange() ContainerStateChange {
	testContainer := &apicontainer.Container{
		Name:              "cont",
		NetworkModeUnsafe: "bridge",
		Ports: []apicontainer.PortBinding{
			{
				ContainerPort: 10,
				HostPort:      10,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
			{
				ContainerPort: 12,
				HostPort:      12,
				Protocol:      apicontainer.TransportProtocolUDP,
			},
			{
				ContainerPort: 15,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
			{
				ContainerPortRange: "21-22",
				Protocol:           apicontainer.TransportProtocolUDP,
			},
			{
				ContainerPortRange: "96-97",
				Protocol:           apicontainer.TransportProtocolTCP,
			},
		},
		ContainerHasPortRange: true,
		ContainerPortSet: map[int]struct{}{
			10: {},
			12: {},
			15: {},
		},
		ContainerPortRangeMap: map[string]string{
			"21-22": "60001-60002",
			"96-97": "47001-47002",
		},
	}

	testContainerStateChange := ContainerStateChange{
		TaskArn:       "arn",
		ContainerName: "cont",
		Status:        apicontainerstatus.ContainerRunning,
		Container:     testContainer,
		PortBindings: []apicontainer.PortBinding{
			{
				ContainerPort: 10,
				HostPort:      10,
				BindIP:        "0.0.0.0",
				Protocol:      apicontainer.TransportProtocolTCP,
			},
			{
				ContainerPort: 12,
				HostPort:      12,
				BindIP:        "1.2.3.4",
				Protocol:      apicontainer.TransportProtocolUDP,
			},
			{
				ContainerPort: 15,
				HostPort:      20,
				BindIP:        "5.6.7.8",
				Protocol:      apicontainer.TransportProtocolTCP,
			},
			{
				ContainerPort: 21,
				HostPort:      60001,
				BindIP:        "::",
				Protocol:      apicontainer.TransportProtocolUDP,
			},
			{
				ContainerPort: 22,
				HostPort:      60002,
				BindIP:        "::",
				Protocol:      apicontainer.TransportProtocolUDP,
			},
			{
				ContainerPort: 96,
				HostPort:      47001,
				BindIP:        "0.0.0.0",
				Protocol:      apicontainer.TransportProtocolTCP,
			},
			{
				ContainerPort: 97,
				HostPort:      47002,
				BindIP:        "0.0.0.0",
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}

	return testContainerStateChange
}

func TestNewTaskStateChangeEvent(t *testing.T) {
	tcs := []struct {
		name          string
		task          *apitask.Task
		reason        string
		expected      TaskStateChange
		expectedError string
	}{
		{
			name:          "internal tasks are never reported",
			task:          &apitask.Task{IsInternal: true, Arn: "arn"},
			expectedError: "should not send events for internal tasks or containers: arn",
		},
		{
			name: "manifest_pulled state is not reported if there are no resolved digests",
			task: &apitask.Task{
				Arn:               "arn",
				KnownStatusUnsafe: apitaskstatus.TaskManifestPulled,
				Containers: []*apicontainer.Container{
					{ImageDigest: ""},
					{ImageDigest: "digest", Type: apicontainer.ContainerCNIPause},
				},
			},
			expectedError: "should not send events for internal tasks or containers:" +
				" create task state change event api: status MANIFEST_PULLED not eligible" +
				" for backend reporting as no digests were resolved",
		},
		{
			name: "manifest_pulled state is reported",
			task: &apitask.Task{
				Arn:               "arn",
				KnownStatusUnsafe: apitaskstatus.TaskManifestPulled,
				Containers:        []*apicontainer.Container{{ImageDigest: "digest"}},
			},
			expected: TaskStateChange{TaskARN: "arn", Status: apitaskstatus.TaskManifestPulled},
		},
		{
			name:          "created state is not reported",
			task:          &apitask.Task{Arn: "arn", KnownStatusUnsafe: apitaskstatus.TaskCreated},
			expectedError: "create task state change event api: status not recognized by ECS: CREATED",
		},
		{
			name:     "running state is reported",
			task:     &apitask.Task{Arn: "arn", KnownStatusUnsafe: apitaskstatus.TaskRunning},
			expected: TaskStateChange{TaskARN: "arn", Status: apitaskstatus.TaskRunning},
		},
		{
			name:   "stopped state is reported",
			task:   &apitask.Task{Arn: "arn", KnownStatusUnsafe: apitaskstatus.TaskRunning},
			reason: "container stopped",
			expected: TaskStateChange{
				TaskARN: "arn", Status: apitaskstatus.TaskRunning, Reason: "container stopped",
			},
		},
		{
			name: "already sent status is not reported again",
			task: &apitask.Task{
				Arn:               "arn",
				KnownStatusUnsafe: apitaskstatus.TaskManifestPulled,
				SentStatusUnsafe:  apitaskstatus.TaskManifestPulled,
			},
			expectedError: "create task state change event api: status [MANIFEST_PULLED] already sent",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			res, err := NewTaskStateChangeEvent(tc.task, tc.reason)
			if tc.expectedError == "" {
				require.NoError(t, err)
				tc.expected.Task = tc.task
				assert.Equal(t, tc.expected, res)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}

func TestNewContainerStateChangeEvent(t *testing.T) {
	tcs := []struct {
		name          string
		task          *apitask.Task
		reason        string
		expected      ContainerStateChange
		expectedError string
	}{
		{
			name: "internal containers are not reported",
			task: &apitask.Task{
				Arn: "arn",
				Containers: []*apicontainer.Container{
					{Name: "container", Type: apicontainer.ContainerCNIPause},
				},
			},
			expectedError: "should not send events for internal tasks or containers: container",
		},
		{
			name: "MANIFEST_PULLED state is reported if digest was resolved",
			task: &apitask.Task{
				Arn: "arn",
				Containers: []*apicontainer.Container{
					{
						Name:              "container",
						ImageDigest:       "digest",
						KnownStatusUnsafe: apicontainerstatus.ContainerManifestPulled,
					},
				},
			},
			expected: ContainerStateChange{
				TaskArn:       "arn",
				ContainerName: "container",
				Status:        apicontainerstatus.ContainerManifestPulled,
				ImageDigest:   "digest",
			},
		},
		{
			name: "MANIFEST_PULLED state not is not reported if digest was not resolved",
			task: &apitask.Task{
				Arn: "arn",
				Containers: []*apicontainer.Container{
					{
						Name:              "container",
						ImageDigest:       "",
						KnownStatusUnsafe: apicontainerstatus.ContainerManifestPulled,
					},
				},
			},
			expectedError: "should not send events for internal tasks or containers:" +
				" create container state change event api:" +
				" no need to send MANIFEST_PULLED event" +
				" as no resolved digests were found",
		},
		{
			name: "PULLED state is not reported",
			task: &apitask.Task{
				Arn: "arn",
				Containers: []*apicontainer.Container{
					{
						Name:              "container",
						ImageDigest:       "digest",
						KnownStatusUnsafe: apicontainerstatus.ContainerPulled,
					},
				},
			},
			expectedError: "should not send events for internal tasks or containers:" +
				" create container state change event api: " +
				"status not recognized by ECS: PULLED",
		},
		{
			name: "RUNNING state is reported",
			task: &apitask.Task{
				Arn: "arn",
				Containers: []*apicontainer.Container{
					{
						Name:              "container",
						ImageDigest:       "digest",
						KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
					},
				},
			},
			expected: ContainerStateChange{
				TaskArn:       "arn",
				ContainerName: "container",
				Status:        apicontainerstatus.ContainerRunning,
				ImageDigest:   "digest",
			},
		},
		{
			name: "STOPPED state is reported",
			task: &apitask.Task{
				Arn: "arn",
				Containers: []*apicontainer.Container{
					{
						Name:              "container",
						ImageDigest:       "digest",
						KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
					},
				},
			},
			reason: "container stopped",
			expected: ContainerStateChange{
				TaskArn:       "arn",
				ContainerName: "container",
				Status:        apicontainerstatus.ContainerStopped,
				ImageDigest:   "digest",
				Reason:        "container stopped",
			},
		},
		{
			name: "already sent state is not reported again",
			task: &apitask.Task{
				Arn: "arn",
				Containers: []*apicontainer.Container{
					{
						Name:              "container",
						ImageDigest:       "digest",
						KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
						SentStatusUnsafe:  apicontainerstatus.ContainerRunning,
					},
				},
			},
			expectedError: "should not send events for internal tasks or containers:" +
				" create container state change event api:" +
				" status [RUNNING] already sent for container container, task arn",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			res, err := NewContainerStateChangeEvent(tc.task, tc.task.Containers[0], tc.reason)
			if tc.expectedError == "" {
				require.NoError(t, err)
				tc.expected.Container = tc.task.Containers[0]
				assert.Equal(t, tc.expected, res)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}

func TestContainerStatusChangeStatus(t *testing.T) {
	// Mapped status is ContainerStatusNone when container status is ContainerStatusNone
	var containerStatus apicontainerstatus.ContainerStatus
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerRunning),
		apicontainerstatus.ContainerStatusNone)
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerResourcesProvisioned),
		apicontainerstatus.ContainerStatusNone)

	// Mapped status is ContainerManifestPulled when container status is ContainerManifestPulled
	containerStatus = apicontainerstatus.ContainerManifestPulled
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerRunning),
		apicontainerstatus.ContainerManifestPulled)
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerResourcesProvisioned),
		apicontainerstatus.ContainerManifestPulled)

	// Mapped status is ContainerStatusNone when container status is ContainerPulled
	containerStatus = apicontainerstatus.ContainerPulled
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerRunning),
		apicontainerstatus.ContainerStatusNone)
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerResourcesProvisioned),
		apicontainerstatus.ContainerStatusNone)

	// Mapped status is ContainerStatusNone when container status is ContainerCreated
	containerStatus = apicontainerstatus.ContainerCreated
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerRunning),
		apicontainerstatus.ContainerStatusNone)
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerResourcesProvisioned),
		apicontainerstatus.ContainerStatusNone)

	containerStatus = apicontainerstatus.ContainerRunning
	// Mapped status is ContainerRunning when container status is ContainerRunning
	// and steady state is ContainerRunning
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerRunning),
		apicontainerstatus.ContainerRunning)
	// Mapped status is ContainerStatusNone when container status is ContainerRunning
	// and steady state is ContainerResourcesProvisioned
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerResourcesProvisioned),
		apicontainerstatus.ContainerStatusNone)

	containerStatus = apicontainerstatus.ContainerResourcesProvisioned
	// Mapped status is ContainerRunning when container status is ContainerResourcesProvisioned
	// and steady state is ContainerResourcesProvisioned
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerResourcesProvisioned),
		apicontainerstatus.ContainerRunning)

	// Mapped status is ContainerStopped when container status is ContainerStopped
	containerStatus = apicontainerstatus.ContainerStopped
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerRunning),
		apicontainerstatus.ContainerStopped)
	assert.Equal(t,
		containerStatusChangeStatus(containerStatus, apicontainerstatus.ContainerResourcesProvisioned),
		apicontainerstatus.ContainerStopped)
}

func TestBuildContainerStateChangePayload(t *testing.T) {
	tcs := []struct {
		name          string
		change        ContainerStateChange
		expected      *ecsmodel.ContainerStateChange
		expectedError string
	}{
		{
			name:          "fails when no container name",
			change:        ContainerStateChange{},
			expectedError: "container state change has no container name",
		},
		{
			name: "no result no error when container state is unsupported",
			change: ContainerStateChange{
				ContainerName: "container",
				Status:        apicontainerstatus.ContainerStatusNone,
			},
			expected: nil,
		},
		{
			name: "MANIFEST_PULLED state maps to PENDING",
			change: ContainerStateChange{
				ContainerName: "container",
				Container:     &apicontainer.Container{},
				Status:        apicontainerstatus.ContainerManifestPulled,
				ImageDigest:   "digest",
			},
			expected: &ecsmodel.ContainerStateChange{
				ContainerName:   aws.String("container"),
				ImageDigest:     aws.String("digest"),
				NetworkBindings: []*ecs.NetworkBinding{},
				Status:          aws.String("PENDING"),
			},
		},
		{
			name: "RUNNING maps to RUNNING",
			change: ContainerStateChange{
				ContainerName: "container",
				Container:     &apicontainer.Container{},
				Status:        apicontainerstatus.ContainerRunning,
				ImageDigest:   "digest",
				RuntimeID:     "runtimeid",
			},
			expected: &ecsmodel.ContainerStateChange{
				ContainerName:   aws.String("container"),
				ImageDigest:     aws.String("digest"),
				RuntimeId:       aws.String("runtimeid"),
				NetworkBindings: []*ecs.NetworkBinding{},
				Status:          aws.String("RUNNING"),
			},
		},
		{
			name: "STOPPED maps to STOPPED",
			change: ContainerStateChange{
				ContainerName: "container",
				Container:     &apicontainer.Container{},
				Status:        apicontainerstatus.ContainerStopped,
				ImageDigest:   "digest",
				RuntimeID:     "runtimeid",
				ExitCode:      aws.Int(1),
			},
			expected: &ecsmodel.ContainerStateChange{
				ContainerName:   aws.String("container"),
				ImageDigest:     aws.String("digest"),
				RuntimeId:       aws.String("runtimeid"),
				ExitCode:        aws.Int64(1),
				NetworkBindings: []*ecs.NetworkBinding{},
				Status:          aws.String("STOPPED"),
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			res, err := buildContainerStateChangePayload(tc.change)
			if tc.expectedError == "" {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, res)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}
