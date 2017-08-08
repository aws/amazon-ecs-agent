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

	"github.com/aws/amazon-ecs-agent/agent/config"

	docker "github.com/fsouza/go-dockerclient"
)

const (
	metadataEnvironmentVariable = "ECS_CONTAINER_METADATA_FILE"
	inspectContainerTimeout     = 30 * time.Second
	metadataFile                = "ecs-container-metadata.json"
	metadataPerm                = 0644
)

// MetadataStatus specifies the current update status of the metadata file.
// The purpose of this status is for users to check if the metadata file has
// reached the stage they need before they read the rest of the file to avoid
// race conditions (Since the final stage will need to be after the container
// starts up
// In the future the metadata may require multiple stages of update and these
// statuses should amended/appended accordingly.
type MetadataStatus string

const (
	// MetadataInitial is the initial state of the metadata file which
	// contains metadata provided by the ECS Agent
	MetadataInitial = "INITIAL"
	// MetadataReady is the final state of the metadata file which indicates
	// it has acquired all the data it needs (Currently from the Agent and Docker)
	MetadataReady = "READY"
)

// MetadataManager is an interface that allows us to abstract away the metadata
// operations
type MetadataManager interface {
	SetContainerInstanceARN(string)
	Create(*docker.Config, *docker.HostConfig, string, string) error
	Update(string, string, string) error
	Clean(string) error
}

// metadataManager implements the MetadataManager interface
type metadataManager struct {
	// client is the Docker API Client that the metadata manager uses. It defaults
	// to 1.21 on Linux and 1.24 on Windows
	client dockerMetadataClient
	// cluster is the cluster where this agent is run
	cluster string
	// dataDir is the directory where the metadata is being written. For Linux
	// this is a container directory
	dataDir string
	// dataDirOnHost is the directory from which dataDir is mounted for Linux
	// version of the agent
	dataDirOnHost string
	// containerInstanceARN is the Container Instance ARN registered for this agent
	containerInstanceARN string
}

// NewMetadataManager creates a metadataManager for a given DockerTaskEngine settings.
func NewMetadataManager(client dockerMetadataClient, cfg *config.Config) MetadataManager {
	return &metadataManager{
		client:        client,
		cluster:       cfg.Cluster,
		dataDir:       cfg.DataDir,
		dataDirOnHost: cfg.DataDirOnHost,
	}
}

// SetContainerInstanceARN sets the metadataManager's ContainerInstanceArn which is not available
// at its creation as this information is not present immediately at the agent's startup
func (manager *metadataManager) SetContainerInstanceARN(containerInstanceARN string) {
	manager.containerInstanceARN = containerInstanceARN
}

// Createmetadata creates the metadata file and adds the metadata directory to
// the container's mounted host volumes
// Pointer hostConfig is modified directly so there is risk of concurrency errors.
func (manager *metadataManager) Create(config *docker.Config, hostConfig *docker.HostConfig, taskARN string, containerName string) error {
	// Create task and container directories if they do not yet exist
	metadataDirectoryPath, err := getMetadataFilePath(taskARN, containerName, manager.dataDir)
	// Stop metadata creation if path is malformed for any reason
	if err != nil {
		return fmt.Errorf("container metadata create for task %s container %s: %v", taskARN, containerName, err)
	}

	err = os.MkdirAll(metadataDirectoryPath, os.ModePerm)
	if err != nil {
		return fmt.Errorf("creating metadata directory for task %s: %v", taskARN, err)
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := manager.parseMetadataAtContainerCreate(taskARN, containerName)
	data, err := json.MarshalIndent(metadata, "", "\t")
	if err != nil {
		return fmt.Errorf("create metadata for task %s container %s: %v", taskARN, containerName, err)
	}

	// Write the metadata to file
	err = writeToMetadataFile(data, taskARN, containerName, manager.dataDir)
	if err != nil {
		return err
	}

	// Add the directory of this container's metadata to the container's mount binds
	// Then add the destination directory as an environment variable in the container $METADATA
	binds, env := createBindsEnv(hostConfig.Binds, config.Env, manager.dataDirOnHost, metadataDirectoryPath)
	config.Env = env
	hostConfig.Binds = binds
	return nil
}

// UpdateMetadata updates the metadata file after container starts and dynamic
// metadata is available
func (manager *metadataManager) Update(dockerID string, taskARN string, containerName string) error {
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
	metadata := manager.parseMetadata(dockerContainer, taskARN, containerName)
	data, err := json.MarshalIndent(metadata, "", "\t")
	if err != nil {
		return fmt.Errorf("update metadata for task %s container %s: %v", taskARN, containerName, err)
	}

	return writeToMetadataFile(data, taskARN, containerName, manager.dataDir)
}

// CleanTaskMetadata removes the metadata files of all containers associated with a task
func (manager *metadataManager) Clean(taskARN string) error {
	metadataPath, err := getTaskMetadataDir(taskARN, manager.dataDir)
	if err != nil {
		return err
	}
	return os.RemoveAll(metadataPath)
}
