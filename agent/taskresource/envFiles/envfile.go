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

package envFiles

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/s3"
	"github.com/aws/amazon-ecs-agent/agent/s3/factory"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
)

const (
	ResourceName      = "envfile"
	envFileDirPath    = "envfiles"
	envTempFilePrefix = "tmp_env"
	envFileExtension  = ".env"

	s3DownloadTimeout = 30 * time.Second
)

// EnvironmentFileResource represents envfile as a task resource
// these environment files are retrieved from s3
type EnvironmentFileResource struct {
	cluster     string
	taskARN     string
	region      string
	resourceDir string // path to store env var files

	// env file related attributes
	environmentFilesSource []apicontainer.EnvironmentFile // list of env file objects

	executionCredentialsID string
	credentialsManager     credentials.Manager
	s3ClientCreator        factory.S3ClientCreator
	os                     oswrapper.OS
	ioutil                 ioutilwrapper.IOUtil

	// Fields for the common functionality of task resource. Access to these fields are protected by lock.
	createdAtUnsafe      time.Time
	desiredStatusUnsafe  resourcestatus.ResourceStatus
	knownStatusUnsafe    resourcestatus.ResourceStatus
	appliedStatusUnsafe  resourcestatus.ResourceStatus
	statusToTransitions  map[resourcestatus.ResourceStatus]func() error
	terminalReasonUnsafe string
	terminalReasonOnce   sync.Once
	lock                 sync.RWMutex
}

// NewEnvironmentFileResource creates a new EnvironmentFileResource object
func NewEnvironmentFileResource(cluster, taskARN, region, dataDir string, credentialsManager credentials.Manager,
	executionCredentialsID string) (*EnvironmentFileResource, error) {
	envfileResource := &EnvironmentFileResource{
		cluster:                cluster,
		taskARN:                taskARN,
		region:                 region,
		os:                     oswrapper.NewOS(),
		ioutil:                 ioutilwrapper.NewIOUtil(),
		s3ClientCreator:        factory.NewS3ClientCreator(),
		executionCredentialsID: executionCredentialsID,
		credentialsManager:     credentialsManager,
	}

	taskARNFields := strings.Split(taskARN, "/")
	taskID := taskARNFields[len(taskARNFields)-1]
	// we save envfiles to path: /var/lib/ecs/data/envfiles/cluster_name/task_id/${s3filename.env}
	envfileResource.resourceDir = filepath.Join(dataDir, envFileDirPath, cluster, taskID)

	envfileResource.initStatusToTransition()
	return envfileResource, nil
}

// Initialize initializes the EnvironmentFileResource
func (envfile *EnvironmentFileResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
	envfile.lock.Lock()
	defer envfile.lock.Unlock()

	envfile.initStatusToTransition()
	envfile.credentialsManager = resourceFields.CredentialsManager
	envfile.s3ClientCreator = factory.NewS3ClientCreator()
	envfile.os = oswrapper.NewOS()
	envfile.ioutil = ioutilwrapper.NewIOUtil()

	// if task isn't in 'created' status and desired status is 'running',
	// reset the resource status to 'NONE' so we always retrieve the data
	// this is in case agent crashes
	if taskKnownStatus < status.TaskCreated && taskDesiredStatus <= status.TaskRunning {
		envfile.SetKnownStatus(resourcestatus.ResourceStatusNone)
	}
}

func (envfile *EnvironmentFileResource) initStatusToTransition() {
	resourceStatusToTransitionFunc := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(EnvFileCreated): envfile.Create,
	}
	envfile.statusToTransitions = resourceStatusToTransitionFunc
}

// SetDesiredStatus safely sets the desired status of the resource
func (envfile *EnvironmentFileResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	envfile.lock.Lock()
	defer envfile.lock.Unlock()

	envfile.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the resource
func (envfile *EnvironmentFileResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	envfile.lock.RLock()
	defer envfile.lock.RUnlock()

	return envfile.desiredStatusUnsafe
}

func (envfile *EnvironmentFileResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if envfile.appliedStatusUnsafe == resourcestatus.ResourceStatus(EnvFileStatusNone) {
		return
	}

	// only apply if resource transition has already finished
	if envfile.appliedStatusUnsafe <= knownStatus {
		envfile.appliedStatusUnsafe = resourcestatus.ResourceStatus(EnvFileStatusNone)
	}
}

// SetKnownStatus safely sets the currently known status of the resource
func (envfile *EnvironmentFileResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	envfile.lock.Lock()
	defer envfile.lock.Unlock()

	envfile.knownStatusUnsafe = status
	envfile.updateAppliedStatusUnsafe(status)
}

// GetKnownStatus safely returns the currently known status of the resource
func (envfile *EnvironmentFileResource) GetKnownStatus() resourcestatus.ResourceStatus {
	envfile.lock.RLock()
	defer envfile.lock.RUnlock()

	return envfile.knownStatusUnsafe
}

// SetCreatedAt safely sets the timestamp for the resource's creation time
func (envfile *EnvironmentFileResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}

	envfile.lock.Lock()
	defer envfile.lock.Unlock()

	envfile.createdAtUnsafe = createdAt
}

// GetCreatedAt safely returns the timestamp for the resource's creation time
func (envfile *EnvironmentFileResource) GetCreatedAt() time.Time {
	envfile.lock.RLock()
	defer envfile.lock.RUnlock()

	return envfile.createdAtUnsafe
}

// GetName returns the name fo the resource
func (envfile *EnvironmentFileResource) GetName() string {
	return ResourceName
}

// DesiredTerminal returns true if the resource's desired status is REMOVED
func (envfile *EnvironmentFileResource) DesiredTerminal() bool {
	envfile.lock.RLock()
	defer envfile.lock.RUnlock()

	return envfile.desiredStatusUnsafe == resourcestatus.ResourceStatus(EnvironmentFileStatus(EnvFileRemoved))
}

// KnownCreated returns true if the resource's known status is CREATED
func (envfile *EnvironmentFileResource) KnownCreated() bool {
	envfile.lock.RLock()
	defer envfile.lock.RUnlock()

	return envfile.knownStatusUnsafe == resourcestatus.ResourceStatus(EnvFileCreated)
}

// TerminalStatus returns the last transition state of the resource
func (envfile *EnvironmentFileResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(EnvFileRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`
func (envfile *EnvironmentFileResource) NextKnownState() resourcestatus.ResourceStatus {
	return envfile.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (envfile *EnvironmentFileResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := envfile.statusToTransitions[nextState]
	if !ok {
		return errors.Errorf("resource [%s]: transition to %s impossible", envfile.GetName(),
			envfile.StatusString(nextState))
	}
	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (envfile *EnvironmentFileResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(EnvFileCreated)
}

// SetAppliedStatus sets the applied status of the resource and returns whether
// the resource is already in a transition
func (envfile *EnvironmentFileResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	envfile.lock.Lock()
	defer envfile.lock.Unlock()

	if envfile.appliedStatusUnsafe != resourcestatus.ResourceStatus(EnvFileStatusNone) {
		// set operation failed, return false
		return false
	}

	envfile.appliedStatusUnsafe = status
	return true
}

// StatusString returns the string representation of the resource status
func (envfile *EnvironmentFileResource) StatusString(status resourcestatus.ResourceStatus) string {
	return EnvironmentFileStatus(status).String()
}

// GetTerminalReason returns an error string to propagate up through to to
// state change messages
func (envfile *EnvironmentFileResource) GetTerminalReason() string {
	envfile.lock.RLock()
	defer envfile.lock.RUnlock()

	return envfile.terminalReasonUnsafe
}

func (envfile *EnvironmentFileResource) setTerminalReason(reason string) {
	envfile.lock.Lock()
	defer envfile.lock.Unlock()

	envfile.terminalReasonOnce.Do(func() {
		seelog.Infof("envfile resource: setting terminal reason for task: [%s]", envfile.taskARN)
		envfile.terminalReasonUnsafe = reason
	})
}

// Create performs resource creation. This retrieves env file contents in parallel
// from s3 and writes them to disk
func (envfile *EnvironmentFileResource) Create() error {
	seelog.Debugf("Creating envfile resource.")
	// make sure it has the task execution role
	executionCredentials, ok := envfile.credentialsManager.GetTaskCredentials(envfile.executionCredentialsID)
	if !ok {
		err := errors.New("environment file resource: unable to find execution role credentials")
		envfile.setTerminalReason(err.Error())
		return err
	}

	iamCredentials := executionCredentials.GetIAMRoleCredentials()
	for _, envfileSource := range envfile.environmentFilesSource {
		// if we support types besides S3 ARN, we will need to add filtering before the below method is called

		err := envfile.downloadEnvfileFromS3(envfileSource.Value, iamCredentials)
		if err != nil {
			err = errors.Wrapf(err, "unable to download envfile with ARN %v from s3", envfileSource.Value)
			envfile.setTerminalReason(err.Error())
			return err
		}
	}

	return nil
}

func (envfile *EnvironmentFileResource) downloadEnvfileFromS3(envFilePath string, iamCredentials credentials.IAMRoleCredentials) error {
	bucket, key, err := s3.ParseS3ARN(envFilePath)
	if err != nil {
		return errors.Wrapf(err, "unable to parse bucket and key from s3 ARN specified in environmentFile %s", envFilePath)
	}

	s3Client, err := envfile.s3ClientCreator.NewS3ClientForBucket(bucket, envfile.region, iamCredentials)
	if err != nil {
		return errors.Wrapf(err, "unable to initialize s3 client for bucket %s", bucket)
	}

	seelog.Debugf("Downlading envfile with bucket name %v and key name %v", bucket, key)
	// we save envfiles to path: /var/lib/ecs/data/envfiles/cluster_name/task_id/${s3filename.env}
	downloadPath := filepath.Join(envfile.resourceDir, key)
	err = envfile.writeEnvFile(func(file oswrapper.File) error {
		return s3.DownloadFile(bucket, key, s3DownloadTimeout, file, s3Client)
	}, downloadPath)

	if err != nil {
		return errors.Wrapf(err, "unable to download env file with key %s from bucket %s", key, bucket)
	}

	seelog.Debugf("Downloaded envfile from s3 and saved to %s", downloadPath)
	return nil
}

func (envfile *EnvironmentFileResource) writeEnvFile(writeFunc func(file oswrapper.File) error, fullPathName string) error {
	// File moves (renaming) are atomic while file writes are not
	// so we write to a temp file before renaming to actual file
	// multiple programs calling TempFile will not reference the same file
	// so this should be ok to be called by multiple go routines
	tmpFile, err := envfile.ioutil.TempFile(envfile.resourceDir, envTempFilePrefix)
	if err != nil {
		seelog.Errorf("Something went wrong trying to create a temp file with prefix %s", envTempFilePrefix)
		return err
	}
	defer tmpFile.Close()

	err = writeFunc(tmpFile)
	if err != nil {
		seelog.Errorf("Something went wrong trying to write to tmpFile %s", tmpFile.Name())
		return err
	}

	// persist file to disk
	err = tmpFile.Sync()
	if err != nil {
		seelog.Errorf("Something went wrong trying to persist envfile to disk")
		return err
	}

	err = envfile.os.Rename(tmpFile.Name(), fullPathName)
	if err != nil {
		seelog.Errorf("Something went wrong when trying to rename envfile from %s to %s", tmpFile.Name(), fullPathName)
		return err
	}

	return nil
}

// Cleanup removes env file directory for the task
func (envfile *EnvironmentFileResource) Cleanup() error {
	err := envfile.os.RemoveAll(envfile.resourceDir)
	if err != nil {
		return fmt.Errorf("unable to remove envfile resource directory %s: %v", envfile.resourceDir, err)
	}

	seelog.Infof("Removed envfile resource directory at %s", envfile.resourceDir)
	return nil
}

type environmentFileResourceJSON struct {
	TaskARN                string                         `json:"taskARN"`
	CreatedAt              *time.Time                     `json:"createdAt,omitempty"`
	DesiredStatus          *EnvironmentFileStatus         `json:"desiredStatus"`
	KnownStatus            *EnvironmentFileStatus         `json:"knownStatus"`
	EnvironmentFilesSource []apicontainer.EnvironmentFile `json:"environmentFilesSource"`
	ExecutionCredentialsID string                         `json:"executionCredentialsID"`
}

// MarshalJSON serializes the EnvironmentFileResource struct to JSON
func (envfile *EnvironmentFileResource) MarshalJSON() ([]byte, error) {
	if envfile == nil {
		return nil, errors.New("envfile resource is nil")
	}
	createdAt := envfile.GetCreatedAt()
	return json.Marshal(environmentFileResourceJSON{
		TaskARN:   envfile.taskARN,
		CreatedAt: &createdAt,
		DesiredStatus: func() *EnvironmentFileStatus {
			desiredState := envfile.GetDesiredStatus()
			envfileStatus := EnvironmentFileStatus(desiredState)
			return &envfileStatus
		}(),
		KnownStatus: func() *EnvironmentFileStatus {
			knownState := envfile.GetKnownStatus()
			envfileStatus := EnvironmentFileStatus(knownState)
			return &envfileStatus
		}(),
		EnvironmentFilesSource: envfile.environmentFilesSource,
		ExecutionCredentialsID: envfile.executionCredentialsID,
	})

}

// UnmarshalJSON deserializes the raw JSON to an EnvironmentFileResource struct
func (envfile *EnvironmentFileResource) UnmarshalJSON(b []byte) error {
	envfileJson := environmentFileResourceJSON{}

	if err := json.Unmarshal(b, &envfileJson); err != nil {
		return err
	}

	if envfileJson.DesiredStatus != nil {
		envfile.SetDesiredStatus(resourcestatus.ResourceStatus(*envfileJson.DesiredStatus))
	}

	if envfileJson.KnownStatus != nil {
		envfile.SetKnownStatus(resourcestatus.ResourceStatus(*envfileJson.KnownStatus))
	}

	if envfileJson.CreatedAt != nil && !envfileJson.CreatedAt.IsZero() {
		envfile.SetCreatedAt(*envfileJson.CreatedAt)
	}

	if envfileJson.EnvironmentFilesSource != nil {
		envfile.environmentFilesSource = envfileJson.EnvironmentFilesSource
	}

	envfile.taskARN = envfileJson.TaskARN
	envfile.executionCredentialsID = envfileJson.ExecutionCredentialsID

	return nil
}
