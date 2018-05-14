// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package container

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldReportToBackend(t *testing.T) {
	// ContainerStatusNone is not reported to backend
	var containerStatus ContainerStatus
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

func TestBackendStatus(t *testing.T) {
	// BackendStatus is ContainerStatusNone when container status is ContainerStatusNone
	var containerStatus ContainerStatus
	assert.Equal(t, containerStatus.BackendStatus(ContainerRunning), ContainerStatusNone)
	assert.Equal(t, containerStatus.BackendStatus(ContainerResourcesProvisioned), ContainerStatusNone)

	// BackendStatus is still ContainerStatusNone when container status is ContainerPulled
	containerStatus = ContainerPulled
	assert.Equal(t, containerStatus.BackendStatus(ContainerRunning), ContainerStatusNone)
	assert.Equal(t, containerStatus.BackendStatus(ContainerResourcesProvisioned), ContainerStatusNone)

	// BackendStatus is still ContainerStatusNone when container status is ContainerCreated
	containerStatus = ContainerCreated
	assert.Equal(t, containerStatus.BackendStatus(ContainerRunning), ContainerStatusNone)
	assert.Equal(t, containerStatus.BackendStatus(ContainerResourcesProvisioned), ContainerStatusNone)

	containerStatus = ContainerRunning
	// BackendStatus is ContainerRunning when container status is ContainerRunning
	// and steady state is ContainerRunning
	assert.Equal(t, containerStatus.BackendStatus(ContainerRunning), ContainerRunning)
	// BackendStatus is still ContainerStatusNone when container status is ContainerRunning
	// and steady state is ContainerResourcesProvisioned
	assert.Equal(t, containerStatus.BackendStatus(ContainerResourcesProvisioned), ContainerStatusNone)

	containerStatus = ContainerResourcesProvisioned
	// BackendStatus is still ContainerRunning when container status is ContainerResourcesProvisioned
	// and steady state is ContainerResourcesProvisioned
	assert.Equal(t, containerStatus.BackendStatus(ContainerResourcesProvisioned), ContainerRunning)

	// BackendStatus is ContainerStopped when container status is ContainerStopped
	containerStatus = ContainerStopped
	assert.Equal(t, containerStatus.BackendStatus(ContainerRunning), ContainerStopped)
	assert.Equal(t, containerStatus.BackendStatus(ContainerResourcesProvisioned), ContainerStopped)
}

type testContainerStatus struct {
	SomeStatus ContainerStatus `json:"status"`
}

func TestUnmarshalContainerStatus(t *testing.T) {
	status := ContainerStatusNone

	err := json.Unmarshal([]byte(`"RUNNING"`), &status)
	if err != nil {
		t.Error(err)
	}
	if status != ContainerRunning {
		t.Error("RUNNING should unmarshal to RUNNING, not " + status.String())
	}

	var test testContainerStatus
	err = json.Unmarshal([]byte(`{"status":"STOPPED"}`), &test)
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
