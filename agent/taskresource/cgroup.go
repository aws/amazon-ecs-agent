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

package taskresource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/cihub/seelog"
	"github.com/containerd/cgroups"
	"github.com/pkg/errors"
)

const (
	memorySubsystem         = "/memory"
	memoryUseHierarchy      = "memory.use_hierarchy"
	rootReadOnlyPermissions = os.FileMode(400)
)

var (
	enableMemoryHierarchy = []byte(strconv.Itoa(1))
)

// CgroupResource represents Cgroup resource
type CgroupResource struct {
	control             cgroup.Control
	CgroupRoot          string
	CgroupMountPath     string
	ioutil              ioutilwrapper.IOUtil
	createdAt           time.Time
	desiredStatusUnsafe CgroupStatus
	knownStatusUnsafe   CgroupStatus
	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// NewCgroupResource is used to return an object that implements the Resource interface
func NewCgroupResource(cgroupRoot string, cgroupMountPath string) *CgroupResource {
	return &CgroupResource{
		// control creates interface to cgroups library methods
		control:         cgroup.New(),
		CgroupRoot:      cgroupRoot,
		CgroupMountPath: cgroupMountPath,
		ioutil:          ioutilwrapper.NewIOUtil(),
	}
}

// SetDesiredStatus safely sets the desired status of the resource
func (c *CgroupResource) SetDesiredStatus(status CgroupStatus) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (c *CgroupResource) GetDesiredStatus() CgroupStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.desiredStatusUnsafe
}

// SetKnownStatus safely sets the currently known status of the resource
func (c *CgroupResource) SetKnownStatus(status CgroupStatus) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.knownStatusUnsafe = status
}

// GetKnownStatus safely returns the currently known status of the task
func (c *CgroupResource) GetKnownStatus() CgroupStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.knownStatusUnsafe
}

// SetCreatedAt sets the timestamp for resource's creation time
func (c *CgroupResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	c.createdAt = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (c *CgroupResource) GetCreatedAt() time.Time {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.createdAt
}

func (c *CgroupResource) Create(task *api.Task) error {
	err := c.cgroupInit()
	if err != nil {
		seelog.Criticalf("unable to setup '/ecs' cgroup: %v", err)
		return err
	}
	err = c.setupTaskCgroup(task)
	if err != nil {
		seelog.Criticalf("unable to setup platform resources: %v", err)
		return err
	}
	return nil
}

func (c *CgroupResource) cgroupInit() error {
	if c.control.Exists(config.DefaultTaskCgroupPrefix) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", config.DefaultTaskCgroupPrefix)
		return nil
	}
	return c.control.Init()
}

func (c *CgroupResource) setupTaskCgroup(task *api.Task) error {
	cgroupRoot := c.CgroupRoot
	seelog.Debugf("Setting up cgroup at: %s for task: %s", cgroupRoot, task.Arn)

	if c.control.Exists(cgroupRoot) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", cgroupRoot)
		return nil
	}

	linuxResourceSpec, err := task.BuildLinuxResourceSpec()
	if err != nil {
		return errors.Wrapf(err, "resource: setup cgroup: unable to build resource spec for task: %s", task.Arn)
	}

	cgroupSpec := cgroup.Spec{
		Root:  cgroupRoot,
		Specs: &linuxResourceSpec,
	}

	_, err = c.control.Create(&cgroupSpec)
	if err != nil {
		return errors.Wrapf(err, "resource: setup cgroup: unable to create cgroup at %s for task: %s", cgroupRoot, task.Arn)
	}

	// echo 1 > memory.use_hierarchy
	memoryHierarchyPath := filepath.Join(c.CgroupMountPath, memorySubsystem, cgroupRoot, memoryUseHierarchy)
	err = c.ioutil.WriteFile(memoryHierarchyPath, enableMemoryHierarchy, rootReadOnlyPermissions)
	if err != nil {
		return errors.Wrap(err, "resource: setup cgroup: unable to set use hierarchy flag")
	}

	return nil
}

func (c *CgroupResource) Cleanup() error {
	err := c.control.Remove(c.CgroupRoot)
	// Explicitly handle cgroup deleted error
	if err != nil {
		if err == cgroups.ErrCgroupDeleted {
			seelog.Warnf("Cgroup at %s has already been removed: %v", c.CgroupRoot, err)
			return nil
		}
		return errors.Wrapf(err, "resource: cleanup cgroup: unable to remove cgroup at %s", c.CgroupRoot)
	}
	return nil
}

// cgroupResourceJSON duplicates CgroupResource fields, only for marshalling and unmarshalling purposes
type cgroupResourceJSON struct {
	CgroupRoot      string `json:"CgroupRoot"`
	CgroupMountPath string `json:"CgroupMountPath"`
	CreatedAt       time.Time
	DesiredStatus   *CgroupStatus `json:"DesiredStatus"`
	KnownStatus     *CgroupStatus `json:"KnownStatus"`
}

// MarshalJSON marshals CgroupResource object using duplicate struct CgroupResourceJSON
func (c *CgroupResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(cgroupResourceJSON{
		c.CgroupRoot,
		c.CgroupMountPath,
		c.GetCreatedAt(),
		func() *CgroupStatus { desiredState := c.GetDesiredStatus(); return &desiredState }(),
		func() *CgroupStatus { knownState := c.GetKnownStatus(); return &knownState }(),
	})
}

// UnmarshalJSON unmarshals CgroupResource object using duplicate struct CgroupResourceJSON
func (c *CgroupResource) UnmarshalJSON(b []byte) error {
	temp := &cgroupResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	c.CgroupRoot = temp.CgroupRoot
	c.CgroupMountPath = temp.CgroupMountPath
	if temp.DesiredStatus != nil {
		c.SetDesiredStatus(*temp.DesiredStatus)
	}
	if temp.KnownStatus != nil {
		c.SetKnownStatus(*temp.KnownStatus)
	}
	return nil
}
