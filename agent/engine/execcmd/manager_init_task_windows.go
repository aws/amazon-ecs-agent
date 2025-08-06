//go:build windows

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
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/config"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	folderPerm = 0755
)

var (
	ecsAgentExecDepsDir = config.AmazonECSProgramFiles + "\\managed-agents\\execute-command"

	// ecsAgentDepsBinDir is the directory where ECS Agent will read versions of SSM agent
	ecsAgentDepsBinDir = ecsAgentExecDepsDir + "\\bin"

	ContainerDepsFolder = config.AmazonProgramFiles + "\\SSM"

	SSMAgentBinName       = "amazon-ssm-agent.exe"
	SSMAgentWorkerBinName = "ssm-agent-worker.exe"
	SessionWorkerBinName  = "ssm-session-worker.exe"

	HostLogDir      = config.AmazonECSProgramData + "\\exec"
	ContainerLogDir = config.AmazonProgramData + "\\SSM"

	SSMPluginDir = config.AmazonProgramFiles + "\\SSM\\Plugins"
	// since ecs agent windows is not running in a container, the agent and host log dirs are the same
	ECSAgentExecLogDir = config.AmazonECSProgramData + "\\exec"

	// ECSAgentExecConfigDir is the directory where ECS Agent will write the ExecAgent config files to
	ECSAgentExecConfigDir = ecsAgentExecDepsDir + "\\" + ContainerConfigDirName
	// HostExecConfigDir is the dir where ExecAgents Config files will live
	HostExecConfigDir = hostExecDepsDir + "\\" + ContainerConfigDirName

	configFiles = []string{
		"amazon-ssm-agent.json",
		"seelog.xml",
	}

	execAgentLogConfigTemplate = `<!--amazon-ssm-agent uses seelog logging -->
<!--Seelog has github wiki pages, which contain detailed how-tos references: https://github.com/cihub/seelog/wiki -->
<!--Seelog examples can be found here: https://github.com/cihub/seelog-examples -->
<seelog type="adaptive" mininterval="2000000" maxinterval="100000000" critmsgcount="500" minlevel="info">
    <exceptions>
        <exception filepattern="test*" minlevel="error"/>
    </exceptions>
    <outputs formatid="fmtinfo">
        <console formatid="fmtinfo"/>
        <rollingfile type="size" filename="C:\ProgramData\Amazon\SSM\Logs\amazon-ssm-agent.log" maxsize="30000000" maxrolls="5"/>
        <filter levels="error,critical" formatid="fmterror">
            <rollingfile type="size" filename="C:\ProgramData\Amazon\SSM\Logs\errors.log" maxsize="10000000" maxrolls="5"/>
        </filter>
    </outputs>
    <formats>
        <format id="fmterror" format="%Date %Time %LEVEL [%FuncShort @ %File.%Line] %Msg%n"/>
        <format id="fmtdebug" format="%Date %Time %LEVEL [%FuncShort @ %File.%Line] %Msg%n"/>
        <format id="fmtinfo" format="%Date %Time %LEVEL %Msg%n"/>
    </formats>
</seelog>`
)

var GetExecAgentConfigDir = getAgentConfigDir

// Retrieves cached config dir, creates new one if needed
func getAgentConfigDir(sessionLimit int) (string, error) {
	agentConfig := fmt.Sprintf(execAgentConfigTemplate, sessionLimit)
	hash := getExecAgentConfigHash(agentConfig + execAgentLogConfigTemplate)
	// check if cached config dir exists already
	configDirPath := filepath.Join(ECSAgentExecConfigDir, hash)
	if isDir(configDirPath) && validConfigDirExists(configDirPath, hash) {
		return hash, nil
	}
	// check if config dir is a file; if true, remove it
	if fileExists(configDirPath) {
		if err := removeAll(configDirPath); err != nil {
			return "", err
		}
	}
	// create new config dir
	if err := createNewExecAgentConfigDir(agentConfig, configDirPath); err != nil {
		return "", err
	}
	return hash, nil
}

var createNewExecAgentConfigDir = createNewConfigDir

var mkdirAll = os.MkdirAll

func createNewConfigDir(agentConfig, configDirPath string) error {
	// make top level config directory
	err := mkdirAll(configDirPath, folderPerm)
	if err != nil {
		return err
	}

	// make individual config files
	agentConfigFilePath := filepath.Join(configDirPath, containerConfigFileName)
	err = createNewExecAgentConfigFile(agentConfig, agentConfigFilePath)
	if err != nil {
		return err
	}

	logConfigFilePath := filepath.Join(configDirPath, ExecAgentLogConfigFileName)
	err = createNewExecAgentConfigFile(execAgentLogConfigTemplate, logConfigFilePath)
	if err != nil {
		return err
	}

	return nil
}

func validConfigDirExists(configDirPath, expectedHash string) bool {
	// check that each config file exists and create a hash of their summed contents
	contentSum := ""
	for _, file := range configFiles {
		if isDir(filepath.Join(configDirPath, file)) {
			return false
		}
		config, err := getFileContent(filepath.Join(configDirPath, file))
		if err != nil {
			return false
		}
		contentSum += string(config)
	}

	hash := getExecAgentConfigHash(contentSum)
	return strings.Compare(expectedHash, hash) == 0
}

// This function creates any necessary config directories/files and ensures that
// the ssm-agent binaries, configs, logs, and plugin is bind mounted
func addRequiredBindMounts(taskId, cn, latestBinVersionDir, uuid string, sessionWorkersLimit int, hostConfig *dockercontainer.HostConfig) error {
	// In windows host mounts are not created automatically, so need to create
	rErr := os.MkdirAll(filepath.Join(HostLogDir, taskId, cn), folderPerm)
	if rErr != nil {
		return rErr
	}

	configDirHash, rErr := GetExecAgentConfigDir(sessionWorkersLimit)
	if rErr != nil {
		rErr = fmt.Errorf("could not generate ExecAgent Config dir: %v", rErr)
		return rErr
	}

	// Add ssm binary mount
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		latestBinVersionDir,
		ContainerDepsFolder))

	// Add ssm configuration dir mount
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(ECSAgentExecConfigDir, configDirHash),
		filepath.Join(ContainerDepsFolder, "configuration")))

	// Add ssm log bind mount
	hostConfig.Binds = append(hostConfig.Binds, getBindMountMapping(
		filepath.Join(HostLogDir, taskId, cn),
		ContainerLogDir))

	// add ssm plugin bind mount (needed for execcmd windows)
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		SSMPluginDir,
		SSMPluginDir))

	return nil
}
