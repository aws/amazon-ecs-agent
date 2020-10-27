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
	"strings"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
)

const (
	// TODO: [ecs-exec] decide if this needs to be configurable or put in a specific place in our optimized AMIs
	ContainerBinDir      = "/usr/bin"
	BinName              = "amazon-ssm-agent"
	SessionWorkerBinName = "ssm-session-worker"
	ConfigFileName       = "amazon-ssm-agent.json"

	// TODO: [ecs-exec] decide if this needs to be configurable or put in a specific place in our optimized AMIs
	HostLogDir          = "/var/log/ecs/exec"
	ContainerLogDir     = "/var/log/amazon/ssm"
	ECSAgentExecLogDir  = "/log/exec"
	HostCertFile        = "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
	ContainerCertFile   = "/etc/ssl/certs/ca-certificates.crt"
	ContainerConfigFile = "/etc/amazon/ssm/amazon-ssm-agent.json"

	namelessContainerPrefix = "nameless-container-"
	internalNamePrefix      = "internal-execute-command-agent"
	logVolumeNamePrefix     = internalNamePrefix + "-log"
	certVolumeName          = internalNamePrefix + "-tls-cert"
	configVolumeName        = internalNamePrefix + "-config"

	ecsAgentExecDepsDir = "/managed-agents/execute-command"
	// ECSAgentExecConfigDir is the directory where ECS Agent will write the
	// ExecAgent config files to
	ECSAgentExecConfigDir = ecsAgentExecDepsDir + "/config"
	hostExecDepsDir       = "/var/lib/ecs/deps/execute-command"
	// HostExecConfigDir is the dir where ExecAgents Config files will live
	HostExecConfigDir = hostExecDepsDir + "/config"
	// filePerm is the permission for the exec agent config file.
	filePerm            = 0644
	defaultSessionLimit = 2
)

var (
	execAgentConfigTemplate = `
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
	}`
	execAgentConfigFileNameTemplate = `amazon-ssm-agent-%s.json`
)

// InitializeTask adds the necessary volumes and mount points in all of the task containers in order for the
// exec cmd agent to run upon container start up.
// TODO: [ecs-exec] Should we validate the ssm agent binaries & certs are valid and fail here if they're not? (bind mount will succeed even if files don't exist in host)
func (m *manager) InitializeTask(task *apitask.Task) error {
	if !task.IsExecCommandAgentEnabled() {
		return nil
	}

	tId, err := task.GetID()
	if err != nil {
		return err
	}

	execCommandAgentBinNames := []string{BinName, SessionWorkerBinName}

	// Append an internal volume for each of the exec agent binary names
	for _, bn := range execCommandAgentBinNames {
		task.Volumes = append(task.Volumes,
			apitask.TaskVolume{
				Type: apitask.HostVolumeType,
				Name: buildVolumeNameForBinary(bn),
				Volume: &taskresourcevolume.FSHostVolume{
					FSSourcePath: filepath.Join(m.hostBinDir, bn),
				},
			})
	}

	// Append certificates volume
	task.Volumes = append(task.Volumes,
		apitask.TaskVolume{
			Type: apitask.HostVolumeType,
			Name: certVolumeName,
			Volume: &taskresourcevolume.FSHostVolume{
				FSSourcePath: HostCertFile,
			},
		})

	// Add log volumes and mount points to all containers in this task
	for _, c := range task.Containers {
		lvn := fmt.Sprintf("%s-%s-%s", logVolumeNamePrefix, tId, c.Name)
		cn := buildContainerNameForBinary(c)
		task.Volumes = append(task.Volumes, apitask.TaskVolume{
			Type: apitask.HostVolumeType,
			Name: lvn,
			Volume: &taskresourcevolume.FSHostVolume{
				FSSourcePath: filepath.Join(HostLogDir, tId, cn),
			},
		})

		c.MountPoints = append(c.MountPoints,
			apicontainer.MountPoint{
				SourceVolume:  lvn,
				ContainerPath: ContainerLogDir,
				ReadOnly:      false,
			},
			apicontainer.MountPoint{
				SourceVolume:  certVolumeName,
				ContainerPath: ContainerCertFile,
				ReadOnly:      true,
			},
		)

		for _, bn := range execCommandAgentBinNames {
			c.MountPoints = append(c.MountPoints,
				apicontainer.MountPoint{
					SourceVolume:  buildVolumeNameForBinary(bn),
					ContainerPath: filepath.Join(ContainerBinDir, bn),
					ReadOnly:      true,
				})
		}
	}
	return nil
}

func buildVolumeNameForBinary(binaryName string) string {
	return fmt.Sprintf("%s-%s", internalNamePrefix, binaryName)
}

func buildContainerNameForBinary(c *apicontainer.Container) string {
	// Trim leading hyphens since they're not valid directory names
	cn := strings.TrimLeft(c.Name, "-")
	if cn == "" {
		// Fallback name in the extreme case that we end up with an empty string after trimming all leading hyphens.
		return namelessContainerPrefix + uuid.New()
	}
	return cn
}

// AddAgentConfigMount adds the ExecAgentConfigFile to the hostConfig binds
func (m *manager) AddAgentConfigMount(hostConfig *dockercontainer.HostConfig, execMD apicontainer.ExecCommandAgentMetadata) error {
	sessionLimit := execMD.SessionLimit
	// TODO: change this to < 0 if 0 session is valid
	if sessionLimit <= 0 {
		sessionLimit = defaultSessionLimit
	}
	configFile, err := GetExecAgentConfigFileName(sessionLimit)
	if err != nil {
		return fmt.Errorf("could not generate ExecAgent Config File: %v", err)
	}
	configBindMount := filepath.Join(HostExecConfigDir, configFile) + ":" + ContainerConfigFile + ":ro"
	hostConfig.Binds = append(hostConfig.Binds, configBindMount)
	return nil
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
