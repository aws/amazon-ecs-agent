//go:build !linux && !windows
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

package app

import (
	"errors"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/cihub/seelog"
)

const (
	capabilityDepsRootDir = ""
)

var (
	capabilityExecRequiredBinaries = []string{}
	dependencies                   = map[string][]string{}
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
	return capabilities
}

func (agent *ecsAgent) appendENITrunkingCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendPIDAndIPCNamespaceSharingCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendAppMeshCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendTaskEIACapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensFluentdCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensFluentbitCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendEFSCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensLoggingDriverCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensLoggingDriverConfigCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensConfigCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendGMSACapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendEFSVolumePluginCapabilities(capabilities []*ecs.Attribute, pluginCapability string) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendIPv6Capability(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFSxWindowsFileServerCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

// getTaskENIPluginVersionAttribute for unsupported platform would return an error
func (agent *ecsAgent) getTaskENIPluginVersionAttribute() (*ecs.Attribute, error) {
	return nil, errors.New("unsupported platform")
}

func defaultIsPlatformExecSupported() (bool, error) {
	return false, nil
}
