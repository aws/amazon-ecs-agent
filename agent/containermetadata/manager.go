// Copyright 2017-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"context"
	"encoding/json"
	"fmt"
	"os"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	// metadataEnvironmentVariable is the environment variable passed to the
	// container for the metadata file path.
	metadataEnvironmentVariable = "ECS_CONTAINER_METADATA_FILE"
	metadataFile                = "ecs-container-metadata.json"
	metadataPerm                = 0644
)

// Manager is an interface that allows us to abstract away the metadata
// operations
type Manager interface {
	SetContainerInstanceARN(string)
	SetAvailabilityZone(string)
	SetHostPrivateIPv4Address(string)
	SetHostPublicIPv4Address(string)
	Create(*dockercontainer.Config, *dockercontainer.HostConfig, *apitask.Task, string, []string) error
	Update(context.Context, string, *apitask.Task, string) error
	Clean(string) error
}

// metadataManager implements the Manager interface
type metadataManager struct {
	// client is the Docker API Client that the metadata manager uses. It defaults
	// to 1.21 on Linux and 1.24 on Windows
	client DockerMetadataClient
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
	// osWrap is a wrapper for 'os' package operations
	osWrap oswrapper.OS
	// ioutilWrap is a wrapper for 'ioutil' package operations
	ioutilWrap ioutilwrapper.IOUtil
	// availabilityZone is the availabiltyZone where task is in
	availabilityZone string
	// hostPrivateIPv4Address is the private IPv4 address associated with the EC2 instance
	hostPrivateIPv4Address string
	// hostPublicIPv4Address is the public IPv4 address associated with the EC2 instance
	hostPublicIPv4Address string
}

// NewManager creates a metadataManager for a given DockerTaskEngine settings.
func NewManager(client DockerMetadataClient, cfg *config.Config) Manager {
	return &metadataManager{
		client:        client,
		cluster:       cfg.Cluster,
		dataDir:       cfg.DataDir,
		dataDirOnHost: cfg.DataDirOnHost,
		osWrap:        oswrapper.NewOS(),
		ioutilWrap:    ioutilwrapper.NewIOUtil(),
	}
}

// SetContainerInstanceARN sets the metadataManager's ContainerInstanceArn which is not available
// at its creation as this information is not present immediately at the agent's startup
func (manager *metadataManager) SetContainerInstanceARN(containerInstanceARN string) {
	manager.containerInstanceARN = containerInstanceARN
}

// SetAvailabilityzone sets the metadataManager's AvailabilityZone which is not available
// at its creation as this information is not present immediately at the agent's startup
func (manager *metadataManager) SetAvailabilityZone(availabilityZone string) {
	manager.availabilityZone = availabilityZone
}

// SetHostPrivateIPv4Address sets the metadataManager's hostPrivateIPv4Address which is not available
// at its creation as this information is not present immediately at the agent's startup
func (manager *metadataManager) SetHostPrivateIPv4Address(ipv4address string) {
	manager.hostPrivateIPv4Address = ipv4address
}

// SetHostPublicIPv4Address sets the metadataManager's hostPublicIPv4Address which is not available
// at its creation as this information is not present immediately at the agent's startup
func (manager *metadataManager) SetHostPublicIPv4Address(ipv4address string) {
	manager.hostPublicIPv4Address = ipv4address
}

// Create creates the metadata file and adds the metadata directory to
// the container's mounted host volumes
// Pointer hostConfig is modified directly so there is risk of concurrency errors.
func (manager *metadataManager) Create(config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig,
	task *apitask.Task, containerName string, dockerSecurityOptions []string) error {
	// Create task and container directories if they do not yet exist
	metadataDirectoryPath, err := getMetadataFilePath(task.Arn, containerName, manager.dataDir)
	// Stop metadata creation if path is malformed for any reason
	if err != nil {
		return fmt.Errorf("container metadata create for task %s container %s: %v", task.Arn, containerName, err)
	}

	err = manager.osWrap.MkdirAll(metadataDirectoryPath, os.ModePerm)
	if err != nil {
		return fmt.Errorf("creating metadata directory for task %s: %v", task.Arn, err)
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := manager.parseMetadataAtContainerCreate(task, containerName)
	err = manager.marshalAndWrite(metadata, task.Arn, containerName)
	if err != nil {
		return err
	}

	// Add the directory of this container's metadata to the container's mount binds
	// Then add the destination directory as an environment variable in the container $METADATA
	binds, env := createBindsEnv(hostConfig.Binds, config.Env, manager.dataDirOnHost, metadataDirectoryPath, dockerSecurityOptions)
	config.Env = env
	hostConfig.Binds = binds
	return nil
}

// Update updates the metadata file after container starts and dynamic metadata is available
func (manager *metadataManager) Update(ctx context.Context, dockerID string, task *apitask.Task, containerName string) error {
	// Get docker container information through api call
	dockerContainer, err := manager.client.InspectContainer(ctx, dockerID, dockerclient.InspectContainerTimeout)
	if err != nil {
		return err
	}

	// Ensure we do not update a container that is invalid or is not running
	if dockerContainer == nil || !dockerContainer.State.Running {
		return fmt.Errorf("container metadata update for container %s in task %s: container not running or invalid", containerName, task.Arn)
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := manager.parseMetadata(dockerContainer, task, containerName)
	return manager.marshalAndWrite(metadata, task.Arn, containerName)
}

// Clean removes the metadata files of all containers associated with a task
func (manager *metadataManager) Clean(taskARN string) error {
	metadataPath, err := getTaskMetadataDir(taskARN, manager.dataDir)
	if err != nil {
		return fmt.Errorf("clean task metadata: unable to get metadata directory for task %s: %v", taskARN, err)
	}
	return manager.osWrap.RemoveAll(metadataPath)
}

func (manager *metadataManager) marshalAndWrite(metadata Metadata, taskARN string, containerName string) error {
	data, err := json.MarshalIndent(metadata, "", "\t")
	if err != nil {
		return fmt.Errorf("create metadata for container %s in task %s: failed to marshal metadata: %v", containerName, taskARN, err)
	}

	// Write the metadata to file
	return writeToMetadataFile(manager.osWrap, manager.ioutilWrap, data, taskARN, containerName, manager.dataDir)
}
