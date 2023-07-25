//go:build linux
// +build linux

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
	"encoding/json"
	"fmt"
	"io/fs"
	"os"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	utils "github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/loader"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"
)

const (
	// all daemons will share the same user id
	// note: AppNetUID is 20000
	daemonUID                                   = 20001
	daemonMountPermission           fs.FileMode = 0700
	ecsAgentLogFileENV                          = "ECS_LOGFILE"
	defaultECSAgentLogPathContainer             = "/log"
)

var mkdirAllAndChown = utils.MkdirAllAndChown

func (dm *daemonManager) CreateDaemonTask() (*apitask.Task, error) {
	imageName := dm.managedDaemon.GetImageName()
	loadedImageRef := dm.managedDaemon.GetLoadedDaemonImageRef()
	containerRunning := apicontainerstatus.ContainerRunning
	dockerHostConfig := dockercontainer.HostConfig{
		NetworkMode: apitask.HostNetworkMode,
		// the default value of 0 for MaximumRetryCount means retry indefinitely
		RestartPolicy: dockercontainer.RestartPolicy{
			Name:              "on-failure",
			MaximumRetryCount: 0,
		},
	}
	if !dm.managedDaemon.IsValidManagedDaemon() {
		return nil, fmt.Errorf("%s is an invalid managed daemon", imageName)
	}
	for _, mount := range dm.managedDaemon.GetMountPoints() {
		err := mkdirAllAndChown(mount.SourceVolumeHostPath, daemonMountPermission, daemonUID, os.Getegid())
		if err != nil {
			return nil, err
		}
		dockerHostConfig.Binds = append(dockerHostConfig.Binds,
			fmt.Sprintf("%s:%s", mount.SourceVolumeHostPath, mount.ContainerPath))
	}
	rawHostConfig, err := json.Marshal(&dockerHostConfig)
	if err != nil {
		return nil, err
	}
	healthConfig := dm.managedDaemon.GetDockerHealthConfig()
	var rawConfig = ""
	rawHealthConfig, err := json.Marshal(&healthConfig)
	if err != nil {
		return nil, err
	}
	// The raw host config needs to be created this way - if we marshal the entire config object
	// directly, and the object only contains healthcheck, all other fields will be written as empty/nil
	// in the result string. This will override the configurations that comes with the container image
	// (CMD for example)
	rawConfig = fmt.Sprintf("{\"Healthcheck\":%s}", string(rawHealthConfig))
	daemonTask := &apitask.Task{
		Arn:                 fmt.Sprintf("arn:::::/%s-%s", dm.managedDaemon.GetImageName(), uuid.NewUUID()),
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{{
			// name must be unique among running containers
			// We should only have a single managed daemon of each type per instance
			Name:                      fmt.Sprintf("ecs-managed-%s", dm.managedDaemon.GetImageName()),
			Image:                     loadedImageRef,
			ContainerArn:              fmt.Sprintf("arn:::::/instance-%s", imageName),
			Type:                      apicontainer.ContainerManagedDaemon,
			Command:                   dm.managedDaemon.GetCommand(),
			TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			Essential:                 true,
			SteadyStateStatusUnsafe:   &containerRunning,
			DockerConfig: apicontainer.DockerConfig{
				Config:     aws.String(rawConfig),
				HostConfig: aws.String(string(rawHostConfig)),
			},
			HealthCheckType: "DOCKER",
		}},
		LaunchType:         "EC2",
		NetworkMode:        apitask.HostNetworkMode,
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		IsInternal:         true,
	}
	// add managed daemon environment to daemon task container
	daemonTask.Containers[0].MergeEnvironmentVariables(dm.managedDaemon.GetEnvironment())
	return daemonTask, nil
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

// isImageLoaded uses the image ref with its tag
func (dm *daemonManager) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return loader.IsImageLoaded(dm.managedDaemon.GetImageRef(), dockerClient)
}
