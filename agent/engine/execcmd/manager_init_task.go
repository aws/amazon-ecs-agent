//go:build linux || windows

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
	"sort"
	"strconv"
	"strings"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	dockercontainer "github.com/docker/docker/api/types/container"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/pborman/uuid"
)

const (
	namelessContainerPrefix = "nameless-container-"

	// filePerm is the permission for the exec agent config file.
	filePerm            = 0644
	defaultSessionLimit = 2

	containerConfigFileName = "amazon-ssm-agent.json"
	ContainerConfigDirName  = "config"

	ExecAgentLogConfigFileName = "seelog.xml"
)

var (
	execAgentConfigTemplate = `{
	"Mgs": {
		"Region": "",
		"Endpoint": "",
		"StopTimeoutMillis": 20000,
		"SessionWorkersLimit": %d
	},
	"Agent": {
		"Region": "",
		"OrchestrationRootDir": "",
		"ContainerMode": true
	}
}`

	errExecCommandManagedAgentNotFound = fmt.Errorf("managed agent not found (%s)", ExecuteCommandAgentName)
)

// InitializeContainer adds the necessary bind mounts in order for the ExecCommandAgent to run properly in the container
// TODO: [ecs-exec] Should we validate the ssm agent binaries & certs are valid and fail here if they're not? (bind mount will succeed even if files don't exist in host)
func (m *manager) InitializeContainer(taskId string, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) (rErr error) {
	defer func() {
		if rErr != nil {
			container.UpdateManagedAgentByName(ExecuteCommandAgentName, apicontainer.ManagedAgentState{
				InitFailed: true,
				Status:     apicontainerstatus.ManagedAgentStopped,
				Reason:     rErr.Error(),
			})
		}
	}()
	ma, ok := container.GetManagedAgentByName(ExecuteCommandAgentName)
	if !ok {
		return errExecCommandManagedAgentNotFound
	}
	sessionWorkersLimit := getSessionWorkersLimit(ma)
	cn := fileSystemSafeContainerName(container)
	uuid := newUUID()

	latestBinVersionDir, rErr := m.getLatestVersionedHostBinDir()
	if rErr != nil {
		return rErr
	}

	rErr = addRequiredBindMounts(taskId, cn, latestBinVersionDir, uuid, sessionWorkersLimit, hostConfig)
	if rErr != nil {
		return rErr
	}

	container.UpdateManagedAgentByName(ExecuteCommandAgentName, apicontainer.ManagedAgentState{
		ID: uuid,
	})

	return nil
}

func (m *manager) getLatestVersionedHostBinDir() (string, error) {
	versions, err := retrieveAgentVersions(ecsAgentDepsBinDir)
	if err != nil {
		return "", err
	}
	sort.Sort(sort.Reverse(byAgentVersion(versions)))

	var latest string
	for _, v := range versions {
		vStr := v.String()
		ecsAgentDepsVersionedBinDir := filepath.Join(ecsAgentDepsBinDir, vStr)
		if !fileExists(filepath.Join(ecsAgentDepsVersionedBinDir, SSMAgentBinName)) {
			continue // try falling back to the previous version
		}
		// TODO: [ecs-exec] This requirement will be removed for SSM agent V2
		if !fileExists(filepath.Join(ecsAgentDepsVersionedBinDir, SSMAgentWorkerBinName)) {
			continue // try falling back to the previous version
		}
		if !fileExists(filepath.Join(ecsAgentDepsVersionedBinDir, SessionWorkerBinName)) {
			continue // try falling back to the previous version
		}
		latest = filepath.Join(m.hostBinDir, vStr)
		break
	}
	if latest == "" {
		return "", fmt.Errorf("no valid versions were found in %s", m.hostBinDir)
	}
	return latest, nil
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
	limitStr, ok := ma.Properties["sessionLimit"]
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

var removeAll = os.RemoveAll

var getFileContent = readFileContent

func readFileContent(filePath string) ([]byte, error) {
	return ioutil.ReadFile(filePath)
}

func getExecAgentConfigHash(config string) string {
	hash := sha256.New()
	hash.Write([]byte(config))
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
}

var osStat = os.Stat

func fileExists(path string) bool {
	if fi, err := osStat(path); err == nil {
		return !fi.IsDir()
	}
	return false
}

func isDir(path string) bool {
	if fi, err := osStat(path); err == nil {
		return fi.IsDir()
	}
	return false
}

var createNewExecAgentConfigFile = createNewConfigFile

func createNewConfigFile(config, configFilePath string) error {
	return ioutil.WriteFile(configFilePath, []byte(config), filePerm)
}
