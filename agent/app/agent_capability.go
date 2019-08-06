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
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// capabilityPrefix is deprecated. For new capabilities, use attributePrefix.
	capabilityPrefix                            = "com.amazonaws.ecs.capability."
	attributePrefix                             = "ecs.capability."
	capabilityTaskIAMRole                       = "task-iam-role"
	capabilityTaskIAMRoleNetHost                = "task-iam-role-network-host"
	taskENIAttributeSuffix                      = "task-eni"
	taskENIBlockInstanceMetadataAttributeSuffix = "task-eni-block-instance-metadata"
	appMeshAttributeSuffix                      = "aws-appmesh"
	cniPluginVersionSuffix                      = "cni-plugin-version"
	capabilityTaskCPUMemLimit                   = "task-cpu-mem-limit"
	capabilityDockerPluginInfix                 = "docker-plugin."
	attributeSeparator                          = "."
	capabilityPrivateRegistryAuthASM            = "private-registry-authentication.secretsmanager"
	capabilitySecretEnvSSM                      = "secrets.ssm.environment-variables"
	capabilitySecretEnvASM                      = "secrets.asm.environment-variables"
	capabilitySecretLogDriverSSM                = "secrets.ssm.bootstrap.log-driver"
	capabilitySecretLogDriverASM                = "secrets.asm.bootstrap.log-driver"
	capabiltyPIDAndIPCNamespaceSharing          = "pid-ipc-namespace-sharing"
	capabilityNvidiaDriverVersionInfix          = "nvidia-driver-version."
	capabilityECREndpoint                       = "ecr-endpoint"
	capabilityContainerOrdering                 = "container-ordering"
	taskEIAAttributeSuffix                      = "task-eia"
	taskEIAWithOptimizedCPU                     = "task-eia.optimized-cpu"
	taskENITrunkingAttributeSuffix              = "task-eni-trunking"
	branchCNIPluginVersionSuffix                = "branch-cni-plugin-version"
)

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
//    com.amazonaws.ecs.capability.logging-driver.none
//    com.amazonaws.ecs.capability.selinux
//    com.amazonaws.ecs.capability.apparmor
//    com.amazonaws.ecs.capability.ecr-auth
//    com.amazonaws.ecs.capability.task-iam-role
//    com.amazonaws.ecs.capability.task-iam-role-network-host
//    ecs.capability.docker-volume-driver.${driverName}
//    ecs.capability.task-eni
//    ecs.capability.task-eni-block-instance-metadata
//    ecs.capability.execution-role-ecr-pull
//    ecs.capability.execution-role-awslogs
//    ecs.capability.container-health-check
//    ecs.capability.private-registry-authentication.secretsmanager
//    ecs.capability.secrets.ssm.environment-variables
//    ecs.capability.secrets.ssm.bootstrap.log-driver
//    ecs.capability.pid-ipc-namespace-sharing
//    ecs.capability.ecr-endpoint
//    ecs.capability.secrets.asm.environment-variables
//    ecs.capability.secrets.asm.bootstrap.log-driver
//    ecs.capability.aws-appmesh
//    ecs.capability.task-eia
//    ecs.capability.task-eni-trunking
//    ecs.capability.task-eia.optimized-cpu
func (agent *ecsAgent) capabilities() ([]*ecs.Attribute, error) {
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

	capabilities = agent.appendLoggingDriverCapabilities(capabilities)

	if agent.cfg.SELinuxCapable {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"selinux")
	}
	if agent.cfg.AppArmorCapable {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"apparmor")
	}

	capabilities = agent.appendTaskIamRoleCapabilities(capabilities, supportedVersions)

	capabilities, err := agent.appendTaskCPUMemLimitCapabilities(capabilities, supportedVersions)
	if err != nil {
		return nil, err
	}

	capabilities = agent.appendTaskENICapabilities(capabilities)
	capabilities = agent.appendENITrunkingCapabilities(capabilities)
	capabilities = agent.appendDockerDependentCapabilities(capabilities, supportedVersions)

	// TODO: gate this on docker api version when ecs supported docker includes
	// credentials endpoint feature from upstream docker
	if agent.cfg.OverrideAWSLogsExecutionRole {
		capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+"execution-role-awslogs")
	}

	capabilities = agent.appendVolumeDriverCapabilities(capabilities)

	// ecs agent version 1.19.0 supports private registry authentication using
	// aws secrets manager
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityPrivateRegistryAuthASM)

	// ecs agent version 1.22.0 supports ecs secrets integrating with aws systems manager
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilitySecretEnvSSM)

	// ecs agent version 1.27.0 supports ecs secrets for logging drivers
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilitySecretLogDriverSSM)

	if agent.cfg.GPUSupportEnabled {
		capabilities = agent.appendNvidiaDriverVersionAttribute(capabilities)
	}
	// support ecr endpoint override
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityECREndpoint)

	// ecs agent version 1.23.0 supports ecs secrets integrating with aws secrets manager
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilitySecretEnvASM)

	// ecs agent version 1.27.0 supports ecs secrets for logging drivers
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilitySecretLogDriverASM)

	// support container ordering in agent
	capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityContainerOrdering)

	// ecs agent version 1.22.0 supports sharing PID namespaces and IPC resource namespaces
	// with host EC2 instance and among containers within the task
	capabilities = agent.appendPIDAndIPCNamespaceSharingCapabilities(capabilities)

	// ecs agent version 1.26.0 supports aws-appmesh cni plugin
	capabilities = agent.appendAppMeshCapabilities(capabilities)

	// support elastic inference in agent
	capabilities = agent.appendTaskEIACapabilities(capabilities)

	return capabilities, nil
}

func (agent *ecsAgent) appendDockerDependentCapabilities(capabilities []*ecs.Attribute,
	supportedVersions map[dockerclient.DockerVersion]bool) []*ecs.Attribute {
	if _, ok := supportedVersions[dockerclient.Version_1_19]; ok {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"ecr-auth")
		capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+"execution-role-ecr-pull")
	}

	if _, ok := supportedVersions[dockerclient.Version_1_24]; ok && !agent.cfg.DisableDockerHealthCheck {
		// Docker health check was added in API 1.24
		capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+"container-health-check")
	}
	return capabilities
}

func (agent *ecsAgent) appendLoggingDriverCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
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
	return capabilities
}

func (agent *ecsAgent) appendTaskIamRoleCapabilities(capabilities []*ecs.Attribute, supportedVersions map[dockerclient.DockerVersion]bool) []*ecs.Attribute {
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
	return capabilities
}

func (agent *ecsAgent) appendTaskCPUMemLimitCapabilities(capabilities []*ecs.Attribute, supportedVersions map[dockerclient.DockerVersion]bool) ([]*ecs.Attribute, error) {
	if agent.cfg.TaskCPUMemLimit.Enabled() {
		if _, ok := supportedVersions[dockerclient.Version_1_22]; ok {
			capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityTaskCPUMemLimit)
		} else if agent.cfg.TaskCPUMemLimit == config.ExplicitlyEnabled {
			// explicitly enabled -- return an error because we cannot fulfil an explicit request
			return nil, errors.New("engine: Task CPU + Mem limit cannot be enabled due to unsupported Docker version")
		} else {
			// implicitly enabled -- don't register the capability, but degrade gracefully
			seelog.Warn("Task CPU + Mem Limit disabled due to unsupported Docker version. API version 1.22 or greater is required.")
			agent.cfg.TaskCPUMemLimit = config.ExplicitlyDisabled
		}
	}
	return capabilities, nil
}

func (agent *ecsAgent) appendTaskENICapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	if agent.cfg.TaskENIEnabled {
		// The assumption here is that all of the dependencies for supporting the
		// Task ENI in the Agent have already been validated prior to the invocation of
		// the `agent.capabilities()` call
		capabilities = append(capabilities, &ecs.Attribute{
			Name: aws.String(attributePrefix + taskENIAttributeSuffix),
		})
		taskENIVersionAttribute, err := agent.getTaskENIPluginVersionAttribute()
		if err != nil {
			return capabilities
		}
		capabilities = append(capabilities, taskENIVersionAttribute)

		// We only care about AWSVPCBlockInstanceMetdata if Task ENI is enabled
		if agent.cfg.AWSVPCBlockInstanceMetdata {
			// If the Block Instance Metadata flag is set for AWS VPC networking mode, register a capability
			// indicating the same
			capabilities = append(capabilities, &ecs.Attribute{
				Name: aws.String(attributePrefix + taskENIBlockInstanceMetadataAttributeSuffix),
			})
		}
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
