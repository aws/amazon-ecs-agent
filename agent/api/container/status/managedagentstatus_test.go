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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerminal(t *testing.T) {
	terminalStatus := ManagedAgentStopped
	runningStatus := ManagedAgentRunning
	assert.True(t, terminalStatus.Terminal())
	assert.False(t, runningStatus.Terminal())
}

func TestIsRunning(t *testing.T) {
	terminalStatus := ManagedAgentStopped
	runningStatus := ManagedAgentRunning
	assert.True(t, runningStatus.IsRunning())
	assert.False(t, terminalStatus.IsRunning())
}

func TestShouldReportManagedAgentStatusToBackend(t *testing.T) {
	// We should not report ManagedAgentStatusNone
	var managedAgentStatus ManagedAgentStatus
	assert.Equal(t, false, managedAgentStatus.ShouldReportToBackend())
	// We should report ManagedAgentCreated (ie PENDING)
	managedAgentStatus = ManagedAgentCreated
	assert.Equal(t, true, managedAgentStatus.ShouldReportToBackend())
	// We should report ManagedAgentRunning (ie RUNNING)
	managedAgentStatus = ManagedAgentRunning
	assert.Equal(t, true, managedAgentStatus.ShouldReportToBackend())
	// We should report ManagedAgentStooped (ie STOPPED)
	managedAgentStatus = ManagedAgentStopped
	assert.Equal(t, true, managedAgentStatus.ShouldReportToBackend())
}

func TestManagedAgentBackendStatus(t *testing.T) {
	// BackendStatus is "PENDING" when managedAgent status is ManagedAgentStatusNone
	var managedAgentStatus ManagedAgentStatus
	assert.Equal(t, "PENDING", managedAgentStatus.BackendStatus())

	managedAgentStatus = ManagedAgentCreated
	// BackendStatus is "PENDING" when managedAgent status is ManagedAgentCreated
	assert.Equal(t, "PENDING", managedAgentStatus.BackendStatus())

	managedAgentStatus = ManagedAgentRunning
	// BackendStatus is "RUNNING" when managedAgent status is ManagedAgentRunning
	assert.Equal(t, "RUNNING", managedAgentStatus.BackendStatus())

	// BackendStatus is "STOPPED" when managedAgent status is ManagedAgentStopped
	managedAgentStatus = ManagedAgentStopped
	assert.Equal(t, "STOPPED", managedAgentStatus.BackendStatus())
}

type testManagedAgentStatus struct {
	SomeStatus ManagedAgentStatus `json:"status"`
}

func TestUnmarshalManagedAgentStatus(t *testing.T) {
	status := ManagedAgentStatusNone

	err := json.Unmarshal([]byte(`"RUNNING"`), &status)
	if err != nil {
		t.Error(err)
	}
	if status != ManagedAgentRunning {
		t.Error("RUNNING should unmarshal to RUNNING, not " + status.String())
	}

	var test testManagedAgentStatus
	err = json.Unmarshal([]byte(`{"status":"STOPPED"}`), &test)
	if err != nil {
		t.Error(err)
	}
	if test.SomeStatus != ManagedAgentStopped {
		t.Error("STOPPED should unmarshal to STOPPED, not " + test.SomeStatus.String())
	}
}
