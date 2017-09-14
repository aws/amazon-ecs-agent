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
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// cgroupWrapper implements the Resource interface
type cgroupWrapper struct {
	control cgroup.Control
}

// New is used to return an object that implements the Resource interface
func New() Resource {
	return newResources(cgroup.New())
}

func newResources(control cgroup.Control) Resource {
	return &cgroupWrapper{
		control: control,
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

// cgroupInit is used to create the root '/ecs/ cgroup
func (c *cgroupWrapper) cgroupInit() error {
	if c.control.Exists(config.DefaultTaskCgroupPrefix) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", config.DefaultTaskCgroupPrefix)
		return nil
	}
	return c.control.Init()
}

// setupCgroup is used to create the task cgroup
func (c *cgroupWrapper) setupCgroup(task *api.Task) error {
	cgroupRoot, err := task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrap(err, "resource: setup cgroup: unable to determine cgroup root")
	}

	seelog.Debugf("Setting up cgroup at: %s", cgroupRoot)

	if c.control.Exists(cgroupRoot) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", cgroupRoot)
		return nil
	}

	linuxResourceSpec, err := task.BuildLinuxResourceSpec()
	if err != nil {
		return errors.Wrap(err, "resource: setup cgroup: unable to build resource spec")
	}

	cgroupSpec := cgroup.Spec{
		Root:  cgroupRoot,
		Specs: &linuxResourceSpec,
	}

	cgrp, err := c.control.Create(&cgroupSpec)
	if err != nil {
		return errors.Wrapf(err, "resource: setup cgroup: unable to create cgroup at %s", cgroupRoot)
	}

	// NOTE: This should be impossible
	if cgrp == nil {
		seelog.Criticalf("Invalid cgroup creation at %s", cgroupRoot)
		return errors.New("resource: setup cgroup: invalid cgroup object")
	}

	return nil
}

// cleanupCgroup is used to remove the task cgroup
func (c *cgroupWrapper) cleanupCgroup(task *api.Task) error {
	cgroupRoot, err := task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrap(err, "resource: cleanup cgroup: unable to determine cgroup root")
	}

	seelog.Debugf("Cleaning up cgroup at: %s", cgroupRoot)

	err = c.control.Remove(cgroupRoot)
	if err != nil {
		return errors.Wrapf(err, "resource: cleanup cgroup: unable to remove cgroup at %s", cgroupRoot)
	}

	return nil
}
