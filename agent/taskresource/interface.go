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

package taskresource

import (
	"encoding/json"
	"time"
)

// ResourceStatus is an enumeration of valid states of task resource lifecycle
type ResourceStatus int32

const (
	// ResourceStatusNone is the zero state of a task resource
	ResourceStatusNone ResourceStatus = iota
	// ResourceCreated represents state where task resource has been created
	ResourceCreated
	// ResourceRemoved represents state where task resource has been cleaned up
	ResourceRemoved
)

// TaskResource is a wrapper for task level resource methods we need
type TaskResource interface {
	// SetDesiredStatus sets the desired status of the resource
	SetDesiredStatus(ResourceStatus)
	// GetDesiredStatus gets the desired status of the resource
	GetDesiredStatus() ResourceStatus
	// SetKnownStatus sets the desired status of the resource
	SetKnownStatus(ResourceStatus)
	// GetKnownStatus gets the desired status of the resource
	GetKnownStatus() ResourceStatus
	// SetCreatedAt sets the timestamp for resource's creation time
	SetCreatedAt(time.Time)
	// GetCreatedAt sets the timestamp for resource's creation time
	GetCreatedAt() time.Time
	// Create performs resource creation
	Create() error
	// Cleanup performs resource cleanup
	Cleanup() error
	// GetName returns the unique name of the resource
	GetName() string
	// DesiredTeminal returns true if remove is in terminal state
	DesiredTerminal() bool
	// KnownCreated returns true if resource state is CREATED
	KnownCreated() bool
	// TerminalStatus returns the last transition state of the resource
	TerminalStatus() ResourceStatus
	// NextKnownState returns resource's next state
	NextKnownState() ResourceStatus
	// ApplyTransition calls the function required to move to the specified status
	ApplyTransition(ResourceStatus) error
	// SteadyState returns the transition state of the resource defined as "ready"
	SteadyState() ResourceStatus
	// SetAppliedStatus sets the applied status of resource and returns whether
	// the resource is already in a transition
	SetAppliedStatus(status ResourceStatus) bool
	// StatusString returns the string of the resource status
	StatusString(status ResourceStatus) string
	// Initialize will initialze the resource fields of the resource
	Initialize(*ResourceFields)

	json.Marshaler
	json.Unmarshaler
}
