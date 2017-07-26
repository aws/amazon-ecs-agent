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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
)

const (
	mountPoint                     = "/ecs/metadata"
	ContainerMetadataClientVersion = dockerclient.Version_1_21
)

// CreateMetadata creates the metadata file and adds the metadata directory to
// the container's mounted host volumes
// binds []string is passed by value to avoid race conditions by multiple
// calls to CreateMetadata, although this should never actually happen
func (manager *metadataManager) CreateMetadata(binds []string, task *api.Task, container *api.Container) ([]string, error) {
	// Do nothing if disabled
	if !manager.cfg.ContainerMetadataEnabled {
		return nil, nil
	}

	// Do not create metadata file for internal containers
	if container.IsInternal {
		return binds, nil
	}

	// Create task and container directories if they do not yet exist
	mdDirectoryPath, err := getMetadataFilePath(task, container, manager.cfg.DataDir)
	// Stop metadata creation if path is malformed for any reason
	if err != nil {
		return binds, fmt.Errorf("container metadata create: %v", err)
	}

	err = os.MkdirAll(mdDirectoryPath, os.ModePerm)
	if err != nil {
		return binds, err
	}

	// Create metadata file
	mdFilePath := filepath.Join(mdDirectoryPath, metadataFile)
	err = ioutil.WriteFile(mdFilePath, nil, readOnlyPerm)
	if err != nil {
		return binds, err
	}

	// Acquire the metadata then write it in JSON format to the file
	metadata := manager.acquireMetadataAtContainerCreate(task)
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
