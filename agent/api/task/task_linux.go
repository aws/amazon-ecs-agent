// +build linux

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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	resourcetype "github.com/aws/amazon-ecs-agent/agent/taskresource/types"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

const (
	// With a 100ms CPU period, we can express 0.01 vCPU to 10 vCPUs
	maxTaskVCPULimit = 10
	// Reference: http://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_ContainerDefinition.html
	minimumCPUShare = 2

	minimumCPUPercent = 0
	bytesPerMegabyte  = 1024 * 1024

	internalExecCommandAgentNamePrefix          = "internal-execute-command-agent"
	internalExecCommandAgentLogVolumeNamePrefix = internalExecCommandAgentNamePrefix + "-log"
	internalExecCommandAgentCertVolumeName      = internalExecCommandAgentNamePrefix + "-tls-cert"

	// TODO: [ecs-exec] decide if this needs to be configurable or put in a specific place in our optimized AMIs
	execCommandAgentHostBinDir           = "/home/ec2-user/ssm-agent/linux_amd64"
	execCommandAgentBinName              = "amazon-ssm-agent"
	execCommandAgentSessionWorkerBinName = "ssm-session-worker"
	execCommandAgentSessionLoggerBinName = "ssm-session-logger"

	// TODO: [ecs-exec] decide if this needs to be configurable or put in a specific place in our optimized AMIs
	execCommandAgentContainerLogDir   = "/var/log/amazon/ssm"
	execCommandAgentContainerBinDir   = "/usr/bin"
	execCommandAgentHostCertFile      = "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
	execCommandAgentContainerCertFile = "/etc/ssl/certs/ca-certificates.crt"

	execCommandAgentNamelessContainerPrefix = "nameless-container-"
)

// PlatformFields consists of fields specific to Linux for a task
type PlatformFields struct{}

func (task *Task) adjustForPlatform(cfg *config.Config) {
	task.lock.Lock()
	defer task.lock.Unlock()
	task.MemoryCPULimitsEnabled = cfg.TaskCPUMemLimit.Enabled()
}

func (task *Task) initializeCgroupResourceSpec(cgroupPath string, cGroupCPUPeriod time.Duration, resourceFields *taskresource.ResourceFields) error {
	cgroupRoot, err := task.BuildCgroupRoot()
	if err != nil {
		return errors.Wrapf(err, "cgroup resource: unable to determine cgroup root for task")
	}
	resSpec, err := task.BuildLinuxResourceSpec(cGroupCPUPeriod)
	if err != nil {
		return errors.Wrapf(err, "cgroup resource: unable to build resource spec for task")
	}
	cgroupResource := cgroup.NewCgroupResource(task.Arn, resourceFields.Control,
		resourceFields.IOUtil, cgroupRoot, cgroupPath, resSpec)
	task.AddResource(resourcetype.CgroupKey, cgroupResource)
	for _, container := range task.Containers {
		container.BuildResourceDependency(cgroupResource.GetName(),
			resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			apicontainerstatus.ContainerPulled)
	}
	return nil
}

// BuildCgroupRoot helps build the task cgroup prefix
// Example: /ecs/task-id
func (task *Task) BuildCgroupRoot() (string, error) {
	taskID, err := task.GetID()
	if err != nil {
		return "", errors.Wrapf(err, "task build cgroup root: unable to get task-id from task ARN: %s", task.Arn)
	}

	return filepath.Join(config.DefaultTaskCgroupPrefix, taskID), nil
}

// BuildLinuxResourceSpec returns a linuxResources object for the task cgroup
func (task *Task) BuildLinuxResourceSpec(cGroupCPUPeriod time.Duration) (specs.LinuxResources, error) {
	linuxResourceSpec := specs.LinuxResources{}

	// If task level CPU limits are requested, set CPU quota + CPU period
	// Else set CPU shares
	if task.CPU > 0 {
		linuxCPUSpec, err := task.buildExplicitLinuxCPUSpec(cGroupCPUPeriod)
		if err != nil {
			return specs.LinuxResources{}, err
		}
		linuxResourceSpec.CPU = &linuxCPUSpec
	} else {
		linuxCPUSpec := task.buildImplicitLinuxCPUSpec()
		linuxResourceSpec.CPU = &linuxCPUSpec
	}

	// Validate and build task memory spec
	// NOTE: task memory specifications are optional
	if task.Memory > 0 {
		linuxMemorySpec, err := task.buildLinuxMemorySpec()
		if err != nil {
			return specs.LinuxResources{}, err
		}
		linuxResourceSpec.Memory = &linuxMemorySpec
	}

	return linuxResourceSpec, nil
}

// buildExplicitLinuxCPUSpec builds CPU spec when task CPU limits are
// explicitly requested
func (task *Task) buildExplicitLinuxCPUSpec(cGroupCPUPeriod time.Duration) (specs.LinuxCPU, error) {
	if task.CPU > maxTaskVCPULimit {
		return specs.LinuxCPU{},
			errors.Errorf("task CPU spec builder: unsupported CPU limits, requested=%f, max-supported=%d",
				task.CPU, maxTaskVCPULimit)
	}
	taskCPUPeriod := uint64(cGroupCPUPeriod / time.Microsecond)
	taskCPUQuota := int64(task.CPU * float64(taskCPUPeriod))

	// TODO: DefaultCPUPeriod only permits 10VCPUs.
	// Adaptive calculation of CPUPeriod required for further support
	// (samuelkarp) The largest available EC2 instance in terms of CPU count is a x1.32xlarge,
	// with 128 vCPUs. If we assume a fixed evaluation period of 100ms (100000us),
	// we'd need a quota of 12800000us, which is longer than the maximum of 1000000.
	// For 128 vCPUs, we'd probably need something like a 1ms (1000us - the minimum)
	// evaluation period, an 128000us quota in order to stay within the min/max limits.
	return specs.LinuxCPU{
		Quota:  &taskCPUQuota,
		Period: &taskCPUPeriod,
	}, nil
}

// buildImplicitLinuxCPUSpec builds the implicit task CPU spec when
// task CPU and memory limit feature is enabled
func (task *Task) buildImplicitLinuxCPUSpec() specs.LinuxCPU {
	// If task level CPU limits are missing,
	// aggregate container CPU shares when present
	var taskCPUShares uint64
	for _, container := range task.Containers {
		if container.CPU > 0 {
			taskCPUShares += uint64(container.CPU)
		}
	}

	// If there are are no CPU limits at task or container level,
	// default task CPU shares
	if taskCPUShares == 0 {
		// Set default CPU shares
		taskCPUShares = minimumCPUShare
	}

	return specs.LinuxCPU{
		Shares: &taskCPUShares,
	}
}

// buildLinuxMemorySpec validates and builds the task memory spec
func (task *Task) buildLinuxMemorySpec() (specs.LinuxMemory, error) {
	// If task memory limit is not present, cgroup parent memory is not set
	// If task memory limit is set, ensure that no container
	// of this task has a greater request
	for _, container := range task.Containers {
		containerMemoryLimit := int64(container.Memory)
		if containerMemoryLimit > task.Memory {
			return specs.LinuxMemory{},
				errors.Errorf("task memory spec builder: container memory limit(%d) greater than task memory limit(%d)",
					containerMemoryLimit, task.Memory)
		}
	}

	// Kernel expects memory to be expressed in bytes
	memoryBytes := task.Memory * bytesPerMegabyte
	return specs.LinuxMemory{
		Limit: &memoryBytes,
	}, nil
}

// platformHostConfigOverride to override platform specific feature sets
func (task *Task) platformHostConfigOverride(hostConfig *dockercontainer.HostConfig) error {
	// Override cgroup parent
	return task.overrideCgroupParent(hostConfig)
}

// overrideCgroupParent updates hostconfig with cgroup parent when task cgroups
// are enabled
func (task *Task) overrideCgroupParent(hostConfig *dockercontainer.HostConfig) error {
	task.lock.RLock()
	defer task.lock.RUnlock()
	if task.MemoryCPULimitsEnabled {
		cgroupRoot, err := task.BuildCgroupRoot()
		if err != nil {
			return errors.Wrapf(err, "task cgroup override: unable to obtain cgroup root for task: %s", task.Arn)
		}
		hostConfig.CgroupParent = cgroupRoot
	}
	return nil
}

// dockerCPUShares converts containerCPU shares if needed as per the logic stated below:
// Docker silently converts 0 to 1024 CPU shares, which is probably not what we
// want.  Instead, we convert 0 to 2 to be closer to expected behavior. The
// reason for 2 over 1 is that 1 is an invalid value (Linux's choice, not Docker's).
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

// initializeExecCommandAgentResources specifies the necessary volumes and mount points in all of the task containers in order for the
// exec agent to run upon container start.
func (task *Task) initializeExecCommandAgentResources(cfg *config.Config) error {
	if !task.IsExecCommandAgentEnabled() {
		return nil
	}

	tId, err := task.GetID()
	if err != nil {
		return err
	}

	execCommandAgentBinNames := []string{execCommandAgentBinName, execCommandAgentSessionWorkerBinName, execCommandAgentSessionLoggerBinName}

	// Append an internal volume for each of the exec agent binary names
	for _, bn := range execCommandAgentBinNames {
		task.Volumes = append(task.Volumes,
			TaskVolume{
				Type: HostVolumeType,
				Name: buildVolumeNameForExecCommandAgentBinary(bn),
				Volume: &taskresourcevolume.FSHostVolume{
					FSSourcePath: filepath.Join(execCommandAgentHostBinDir, bn),
				},
			})
	}

	// Append certificates volume
	task.Volumes = append(task.Volumes,
		TaskVolume{
			Type: HostVolumeType,
			Name: internalExecCommandAgentCertVolumeName,
			Volume: &taskresourcevolume.FSHostVolume{
				FSSourcePath: execCommandAgentHostCertFile,
			},
		})

	// Add log volumes and mount points to all containers in this task
	for _, c := range task.Containers {
		lvn := fmt.Sprintf("%s-%s-%s", internalExecCommandAgentLogVolumeNamePrefix, tId, c.Name)
		cn := buildContainerNameForExecCommandAgentBinary(c)
		task.Volumes = append(task.Volumes, TaskVolume{
			Type: HostVolumeType,
			Name: lvn,
			Volume: &taskresourcevolume.FSHostVolume{
				FSSourcePath: filepath.Join(filepath.Dir(os.Getenv(logger.LOGFILE_ENV_VAR)), tId, cn),
			},
		})

		c.MountPoints = append(c.MountPoints,
			apicontainer.MountPoint{
				SourceVolume:  lvn,
				ContainerPath: execCommandAgentContainerLogDir,
				ReadOnly:      false,
			},
			apicontainer.MountPoint{
				SourceVolume:  internalExecCommandAgentCertVolumeName,
				ContainerPath: execCommandAgentContainerCertFile,
				ReadOnly:      true,
			},
		)

		for _, bn := range execCommandAgentBinNames {
			c.MountPoints = append(c.MountPoints,
				apicontainer.MountPoint{
					SourceVolume:  buildVolumeNameForExecCommandAgentBinary(bn),
					ContainerPath: filepath.Join(execCommandAgentContainerBinDir, bn),
					ReadOnly:      true,
				})
		}
	}
	return nil
}

func buildVolumeNameForExecCommandAgentBinary(binaryName string) string {
	return fmt.Sprintf("%s-%s", internalExecCommandAgentNamePrefix, binaryName)
}

func buildContainerNameForExecCommandAgentBinary(c *apicontainer.Container) string {
	// Trim leading hyphens since they're not valid directory names
	cn := strings.TrimLeft(c.Name, "-")
	if cn == "" {
		// Fallback name in the extreme case that we end up with an empty string after trimming all leading hyphens.
		return execCommandAgentNamelessContainerPrefix + uuid.New()
	}
	return cn
}

func enableIPv6SysctlSetting(hostConfig *dockercontainer.HostConfig) {
	if hostConfig.Sysctls == nil {
		hostConfig.Sysctls = make(map[string]string)
	}
	hostConfig.Sysctls[disableIPv6SysctlKey] = sysctlValueOff
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
