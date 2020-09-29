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
	"errors"
	"strings"
)

const (
	// ManagedAgentStatusNone is the zero state of a managed agent
	ManagedAgentStatusNone ManagedAgentStatus = iota
	// ManagedAgentCreated represents a managed agent which has dependencies in place
	ManagedAgentCreated
	// ManagedAgentRunning represents a managed agent that has started sucessfully
	ManagedAgentRunning
	// ManagedAgentStopped represents a managed agent that has stopped
	ManagedAgentStopped
)

// ManagedAgentStatus is an enumeration of valid states in the managed agent lifecycle
type ManagedAgentStatus int32

var managedAgentStatusMap = map[string]ManagedAgentStatus{
	"NONE":    ManagedAgentStatusNone,
	"CREATED": ManagedAgentCreated,
	"RUNNING": ManagedAgentRunning,
	"STOPPED": ManagedAgentStopped,
}

// String returns a human readable string representation of this object
func (mas ManagedAgentStatus) String() string {
	for k, v := range managedAgentStatusMap {
		if v == mas {
			return k
		}
	}
	return "NONE"
}

// Terminal returns true if the managed agent status is STOPPED
func (mas ManagedAgentStatus) Terminal() bool {
	return mas == ManagedAgentStopped
}

// IsRunning returns true if the managed agent status is either RUNNING
func (mas ManagedAgentStatus) IsRunning() bool {
	return mas == ManagedAgentRunning
}

// BackendStatus maps the internal managedAgent status in the agent to that in the backend
// This exists to force the status to be one of PENDING|RUNNING}|STOPPED
func (mas ManagedAgentStatus) BackendStatus() string {
	switch mas {
	case ManagedAgentRunning:
		return mas.String()
	case ManagedAgentStopped:
		return mas.String()
	}
	return "PENDING"
}

// ShouldReportToBackend returns true if the managedAgent status is recognized as a
// valid state by ECS. Note that not all managedAgent statuses are recognized by ECS
// or map to ECS states
// For now we expect PENDING/RUNNING/STOPPED as valid states
// we should only skip events that have ManagedAgentStatusNone
func (mas ManagedAgentStatus) ShouldReportToBackend() bool {
	return mas == ManagedAgentCreated || mas == ManagedAgentRunning || mas == ManagedAgentStopped
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ManagedAgentStatus data
func (mas *ManagedAgentStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*mas = ManagedAgentStatusNone
		return nil
	}
	if b[0] != '"' || b[len(b)-1] != '"' {
		*mas = ManagedAgentStatusNone
		return errors.New("managed agent status unmarshal: status must be a string or null; Got " + string(b))
	}

	stat, ok := managedAgentStatusMap[string(b[1:len(b)-1])]
	if !ok {
		*mas = ManagedAgentStatusNone
		return errors.New("managed agent status unmarshal: unrecognized status")
	}
	*mas = stat
	return nil
}

// MarshalJSON overrides the logic for JSON-encoding the ManagedAgentStatus type
func (mas *ManagedAgentStatus) MarshalJSON() ([]byte, error) {
	if mas == nil {
		return nil, nil
	}
	return []byte(`"` + mas.String() + `"`), nil
}
