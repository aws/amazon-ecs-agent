// +build !linux,!windows

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package task

import (
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ecscni"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"
)

const (
	minimumCPUPercent = 0
)

// PlatformFields consists of fields specific to Linux for a task
type PlatformFields struct{}

func (task *Task) adjustForPlatform(cfg *config.Config) {
	task.lock.Lock()
	defer task.lock.Unlock()
	task.MemoryCPULimitsEnabled = cfg.TaskCPUMemLimit.Enabled()
}

func (task *Task) initializeCgroupResourceSpec(cgroupPath string, cGroupCPUPeriod time.Duration, resourceFields *taskresource.ResourceFields) error {
	return nil
}

func (task *Task) platformHostConfigOverride(hostConfig *dockercontainer.HostConfig) error {
	return nil
}

// duplicate of dockerCPUShares in task_linux.go
func (task *Task) dockerCPUShares(containerCPU uint) int64 {
	if containerCPU <= 1 {
		seelog.Debugf(
			"Converting CPU shares to allowed minimum of 2 for task arn: [%s] and cpu shares: %d",
			task.Arn, containerCPU)
		return 2
	}
	return int64(containerCPU)
}

// requiresCredentialSpecResource returns true if at least one container in the task
// needs a valid credentialspec resource
func (task *Task) requiresCredentialSpecResource() bool {
	return false
}

// initializeCredentialSpecResource builds the resource dependency map for the credentialspec resource
func (task *Task) initializeCredentialSpecResource(config *config.Config, credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields) error {
	return errors.New("task credentialspec is only supported on windows")
}

// GetCredentialSpecResource retrieves credentialspec resource from resource map
func (task *Task) GetCredentialSpecResource() ([]taskresource.TaskResource, bool) {
	return []taskresource.TaskResource{}, false
}

func enableIPv6SysctlSetting(hostConfig *dockercontainer.HostConfig) {
	return
}

// requiresFSxWindowsFileServerResource returns true if at least one volume in the task
// is of type 'fsxWindowsFileServer'
func (task *Task) requiresFSxWindowsFileServerResource() bool {
	return false
}

// initializeFSxWindowsFileServerResource builds the resource dependency map for the fsxwindowsfileserver resource
func (task *Task) initializeFSxWindowsFileServerResource(cfg *config.Config, credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields) error {
	return errors.New("task with FSx for Windows File Server volumes is only supported on Windows container instance")
}

// BuildCNIConfig builds the configuration for the CNI plugins
// On unsupported platforms, we will not support this functionality
func (task *Task) BuildCNIConfig(includeIPAMConfig bool, cniConfig *ecscni.Config) (*ecscni.Config, error) {
	return nil, errors.New("unsupported platform")
}
