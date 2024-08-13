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

package testutils

import (
	"fmt"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/stretchr/testify/assert"
)

func TestTaskEqual(t *testing.T) {

	testCases := []struct {
		rhs           apitask.Task
		lhs           apitask.Task
		shouldBeEqual bool
	}{
		// Equal Pairs
		{apitask.Task{Arn: "a"}, apitask.Task{Arn: "a"}, true},
		{apitask.Task{Family: "a"}, apitask.Task{Family: "a"}, true},
		{apitask.Task{Version: "a"}, apitask.Task{Version: "a"}, true},
		{apitask.Task{Containers: []*apicontainer.Container{{Name: "a"}}}, apitask.Task{Containers: []*apicontainer.Container{{Name: "a"}}}, true},
		{apitask.Task{DesiredStatusUnsafe: apitaskstatus.TaskRunning}, apitask.Task{DesiredStatusUnsafe: apitaskstatus.TaskRunning}, true},
		{apitask.Task{KnownStatusUnsafe: apitaskstatus.TaskRunning}, apitask.Task{KnownStatusUnsafe: apitaskstatus.TaskRunning}, true},

		// Unequal Pairs
		{apitask.Task{Arn: "a"}, apitask.Task{Arn: "あ"}, false},
		{apitask.Task{Family: "a"}, apitask.Task{Family: "あ"}, false},
		{apitask.Task{Version: "a"}, apitask.Task{Version: "あ"}, false},
		{apitask.Task{Containers: []*apicontainer.Container{{Name: "a"}}}, apitask.Task{Containers: []*apicontainer.Container{{Name: "あ"}}}, false},
		{apitask.Task{DesiredStatusUnsafe: apitaskstatus.TaskRunning}, apitask.Task{DesiredStatusUnsafe: apitaskstatus.TaskStopped}, false},
		{apitask.Task{KnownStatusUnsafe: apitaskstatus.TaskRunning}, apitask.Task{KnownStatusUnsafe: apitaskstatus.TaskStopped}, false},
	}

	for index, tc := range testCases {
		t.Run(fmt.Sprintf("index %d expected %t", index, tc.shouldBeEqual), func(t *testing.T) {
			assert.Equal(t, TasksEqual(&tc.lhs, &tc.rhs), tc.shouldBeEqual, "TasksEqual not working as expected. Check index failure.")
			// Symmetric
			assert.Equal(t, TasksEqual(&tc.rhs, &tc.lhs), tc.shouldBeEqual, "Symmetric equality check failed. Check index failure.")
		})
	}
}
