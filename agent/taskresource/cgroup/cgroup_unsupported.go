//go:build !linux
// +build !linux

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

package cgroup

import (
	"errors"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

// CgroupResource represents Cgroup resource
type CgroupResource struct{}

// SetDesiredStatus safely sets the desired status of the resource
func (c *CgroupResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {}

// GetDesiredStatus safely returns the desired status of the task
func (c *CgroupResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// GetName safely returns the name of the resource
func (c *CgroupResource) GetName() string {
	return "undefined"
}

// DesiredTerminal returns true if the cgroup's desired status is REMOVED
func (c *CgroupResource) DesiredTerminal() bool {
	return false
}

// KnownCreated returns true if the cgroup's known status is CREATED
func (c *CgroupResource) KnownCreated() bool {
	return false
}

// TerminalStatus returns the last transition state of cgroup
func (c *CgroupResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// NextKnownState returns the state that the resource should progress to based
// on its `KnownState`.
func (c *CgroupResource) NextKnownState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// ApplyTransition calls the function required to move to the specified status
func (c *CgroupResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	return errors.New("unsupported platform")
}

// SteadyState returns the transition state of the resource defined as "ready"
func (c *CgroupResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetKnownStatus safely sets the currently known status of the resource
func (c *CgroupResource) SetKnownStatus(status resourcestatus.ResourceStatus) {}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (c *CgroupResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	return false
}

// GetKnownStatus safely returns the currently known status of the task
func (c *CgroupResource) GetKnownStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// GetAppliedStatus safely returns the currently applied status of the resource
func (c *CgroupResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetCreatedAt sets the timestamp for resource's creation time
func (c *CgroupResource) SetCreatedAt(createdAt time.Time) {}

// GetCreatedAt sets the timestamp for resource's creation time
func (c *CgroupResource) GetCreatedAt() time.Time {
	return time.Time{}
}

// Create creates cgroup root for the task
func (c *CgroupResource) Create() error {
	return errors.New("unsupported platform")
}

// Cleanup removes the cgroup root created for the task
func (c *CgroupResource) Cleanup() error {
	return errors.New("unsupported platform")
}

// StatusString returns the string of the cgroup resource status
func (c *CgroupResource) StatusString(status resourcestatus.ResourceStatus) string {
	return "undefined"
}

func (c *CgroupResource) GetTerminalReason() string {
	return "undefined"
}

// MarshalJSON marshals CgroupResource object
func (c *CgroupResource) MarshalJSON() ([]byte, error) {
	return nil, errors.New("unsupported platform")
}

// UnmarshalJSON unmarshals CgroupResource object
func (c *CgroupResource) UnmarshalJSON(b []byte) error {
	return errors.New("unsupported platform")
}

// Initialize fills the resource fileds
func (cgroup *CgroupResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
}

func (cgroup *CgroupResource) DependOnTaskNetwork() bool {
	return false
}

func (cgroup *CgroupResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) {
}

func (cgroup *CgroupResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
