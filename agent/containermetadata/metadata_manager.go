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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"

	docker "github.com/fsouza/go-dockerclient"
)

const (
	inspectContainerTimeout = 30 * time.Second
)

// MetadataManager is an interface that allows us to abstract away the metadata
// operations
type MetadataManager interface {
	SetContainerInstanceARN(string)
	CreateMetadata(*docker.HostConfig, *api.Task, *api.Container) error
	UpdateMetadata(string, *api.Task, *api.Container) error
	CleanTaskMetadata(*api.Task) error
}

// metadataManager implements the MetadataManager interface
type metadataManager struct {
	client               dockerMetadataClient
	cluster              string
	enabled              bool
	dataDir              string
	dataDirOnHost        string
	containerInstanceARN string
}

// NewMetadataManager creates a metadataManager for a given DockerTaskEngine settings.
func NewMetadataManager(client dockerMetadataClient, cfg *config.Config) MetadataManager {
	return &metadataManager{
		client:        client,
		cluster:       cfg.Cluster,
		enabled:       cfg.ContainerMetadataEnabled,
		dataDir:       cfg.DataDir,
		dataDirOnHost: cfg.DataDirOnHost,
	}
}

// SetContainerInstanceARN sets the metadataManager's ContainerInstanceArn which is not available
// at its creation as this information is not present immediately at the agent's startup
func (manager *metadataManager) SetContainerInstanceARN(containerInstanceARN string) {
	manager.containerInstanceARN = containerInstanceARN
}

// createmetadata creates the metadata file and adds the metadata directory to
// the container's mounted host volumes
// Pointer hostConfig is modified directly so there is risk of concurrency errors.
func (manager *metadataManager) CreateMetadata(hostConfig *docker.HostConfig, task *api.Task, container *api.Container) error {
	// Do not create metadata file for internal containers
	if container.IsInternal {
		return nil
	}

	// Create task and container directories if they do not yet exist
	metadataDirectoryPath, err := getMetadataFilePath(task, container, manager.dataDir)
	// Stop metadata creation if path is malformed for any reason
	if err != nil {
		return fmt.Errorf("container metadata create: %v", err)
	}

	err = os.MkdirAll(metadataDirectoryPath, os.ModePerm)
	if err != nil {
		return err
	}

	// Create metadata file
	err = createMetadataFile(metadataDirectoryPath)
	if err != nil {
		return err
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := manager.parseMetadataAtContainerCreate(task, container)
	data, err := json.MarshalIndent(metadata, "", "\t")
	err = writeToMetadataFile(data, task, container, manager.dataDir)
	if err != nil {
		return err
	}

	// Add the directory of this container's metadata to the container's mount binds
	// We do this at the end so that we only mount the directory if there are no errors
	binds := createBinds(hostConfig.Binds, manager.dataDirOnHost, metadataDirectoryPath, mountPoint, container.Name)
	hostConfig.Binds = binds
	return nil
}

// UpdateMetadata updates the metadata file after container starts and dynamic
// metadata is available
func (manager *metadataManager) UpdateMetadata(dockerID string, task *api.Task, container *api.Container) error {
	// Do not update (non-existent) metadata file for internal containers
	if container.IsInternal {
		return nil
	}

	// Verify metadata file exists before proceeding
	if !metadataFileExists(task, container, manager.dataDir) {
		expectedPath, _ := getMetadataFilePath(task, container, manager.dataDir)
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

	// Acquire the metadata then write it in JSON format to the file
	metadata := manager.parseMetadata(dockerContainer, task, container)
	data, err := json.MarshalIndent(metadata, "", "\t")
	return writeToMetadataFile(data, task, container, manager.dataDir)
}

// CleanTaskMetadata removes the metadata files of all containers associated with a task
func (manager *metadataManager) CleanTaskMetadata(task *api.Task) error {
	metadataPath, err := getTaskMetadataDir(task, manager.dataDir)
	if err != nil {
		return err
	}
	return os.RemoveAll(metadataPath)
}
