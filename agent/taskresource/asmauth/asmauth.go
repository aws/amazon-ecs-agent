// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/asm"
	"github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

const (
	// ResourceName is the name of the ASM auth resource
	ResourceName              = "asm-auth"
	resourceProvisioningError = "TaskResourceError: Agent could not create task's platform resources"
)

// ASMAuthResource represents private registry credentials as a task resource.
// These credentials are stored in AWS Secrets Manager
type ASMAuthResource struct {
	taskARN             string
	createdAt           time.Time
	desiredStatusUnsafe resourcestatus.ResourceStatus
	knownStatusUnsafe   resourcestatus.ResourceStatus
	// appliedStatus is the status that has been "applied" (e.g., we've called some
	// operation such as 'Create' on the resource) but we don't yet know that the
	// application was successful, which may then change the known status. This is
	// used while progressing resource states in progressTask() of task manager
	appliedStatus                      resourcestatus.ResourceStatus
	resourceStatusToTransitionFunction map[resourcestatus.ResourceStatus]func() error
	credentialsManager                 credentials.Manager
	executionCredentialsID             string

	// required for asm private registry auth
	requiredASMResources []*apicontainer.ASMAuthData
	dockerAuthData       map[string]types.AuthConfig
	// asmClientCreator is a factory interface that creates new ASM clients. This is
	// needed mostly for testing as we're creating an asm client per every item in
	// the requiredASMResources list. Each of these items could be from different
	// regions.
	// TODO: Refactor this struct so that each ASMAuthData gets associated with
	// exactly one ASMAuthResource object
	asmClientCreator factory.ClientCreator

	// terminalReason should be set for resource creation failures. This ensures
	// the resource object carries some context for why provisoning failed.
	terminalReason     string
	terminalReasonOnce sync.Once

	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// NewASMAuthResource creates a new ASMAuthResource object
func NewASMAuthResource(taskARN string,
	asmRequirements []*apicontainer.ASMAuthData,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	asmClientCreator factory.ClientCreator) *ASMAuthResource {

	c := &ASMAuthResource{
		taskARN:                taskARN,
		requiredASMResources:   asmRequirements,
		credentialsManager:     credentialsManager,
		executionCredentialsID: executionCredentialsID,
		asmClientCreator:       asmClientCreator,
	}

	c.initStatusToTransition()
	return c
}

func (auth *ASMAuthResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(ASMAuthStatusCreated): auth.Create,
	}
	auth.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

func (auth *ASMAuthResource) setTerminalReason(reason string) {
	auth.terminalReasonOnce.Do(func() {
		seelog.Infof("ASM Auth: setting terminal reason for asm auth resource in task: [%s]", auth.taskARN)
		auth.terminalReason = reason
	})
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (auth *ASMAuthResource) GetTerminalReason() string {
	return auth.terminalReason
}

// SetDesiredStatus safely sets the desired status of the resource
func (auth *ASMAuthResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	auth.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (auth *ASMAuthResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.desiredStatusUnsafe
}

// GetName safely returns the name of the resource
func (auth *ASMAuthResource) GetName() string {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return ResourceName
}

// DesiredTerminal returns true if the cgroup's desired status is REMOVED
func (auth *ASMAuthResource) DesiredTerminal() bool {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.desiredStatusUnsafe == resourcestatus.ResourceStatus(ASMAuthStatusRemoved)
}

// KnownCreated returns true if the cgroup's known status is CREATED
func (auth *ASMAuthResource) KnownCreated() bool {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.knownStatusUnsafe == resourcestatus.ResourceStatus(ASMAuthStatusCreated)
}

// TerminalStatus returns the last transition state of cgroup
func (auth *ASMAuthResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(ASMAuthStatusRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (auth *ASMAuthResource) NextKnownState() resourcestatus.ResourceStatus {
	return auth.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (auth *ASMAuthResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := auth.resourceStatusToTransitionFunction[nextState]
	if !ok {
		return errors.Errorf("resource [%s]: transition to %s impossible", auth.GetName(),
			auth.StatusString(nextState))
	}
	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (auth *ASMAuthResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(ASMAuthStatusCreated)
}

// SetKnownStatus safely sets the currently known status of the resource
func (auth *ASMAuthResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	auth.knownStatusUnsafe = status
	auth.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (auth *ASMAuthResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if auth.appliedStatus == resourcestatus.ResourceStatus(ASMAuthStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if auth.appliedStatus <= knownStatus {
		auth.appliedStatus = resourcestatus.ResourceStatus(ASMAuthStatusNone)
	}
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (auth *ASMAuthResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	if auth.appliedStatus != resourcestatus.ResourceStatus(ASMAuthStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	auth.appliedStatus = status
	return true
}

// GetKnownStatus safely returns the currently known status of the task
func (auth *ASMAuthResource) GetKnownStatus() resourcestatus.ResourceStatus {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.knownStatusUnsafe
}

// StatusString returns the string of the cgroup resource status
func (auth *ASMAuthResource) StatusString(status resourcestatus.ResourceStatus) string {
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

// Create fetches credentials from ASM
func (auth *ASMAuthResource) Create() error {
	seelog.Infof("ASM Auth: Retrieving credentials for containers in task: [%s]", auth.taskARN)
	if auth.dockerAuthData == nil {
		auth.dockerAuthData = make(map[string]types.AuthConfig)
	}
	for _, a := range auth.GetRequiredASMResources() {
		err := auth.retrieveASMDockerAuthData(a)
		if err != nil {
			auth.setTerminalReason(err.Error())
			return err
		}
	}

	return nil
}

func (auth *ASMAuthResource) retrieveASMDockerAuthData(asmAuthData *apicontainer.ASMAuthData) error {
	secretID := asmAuthData.CredentialsParameter
	if _, ok := auth.GetASMDockerAuthConfig(secretID); ok {
		// resource for this secretID already retrieved
		return nil
	}

	executionCredentials, ok := auth.credentialsManager.GetTaskCredentials(auth.GetExecutionCredentialsID())
	if !ok {
		// No need to log here. managedTask.applyResourceState already does that
		return errors.New("asm resource: unable to find execution role credentials")
	}
	iamCredentials := executionCredentials.GetIAMRoleCredentials()
	asmClient := auth.asmClientCreator.NewASMClient(asmAuthData.Region, iamCredentials)
	seelog.Debugf("ASM Auth: Retrieving resource with ID [%s] in task: [%s]", secretID, auth.taskARN)
	dac, err := asm.GetDockerAuthFromASM(secretID, asmClient)
	if err != nil {
		return err
	}

	auth.lock.Lock()
	defer auth.lock.Unlock()

	// put retrieved dac in dockerAuthMap
	auth.dockerAuthData[secretID] = dac

	return nil
}

// GetRequiredASMResources returns the list of ASMAuthData that has to be
// retrieved from AWS Secrets Manager
func (auth *ASMAuthResource) GetRequiredASMResources() []*apicontainer.ASMAuthData {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.requiredASMResources
}

// GetExecutionCredentialsID returns the execution role's credential ID
func (auth *ASMAuthResource) GetExecutionCredentialsID() string {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.executionCredentialsID
}

// Cleanup removes the asm auth resource created for the task
func (auth *ASMAuthResource) Cleanup() error {
	auth.clearASMDockerAuthConfig()
	return nil
}

// clearASMDockerAuthConfig cycles through the collection of docker private
// registry auth data and removes them from the task
func (auth *ASMAuthResource) clearASMDockerAuthConfig() {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	for k := range auth.dockerAuthData {
		delete(auth.dockerAuthData, k)
	}
}

// GetASMDockerAuthConfig retrieves the docker private registry auth data from
// the task
func (auth *ASMAuthResource) GetASMDockerAuthConfig(secretID string) (types.AuthConfig, bool) {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	d, ok := auth.dockerAuthData[secretID]
	return d, ok
}

func (auth *ASMAuthResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
	auth.initStatusToTransition()
	auth.credentialsManager = resourceFields.CredentialsManager
	auth.asmClientCreator = resourceFields.ASMClientCreator
	if taskKnownStatus < status.TaskPulled && // Containers in the task need to be pulled
		taskDesiredStatus <= status.TaskRunning { // and the task is not terminal.
		// Reset the ASM resource's known status as None so that the NONE -> CREATED
		// transition gets triggered
		auth.SetKnownStatus(resourcestatus.ResourceStatusNone)
	}
}

type asmAuthResourceJSON struct {
	TaskARN                string                      `json:"taskARN"`
	CreatedAt              *time.Time                  `json:"createdAt,omitempty"`
	DesiredStatus          *ASMAuthStatus              `json:"desiredStatus"`
	KnownStatus            *ASMAuthStatus              `json:"knownStatus"`
	RequiredASMResources   []*apicontainer.ASMAuthData `json:"asmResources"`
	ExecutionCredentialsID string                      `json:"executionCredentialsID"`
}

// MarshalJSON serialises the ASMAuthResource struct to JSON
func (auth *ASMAuthResource) MarshalJSON() ([]byte, error) {
	if auth == nil {
		return nil, errors.New("asm-auth resource is nil")
	}
	createdAt := auth.GetCreatedAt()
	return json.Marshal(asmAuthResourceJSON{
		TaskARN:   auth.taskARN,
		CreatedAt: &createdAt,
		DesiredStatus: func() *ASMAuthStatus {
			desiredState := auth.GetDesiredStatus()
			status := ASMAuthStatus(desiredState)
			return &status
		}(),
		KnownStatus: func() *ASMAuthStatus {
			knownState := auth.GetKnownStatus()
			status := ASMAuthStatus(knownState)
			return &status
		}(),
		RequiredASMResources:   auth.GetRequiredASMResources(),
		ExecutionCredentialsID: auth.GetExecutionCredentialsID(),
	})
}

// UnmarshalJSON deserialises the raw JSON to a ASMAuthResource struct
func (auth *ASMAuthResource) UnmarshalJSON(b []byte) error {
	temp := asmAuthResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	if temp.DesiredStatus != nil {
		auth.SetDesiredStatus(resourcestatus.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		auth.SetKnownStatus(resourcestatus.ResourceStatus(*temp.KnownStatus))
	}
	if temp.CreatedAt != nil && !temp.CreatedAt.IsZero() {
		auth.SetCreatedAt(*temp.CreatedAt)
	}
	if temp.RequiredASMResources != nil {
		auth.requiredASMResources = temp.RequiredASMResources
	}
	auth.taskARN = temp.TaskARN
	auth.executionCredentialsID = temp.ExecutionCredentialsID

	return nil
}
