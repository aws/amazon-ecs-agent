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

package status

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testTaskStatus struct {
	SomeStatus TaskStatus `json:"status"`
}

func TestUnmarshalTaskStatusSimple(t *testing.T) {
	tcs := []struct {
		statusString string
		expected     TaskStatus
	}{
		{"NONE", TaskStatusNone},
		{"MANIFEST_PULLED", TaskManifestPulled},
		{"CREATED", TaskCreated},
		{"RUNNING", TaskRunning},
		{"STOPPED", TaskStopped},
	}
	for _, tc := range tcs {
		t.Run(tc.statusString, func(t *testing.T) {
			status := TaskStatusNone
			err := json.Unmarshal([]byte(fmt.Sprintf("%q", tc.statusString)), &status)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, status)
		})
	}
}

func TestMarshaTaskStatusPointer(t *testing.T) {
	tcs := []struct {
		status   TaskStatus
		expected string
	}{
		{status: TaskStatusNone, expected: `"NONE"`},
		{status: TaskManifestPulled, expected: `"MANIFEST_PULLED"`},
		{status: TaskCreated, expected: `"CREATED"`},
		{status: TaskRunning, expected: `"RUNNING"`},
		{status: TaskStopped, expected: `"STOPPED"`},
	}
	for _, tc := range tcs {
		t.Run(tc.expected, func(t *testing.T) {
			marshled, err := json.Marshal(&tc.status)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, string(marshled))
		})
	}
}

func TestUnmarshalTaskStatusNested(t *testing.T) {
	var test testTaskStatus
	err := json.Unmarshal([]byte(`{"status":"STOPPED"}`), &test)
	if err != nil {
		t.Error(err)
	}
	if test.SomeStatus != TaskStopped {
		t.Error("STOPPED should unmarshal to STOPPED, not " + test.SomeStatus.String())
	}
}

// Tests that a pointer to a struct that has a TaskStatus is marshaled correctly.
func TestMarshalTaskStatusNestedPointer(t *testing.T) {
	tcs := []struct {
		status   TaskStatus
		expected string
	}{
		{status: TaskStatusNone, expected: "NONE"},
		{status: TaskManifestPulled, expected: "MANIFEST_PULLED"},
		{status: TaskCreated, expected: "CREATED"},
		{status: TaskRunning, expected: "RUNNING"},
		{status: TaskStopped, expected: "STOPPED"},
	}
	for _, tc := range tcs {
		t.Run(tc.status.String(), func(t *testing.T) {
			marshaled, err := json.Marshal(&testTaskStatus{tc.status})
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf(`{"status":%q}`, tc.expected), string(marshaled))
		})
	}
}
