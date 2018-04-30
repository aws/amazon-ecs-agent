// +build windows

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
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

const (
	//memorySwappinessDefault is the expected default value for this platform
	memorySwappinessDefault = -1
	// cpuSharesPerCore represents the cpu shares of a cpu core in docker
	cpuSharesPerCore  = 1024
	percentageFactor  = 100
	minimumCPUPercent = 1
)

// platformFields consists of fields specific to Windows for a task
type platformFields struct {
	// cpuUnbounded determines whether a mix of unbounded and bounded CPU tasks
	// are allowed to run in the instance
	cpuUnbounded bool
}

var cpuShareScaleFactor = runtime.NumCPU() * cpuSharesPerCore

// adjustForPlatform makes Windows-specific changes to the task after unmarshal
func (task *Task) adjustForPlatform(cfg *config.Config) {
	task.downcaseAllVolumePaths()
	platformFields := platformFields{
		cpuUnbounded: cfg.PlatformVariables.CPUUnbounded,
	}
	task.platformFields = platformFields
}

// downcaseAllVolumePaths forces all volume paths (host path and container path)
// to be lower-case.  This is to account for Windows case-insensitivity and the
// case-sensitive string comparison that takes place elsewhere in the code.
func (task *Task) downcaseAllVolumePaths() {
	for _, volume := range task.Volumes {
		if hostVol, ok := volume.Volume.(*FSHostVolume); ok {
			hostVol.FSSourcePath = getCanonicalPath(hostVol.FSSourcePath)
		}
	}
	for _, container := range task.Containers {
		for i, mountPoint := range container.MountPoints {
			// container.MountPoints is a slice of values, not a slice of pointers so
			// we need to mutate the actual value instead of the copied value
			container.MountPoints[i].ContainerPath = getCanonicalPath(mountPoint.ContainerPath)
		}
	}
}

func getCanonicalPath(path string) string {
	return filepath.Clean(strings.ToLower(path))
}

// platformHostConfigOverride provides an entry point to set up default HostConfig options to be
// passed to Docker API.
func (task *Task) platformHostConfigOverride(hostConfig *docker.HostConfig) error {
	task.overrideDefaultMemorySwappiness(hostConfig)
	// Convert the CPUShares to CPUPercent
	hostConfig.CPUPercent = hostConfig.CPUShares * percentageFactor / int64(cpuShareScaleFactor)
	if hostConfig.CPUPercent == 0 && hostConfig.CPUShares != 0 {
		// if CPU percent is too low, we set it to the minimum(linux and some windows tasks).
		// if the CPU is explicitly set to zero or not set at all, and CPU unbounded
		// tasks are allowed for windows, let CPU percent be zero.
		// this is a workaround to allow CPU unbounded tasks(https://github.com/aws/amazon-ecs-agent/issues/1127)
		hostConfig.CPUPercent = minimumCPUPercent
	}
	hostConfig.CPUShares = 0
	return nil
}

// overrideDefaultMemorySwappiness Overrides the value of MemorySwappiness to -1
// Version 1.12.x of Docker for Windows would ignore the unsupported option MemorySwappiness.
// Version 17.03.x will cause an error if any value other than -1 is passed in for MemorySwappiness.
// This bug is not noticed when no value is passed in. However, the go-dockerclient client version
// we are using removed the json option omitempty causing this parameter to default to 0 if empty.
// https://github.com/fsouza/go-dockerclient/commit/72342f96fabfa614a94b6ca57d987eccb8a836bf
func (task *Task) overrideDefaultMemorySwappiness(hostConfig *docker.HostConfig) {
	hostConfig.MemorySwappiness = memorySwappinessDefault
}

// dockerCPUShares converts containerCPU shares if needed as per the logic stated below:
// Docker silently converts 0 to 1024 CPU shares, which is probably not what we
// want.  Instead, we convert 0 to 2 to be closer to expected behavior. The
// reason for 2 over 1 is that 1 is an invalid value (Linux's choice, not Docker's).
func (task *Task) dockerCPUShares(containerCPU uint) int64 {
	if containerCPU <= 1 && !task.platformFields.cpuUnbounded {
		seelog.Debugf(
			"Converting CPU shares to allowed minimum of 2 for task arn: [%s] and cpu shares: %d",
			task.Arn, containerCPU)
		return 2
	}
	return int64(containerCPU)
}

func (task *Task) initializeCgroupResourceSpec(cgroupPath string, resourceFields *taskresource.ResourceFields) error {
	return errors.New("unsupported platform")
}
