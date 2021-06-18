//go:build linux

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
package execcmd

import (
	"fmt"
	"path/filepath"
	"strings"

	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	ecsAgentExecDepsDir = "/managed-agents/execute-command"

	// ecsAgentDepsBinDir is the directory where ECS Agent will read versions of SSM agent
	ecsAgentDepsBinDir   = ecsAgentExecDepsDir + "/bin"
	ecsAgentDepsCertsDir = ecsAgentExecDepsDir + "/certs"

	ContainerDepsDirPrefix = "/ecs-execute-command-"

	SSMAgentBinName       = "amazon-ssm-agent"
	SSMAgentWorkerBinName = "ssm-agent-worker"
	SessionWorkerBinName  = "ssm-session-worker"

	HostLogDir         = "/var/log/ecs/exec"
	ContainerLogDir    = "/var/log/amazon/ssm"
	ECSAgentExecLogDir = "/log/exec"

	HostCertFile            = "/var/lib/ecs/deps/execute-command/certs/tls-ca-bundle.pem"
	ContainerCertFileSuffix = "certs/amazon-ssm-agent.crt"

	ContainerConfigFileSuffix = "configuration/" + containerConfigFileName

	// ECSAgentExecConfigDir is the directory where ECS Agent will write the ExecAgent config files to
	ECSAgentExecConfigDir = ecsAgentExecDepsDir + "/" + ContainerConfigDirName
	// HostExecConfigDir is the dir where ExecAgents Config files will live
	HostExecConfigDir      = hostExecDepsDir + "/" + ContainerConfigDirName
	ContainerLogConfigFile = "configuration/" + ExecAgentLogConfigFileName
)

var (
	execAgentLogConfigTemplate = `<seelog type="adaptive" mininterval="2000000"
	maxinterval="100000000" critmsgcount="500" minlevel="info">
<exceptions>
<exception filepattern="test*" minlevel="error"/>
</exceptions>
<outputs formatid="fmtinfo">
<console formatid="fmtinfo"/>
<rollingfile type="size"
	filename="/var/log/amazon/ssm/amazon-ssm-agent.log"
	maxsize="40000000" maxrolls="1"/>
<filter levels="error,critical" formatid="fmterror">
   <rollingfile type="size"
		filename="/var/log/amazon/ssm/errors.log"
		maxsize="10000000" maxrolls="1"/>
</filter>
</outputs>
<formats>
<format id="fmterror" format="%Date %Time %LEVEL [%FuncShort @ %File.%Line] %Msg%n"/>
<format id="fmtdebug" format="%Date %Time %LEVEL [%FuncShort @ %File.%Line] %Msg%n"/>
<format id="fmtinfo" format="%Date %Time %LEVEL %Msg%n"/>
</formats>
</seelog>`
	// TODO: [ecs-exec] seelog config needs to be implemented following a similar approach to ss, config
	execAgentConfigFileNameTemplate = `amazon-ssm-agent-%s.json`
	logConfigFileNameTemplate       = `seelog-%s.xml`
)

var GetExecAgentLogConfigFile = getAgentLogConfigFile

func getAgentLogConfigFile() (string, error) {
	hash := getExecAgentConfigHash(execAgentLogConfigTemplate)
	logConfigFileName := fmt.Sprintf(logConfigFileNameTemplate, hash)
	// check if config file exists already
	logConfigFilePath := filepath.Join(ECSAgentExecConfigDir, logConfigFileName)
	if fileExists(logConfigFilePath) && validConfigExists(logConfigFilePath, hash) {
		return logConfigFileName, nil
	}
	// check if config file is a dir; if true, remove it
	if isDir(logConfigFilePath) {
		if err := removeAll(logConfigFilePath); err != nil {
			return "", err
		}
	}
	// create new seelog config file
	if err := createNewExecAgentConfigFile(execAgentLogConfigTemplate, logConfigFilePath); err != nil {
		return "", err
	}
	return logConfigFileName, nil
}

func validConfigExists(configFilePath, expectedHash string) bool {
	config, err := getFileContent(configFilePath)
	if err != nil {
		return false
	}
	hash := getExecAgentConfigHash(string(config))
	return strings.Compare(expectedHash, hash) == 0
}

var GetExecAgentConfigFileName = getAgentConfigFileName

func getAgentConfigFileName(sessionLimit int) (string, error) {
	config := fmt.Sprintf(execAgentConfigTemplate, sessionLimit)
	hash := getExecAgentConfigHash(config)
	configFileName := fmt.Sprintf(execAgentConfigFileNameTemplate, hash)
	// check if config file exists already
	configFilePath := filepath.Join(ECSAgentExecConfigDir, configFileName)
	if fileExists(configFilePath) && validConfigExists(configFilePath, hash) {
		return configFileName, nil
	}
	// check if config file is a dir; if true, remove it
	if isDir(configFilePath) {
		if err := removeAll(configFilePath); err != nil {
			return "", err
		}
	}
	// config doesn't exist; create a new one
	if err := createNewExecAgentConfigFile(config, configFilePath); err != nil {
		return "", err
	}
	return configFileName, nil
}

func certsExist() bool {
	return fileExists(filepath.Join(ecsAgentDepsCertsDir, "tls-ca-bundle.pem"))
}

// This function creates any necessary config directories/files and ensures that
// the ssm-agent binaries, configs, logs, and plugin is bind mounted
func addRequiredBindMounts(taskId, cn, latestBinVersionDir, uuid string, sessionWorkersLimit int, hostConfig *dockercontainer.HostConfig) error {
	configFile, rErr := GetExecAgentConfigFileName(sessionWorkersLimit)
	if rErr != nil {
		rErr = fmt.Errorf("could not generate ExecAgent Config File: %v", rErr)
		return rErr
	}
	logConfigFile, rErr := GetExecAgentLogConfigFile()
	if rErr != nil {
		rErr = fmt.Errorf("could not generate ExecAgent LogConfig file: %v", rErr)
		return rErr
	}

	containerDepsFolder := ContainerDepsDirPrefix + uuid

	if !certsExist() {
		rErr = fmt.Errorf("could not find certs")
		return rErr
	}

	// Add ssm binary mounts
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(latestBinVersionDir, SSMAgentBinName),
		filepath.Join(containerDepsFolder, SSMAgentBinName)))

	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(latestBinVersionDir, SSMAgentWorkerBinName),
		filepath.Join(containerDepsFolder, SSMAgentWorkerBinName)))

	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(latestBinVersionDir, SessionWorkerBinName),
		filepath.Join(containerDepsFolder, SessionWorkerBinName)))

	// Add exec agent config file mount
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(HostExecConfigDir, configFile),
		filepath.Join(containerDepsFolder, ContainerConfigFileSuffix)))

	// Add exec agent log config file mount
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(HostExecConfigDir, logConfigFile),
		filepath.Join(containerDepsFolder, ContainerLogConfigFile)))

	// Append TLS cert mount
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		HostCertFile,
		filepath.Join(containerDepsFolder, ContainerCertFileSuffix)))

	// Add ssm log bind mount
	hostConfig.Binds = append(hostConfig.Binds, getBindMountMapping(
		filepath.Join(HostLogDir, taskId, cn),
		ContainerLogDir))
	return nil
}
