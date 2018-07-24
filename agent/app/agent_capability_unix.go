// +build !windows

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/cihub/seelog"
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
	standardizedPlugins, err := agent.dockerClient.ListPluginsWithFilters(agent.ctx, pluginEnabled, volumeDriverType, dockerapi.ListPluginsTimeout)
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
