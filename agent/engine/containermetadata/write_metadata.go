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
	return filepath.Join(dataDir, "metadata", taskID, container.Name)
}

// mdFileExist checks if metadata file exists or not
func mdFileExist(task *api.Task, container *api.Container, dataDir string) bool {
	mdFileDir := getMetadataFilePath(task, container, dataDir)
	mdFilePath := filepath.Join(mdFileDir, metadataFile)
	if _, err := os.Stat(mdFilePath); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// writeToMetadata puts the metadata into JSON format and writes into
// the metadata file
func (md *Metadata) writeToMetadataFile(task *api.Task, container *api.Container, dataDir string) error {
	data, err := json.MarshalIndent(md, "", "\t")
	if err != nil {
		return err
	}
	mdFileDir := getMetadataFilePath(task, container, dataDir)
	mdFilePath := filepath.Join(mdFileDir, metadataFile)

	mdFile, err := os.OpenFile(mdFilePath, os.O_WRONLY, 0644)
	defer mdFile.Close()
	if err != nil {
		return err
	}
	_, err = mdFile.Write(data)
	return err
}

// getTaskMetadataDir acquires the directory with all of the metadata
// files of a given task
func getTaskMetadataDir(task *api.Task, dataDir string) string {
	return filepath.Join(dataDir, "metadata", getTaskIDfromArn(task.Arn))
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

// MetadataManager is an interface that allows us to abstract away the metadata
// operations
type MetadataManager interface {
	CreateMetadata(*[]string, *api.Task, *api.Container) error
	UpdateMetadata(string, *api.Task, *api.Container) error
	CleanTaskMetadata(*api.Task) error
}

// metadataManager implements the MetadataManager interface
type metadataManager struct {
	client dockerDummyClient
	cfg    *config.Config
}

// NewMetadataManager creates a metadataManager for a given DockerTaskEngine settings.
func NewMetadataManager(client dockerDummyClient, cfg *config.Config) MetadataManager {
	manager := &metadataManager{
		client: client,
		cfg:    cfg,
	}
	return manager
}

// CreateMetadata creates the metadata file and adds the metadata directory to
// the container's mounted host volumes
func (manager *metadataManager) CreateMetadata(binds *[]string, task *api.Task, container *api.Container) error {
	// Check if manager has invalid entries
	var err error
	if manager.cfg == nil {
		err = fmt.Errorf("Failed to create metadata: Invalid inputs")
		return err
	}

	// Do not create metadata file for internal containers
	// Add error handling for this case? Probably no need since
	// Internal containers should not be visible to users anyways
	if container.IsInternal {
		return nil
	}

	// Create task and container directories if they do not yet exist
	mdDirectoryPath := getMetadataFilePath(task, container, manager.cfg.DataDir)
	err = os.MkdirAll(mdDirectoryPath, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("Failed to create metadata directory at %s: %s", mdDirectoryPath, err.Error())
		return err
	}

	// Add the directory of this container's metadata to the container's mount binds
	// This is the only operating system specific point here, so it would be nice if there
	// were some elegant way to do this for both windows and linux at the same time
	instanceBind := fmt.Sprintf("%s/%s:/ecs/metadata/%s", manager.cfg.InstanceDataDir, mdDirectoryPath, container.Name)
	*binds = append(*binds, instanceBind)

	// Create metadata file
	mdFilePath := filepath.Join(mdDirectoryPath, metadataFile)
	err = ioutil.WriteFile(mdFilePath, nil, 0644)
	if err != nil {
		err = fmt.Errorf("Failed to create metadata file at %s: %s", mdFilePath, err.Error())
		return err
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := acquireMetadataAtContainerCreate(manager.client, manager.cfg, task)
	err = metadata.writeToMetadataFile(task, container, manager.cfg.DataDir)
	if err != nil {
		err = fmt.Errorf("Failed to write to metadata file %s: %s", mdFilePath, err.Error())
		return err
	}
	return nil
}

// UpdateMetadata updates the metadata file after container starts and dynamic
// metadata is available
func (manager *metadataManager) UpdateMetadata(dockerID string, task *api.Task, container *api.Container) error {
	// Check if manager has invalid entries
	var err error
	if manager.cfg == nil {
		err = fmt.Errorf("Failed to update metadata: Invalid inputs")
		return err
	}

	// Do not update (non-existent) metadata file for internal containers
	if container.IsInternal {
		return nil
	}

	// Verify metadata file exists before proceeding
	if !mdFileExist(task, container, manager.cfg.DataDir) {
		err = fmt.Errorf("Failed to update metadata for container %s of task %s: File does not exist", container, task)
		return err
	}

	// Get docker container information through api call
	dockerContainer, err := manager.client.InspectContainer(dockerID, inspectContainerTimeout)
	if err != nil {
		err = fmt.Errorf("Failed to inspect container %s of task %s: %s", container, task, err.Error())
		return err
	}

	// Ensure we do not update a stopped, dead, or invalid container
	if dockerContainer == nil || dockerContainer.State.Dead || !dockerContainer.State.FinishedAt.IsZero() {
		err = fmt.Errorf("Failed ot update metadata for container %s of task %s: Container stopped or invalid")
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := acquireMetadata(manager.client, dockerContainer, manager.cfg, task)
	err = metadata.writeToMetadataFile(task, container, manager.cfg.DataDir)
	if err != nil {
		err = fmt.Errorf("Failed to update metadata for container %s of task %s: %s", container, task, err.Error())
	} else {
		seelog.Debugf("Updated metadata file for task %s container %s", task, container)
	}
	return err
}

// CleanTaskMetadata removes the metadata files of all containers associated with a task
func (manager *metadataManager) CleanTaskMetadata(task *api.Task) error {
	// Check if manager has invalid entries
	var err error
	if task == nil || manager.cfg == nil {
		err = fmt.Errorf("Failed to clean metadata directory: Invalid inputs")
		return err
	}
	mdPath := getTaskMetadataDir(task, manager.cfg.DataDir)
	return removeContents(mdPath)
}
