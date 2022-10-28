//go:build linux
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
	"path/filepath"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"

	"github.com/aws/amazon-ecs-agent/agent/utils"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	resourcetype "github.com/aws/amazon-ecs-agent/agent/taskresource/types"
	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	dockercontainer "github.com/docker/docker/api/types/container"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	// Reference: http://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_ContainerDefinition.html
	minimumCPUShare = 2

	minimumCPUPercent = 0
	bytesPerMegabyte  = 1024 * 1024
)

// PlatformFields consists of fields specific to Linux for a task
type PlatformFields struct{}

func (task *Task) adjustForPlatform(cfg *config.Config) {
	task.lock.Lock()
	defer task.lock.Unlock()
	task.MemoryCPULimitsEnabled = cfg.TaskCPUMemLimit.Enabled()
}

func (task *Task) initializeCgroupResourceSpec(cgroupPath string, cGroupCPUPeriod time.Duration, resourceFields *taskresource.ResourceFields) error {
	if !task.MemoryCPULimitsEnabled {
		if task.CPU > 0 || task.Memory > 0 {
			// Client-side validation/warning if a task with task-level CPU/memory limits specified somehow lands on an instance
			// where agent does not support it. These limits will be ignored.
			logger.Warn("Ignoring task-level CPU/memory limits since agent does not support the TaskCPUMemLimits capability", logger.Fields{
				field.TaskID: task.GetID(),
			})
		}
		return nil
	}

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
// Example v1: /ecs/task-id
// Example v2: ecstasks-$TASKID.slice
func (task *Task) BuildCgroupRoot() (string, error) {
	taskID, err := utils.TaskIdFromArn(task.Arn)
	if err != nil {
		return "", err
	}

	if config.CgroupV2 {
		return buildCgroupV2Root(taskID), nil
	}
	return buildCgroupV1Root(taskID), nil
}

func buildCgroupV1Root(taskID string) string {
	return filepath.Join(config.DefaultTaskCgroupV1Prefix, taskID)
}

// buildCgroupV2Root creates a root cgroup using the systemd driver's special "-"
// character. The "-" specifies a parent slice, so tasks and their containers end up
// looking like this in the cgroup directory:
//
//	/sys/fs/cgroup/ecstasks.slice/
//	├── ecstasks-XXXXf406f70c4c678073ae96944fXXXX.slice
//	│   └── docker-XXXX7c6dc81f2e9a8bf1c566dc769733ccba594b3007dd289a0f50ad7923XXXX.scope
//	└── ecstasks-XXXX30467358463ab6bbba4e73afXXXX.slice
//	    └── docker-XXXX7ef4e942552437c96051356859c1df169f16e1cf9a9fc96fd30614e6XXXX.scope
func buildCgroupV2Root(taskID string) string {
	return fmt.Sprintf("%s-%s.slice", config.DefaultTaskCgroupV2Prefix, taskID)
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
	taskCPUPeriod := uint64(cGroupCPUPeriod / time.Microsecond)
	taskCPUQuota := int64(task.CPU * float64(taskCPUPeriod))

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
		if container.CPU < minimumCPUShare {
			taskCPUShares += minimumCPUShare
		} else {
			taskCPUShares += uint64(container.CPU)
		}
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

// initializeCredentialSpecResource builds the resource dependency map for the credentialspec resource
func (task *Task) initializeCredentialSpecResource(config *config.Config, credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields) error {
	//TBD: Add code to support gMSA on linux
	credspecContainerMapping := task.getAllCredentialSpecRequirements()
	seelog.Info(credspecContainerMapping)
	return errors.New("task credentialspec is only supported on windows")
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

// BuildCNIConfigAwsvpc builds a list of CNI network configurations for the task.
// If includeIPAMConfig is set to true, the list also includes the bridge IPAM configuration.
func (task *Task) BuildCNIConfigAwsvpc(includeIPAMConfig bool, cniConfig *ecscni.Config) (*ecscni.Config, error) {
	if !task.IsNetworkModeAWSVPC() {
		return nil, errors.New("task config: task network mode is not AWSVPC")
	}

	var netconf *libcni.NetworkConfig
	var ifName string
	var err error

	// Build a CNI network configuration for each ENI.
	for _, eni := range task.ENIs {
		switch eni.InterfaceAssociationProtocol {
		// If the association protocol is set to "default" or unset (to preserve backwards
		// compatibility), consider it a "standard" ENI attachment.
		case "", apieni.DefaultInterfaceAssociationProtocol:
			cniConfig.ID = eni.MacAddress
			ifName, netconf, err = ecscni.NewENINetworkConfig(eni, cniConfig)
		case apieni.VLANInterfaceAssociationProtocol:
			cniConfig.ID = eni.MacAddress
			ifName, netconf, err = ecscni.NewBranchENINetworkConfig(eni, cniConfig)
		default:
			err = errors.Errorf("task config: unknown interface association type: %s",
				eni.InterfaceAssociationProtocol)
		}

		if err != nil {
			return nil, err
		}

		cniConfig.NetworkConfigs = append(cniConfig.NetworkConfigs, &ecscni.NetworkConfig{
			IfName:           ifName,
			CNINetworkConfig: netconf,
		})
	}

	// Build the bridge CNI network configuration.
	// All AWSVPC tasks have a bridge network.
	ifName, netconf, err = ecscni.NewBridgeNetworkConfig(cniConfig, includeIPAMConfig)
	if err != nil {
		return nil, err
	}
	cniConfig.NetworkConfigs = append(cniConfig.NetworkConfigs, &ecscni.NetworkConfig{
		IfName:           ifName,
		CNINetworkConfig: netconf,
	})

	// Build a CNI network configuration for AppMesh if enabled.
	appMeshConfig := task.GetAppMesh()
	if appMeshConfig != nil {
		ifName, netconf, err = ecscni.NewAppMeshConfig(appMeshConfig, cniConfig)
		if err != nil {
			return nil, err
		}
		cniConfig.NetworkConfigs = append(cniConfig.NetworkConfigs, &ecscni.NetworkConfig{
			IfName:           ifName,
			CNINetworkConfig: netconf,
		})
	}

	// Build a CNI network configuration for ServiceConnect-enabled task in AWSVPC mode
	if task.IsServiceConnectEnabled() {
		ifName, netconf, err = ecscni.NewServiceConnectNetworkConfig(
			task.ServiceConnectConfig,
			ecscni.NAT,
			false,
			task.shouldEnableIPv4(),
			task.shouldEnableIPv6(),
			cniConfig)
		if err != nil {
			return nil, err
		}
		cniConfig.NetworkConfigs = append(cniConfig.NetworkConfigs, &ecscni.NetworkConfig{
			IfName:           ifName,
			CNINetworkConfig: netconf,
		})
	}

	cniConfig.ContainerNetNS = fmt.Sprintf(ecscni.NetnsFormat, cniConfig.ContainerPID)

	return cniConfig, nil
}

// BuildCNIConfigBridgeMode builds a list of CNI network configurations for a task in docker bridge mode.
// Currently the only plugin in available is for Service Connect
func (task *Task) BuildCNIConfigBridgeMode(cniConfig *ecscni.Config, containerName string) (*ecscni.Config, error) {
	if !task.IsNetworkModeBridge() || !task.IsServiceConnectEnabled() {
		return nil, errors.New("only bridge-mode Service-Connect-enabled task should invoke BuildCNIConfigBridgeMode")
	}

	var netconf *libcni.NetworkConfig
	var ifName string
	var err error

	scNetworkConfig := task.GetServiceConnectNetworkConfig()
	ifName, netconf, err = ecscni.NewServiceConnectNetworkConfig(
		task.ServiceConnectConfig,
		ecscni.TPROXY,
		!task.IsContainerServiceConnectPause(containerName),
		scNetworkConfig.SCPauseIPv4Addr != "",
		scNetworkConfig.SCPauseIPv6Addr != "",
		cniConfig)
	if err != nil {
		return nil, err
	}
	cniConfig.NetworkConfigs = append(cniConfig.NetworkConfigs, &ecscni.NetworkConfig{
		IfName:           ifName,
		CNINetworkConfig: netconf,
	})

	cniConfig.ContainerNetNS = fmt.Sprintf(ecscni.NetnsFormat, cniConfig.ContainerPID)
	return cniConfig, nil
}
