// +build linux

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
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
)

const (
	namelessContainerPrefix = "nameless-container-"

	ecsAgentExecDepsDir = "/managed-agents/execute-command"

	ContainerDepsDirPrefix = "/ecs-execute-command-"

	// filePerm is the permission for the exec agent config file.
	filePerm            = 0644
	defaultSessionLimit = 2

	SSMAgentBinName       = "amazon-ssm-agent"
	SSMAgentWorkerBinName = "ssm-agent-worker"
	SessionWorkerBinName  = "ssm-session-worker"

	HostLogDir         = "/var/log/ecs/exec"
	ContainerLogDir    = "/var/log/amazon/ssm"
	ECSAgentExecLogDir = "/log/exec"

	HostCertFile            = "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
	ContainerCertFileSuffix = "certs/amazon-ssm-agent.crt"

	containerConfigFileName   = "amazon-ssm-agent.json"
	ContainerConfigDirName    = "config"
	ContainerConfigFileSuffix = "configuration/" + containerConfigFileName

	// ECSAgentExecConfigDir is the directory where ECS Agent will write the ExecAgent config files to
	ECSAgentExecConfigDir = ecsAgentExecDepsDir + "/" + ContainerConfigDirName
	// HostExecConfigDir is the dir where ExecAgents Config files will live
	HostExecConfigDir = hostExecDepsDir + "/" + ContainerConfigDirName
)

var (
	execAgentConfigTemplate = `{
	"Agent": {
		"Region": "",
		"OrchestrationRootDir": "",
		"ContainerMode": true,
		"Mgs": {
			"Region": "",
			"Endpoint": "",
			"StopTimeoutMillis": 20000,
			"SessionWorkersLimit": %d
		}
	}
}`
	// TODO: [ecs-exec] seelog config needs to be implemented following a similar approach to ss, config
	execAgentConfigFileNameTemplate    = `amazon-ssm-agent-%s.json`
	errExecCommandManagedAgentNotFound = fmt.Errorf("managed agent not found (%s)", ExecuteCommandAgentName)
)

// InitializeContainer adds the necessary bind mounts in order for the ExecCommandAgent to run properly in the container
// TODO: [ecs-exec] Should we validate the ssm agent binaries & certs are valid and fail here if they're not? (bind mount will succeed even if files don't exist in host)
func (m *manager) InitializeContainer(taskId string, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error {
	ma, ok := container.GetManagedAgentByName(ExecuteCommandAgentName)
	if !ok {
		return errExecCommandManagedAgentNotFound
	}
	configFile, err := GetExecAgentConfigFileName(getSessionWorkersLimit(ma))
	if err != nil {
		return fmt.Errorf("could not generate ExecAgent Config File: %v", err)
	}

	uuid := newUUID()
	containerDepsFolder := ContainerDepsDirPrefix + uuid

	// Add ssm binary mounts
	// TODO: [ecs-exec]: m.hostBinDir should determine the latest version of ssm agent available in the deps folder
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(m.hostBinDir, SSMAgentBinName),
		filepath.Join(containerDepsFolder, SSMAgentBinName)))

	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(m.hostBinDir, SSMAgentWorkerBinName),
		filepath.Join(containerDepsFolder, SSMAgentWorkerBinName)))

	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(m.hostBinDir, SessionWorkerBinName),
		filepath.Join(containerDepsFolder, SessionWorkerBinName)))

	// Add ssm agent config file mount
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		filepath.Join(HostExecConfigDir, configFile),
		filepath.Join(containerDepsFolder, ContainerConfigFileSuffix)))

	// Append TLS cert mount
	hostConfig.Binds = append(hostConfig.Binds, getReadOnlyBindMountMapping(
		HostCertFile, // TODO: [ecs-exec] decision pending - review the location of the certs in the host
		filepath.Join(containerDepsFolder, ContainerCertFileSuffix)))

	// Add ssm log bind mount
	cn := fileSystemSafeContainerName(container)
	hostConfig.Binds = append(hostConfig.Binds, getBindMountMapping(
		filepath.Join(HostLogDir, taskId, cn),
		ContainerLogDir))

	container.UpdateManagedAgentByName(ExecuteCommandAgentName, apicontainer.ManagedAgentState{
		ID: uuid,
	})

	return nil
}

func getReadOnlyBindMountMapping(hostDir, containerDir string) string {
	return getBindMountMapping(hostDir, containerDir) + ":ro"
}

func getBindMountMapping(hostDir, containerDir string) string {
	return hostDir + ":" + containerDir
}

var newUUID = uuid.New

func fileSystemSafeContainerName(c *apicontainer.Container) string {
	// Trim leading hyphens since they're not valid directory names
	cn := strings.TrimLeft(c.Name, "-")
	if cn == "" {
		// Fallback name in the extreme case that we end up with an empty string after trimming all leading hyphens.
		return namelessContainerPrefix + newUUID()
	}
	return cn
}

func getSessionWorkersLimit(ma apicontainer.ManagedAgent) int {
	// TODO [ecs-exec] : verify that returning the default session limit (2) is ok in case of any errors, misconfiguration
	limit := defaultSessionLimit
	if ma.Properties == nil { // This means ACS didn't send the limit
		return limit
	}
	limitStr, ok := ma.Properties["SessionWorkersLimit"]
	if !ok { // This also means ACS didn't send the limit
		return limit
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil { // This means ACS send a limit that can't be converted to an int
		return limit
	}
	if limit <= 0 {
		limit = defaultSessionLimit
	}
	return limit
}

var GetExecAgentConfigFileName = getAgentConfigFileName

func getAgentConfigFileName(sessionLimit int) (string, error) {
	config := fmt.Sprintf(execAgentConfigTemplate, sessionLimit)
	hash := getExecAgentConfigHash(config)
	configFileName := fmt.Sprintf(execAgentConfigFileNameTemplate, hash)
	// check if config file exists already
	configFilePath := filepath.Join(ECSAgentExecConfigDir, configFileName)
	if execAgentConfigFileExists(configFilePath) {
		// TODO: verify the hash of the existing file contents
		return configFileName, nil
	}
	// config doesn't exist; create a new one
	if err := createNewExecAgentConfigFile(config, configFilePath); err != nil {
		return "", err
	}
	return configFileName, nil
}

func getExecAgentConfigHash(config string) string {
	hash := sha256.New()
	hash.Write([]byte(config))
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
}

var execAgentConfigFileExists = configFileExists

func configFileExists(configFilePath string) bool {
	if _, err := os.Stat(configFilePath); err == nil {
		return true
	}
	return false
}

var createNewExecAgentConfigFile = createNewConfigFile

func createNewConfigFile(config, configFilePath string) error {
	return ioutil.WriteFile(configFilePath, []byte(config), filePerm)
}
