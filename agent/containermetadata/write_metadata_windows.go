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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
)

const (
	mountPoint                     = `C:\ProgramData\Amazon\ECS\metadata`
	ContainerMetadataClientVersion = dockerclient.Version_1_24
)

// createMetadataFile initializes the metadata file
func createMetadataFile(metadataDirectoryPath) error {
	metadataFilePath := filepath.Join(metadataDirectoryPath, metadataFile)
	//	err := ioutil.WriteFile(metadataFilePath, nil, readOnlyPerm)
	file, err := os.Create(metadataFilePath)
	if err != nil {
		return err
	}
	defer file.Close()
	err = file.Chmod(readOnlyPerm)
	if err != nil {
		return err
	}
	return file.Sync()
}

// createBinds will do the appropriate formatting to add a new mount in a container's HostConfig
func createBinds(binds []string, dataDirOnHost string, metadataDirectoryPath string, mountPoint string, containerName string) []string {
	instanceBind := fmt.Sprintf(`%s:%s\%s`, metadataDirectoryPath, mountPoint, containerName)
	binds = append(binds, instanceBind)
	return binds
}

// writeToMetadata puts the metadata into JSON format and writes into
// the metadata file
func (md *Metadata) writeToMetadataFile(task *api.Task, container *api.Container, dataDir string) error {
	data, err := json.MarshalIndent(md, "", "\t")
	if err != nil {
		return err
	}
	metadataFileDir, err := getMetadataFilePath(task, container, dataDir)
	// Boundary case if file path is bad (Such as if task arn is incorrectly formatted)
	if err != nil {
		return fmt.Errorf("Failed to write to metadata file: %v", err)
	}
	metadataFilePath := filepath.Join(metadataFileDir, metadataFile)

	file, err := os.OpenFile(metadataFilePath, os.O_WRONLY, readOnlyPerm)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	if err != nil {
		return err
	}
	return file.Sync()
}
