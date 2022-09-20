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

func TestNewTaskProtection(t *testing.T) {
	testcases := []struct {
		name              string
		protectionEnabled bool
		expiresInMinutes  *int
		expected          *taskProtection
		err               *string
	}{
		{
			name:              "Task protection timeout invalid",
			protectionEnabled: true,
			expiresInMinutes:  utils.IntPtr(-3),
			err:               utils.Strptr("expiration duration must be greater than zero minutes for enabled task protection"),
		},
		{
			name:              "nil timeout allowed for enabled protection",
			protectionEnabled: true,
			expected:          &taskProtection{protectionEnabled: true},
		},
		{
			name:              "nil timeout allowed for disabled protection",
			protectionEnabled: false,
			expected:          &taskProtection{protectionEnabled: false},
		},
		{
			name:              "non-nil timeout happy",
			protectionEnabled: true,
			expiresInMinutes:  utils.IntPtr(3),
			expected: &taskProtection{
				protectionEnabled: true,
				expiresInMinutes:  utils.IntPtr(3),
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := NewTaskProtection(tc.protectionEnabled, tc.expiresInMinutes)
			if tc.err != nil {
				assert.EqualError(t, err, *tc.err)
			}
			assert.Equal(t, tc.expected, actual)
		})
	}
}
