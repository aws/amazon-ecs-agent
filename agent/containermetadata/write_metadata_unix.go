//go:build !windows
// +build !windows

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

package containermetadata

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"

	"github.com/cihub/seelog"
	"github.com/pborman/uuid"
)

const (
	mountPoint            = "/opt/ecs/metadata"
	tempFile              = "temp_metadata_file"
	bindMode              = "Z"
	selinuxSecurityOption = "selinux"
)

// createBindsEnv will do the appropriate formatting to add a new mount in a container's HostConfig
// and add the metadata file path as an environment variable ECS_CONTAINER_METADATA_FILE
// We add an additional uuid to the path to ensure it does not conflict with user mounts
func createBindsEnv(binds []string, env []string, dataDirOnHost string, metadataDirectoryPath string, dockerSecurityOptions []string) ([]string, []string) {
	selinuxEnabled := false
	for _, option := range dockerSecurityOptions {
		if option == selinuxSecurityOption {
			selinuxEnabled = true
		}
	}

	randID := uuid.New()
	instanceBind := fmt.Sprintf(`%s/%s:%s/%s`, dataDirOnHost, metadataDirectoryPath, mountPoint, randID)
	if selinuxEnabled {
		seelog.Info("Selinux is enabled on docker, mounting data directory in Z mode")
		instanceBind = fmt.Sprintf(`%s:%s`, instanceBind, bindMode)
	}
	metadataEnvVariable := fmt.Sprintf("%s=%s/%s/%s", metadataEnvironmentVariable, mountPoint, randID, metadataFile)
	binds = append(binds, instanceBind)
	env = append(env, metadataEnvVariable)
	return binds, env
}

var rename = os.Rename

var TempFile = func(dir, pattern string) (oswrapper.File, error) {
	return ioutil.TempFile(dir, pattern)
}

// writeToMetadata puts the metadata into JSON format and writes into
// the metadata file
func writeToMetadataFile(data []byte, taskARN string, containerName string, dataDir string) error {
	metadataFileDir, err := getMetadataFilePath(taskARN, containerName, dataDir)
	// Boundary case if file path is bad (Such as if task arn is incorrectly formatted)
	if err != nil {
		return fmt.Errorf("write to metadata file for task %s container %s: %v", taskARN, containerName, err)
	}
	metadataFileName := filepath.Join(metadataFileDir, metadataFile)

	temp, err := TempFile(metadataFileDir, tempFile)
	if err != nil {
		return err
	}
	defer temp.Close()
	_, err = temp.Write(data)
	if err != nil {
		return err
	}
	err = temp.Chmod(metadataPerm)
	if err != nil {
		return err
	}
	return rename(temp.Name(), metadataFileName)
}
