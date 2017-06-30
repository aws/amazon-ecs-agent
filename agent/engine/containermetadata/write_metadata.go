// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/cihub/seelog"
)

const (
	inspectContainerTimeout = 30 * time.Second
	metadataFile            = "metadata.json"
)

// getTaskIDfromArn parses a task Arn and produces the task ID
func getTaskIDfromArn(taskarn string) string {
	colonSplitArn := strings.SplitN(taskarn, ":", 6)
	// Incorrectly formatted Arn (Should not happen)
	if len(colonSplitArn) < 6 {
		return ""
	}
	arnTaskPartSplit := strings.SplitN(colonSplitArn[5], "/", 2)
	// Incorrectly formatted Arn (Should not happen)
	if len(arnTaskPartSplit) < 2 {
		return ""
	}
	return arnTaskPartSplit[1]
}

// getMetadataFilePath gives the metadata file path for any agent-managed container
func getMetadataFilePath(task *api.Task, container *api.Container, dataDir string) string {
	taskID := getTaskIDfromArn(task.Arn)
	// Empty task ID indicates malformed Arn (Should not happen)
	if taskID == "" {
		return ""
	}
	return fmt.Sprintf("%s/metadata/%s/%s/", dataDir, taskID, container.Name)
}

// writeToMetadata puts the metadata into JSON format and writes into
// the metadata file
func (md *Metadata) writeToMetadataFile(task *api.Task, container *api.Container, dataDir string) error {
	data, err := json.MarshalIndent(md, "", "\t")
	if err != nil {
		return err
	}
	mdFileDir := getMetadataFilePath(task, container, dataDir)
	mdFilePath := fmt.Sprintf("%s/%s", mdFileDir, metadataFile)

	mdFile, err := os.OpenFile(mdFilePath, os.O_WRONLY, 0644)
	defer mdFile.Close()
	if err != nil {
		return err
	}
	_, err = mdFile.Write(data)
	return err
}

// CreateMetadata creates the metadata file and adds the metadata directory to
// the container's mounted host volumes
func CreateMetadata(client dockerDummyClient, cfg *config.Config, binds *[]string, task *api.Task, container *api.Container) error {
	// Do not create metadata file for internal containers
	// TODO: Add error handling for this case? Probably no need since
	// Internal containers should not be visible to users anyways
	if container.IsInternal {
		return nil
	}

	// Create task and container directories if they do not yet exist
	mdDirectoryPath := getMetadataFilePath(task, container, cfg.DataDir)
	err := os.MkdirAll(mdDirectoryPath, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("Failed to create metadata directory at %s: %s", mdDirectoryPath, err.Error())
		return err
	}

	// Add the directory of this container's metadata to the container's mount binds
	instanceBind := fmt.Sprintf("%s/%s:/ecs/metadata/%s", cfg.InstanceDataDir, mdDirectoryPath, container.Name)
	*binds = append(*binds, instanceBind)

	// Create metadata file
	mdFilePath := fmt.Sprintf("%s/%s", mdDirectoryPath, metadataFile)
	err = ioutil.WriteFile(mdFilePath, nil, 0644)
	if err != nil {
		err = fmt.Errorf("Failed to create metadata file at %s: %s", mdFilePath, err.Error())
		return err
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := acquireMetadataAtContainerCreate(client, cfg, task)
	err = metadata.writeToMetadataFile(task, container, cfg.DataDir)
	if err != nil {
		err = fmt.Errorf("Failed to write to metadata file %s: %s", mdFilePath, err.Error())
		return err
	}
	return nil
}

// UpdateMetadata updates the metadata file after container starts and dynamic
// metadata is available
func UpdateMetadata(client dockerDummyClient, cfg *config.Config, dockerID string, task *api.Task, container *api.Container) error {
	// Do not update (non-existent) metadata file for internal containers
	if container.IsInternal {
		return nil
	}

	// Get docker container information through api call
	dockerContainer, err := client.InspectContainer(dockerID, inspectContainerTimeout)
	if err != nil {
		err = fmt.Errorf("Failed to inspect container %s of task %s: %s", container, task, err.Error())
		return err
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := acquireMetadata(client, dockerContainer, cfg, task)
	err = metadata.writeToMetadataFile(task, container, cfg.DataDir)
	if err != nil {
		err = fmt.Errorf("Failed to update metadata for container %s of task %s: %s", container, task, err.Error())
	} else {
		seelog.Debugf("Updated metadata file for task %s container %s", task, container)
	}
	return err
}

// getTaskMetadataDir acquires the directory with all of the metadata
// files of a given task
func getTaskMetadataDir(task *api.Task, dataDir string) string {
	return fmt.Sprintf("%s/metadata/%s/", dataDir, getTaskIDfromArn(task.Arn))
}

// removeContents removes a directory and all its children. We use this
// instead of os.RemoveAll to handle case where the directory does not exist
func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return nil
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return os.Remove(dir)
}

// CleanTaskMetadata removes the metadata files of all containers associated with a task
func CleanTaskMetadata(task *api.Task, dataDir string) error {
	mdPath := getTaskMetadataDir(task, dataDir)
	return removeContents(mdPath)
}
