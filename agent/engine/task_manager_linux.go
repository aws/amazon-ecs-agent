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

package engine

import (
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// control is used to manipulate cgroups and ease testing
var control cgroup.Control

// SetupPlatformResources sets up platform level resources
func (mtask *managedTask) SetupPlatformResources() error {
	if mtask.Task.CgroupEnabled() {
		setControl(cgroup.New())
		return mtask.setupCgroup()
	}
	return nil
}

// CleanupPlatformResources cleans up platform level resources
func (mtask *managedTask) CleanupPlatformResources() error {
	if mtask.Task.CgroupEnabled() {
		return mtask.cleanupCgroup()
	}
	return nil
}

// setupCgroup sets up the cgroup for each managed task
func (mtask *managedTask) setupCgroup() error {
	cgroupRoot, err := mtask.Task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrap(err, "mtask setup cgroup: unable to determine cgroup root")
	}

	seelog.Debugf("Setting up cgroup at: %s", cgroupRoot)

	if control.Exists(cgroupRoot) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", cgroupRoot)
		return nil
	}

	linuxResourceSpec := mtask.Task.BuildLinuxResourceSpec()

	cgroupSpec := cgroup.Spec{
		Root:  cgroupRoot,
		Specs: &linuxResourceSpec,
	}

	cgrp, err := control.Create(&cgroupSpec)
	if err != nil {
		return errors.Wrapf(err, "mtask setup cgroup: unable to create cgroup:  %s", cgroupRoot)
	}

	// NOTE: This should be impossible
	if cgrp == nil {
		seelog.Criticalf("Invalid cgroup creation at %s", cgroupRoot)
		return errors.New("mtask setup cgroup: invalid cgroup object")
	}

	return nil
}

// cleanupCgroup removes the task cgroup
func (mtask *managedTask) cleanupCgroup() error {
	cgroupRoot, err := mtask.Task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrap(err, "mtask cleanup cgroup: unable to determine cgroup root")
	}

	seelog.Debugf("Cleaning up cgroup at: %s", cgroupRoot)

	err = control.Remove(cgroupRoot)
	if err != nil {
		return errors.Wrapf(err, "mtask cleanup cgroup: unable to remove cgroup: %s", cgroupRoot)
	}

	return nil
}

// setControl is used to set the cgroup controller
func setControl(controller cgroup.Control) {
	control = controller
}
