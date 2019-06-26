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

package logrouter

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
	// ResourceName is the name of the log router resource.
	ResourceName = "logrouter"
	// tempFile is the name of the temp file generated during generating config file.
	tempFile = "temp_config_file"
	// configFilePerm is the permission for the log router config file.
	configFilePerm = 0644
)

// LogRouterResource models fluentd/fluentbit log router related resources as a task resource.
type LogRouterResource struct {
	// Fields that are specific to log router resource.
	cluster               string
	taskARN               string
	taskDefinition        string
	ec2InstanceID         string
	resourceDir           string
	logRouterType         string
	ecsMetadataEnabled    bool
	containerToLogOptions map[string]map[string]string
	os                    oswrapper.OS
	ioutil                ioutilwrapper.IOUtil

	// Fields for the common functionality of task resource.
	createdAtUnsafe     time.Time
	desiredStatusUnsafe resourcestatus.ResourceStatus
	knownStatusUnsafe   resourcestatus.ResourceStatus
	appliedStatusUnsafe resourcestatus.ResourceStatus
	statusToTransitions map[resourcestatus.ResourceStatus]func() error
	terminalReason      string
	terminalReasonOnce  sync.Once
	lock                sync.RWMutex
}

// NewLogRouterResource returns a new LogRouterResource.
func NewLogRouterResource(cluster, taskARN, taskDefinition, ec2InstanceID, dataDir, logRouterType string,
	ecsMetadataEnabled bool, containerToLogOptions map[string]map[string]string) *LogRouterResource {
	logRouterResource := &LogRouterResource{
		cluster:               cluster,
		taskARN:               taskARN,
		taskDefinition:        taskDefinition,
		ec2InstanceID:         ec2InstanceID,
		logRouterType:         logRouterType,
		ecsMetadataEnabled:    ecsMetadataEnabled,
		containerToLogOptions: containerToLogOptions,
		os:                    oswrapper.NewOS(),
		ioutil:                ioutilwrapper.NewIOUtil(),
	}

	fields := strings.Split(taskARN, "/")
	taskID := fields[len(fields)-1]
	logRouterResource.resourceDir = filepath.Join(filepath.Join(dataDir, "logrouter"), taskID)

	logRouterResource.initStatusToTransition()
	return logRouterResource
}

// Initialize initializes the resource.
func (logRouter *LogRouterResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus, taskDesiredStatus status.TaskStatus) {
	logRouter.lock.Lock()
	defer logRouter.lock.Unlock()

	logRouter.initStatusToTransition()
}

func (logRouter *LogRouterResource) initStatusToTransition() {
	resourceStatusToTransitionFunction := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(LogRouterCreated): logRouter.Create,
	}
	logRouter.statusToTransitions = resourceStatusToTransitionFunction
}

// GetName returns the name of the resource.
func (logRouter *LogRouterResource) GetName() string {
	return ResourceName
}

// DesiredTerminal returns true if the resource's desired status is REMOVED.
func (logRouter *LogRouterResource) DesiredTerminal() bool {
	logRouter.lock.RLock()
	defer logRouter.lock.RUnlock()

	return logRouter.desiredStatusUnsafe == resourcestatus.ResourceStatus(LogRouterRemoved)
}

func (logRouter *LogRouterResource) setTerminalReason(reason string) {
	logRouter.terminalReasonOnce.Do(func() {
		seelog.Infof("log router resource: setting terminal reason for task: [%s]", logRouter.taskARN)
		logRouter.terminalReason = reason
	})
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages.
func (logRouter *LogRouterResource) GetTerminalReason() string {
	logRouter.lock.RLock()
	defer logRouter.lock.RUnlock()

	return logRouter.terminalReason
}

// SetDesiredStatus safely sets the desired status of the resource.
func (logRouter *LogRouterResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	logRouter.lock.Lock()
	defer logRouter.lock.Unlock()

	logRouter.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task.
func (logRouter *LogRouterResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	logRouter.lock.RLock()
	defer logRouter.lock.RUnlock()

	return logRouter.desiredStatusUnsafe
}

// SetKnownStatus safely sets the currently known status of the resource.
func (logRouter *LogRouterResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	logRouter.lock.Lock()
	defer logRouter.lock.Unlock()

	logRouter.knownStatusUnsafe = status
}

// GetKnownStatus safely returns the currently known status of the task.
func (logRouter *LogRouterResource) GetKnownStatus() resourcestatus.ResourceStatus {
	logRouter.lock.RLock()
	defer logRouter.lock.RUnlock()

	return logRouter.knownStatusUnsafe
}

// KnownCreated returns true if the resource's known status is CREATED.
func (logRouter *LogRouterResource) KnownCreated() bool {
	logRouter.lock.RLock()
	defer logRouter.lock.RUnlock()

	return logRouter.knownStatusUnsafe == resourcestatus.ResourceStatus(LogRouterCreated)
}

// TerminalStatus returns the last transition state of resource.
func (logRouter *LogRouterResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(LogRouterRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (logRouter *LogRouterResource) NextKnownState() resourcestatus.ResourceStatus {
	return logRouter.GetKnownStatus() + 1
}

// SteadyState returns the transition state of the resource defined as "ready".
func (logRouter *LogRouterResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(LogRouterCreated)
}

// ApplyTransition calls the function required to move to the specified status.
func (logRouter *LogRouterResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	logRouter.lock.Lock()
	defer logRouter.lock.Unlock()

	transitionFunc, ok := logRouter.statusToTransitions[nextState]
	if !ok {
		err := errors.Errorf("resource [%s]: impossible to transition to %s", ResourceName,
			logRouter.StatusString(nextState))
		logRouter.setTerminalReason(err.Error())
		return err
	}
	return transitionFunc()
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition.
func (logRouter *LogRouterResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	logRouter.lock.Lock()
	defer logRouter.lock.Unlock()

	if logRouter.appliedStatusUnsafe != resourcestatus.ResourceStatus(LogRouterStatusNone) {
		// Return false to indicate the set operation failed.
		return false
	}

	logRouter.appliedStatusUnsafe = status
	return true
}

// GetAppliedStatus returns the applied status.
func (logRouter *LogRouterResource) GetAppliedStatus() resourcestatus.ResourceStatus {
	logRouter.lock.RLock()
	defer logRouter.lock.RUnlock()

	return logRouter.appliedStatusUnsafe
}

// StatusString returns the string representation of the resource status.
func (logRouter *LogRouterResource) StatusString(status resourcestatus.ResourceStatus) string {
	return LogRouterStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time.
func (logRouter *LogRouterResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	logRouter.lock.Lock()
	defer logRouter.lock.Unlock()

	logRouter.createdAtUnsafe = createdAt
}

// GetCreatedAt returns the timestamp for resource's creation time.
func (logRouter *LogRouterResource) GetCreatedAt() time.Time {
	logRouter.lock.RLock()
	defer logRouter.lock.RUnlock()

	return logRouter.createdAtUnsafe
}

// Create performs resource creation. This includes creating a config directory, a socket directory, and generating
// a config file under the config directory.
func (logRouter *LogRouterResource) Create() error {
	// Fail fast if log router type is invalid.
	if logRouter.logRouterType != LogRouterTypeFluentd && logRouter.logRouterType != LogRouterTypeFluentbit {
		err := errors.New(fmt.Sprintf("invalid log router type: %s", logRouter.logRouterType))
		logRouter.setTerminalReason(err.Error())
		return err
	}

	err := logRouter.createDirectories()
	if err != nil {
		err = errors.Wrapf(err, "unable to initialize resource directory %s", logRouter.resourceDir)
		logRouter.setTerminalReason(err.Error())
		return err
	}

	err = logRouter.generateConfigFile()
	if err != nil {
		err = errors.Wrap(err, "unable to generate log router config file")
		logRouter.setTerminalReason(err.Error())
		return err
	}

	return nil
}

// createDirectories creates two directories:
//  - $(DATA_DIR)/logrouter/$(TASK_ID)/config: used to store log router config file. The config file under this directory
//    will be mounted to the log router container at an expected path.
//  - $(DATA_DIR)/logrouter/$(TASK_ID)/socket: used to store the unix socket. This directory will be mounted to
//    the log router container and it will generate a socket file under this directory. Containers that use log router
//    will then use this socket to send logs to log router.
func (logRouter *LogRouterResource) createDirectories() error {
	configDir := filepath.Join(logRouter.resourceDir, "config")
	err := logRouter.os.MkdirAll(configDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "unable to create config directory")
	}

	socketDir := filepath.Join(logRouter.resourceDir, "socket")
	err = logRouter.os.MkdirAll(socketDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "unable to create socket directory")
	}
	return nil
}

// generateConfigFile generates a log router config file at $(RESOURCE_DIR)/config/fluent.conf.
// This contains configs needed by the router container to properly route logs.
func (logRouter *LogRouterResource) generateConfigFile() error {
	config, err := logRouter.generateConfig()
	if err != nil {
		return errors.Wrap(err, "unable to generate log router config")
	}

	temp, err := logRouter.ioutil.TempFile(logRouter.resourceDir, tempFile)
	if err != nil {
		return err
	}
	defer temp.Close()
	if logRouter.logRouterType == LogRouterTypeFluentd {
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

	confFilePath := filepath.Join(logRouter.resourceDir, "config", "fluent.conf")
	err = logRouter.os.Rename(temp.Name(), confFilePath)
	if err != nil {
		return err
	}

	seelog.Infof("Generated log router config file at: %s", confFilePath)
	return nil
}

// Cleanup performs resource cleanup.
func (logRouter *LogRouterResource) Cleanup() error {
	err := logRouter.os.RemoveAll(logRouter.resourceDir)
	if err != nil {
		return fmt.Errorf("unable to remove log router resource directory %s: %v", logRouter.resourceDir, err)
	}

	seelog.Infof("Removed log router resource directory at %s", logRouter.resourceDir)
	return nil
}
