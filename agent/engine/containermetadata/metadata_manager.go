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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"

	"github.com/cihub/seelog"
)

const (
	inspectContainerTimeout = 30 * time.Second
)

// MetadataManager is an interface that allows us to abstract away the metadata
// operations
type MetadataManager interface {
	SetContainerInstanceArn(string)
	CreateMetadata([]string, *api.Task, *api.Container) ([]string, error)
	UpdateMetadata(string, *api.Task, *api.Container) error
	CleanTaskMetadata(*api.Task) error
}

// metadataManager implements the MetadataManager interface
type metadataManager struct {
	client               dockerMetadataClient
	cfg                  *config.Config
	containerInstanceArn string
}

// NewMetadataManager creates a metadataManager for a given DockerTaskEngine settings.
func NewMetadataManager(client dockerMetadataClient, cfg *config.Config) MetadataManager {
	return &metadataManager{
		client: client,
		cfg:    cfg,
	}
}

// SetContainerInstanceArn sets the metadataManager's ContainerInstanceArn which is not available
// at its creation as this information is not present immediately at the agent's startup
func (manager *metadataManager) SetContainerInstanceArn(containerInstanceArn string) {
	// Do nothing if disabled
	if !manager.cfg.ContainerMetadataEnabled {
		return
	}
	manager.containerInstanceArn = containerInstanceArn
}

// UpdateMetadata updates the metadata file after container starts and dynamic
// metadata is available
func (manager *metadataManager) UpdateMetadata(dockerID string, task *api.Task, container *api.Container) error {
	// Do nothing if disabled
	if !manager.cfg.ContainerMetadataEnabled {
		return nil
	}

	// Do not update (non-existent) metadata file for internal containers
	if container.IsInternal {
		return nil
	}

	// Verify metadata file exists before proceeding
	if !metadataFileExists(task, container, manager.cfg.DataDir) {
		expectedPath, _ := getMetadataFilePath(task, container, manager.cfg.DataDir)
		return fmt.Errorf("container metadata update: %s does not exist", expectedPath)
	}

	// Get docker container information through api call
	dockerContainer, err := manager.client.InspectContainer(dockerID, inspectContainerTimeout)
	if err != nil {
		return err
	}

	// Ensure we do not update a container that is invalid or is not running
	if dockerContainer == nil || !dockerContainer.State.Running {
		return fmt.Errorf("container metadata update: container not running or invalid")
	}

	// Get last metadata file creation time
	createTime, err := getMetadataFileUpdateTime(task, container, manager.cfg.DataDir)
	createTimeFmt := ""
	if err != nil {
		seelog.Errorf("container metadata update: %v", err)
	} else {
		createTimeFmt = createTime.Format(time.StampNano)
	}
	updateTimeFmt := ""

	// Acquire the metadata then write it in JSON format to the file
	metadata := manager.acquireMetadata(createTimeFmt, updateTimeFmt, dockerContainer, task)
	err = metadata.writeToMetadataFile(task, container, manager.cfg.DataDir)
	if err != nil {
		return err
	}

	// Update the file again with time stamp of last file update. This will cause the file to have
	// a different time stamp from its actual file stat inspection but this is unavoidable in our
	// design as we must populate the metadata with the update time before updating
	updateTime, err := getMetadataFileUpdateTime(task, container, manager.cfg.DataDir)
	if err != nil {
		seelog.Errorf("container metadata update: %v", err)
	} else {
		updateTimeFmt = updateTime.Format(time.StampNano)
	}
	// We fetch the metadata and write it once more to put the update time into the file. This update
	// time is the time of the last update where we update with new metadata
	metadata = manager.acquireMetadata(createTimeFmt, updateTimeFmt, dockerContainer, task)
	return metadata.writeToMetadataFile(task, container, manager.cfg.DataDir)
}

// CleanTaskMetadata removes the metadata files of all containers associated with a task
func (manager *metadataManager) CleanTaskMetadata(task *api.Task) error {
	// Do nothing if disabled
	if !manager.cfg.ContainerMetadataEnabled {
		return nil
	}

	mdPath := getTaskMetadataDir(task, manager.cfg.DataDir)
	return os.RemoveAll(mdPath)
}
