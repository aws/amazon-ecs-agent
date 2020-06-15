// +build windows

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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/s3"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	"github.com/aws/amazon-ecs-agent/agent/ssm"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	tempFileName = "temp_file"
	// filePerm is the permission for the credentialspec file.
	filePerm = 0644

	s3DownloadTimeout = 30 * time.Second

	// Environment variables to setup resource location
	envProgramData              = "ProgramData"
	dockerCredentialSpecDataDir = "docker/credentialspecs"
)

// CredentialSpecResource is the abstraction for credentialspec resources
type CredentialSpecResource struct {
	taskARN                string
	region                 string
	executionCredentialsID string
	credentialsManager     credentials.Manager
	ioutil                 ioutilwrapper.IOUtil
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
	// credentialSpecResourceLocation is the location for all the tasks' credentialspec artifacts
	credentialSpecResourceLocation string
	// required for processing credentialspecs
	// Example item := credentialspec:file://credentialspec.json
	requiredCredentialSpecs []string
	// map to transform credentialspec values, key is a input credentialspec
	// Examples:
	// * key := credentialspec:file://credentialspec.json, value := credentialspec=file://credentialspec.json
	// * key := credentialspec:s3ARN, value := credentialspec=file://CredentialSpecResourceLocation/s3_taskARN_fileName.json
	// * key := credentialspec:ssmARN, value := credentialspec=file://CredentialSpecResourceLocation/ssm_taskARN_param.json
	CredSpecMap map[string]string
	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// NewCredentialSpecResource creates a new CredentialSpecResource object
func NewCredentialSpecResource(taskARN, region string,
	credentialSpecs []string,
	executionCredentialsID string,
	credentialsManager credentials.Manager,
	ssmClientCreator ssmfactory.SSMClientCreator,
	s3ClientCreator s3factory.S3ClientCreator) (*CredentialSpecResource, error) {

	s := &CredentialSpecResource{
		taskARN:                 taskARN,
		region:                  region,
		requiredCredentialSpecs: credentialSpecs,
		credentialsManager:      credentialsManager,
		executionCredentialsID:  executionCredentialsID,
		ssmClientCreator:        ssmClientCreator,
		s3ClientCreator:         s3ClientCreator,
		CredSpecMap:             make(map[string]string),
		ioutil:                  ioutilwrapper.NewIOUtil(),
	}

	err := s.setCredentialSpecResourceLocation()
	if err != nil {
		return nil, err
	}

	s.initStatusToTransition()
	return s, nil
}

func (cs *CredentialSpecResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(CredentialSpecCreated): cs.Create,
	}
	cs.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

func (cs *CredentialSpecResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {

	cs.credentialsManager = resourceFields.CredentialsManager
	cs.ssmClientCreator = resourceFields.SSMClientCreator
	cs.s3ClientCreator = resourceFields.S3ClientCreator
	cs.initStatusToTransition()
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

// getRequiredCredentialSpecs returns the requiredCredentialSpecs field of credentialspec task resource
func (cs *CredentialSpecResource) getRequiredCredentialSpecs() []string {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.requiredCredentialSpecs
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

// Create is used to create all the credentialspec resources for a given task
func (cs *CredentialSpecResource) Create() error {
	var err error
	var iamCredentials credentials.IAMRoleCredentials

	executionCredentials, ok := cs.credentialsManager.GetTaskCredentials(cs.getExecutionCredentialsID())
	if ok {
		iamCredentials = executionCredentials.GetIAMRoleCredentials()
	}

	for _, credSpecStr := range cs.requiredCredentialSpecs {
		credSpecSplit := strings.SplitAfterN(credSpecStr, "credentialspec:", 2)
		if len(credSpecSplit) != 2 {
			seelog.Errorf("Invalid credentialspec: %s", credSpecStr)
			continue
		}
		credSpecValue := credSpecSplit[1]

		if strings.HasPrefix(credSpecValue, "file://") {
			err = cs.handleCredentialspecFile(credSpecStr)
			if err != nil {
				seelog.Errorf("Failed to handle the credentialspec file: %v", err)
				cs.setTerminalReason(err.Error())
				return err
			}
			continue
		}

		parsedARN, err := arn.Parse(credSpecValue)
		if err != nil {
			cs.setTerminalReason(err.Error())
			return err
		}

		parsedARNService := parsedARN.Service
		if parsedARNService == "s3" {
			err = cs.handleS3CredentialspecFile(credSpecStr, credSpecValue, iamCredentials)
			if err != nil {
				seelog.Errorf("Failed to handle the credentialspec file from s3: %v", err)
				cs.setTerminalReason(err.Error())
				return err
			}
		} else if parsedARNService == "ssm" {
			err = cs.handleSSMCredentialspecFile(credSpecStr, credSpecValue, iamCredentials)
			if err != nil {
				seelog.Errorf("Failed to handle the credentialspec file from SSM: %v", err)
				cs.setTerminalReason(err.Error())
				return err
			}
		} else {
			err = errors.New("unsupported credentialspec ARN, only s3/ssm ARNs are valid")
			cs.setTerminalReason(err.Error())
			return err
		}
	}

	return nil
}

func (cs *CredentialSpecResource) handleCredentialspecFile(credentialspec string) error {
	credSpecSplit := strings.SplitAfterN(credentialspec, "credentialspec:", 2)
	if len(credSpecSplit) != 2 {
		seelog.Errorf("Invalid credentialspec: %s", credentialspec)
		return errors.New("invalid credentialspec file specification")
	}
	credSpecFile := credSpecSplit[1]

	if !strings.HasPrefix(credSpecFile, "file://") {
		return errors.New("invalid credentialspec file specification")
	}

	dockerHostconfigSecOptCredSpec := strings.Replace(credentialspec, "credentialspec:", "credentialspec=", 1)
	cs.updateCredSpecMapping(credentialspec, dockerHostconfigSecOptCredSpec)

	return nil
}

func (cs *CredentialSpecResource) handleS3CredentialspecFile(originalCredentialspec, credentialspecS3ARN string, iamCredentials credentials.IAMRoleCredentials) error {
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		return err
	}

	parsedARN, err := arn.Parse(credentialspecS3ARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	bucket, key, err := s3.ParseS3ARN(credentialspecS3ARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	s3Client, err := cs.s3ClientCreator.NewS3ClientForBucket(bucket, cs.region, iamCredentials)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	resourceBase := filepath.Base(parsedARN.Resource)
	taskArnSplit := strings.Split(cs.taskARN, "/")
	length := len(taskArnSplit)
	if length < 2 {
		return errors.New("Failed to retrieve taskId from taskArn.")
	}

	localCredSpecFilePath := fmt.Sprintf("%s\\s3_%v_%s", cs.credentialSpecResourceLocation, taskArnSplit[length-1], resourceBase)
	err = cs.writeS3File(func(file oswrapper.File) error {
		return s3.DownloadFile(bucket, key, s3DownloadTimeout, file, s3Client)
	}, localCredSpecFilePath)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	dockerHostconfigSecOptCredSpec := fmt.Sprintf("credentialspec=file://%s", filepath.Base(localCredSpecFilePath))
	cs.updateCredSpecMapping(originalCredentialspec, dockerHostconfigSecOptCredSpec)

	return nil
}

func (cs *CredentialSpecResource) handleSSMCredentialspecFile(originalCredentialspec, credentialspecSSMARN string, iamCredentials credentials.IAMRoleCredentials) error {
	if iamCredentials == (credentials.IAMRoleCredentials{}) {
		err := errors.New("credentialspec resource: unable to find execution role credentials")
		cs.setTerminalReason(err.Error())
		return err
	}

	parsedARN, err := arn.Parse(credentialspecSSMARN)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	ssmClient := cs.ssmClientCreator.NewSSMClient(cs.region, iamCredentials)

	ssmParam := filepath.Base(parsedARN.Resource)
	ssmParams := []string{ssmParam}

	ssmParamMap, err := ssm.GetParametersFromSSM(ssmParams, ssmClient)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	ssmParamData := ssmParamMap[ssmParam]
	taskArnSplit := strings.Split(cs.taskARN, "/")
	length := len(taskArnSplit)
	if length < 2 {
		return errors.New("Failed to retrieve taskId from taskArn.")
	}
	localCredSpecFilePath := fmt.Sprintf("%s\\ssm_%v_%s", cs.credentialSpecResourceLocation, taskArnSplit[length-1], ssmParam)
	err = cs.writeSSMFile(ssmParamData, localCredSpecFilePath)
	if err != nil {
		cs.setTerminalReason(err.Error())
		return err
	}

	dockerHostconfigSecOptCredSpec := fmt.Sprintf("credentialspec=file://%s", filepath.Base(localCredSpecFilePath))
	cs.updateCredSpecMapping(originalCredentialspec, dockerHostconfigSecOptCredSpec)

	return nil
}

var rename = os.Rename

func (cs *CredentialSpecResource) writeS3File(writeFunc func(file oswrapper.File) error, filePath string) error {
	temp, err := cs.ioutil.TempFile(cs.credentialSpecResourceLocation, tempFileName)
	if err != nil {
		return err
	}

	err = writeFunc(temp)
	if err != nil {
		return err
	}

	err = temp.Close()
	if err != nil {
		seelog.Errorf("Error while closing the handle to file %s: %v", temp.Name(), err)
		return err
	}

	err = rename(temp.Name(), filePath)
	if err != nil {
		seelog.Errorf("Error while renaming the temporary file %s: %v", temp.Name(), err)
		return err
	}
	return nil
}

func (cs *CredentialSpecResource) writeSSMFile(ssmParamData, filePath string) error {
	return cs.ioutil.WriteFile(filePath, []byte(ssmParamData), filePerm)
}

func (cs *CredentialSpecResource) getCredSpecMap() map[string]string {
	cs.lock.RLock()
	defer cs.lock.RUnlock()

	return cs.CredSpecMap
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

func (cs *CredentialSpecResource) updateCredSpecMapping(credSpecInput, targetCredSpec string) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	seelog.Debugf("Updating credentialspec mapping for %s with %s", credSpecInput, targetCredSpec)
	cs.CredSpecMap[credSpecInput] = targetCredSpec
}

// Cleanup removes the credentialspec created for the task
func (cs *CredentialSpecResource) Cleanup() error {
	cs.clearCredentialSpec()
	return nil
}

var remove = os.Remove

// clearCredentialSpec cycles through the collection of credentialspec data and
// removes them from the task
func (cs *CredentialSpecResource) clearCredentialSpec() {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	for key, value := range cs.CredSpecMap {
		if strings.HasPrefix(key, "credentialspec:file://") {
			seelog.Debugf("Skipping cleanup of local credentialspec file option: %s", key)
			continue
		}
		// Split credentialspec to obtain local file-name
		credSpecSplit := strings.SplitAfterN(value, "credentialspec=file://", 2)
		if len(credSpecSplit) != 2 {
			seelog.Warnf("Unable to parse target credentialspec: %s", value)
			continue
		}
		localCredentialSpecFile := credSpecSplit[1]
		localCredentialSpecFilePath := filepath.Join(cs.credentialSpecResourceLocation, localCredentialSpecFile)
		err := remove(localCredentialSpecFilePath)
		if err != nil {
			seelog.Warnf("Unable to clear local credential spec file %s for task %s", localCredentialSpecFile, cs.taskARN)
		}
		delete(cs.CredSpecMap, key)
	}
}

// CredentialSpecResourceJSON is the json representation of the credentialspec resource
type CredentialSpecResourceJSON struct {
	TaskARN                 string                `json:"taskARN"`
	CreatedAt               *time.Time            `json:"createdAt,omitempty"`
	DesiredStatus           *CredentialSpecStatus `json:"desiredStatus"`
	KnownStatus             *CredentialSpecStatus `json:"knownStatus"`
	RequiredCredentialSpecs []string              `json:"credentialSpecResources"`
	CredSpecMap             map[string]string     `json:"CredSpecMap"`
	ExecutionCredentialsID  string                `json:"executionCredentialsID"`
}

// MarshalJSON serialises the CredentialSpecResourceJSON struct to JSON
func (cs *CredentialSpecResource) MarshalJSON() ([]byte, error) {
	if cs == nil {
		return nil, errors.New("credential specresource is nil")
	}
	createdAt := cs.GetCreatedAt()
	return json.Marshal(CredentialSpecResourceJSON{
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
		RequiredCredentialSpecs: cs.getRequiredCredentialSpecs(),
		CredSpecMap:             cs.getCredSpecMap(),
		ExecutionCredentialsID:  cs.getExecutionCredentialsID(),
	})
}

// UnmarshalJSON deserialises the raw JSON to a CredentialSpecResourceJSON struct
func (cs *CredentialSpecResource) UnmarshalJSON(b []byte) error {
	temp := CredentialSpecResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
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
	if temp.RequiredCredentialSpecs != nil {
		cs.requiredCredentialSpecs = temp.RequiredCredentialSpecs
	}
	if temp.CredSpecMap != nil {
		cs.CredSpecMap = temp.CredSpecMap
	}
	cs.taskARN = temp.TaskARN
	cs.executionCredentialsID = temp.ExecutionCredentialsID

	return nil
}

func (cs *CredentialSpecResource) setCredentialSpecResourceLocation() error {
	// TODO: Use registry to setup credentialspec resource location
	// This should always be available on Windows instances
	programDataDir := os.Getenv(envProgramData)
	if programDataDir != "" {
		// Sample resource location: C:\ProgramData\docker\credentialspecs
		cs.credentialSpecResourceLocation = filepath.Join(programDataDir, dockerCredentialSpecDataDir)
	}

	if cs.credentialSpecResourceLocation == "" {
		return errors.New("credentialspec resource location not available")
	}

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
