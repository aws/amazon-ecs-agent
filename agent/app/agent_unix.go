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

package app

import (
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup"
	"github.com/cihub/seelog"
)

// control is here to help create the cgroup root '/ecs' when
// the feature is enabled. It also aids in unit testing
var control cgroup.Control

// initPlatformResources initializes platform resource controllers
// Currently it helps initialize cgroup
func (agent *ecsAgent) initPlatformResources() {
	if agent.cfg.TaskCPUMemLimit {
		setControl(cgroup.New())
	}
}

// setupPlatformResources helps set up the platform resource controllers
// Currently used to setup '/ecs' cgroup
func (agent *ecsAgent) setupPlatformResources() error {
	if agent.cfg.TaskCPUMemLimit {
		return setupTaskCgroupPrefix()
	}
	return nil
}

// setupTaskCgroupPrefix creates the '/ecs' cgroup
func setupTaskCgroupPrefix() error {
	if control.Exists(config.DefaultTaskCgroupPrefix) {
		seelog.Debugf("Cgroup at %s already exists, skipping creation", config.DefaultTaskCgroupPrefix)
		return nil
	}
	return control.Init()
}

// setControl sets the cgroup controller
func setControl(controller cgroup.Control) {
	control = controller
}
