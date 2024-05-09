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

func TestShouldReportToBackend(t *testing.T) {
	// ContainerStatusNone is not reported to backend
	var containerStatus ContainerStatus
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerRunning))
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerResourcesProvisioned))

	// ContainerManifestPulled is not reported to backend
	containerStatus = ContainerManifestPulled
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerRunning))
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerResourcesProvisioned))

	// ContainerPulled is not reported to backend
	containerStatus = ContainerPulled
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerRunning))
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerResourcesProvisioned))

	// ContainerCreated is not reported to backend
	containerStatus = ContainerCreated
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerRunning))
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerResourcesProvisioned))

	containerStatus = ContainerRunning
	// ContainerRunning is reported to backend if the steady state is RUNNING as well
	assert.True(t, containerStatus.ShouldReportToBackend(ContainerRunning))
	// ContainerRunning is not reported to backend if the steady state is RESOURCES_PROVISIONED
	assert.False(t, containerStatus.ShouldReportToBackend(ContainerResourcesProvisioned))

	containerStatus = ContainerResourcesProvisioned
	// ContainerResourcesProvisioned is reported to backend if the steady state
	// is RESOURCES_PROVISIONED
	assert.True(t, containerStatus.ShouldReportToBackend(ContainerResourcesProvisioned))

	// ContainerStopped is not reported to backend
	containerStatus = ContainerStopped
	assert.True(t, containerStatus.ShouldReportToBackend(ContainerRunning))
	assert.True(t, containerStatus.ShouldReportToBackend(ContainerResourcesProvisioned))

}

type testContainerStatus struct {
	SomeStatus ContainerStatus `json:"status"`
}

// Tests that pointers to a struct that contains ContainerStatus are marshaled correctly.
func TestMarshalContainerStatusNestedPointer(t *testing.T) {
	tcs := []struct {
		status   ContainerStatus
		expected string
	}{
		{status: ContainerStatusNone, expected: "NONE"},
		{status: ContainerManifestPulled, expected: "MANIFEST_PULLED"},
		{status: ContainerPulled, expected: "PULLED"},
		{status: ContainerCreated, expected: "CREATED"},
		{status: ContainerRunning, expected: "RUNNING"},
		{status: ContainerResourcesProvisioned, expected: "RESOURCES_PROVISIONED"},
		{status: ContainerStopped, expected: "STOPPED"},
	}
	for _, tc := range tcs {
		marshaled, err := json.Marshal(&testContainerStatus{SomeStatus: tc.status})
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf(`{"status":%q}`, tc.status), string(marshaled))
	}
}

// Tests that ContainerStatus is unmarshaled correctly.
func TestUnmarshalContainerStatusSimple(t *testing.T) {
	tcs := []struct {
		statusString string
		expected     ContainerStatus
	}{
		{statusString: "NONE", expected: ContainerStatusNone},
		{statusString: "MANIFEST_PULLED", expected: ContainerManifestPulled},
		{statusString: "PULLED", expected: ContainerPulled},
		{statusString: "CREATED", expected: ContainerCreated},
		{statusString: "RUNNING", expected: ContainerRunning},
		{statusString: "RESOURCES_PROVISIONED", expected: ContainerResourcesProvisioned},
		{statusString: "STOPPED", expected: ContainerStopped},
	}
	for _, tc := range tcs {
		t.Run(tc.statusString, func(t *testing.T) {
			status := ContainerStatusNone
			err := json.Unmarshal([]byte(fmt.Sprintf("%q", tc.statusString)), &status)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, status)
		})
	}
}

// Tests that structs that have a ContainerStatus field are unmarshaled correctly.
func TestUnmarshalContainerStatusNested(t *testing.T) {
	var test testContainerStatus
	err := json.Unmarshal([]byte(`{"status":"STOPPED"}`), &test)
	if err != nil {
		t.Error(err)
	}
	if test.SomeStatus != ContainerStopped {
		t.Error("STOPPED should unmarshal to STOPPED, not " + test.SomeStatus.String())
	}
}

func TestHealthBackendStatus(t *testing.T) {
	testCases := []struct {
		status ContainerHealthStatus
		result string
	}{
		{
			status: ContainerHealthy,
			result: "HEALTHY",
		},
		{
			status: ContainerUnhealthy,
			result: "UNHEALTHY",
		}, {
			status: ContainerHealthUnknown,
			result: "UNKNOWN",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Status: %s BackendStatus: %s", tc.status, tc.result), func(t *testing.T) {
			assert.Equal(t, tc.status.BackendStatus(), tc.result)
		})
	}
}

func TestUnmarshalContainerHealthStatus(t *testing.T) {
	testCases := []struct {
		Status ContainerHealthStatus
		String string
	}{
		{
			Status: ContainerHealthUnknown,
			String: `"UNKNOWN"`,
		},
		{
			Status: ContainerHealthy,
			String: `"HEALTHY"`,
		},
		{
			Status: ContainerUnhealthy,
			String: `"UNHEALTHY"`,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Status: %s, String value: %s", tc.Status, tc.String), func(t *testing.T) {
			var status ContainerHealthStatus
			err := json.Unmarshal([]byte(tc.String), &status)
			assert.NoError(t, err)
			assert.Equal(t, status, tc.Status)
		})
	}
}

// Tests that all container statuses are marshaled to JSON with a quoted string.
// Also tests that JSON marshaled container status can be unmarshaled.
func TestContainerStatusMarshalUnmarshalJSON(t *testing.T) {
	for strStatus, status := range containerStatusMap {
		t.Run(fmt.Sprintf("marshal-unmarshal %v", strStatus), func(t *testing.T) {
			marshaled, err := json.Marshal(status)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("%q", strStatus), string(marshaled))

			var unmarshaled ContainerStatus
			err = json.Unmarshal(marshaled, &unmarshaled)
			require.NoError(t, err)
			require.Equal(t, status, unmarshaled)
		})
	}
}

// Tests that a container status marshaled as text can be unmarshaled.
func TestContainerStatusMarshalUnmarshalText(t *testing.T) {
	for strStatus, status := range containerStatusMap {
		t.Run(fmt.Sprintf("marshal-unmarshal %v", strStatus), func(t *testing.T) {
			marshaled, err := status.MarshalText()
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("%s", strStatus), string(marshaled))

			var unmarshaled ContainerStatus
			err = unmarshaled.UnmarshalText(marshaled)
			require.NoError(t, err)
			require.Equal(t, status, unmarshaled)
		})
	}
}

// Tests that MarshalText works as expected for container status pointers.
func TestContainerStatusMarshalPointer(t *testing.T) {
	status := ContainerPulled
	ptr := &status
	marshaled, err := ptr.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "PULLED", string(marshaled))
}

// Tests that unmarshaling an invalid text to container status fails.
func TestContainerStatusTextUnmarshalError(t *testing.T) {
	var status ContainerStatus
	assert.EqualError(t, status.UnmarshalText([]byte("invalidStatus")),
		"container status text unmarshal: unrecognized status: invalidStatus")
}

// Tests that string based statuses are used when a map with container status as keys is
// marshaled to JSON.
func TestContainerStatusKeyMarshal(t *testing.T) {
	someMap := map[ContainerStatus]string{
		ContainerStatusNone:           "",
		ContainerPulled:               "",
		ContainerCreated:              "",
		ContainerRunning:              "",
		ContainerResourcesProvisioned: "",
		ContainerStopped:              "",
	}
	marshaled, err := json.Marshal(someMap)
	require.NoError(t, err)

	var unmarshaledMap map[string]string
	err = json.Unmarshal(marshaled, &unmarshaledMap)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		`NONE`:                  "",
		`PULLED`:                "",
		`CREATED`:               "",
		`RUNNING`:               "",
		`RESOURCES_PROVISIONED`: "",
		`STOPPED`:               "",
	}, unmarshaledMap)

	var unmarshaled map[ContainerStatus]string
	err = json.Unmarshal(marshaled, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, someMap, unmarshaled)
}

// Tests that JSON unmarshal of container status is backwards-compatible with legacy integer
// based representations for JSON object keys.
func TestContainerStatusJSONUnmarshalInt(t *testing.T) {
	tcs := map[string]ContainerStatus{
		`"0"`: ContainerStatusNone,
		`"1"`: ContainerPulled,
		`"2"`: ContainerCreated,
		`"3"`: ContainerRunning,
		`"4"`: ContainerResourcesProvisioned,
		`"5"`: ContainerStopped,
		`"6"`: ContainerZombie,
	}
	for intStatus, status := range tcs {
		t.Run(fmt.Sprintf("%s - %s", intStatus, status.String()), func(t *testing.T) {
			var unmarshaled ContainerStatus
			err := json.Unmarshal([]byte(intStatus), &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, status, unmarshaled)
		})
	}
}

// Tests for BackendStatusString method when a steady state is not provided.
func TestContainerBackendStatusStringDefaultSteadyState(t *testing.T) {
	tcs := []struct {
		status   ContainerStatus
		expected string
	}{
		{ContainerStatusNone, "PENDING"},
		{ContainerManifestPulled, "PENDING"},
		{ContainerPulled, "PENDING"},
		{ContainerCreated, "PENDING"},
		{ContainerRunning, "RUNNING"},
		{ContainerResourcesProvisioned, "PENDING"},
		{ContainerStopped, "STOPPED"},
		{ContainerZombie, "PENDING"},
	}
	for _, tc := range tcs {
		t.Run(fmt.Sprintf("%d", tc.status), func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.status.BackendStatusString())
		})
	}
}
