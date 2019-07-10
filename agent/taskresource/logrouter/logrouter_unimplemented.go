// +build !linux

// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package logrouter

import (
	"errors"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

const (
	// ResourceName is the name of the log router resource.
	ResourceName = "logrouter"
	// LogRouterTypeFluentd is the type of a fluentd log router.
	LogRouterTypeFluentd = "fluentd"
	// LogRouterTypeFluentbit is the type of a fluentbit log router.
	LogRouterTypeFluentbit = "fluentbit"
)

// LogRouterResource represents the log router resource.
type LogRouterResource struct{}

// NewLogRouterResource returns a new LogRouterResource.
func NewLogRouterResource(cluster, taskARN, taskDefinition, ec2InstanceID, dataDir, logRouterType string,
	ecsMetadataEnabled bool, containerToLogOptions map[string]map[string]string) *LogRouterResource {
	return &LogRouterResource{}
}

// SetDesiredStatus safely sets the desired status of the resource.
func (logRouter *LogRouterResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {}

// GetDesiredStatus safely returns the desired status of the resource.
func (logRouter *LogRouterResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// GetName safely returns the name of the resource.
func (logRouter *LogRouterResource) GetName() string {
	return "undefined"
}

// DesiredTerminal returns true if the resource's desired status is REMOVED.
func (logRouter *LogRouterResource) DesiredTerminal() bool {
	return false
}

// KnownCreated returns true if the resource's known status is CREATED.
func (logRouter *LogRouterResource) KnownCreated() bool {
	return false
}

// TerminalStatus returns the last transition state of log router resource.
func (logRouter *LogRouterResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// NextKnownState returns the state that the resource should progress to based
// on its `KnownState`.
func (logRouter *LogRouterResource) NextKnownState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// ApplyTransition calls the function required to move to the specified status.
func (logRouter *LogRouterResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	return errors.New("not implemented")
}

// SteadyState returns the transition state of the resource defined as "ready".
func (logRouter *LogRouterResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetKnownStatus safely sets the currently known status of the resource.
func (logRouter *LogRouterResource) SetKnownStatus(status resourcestatus.ResourceStatus) {}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition.
func (logRouter *LogRouterResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	return false
}

// GetKnownStatus safely returns the currently known status of the resource.
func (logRouter *LogRouterResource) GetKnownStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetCreatedAt sets the timestamp for resource's creation time.
func (logRouter *LogRouterResource) SetCreatedAt(createdAt time.Time) {}

// GetCreatedAt sets the timestamp for resource's creation time.
func (logRouter *LogRouterResource) GetCreatedAt() time.Time {
	return time.Time{}
}

// Create creates the log router resource.
func (logRouter *LogRouterResource) Create() error {
	return errors.New("not implemented")
}

// Cleanup cleans up the log router resource.
func (logRouter *LogRouterResource) Cleanup() error {
	return errors.New("not implemented")
}

// StatusString returns the string of the log router resource status.
func (logRouter *LogRouterResource) StatusString(status resourcestatus.ResourceStatus) string {
	return "undefined"
}

// GetTerminalReason returns the terminal reason for the resource.
func (logRouter *LogRouterResource) GetTerminalReason() string {
	return "undefined"
}

// MarshalJSON marshals LogRouterResource object.
func (logRouter *LogRouterResource) MarshalJSON() ([]byte, error) {
	return nil, errors.New("not implemented")
}

// UnmarshalJSON unmarshals LogRouterResource object.
func (logRouter *LogRouterResource) UnmarshalJSON(b []byte) error {
	return errors.New("not implemented")
}

// Initialize fills in the resource fields.
func (logRouter *LogRouterResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
}
