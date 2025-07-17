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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
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

func (agent *ecsAgent) appendVolumeDriverCapabilities(capabilities []types.Attribute) []types.Attribute {
	// "local" is default docker driver
	return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityDockerPluginInfix+volume.DockerLocalVolumeDriver)
}

func (agent *ecsAgent) appendNvidiaDriverVersionAttribute(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendENITrunkingCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendPIDAndIPCNamespaceSharingCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendAppMeshCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendTaskEIACapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensFluentdCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensFluentbitCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensNonRootUserCapability(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendEFSCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensLoggingDriverCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensLoggingDriverConfigCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFirelensConfigCapabilities(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendEFSVolumePluginCapabilities(capabilities []types.Attribute, pluginCapability string) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendIPv6Capability(capabilities []types.Attribute) []types.Attribute {
	return capabilities
}

func (agent *ecsAgent) appendFSxWindowsFileServerCapabilities(capabilities []types.Attribute) []types.Attribute {
	if agent.cfg.FSxWindowsFileServerCapable.Enabled() {
		return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityFSxWindowsFileServer)
	}

	return capabilities
}

// getTaskENIPluginVersionAttribute returns the version information of the ECS
// CNI plugins. It just executes the vpc-eni plugin to get the Version information.
// Currently, only this plugin is used by ECS Windows for awsvpc mode.
func (agent *ecsAgent) getTaskENIPluginVersionAttribute() (types.Attribute, error) {
	version, err := agent.cniClient.Version(ecscni.ECSVPCENIPluginExecutable)
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

var isWindows2016 = config.IsWindows2016

func defaultIsPlatformExecSupported() (bool, error) {
	if windows2016, err := isWindows2016(); err != nil || windows2016 {
		return false, err
	}
	return true, nil
}

// var to allow mocking for checkNetworkTooling
var isFaultInjectionToolingAvailable = checkFaultInjectionTooling

// checkFaultInjectionTooling checks for the required network packages like iptables, tc
// to be available on the host before ecs.capability.fault-injection can be advertised
func checkFaultInjectionTooling(_ *config.Config) bool {
	seelog.Warnf("Fault injection tooling is not supported on windows")
	return false
}
