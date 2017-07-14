// +build !windows
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
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/cihub/seelog"
)

const (
	inspectContainerTimeout = 30 * time.Second
	mountPoint              = "/ecs/metadata"
)

// MetadataManager is an interface that allows us to abstract away the metadata
// operations
type MetadataManager interface {
	CreateMetadata([]string, *api.Task, *api.Container) ([]string, error)
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
	seelog.Debug("Creating metadata manager")
	return &metadataManager{
		client: client,
		cfg:    cfg,
	}
}

// CreateMetadata creates the metadata file and adds the metadata directory to
// the container's mounted host volumes
// binds []string is passed by value to avoid race conditions by multiple
// calls to CreateMetadata, although this should never actually happen
func (manager *metadataManager) CreateMetadata(binds []string, task *api.Task, container *api.Container) ([]string, error) {
	// Check if manager has invalid entries
	var err error
	if manager.cfg == nil {
		err = fmt.Errorf("Invalid configuration")
		return binds, err
	}

	// Do not create metadata file for internal containers
	// Add error handling for this case? Probably no need since
	// Internal containers should not be visible to users anyways
	if container.IsInternal {
		return binds, nil
	}

	// Create task and container directories if they do not yet exist
	mdDirectoryPath, err := getMetadataFilePath(task, container, manager.cfg.DataDir)
	// Stop metadata creation if path is malformed for any reason
	if err != nil {
		err = fmt.Errorf("Invalid metadata file path due to malformed input")
		return binds, err
	}

	err = os.MkdirAll(mdDirectoryPath, os.ModePerm)
	if err != nil {
		return binds, err
	}

	// Create metadata file
	mdFilePath := filepath.Join(mdDirectoryPath, metadataFile)
	err = ioutil.WriteFile(mdFilePath, nil, 0644)
	if err != nil {
		return binds, err
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := acquireMetadataAtContainerCreate(manager.client, manager.cfg, task)
	err = metadata.writeToMetadataFile(task, container, manager.cfg.DataDir)
	if err != nil {
		return binds, err
	}

	// Add the directory of this container's metadata to the container's mount binds
	// We do this at the end so that we only mount the directory if there are no errors
	// This is the only operating system specific point here, so it would be nice if there
	// were some elegant way to do this for both windows and linux at the same time
	instanceBind := fmt.Sprintf("%s/%s:%s/%s", manager.cfg.DataDirOnHost, mdDirectoryPath, mountPoint, container.Name)
	binds = append(binds, instanceBind)
	return binds, nil
}

// UpdateMetadata updates the metadata file after container starts and dynamic
// metadata is available
func (manager *metadataManager) UpdateMetadata(dockerID string, task *api.Task, container *api.Container) error {
	// Check if manager has invalid entries
	var err error
	if manager.cfg == nil {
		err = fmt.Errorf("Invalid configuration")
		return err
	}

	// Do not update (non-existent) metadata file for internal containers
	if container.IsInternal {
		return nil
	}

	// Verify metadata file exists before proceeding
	if !mdFileExist(task, container, manager.cfg.DataDir) {
		err = fmt.Errorf("File does not exist")
		return err
	}

	// Get docker container information through api call
	dockerContainer, err := manager.client.InspectContainer(dockerID, inspectContainerTimeout)
	if err != nil {
		return err
	}

	// Ensure we do not update a container that is invalid or is not running
	if dockerContainer == nil || !dockerContainer.State.Running {
		err = fmt.Errorf("Container not running or invalid")
		return err
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := acquireMetadata(manager.client, dockerContainer, manager.cfg, task)
	return metadata.writeToMetadataFile(task, container, manager.cfg.DataDir)
}

// CleanTaskMetadata removes the metadata files of all containers associated with a task
func (manager *metadataManager) CleanTaskMetadata(task *api.Task) error {
	// Check if manager has invalid entries
	var err error
	if task == nil || manager.cfg == nil {
		err = fmt.Errorf("Invalid task or config")
		return err
	}
	mdPath := getTaskMetadataDir(task, manager.cfg.DataDir)
	return removeContents(mdPath)
}
