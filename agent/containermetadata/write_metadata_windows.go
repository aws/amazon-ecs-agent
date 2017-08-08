// +build windows

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"os"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"

	"github.com/pborman/uuid"
)

const (
	mountPoint                     = `C:\ProgramData\Amazon\ECS\metadata`
	ContainerMetadataClientVersion = dockerclient.Version_1_24
)

// createBindsEnv will do the appropriate formatting to add a new mount in a container's HostConfig
// and add the metadata file path as an environment variable ECS_CONTAINER_METADATA_FILE
func createBindsEnv(binds []string, env []string, dataDirOnHost string, metadataDirectoryPath string) ([]string, []string) {
	randID := uuid.New()
	instanceBind := fmt.Sprintf(`%s:%s\%s`, metadataDirectoryPath, mountPoint, randID)
	metadataEnvVariable := fmt.Sprintf(`%s=%s\%s\%s`, metadataEnvironmentVariable, mountPoint, randID, metadataFile)
	binds = append(binds, instanceBind)
	env = append(env, metadataEnvVariable)
	return binds, env
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

	file, err := os.OpenFile(metadataFileName, os.O_WRONLY, metadataPerm)
	if err != nil {
		// Retry if file does not exist
		if os.IsNotExist(err) {
			file, err = os.Create(metadataFileName)
		}
		if err != nil {
			return err
		}
	}
	defer file.Close()
	_, err = file.Write(data)
	if err != nil {
		return err
	}
	return file.Sync()
}
