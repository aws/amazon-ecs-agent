// +build linux

// Copyright 2014-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
)

const (
	AVX         = "avx"
	AVX2        = "avx2"
	SSE41       = "sse4_1"
	SSE42       = "sse4_2"
	CpuInfoPath = "/proc/cpuinfo"
)

func (agent *ecsAgent) appendVolumeDriverCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
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

func (agent *ecsAgent) appendNvidiaDriverVersionAttribute(capabilities []*ecs.Attribute) []*ecs.Attribute {
	if agent.resourceFields != nil && agent.resourceFields.NvidiaGPUManager != nil {
		driverVersion := agent.resourceFields.NvidiaGPUManager.GetDriverVersion()
		if driverVersion != "" {
			capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityNvidiaDriverVersionInfix+driverVersion)
		}
	}
	return capabilities
}

func (agent *ecsAgent) appendENITrunkingCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	if !agent.cfg.ENITrunkingEnabled {
		return capabilities
	}

	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+taskENITrunkingAttributeSuffix)

	return agent.appendBranchENIPluginVersionAttribute(capabilities)
}

func (agent *ecsAgent) appendBranchENIPluginVersionAttribute(capabilities []*ecs.Attribute) []*ecs.Attribute {
	version, err := agent.cniClient.Version(ecscni.ECSBranchENIPluginName)
	if err != nil {
		seelog.Warnf(
			"Unable to determine the version of the plugin '%s': %v",
			ecscni.ECSBranchENIPluginName, err)
		return capabilities
	}

	return append(capabilities, &ecs.Attribute{
		Name:  aws.String(attributePrefix + branchCNIPluginVersionSuffix),
		Value: aws.String(version),
	})
}

func (agent *ecsAgent) appendPIDAndIPCNamespaceSharingCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabiltyPIDAndIPCNamespaceSharing)
}

func (agent *ecsAgent) appendAppMeshCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return appendNameOnlyAttribute(capabilities, attributePrefix+appMeshAttributeSuffix)
}

func (agent *ecsAgent) appendTaskEIACapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {

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
