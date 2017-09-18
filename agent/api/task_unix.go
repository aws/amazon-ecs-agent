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
	"path/filepath"
	"time"

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

	defaultCPUPeriod = 100 * time.Millisecond // 100ms
	// With a 100ms CPU period, we can express 0.01 vCPU to 10 vCPUs
	maxTaskVCPULimit = 10
	// Reference: http://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_ContainerDefinition.html
	minimumCPUShare = 2
)

func (task *Task) adjustForPlatform(cfg *config.Config) {
	task.memoryCPULimitsEnabledLock.Lock()
	defer task.memoryCPULimitsEnabledLock.Unlock()
	task.memoryCPULimitsEnabled = cfg.TaskCPUMemLimit
}

func getCanonicalPath(path string) string { return path }

// BuildCgroupRoot helps build the task cgroup prefix
// Example: /ecs/task-id
func (task *Task) BuildCgroupRoot() (string, error) {
	taskID, err := task.GetID()
	if err != nil {
		return "", errors.Wrapf(err, "task build cgroup root: unable to get task-id from taskARN: %s", task.Arn)
	}

	return filepath.Join(config.DefaultTaskCgroupPrefix, taskID), nil
}

// BuildLinuxResourceSpec returns a linuxResources object for the task cgroup
func (task *Task) BuildLinuxResourceSpec() (specs.LinuxResources, error) {
	linuxResourceSpec := specs.LinuxResources{}

	// If task level CPU limits are requested, set CPU quota + CPU period
	if !utils.ZeroOrNil(task.VCPULimit) {
		if task.VCPULimit > maxTaskVCPULimit || task.VCPULimit < 0 {
			return specs.LinuxResources{},
				errors.Errorf("task resource spec builder: unsupported CPU limits, requested=%f, max-supported=%d",
					task.VCPULimit, maxTaskVCPULimit)
		}
		taskCPUPeriod := uint64(defaultCPUPeriod / time.Microsecond)
		taskCPUQuota := int64(task.VCPULimit * float64(taskCPUPeriod))

		// TODO: DefaultCPUPeriod only permits 10VCPUs.
		// Adaptive calculation of CPUPeriod required for further support
		linuxResourceSpec.CPU = &specs.LinuxCPU{
			Quota:  &taskCPUQuota,
			Period: &taskCPUPeriod,
		}
	} else {
		// If task level CPU limits are missing,
		// aggregate container CPU shares when present
		var taskCPUShares uint64
		for _, container := range task.Containers {
			if !utils.ZeroOrNil(container.CPU) {
				taskCPUShares += uint64(container.CPU)
			}
		}

		// If there are are no CPU limits at task or container level,
		// default task CPU shares
		if utils.ZeroOrNil(taskCPUShares) {
			// Set default CPU shares
			taskCPUShares = minimumCPUShare
		}

		linuxResourceSpec.CPU = &specs.LinuxCPU{
			Shares: &taskCPUShares,
		}
	}

	// If task memory limit is not present, cgroup parent memory is not set
	// If task memory limit is set, ensure that no container
	// of this task has a greater request
	if !utils.ZeroOrNil(task.MemoryLimit) {
		taskMemoryLimit := int64(task.MemoryLimit)
		for _, container := range task.Containers {
			containerMemoryLimit := int64(container.Memory)
			if !utils.ZeroOrNil(containerMemoryLimit) && containerMemoryLimit > taskMemoryLimit {
				return specs.LinuxResources{},
					errors.Errorf("task resource spec builder: container memory limit(%d) greater than task memory limit(%d)",
						containerMemoryLimit, taskMemoryLimit)
			}
		}
		linuxResourceSpec.Memory = &specs.LinuxMemory{
			Limit: &taskMemoryLimit,
		}
	}

	return linuxResourceSpec, nil
}

// platformHostConfigOverride to override platform specific feature sets
func (task *Task) platformHostConfigOverride(hostConfig *docker.HostConfig) error {
	// Override cgroup parent
	return task.overrideCgroupParent(hostConfig)
}

// overrideCgroupParent updates hostconfig with cgroup parent when task cgroups
// are enabled
func (task *Task) overrideCgroupParent(hostConfig *docker.HostConfig) error {
	task.memoryCPULimitsEnabledLock.RLock()
	defer task.memoryCPULimitsEnabledLock.RUnlock()
	if task.memoryCPULimitsEnabled {
		cgroupRoot, err := task.BuildCgroupRoot()
		if err != nil {
			seelog.Debugf("Unable to obtain task cgroup root for task %s: %v", task.Arn, err)
			return errors.Wrapf(err, "task cgroup override: unable to obtain cgroup root for task: %s", task.Arn)
		}
		hostConfig.CgroupParent = cgroupRoot
	}
	return nil
}
