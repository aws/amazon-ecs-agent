// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	capabilityPrefix             = "com.amazonaws.ecs.capability."
	capabilityTaskIAMRole        = "task-iam-role"
	capabilityTaskIAMRoleNetHost = "task-iam-role-network-host"
	attributePrefix              = "ecs.capability."
	taskENIAttributeSuffix       = "task-eni"
	cniPluginVersionSuffix       = "cni-plugin-version"
)

// awsVPCCNIPlugins is a list of CNI plugins required by the ECS Agent
// to configure the ENI for a task
var awsVPCCNIPlugins = []string{ecscni.ECSENIPluginName,
	ecscni.ECSBridgePluginName,
	ecscni.ECSIPAMPluginName,
}

// capabilities returns the supported capabilities of this agent / docker-client pair.
// Currently, the following capabilities are possible:
//
//    com.amazonaws.ecs.capability.privileged-container
//    com.amazonaws.ecs.capability.docker-remote-api.1.17
//    com.amazonaws.ecs.capability.docker-remote-api.1.18
//    com.amazonaws.ecs.capability.docker-remote-api.1.19
//    com.amazonaws.ecs.capability.docker-remote-api.1.20
//    com.amazonaws.ecs.capability.logging-driver.json-file
//    com.amazonaws.ecs.capability.logging-driver.syslog
//    com.amazonaws.ecs.capability.logging-driver.fluentd
//    com.amazonaws.ecs.capability.logging-driver.journald
//    com.amazonaws.ecs.capability.logging-driver.gelf
//    com.amazonaws.ecs.capability.selinux
//    com.amazonaws.ecs.capability.apparmor
//    com.amazonaws.ecs.capability.ecr-auth
//    com.amazonaws.ecs.capability.task-iam-role
//    com.amazonaws.ecs.capability.task-iam-role-network-host
//    ecs.capability.task-eni.0.1.0
func (agent *ecsAgent) capabilities() []*ecs.Attribute {
	var capabilities []*ecs.Attribute

	if !agent.cfg.PrivilegedDisabled {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"privileged-container")
	}

	supportedVersions := make(map[dockerclient.DockerVersion]bool)
	// Determine API versions to report as supported. Supported versions are also used for capability-enablement, except
	// logging drivers.
	for _, version := range agent.dockerClient.SupportedVersions() {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"docker-remote-api."+string(version))
		supportedVersions[version] = true
	}

	knownVersions := make(map[dockerclient.DockerVersion]struct{})
	// Determine known API versions. Known versions are used exclusively for logging-driver enablement, since none of
	// the structural API elements change.
	for _, version := range agent.dockerClient.KnownVersions() {
		knownVersions[version] = struct{}{}
	}

	for _, loggingDriver := range agent.cfg.AvailableLoggingDrivers {
		requiredVersion := dockerclient.LoggingDriverMinimumVersion[loggingDriver]
		if _, ok := knownVersions[requiredVersion]; ok {
			capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"logging-driver."+string(loggingDriver))
		}
	}

	if agent.cfg.SELinuxCapable {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"selinux")
	}
	if agent.cfg.AppArmorCapable {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"apparmor")
	}

	if _, ok := supportedVersions[dockerclient.Version_1_19]; ok {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"ecr-auth")
	}

	if agent.cfg.TaskIAMRoleEnabled {
		// The "task-iam-role" capability is supported for docker v1.7.x onwards
		// Refer https://github.com/docker/docker/blob/master/docs/reference/api/docker_remote_api.md
		// to lookup the table of docker supportedVersions to API supportedVersions
		if _, ok := supportedVersions[dockerclient.Version_1_19]; ok {
			capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+capabilityTaskIAMRole)
		} else {
			seelog.Warn("Task IAM Role not enabled due to unsuppported Docker version")
		}
	}

	if agent.cfg.TaskIAMRoleEnabledForNetworkHost {
		// The "task-iam-role-network-host" capability is supported for docker v1.7.x onwards
		if _, ok := supportedVersions[dockerclient.Version_1_19]; ok {
			capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+capabilityTaskIAMRoleNetHost)
		} else {
			seelog.Warn("Task IAM Role for Host Network not enabled due to unsuppported Docker version")
		}
	}

	if agent.cfg.TaskENIEnabled {
		taskENIAttribute, err := agent.getTaskENIAttribute()
		if err != nil {
			return capabilities
		}
		capabilities = append(capabilities, taskENIAttribute)
		taskENIVersionAttribute, err := agent.getTaskENIPluginVersionAttribute()
		if err != nil {
			return capabilities
		}
		capabilities = append(capabilities, taskENIVersionAttribute)
	}

	return capabilities
}

// getTaskENIPluginVersionAttribute returns the version information of the ECS
// CNI plugins. It just executes the ENI plugin as the assumption is that these
// plugins are packaged with the ECS Agent, which means all of the other plugins
// should also emit the same version information. Also, the version information
// doesn't contribute to placement decisions and just serves as additional
// debugging information
func (agent *ecsAgent) getTaskENIPluginVersionAttribute() (*ecs.Attribute, error) {
	version, err := agent.cniClient.Version(ecscni.ECSENIPluginName)
	if err != nil {
		seelog.Warnf(
			"Unable to determine the version of the plugin '%s': %v",
			ecscni.ECSENIPluginName, err)
		return nil, err
	}

	return &ecs.Attribute{
		Name:  aws.String(attributePrefix + cniPluginVersionSuffix),
		Value: aws.String(version),
	}, nil
}

func appendNameOnlyAttribute(attributes []*ecs.Attribute, name string) []*ecs.Attribute {
	return append(attributes, &ecs.Attribute{Name: aws.String(name)})
}

// getTaskENIAttribute returns the ECS "task-eni" attribute if all the necessary
// CNI plugins are present and if they contain the capabilities required to
// configure an ENI for a task
func (agent *ecsAgent) getTaskENIAttribute() (*ecs.Attribute, error) {
	// Check if we can get capabilities from each plugin
	for _, plugin := range awsVPCCNIPlugins {
		capabilities, err := agent.cniClient.Capabilities(plugin)
		if err != nil {
			seelog.Warnf(
				"Task ENI not enabled due to error in getting capabilities supported by the plugin '%s': %v",
				plugin, err)
			return nil, err
		}
		if !contains(capabilities, ecscni.CapabilityAWSVPCNetworkingMode) {
			seelog.Warnf(
				"Task ENI not enabled as plugin '%s' doesn't support the capability: %s",
				plugin, ecscni.CapabilityAWSVPCNetworkingMode)
			return nil, errors.Errorf(
				"engine get task-eni attribute: plugin '%s' doesn't support capability: %s",
				plugin, ecscni.CapabilityAWSVPCNetworkingMode)
		}
	}

	return &ecs.Attribute{
		Name: aws.String(attributePrefix + taskENIAttributeSuffix),
	}, nil
}

func contains(capabilities []string, capability string) bool {
	for _, cap := range capabilities {
		if cap == capability {
			return true
		}
	}

	return false
}
