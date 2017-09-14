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

// resource to implement the Resources interface
type resources struct {
	control cgroup.Control
}

// New is used to return an object of the resources
func New() Resources {
	return newResources(cgroup.New())
}

func newResources(control cgroup.Control) Resources {
	return &resources{
		control: control,
	}
}

// Init is used to initialize the resources
func (r *resources) Init() error {
	return r.cgroupInit()
}

// Setup sets up the resources
func (r *resources) Setup(task *api.Task) error {
	return r.setupCgroup(task)
}

// Cleanup removes the resources
func (r *resources) Cleanup(task *api.Task) error {
	return r.cleanupCgroup(task)
}

// cgroupInit is used to create the root '/ecs/ cgroup
func (r *resources) cgroupInit() error {
	if r.control.Exists(config.DefaultTaskCgroupPrefix) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", config.DefaultTaskCgroupPrefix)
		return nil
	}
	return r.control.Init()
}

// setupCgroup is used to create the task cgroup
func (r *resources) setupCgroup(task *api.Task) error {
	cgroupRoot, err := task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrap(err, "resources: setup cgroup: unable to determine cgroup root")
	}

	seelog.Debugf("Setting up cgroup at: %s", cgroupRoot)

	if r.control.Exists(cgroupRoot) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", cgroupRoot)
		return nil
	}

	linuxResourceSpec, err := task.BuildLinuxResourceSpec()
	if err != nil {
		return errors.Wrap(err, "resources: setup cgroup: unable to build resource spec")
	}

	cgroupSpec := cgroup.Spec{
		Root:  cgroupRoot,
		Specs: &linuxResourceSpec,
	}

	cgrp, err := r.control.Create(&cgroupSpec)
	if err != nil {
		return errors.Wrapf(err, "resources: setup cgroup: unable to create cgroup at %s", cgroupRoot)
	}

	// NOTE: This should be impossible
	if cgrp == nil {
		seelog.Criticalf("Invalid cgroup creation at %s", cgroupRoot)
		return errors.New("resources: setup cgroup: invalid cgroup object")
	}

	return nil
}

// cleanupCgroup is used to remove the task cgroup
func (r *resources) cleanupCgroup(task *api.Task) error {
	cgroupRoot, err := task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrap(err, "resources: cleanup cgroup: unable to determine cgroup root")
	}

	seelog.Debugf("Cleaning up cgroup at: %s", cgroupRoot)

	err = r.control.Remove(cgroupRoot)
	if err != nil {
		return errors.Wrapf(err, "resources: cleanup cgroup: unable to remove cgroup at %s", cgroupRoot)
	}

	return nil
}
