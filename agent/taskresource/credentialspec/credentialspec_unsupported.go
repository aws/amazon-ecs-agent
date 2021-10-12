//go:build !windows

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

package credentialspec

import (
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/pkg/errors"
)

// CredentialSpecResource is the abstraction for credentialspec resources
type CredentialSpecResource struct {
}

// NewCredentialSpecResource creates a new CredentialSpecResource object
func NewCredentialSpecResource(taskARN, region string,
	credentialSpecs []string,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	s3ClientCreator s3factory.S3ClientCreator) (*CredentialSpecResource, error) {
	return nil, errors.New("not supported")
}

func (cs *CredentialSpecResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (cs *CredentialSpecResource) GetTerminalReason() string {
	return "undefined"
}

// GetDesiredStatus safely returns the desired status of the task
func (cs *CredentialSpecResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetDesiredStatus safely sets the desired status of the resource
func (cs *CredentialSpecResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
}

// DesiredTerminal returns true if the credentialspec's desired status is REMOVED
func (cs *CredentialSpecResource) DesiredTerminal() bool {
	return false
}

// KnownCreated returns true if the credentialspec's known status is CREATED
func (cs *CredentialSpecResource) KnownCreated() bool {
	return false
}

// TerminalStatus returns the last transition state of credentialspec
func (cs *CredentialSpecResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (cs *CredentialSpecResource) NextKnownState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// ApplyTransition calls the function required to move to the specified status
func (cs *CredentialSpecResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	return errors.New("not implemented")
}

// SteadyState returns the transition state of the resource defined as "ready"
func (cs *CredentialSpecResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// SetKnownStatus safely sets the currently known status of the resource
func (cs *CredentialSpecResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (cs *CredentialSpecResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	return false
}

// GetKnownStatus safely returns the currently known status of the task
func (cs *CredentialSpecResource) GetKnownStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

// StatusString returns the string of the cgroup resource status
func (cs *CredentialSpecResource) StatusString(status resourcestatus.ResourceStatus) string {
	return "undefined"
}

// SetCreatedAt sets the timestamp for resource's creation time
func (cs *CredentialSpecResource) SetCreatedAt(createdAt time.Time) {
}

// GetCreatedAt sets the timestamp for resource's creation time
func (cs *CredentialSpecResource) GetCreatedAt() time.Time {
	return time.Time{}
}

// GetName safely returns the name of the resource
func (cs *CredentialSpecResource) GetName() string {
	return "undefined"
}

// Create is used to create all the credentialspec resources for a given task
func (cs *CredentialSpecResource) Create() error {
	return errors.New("not implemented")
}

func (cs *CredentialSpecResource) GetTargetMapping(credSpecInput string) (string, error) {
	return "", errors.New("not implemented")
}

// Cleanup removes the credentialspec created for the task
func (cs *CredentialSpecResource) Cleanup() error {
	return errors.New("not implemented")
}

// MarshalJSON serialises the CredentialSpecResourceJSON struct to JSON
func (cs *CredentialSpecResource) MarshalJSON() ([]byte, error) {
	return nil, errors.New("not implemented")
}

// UnmarshalJSON deserialises the raw JSON to a CredentialSpecResourceJSON struct
func (cs *CredentialSpecResource) UnmarshalJSON(b []byte) error {
	return errors.New("not implemented")
}

// GetAppliedStatus safely returns the currently applied status of the resource
func (cs *CredentialSpecResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatusNone
}

func (cs *CredentialSpecResource) DependOnTaskNetwork() bool {
	return false
}

func (cs *CredentialSpecResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) {
}

func (cs *CredentialSpecResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
