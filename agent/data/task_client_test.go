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

package data

import (
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTaskArn1 = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc"
	testTaskArn2 = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/def"
)

func TestManageTask(t *testing.T) {
	testClient := newTestClient(t)

	testTask := &apitask.Task{
		Arn: testTaskArn1,
	}

	require.NoError(t, testClient.SaveTask(testTask))
	res, err := testClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, res, 1)

	require.NoError(t, testClient.DeleteTask("abc"))
	res, err = testClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, res, 0)
}

func TestSaveTaskInvalidID(t *testing.T) {
	testClient := newTestClient(t)

	testTask := &apitask.Task{
		Arn: "invalid-arn",
	}
	assert.Error(t, testClient.SaveTask(testTask))
}

func TestHasNonTerminalTasks(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []*apitask.Task
		expected bool
	}{
		{
			name:     "no tasks",
			tasks:    nil,
			expected: false,
		},
		{
			name: "all stopped",
			tasks: func() []*apitask.Task {
				task := &apitask.Task{Arn: testTaskArn1}
				task.SetKnownStatus(apitaskstatus.TaskStopped)
				return []*apitask.Task{task}
			}(),
			expected: false,
		},
		{
			name: "one running",
			tasks: func() []*apitask.Task {
				task := &apitask.Task{Arn: testTaskArn1}
				task.SetKnownStatus(apitaskstatus.TaskRunning)
				return []*apitask.Task{task}
			}(),
			expected: true,
		},
		{
			name: "mixed stopped and running",
			tasks: func() []*apitask.Task {
				stopped := &apitask.Task{Arn: testTaskArn1}
				stopped.SetKnownStatus(apitaskstatus.TaskStopped)
				running := &apitask.Task{Arn: testTaskArn2}
				running.SetKnownStatus(apitaskstatus.TaskRunning)
				return []*apitask.Task{stopped, running}
			}(),
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testClient := newTestClient(t)
			for _, task := range tc.tasks {
				require.NoError(t, testClient.SaveTask(task))
			}
			assert.Equal(t, tc.expected, testClient.HasNonTerminalTasks())
		})
	}
}
