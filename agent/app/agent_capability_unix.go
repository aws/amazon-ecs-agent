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

package app

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils/netconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/cihub/seelog"
)

const (
	AVX                         = "avx"
	AVX2                        = "avx2"
	SSE41                       = "sse4_1"
	SSE42                       = "sse4_2"
	CpuInfoPath                 = "/proc/cpuinfo"
	capabilityDepsRootDir       = "/managed-agents"
	modInfoCmd                  = "modinfo"
	faultInjectionKernelModules = "sch_netem"
	ctxTimeoutDuration          = 60 * time.Second
	tcShowCmdString             = "tc -j q show dev %s parent 1:1"
)

var (
	certsDir                    = filepath.Join(capabilityExecRootDir, capabilityExecCertsRelativePath)
	capabilityExecRequiredCerts = []string{
		"tls-ca-bundle.pem",
	}
	capabilityExecRequiredBinaries = []string{
		"amazon-ssm-agent",
		"ssm-agent-worker",
		"ssm-session-worker",
	}

	// top-level folders, /bin, /config, /certs
	dependencies = map[string][]string{
		binDir:    []string{},
		configDir: []string{},
		certsDir:  capabilityExecRequiredCerts,
	}
)

func (agent *ecsAgent) appendVolumeDriverCapabilities(capabilities []types.Attribute) []types.Attribute {
	// "local" is default docker driver
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityDockerPluginInfix+volume.DockerLocalVolumeDriver)

	// for non-standardized plugins, call docker pkg's plugins.Scan()
	nonStandardizedPlugins, err := agent.mobyPlugins.Scan()
	if err != nil {
		seelog.Warnf("Scanning plugins failed: %v", err)
		// do not return yet, we need the list of plugins below. range handles nil slice.
	}

	for _, pluginName := range nonStandardizedPlugins {
		// Replace the ':' to '.' in the plugin name for attributes
		capabilities = appendNameOnlyAttribute(capabilities,
			attributePrefix+capabilityDockerPluginInfix+strings.Replace(pluginName, config.DockerTagSeparator, attributeSeparator, -1))
	}

	// for standardized plugins, call docker's plugin ls API
	pluginEnabled := true
	volumeDriverType := []string{dockerapi.VolumeDriverType}
	standardizedPlugins, err := agent.dockerClient.ListPluginsWithFilters(agent.ctx, pluginEnabled, volumeDriverType, dockerclient.ListPluginsTimeout)
	if err != nil {
		seelog.Warnf("Listing plugins with filters enabled=%t, capabilities=%v failed: %v", pluginEnabled, volumeDriverType, err)
		return capabilities
	}

	// For plugin with default tag latest, register two attributes with and without the latest tag
	// as the tag is optional and can be added by docker or customer
	for _, pluginName := range standardizedPlugins {
		names := strings.Split(pluginName, config.DockerTagSeparator)
		if len(names) > 1 && names[len(names)-1] == config.DefaultDockerTag {
			capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityDockerPluginInfix+strings.Join(names[:len(names)-1], attributeSeparator))
		}

		capabilities = appendNameOnlyAttribute(capabilities,
			attributePrefix+capabilityDockerPluginInfix+strings.Replace(pluginName, config.DockerTagSeparator, attributeSeparator, -1))
	}
	return capabilities
}

func (agent *ecsAgent) appendNvidiaDriverVersionAttribute(capabilities []types.Attribute) []types.Attribute {
	if agent.resourceFields != nil && agent.resourceFields.NvidiaGPUManager != nil {
		driverVersion := agent.resourceFields.NvidiaGPUManager.GetDriverVersion()
		if driverVersion != "" {
			capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityNvidiaDriverVersionInfix+driverVersion)
			capabilities = append(capabilities, types.Attribute{
				Name:  aws.String(attributePrefix + capabilityGpuDriverVersion),
				Value: aws.String(driverVersion),
			})
		}
	}
	return capabilities
}

func (agent *ecsAgent) appendENITrunkingCapabilities(capabilities []types.Attribute) []types.Attribute {
	if !agent.cfg.ENITrunkingEnabled.Enabled() {
		return capabilities
	}
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+taskENITrunkingAttributeSuffix)
	return agent.appendBranchENIPluginVersionAttribute(capabilities)
}

func (agent *ecsAgent) appendBranchENIPluginVersionAttribute(capabilities []types.Attribute) []types.Attribute {
	version, err := agent.cniClient.Version(ecscni.ECSBranchENIPluginName)
	if err != nil {
		seelog.Warnf(
			"Unable to determine the version of the plugin '%s': %v",
			ecscni.ECSBranchENIPluginName, err)
		return capabilities
	}

	return append(capabilities, types.Attribute{
		Name:  aws.String(attributePrefix + branchCNIPluginVersionSuffix),
		Value: aws.String(version),
	})
}

func (agent *ecsAgent) appendPIDAndIPCNamespaceSharingCapabilities(capabilities []types.Attribute) []types.Attribute {
	isLoaded, err := agent.pauseLoader.IsLoaded(agent.dockerClient)
	if !isLoaded || err != nil {
		seelog.Warnf("Pause container is not loaded, did not append PID and IPC capabilities: %v", err)
		return capabilities
	}
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabiltyPIDAndIPCNamespaceSharing)
}

func (agent *ecsAgent) appendAppMeshCapabilities(capabilities []types.Attribute) []types.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+appMeshAttributeSuffix)
}

func (agent *ecsAgent) appendTaskEIACapabilities(capabilities []types.Attribute) []types.Attribute {
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+taskEIAAttributeSuffix)

	eiaRequiredFlags := []string{AVX, AVX2, SSE41, SSE42}
	cpuInfo, err := utils.ReadCPUInfo(CpuInfoPath)
	if err != nil {
		seelog.Warnf("Unable to read cpuinfo: %v", err)
		return capabilities
	}

	flagMap := utils.GetCPUFlags(cpuInfo)
	missingFlags := []string{}
	for _, requiredFlag := range eiaRequiredFlags {
		if _, ok := flagMap[requiredFlag]; !ok {
			missingFlags = append(missingFlags, requiredFlag)
		}
	}

	if len(missingFlags) > 0 {
		seelog.Infof("Missing cpu flags for EIA support: %v", strings.Join(missingFlags, ","))
		return capabilities
	}

	return appendNameOnlyAttribute(capabilities, attributePrefix+taskEIAWithOptimizedCPU)
}

func (agent *ecsAgent) appendFirelensFluentdCapabilities(capabilities []types.Attribute) []types.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityFirelensFluentd)
}

func (agent *ecsAgent) appendFirelensFluentbitCapabilities(capabilities []types.Attribute) []types.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityFirelensFluentbit)
}

func (agent *ecsAgent) appendFirelensNonRootUserCapability(capabilities []types.Attribute) []types.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityFirelensNonRootUser)
}

func (agent *ecsAgent) appendFirelensLoggingDriverCapabilities(capabilities []types.Attribute) []types.Attribute {
	return appendNameOnlyAttribute(capabilities, capabilityPrefix+capabilityFirelensLoggingDriver)
}

func (agent *ecsAgent) appendFirelensLoggingDriverConfigCapabilities(capabilities []types.Attribute) []types.Attribute {
	return appendNameOnlyAttribute(capabilities,
		attributePrefix+capabilityFirelensLoggingDriver+capabilityFireLensLoggingDriverConfigBufferLimitSuffix)
}

func (agent *ecsAgent) appendFirelensConfigCapabilities(capabilities []types.Attribute) []types.Attribute {
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityFirelensConfigFile)
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityFirelensConfigS3)
}

func (agent *ecsAgent) appendEFSCapabilities(capabilities []types.Attribute) []types.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityEFS)
}

func (agent *ecsAgent) appendEFSVolumePluginCapabilities(capabilities []types.Attribute, pluginCapability string) []types.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+pluginCapability)
}

func (agent *ecsAgent) appendIPv6Capability(capabilities []types.Attribute) []types.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+taskENIIPv6AttributeSuffix)
}

func (agent *ecsAgent) appendFSxWindowsFileServerCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

// getTaskENIPluginVersionAttribute returns the version information of the ECS
// CNI plugins. It just executes the ENI plugin as the assumption is that these
// plugins are packaged with the ECS Agent, which means all of the other plugins
// should also emit the same version information. Also, the version information
// doesn't contribute to placement decisions and just serves as additional
// debugging information
func (agent *ecsAgent) getTaskENIPluginVersionAttribute() (types.Attribute, error) {
	version, err := agent.cniClient.Version(ecscni.VPCENIPluginName)
	if err != nil {
		seelog.Warnf(
			"Unable to determine the version of the plugin '%s': %v",
			ecscni.VPCENIPluginName, err)
		return types.Attribute{}, err
	}

	return types.Attribute{
		Name:  aws.String(attributePrefix + cniPluginVersionSuffix),
		Value: aws.String(version),
	}, nil
}

func defaultIsPlatformExecSupported() (bool, error) {
	return true, nil
}

// var to allow mocking for checkNetworkTooling
var isFaultInjectionToolingAvailable = checkFaultInjectionTooling

// wrapper around exec.LookPath
var lookPathFunc = exec.LookPath
var osExecWrapper = execwrapper.NewExec()
var networkConfigClient = netconfig.NewNetworkConfigClient()

// checkFaultInjectionTooling checks for the required network packages like iptables, tc
// to be available on the host before ecs.capability.fault-injection can be advertised
func checkFaultInjectionTooling(cfg *config.Config) bool {
	tools := []string{"iptables", "tc", "nsenter"}
	if cfg.InstanceIPCompatibility.IsIPv6Only() {
		// ip6tables is a required dependency on IPv6-only instances.
		// TODO: Consider making ip6tables a required dependency for all instances (need to consider backwards compatibility)
		tools = append(tools, "ip6tables")
	}
	for _, tool := range tools {
		if _, err := lookPathFunc(tool); err != nil {
			seelog.Warnf(
				"Failed to find network tool %s that is needed for fault-injection feature: %v",
				tool, err)
			return false
		}
	}
	return checkFaultInjectionModules() && checkTCShowTooling()
}

// checkFaultInjectionModules checks for the required kernel modules such as sch_netem to be installed
// and avaialble on the host before ecs.capability.fault-injection can be advertised
func checkFaultInjectionModules() bool {
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
	defer cancel()
	_, err := osExecWrapper.CommandContext(ctxWithTimeout, modInfoCmd, faultInjectionKernelModules).CombinedOutput()
	if err != nil {
		seelog.Warnf("Failed to find kernel module %s that is needed for fault-injection feature: %v", faultInjectionKernelModules, err)
		return false
	}
	return true
}

func checkTCShowTooling() bool {
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
	defer cancel()
	hostDeviceName, netErr := netconfig.DefaultNetInterfaceName(networkConfigClient.NetlinkClient)
	if netErr != nil {
		seelog.Warnf("Failed to obtain the network interface device name on the host: %v", netErr)
		return false
	}
	tcShowCmd := fmt.Sprintf(tcShowCmdString, hostDeviceName)
	cmdList := strings.Split(tcShowCmd, " ")
	_, err := osExecWrapper.CommandContext(ctxWithTimeout, cmdList[0], cmdList[1:]...).CombinedOutput()
	if err != nil {
		seelog.Warnf("Failed to call %s which is needed for fault-injection feature: %v", tcShowCmd, err)
		return false
	}
	return true
}
