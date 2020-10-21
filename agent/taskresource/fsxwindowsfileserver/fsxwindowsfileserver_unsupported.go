// +build !windows

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

package fsxwindowsfileserver

import (
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	fsxfactory "github.com/aws/amazon-ecs-agent/agent/fsx/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/pkg/errors"
)

// FSxWindowsFileServerResource represents a fsxwindowsfileserver resource
type FSxWindowsFileServerResource struct {
}

// FSxWindowsFileServerVolumeConfig represents fsxWindowsFileServer volume configuration.
type FSxWindowsFileServerVolumeConfig struct {
}

// NewFSxWindowsFileServerResource creates a new FSxWindowsFileServerResource object
func NewFSxWindowsFileServerResource(
	taskARN,
	region string,
	name string,
	volumeType string,
	volumeConfig *FSxWindowsFileServerVolumeConfig,
	hostPath string,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	asmClientCreator asmfactory.ClientCreator,
	fsxClientCreator fsxfactory.FSxClientCreator) (*FSxWindowsFileServerResource, error) {
	return nil, errors.New("not supported")
}

func (fv *FSxWindowsFileServerResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (fv *FSxWindowsFileServerResource) GetTerminalReason() string {
	return "undefined"
}

// GetDesiredStatus safely returns the desired status of the task
func (fv *FSxWindowsFileServerResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetDesiredStatus safely sets the desired status of the resource
func (fv *FSxWindowsFileServerResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
}

// DesiredTerminal returns true if the fsxwindowsfileserver resource's desired status is REMOVED
func (fv *FSxWindowsFileServerResource) DesiredTerminal() bool {
	return false
}

// KnownCreated returns true if the fsxwindowsfileserver resource's known status is CREATED
func (fv *FSxWindowsFileServerResource) KnownCreated() bool {
	return false
}

// TerminalStatus returns the last transition state of fsxwindowsfileserver resource
func (fv *FSxWindowsFileServerResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (fv *FSxWindowsFileServerResource) NextKnownState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// ApplyTransition calls the function required to move to the specified status
func (fv *FSxWindowsFileServerResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	return errors.New("not supported")
}

// SteadyState returns the transition state of the resource defined as "ready"
func (fv *FSxWindowsFileServerResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetKnownStatus safely sets the currently known status of the resource
func (fv *FSxWindowsFileServerResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (fv *FSxWindowsFileServerResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	return false
}

// GetKnownStatus safely returns the currently known status of the task
func (fv *FSxWindowsFileServerResource) GetKnownStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// StatusString returns the string of the fsxwindowsfileserver resource status
func (fv *FSxWindowsFileServerResource) StatusString(status resourcestatus.ResourceStatus) string {
	return "undefined"
}

// SetCreatedAt sets the timestamp for resource's creation time
func (fv *FSxWindowsFileServerResource) SetCreatedAt(createdAt time.Time) {
}

// GetCreatedAt sets the timestamp for resource's creation time
func (fv *FSxWindowsFileServerResource) GetCreatedAt() time.Time {
	return time.Time{}
}

// Source returns the host path of the fsxwindowsfileserver resource which is used as the source of the volume mount
func (cfg *FSxWindowsFileServerVolumeConfig) Source() string {
	return "undefined"
}

// GetName safely returns the name of the resource
func (fv *FSxWindowsFileServerResource) GetName() string {
	return "undefined"
}

// Create is used to create all the fsxwindowsfileserver resources for a given task
func (fv *FSxWindowsFileServerResource) Create() error {
	return errors.New("not supported")
}

// Cleanup removes the fsxwindowsfileserver resources created for the task
func (fv *FSxWindowsFileServerResource) Cleanup() error {
	return errors.New("not supported")
}

// MarshalJSON serialises the FSxWindowsFileServerResourceJSON struct to JSON
func (fv *FSxWindowsFileServerResource) MarshalJSON() ([]byte, error) {
	return nil, errors.New("not supported")
}

// UnmarshalJSON deserialises the raw JSON to a FSxWindowsFileServerResourceJSON struct
func (fv *FSxWindowsFileServerResource) UnmarshalJSON(b []byte) error {
	return errors.New("not supported")
}

// GetAppliedStatus safely returns the currently applied status of the resource
func (fv *FSxWindowsFileServerResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

func (fv *FSxWindowsFileServerResource) DependOnTaskNetwork() bool {
	return false
}

func (fv *FSxWindowsFileServerResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) {
}

func (fv *FSxWindowsFileServerResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
