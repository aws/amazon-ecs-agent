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
	"github.com/aws/amazon-ecs-agent/agent/s3/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/utils/bufiowrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	ResourceName         = "envfile"
	envFileDirPath       = "envfiles"
	envTempFilePrefix    = "tmp_env"
	envFileExtension     = ".env"
	commentIndicator     = "#"
	envVariableDelimiter = "="

	renameBackoffMin      = 500 * time.Millisecond
	renameBackoffMax      = 5 * time.Second
	renameBackoffJitter   = 0.0
	renameBackoffMultiple = 1.5
	renameRetryAttempts   = 5

	s3DownloadTimeout = 30 * time.Second
)

// EnvironmentFileResource represents envfile as a task resource
// these environment files are retrieved from s3
type EnvironmentFileResource struct {
	cluster       string
	taskARN       string
	region        string
	resourceDir   string // path to store env var files
	containerName string

	// env file related attributes
	environmentFilesSource []apicontainer.EnvironmentFile // list of env file objects

	executionCredentialsID string
	credentialsManager     credentials.Manager
	s3ClientCreator        factory.S3ClientCreator
	ioutil                 ioutilwrapper.IOUtil
	bufio                  bufiowrapper.Bufio

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
func NewEnvironmentFileResource(cluster, taskARN, region, dataDir, containerName string, envfiles []apicontainer.EnvironmentFile,
	credentialsManager credentials.Manager, executionCredentialsID string) (*EnvironmentFileResource, error) {
	envfileResource := &EnvironmentFileResource{
		cluster:                cluster,
		taskARN:                taskARN,
		region:                 region,
		containerName:          containerName,
		environmentFilesSource: envfiles,
		ioutil:                 ioutilwrapper.NewIOUtil(),
		bufio:                  bufiowrapper.NewBufio(),
		s3ClientCreator:        factory.NewS3ClientCreator(),
		executionCredentialsID: executionCredentialsID,
		credentialsManager:     credentialsManager,
	}

	taskARNFields := strings.Split(taskARN, "/")
	taskID := taskARNFields[len(taskARNFields)-1]
	// we save envfiles for a task to path: /var/lib/ecs/data/envfiles/cluster_name/task_id/
	envfileResource.resourceDir = filepath.Join(dataDir, envFileDirPath, cluster, taskID)

	envfileResource.initStatusToTransition()
	return envfileResource, nil
}

// Initialize initializes the EnvironmentFileResource
func (envfile *EnvironmentFileResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {
	envfile.lock.Lock()

	envfile.initStatusToTransition()
	envfile.credentialsManager = resourceFields.CredentialsManager
	envfile.s3ClientCreator = factory.NewS3ClientCreator()
	envfile.ioutil = ioutilwrapper.NewIOUtil()
	envfile.bufio = bufiowrapper.NewBufio()
	envfile.lock.Unlock()

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

// Create performs resource creation. This retrieves env file contents concurrently
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

	var wg sync.WaitGroup
	errorEvents := make(chan error, len(envfile.environmentFilesSource))

	iamCredentials := executionCredentials.GetIAMRoleCredentials()
	for _, envfileSource := range envfile.environmentFilesSource {
		wg.Add(1)
		// if we support types besides S3 ARN, we will need to add filtering before the below method is called
		// call an additional go routine per env file
		go envfile.downloadEnvfileFromS3(envfileSource.Value, iamCredentials, &wg, errorEvents)
	}

	wg.Wait()
	close(errorEvents)

	if len(errorEvents) > 0 {
		var terminalReasons []string
		for err := range errorEvents {
			terminalReasons = append(terminalReasons, err.Error())
		}

		errorString := strings.Join(terminalReasons, ";")
		envfile.setTerminalReason(errorString)
		return errors.New(errorString)
	}

	return nil
}

var mkdirAll = os.MkdirAll

// createEnvfileDirectory creates the directory that we will be writing the
// envfile to - needs to be called for each different envfile
func (envfile *EnvironmentFileResource) createEnvfileDirectory(bucket, key string) error {
	// create directories to include bucket and key but not the actual resulting file
	keyDir := filepath.Dir(key)
	envfileDir := filepath.Join(envfile.resourceDir, bucket, keyDir)
	err := mkdirAll(envfileDir, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "unable to create envfiles directory with bucket %s", bucket)
	}

	return nil
}

func (envfile *EnvironmentFileResource) downloadEnvfileFromS3(envFilePath string, iamCredentials credentials.IAMRoleCredentials,
	wg *sync.WaitGroup, errorEvents chan error) {
	defer wg.Done()

	bucket, key, err := s3.ParseS3ARN(envFilePath)
	if err != nil {
		errorEvents <- fmt.Errorf("unable to parse bucket and key from s3 ARN specified in environmentFile %s, error: %v", envFilePath, err)
		return
	}

	s3Client, err := envfile.s3ClientCreator.NewS3ManagerClient(bucket, envfile.region, iamCredentials)
	if err != nil {
		errorEvents <- fmt.Errorf("unable to initialize s3 client for bucket %s, error: %v", bucket, err)
		return
	}

	err = envfile.createEnvfileDirectory(bucket, key)
	if err != nil {
		errorEvents <- fmt.Errorf("unable to initialize envfile resource directory, error: %v", err)
		return
	}

	seelog.Debugf("Downloading envfile with bucket name %v and key name %v", bucket, key)
	// we save envfiles to path: /var/lib/ecs/data/envfiles/cluster_name/task_id/${s3bucketname}/${s3filename.env}
	downloadPath := filepath.Join(envfile.resourceDir, bucket, key)
	err = envfile.writeEnvFile(func(file oswrapper.File) error {
		return s3.DownloadFile(bucket, key, s3DownloadTimeout, file, s3Client)
	}, downloadPath)

	if err != nil {
		errorEvents <- fmt.Errorf("unable to download env file with key %s from bucket %s, error: %v", key, bucket, err)
		return
	}

	seelog.Debugf("Downloaded envfile from s3 and saved to %s", downloadPath)
}

var rename = os.Rename

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
	// defer tmpFile.Close() in case something goes wrong and we don't actually hit the manual Close call
	// the source for golang *os.File.Close() shows that subsequent calls to Close() after the first
	// will do nothing except return syscall.EINVAL, so it is ok to make potentially multiple .Close calls
	defer tmpFile.Close()

	if err = writeFunc(tmpFile); err != nil {
		seelog.Errorf("Something went wrong trying to write to tmpFile %s", tmpFile.Name())
		return err
	}

	err = tmpFile.Close()
	if err != nil {
		seelog.Errorf("Error while closing temporary file %s created for envfile resource", tmpFile.Name())
		return err
	}

	backoff := retry.NewExponentialBackoff(renameBackoffMin, renameBackoffMax, renameBackoffJitter, renameBackoffMultiple)
	err = retry.RetryNWithBackoff(backoff, renameRetryAttempts, func() error {
		return rename(tmpFile.Name(), fullPathName)
	})
	if err != nil {
		seelog.Errorf("Something went wrong when trying to rename envfile from %s to %s", tmpFile.Name(), fullPathName)
		return err
	}

	return nil
}

var removeAll = os.RemoveAll

// Cleanup removes env file directory for the task
func (envfile *EnvironmentFileResource) Cleanup() error {
	err := removeAll(envfile.resourceDir)
	if err != nil {
		return fmt.Errorf("unable to remove envfile resource directory %s: %v", envfile.resourceDir, err)
	}

	seelog.Infof("Removed envfile resource directory at %s", envfile.resourceDir)
	return nil
}

type environmentFileResourceJSON struct {
	TaskARN                string                         `json:"taskARN"`
	ContainerName          string                         `json:"containerName"`
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
		TaskARN:       envfile.taskARN,
		ContainerName: envfile.containerName,
		CreatedAt:     &createdAt,
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
	envfile.containerName = envfileJson.ContainerName
	envfile.executionCredentialsID = envfileJson.ExecutionCredentialsID

	return nil
}

// GetContainerName returns the container that this resource is created for
func (envfile *EnvironmentFileResource) GetContainerName() string {
	return envfile.containerName
}

// this method converts EnvironmentFile objects into the path that it would've been downloaded at
// and returns the list
func (envfile *EnvironmentFileResource) convertEnvfileToPath() ([]string, error) {
	var envfileLocations []string

	for _, envfileObj := range envfile.environmentFilesSource {
		bucket, key, err := s3.ParseS3ARN(envfileObj.Value)
		if err != nil {
			seelog.Errorf("unable to parse bucket and key from s3 ARN specified in environmentFile %s", envfileObj.Value)
			return nil, err
		}

		downloadPath := filepath.Join(envfile.resourceDir, bucket, key)
		envfileLocations = append(envfileLocations, downloadPath)
	}

	return envfileLocations, nil
}

// ReadEnvVarsFromEnvFiles reads the environment files that have been downloaded
// and puts them into a list of maps
func (envfile *EnvironmentFileResource) ReadEnvVarsFromEnvfiles() ([]map[string]string, error) {
	var envVarsPerEnvfile []map[string]string
	envfileLocations, err := envfile.convertEnvfileToPath()
	if err != nil {
		return nil, err
	}

	for _, envfilePath := range envfileLocations {
		envVars, err := envfile.readEnvVarsFromFile(envfilePath)
		if err != nil {
			return nil, err
		}
		envVarsPerEnvfile = append(envVarsPerEnvfile, envVars)
	}

	return envVarsPerEnvfile, nil
}

var open = func(name string) (oswrapper.File, error) {
	return os.Open(name)
}

func (envfile *EnvironmentFileResource) readEnvVarsFromFile(envfilePath string) (map[string]string, error) {
	file, err := open(envfilePath)
	if err != nil {
		seelog.Errorf("Unable to open environment file at %s to read the variables", envfilePath)
		return nil, err
	}
	defer file.Close()

	scanner := envfile.bufio.NewScanner(file)
	envVars := make(map[string]string)
	lineNum := 0
	for scanner.Scan() {
		lineNum += 1
		line := scanner.Text()
		// if line starts with a #, ignore
		if strings.HasPrefix(line, commentIndicator) {
			continue
		}
		// only read the line that has "="
		if strings.Contains(line, envVariableDelimiter) {
			variables := strings.SplitN(line, envVariableDelimiter, 2)
			// verify that there is at least a character on each side
			if len(variables[0]) > 0 && len(variables[1]) > 0 {
				envVars[variables[0]] = variables[1]
			} else {
				seelog.Infof("Not applying line %d of environment file %s, key or value is empty.", lineNum, envfilePath)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		seelog.Errorf("Something went wrong trying to read environment file at %s", envfilePath)
		return nil, err
	}

	return envVars, nil
}

// GetAppliedStatus safely returns the currently applied status of the resource
func (envfile *EnvironmentFileResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	envfile.lock.RLock()
	defer envfile.lock.RUnlock()

	return envfile.appliedStatusUnsafe
}

// DependOnTaskNetwork shows whether the resource creation needs task network setup beforehand
func (envfile *EnvironmentFileResource) DependOnTaskNetwork() bool {
	return false
}

// BuildContainerDependency adds a new dependency container and its satisfied status
func (envfile *EnvironmentFileResource) BuildContainerDependency(containerName string, satisfied apicontainerstatus.ContainerStatus,
	dependent resourcestatus.ResourceStatus) {
}

// GetContainerDependencies returns dependent containers for a status
func (envfile *EnvironmentFileResource) GetContainerDependencies(dependent resourcestatus.ResourceStatus) []apicontainer.ContainerDependency {
	return nil
}
