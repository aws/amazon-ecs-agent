// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package resources

import (
	"os"
	"path/filepath"
	"strconv"

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

// cgroupWrapper implements the Resource interface
type cgroupWrapper struct {
	control    cgroup.Control
	ioutil     ioutilwrapper.IOUtil
	cgroupPath string
}

// New is used to return an object that implements the Resource interface
func New() Resource {
	return newResources(cgroup.New(), ioutilwrapper.NewIOUtil())
}

func newResources(control cgroup.Control, ioutil ioutilwrapper.IOUtil) Resource {
	return &cgroupWrapper{
		control: control,
		ioutil:  ioutil,
	}
}

// Init is used to initialize the resource
func (c *cgroupWrapper) Init() error {
	return c.cgroupInit()
}

// Setup sets up the resource
func (c *cgroupWrapper) Setup(task *api.Task) error {
	return c.setupCgroup(task)
}

// Cleanup removes the resource
func (c *cgroupWrapper) Cleanup(task *api.Task) error {
	return c.cleanupCgroup(task)
}

func (c *cgroupWrapper) ApplyConfigDependencies(cfg *config.Config) {
	c.cgroupPath = cfg.CgroupPath
}

// cgroupInit is used to create the root '/ecs/ cgroup
func (c *cgroupWrapper) cgroupInit() error {
	if c.control.Exists(config.DefaultTaskCgroupPrefix) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", config.DefaultTaskCgroupPrefix)
		return nil
	}

	return c.control.Init()
}

// setupCgroup is used to create the task cgroup
// and enable memory.use_hierarchy at the '/ecs/<task-id>' level
func (c *cgroupWrapper) setupCgroup(task *api.Task) error {
	cgroupRoot, err := task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrapf(err, "resource: setup cgroup: unable to determine cgroup root for task: %s", task.Arn)
	}

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
	memoryHierarchyPath := filepath.Join(c.cgroupPath, memorySubsystem, cgroupRoot, memoryUseHierarchy)
	err = c.ioutil.WriteFile(memoryHierarchyPath, enableMemoryHierarchy, rootReadOnlyPermissions)
	if err != nil {
		return errors.Wrapf(err, "resource: setup cgroup: unable to set use hierarchy flag")
	}

	return nil
}

// cleanupCgroup is used to remove the task cgroup
func (c *cgroupWrapper) cleanupCgroup(task *api.Task) error {
	cgroupRoot, err := task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrapf(err, "resource: cleanup cgroup: unable to determine cgroup root for task: %s", task.Arn)
	}

	seelog.Debugf("Cleaning up cgroup at: %s for task: %s", cgroupRoot, task.Arn)

	err = c.control.Remove(cgroupRoot)
	// Explicitly handle cgroup deleted error
	if err != nil {
		if err == cgroups.ErrCgroupDeleted {
			seelog.Warnf("Cgroup at %s has already been removed for task %s: %v", cgroupRoot, task.Arn, err)
			return nil
		}
		return errors.Wrapf(err, "resource: cleanup cgroup: unable to remove cgroup at %s for task: %s", cgroupRoot, task.Arn)
	}
	return nil
}
