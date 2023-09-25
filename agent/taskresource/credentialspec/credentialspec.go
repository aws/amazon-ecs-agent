// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package credentialspec

import (
	"encoding/json"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

type CredentialSpecResourceCommon struct {
	taskARN                string
	region                 string
	executionCredentialsID string
	credentialsManager     credentials.Manager
	createdAt              time.Time
	desiredStatusUnsafe    resourcestatus.ResourceStatus
	knownStatusUnsafe      resourcestatus.ResourceStatus
	// appliedStatus is the status that has been "applied" (e.g., we've called some
	// operation such as 'Create' on the resource) but we don't yet know that the
	// application was successful, which may then change the known status. This is
	// used while progressing resource states in progressTask() of task manager
	appliedStatus                      resourcestatus.ResourceStatus
	resourceStatusToTransitionFunction map[resourcestatus.ResourceStatus]func() error
	// terminalReason should be set for resource creation failures. This ensures
	// the resource object carries some context for why provisioning failed.
	terminalReason     string
	terminalReasonOnce sync.Once
	// ssmClientCreator is a factory interface that creates new SSM clients. This is
	// needed mostly for testing.
	ssmClientCreator ssmfactory.SSMClientCreator
	// s3ClientCreator is a factory interface that creates new S3 clients. This is
	// needed mostly for testing.
	s3ClientCreator s3factory.S3ClientCreator
	// secretsManagerClientCreator is a factory interface that creates new secret manager clients. This is
	// needed mostly for testing.
	secretsmanagerClientCreator asmfactory.ClientCreator
	// map to transform credentialspec values, key is an input credentialspec
	// Examples: (windows)
	// * key := credentialspec:file://credentialspec.json, value := credentialspec=file://credentialspec.json
	// * key := credentialspec:s3ARN, value := credentialspec=file://CredentialSpecResourceLocation/s3_taskARN_fileName.json
	// * key := credentialspec:ssmARN, value := credentialspec=file://CredentialSpecResourceLocation/ssm_taskARN_param.json
	// (linux)
	// * key := credentialspec:file://credentialspec.json, value := Path to kerberos tickets on the host machine
	// * key := credentialspec:ssmARN, value := Path to kerberos tickets on the host machine
	// * key := credentialspec:asmARN, value := Path to kerberos tickets on the host machine
	CredSpecMap map[string]string
	// The essential map of credentialspecs needed for the containers. It stores the map with the credentialSpecARN as
	// the key container name as the value.
	// Example item := arn:aws:ssm:us-east-1:XXXXXXXXXXXXX:parameter/x/y/c:container-sql
	// This stores the map of a credential spec to corresponding container name
	credentialSpecContainerMap map[string]string
	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

func (cs *CredentialSpecResource) Initialize(resourceFields *taskresource.ResourceFields,
	_ status.TaskStatus,
	_ status.TaskStatus) {

	cs.credentialsManager = resourceFields.CredentialsManager
	cs.ssmClientCreator = resourceFields.SSMClientCreator
	cs.s3ClientCreator = resourceFields.S3ClientCreator
	cs.secretsmanagerClientCreator = resourceFields.ASMClientCreator
	cs.initStatusToTransition()
}

func (cs *CredentialSpecResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(CredentialSpecCreated): cs.Create,
	}
	cs.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (cs *CredentialSpecResource) GetTerminalReason() string {
	return cs.terminalReason
}

func (cs *CredentialSpecResource) setTerminalReason(reason string) {
	cs.terminalReasonOnce.Do(func() {
		seelog.Debugf("credentialspec resource: setting terminal reason for credentialspec resource in task: [%s]", cs.taskARN)
		cs.terminalReason = reason
	})
}

// GetDesiredStatus safely returns the desired status of the task
func (cs *CredentialSpecResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.desiredStatusUnsafe
}

// SetDesiredStatus safely sets the desired status of the resource
func (cs *CredentialSpecResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	cs.desiredStatusUnsafe = status
}

// DesiredTerminal returns true if the credentialspec's desired status is REMOVED
func (cs *CredentialSpecResource) DesiredTerminal() bool {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.desiredStatusUnsafe == resourcestatus.ResourceStatus(CredentialSpecRemoved)
}

// KnownCreated returns true if the credentialspec's known status is CREATED
func (cs *CredentialSpecResource) KnownCreated() bool {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.knownStatusUnsafe == resourcestatus.ResourceStatus(CredentialSpecCreated)
}

// TerminalStatus returns the last transition state of credentialspec
func (cs *CredentialSpecResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(CredentialSpecRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (cs *CredentialSpecResource) NextKnownState() resourcestatus.ResourceStatus {
	return cs.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (cs *CredentialSpecResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := cs.resourceStatusToTransitionFunction[nextState]
	if !ok {
		err := errors.Errorf("resource [%s]: transition to %s impossible", cs.GetName(),
			cs.StatusString(nextState))
		cs.setTerminalReason(err.Error())
		return err
	}

	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (cs *CredentialSpecResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(CredentialSpecCreated)
}

// SetKnownStatus safely sets the currently known status of the resource
func (cs *CredentialSpecResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	cs.knownStatusUnsafe = status
	cs.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (cs *CredentialSpecResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if cs.appliedStatus == resourcestatus.ResourceStatus(CredentialSpecStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if cs.appliedStatus <= knownStatus {
		cs.appliedStatus = resourcestatus.ResourceStatus(CredentialSpecStatusNone)
	}
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (cs *CredentialSpecResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	if cs.appliedStatus != resourcestatus.ResourceStatus(CredentialSpecStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	cs.appliedStatus = status
	return true
}

// GetKnownStatus safely returns the currently known status of the task
func (cs *CredentialSpecResource) GetKnownStatus() resourcestatus.ResourceStatus {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.knownStatusUnsafe
}

// StatusString returns the string of the cgroup resource status
func (cs *CredentialSpecResource) StatusString(status resourcestatus.ResourceStatus) string {
	return CredentialSpecStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time
func (cs *CredentialSpecResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()

	cs.createdAt = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (cs *CredentialSpecResource) GetCreatedAt() time.Time {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.createdAt
}

// getExecutionCredentialsID returns the execution role's credential ID
func (cs *CredentialSpecResource) getExecutionCredentialsID() string {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.executionCredentialsID
}

// GetName safely returns the name of the resource
func (cs *CredentialSpecResource) GetName() string {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return ResourceName
}

func (cs *CredentialSpecResource) GetTargetMapping(credSpecInput string) (string, error) {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	targetCredSpecMapping, ok := cs.CredSpecMap[credSpecInput]
	if !ok {
		return "", errors.New("unable to obtain credentialspec mapping")
	}

	return targetCredSpecMapping, nil
}

// CredentialSpecResourceJSON is the json representation of the credentialspec resource
type CredentialSpecResourceJSONCommon struct {
	TaskARN                    string                `json:"taskARN"`
	CreatedAt                  *time.Time            `json:"createdAt,omitempty"`
	DesiredStatus              *CredentialSpecStatus `json:"desiredStatus"`
	KnownStatus                *CredentialSpecStatus `json:"knownStatus"`
	CredentialSpecContainerMap map[string]string     `json:"CredentialSpecContainerMap"`
	CredSpecMap                map[string]string     `json:"CredSpecMap"`
	ExecutionCredentialsID     string                `json:"executionCredentialsID"`
}

// MarshalJSON serialises the CredentialSpecResourceJSON struct to JSON
func (cs *CredentialSpecResource) MarshalJSON() ([]byte, error) {
	if cs == nil {
		return nil, errors.New("credential specresource is nil")
	}
	createdAt := cs.GetCreatedAt()

	credentialSpecResourceJSON := CredentialSpecResourceJSON{
		CredentialSpecResourceJSONCommon: &CredentialSpecResourceJSONCommon{
			TaskARN:   cs.taskARN,
			CreatedAt: &createdAt,
			DesiredStatus: func() *CredentialSpecStatus {
				desiredState := cs.GetDesiredStatus()
				s := CredentialSpecStatus(desiredState)
				return &s
			}(),
			KnownStatus: func() *CredentialSpecStatus {
				knownState := cs.GetKnownStatus()
				s := CredentialSpecStatus(knownState)
				return &s
			}(),
			CredentialSpecContainerMap: cs.credentialSpecContainerMap,
			CredSpecMap:                cs.getCredSpecMap(),
			ExecutionCredentialsID:     cs.getExecutionCredentialsID(),
		},
	}
	cs.MarshallPlatformSpecificFields(&credentialSpecResourceJSON)
	return json.Marshal(credentialSpecResourceJSON)
}

func (cs *CredentialSpecResource) getCredSpecMap() map[string]string {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.CredSpecMap
}

// UnmarshalJSON deserialises the raw JSON to a CredentialSpecResourceJSON struct
func (cs *CredentialSpecResource) UnmarshalJSON(b []byte) error {
	temp := CredentialSpecResourceJSON{
		CredentialSpecResourceJSONCommon: &CredentialSpecResourceJSONCommon{},
	}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	if cs.CredentialSpecResourceCommon == nil {
		cs.CredentialSpecResourceCommon = &CredentialSpecResourceCommon{}
	}

	if temp.DesiredStatus != nil {
		cs.SetDesiredStatus(resourcestatus.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		cs.SetKnownStatus(resourcestatus.ResourceStatus(*temp.KnownStatus))
	}
	if temp.CreatedAt != nil && !temp.CreatedAt.IsZero() {
		cs.SetCreatedAt(*temp.CreatedAt)
	}
	if temp.CredentialSpecContainerMap != nil {
		cs.credentialSpecContainerMap = temp.CredentialSpecContainerMap
	}
	if temp.CredSpecMap != nil {
		cs.CredSpecMap = temp.CredSpecMap
	}
	cs.taskARN = temp.TaskARN
	cs.executionCredentialsID = temp.ExecutionCredentialsID
	cs.UnmarshallPlatformSpecificFields(temp)

	return nil
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
