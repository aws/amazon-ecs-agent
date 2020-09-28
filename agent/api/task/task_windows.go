// +build windows

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
	"errors"
	"runtime"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/fsxwindowsfileserver"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	resourcetype "github.com/aws/amazon-ecs-agent/agent/taskresource/types"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	// cpuSharesPerCore represents the cpu shares of a cpu core in docker
	cpuSharesPerCore  = 1024
	percentageFactor  = 100
	minimumCPUPercent = 1
)

// PlatformFields consists of fields specific to Windows for a task
type PlatformFields struct {
	// CpuUnbounded determines whether a mix of unbounded and bounded CPU tasks
	// are allowed to run in the instance
	CpuUnbounded config.BooleanDefaultFalse `json:"cpuUnbounded"`
	// MemoryUnbounded determines whether a mix of unbounded and bounded Memory tasks
	// are allowed to run in the instance
	MemoryUnbounded config.BooleanDefaultFalse `json:"memoryUnbounded"`
}

var cpuShareScaleFactor = runtime.NumCPU() * cpuSharesPerCore

// adjustForPlatform makes Windows-specific changes to the task after unmarshal
func (task *Task) adjustForPlatform(cfg *config.Config) {
	task.downcaseAllVolumePaths()
	platformFields := PlatformFields{
		CpuUnbounded:    cfg.PlatformVariables.CPUUnbounded,
		MemoryUnbounded: cfg.PlatformVariables.MemoryUnbounded,
	}
	task.PlatformFields = platformFields
}

// downcaseAllVolumePaths forces all volume paths (host path and container path)
// to be lower-case.  This is to account for Windows case-insensitivity and the
// case-sensitive string comparison that takes place elsewhere in the code.
func (task *Task) downcaseAllVolumePaths() {
	for _, volume := range task.Volumes {
		if hostVol, ok := volume.Volume.(*taskresourcevolume.FSHostVolume); ok {
			hostVol.FSSourcePath = utils.GetCanonicalPath(hostVol.FSSourcePath)
		}
	}
	for _, container := range task.Containers {
		for i, mountPoint := range container.MountPoints {
			// container.MountPoints is a slice of values, not a slice of pointers so
			// we need to mutate the actual value instead of the copied value
			container.MountPoints[i].ContainerPath = utils.GetCanonicalPath(mountPoint.ContainerPath)
		}
	}
}

// platformHostConfigOverride provides an entry point to set up default HostConfig options to be
// passed to Docker API.
func (task *Task) platformHostConfigOverride(hostConfig *dockercontainer.HostConfig) error {
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

	if hostConfig.Memory <= 0 && task.PlatformFields.MemoryUnbounded.Enabled() {
		// As of version  17.06.2-ee-6 of docker. MemoryReservation is not supported on windows. This ensures that
		// this parameter is not passed, allowing to launch a container without a hard limit.
		hostConfig.MemoryReservation = 0
	}

	return nil
}

// dockerCPUShares converts containerCPU shares if needed as per the logic stated below:
// Docker silently converts 0 to 1024 CPU shares, which is probably not what we
// want.  Instead, we convert 0 to 2 to be closer to expected behavior. The
// reason for 2 over 1 is that 1 is an invalid value (Linux's choice, not Docker's).
func (task *Task) dockerCPUShares(containerCPU uint) int64 {
	if containerCPU <= 1 && !task.PlatformFields.CpuUnbounded.Enabled() {
		seelog.Debugf(
			"Converting CPU shares to allowed minimum of 2 for task arn: [%s] and cpu shares: %d",
			task.Arn, containerCPU)
		return 2
	}
	return int64(containerCPU)
}

func (task *Task) initializeCgroupResourceSpec(cgroupPath string, cGroupCPUPeriod time.Duration, resourceFields *taskresource.ResourceFields) error {
	return errors.New("unsupported platform")
}

// requiresCredentialSpecResource returns true if at least one container in the task
// needs a valid credentialspec resource
func (task *Task) requiresCredentialSpecResource() bool {
	for _, container := range task.Containers {
		if container.RequiresCredentialSpec() {
			return true
		}
	}
	return false
}

// initializeCredentialSpecResource builds the resource dependency map for the credentialspec resource
func (task *Task) initializeCredentialSpecResource(config *config.Config, credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields) error {
	credentialspecResource, err := credentialspec.NewCredentialSpecResource(task.Arn, config.AWSRegion, task.getAllCredentialSpecRequirements(),
		task.ExecutionCredentialsID, credentialsManager, resourceFields.SSMClientCreator, resourceFields.S3ClientCreator)
	if err != nil {
		return err
	}

	task.AddResource(credentialspec.ResourceName, credentialspecResource)

	// for every container that needs credential spec vending, it needs to wait for all credential spec resources
	for _, container := range task.Containers {
		if container.RequiresCredentialSpec() {
			container.BuildResourceDependency(credentialspecResource.GetName(),
				resourcestatus.ResourceStatus(credentialspec.CredentialSpecCreated),
				apicontainerstatus.ContainerCreated)
		}
	}

	return nil
}

// getAllCredentialSpecRequirements is used to build all the credential spec requirements for the task
func (task *Task) getAllCredentialSpecRequirements() []string {
	reqs := []string{}

	for _, container := range task.Containers {
		credentialSpec, err := container.GetCredentialSpec()
		if err == nil && credentialSpec != "" && !utils.StrSliceContains(reqs, credentialSpec) {
			reqs = append(reqs, credentialSpec)
		}
	}

	return reqs
}

// GetCredentialSpecResource retrieves credentialspec resource from resource map
func (task *Task) GetCredentialSpecResource() ([]taskresource.TaskResource, bool) {
	task.lock.RLock()
	defer task.lock.RUnlock()

	res, ok := task.ResourcesMapUnsafe[credentialspec.ResourceName]
	return res, ok
}

func enableIPv6SysctlSetting(hostConfig *dockercontainer.HostConfig) {
	return
}

// requiresFSxWindowsFileServerResource returns true if at least one volume in the task
// is of type 'fsxWindowsFileServer'
func (task *Task) requiresFSxWindowsFileServerResource() bool {
	for _, volume := range task.Volumes {
		if volume.Type == FSxWindowsFileServerVolumeType {
			return true
		}
	}
	return false
}

// initializeFSxWindowsFileServerResource builds the resource dependency map for the fsxwindowsfileserver resource
func (task *Task) initializeFSxWindowsFileServerResource(cfg *config.Config, credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields) error {
	for i, vol := range task.Volumes {
		if vol.Type != FSxWindowsFileServerVolumeType {
			continue
		}

		fsxWindowsFileServerVol, ok := vol.Volume.(*fsxwindowsfileserver.FSxWindowsFileServerVolumeConfig)
		if !ok {
			return errors.New("task volume: volume configuration does not match the type 'fsxWindowsFileServer'")
		}

		err := task.addFSxWindowsFileServerResource(cfg, credentialsManager, resourceFields, &task.Volumes[i], fsxWindowsFileServerVol)
		if err != nil {
			return err
		}
	}
	return nil
}

// addFSxWindowsFileServerResource creates a fsxwindowsfileserver resource for every fsxwindowsfileserver task volume
// and updates container dependency
func (task *Task) addFSxWindowsFileServerResource(
	cfg *config.Config,
	credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields,
	vol *TaskVolume,
	fsxWindowsFileServerVol *fsxwindowsfileserver.FSxWindowsFileServerVolumeConfig,
) error {
	fsxwindowsfileserverResource, err := fsxwindowsfileserver.NewFSxWindowsFileServerResource(
		task.Arn,
		cfg.AWSRegion,
		vol.Name,
		FSxWindowsFileServerVolumeType,
		fsxWindowsFileServerVol,
		task.ExecutionCredentialsID,
		credentialsManager,
		resourceFields.SSMClientCreator,
		resourceFields.ASMClientCreator,
		resourceFields.FSxClientCreator)
	if err != nil {
		return err
	}

	vol.Volume = &fsxwindowsfileserverResource.VolumeConfig
	task.AddResource(resourcetype.FSxWindowsFileServerKey, fsxwindowsfileserverResource)
	task.updateContainerVolumeDependency(vol.Name)

	return nil
}
