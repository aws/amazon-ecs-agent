// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"testing"

	. "github.com/aws/amazon-ecs-agent/agent/api"
)

func TestTaskEqual(t *testing.T) {
	equalPairs := []Task{
		Task{Arn: "a"}, Task{Arn: "a"},
		Task{Family: "a"}, Task{Family: "a"},
		Task{Version: "a"}, Task{Version: "a"},
		Task{Containers: []*Container{&Container{Name: "a"}}}, Task{Containers: []*Container{&Container{Name: "a"}}},
		Task{DesiredStatus: TaskRunning}, Task{DesiredStatus: TaskRunning},
		Task{KnownStatus: TaskRunning}, Task{KnownStatus: TaskRunning},
	}

	unequalPairs := []Task{
		Task{Arn: "a"}, Task{Arn: "あ"},
		Task{Family: "a"}, Task{Family: "あ"},
		Task{Version: "a"}, Task{Version: "あ"},
		Task{Containers: []*Container{&Container{Name: "a"}}}, Task{Containers: []*Container{&Container{Name: "あ"}}},
		Task{DesiredStatus: TaskRunning}, Task{DesiredStatus: TaskStopped},
		Task{KnownStatus: TaskRunning}, Task{KnownStatus: TaskStopped},
	}

	for i := 0; i < len(equalPairs); i += 2 {
		if !TasksEqual(&equalPairs[i], &equalPairs[i+1]) {
			t.Error(i, equalPairs[i], " should equal ", equalPairs[i+1])
		}
		// Should be symetric
		if !TasksEqual(&equalPairs[i+1], &equalPairs[i]) {
			t.Error(i, "(symetric)", equalPairs[i+1], " should equal ", equalPairs[i])
		}
	}

	for i := 0; i < len(unequalPairs); i += 2 {
		if TasksEqual(&unequalPairs[i], &unequalPairs[i+1]) {
			t.Error(i, unequalPairs[i], " shouldn't equal ", unequalPairs[i+1])
		}
		//symetric
		if TasksEqual(&unequalPairs[i+1], &unequalPairs[i]) {
			t.Error(i, "(symetric)", unequalPairs[i+1], " shouldn't equal ", unequalPairs[i])
		}
	}
}
