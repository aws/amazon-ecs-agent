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
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
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
