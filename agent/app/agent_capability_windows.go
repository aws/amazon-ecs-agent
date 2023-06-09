//go:build windows
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

package app

import (
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
)

var (
	capabilityDepsRootDir  = filepath.Join(config.AmazonECSProgramFiles, "managed-agents")
	ssmPluginDir           = filepath.Join(config.AmazonProgramFiles, "SSM", "Plugins")
	sessionManagerShellDir = filepath.Join(ssmPluginDir, "SessionManagerShell")
	awsCloudWatchDir       = filepath.Join(ssmPluginDir, "awsCloudWatch")
	awsDomainJoin          = filepath.Join(ssmPluginDir, "awsDomainJoin")

	capabilityExecRequiredBinaries = []string{
		"amazon-ssm-agent.exe",
		"ssm-agent-worker.exe",
		"ssm-session-worker.exe",
	}

	// top-level folders, /bin, /config, /plugins
	dependencies = map[string][]string{
		binDir:                 []string{},
		configDir:              []string{},
		ssmPluginDir:           []string{},
		sessionManagerShellDir: []string{},
		awsCloudWatchDir:       []string{},
		awsDomainJoin:          []string{},
	}
)

func (agent *ecsAgent) appendVolumeDriverCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	// "local" is default docker driver
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityDockerPluginInfix+volume.DockerLocalVolumeDriver)
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

func (agent *ecsAgent) appendEFSVolumePluginCapabilities(capabilities []*ecs.Attribute, pluginCapability string) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendIPv6Capability(capabilities []*ecs.Attribute) []*ecs.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFSxWindowsFileServerCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	if agent.cfg.FSxWindowsFileServerCapable.Enabled() {
		return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityFSxWindowsFileServer)
	}

	return capabilities
}

// getTaskENIPluginVersionAttribute returns the version information of the ECS
// CNI plugins. It just executes the ECSVPCENIPluginName plugin to get the Version information.
// Currently, only this plugin is used by ECS Windows for awsvpc mode.
func (agent *ecsAgent) getTaskENIPluginVersionAttribute() (*ecs.Attribute, error) {
	version, err := agent.cniClient.Version(ecscni.ECSVPCENIPluginExecutable)
	if err != nil {
		seelog.Warnf(
			"Unable to determine the version of the plugin '%s': %v",
			ecscni.ECSVPCENIPluginName, err)
		return nil, err
	}

	return &ecs.Attribute{
		Name:  aws.String(attributePrefix + cniPluginVersionSuffix),
		Value: aws.String(version),
	}, nil
}

var isWindows2016 = config.IsWindows2016

func defaultIsPlatformExecSupported() (bool, error) {
	if windows2016, err := isWindows2016(); err != nil || windows2016 {
		return false, err
	}
	return true, nil
}
