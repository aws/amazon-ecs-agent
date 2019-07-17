// +build linux
// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package firelens

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
)

const (
	// ResourceName is the name of the firelens resource.
	ResourceName = "firelens"
	// tempFile is the name of the temp file generated during generating config file.
	tempFile = "temp_config_file"
	// configFilePerm is the permission for the firelens config file.
	configFilePerm = 0644
)

// FirelensResource models fluentd/fluentbit firelens container related resources as a task resource.
type FirelensResource struct {
	// Fields that are specific to firelens resource. They are only set at initialization so are not protected by lock.
	cluster               string
	taskARN               string
	taskDefinition        string
	ec2InstanceID         string
	resourceDir           string
	firelensConfigType    string
	ecsMetadataEnabled    bool
	containerToLogOptions map[string]map[string]string
	os                    oswrapper.OS
	ioutil                ioutilwrapper.IOUtil

	// Fields for the common functionality of task resource. Access to these fields are protected by lock.
	createdAtUnsafe     time.Time
	desiredStatusUnsafe resourcestatus.ResourceStatus
	knownStatusUnsafe   resourcestatus.ResourceStatus
	appliedStatusUnsafe resourcestatus.ResourceStatus
	statusToTransitions map[resourcestatus.ResourceStatus]func() error
	terminalReason      string
	terminalReasonOnce  sync.Once
	lock                sync.RWMutex
}

// NewFirelensResource returns a new FirelensResource.
func NewFirelensResource(cluster, taskARN, taskDefinition, ec2InstanceID, dataDir, firelensConfigType string,
	ecsMetadataEnabled bool, containerToLogOptions map[string]map[string]string) *FirelensResource {
	firelensResource := &FirelensResource{
		cluster:               cluster,
		taskARN:               taskARN,
		taskDefinition:        taskDefinition,
		ec2InstanceID:         ec2InstanceID,
		firelensConfigType:    firelensConfigType,
		ecsMetadataEnabled:    ecsMetadataEnabled,
		containerToLogOptions: containerToLogOptions,
		os:                    oswrapper.NewOS(),
		ioutil:                ioutilwrapper.NewIOUtil(),
	}

	fields := strings.Split(taskARN, "/")
	taskID := fields[len(fields)-1]
	firelensResource.resourceDir = filepath.Join(filepath.Join(dataDir, "firelens"), taskID)

	firelensResource.initStatusToTransition()
	return firelensResource
}

// GetCluster returns the cluster.
// Note: this method is currently only used in test. If this is going to be invoked by code, you should add lock.
func (firelens *FirelensResource) GetCluster() string {
	return firelens.cluster
}

// GetTaskARN returns the task arn.
// Note: this method is currently only used in test. If this is going to be invoked by code, you should add lock.
func (firelens *FirelensResource) GetTaskARN() string {
	return firelens.taskARN
}

// GetTaskDefinition returns the task definition.
// Note: this method is currently only used in test. If this is going to be invoked by code, you should add lock.
func (firelens *FirelensResource) GetTaskDefinition() string {
	return firelens.taskDefinition
}

// GetEC2InstanceID returns the ec2 instance id.
// Note: this method is currently only used in test. If this is going to be invoked by code, you should add lock.
func (firelens *FirelensResource) GetEC2InstanceID() string {
	return firelens.ec2InstanceID
}

// GetResourceDir returns the resource dir.
// Note: this method is currently only used in test. If this is going to be invoked by code, you should add lock.
func (firelens *FirelensResource) GetResourceDir() string {
	return firelens.resourceDir
}

// GetECSMetadataEnabled returns whether ecs metadata is enabled.
// Note: this method is currently only used in test. If this is going to be invoked by code, you should add lock.
func (firelens *FirelensResource) GetECSMetadataEnabled() bool {
	return firelens.ecsMetadataEnabled
}

// GetContainerToLogOptions returns a map of containers' log options.
// Note: this method is currently only used in test. If this is going to be invoked by code, you should add lock.
func (firelens *FirelensResource) GetContainerToLogOptions() map[string]map[string]string {
	return firelens.containerToLogOptions
}

// Initialize initializes the resource.
func (firelens *FirelensResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus, taskDesiredStatus status.TaskStatus) {
	firelens.lock.Lock()
	defer firelens.lock.Unlock()

	// Initialize the fields that won't be populated by unmarshalling from state file.
	firelens.initStatusToTransition()
	firelens.os = oswrapper.NewOS()
	firelens.ioutil = ioutilwrapper.NewIOUtil()
}

func (firelens *FirelensResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(FirelensCreated): firelens.Create,
	}
	firelens.statusToTransitions = resourceStatusToTransitionFunction
}

// GetName returns the name of the resource.
func (firelens *FirelensResource) GetName() string {
	return ResourceName
}

// DesiredTerminal returns true if the resource's desired status is REMOVED.
func (firelens *FirelensResource) DesiredTerminal() bool {
	firelens.lock.RLock()
	defer firelens.lock.RUnlock()

	return firelens.desiredStatusUnsafe == resourcestatus.ResourceStatus(FirelensRemoved)
}

func (firelens *FirelensResource) setTerminalReason(reason string) {
	firelens.terminalReasonOnce.Do(func() {
		seelog.Infof("firelens resource: setting terminal reason for task: [%s]", firelens.taskARN)
		firelens.terminalReason = reason
	})
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages.
func (firelens *FirelensResource) GetTerminalReason() string {
	firelens.lock.RLock()
	defer firelens.lock.RUnlock()

	return firelens.terminalReason
}

// SetDesiredStatus safely sets the desired status of the resource.
func (firelens *FirelensResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	firelens.lock.Lock()
	defer firelens.lock.Unlock()

	firelens.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task.
func (firelens *FirelensResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	firelens.lock.RLock()
	defer firelens.lock.RUnlock()

	return firelens.desiredStatusUnsafe
}

// SetKnownStatus safely sets the currently known status of the resource.
func (firelens *FirelensResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	firelens.lock.Lock()
	defer firelens.lock.Unlock()

	firelens.knownStatusUnsafe = status
	firelens.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status.
func (firelens *FirelensResource) updateAppliedStatusUnsafe(knownStatus resourcestatus.ResourceStatus) {
	if firelens.appliedStatusUnsafe == resourcestatus.ResourceStatus(FirelensStatusNone) {
		return
	}

	if firelens.appliedStatusUnsafe <= knownStatus {
		// Set applied status back to none to indicate that the transition has finished.
		firelens.appliedStatusUnsafe = resourcestatus.ResourceStatus(FirelensStatusNone)
	}
}

// GetKnownStatus safely returns the currently known status of the task.
func (firelens *FirelensResource) GetKnownStatus() resourcestatus.ResourceStatus {
	firelens.lock.RLock()
	defer firelens.lock.RUnlock()

	return firelens.knownStatusUnsafe
}

// KnownCreated returns true if the resource's known status is CREATED.
func (firelens *FirelensResource) KnownCreated() bool {
	firelens.lock.RLock()
	defer firelens.lock.RUnlock()

	return firelens.knownStatusUnsafe == resourcestatus.ResourceStatus(FirelensCreated)
}

// TerminalStatus returns the last transition state of resource.
func (firelens *FirelensResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(FirelensRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (firelens *FirelensResource) NextKnownState() resourcestatus.ResourceStatus {
	return firelens.GetKnownStatus() + 1
}

// SteadyState returns the transition state of the resource defined as "ready".
func (firelens *FirelensResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(FirelensCreated)
}

// ApplyTransition calls the function required to move to the specified status.
func (firelens *FirelensResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	firelens.lock.Lock()
	defer firelens.lock.Unlock()

	transitionFunc, ok := firelens.statusToTransitions[nextState]
	if !ok {
		err := errors.Errorf("resource [%s]: impossible to transition to %s", ResourceName,
			firelens.StatusString(nextState))
		firelens.setTerminalReason(err.Error())
		return err
	}
	return transitionFunc()
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition.
func (firelens *FirelensResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	firelens.lock.Lock()
	defer firelens.lock.Unlock()

	if firelens.appliedStatusUnsafe != resourcestatus.ResourceStatus(FirelensStatusNone) {
		// Return false to indicate the set operation failed.
		return false
	}

	firelens.appliedStatusUnsafe = status
	return true
}

// GetAppliedStatus returns the applied status.
func (firelens *FirelensResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	firelens.lock.RLock()
	defer firelens.lock.RUnlock()

	return firelens.appliedStatusUnsafe
}

// StatusString returns the string representation of the resource status.
func (firelens *FirelensResource) StatusString(status resourcestatus.ResourceStatus) string {
	return FirelensStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time.
func (firelens *FirelensResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	firelens.lock.Lock()
	defer firelens.lock.Unlock()

	firelens.createdAtUnsafe = createdAt
}

// GetCreatedAt returns the timestamp for resource's creation time.
func (firelens *FirelensResource) GetCreatedAt() time.Time {
	firelens.lock.RLock()
	defer firelens.lock.RUnlock()

	return firelens.createdAtUnsafe
}

// Create performs resource creation. This includes creating a config directory, a socket directory, and generating
// a config file under the config directory.
func (firelens *FirelensResource) Create() error {
	// Fail fast if firelens configuration type is invalid.
	if firelens.firelensConfigType != FirelensConfigTypeFluentd &&
		firelens.firelensConfigType != FirelensConfigTypeFluentbit {
		err := errors.New(fmt.Sprintf("invalid firelens configuration type: %s", firelens.firelensConfigType))
		firelens.setTerminalReason(err.Error())
		return err
	}

	err := firelens.createDirectories()
	if err != nil {
		err = errors.Wrapf(err, "unable to initialize resource directory %s", firelens.resourceDir)
		firelens.setTerminalReason(err.Error())
		return err
	}

	err = firelens.generateConfigFile()
	if err != nil {
		err = errors.Wrap(err, "unable to generate firelens config file")
		firelens.setTerminalReason(err.Error())
		return err
	}

	return nil
}

// createDirectories creates two directories:
//  - $(DATA_DIR)/firelens/$(TASK_ID)/config: used to store firelens config file. The config file under this directory
//    will be mounted to the firelens container at an expected path.
//  - $(DATA_DIR)/firelens/$(TASK_ID)/socket: used to store the unix socket. This directory will be mounted to
//    the firelens container and it will generate a socket file under this directory. Containers that use firelens to
//    send logs will then use this socket to send logs to the firelens container.
// Note: socket path has a limit of at most 108 characters on Linux. If using default data dir, the
// resulting socket path will be 79 characters (/var/lib/ecs/data/firelens/<task-id>/socket/fluent.sock) which is fine.
// However if ECS_HOST_DATA_DIR is specified to be a longer path, we will exceed the limit and fail. I don't really
// see a way to avoid this failure since ECS_HOST_DATA_DIR can be arbitrary long..
func (firelens *FirelensResource) createDirectories() error {
	configDir := filepath.Join(firelens.resourceDir, "config")
	err := firelens.os.MkdirAll(configDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "unable to create config directory")
	}

	socketDir := filepath.Join(firelens.resourceDir, "socket")
	err = firelens.os.MkdirAll(socketDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "unable to create socket directory")
	}
	return nil
}

// generateConfigFile generates a firelens config file at $(RESOURCE_DIR)/config/fluent.conf.
// This contains configs needed by the firelens container.
func (firelens *FirelensResource) generateConfigFile() error {
	config, err := firelens.generateConfig()
	if err != nil {
		return errors.Wrap(err, "unable to generate firelens config")
	}

	temp, err := firelens.ioutil.TempFile(firelens.resourceDir, tempFile)
	if err != nil {
		return err
	}
	defer temp.Close()
	if firelens.firelensConfigType == FirelensConfigTypeFluentd {
		err = config.WriteFluentdConfig(temp)
	} else {
		err = config.WriteFluentBitConfig(temp)
	}
	if err != nil {
		return err
	}

	err = temp.Chmod(os.FileMode(configFilePerm))
	if err != nil {
		return err
	}

	// Persist the config file to disk.
	err = temp.Sync()
	if err != nil {
		return err
	}

	confFilePath := filepath.Join(firelens.resourceDir, "config", "fluent.conf")
	err = firelens.os.Rename(temp.Name(), confFilePath)
	if err != nil {
		return err
	}

	seelog.Infof("Generated firelens config file at: %s", confFilePath)
	return nil
}

// Cleanup performs resource cleanup.
func (firelens *FirelensResource) Cleanup() error {
	err := firelens.os.RemoveAll(firelens.resourceDir)
	if err != nil {
		return fmt.Errorf("unable to remove firelens resource directory %s: %v", firelens.resourceDir, err)
	}

	seelog.Infof("Removed firelens resource directory at %s", firelens.resourceDir)
	return nil
}
