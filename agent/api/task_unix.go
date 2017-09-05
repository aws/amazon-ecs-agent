// +build !windows

// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	portBindingHostIP = "0.0.0.0"

	//memorySwappinessDefault is the expected default value for this platform. This is used in task_windows.go
	//and is maintained here for unix default. Also used for testing
	memorySwappinessDefault = 0
	sepForwardSlash         = "/"
	defaultCPUPeriod        = 100000 // 100ms
)

func (task *Task) adjustForPlatform() {}

func getCanonicalPath(path string) string { return path }

// CgroupEnabled checks whether cgroups need be enabled at the task level
func (task *Task) CgroupEnabled() bool {
	// TODO: check for more conditions
	// CPU + Mem -> true
	// CPU -> true
	// Mem -> false
	// !CPU || !Mem -> false
	if !utils.ZeroOrNil(task.VCPULimit) {
		return true
	}
	return false
}

// BuildCgroupRoot helps build the task cgroup prefix
// Example: /ecs/task-id
func (task *Task) BuildCgroupRoot() (string, error) {
	taskID, err := task.GetID()
	if err != nil {
		return "", errors.Wrapf(err, "task build cgroup root: unable to get task-id")
	}

	cgroupRoot := strings.Join([]string{config.DefaultTaskCgroupPrefix, taskID}, sepForwardSlash)
	return cgroupRoot, nil
}

// BuildLinuxResourceSpec returns a linuxResources object for the task cgroup
func (task *Task) BuildLinuxResourceSpec() specs.LinuxResources {
	linuxResourceSpec := specs.LinuxResources{}

	if !(utils.ZeroOrNil(task.VCPULimit)) {
		taskCPUPeriod := uint64(defaultCPUPeriod)
		taskCPUQuota := int64(task.VCPULimit * defaultCPUPeriod)

		linuxResourceSpec.CPU = &specs.LinuxCPU{
			Quota:  &taskCPUQuota,
			Period: &taskCPUPeriod,
		}
	}

	if !(utils.ZeroOrNil(task.MemoryLimit)) {
		taskMemoryLimit := int64(task.MemoryLimit)
		linuxResourceSpec.Memory = &specs.LinuxMemory{
			Limit: &taskMemoryLimit,
		}
	}

	return linuxResourceSpec
}

// platformHostConfigOverride to override platform specific feature sets
func (task *Task) platformHostConfigOverride(hostConfig *docker.HostConfig) {
	// Override cgroup parent
	task.overrideCgroupParent(hostConfig)
}

// overrideCgroupParent updates hostconfig with cgroup parent when task cgroups
// are enabled
func (task *Task) overrideCgroupParent(hostConfig *docker.HostConfig) {
	if !task.CgroupEnabled() {
		return
	}

	cgroupRoot, err := task.BuildCgroupRoot()
	if err != nil {
		seelog.Debugf("Unable to obtain task cgroup root: %v", err)
		return
	}
	hostConfig.CgroupParent = cgroupRoot
}
