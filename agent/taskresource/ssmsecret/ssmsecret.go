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

package ssmsecret

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ssm"
	"github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

const (
	// ResourceName is the name of the ssmsecret resource
	ResourceName = "ssmsecret"

	//MaxBatchNum is the maximum batch number that ssm GetParameters API can accept at one time
	MaxBatchNum = 10
)

// SSMSecretResource represents secrets as a task resource.
// The secrets are stored in SSM Parameter Store.
type SSMSecretResource struct {
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

	// required for store ssm secrets value, key is region of secret
	requiredSecrets map[string][]apicontainer.Secret
	// map to store secret values, key is a combination of valueFrom and region
	secretData map[string]string

	// ssmClientCreator is a factory interface that creates new SSM clients. This is
	// needed mostly for testing.
	ssmClientCreator factory.SSMClientCreator

	// terminalReason should be set for resource creation failures. This ensures
	// the resource object carries some context for why provisioning failed.
	terminalReason     string
	terminalReasonOnce sync.Once

	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// NewSSMSecretResource creates a new SSMSecretResource object
func NewSSMSecretResource(taskARN string,
	ssmSecrets map[string][]apicontainer.Secret,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator factory.SSMClientCreator) *SSMSecretResource {

	s := &SSMSecretResource{
		taskARN:                taskARN,
		requiredSecrets:        ssmSecrets,
		credentialsManager:     credentialsManager,
		executionCredentialsID: executionCredentialsID,
		ssmClientCreator:       ssmClientCreator,
	}

	s.initStatusToTransition()
	return s
}

func (secret *SSMSecretResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(SSMSecretCreated): secret.Create,
	}
	secret.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

func (secret *SSMSecretResource) setTerminalReason(reason string) {
	secret.terminalReasonOnce.Do(func() {
		seelog.Infof("ssm secret resource: setting terminal reason for ssm secret resource in task: [%s]", secret.taskARN)
		secret.terminalReason = reason
	})
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (secret *SSMSecretResource) GetTerminalReason() string {
	return secret.terminalReason
}

// SetDesiredStatus safely sets the desired status of the resource
func (secret *SSMSecretResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	secret.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (secret *SSMSecretResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.desiredStatusUnsafe
}

// GetName safely returns the name of the resource
func (secret *SSMSecretResource) GetName() string {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return ResourceName
}

// DesiredTerminal returns true if the secret's desired status is REMOVED
func (secret *SSMSecretResource) DesiredTerminal() bool {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.desiredStatusUnsafe == resourcestatus.ResourceStatus(SSMSecretRemoved)
}

// KnownCreated returns true if the secret's known status is CREATED
func (secret *SSMSecretResource) KnownCreated() bool {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.knownStatusUnsafe == resourcestatus.ResourceStatus(SSMSecretCreated)
}

// TerminalStatus returns the last transition state of cgroup
func (secret *SSMSecretResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(SSMSecretRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (secret *SSMSecretResource) NextKnownState() resourcestatus.ResourceStatus {
	return secret.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (secret *SSMSecretResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := secret.resourceStatusToTransitionFunction[nextState]
	if !ok {
		return errors.Errorf("resource [%s]: transition to %s impossible", secret.GetName(),
			secret.StatusString(nextState))
	}
	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (secret *SSMSecretResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(SSMSecretCreated)
}

// SetKnownStatus safely sets the currently known status of the resource
func (secret *SSMSecretResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	secret.knownStatusUnsafe = status
	secret.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (secret *SSMSecretResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if secret.appliedStatus == resourcestatus.ResourceStatus(SSMSecretStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if secret.appliedStatus <= knownStatus {
		secret.appliedStatus = resourcestatus.ResourceStatus(SSMSecretStatusNone)
	}
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (secret *SSMSecretResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	if secret.appliedStatus != resourcestatus.ResourceStatus(SSMSecretStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	secret.appliedStatus = status
	return true
}

// GetKnownStatus safely returns the currently known status of the task
func (secret *SSMSecretResource) GetKnownStatus() resourcestatus.ResourceStatus {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.knownStatusUnsafe
}

// StatusString returns the string of the cgroup resource status
func (secret *SSMSecretResource) StatusString(status resourcestatus.ResourceStatus) string {
	return SSMSecretStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time
func (secret *SSMSecretResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	secret.lock.Lock()
	defer secret.lock.Unlock()

	secret.createdAt = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (secret *SSMSecretResource) GetCreatedAt() time.Time {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.createdAt
}

// Create fetches secret value from SSM in batches. It spins up multiple goroutines in order to
// retrieve values in parallel.
func (secret *SSMSecretResource) Create() error {

	// To fail fast, check execution role first
	executionCredentials, ok := secret.credentialsManager.GetTaskCredentials(secret.getExecutionCredentialsID())
	if !ok {
		// No need to log here. managedTask.applyResourceState already does that
		err := errors.New("ssm secret resource: unable to find execution role credentials")
		secret.setTerminalReason(err.Error())
		return err
	}
	iamCredentials := executionCredentials.GetIAMRoleCredentials()

	var wg sync.WaitGroup

	// Get the maximum number of errors can be returned, which will be one error per goroutine
	chanLen := secret.getGoRoutineMaxNum()
	errorEvents := make(chan error, chanLen)

	seelog.Infof("ssm secret resource: retrieving secrets for containers in task: [%s]", secret.taskARN)
	secret.secretData = make(map[string]string)

	for region, secrets := range secret.getRequiredSecrets() {
		wg.Add(1)
		// Spin up goroutine each region to speed up processing time
		go secret.retrieveSSMSecretValuesByRegion(region, secrets, iamCredentials, &wg, errorEvents)
	}

	wg.Wait()

	// Get the first error returned and set as terminal reason
	select {
	case err := <-errorEvents:
		secret.setTerminalReason(err.Error())
		return err
	default:
		return nil
	}
}

// getGoRoutineMaxNum calculates the maximum number of goroutines that we need to spin up
// to retrieve secret values from SSM parameter store. Assume each goroutine initiates one
// SSM GetParameters call and each call will have 10 parameters
func (secret *SSMSecretResource) getGoRoutineMaxNum() int {
	total := 0
	for _, secrets := range secret.requiredSecrets {
		total += len(secrets)/MaxBatchNum + 1
	}
	return total
}

// retrieveSSMSecretValuesByRegion reads secret values from cache first, if not exists, batches secrets based on field
// valueFrom and call retrieveSSMSecretValues to retrieve values from SSM
func (secret *SSMSecretResource) retrieveSSMSecretValuesByRegion(region string, secrets []apicontainer.Secret, iamCredentials credentials.IAMRoleCredentials, wg *sync.WaitGroup, errorEvents chan error) {
	seelog.Infof("ssm secret resource: retrieving secrets for region %s in task: [%s]", region, secret.taskARN)
	defer wg.Done()

	var wgPerRegion sync.WaitGroup
	var secretNames []string

	for _, s := range secrets {
		secretKey := s.GetSecretResourceCacheKey()
		if _, ok := secret.GetCachedSecretValue(secretKey); ok {
			continue
		}
		secretNames = append(secretNames, s.ValueFrom)
		if len(secretNames) == MaxBatchNum {
			secretNamesTmp := make([]string, MaxBatchNum)
			copy(secretNamesTmp, secretNames)
			wgPerRegion.Add(1)
			go secret.retrieveSSMSecretValues(region, secretNamesTmp, iamCredentials, &wgPerRegion, errorEvents)
			secretNames = []string{}
		}
	}

	if len(secretNames) > 0 {
		wgPerRegion.Add(1)
		go secret.retrieveSSMSecretValues(region, secretNames, iamCredentials, &wgPerRegion, errorEvents)
	}
	wgPerRegion.Wait()
}

// retrieveSSMSecretValues retrieves secret values from SSM parameter store and caches them into memory
func (secret *SSMSecretResource) retrieveSSMSecretValues(region string, names []string, iamCredentials credentials.IAMRoleCredentials, wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()

	ssmClient := secret.ssmClientCreator.NewSSMClient(region, iamCredentials)
	seelog.Debugf("ssm secret resource: retrieving resource for secrets %v in region [%s] in task: [%s]", names, region, secret.taskARN)
	secValueMap, err := ssm.GetSecretsFromSSM(names, ssmClient)
	if err != nil {
		errorEvents <- fmt.Errorf("fetching secret data from SSM Parameter Store in %s: %v", region, err)
		return
	}

	secret.lock.Lock()
	defer secret.lock.Unlock()

	// put secret value in secretData
	for secretName, secretValue := range secValueMap {
		secretKey := secretName + "_" + region
		secret.secretData[secretKey] = secretValue
	}
}

// getRequiredSecrets returns the requiredSecrets field of ssmsecret task resource
func (secret *SSMSecretResource) getRequiredSecrets() map[string][]apicontainer.Secret {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.requiredSecrets
}

// getExecutionCredentialsID returns the execution role's credential ID
func (secret *SSMSecretResource) getExecutionCredentialsID() string {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.executionCredentialsID
}

// Cleanup removes the secret value created for the task
func (secret *SSMSecretResource) Cleanup() error {
	secret.clearSSMSecretValue()
	return nil
}

// clearSSMSecretValue cycles through the collection of secret value data and
// removes them from the task
func (secret *SSMSecretResource) clearSSMSecretValue() {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	for key := range secret.secretData {
		delete(secret.secretData, key)
	}
}

// GetCachedSecretValue retrieves the secret value from secretData field
func (secret *SSMSecretResource) GetCachedSecretValue(secretKey string) (string, bool) {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	s, ok := secret.secretData[secretKey]
	return s, ok
}

// SetCachedSecretValue set the secret value in the secretData field given the key and value
func (secret *SSMSecretResource) SetCachedSecretValue(secretKey string, secretValue string) {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	if secret.secretData == nil {
		secret.secretData = make(map[string]string)
	}

	secret.secretData[secretKey] = secretValue
}

func (secret *SSMSecretResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
	secret.initStatusToTransition()
	secret.credentialsManager = resourceFields.CredentialsManager
	secret.ssmClientCreator = resourceFields.SSMClientCreator

	// if task hasn't turn to 'created' status, and it's desire status is 'running'
	// the resource status needs to be reset to 'NONE' status so the secret value
	// will be retrieved again
	if taskKnownStatus < status.TaskCreated &&
		taskDesiredStatus <= status.TaskRunning {
		secret.SetKnownStatus(resourcestatus.ResourceStatusNone)
	}
}

type SSMSecretResourceJSON struct {
	TaskARN                string                           `json:"taskARN"`
	CreatedAt              *time.Time                       `json:"createdAt,omitempty"`
	DesiredStatus          *SSMSecretStatus                 `json:"desiredStatus"`
	KnownStatus            *SSMSecretStatus                 `json:"knownStatus"`
	RequiredSecrets        map[string][]apicontainer.Secret `json:"secretResources"`
	ExecutionCredentialsID string                           `json:"executionCredentialsID"`
}

// MarshalJSON serialises the SSMSecretResource struct to JSON
func (secret *SSMSecretResource) MarshalJSON() ([]byte, error) {
	if secret == nil {
		return nil, errors.New("ssmsecret resource is nil")
	}
	createdAt := secret.GetCreatedAt()
	return json.Marshal(SSMSecretResourceJSON{
		TaskARN:   secret.taskARN,
		CreatedAt: &createdAt,
		DesiredStatus: func() *SSMSecretStatus {
			desiredState := secret.GetDesiredStatus()
			s := SSMSecretStatus(desiredState)
			return &s
		}(),
		KnownStatus: func() *SSMSecretStatus {
			knownState := secret.GetKnownStatus()
			s := SSMSecretStatus(knownState)
			return &s
		}(),
		RequiredSecrets:        secret.getRequiredSecrets(),
		ExecutionCredentialsID: secret.getExecutionCredentialsID(),
	})
}

// UnmarshalJSON deserialises the raw JSON to a SSMSecretResource struct
func (secret *SSMSecretResource) UnmarshalJSON(b []byte) error {
	temp := SSMSecretResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	if temp.DesiredStatus != nil {
		secret.SetDesiredStatus(resourcestatus.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		secret.SetKnownStatus(resourcestatus.ResourceStatus(*temp.KnownStatus))
	}
	if temp.CreatedAt != nil && !temp.CreatedAt.IsZero() {
		secret.SetCreatedAt(*temp.CreatedAt)
	}
	if temp.RequiredSecrets != nil {
		secret.requiredSecrets = temp.RequiredSecrets
	}
	secret.taskARN = temp.TaskARN
	secret.executionCredentialsID = temp.ExecutionCredentialsID

	return nil
}

// GetAppliedStatus safely returns the currently applied status of the resource
func (secret *SSMSecretResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.appliedStatus
}

func (secret *SSMSecretResource) DependOnTaskNetwork() bool {
	return false
}

func (secret *SSMSecretResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) {
}

func (secret *SSMSecretResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
