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

package cgroup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	control "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/cihub/seelog"
	"github.com/containerd/cgroups"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	memorySubsystem         = "/memory"
	memoryUseHierarchy      = "memory.use_hierarchy"
	rootReadOnlyPermissions = os.FileMode(400)
	resourceName            = "cgroup"
)

var (
	enableMemoryHierarchy = []byte(strconv.Itoa(1))
)

// CgroupResource represents Cgroup resource
type CgroupResource struct {
	taskARN             string
	control             control.Control
	cgroupRoot          string
	cgroupMountPath     string
	resourceSpec        specs.LinuxResources
	ioutil              ioutilwrapper.IOUtil
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
}

// NewCgroupResource is used to return an object that implements the Resource interface
func NewCgroupResource(taskARN string,
	control control.Control,
	ioutil ioutilwrapper.IOUtil,
	cgroupRoot string,
	cgroupMountPath string,
	resourceSpec specs.LinuxResources) *CgroupResource {
	c := &CgroupResource{
		taskARN:         taskARN,
		control:         control,
		ioutil:          ioutil,
		cgroupRoot:      cgroupRoot,
		cgroupMountPath: cgroupMountPath,
		resourceSpec:    resourceSpec,
	}
	c.initializeResourceStatusToTransitionFunction()
	return c
}

func (cgroup *CgroupResource) initializeResourceStatusToTransitionFunction() {
	resourceStatusToTransitionFunction := map[taskresource.ResourceStatus]func() error{
		taskresource.ResourceStatus(CgroupCreated): cgroup.Create,
	}
	cgroup.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

func (cgroup *CgroupResource) SetIOUtil(ioutil ioutilwrapper.IOUtil) {
	cgroup.ioutil = ioutil
}

// SetDesiredStatus safely sets the desired status of the resource
func (cgroup *CgroupResource) SetDesiredStatus(status taskresource.ResourceStatus) {
	cgroup.lock.Lock()
	defer cgroup.lock.Unlock()

	cgroup.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (cgroup *CgroupResource) GetDesiredStatus() taskresource.ResourceStatus {
	cgroup.lock.RLock()
	defer cgroup.lock.RUnlock()

	return cgroup.desiredStatusUnsafe
}

// GetName safely returns the name of the resource
func (cgroup *CgroupResource) GetName() string {
	cgroup.lock.RLock()
	defer cgroup.lock.RUnlock()

	return resourceName
}

// DesiredTerminal returns true if the cgroup's desired status is REMOVED
func (cgroup *CgroupResource) DesiredTerminal() bool {
	cgroup.lock.RLock()
	defer cgroup.lock.RUnlock()

	return cgroup.desiredStatusUnsafe == taskresource.ResourceStatus(CgroupRemoved)
}

// KnownCreated returns true if the cgroup's known status is CREATED
func (cgroup *CgroupResource) KnownCreated() bool {
	cgroup.lock.RLock()
	defer cgroup.lock.RUnlock()

	return cgroup.knownStatusUnsafe == taskresource.ResourceStatus(CgroupCreated)
}

// TerminalStatus returns the last transition state of cgroup
func (cgroup *CgroupResource) TerminalStatus() taskresource.ResourceStatus {
	return taskresource.ResourceStatus(CgroupRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (cgroup *CgroupResource) NextKnownState() taskresource.ResourceStatus {
	return cgroup.GetKnownStatus() + 1
}

// ApplyTransition calls the function required to move to the specified status
func (cgroup *CgroupResource) ApplyTransition(nextState taskresource.ResourceStatus) error {
	transitionFunc, ok := cgroup.resourceStatusToTransitionFunction[nextState]
	if !ok {
		seelog.Errorf("Cgroup Resource [%s]: unsupported desired state transition [%s]: %s",
			cgroup.taskARN, cgroup.GetName(), cgroup.StatusString(nextState))
		return errors.Errorf("resource [%s]: transition to %s impossible", cgroup.GetName(),
			cgroup.StatusString(nextState))
	}
	return transitionFunc()
}

// SteadyState returns the transition state of the resource defined as "ready"
func (cgroup *CgroupResource) SteadyState() taskresource.ResourceStatus {
	return taskresource.ResourceStatus(CgroupCreated)
}

// SetKnownStatus safely sets the currently known status of the resource
func (cgroup *CgroupResource) SetKnownStatus(status taskresource.ResourceStatus) {
	cgroup.lock.Lock()
	defer cgroup.lock.Unlock()

	cgroup.knownStatusUnsafe = status
	cgroup.updateAppliedStatusUnsafe(status)
}

// updateAppliedStatusUnsafe updates the resource transitioning status
func (cgroup *CgroupResource) updateAppliedStatusUnsafe(knownStatus taskresource.ResourceStatus) {
	if cgroup.appliedStatus == taskresource.ResourceStatus(CgroupStatusNone) {
		return
	}

	// Check if the resource transition has already finished
	if cgroup.appliedStatus <= knownStatus {
		cgroup.appliedStatus = taskresource.ResourceStatus(CgroupStatusNone)
	}
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (cgroup *CgroupResource) SetAppliedStatus(status taskresource.ResourceStatus) bool {
	cgroup.lock.Lock()
	defer cgroup.lock.Unlock()

	if cgroup.appliedStatus != taskresource.ResourceStatus(CgroupStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	cgroup.appliedStatus = status
	return true
}

// GetKnownStatus safely returns the currently known status of the task
func (cgroup *CgroupResource) GetKnownStatus() taskresource.ResourceStatus {
	cgroup.lock.RLock()
	defer cgroup.lock.RUnlock()

	return cgroup.knownStatusUnsafe
}

// StatusString returns the string of the cgroup resource status
func (cgroup *CgroupResource) StatusString(status taskresource.ResourceStatus) string {
	return CgroupStatus(status).String()
}

// SetCreatedAt sets the timestamp for resource's creation time
func (cgroup *CgroupResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	cgroup.lock.Lock()
	defer cgroup.lock.Unlock()

	cgroup.createdAt = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (cgroup *CgroupResource) GetCreatedAt() time.Time {
	cgroup.lock.RLock()
	defer cgroup.lock.RUnlock()

	return cgroup.createdAt
}

// Create creates cgroup root for the task
func (cgroup *CgroupResource) Create() error {
	err := cgroup.setupTaskCgroup()
	if err != nil {
		seelog.Criticalf("Cgroup resource [%s]: unable to setup cgroup root: %v", cgroup.taskARN, err)
		return err
	}
	return nil
}

func (cgroup *CgroupResource) setupTaskCgroup() error {
	cgroupRoot := cgroup.cgroupRoot
	seelog.Debugf("Cgroup resource [%s]: setting up cgroup at: %s", cgroup.taskARN, cgroupRoot)

	if cgroup.control.Exists(cgroupRoot) {
		seelog.Debugf("Cgroup resource [%s]: cgroup at %s already exists, skipping creation", cgroup.taskARN, cgroupRoot)
		return nil
	}

	cgroupSpec := control.Spec{
		Root:  cgroupRoot,
		Specs: &cgroup.resourceSpec,
	}

	_, err := cgroup.control.Create(&cgroupSpec)
	if err != nil {
		return errors.Wrapf(err, "cgroup resource [%s]: setup cgroup: unable to create cgroup at %s", cgroup.taskARN, cgroupRoot)
	}

	// enabling cgroup memory hierarchy by doing 'echo 1 > memory.use_hierarchy'
	memoryHierarchyPath := filepath.Join(cgroup.cgroupMountPath, memorySubsystem, cgroupRoot, memoryUseHierarchy)
	err = cgroup.ioutil.WriteFile(memoryHierarchyPath, enableMemoryHierarchy, rootReadOnlyPermissions)
	if err != nil {
		return errors.Wrapf(err, "cgroup resource [%s]: setup cgroup: unable to set use hierarchy flag", cgroup.taskARN)
	}

	return nil
}

// Cleanup removes the cgroup root created for the task
func (cgroup *CgroupResource) Cleanup() error {
	err := cgroup.control.Remove(cgroup.cgroupRoot)
	// Explicitly handle cgroup deleted error
	if err != nil {
		if err == cgroups.ErrCgroupDeleted {
			seelog.Warnf("Cgroup at %s has already been removed: %v", cgroup.cgroupRoot, err)
			return nil
		}
		return errors.Wrapf(err, "resource: cleanup cgroup: unable to remove cgroup at %s", cgroup.cgroupRoot)
	}
	return nil
}

// cgroupResourceJSON duplicates CgroupResource fields, only for marshalling and unmarshalling purposes
type cgroupResourceJSON struct {
	CgroupRoot      string               `json:"cgroupRoot"`
	CgroupMountPath string               `json:"cgroupMountPath"`
	CreatedAt       time.Time            `json:"createdAt,omitempty"`
	DesiredStatus   *CgroupStatus        `json:"desiredStatus"`
	KnownStatus     *CgroupStatus        `json:"knownStatus"`
	LinuxSpec       specs.LinuxResources `json:"resourceSpec"`
}

// MarshalJSON marshals CgroupResource object using duplicate struct CgroupResourceJSON
func (cgroup *CgroupResource) MarshalJSON() ([]byte, error) {
	if cgroup == nil {
		return nil, errors.New("cgroup resource is nil")
	}
	return json.Marshal(cgroupResourceJSON{
		cgroup.cgroupRoot,
		cgroup.cgroupMountPath,
		cgroup.GetCreatedAt(),
		func() *CgroupStatus {
			desiredState := cgroup.GetDesiredStatus()
			status := CgroupStatus(desiredState)
			return &status
		}(),
		func() *CgroupStatus {
			knownState := cgroup.GetKnownStatus()
			status := CgroupStatus(knownState)
			return &status
		}(),
		cgroup.resourceSpec,
	})
}

// UnmarshalJSON unmarshals CgroupResource object using duplicate struct CgroupResourceJSON
func (cgroup *CgroupResource) UnmarshalJSON(b []byte) error {
	temp := cgroupResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	cgroup.cgroupRoot = temp.CgroupRoot
	cgroup.cgroupMountPath = temp.CgroupMountPath
	cgroup.resourceSpec = temp.LinuxSpec
	if temp.DesiredStatus != nil {
		cgroup.SetDesiredStatus(taskresource.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		cgroup.SetKnownStatus(taskresource.ResourceStatus(*temp.KnownStatus))
	}
	return nil
}

// GetCgroupRoot returns cgroup root of the resource
func (cgroup *CgroupResource) GetCgroupRoot() string {
	cgroup.lock.RLock()
	defer cgroup.lock.RUnlock()
	return cgroup.cgroupRoot
}

// GetCgroupMountPath returns cgroup mount path of the resource
func (cgroup *CgroupResource) GetCgroupMountPath() string {
	cgroup.lock.RLock()
	defer cgroup.lock.RUnlock()
	return cgroup.cgroupMountPath
}

// Initialize initializes the resource fileds in cgroup
func (cgroup *CgroupResource) Initialize(resourceFields *taskresource.ResourceFields) {
	cgroup.lock.Lock()
	defer cgroup.lock.Unlock()

	cgroup.initializeResourceStatusToTransitionFunction()
	cgroup.ioutil = resourceFields.IOUtil
	cgroup.control = resourceFields.Control
}
