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

package types

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/stretchr/testify/assert"
)

// Helper function for testing taskProtectionTypeFromString
func TestTaskProtectionTypeFromString(t *testing.T) {
	testcases := []struct {
		name     string
		input    string
		expected taskProtectionType
		err      *string
	}{
		{
			name:     "Scale-in upper",
			input:    "SCALE_IN",
			expected: TaskProtectionTypeScaleIn,
		},
		{
			name:     "Scale-in lower",
			input:    "scale_in",
			expected: TaskProtectionTypeScaleIn,
		},
		{
			name:     "Disabled upper",
			input:    "DISABLED",
			expected: TaskProtectionTypeDisabled,
		},
		{
			name:     "Disabled lower",
			input:    "disabled",
			expected: TaskProtectionTypeDisabled,
		},
		{
			name:     "Invalid input",
			input:    "invalid",
			expected: "",
			err:      utils.Strptr("unknown task protection type: invalid"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := taskProtectionTypeFromString(tc.input)
			if tc.err != nil {
				assert.EqualError(t, err, *tc.err)
			}
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestNewTaskProtection(t *testing.T) {
	testcases := []struct {
		name              string
		protectionTypeStr string
		timeoutMinutes    *int
		expected          *taskProtection
		err               *string
	}{
		{
			name:              "Task protection type invalid",
			protectionTypeStr: "bad",
			timeoutMinutes:    nil,
			err:               utils.Strptr("protection type is invalid: unknown task protection type: bad"),
		},
		{
			name:              "Task protection timeout invalid",
			protectionTypeStr: string(TaskProtectionTypeScaleIn),
			timeoutMinutes:    utils.IntPtr(-3),
			err:               utils.Strptr("protection timeout must be greater than zero"),
		},
		{
			name:              "nil timeout happy",
			protectionTypeStr: string(TaskProtectionTypeScaleIn),
			expected:          &taskProtection{protectionType: TaskProtectionTypeScaleIn},
		},
		{
			name:              "non-nil timeout happy",
			protectionTypeStr: string(TaskProtectionTypeDisabled),
			timeoutMinutes:    utils.IntPtr(5),
			expected: &taskProtection{
				protectionType:           TaskProtectionTypeDisabled,
				protectionTimeoutMinutes: utils.IntPtr(5),
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := NewTaskProtection(tc.protectionTypeStr, tc.timeoutMinutes)
			if tc.err != nil {
				assert.EqualError(t, err, *tc.err)
			}
			assert.Equal(t, tc.expected, actual)
		})
	}
}
