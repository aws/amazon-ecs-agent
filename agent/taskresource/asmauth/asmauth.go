// +build linux
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

package asmauth

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/asm"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"

	"github.com/cihub/seelog"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

const (
	// asmauth is the private registry authentication data
	resourceName              = "asm-auth"
	resourceProvisioningError = "TaskResourceError: Agent could not create task's platform resources"
)

type ASMAuthResource struct {
	taskARN             string
	createdAt           time.Time
	desiredStatusUnsafe taskresource.ResourceStatus
	knownStatusUnsafe   taskresource.ResourceStatus
	// appliedStatus is the status that has been "applied" (e.g., we've called some
	// operation such as 'Create' on the resource) but we don't yet know that the
	// application was successful, which may then change the known status. This is
	// used while progressing resource states in progressTask() of task manager
	appliedStatus                      taskresource.ResourceStatus
	resourceStatusToTransitionFunction map[taskresource.ResourceStatus]func() error
	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex

	credentialsManager     credentials.Manager
	executionCredentialsID string

	// required for asm private registry auth
	requiredASMResources []apicontainer.ASMAuthData
	dockerAuthData       map[string]docker.AuthConfiguration
	// asmRegionalizedClient
}

// WIP: should take a regionalized client builder
func NewASMAuthResource(taskARN string,
	asmRequirements []apicontainer.ASMAuthData,
	executionCredentialsID string,
	credentialsManager credentials.Manager) *ASMAuthResource {

	c := &ASMAuthResource{
		taskARN:                taskARN,
		requiredASMResources:   asmRequirements,
		credentialsManager:     credentialsManager,
		executionCredentialsID: executionCredentialsID,
	}

	c.initStatusToTransition()
	return c
}

func (auth *ASMAuthResource) retrieveASMResources() error {
	for _, a := range auth.requiredASMResources {
		err := auth.retrieveASMDockerAuthData(a)
		if err != nil {
			return err
		}
	}

	return nil
}

func (auth *ASMAuthResource) retrieveASMDockerAuthData(asmAuthData apicontainer.ASMAuthData) error {
	executionCredentials, ok := auth.credentialsManager.GetTaskCredentials(auth.executionCredentialsID)
	// if !ok return error
	if !ok {
		fmt.Errorf("WIP: could not find execution credentials")
	}
	iamCredentials := executionCredentials.GetIAMRoleCredentials()

	region := asmAuthData.Region
	regionalisedClient := asm.NewRegionalisedASMClient(region, iamCredentials)

	secretID := asmAuthData.CredentialsParameter
	dac, err := asm.GetDockerAuthFromASM(secretID, regionalisedClient)
	if err != nil {
		return err
	}

	// put retrieved dac in dockerAuthMap
	auth.dockerAuthData[secretID] = dac

	return nil
}

func (auth *ASMAuthResource) IsDurable() bool {
	return false
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (auth *ASMAuthResource) GetTerminalReason() string {
	// for cgroups we can send up a static string because this is an
	// implementation detail and unrelated to customer resources
	return resourceProvisioningError
}

func (auth *ASMAuthResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[taskresource.ResourceStatus]func() error{
		taskresource.ResourceStatus(ASMAuthStatusCreated): auth.Create,
	}
	auth.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

// SetDesiredStatus safely sets the desired status of the resource
func (auth *ASMAuthResource) SetDesiredStatus(status taskresource.ResourceStatus) {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	auth.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (auth *ASMAuthResource) GetDesiredStatus() taskresource.ResourceStatus {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.desiredStatusUnsafe
}

// GetName safely returns the name of the resource
func (auth *ASMAuthResource) GetName() string {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return resourceName
}

// DesiredTerminal returns true if the cgroup's desired status is REMOVED
func (auth *ASMAuthResource) DesiredTerminal() bool {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.desiredStatusUnsafe == taskresource.ResourceStatus(ASMAuthStatusRemoved)
}

// KnownCreated returns true if the cgroup's known status is CREATED
func (auth *ASMAuthResource) KnownCreated() bool {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.knownStatusUnsafe == taskresource.ResourceStatus(ASMAuthStatusCreated)
}

// TerminalStatus returns the last transition state of cgroup
func (auth *ASMAuthResource) TerminalStatus() taskresource.ResourceStatus {
	return taskresource.ResourceStatus(ASMAuthStatusRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (auth *ASMAuthResource) NextKnownState() taskresource.ResourceStatus {
	return auth.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (auth *ASMAuthResource) ApplyTransition(nextState taskresource.ResourceStatus) error {
	transitionFunc, ok := auth.resourceStatusToTransitionFunction[nextState]
	if !ok {
		seelog.Errorf("ASM Resource [%s]: unsupported desired state transition [%s]: %s",
			auth.taskARN, auth.GetName(), auth.StatusString(nextState))
		return errors.Errorf("resource [%s]: transition to %s impossible", auth.GetName(),
			auth.StatusString(nextState))
	}
	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (auth *ASMAuthResource) SteadyState() taskresource.ResourceStatus {
	return taskresource.ResourceStatus(ASMAuthStatusCreated)
}

// SetKnownStatus safely sets the currently known status of the resource
func (auth *ASMAuthResource) SetKnownStatus(status taskresource.ResourceStatus) {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	auth.knownStatusUnsafe = status
	auth.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (auth *ASMAuthResource) updateAppliedStatusUnsafe(knownStatus taskresource.ResourceStatus) {
	if auth.appliedStatus == taskresource.ResourceStatus(ASMAuthStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if auth.appliedStatus <= knownStatus {
		auth.appliedStatus = taskresource.ResourceStatus(ASMAuthStatusNone)
	}
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (auth *ASMAuthResource) SetAppliedStatus(status taskresource.ResourceStatus) bool {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	if auth.appliedStatus != taskresource.ResourceStatus(ASMAuthStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	auth.appliedStatus = status
	return true
}

// GetKnownStatus safely returns the currently known status of the task
func (auth *ASMAuthResource) GetKnownStatus() taskresource.ResourceStatus {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.knownStatusUnsafe
}

// StatusString returns the string of the cgroup resource status
func (auth *ASMAuthResource) StatusString(status taskresource.ResourceStatus) string {
	return ASMAuthStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time
func (auth *ASMAuthResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	auth.lock.Lock()
	defer auth.lock.Unlock()

	auth.createdAt = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (auth *ASMAuthResource) GetCreatedAt() time.Time {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.createdAt
}

// Create creates cgroup root for the task
func (auth *ASMAuthResource) Create() error {
	seelog.Info("WIP: calling asmAuthResource.Create()")
	err := auth.retrieveASMResources()
	if err != nil {
		seelog.Criticalf("asm resources [%s]: unable to setup cgroup root: %v", auth.taskARN, err)
		return err
	}

	return nil
}

// Cleanup removes the cgroup root created for the task
func (auth *ASMAuthResource) Cleanup() error {
	seelog.Info("WIP: calling asmAuthResource.Cleanup()")
	auth.clearASMDockerAuthConfig()
	return nil
}

// GetASMDockerAuthConfig retrieves the docker private registry auth data from
// the task
func (auth *ASMAuthResource) GetASMDockerAuthConfig(secretID string) (docker.AuthConfiguration, bool) {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	d, ok := auth.dockerAuthData[secretID]
	return d, ok
}

// ClearASMDockerAuthConfig cycles through the collection of docker private
// registry auth data and removes them from the task
func (auth *ASMAuthResource) clearASMDockerAuthConfig() {
	for k := range auth.dockerAuthData {
		delete(auth.dockerAuthData, k)
	}
}

type asmAuthResourceJSON struct {
	CreatedAt     time.Time      `json:",omitempty"`
	DesiredStatus *ASMAuthStatus `json:"DesiredStatus"`
	KnownStatus   *ASMAuthStatus `json:"KnownStatus"`
}

func (auth *ASMAuthResource) MarshalJSON() ([]byte, error) {
	if auth == nil {
		return nil, errors.New("asm-auth resource is nil")
	}
	return json.Marshal(asmAuthResourceJSON{
		auth.GetCreatedAt(),
		func() *ASMAuthStatus {
			desiredState := auth.GetDesiredStatus()
			status := ASMAuthStatus(desiredState)
			return &status
		}(),
		func() *ASMAuthStatus {
			knownState := auth.GetKnownStatus()
			status := ASMAuthStatus(knownState)
			return &status
		}(),
	})
}

func (auth *ASMAuthResource) UnmarshalJSON(b []byte) error {
	temp := asmAuthResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	if temp.DesiredStatus != nil {
		auth.SetDesiredStatus(taskresource.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		auth.SetKnownStatus(taskresource.ResourceStatus(*temp.KnownStatus))
	}
	return nil
}
