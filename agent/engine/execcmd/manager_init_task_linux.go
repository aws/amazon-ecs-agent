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

	execCommandDepsDir = "/execute-command"
	execAgentConfigDir = execCommandDepsDir + "/config"
	// filePerm is the permission for the exec agent config file.
	filePerm = 0644
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

	// Append config file volume
	task.Volumes = append(task.Volumes,
		apitask.TaskVolume{
			Type: apitask.HostVolumeType,
			Name: configVolumeName,
			Volume: &taskresourcevolume.FSHostVolume{
				// TODO: Change this path after getting the config file name
				FSSourcePath: filepath.Join(m.hostBinDir, ConfigFileName),
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
			apicontainer.MountPoint{
				SourceVolume:  configVolumeName,
				ContainerPath: ContainerConfigFile,
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

func getExecAgentConfigFileName(sessionLimit int) (string, error) {
	config := fmt.Sprintf(execAgentConfigTemplate, sessionLimit)
	hash := getExecAgentConfigHash(config)
	// check if config file exists already
	configFilePath := filepath.Join(execAgentConfigDir, fmt.Sprintf(execAgentConfigFileNameTemplate, hash))
	if execAgentConfigFileExists(configFilePath) {
		return configFilePath, nil
	}
	// config doesn't exist; create a new one
	if err := createNewExecAgentConfigFile(config, configFilePath); err != nil {
		return "", err
	}
	return configFilePath, nil
}

func getExecAgentConfigHash(config string) string {
	hash := sha256.New()
	hash.Write([]byte(config))
	sha := base64.URLEncoding.EncodeToString(hash.Sum(nil))
	return sha
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
