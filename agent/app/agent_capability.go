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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// capabilityPrefix is deprecated. For new capabilities, use attributePrefix.
	capabilityPrefix                                       = "com.amazonaws.ecs.capability."
	attributePrefix                                        = "ecs.capability."
	capabilityTaskIAMRole                                  = "task-iam-role"
	capabilityTaskIAMRoleNetHost                           = "task-iam-role-network-host"
	taskENIAttributeSuffix                                 = "task-eni"
	taskENIIPv6AttributeSuffix                             = "task-eni.ipv6"
	taskENIBlockInstanceMetadataAttributeSuffix            = "task-eni-block-instance-metadata"
	appMeshAttributeSuffix                                 = "aws-appmesh"
	cniPluginVersionSuffix                                 = "cni-plugin-version"
	capabilityTaskCPUMemLimit                              = "task-cpu-mem-limit"
	capabilityIncreasedTaskCPULimit                        = "increased-task-cpu-limit"
	capabilityDockerPluginInfix                            = "docker-plugin."
	attributeSeparator                                     = "."
	capabilityPrivateRegistryAuthASM                       = "private-registry-authentication.secretsmanager"
	capabilitySecretEnvSSM                                 = "secrets.ssm.environment-variables"
	capabilitySecretEnvASM                                 = "secrets.asm.environment-variables"
	capabilitySecretLogDriverSSM                           = "secrets.ssm.bootstrap.log-driver"
	capabilitySecretLogDriverASM                           = "secrets.asm.bootstrap.log-driver"
	capabiltyPIDAndIPCNamespaceSharing                     = "pid-ipc-namespace-sharing"
	capabilityNvidiaDriverVersionInfix                     = "nvidia-driver-version."
	capabilityECREndpoint                                  = "ecr-endpoint"
	capabilityContainerOrdering                            = "container-ordering"
	taskEIAAttributeSuffix                                 = "task-eia"
	taskEIAWithOptimizedCPU                                = "task-eia.optimized-cpu"
	taskENITrunkingAttributeSuffix                         = "task-eni-trunking"
	branchCNIPluginVersionSuffix                           = "branch-cni-plugin-version"
	capabilityFirelensFluentd                              = "firelens.fluentd"
	capabilityFirelensFluentbit                            = "firelens.fluentbit"
	capabilityFirelensLoggingDriver                        = "logging-driver.awsfirelens"
	capabilityFireLensLoggingDriverConfigBufferLimitSuffix = ".log-driver-buffer-limit"
	capabilityFirelensConfigFile                           = "firelens.options.config.file"
	capabilityFirelensConfigS3                             = "firelens.options.config.s3"
	capabilityFullTaskSync                                 = "full-sync"
	capabilityGMSA                                         = "gmsa"
	capabilityEFS                                          = "efs"
	capabilityEFSAuth                                      = "efsAuth"
	capabilityEnvFilesS3                                   = "env-files.s3"
	capabilityFSxWindowsFileServer                         = "fsxWindowsFileServer"
	capabilityExec                                         = "execute-command"
	capabilityExecBinRelativePath                          = "bin"
	capabilityExecConfigRelativePath                       = "config"
	capabilityExecCertsRelativePath                        = "certs"
	capabilityExternal                                     = "external"
	capabilityServiceConnect                               = "service-connect-v1"

	// network capabilities, going forward, please append "network." prefix to any new networking capability we introduce
	networkCapabilityPrefix      = "network."
	capabilityContainerPortRange = networkCapabilityPrefix + "container-port-range"
)

var (
	nameOnlyAttributes = []string{
		// ecs agent version 1.19.0 supports private registry authentication using
		// aws secrets manager
		capabilityPrivateRegistryAuthASM,
		// ecs agent version 1.22.0 supports ecs secrets integrating with aws systems manager
		capabilitySecretEnvSSM,
		// ecs agent version 1.27.0 supports ecs secrets for logging drivers
		capabilitySecretLogDriverSSM,
		// support ecr endpoint override
		capabilityECREndpoint,
		// ecs agent version 1.23.0 supports ecs secrets integrating with aws secrets manager
		capabilitySecretEnvASM,
		// ecs agent version 1.27.0 supports ecs secrets for logging drivers
		capabilitySecretLogDriverASM,
		// support container ordering in agent
		capabilityContainerOrdering,
		// support full task sync
		capabilityFullTaskSync,
		// ecs agent version 1.39.0 supports bulk loading env vars through environmentFiles in S3
		capabilityEnvFilesS3,
		// support container port range in container definition - port mapping field
		capabilityContainerPortRange,
	}
	// use empty struct as value type to simulate set
	capabilityExecInvalidSsmVersions = map[string]struct{}{}

	pathExists              = defaultPathExists
	getSubDirectories       = defaultGetSubDirectories
	isPlatformExecSupported = defaultIsPlatformExecSupported

	// List of capabilities that are not supported on external capacity.
	externalUnsupportedCapabilities = []string{
		attributePrefix + taskENIAttributeSuffix,
		attributePrefix + cniPluginVersionSuffix,
		attributePrefix + taskENIIPv6AttributeSuffix,
		attributePrefix + taskENIBlockInstanceMetadataAttributeSuffix,
		attributePrefix + taskENITrunkingAttributeSuffix,
		attributePrefix + appMeshAttributeSuffix,
		attributePrefix + taskEIAAttributeSuffix,
		attributePrefix + taskEIAWithOptimizedCPU,
		attributePrefix + capabilityServiceConnect,
	}
	// List of capabilities that are only supported on external capaciity. Currently only one but keep as a list
	// for future proof and also align with externalUnsupportedCapabilities.
	externalSpecificCapabilities = []string{
		attributePrefix + capabilityExternal,
	}

	capabilityExecRootDir = filepath.Join(capabilityDepsRootDir, capabilityExec)
	binDir                = filepath.Join(capabilityExecRootDir, capabilityExecBinRelativePath)
	configDir             = filepath.Join(capabilityExecRootDir, capabilityExecConfigRelativePath)
)

// capabilities returns the supported capabilities of this agent / docker-client pair.
// Currently, the following capabilities are possible:
//
//	com.amazonaws.ecs.capability.privileged-container
//	com.amazonaws.ecs.capability.docker-remote-api.1.17
//	com.amazonaws.ecs.capability.docker-remote-api.1.18
//	com.amazonaws.ecs.capability.docker-remote-api.1.19
//	com.amazonaws.ecs.capability.docker-remote-api.1.20
//	com.amazonaws.ecs.capability.logging-driver.json-file
//	com.amazonaws.ecs.capability.logging-driver.syslog
//	com.amazonaws.ecs.capability.logging-driver.fluentd
//	com.amazonaws.ecs.capability.logging-driver.journald
//	com.amazonaws.ecs.capability.logging-driver.gelf
//	com.amazonaws.ecs.capability.logging-driver.none
//	com.amazonaws.ecs.capability.selinux
//	com.amazonaws.ecs.capability.apparmor
//	com.amazonaws.ecs.capability.ecr-auth
//	com.amazonaws.ecs.capability.task-iam-role
//	com.amazonaws.ecs.capability.task-iam-role-network-host
//	ecs.capability.docker-volume-driver.${driverName}
//	ecs.capability.task-eni
//	ecs.capability.task-eni-block-instance-metadata
//	ecs.capability.execution-role-ecr-pull
//	ecs.capability.execution-role-awslogs
//	ecs.capability.container-health-check
//	ecs.capability.private-registry-authentication.secretsmanager
//	ecs.capability.secrets.ssm.environment-variables
//	ecs.capability.secrets.ssm.bootstrap.log-driver
//	ecs.capability.pid-ipc-namespace-sharing
//	ecs.capability.ecr-endpoint
//	ecs.capability.secrets.asm.environment-variables
//	ecs.capability.secrets.asm.bootstrap.log-driver
//	ecs.capability.aws-appmesh
//	ecs.capability.task-eia
//	ecs.capability.task-eni-trunking
//	ecs.capability.task-eia.optimized-cpu
//	ecs.capability.firelens.fluentd
//	ecs.capability.firelens.fluentbit
//	ecs.capability.efs
//	com.amazonaws.ecs.capability.logging-driver.awsfirelens
//	ecs.capability.logging-driver.awsfirelens.log-driver-buffer-limit
//	ecs.capability.firelens.options.config.file
//	ecs.capability.firelens.options.config.s3
//	ecs.capability.full-sync
//	ecs.capability.gmsa
//	ecs.capability.efsAuth
//	ecs.capability.env-files.s3
//	ecs.capability.fsxWindowsFileServer
//	ecs.capability.execute-command
//	ecs.capability.external
//	ecs.capability.service-connect-v1
//	ecs.capability.network.container-port-range
func (agent *ecsAgent) capabilities() ([]*ecs.Attribute, error) {
	var capabilities []*ecs.Attribute

	for _, cap := range nameOnlyAttributes {
		capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+cap)
	}

	if !agent.cfg.PrivilegedDisabled.Enabled() {
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

	if agent.cfg.SELinuxCapable.Enabled() {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"selinux")
	}
	if agent.cfg.AppArmorCapable.Enabled() {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"apparmor")
	}

	capabilities = agent.appendTaskIamRoleCapabilities(capabilities, supportedVersions)

	capabilities, err := agent.appendTaskCPUMemLimitCapabilities(capabilities, supportedVersions)
	if err != nil {
		return nil, err
	}

	capabilities = agent.appendIncreasedTaskCPULimitCapability(capabilities)
	capabilities = agent.appendTaskENICapabilities(capabilities)
	capabilities = agent.appendENITrunkingCapabilities(capabilities)
	capabilities = agent.appendDockerDependentCapabilities(capabilities, supportedVersions)

	// TODO: gate this on docker api version when ecs supported docker includes
	// credentials endpoint feature from upstream docker
	if agent.cfg.OverrideAWSLogsExecutionRole.Enabled() {
		capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+"execution-role-awslogs")
	}

	capabilities = agent.appendVolumeDriverCapabilities(capabilities)

	if agent.cfg.GPUSupportEnabled {
		capabilities = agent.appendNvidiaDriverVersionAttribute(capabilities)
	}

	// ecs agent version 1.22.0 supports sharing PID namespaces and IPC resource namespaces
	// with host EC2 instance and among containers within the task
	capabilities = agent.appendPIDAndIPCNamespaceSharingCapabilities(capabilities)

	// ecs agent version 1.26.0 supports aws-appmesh cni plugin
	capabilities = agent.appendAppMeshCapabilities(capabilities)

	// support elastic inference in agent
	capabilities = agent.appendTaskEIACapabilities(capabilities)

	// support aws router capabilities for fluentd
	capabilities = agent.appendFirelensFluentdCapabilities(capabilities)

	// support aws router capabilities for fluentbit
	capabilities = agent.appendFirelensFluentbitCapabilities(capabilities)

	// support aws router capabilities for log driver router
	capabilities = agent.appendFirelensLoggingDriverCapabilities(capabilities)

	// support aws router capabilities for log driver router config
	capabilities = agent.appendFirelensLoggingDriverConfigCapabilities(capabilities)

	// support efs on ecs capabilities
	capabilities = agent.appendEFSCapabilities(capabilities)

	// support external firelens config
	capabilities = agent.appendFirelensConfigCapabilities(capabilities)

	// support GMSA capabilities
	capabilities = agent.appendGMSACapabilities(capabilities)

	// support efs auth on ecs capabilities
	for _, cap := range agent.cfg.VolumePluginCapabilities {
		capabilities = agent.appendEFSVolumePluginCapabilities(capabilities, cap)
	}

	// support fsxWindowsFileServer on ecs capabilities
	capabilities = agent.appendFSxWindowsFileServerCapabilities(capabilities)
	// add ecs-exec capabilities if applicable
	capabilities, err = agent.appendExecCapabilities(capabilities)
	if err != nil {
		return nil, err
	}
	// add service-connect capabilities if applicable
	capabilities = agent.appendServiceConnectCapabilities(capabilities)

	if agent.cfg.External.Enabled() {
		// Add external specific capability; remove external unsupported capabilities.
		for _, cap := range externalSpecificCapabilities {
			capabilities = appendNameOnlyAttribute(capabilities, cap)
		}
		capabilities = removeAttributesByNames(capabilities, externalUnsupportedCapabilities)
	}

	return capabilities, nil
}

func (agent *ecsAgent) appendDockerDependentCapabilities(capabilities []*ecs.Attribute,
	supportedVersions map[dockerclient.DockerVersion]bool) []*ecs.Attribute {
	if _, ok := supportedVersions[dockerclient.Version_1_19]; ok {
		capabilities = appendNameOnlyAttribute(capabilities, capabilityPrefix+"ecr-auth")
		capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+"execution-role-ecr-pull")
	}

	if _, ok := supportedVersions[dockerclient.Version_1_24]; ok && !agent.cfg.DisableDockerHealthCheck.Enabled() {
		// Docker health check was added in API 1.24
		capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+"container-health-check")
	}
	return capabilities
}

func (agent *ecsAgent) appendGMSACapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	if agent.cfg.GMSACapable {
		return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityGMSA)
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
	if agent.cfg.TaskIAMRoleEnabled.Enabled() {
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
		} else if agent.cfg.TaskCPUMemLimit.Value == config.ExplicitlyEnabled {
			// explicitly enabled -- return an error because we cannot fulfil an explicit request
			return nil, errors.New("engine: Task CPU + Mem limit cannot be enabled due to unsupported Docker version")
		} else {
			// implicitly enabled -- don't register the capability, but degrade gracefully
			seelog.Warn("Task CPU + Mem Limit disabled due to unsupported Docker version. API version 1.22 or greater is required.")
			agent.cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
		}
	}
	return capabilities, nil
}

func (agent *ecsAgent) appendIncreasedTaskCPULimitCapability(capabilities []*ecs.Attribute) []*ecs.Attribute {
	if !agent.cfg.TaskCPUMemLimit.Enabled() {
		// don't register the "increased-task-cpu-limit" capability if the "task-cpu-mem-limit" capability is disabled.
		// "task-cpu-mem-limit" capability may be explicitly disabled or disabled due to unsupported docker version.
		seelog.Warn("Increased Task CPU Limit capability is disabled since the Task CPU + Mem Limit capability is disabled.")
	} else {
		capabilities = appendNameOnlyAttribute(capabilities, attributePrefix+capabilityIncreasedTaskCPULimit)
	}
	return capabilities
}

func (agent *ecsAgent) appendTaskENICapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	if agent.cfg.TaskENIEnabled.Enabled() {
		// The assumption here is that all of the dependencies for supporting the
		// Task ENI in the Agent have already been validated prior to the invocation of
		// the `agent.capabilities()` call
		capabilities = append(capabilities, &ecs.Attribute{
			Name: aws.String(attributePrefix + taskENIAttributeSuffix),
		})
		capabilities = agent.appendIPv6Capability(capabilities)
		taskENIVersionAttribute, err := agent.getTaskENIPluginVersionAttribute()
		if err != nil {
			return capabilities
		}
		capabilities = append(capabilities, taskENIVersionAttribute)

		// We only care about AWSVPCBlockInstanceMetdata if Task ENI is enabled
		if agent.cfg.AWSVPCBlockInstanceMetdata.Enabled() {
			// If the Block Instance Metadata flag is set for AWS VPC networking mode, register a capability
			// indicating the same
			capabilities = append(capabilities, &ecs.Attribute{
				Name: aws.String(attributePrefix + taskENIBlockInstanceMetadataAttributeSuffix),
			})
		}
	}

	return capabilities
}

func (agent *ecsAgent) appendExecCapabilities(capabilities []*ecs.Attribute) ([]*ecs.Attribute, error) {

	// Only Windows 2019 and above are supported, all Linux supported
	if platformSupported, err := isPlatformExecSupported(); err != nil || !platformSupported {
		return capabilities, err
	}

	// for an instance to be exec-enabled, it needs resources needed by SSM (binaries, configuration files and certs)
	// the following bind mounts are defined in ecs-init and added to the ecs-agent container

	if exists, err := dependenciesExist(dependencies); err != nil || !exists {
		return capabilities, err
	}

	// ssm binaries are stored in /bin/<version>/, 1 version is downloaded by ami builder for ECS instances
	binDependencies := map[string][]string{}
	// child folders named by version inside binDir, e.g. 3.0.236.0
	binFolders, err := getSubDirectories(binDir)
	if err != nil {
		return capabilities, err
	}
	// use raw string for regular expression to avoid escaping backslash (\)
	var validSsmVersion = regexp.MustCompile(`^\d+(\.\d+)*$`)
	for _, binFolder := range binFolders {
		if matched := validSsmVersion.Match([]byte(binFolder)); !matched {
			continue
		}
		if _, found := capabilityExecInvalidSsmVersions[binFolder]; found {
			continue
		}

		// check for the same set of binaries for all versions for now
		// change this if future versions of ssm agent require different binaries
		versionSubDirectory := filepath.Join(binDir, binFolder)
		binDependencies[versionSubDirectory] = capabilityExecRequiredBinaries
	}
	if len(binDependencies) < 1 {
		return capabilities, nil
	}
	if exists, err := checkAnyValidDependency(binDependencies); err != nil || !exists {
		return capabilities, err
	}

	return appendNameOnlyAttribute(capabilities, attributePrefix+capabilityExec), nil
}

func (agent *ecsAgent) appendServiceConnectCapabilities(capabilities []*ecs.Attribute) []*ecs.Attribute {
	if loaded, _ := agent.serviceconnectManager.IsLoaded(agent.dockerClient); !loaded {
		_, err := agent.serviceconnectManager.LoadImage(agent.ctx, agent.cfg, agent.dockerClient)
		if err != nil {
			logger.Error("ServiceConnect Capability: Failed to load appnet Agent container. This container instance will not be able to support ServiceConnect tasks",
				logger.Fields{
					field.Error: err,
				},
			)
			return capabilities
		}
	}
	loadedVer, _ := agent.serviceconnectManager.GetLoadedAppnetVersion()
	supportedAppnetInterfaceVerToCapabilities, _ := agent.serviceconnectManager.GetCapabilitiesForAppnetInterfaceVersion(loadedVer)
	if supportedAppnetInterfaceVerToCapabilities == nil {
		logger.Warn("ServiceConnect Capability: No service connect capabilities were found for Appnet version:", logger.Fields{
			field.Image: loadedVer,
		},
		)
	}
	for _, serviceConnectCapability := range supportedAppnetInterfaceVerToCapabilities {
		capabilities = appendNameOnlyAttribute(capabilities, serviceConnectCapability)
	}
	return capabilities
}

func defaultGetSubDirectories(path string) ([]string, error) {
	var subDirectories []string

	fileInfos, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			subDirectories = append(subDirectories, fileInfo.Name())
		}
	}
	return subDirectories, nil
}

func dependenciesExist(dependencies map[string][]string) (bool, error) {
	for directory, files := range dependencies {
		if exists, err := pathExists(directory, true); err != nil || !exists {
			return false, err
		}

		for _, filename := range files {
			path := filepath.Join(directory, filename)
			if exists, err := pathExists(path, false); err != nil || !exists {
				return false, err
			}
		}
	}
	return true, nil
}

func checkAnyValidDependency(dependencies map[string][]string) (bool, error) {
	var validDependencies = 0
	for directory, files := range dependencies {
		filesValid := true
		if exists, err := pathExists(directory, true); err != nil || !exists {
			continue
		}

		for _, filename := range files {
			path := filepath.Join(directory, filename)
			if exists, err := pathExists(path, false); err != nil || !exists {
				filesValid = false
			}
		}

		if filesValid {
			validDependencies++
		}
	}
	if validDependencies >= 1 {
		return true, nil
	}
	return false, fmt.Errorf("no valid dependencies")
}

func defaultPathExists(path string, shouldBeDirectory bool) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	isDirectory := fileInfo.IsDir()
	return (isDirectory && shouldBeDirectory) || (!isDirectory && !shouldBeDirectory), nil
}

func appendNameOnlyAttribute(attributes []*ecs.Attribute, name string) []*ecs.Attribute {
	return append(attributes, &ecs.Attribute{
		Name: aws.String(name),
	})
}

func removeAttributesByNames(attributes []*ecs.Attribute, names []string) []*ecs.Attribute {
	nameMap := make(map[string]struct{})
	for _, name := range names {
		nameMap[name] = struct{}{}
	}

	var ret []*ecs.Attribute
	for _, attr := range attributes {
		if _, ok := nameMap[aws.StringValue(attr.Name)]; !ok {
			ret = append(ret, attr)
		}
	}
	return ret
}
