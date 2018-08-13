// +build !linux,!windows

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

package task

import (
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

const (
	//memorySwappinessDefault is the expected default value for this platform. This is used in task_windows.go
	//and is maintained here for unix default. Also used for testing
	memorySwappinessDefault = 0

	defaultCPUPeriod = 100 * time.Millisecond // 100ms
	// With a 100ms CPU period, we can express 0.01 vCPU to 10 vCPUs
	maxTaskVCPULimit = 10
	// Reference: http://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_ContainerDefinition.html
	minimumCPUShare = 2

	minimumCPUPercent = 0
	bytesPerMegabyte  = 1024 * 1024
)

// platformFields consists of fields specific to Linux for a task
type platformFields struct{}

func (task *Task) adjustForPlatform(cfg *config.Config) {
	task.lock.Lock()
	defer task.lock.Unlock()
	task.MemoryCPULimitsEnabled = cfg.TaskCPUMemLimit.Enabled()
}

func getCanonicalPath(path string) string { return path }

func (task *Task) initializeCgroupResourceSpec(cgroupPath string, resourceFields *taskresource.ResourceFields) error {
	return nil
}

func (task *Task) platformHostConfigOverride(hostConfig *docker.HostConfig) error {
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
