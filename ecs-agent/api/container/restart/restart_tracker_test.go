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

package restart

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unmarshalStandardRestartPolicy allows us to verify that the restart policy unmarshalled
// from JSON (as it comes in the task payload) is translated to expected behavior in
// the restart tracker.
func unmarshalStandardRestartPolicy(t *testing.T, ignoredCode int) RestartPolicy {
	rp := RestartPolicy{}
	jsonBlob := fmt.Sprintf(`{"enabled": true, "restartAttemptPeriod": 60, "ignoredExitCodes": [%d]}`, ignoredCode)
	err := json.Unmarshal([]byte(jsonBlob), &rp)
	require.NoError(t, err)
	return rp
}

func TestShouldRestart(t *testing.T) {
	ignoredCode := 0
	rt := NewRestartTracker(RestartPolicy{
		Enabled:              false,
		IgnoredExitCodes:     []int{ignoredCode},
		RestartAttemptPeriod: 60,
	})
	testCases := []struct {
		name           string
		rp             RestartPolicy
		exitCode       int
		startedAt      time.Time
		desiredStatus  apicontainerstatus.ContainerStatus
		expected       bool
		expectedReason string
	}{
		{
			name: "restart policy disabled",
			rp: RestartPolicy{
				Enabled:              false,
				IgnoredExitCodes:     []int{ignoredCode},
				RestartAttemptPeriod: 60,
			},
			exitCode:       1,
			startedAt:      time.Now().Add(2 * time.Minute),
			desiredStatus:  apicontainerstatus.ContainerRunning,
			expected:       false,
			expectedReason: "restart policy is not enabled",
		},
		{
			name:           "ignored exit code",
			rp:             unmarshalStandardRestartPolicy(t, ignoredCode),
			exitCode:       0,
			startedAt:      time.Now().Add(-2 * time.Minute),
			desiredStatus:  apicontainerstatus.ContainerRunning,
			expected:       false,
			expectedReason: "exit code 0 should be ignored",
		},
		{
			name:           "non ignored exit code",
			rp:             unmarshalStandardRestartPolicy(t, ignoredCode),
			exitCode:       1,
			startedAt:      time.Now().Add(-2 * time.Minute),
			desiredStatus:  apicontainerstatus.ContainerRunning,
			expected:       true,
			expectedReason: "",
		},
		{
			name:           "nil exit code",
			rp:             unmarshalStandardRestartPolicy(t, ignoredCode),
			exitCode:       -1,
			startedAt:      time.Now().Add(-2 * time.Minute),
			desiredStatus:  apicontainerstatus.ContainerRunning,
			expected:       false,
			expectedReason: "exit code is nil",
		},
		{
			name:           "desired status stopped",
			rp:             unmarshalStandardRestartPolicy(t, ignoredCode),
			exitCode:       1,
			startedAt:      time.Now().Add(2 * time.Minute),
			desiredStatus:  apicontainerstatus.ContainerStopped,
			expected:       false,
			expectedReason: "container's desired status is stopped",
		},
		{
			name:           "attempt reset period not elapsed",
			rp:             unmarshalStandardRestartPolicy(t, ignoredCode),
			exitCode:       1,
			startedAt:      time.Now(),
			desiredStatus:  apicontainerstatus.ContainerRunning,
			expected:       false,
			expectedReason: "attempt reset period has not elapsed",
		},
		{
			name:           "attempt reset period not elapsed within one second",
			rp:             unmarshalStandardRestartPolicy(t, ignoredCode),
			exitCode:       1,
			startedAt:      time.Now().Add(-time.Second * 59),
			desiredStatus:  apicontainerstatus.ContainerRunning,
			expected:       false,
			expectedReason: "attempt reset period has not elapsed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rt.RestartPolicy = tc.rp

			// Because we cannot instantiate int pointers directly,
			// check for the exit code and leave this int pointer as nil
			// if there is no value to override it.
			var exitCodeAdjusted *int
			if tc.exitCode != -1 {
				exitCodeAdjusted = &tc.exitCode
			}

			shouldRestart, reason := rt.ShouldRestart(exitCodeAdjusted, tc.startedAt, tc.desiredStatus)
			assert.Equal(t, tc.expected, shouldRestart)
			assert.Equal(t, tc.expectedReason, reason)
		})
	}
}

func TestShouldRestartUsesLastRestart(t *testing.T) {
	rt := NewRestartTracker(RestartPolicy{
		Enabled:              true,
		IgnoredExitCodes:     []int{0},
		RestartAttemptPeriod: 60,
	})
	exitCode := 1

	shouldRestart, reason := rt.ShouldRestart(&exitCode, time.Now().Add(-61*time.Second), apicontainerstatus.ContainerRunning)
	assert.True(t, shouldRestart)

	// After restarting, we should inform restart decisions with LastRestartedAt instead of the passed in startedAt time.
	rt.RecordRestart()
	shouldRestart, reason = rt.ShouldRestart(&exitCode, time.Now().Add(-61*time.Second), apicontainerstatus.ContainerRunning)
	assert.False(t, shouldRestart)
	assert.Equal(t, "attempt reset period has not elapsed", reason)
}

func TestRecordRestart(t *testing.T) {
	rt := NewRestartTracker(RestartPolicy{
		Enabled:              false,
		RestartAttemptPeriod: 60,
	})
	assert.Equal(t, 0, rt.RestartCount)
	for i := 1; i < 1000; i++ {
		restartAt := time.Now()
		rt.RecordRestart()
		assert.Equal(t, i, rt.RestartCount)
		assert.Equal(t, restartAt.Round(time.Second), rt.GetLastRestartAt().Round(time.Second))
	}
}

func TestRecordRestartPolicy(t *testing.T) {
	rt := NewRestartTracker(RestartPolicy{
		Enabled:              false,
		RestartAttemptPeriod: 60,
	})
	assert.Equal(t, 0, rt.RestartCount)
	assert.Equal(t, 0, len(rt.RestartPolicy.IgnoredExitCodes))
	assert.NotNil(t, rt.RestartPolicy)
}
