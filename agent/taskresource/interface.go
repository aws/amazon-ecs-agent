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

package taskresource

import (
	"encoding/json"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
)

// TaskResource is a wrapper for task level resource methods we need
type TaskResource interface {
	// SetDesiredStatus sets the desired status of the resource
	SetDesiredStatus(resourcestatus.ResourceStatus)
	// GetDesiredStatus gets the desired status of the resource
	GetDesiredStatus() resourcestatus.ResourceStatus
	// SetKnownStatus sets the desired status of the resource
	SetKnownStatus(resourcestatus.ResourceStatus)
	// GetKnownStatus gets the desired status of the resource
	GetKnownStatus() resourcestatus.ResourceStatus
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
	// DesiredTerminal returns true if remove is in terminal state
	DesiredTerminal() bool
	// KnownCreated returns true if resource state is CREATED
	KnownCreated() bool
	// TerminalStatus returns the last transition state of the resource
	TerminalStatus() resourcestatus.ResourceStatus
	// NextKnownState returns resource's next state
	NextKnownState() resourcestatus.ResourceStatus
	// ApplyTransition calls the function required to move to the specified status
	ApplyTransition(resourcestatus.ResourceStatus) error
	// SteadyState returns the transition state of the resource defined as "ready"
	SteadyState() resourcestatus.ResourceStatus
	// SetAppliedStatus sets the applied status of resource and returns whether
	// the resource is already in a transition
	SetAppliedStatus(status resourcestatus.ResourceStatus) bool
	// GetAppliedStatus gets the applied status of resource
	GetAppliedStatus() resourcestatus.ResourceStatus
	// StatusString returns the string of the resource status
	StatusString(status resourcestatus.ResourceStatus) string
	// GetTerminalReason returns string describing why the resource failed to
	// provision
	GetTerminalReason() string
	// DependOnTaskNetwork shows whether the resource creation needs task network setup beforehand
	DependOnTaskNetwork() bool
	// GetContainerDependencies returns dependent containers for a status
	GetContainerDependencies(resourcestatus.ResourceStatus) []apicontainer.ContainerDependency
	// BuildContainerDependency adds a new dependency container and its satisfied status
	BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
		dependent resourcestatus.ResourceStatus)
	// Initialize will initialize the resource fields of the resource
	Initialize(res *ResourceFields,
		taskKnownStatus apitaskstatus.TaskStatus, taskDesiredStatus apitaskstatus.TaskStatus)

	json.Marshaler
	json.Unmarshaler
}

// TransitionDependenciesMap is a map of the dependent resource status to other
// dependencies that must be satisfied.
type TransitionDependenciesMap map[resourcestatus.ResourceStatus]TransitionDependencySet

// TransitionDependencySet contains dependencies that impact transitions of
// resources.
type TransitionDependencySet struct {
	// ContainerDependencies is the set of containers on which a transition is
	// dependent.
	ContainerDependencies []apicontainer.ContainerDependency `json:"ContainerDependencies"`
}
