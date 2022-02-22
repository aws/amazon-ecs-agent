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

package asmsecret

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/asm"
	"github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

const (
	// ResourceName is the name of the asmsecret resource
	ResourceName                       = "asmsecret"
	arnDelimiter                       = ":"
	asmARNResourceFormat               = "secret:{secretID}"
	asmARNResourceWithParametersFormat = "secret:secretID:jsonKey:versionStage:versionID"
)

// ASMSecretResource represents secrets as a task resource.
// The secrets are stored in AWS Secrets Manager.
type ASMSecretResource struct {
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

	// map to store all asm deduped secrets in the task, key is a combination of valueFrom and region
	requiredSecrets map[string]apicontainer.Secret
	// map to store secret values, key is a combination of valueFrom and region
	secretData map[string]string

	// ssmClientCreator is a factory interface that creates new SSM clients. This is
	// needed mostly for testing.
	asmClientCreator factory.ClientCreator

	// terminalReason should be set for resource creation failures. This ensures
	// the resource object carries some context for why provisioning failed.
	terminalReason     string
	terminalReasonOnce sync.Once

	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// NewASMSecretResource creates a new ASMSecretResource object
func NewASMSecretResource(taskARN string,
	asmSecrets map[string]apicontainer.Secret,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	asmClientCreator factory.ClientCreator) *ASMSecretResource {

	s := &ASMSecretResource{
		taskARN:                taskARN,
		requiredSecrets:        asmSecrets,
		credentialsManager:     credentialsManager,
		executionCredentialsID: executionCredentialsID,
		asmClientCreator:       asmClientCreator,
	}

	s.initStatusToTransition()
	return s
}

func (secret *ASMSecretResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(ASMSecretCreated): secret.Create,
	}
	secret.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

func (secret *ASMSecretResource) setTerminalReason(reason string) {
	secret.terminalReasonOnce.Do(func() {
		seelog.Infof("ASM secret resource: setting terminal reason for asm secret resource in task: [%s]", secret.taskARN)
		secret.terminalReason = reason
	})
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (secret *ASMSecretResource) GetTerminalReason() string {
	return secret.terminalReason
}

// SetDesiredStatus safely sets the desired status of the resource
func (secret *ASMSecretResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	secret.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (secret *ASMSecretResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.desiredStatusUnsafe
}

// GetName safely returns the name of the resource
func (secret *ASMSecretResource) GetName() string {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return ResourceName
}

// DesiredTerminal returns true if the secret's desired status is REMOVED
func (secret *ASMSecretResource) DesiredTerminal() bool {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.desiredStatusUnsafe == resourcestatus.ResourceStatus(ASMSecretRemoved)
}

// KnownCreated returns true if the secret's known status is CREATED
func (secret *ASMSecretResource) KnownCreated() bool {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.knownStatusUnsafe == resourcestatus.ResourceStatus(ASMSecretCreated)
}

// TerminalStatus returns the last transition state of asmsecret
func (secret *ASMSecretResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(ASMSecretRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (secret *ASMSecretResource) NextKnownState() resourcestatus.ResourceStatus {
	return secret.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (secret *ASMSecretResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := secret.resourceStatusToTransitionFunction[nextState]
	if !ok {
		return errors.Errorf("resource [%s]: transition to %s impossible", secret.GetName(),
			secret.StatusString(nextState))
	}
	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (secret *ASMSecretResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(ASMSecretCreated)
}

// SetKnownStatus safely sets the currently known status of the resource
func (secret *ASMSecretResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	secret.knownStatusUnsafe = status
	secret.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (secret *ASMSecretResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if secret.appliedStatus == resourcestatus.ResourceStatus(ASMSecretStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if secret.appliedStatus <= knownStatus {
		secret.appliedStatus = resourcestatus.ResourceStatus(ASMSecretStatusNone)
	}
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (secret *ASMSecretResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	if secret.appliedStatus != resourcestatus.ResourceStatus(ASMSecretStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	secret.appliedStatus = status
	return true
}

// GetKnownStatus safely returns the currently known status of the task
func (secret *ASMSecretResource) GetKnownStatus() resourcestatus.ResourceStatus {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.knownStatusUnsafe
}

// StatusString returns the string of the cgroup resource status
func (secret *ASMSecretResource) StatusString(status resourcestatus.ResourceStatus) string {
	return ASMSecretStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time
func (secret *ASMSecretResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	secret.lock.Lock()
	defer secret.lock.Unlock()

	secret.createdAt = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (secret *ASMSecretResource) GetCreatedAt() time.Time {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.createdAt
}

// It spins up multiple goroutines in order to retrieve values in parallel.
func (secret *ASMSecretResource) Create() error {

	// To fail fast, check execution role first
	executionCredentials, ok := secret.credentialsManager.GetTaskCredentials(secret.getExecutionCredentialsID())
	if !ok {
		// No need to log here. managedTask.applyResourceState already does that
		err := errors.New("ASM secret resource: unable to find execution role credentials")
		secret.setTerminalReason(err.Error())
		return err
	}
	iamCredentials := executionCredentials.GetIAMRoleCredentials()

	var wg sync.WaitGroup

	// Get the maximum number of errors to be returned, which will be one error per goroutine
	errorEvents := make(chan error, len(secret.requiredSecrets))

	seelog.Debugf("ASM secret resource: retrieving secrets for containers in task: [%s]", secret.taskARN)
	secret.secretData = make(map[string]string)

	for _, asmsecret := range secret.getRequiredSecrets() {
		wg.Add(1)
		// Spin up goroutine per secret to speed up processing time
		go secret.retrieveASMSecretValue(asmsecret, iamCredentials, &wg, errorEvents)
	}

	wg.Wait()
	close(errorEvents)

	if len(errorEvents) > 0 {
		var terminalReasons []string
		for err := range errorEvents {
			terminalReasons = append(terminalReasons, err.Error())
		}

		errorString := strings.Join(terminalReasons, ";")
		secret.setTerminalReason(errorString)
		return errors.New(errorString)
	}
	return nil
}

// retrieveASMSecretValue reads secret value from cache first, if not exists, call GetSecretFromASM to retrieve value
// AWS secrets Manager
func (secret *ASMSecretResource) retrieveASMSecretValue(apiSecret apicontainer.Secret, iamCredentials credentials.IAMRoleCredentials, wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()

	asmClient := secret.asmClientCreator.NewASMClient(apiSecret.Region, iamCredentials)
	seelog.Debugf("ASM secret resource: retrieving resource for secret %v in region %s for task: [%s]", apiSecret.ValueFrom, apiSecret.Region, secret.taskARN)
	input, jsonKey, err := getASMParametersFromInput(apiSecret.ValueFrom)
	if err != nil {
		errorEvents <- fmt.Errorf("trying to retrieve secret with value %s resulted in error: %v", apiSecret.ValueFrom, err)
		return
	}

	if input.SecretId == nil {
		errorEvents <- fmt.Errorf("could not find a secretsmanager secretID from value %s", apiSecret.ValueFrom)
		return

	}

	secretValue, err := asm.GetSecretFromASMWithInput(input, asmClient, jsonKey)
	if err != nil {
		errorEvents <- fmt.Errorf("fetching secret data from AWS Secrets Manager in region %s: %v", apiSecret.Region, err)
		return
	}

	secret.lock.Lock()
	defer secret.lock.Unlock()

	// put secret value in secretData
	secretKey := apiSecret.GetSecretResourceCacheKey()
	secret.secretData[secretKey] = secretValue
}

func pointerOrNil(in string) *string {
	if in == "" {
		return nil
	}

	return aws.String(in)
}

// Agent follows what Cloudformation does here with using Dynamic References to specify Template Values
// in the format secret-id:json-key:version-stage:version-id
// the input will always be a full ARN for ASM
func getASMParametersFromInput(valueFrom string) (input *secretsmanager.GetSecretValueInput, jsonKey string, err error) {
	arnObj, err := arn.Parse(valueFrom)
	if err != nil {
		seelog.Warnf("Unable to parse ARN %s when trying to retrieve ASM secret", valueFrom)
		return nil, "", err
	}

	input = &secretsmanager.GetSecretValueInput{}

	paramValues := strings.Split(arnObj.Resource, arnDelimiter) // arnObj.Resource looks like secret:secretID:...
	if len(paramValues) == len(strings.Split(asmARNResourceFormat, arnDelimiter)) {
		input.SecretId = &valueFrom
		return input, "", nil
	}
	if len(paramValues) != len(strings.Split(asmARNResourceWithParametersFormat, arnDelimiter)) {
		// can't tell what input this is, throw some error
		err = errors.New("an invalid ARN format for the AWS Secrets Manager secret was specified. Specify a valid ARN and try again.")
		return nil, "", err
	}

	input.SecretId = pointerOrNil(reconstructASMARN(arnObj))
	jsonKey = paramValues[2]
	input.VersionStage = pointerOrNil(paramValues[3])
	input.VersionId = pointerOrNil(paramValues[4])

	return input, jsonKey, nil
}

// this method is to reconstruct an ASM ARN that has the enhancement parameters
// attached to it. in order to call secretsmanager:GetSecretValue, the entire ARN
// (including the 6 character special identifier tacked on by ASM) is required or
// just the secret name itself is required.
func reconstructASMARN(arnARN arn.ARN) string {
	// arn resource should look like secret:secretID:jsonKey:versionStage:versionID
	secretIDAndParams := strings.Split(arnARN.Resource, arnDelimiter)
	// reconstruct the secret id without the parameters
	secretID := fmt.Sprintf("%s%s%s", secretIDAndParams[0], arnDelimiter, secretIDAndParams[1])
	secretIDARN := arn.ARN{
		Partition: arnARN.Partition,
		Service:   arnARN.Service,
		Region:    arnARN.Region,
		AccountID: arnARN.AccountID,
		Resource:  secretID,
	}.String()

	return secretIDARN
}

// getRequiredSecrets returns the requiredSecrets field of asmsecret task resource
func (secret *ASMSecretResource) getRequiredSecrets() map[string]apicontainer.Secret {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.requiredSecrets
}

// getExecutionCredentialsID returns the execution role's credential ID
func (secret *ASMSecretResource) getExecutionCredentialsID() string {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.executionCredentialsID
}

// Cleanup removes the secret value created for the task
func (secret *ASMSecretResource) Cleanup() error {
	secret.clearASMSecretValue()
	return nil
}

// clearASMSecretValue cycles through the collection of secret value data and
// removes them from the task
func (secret *ASMSecretResource) clearASMSecretValue() {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	for key := range secret.secretData {
		delete(secret.secretData, key)
	}
}

// GetCachedSecretValue retrieves the secret value from secretData field
func (secret *ASMSecretResource) GetCachedSecretValue(secretKey string) (string, bool) {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	s, ok := secret.secretData[secretKey]
	return s, ok
}

// SetCachedSecretValue set the secret value in the secretData field given the key and value
func (secret *ASMSecretResource) SetCachedSecretValue(secretKey string, secretValue string) {
	secret.lock.Lock()
	defer secret.lock.Unlock()

	if secret.secretData == nil {
		secret.secretData = make(map[string]string)
	}

	secret.secretData[secretKey] = secretValue
}

func (secret *ASMSecretResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
	secret.initStatusToTransition()
	secret.credentialsManager = resourceFields.CredentialsManager
	secret.asmClientCreator = resourceFields.ASMClientCreator

	// if task hasn't turn to 'created' status, and it's desire status is 'running'
	// the resource status needs to be reset to 'NONE' status so the secret value
	// will be retrieved again
	if taskKnownStatus < status.TaskCreated &&
		taskDesiredStatus <= status.TaskRunning {
		secret.SetKnownStatus(resourcestatus.ResourceStatusNone)
	}
}

type ASMSecretResourceJSON struct {
	TaskARN                string                         `json:"taskARN"`
	CreatedAt              *time.Time                     `json:"createdAt,omitempty"`
	DesiredStatus          *ASMSecretStatus               `json:"desiredStatus"`
	KnownStatus            *ASMSecretStatus               `json:"knownStatus"`
	RequiredSecrets        map[string]apicontainer.Secret `json:"secretResources"`
	ExecutionCredentialsID string                         `json:"executionCredentialsID"`
}

// MarshalJSON serialises the ASMSecretResource struct to JSON
func (secret *ASMSecretResource) MarshalJSON() ([]byte, error) {
	if secret == nil {
		return nil, errors.New("asmsecret resource is nil")
	}
	createdAt := secret.GetCreatedAt()
	return json.Marshal(ASMSecretResourceJSON{
		TaskARN:   secret.taskARN,
		CreatedAt: &createdAt,
		DesiredStatus: func() *ASMSecretStatus {
			desiredState := secret.GetDesiredStatus()
			s := ASMSecretStatus(desiredState)
			return &s
		}(),
		KnownStatus: func() *ASMSecretStatus {
			knownState := secret.GetKnownStatus()
			s := ASMSecretStatus(knownState)
			return &s
		}(),
		RequiredSecrets:        secret.getRequiredSecrets(),
		ExecutionCredentialsID: secret.getExecutionCredentialsID(),
	})
}

// UnmarshalJSON deserialises the raw JSON to a ASMSecretResource struct
func (secret *ASMSecretResource) UnmarshalJSON(b []byte) error {
	temp := ASMSecretResourceJSON{}

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
func (secret *ASMSecretResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	secret.lock.RLock()
	defer secret.lock.RUnlock()

	return secret.appliedStatus
}

func (secret *ASMSecretResource) DependOnTaskNetwork() bool {
	return false
}

func (secret *ASMSecretResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) {
}

func (secret *ASMSecretResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
