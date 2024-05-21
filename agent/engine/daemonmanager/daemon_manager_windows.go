//go:build windows
// +build windows

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package daemonmanager

import (
	"context"
	"errors"
	"fmt"
	"os"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/loader"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/docker/docker/api/types"
)

func (dm *daemonManager) CreateDaemonTask() (*apitask.Task, error) {
	return nil, errors.New("daemonmanager.CreateDaemonTask not implemented for Windows")
}

// LoadImage loads the daemon's latest image
func (dm *daemonManager) LoadImage(ctx context.Context, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	var loadErr error
	daemonImageToLoad := dm.managedDaemon.GetImageName()
	daemonImageTarPath := dm.managedDaemon.GetImageTarPath()
	if _, err := os.Stat(daemonImageTarPath); err != nil {
		logger.Warn(fmt.Sprintf("%s container tarball unavailable at path: %s", daemonImageToLoad, daemonImageTarPath), logger.Fields{
			field.Error: err,
		})
	}
	logger.Debug(fmt.Sprintf("Loading %s container image from tarball: %s", daemonImageToLoad, daemonImageTarPath))
	if loadErr = loader.LoadFromFile(ctx, daemonImageTarPath, dockerClient); loadErr != nil {
		logger.Warn(fmt.Sprintf("Unable to load %s container image from tarball: %s", daemonImageToLoad, daemonImageTarPath), logger.Fields{
			field.Error: loadErr,
		})
	}
	dm.managedDaemon.SetLoadedDaemonImageRef(dm.managedDaemon.GetImageRef())
	loadedImageRef := dm.managedDaemon.GetLoadedDaemonImageRef()
	logger.Info(fmt.Sprintf("Successfully loaded %s container image from tarball: %s", daemonImageToLoad, daemonImageTarPath),
		logger.Fields{
			field.Image: loadedImageRef,
		})
	return loader.GetContainerImage(loadedImageRef, dockerClient)
}

func (dm *daemonManager) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return loader.IsImageLoaded(dm.managedDaemon.GetImageRef(), dockerClient)
}

// Returns true if the Daemon image is found on this host, false otherwise.
func (dm *daemonManager) ImageExists() (bool, error) {
	return utils.FileExists(dm.managedDaemon.GetImageTarPath())
}
