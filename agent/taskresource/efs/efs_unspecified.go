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

package efs

import (
	"context"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/pkg/errors"
)

const (
	// ResourceName is the name of the efs resource
	ResourceName = "efs"
	// The root directory to create mount target directory under
	DirPath = "/efs"
)

// EFSConfig represents efs volume configuration
type EFSConfig struct{}

func (efsConfig *EFSConfig) Source() string {
	return ""
}

// EFSResource handles EFS file system mount/unmount and tunnel service start/stop.
type EFSResource struct{}

// NewEFSResource creates a new EFSResource object
func NewEFSResource(taskARN string,
	volumeName string,
	ctx context.Context,
	client dockerapi.DockerClient,
	awsvpc bool,
	targetDir string,
	rootDir string,
	dnsEndpoints []string,
	readOnly bool,
	mountOptions string,
	dataDirOnHost string) *EFSResource {
	return &EFSResource{}
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (efs *EFSResource) GetTerminalReason() string {
	return "undefined"
}

// SetDesiredStatus safely sets the desired status of the resource
func (efs *EFSResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {}

// GetDesiredStatus safely returns the desired status of the task
func (efs *EFSResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// GetName safely returns the name of the resource
func (efs *EFSResource) GetName() string {
	return "undefined"
}

// DesiredTerminal returns true if the efs's desired status is REMOVED
func (efs *EFSResource) DesiredTerminal() bool {
	return false
}

// KnownCreated returns true if the efs's known status is CREATED
func (efs *EFSResource) KnownCreated() bool {
	return false
}

// TerminalStatus returns the last transition state of efs
func (efs *EFSResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (efs *EFSResource) NextKnownState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// ApplyTransition calls the function required to move to the specified status
func (efs *EFSResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	return errors.New("not implemented")
}

// SteadyState returns the transition state of the resource defined as "ready"
func (efs *EFSResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetKnownStatus safely sets the currently known status of the resource
func (efs *EFSResource) SetKnownStatus(status resourcestatus.ResourceStatus) {}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (efs *EFSResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	return false
}

// GetKnownStatus safely returns the currently known status of the task
func (efs *EFSResource) GetKnownStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetCreatedAt sets the timestamp for resource's creation time
func (efs *EFSResource) SetCreatedAt(createdAt time.Time) {}

// GetCreatedAt sets the timestamp for resource's creation time
func (efs *EFSResource) GetCreatedAt() time.Time {
	return time.Time{}
}

// StatusString returns the string of the efs resource status
func (efs *EFSResource) StatusString(status resourcestatus.ResourceStatus) string {
	return "undefined"
}

// Create create host directory and mount the EFS file system. If the encryption is required, send notification
// to start tunnel service
func (efs *EFSResource) Create() error {
	return errors.New("not implemented.")
}

// Cleanup removes the efs value created for the task
func (efs *EFSResource) Cleanup() error {
	return errors.New("not implemented.")
}

func (efs *EFSResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
}

type EFSResourceJSON struct{}

// MarshalJSON marshals EFSResource object using duplicate struct EFSResourceJSON
func (efs *EFSResource) MarshalJSON() ([]byte, error) {
	return nil, errors.New("not implemented")
}

// UnmarshalJSON unmarshals EFSResource object using duplicate struct EFSResourceJSON
func (efs *EFSResource) UnmarshalJSON(b []byte) error {
	return errors.New("not implemented")
}

func (efs *EFSResource) DependOnTaskNetwork() bool {
	return false
}

func (efs *EFSResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

func (efs *EFSResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) error {
	return errors.New("not implemented")
}

func (efs *EFSResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}

func (efs *EFSResource) SetPauseContainerName(name string) {}

// UpdateAppliedStatus safely updates the applied status of the resource
func (efs *EFSResource) UpdateAppliedStatus(status resourcestatus.ResourceStatus) {}
