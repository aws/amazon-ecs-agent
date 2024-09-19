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
	"os"
	"path/filepath"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/loader"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	md "github.com/aws/amazon-ecs-agent/ecs-agent/manageddaemon"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockermount "github.com/docker/docker/api/types/mount"
	"github.com/pborman/uuid"
)

type DaemonManager interface {
	GetManagedDaemon() *md.ManagedDaemon
	CreateDaemonTask() (*apitask.Task, error)
	// Returns true if the Daemon image is found on this host, false otherwise.
	// Error is returned when something goes wrong when looking for the image.
	ImageExists() (bool, error)
	LoadImage(ctx context.Context, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error)
	IsLoaded(dockerClient dockerapi.DockerClient) (bool, error)
}

// each daemon manager manages one single daemon container
type daemonManager struct {
	managedDaemon *md.ManagedDaemon
}

func NewDaemonManager(manageddaemon *md.ManagedDaemon) DaemonManager {
	return &daemonManager{managedDaemon: manageddaemon}
}

func (dm *daemonManager) GetManagedDaemon() *md.ManagedDaemon {
	return dm.managedDaemon
}

var mkdirAllAndChown = utils.MkdirAllAndChown

func (dm *daemonManager) CreateDaemonTask() (*apitask.Task, error) {
	imageName := dm.managedDaemon.GetImageName()
	// create daemon-specific host mounts
	if err := dm.initDaemonDirectoryMounts(imageName); err != nil {
		logger.Error("initDaemonDirectory failure",
			logger.Fields{
				field.Error: err,
			})
		return nil, err
	}

	loadedImageRef := dm.managedDaemon.GetLoadedDaemonImageRef()
	containerRunning := apicontainerstatus.ContainerRunning
	stringCaps := []string{}
	if dm.managedDaemon.GetLinuxParameters() != nil {
		caps := dm.managedDaemon.GetLinuxParameters().Capabilities.Add
		for _, cap := range caps {
			stringCaps = append(stringCaps, *cap)
		}
	}
	dockerHostConfig := dockercontainer.HostConfig{
		Mounts:      []dockermount.Mount{},
		NetworkMode: apitask.HostNetworkMode,
		// the default value of 0 for MaximumRetryCount means retry indefinitely
		RestartPolicy: dockercontainer.RestartPolicy{
			Name:              "on-failure",
			MaximumRetryCount: 0,
		},
		Privileged: dm.managedDaemon.GetPrivileged(),
		CapAdd:     stringCaps,
	}
	if !dm.managedDaemon.IsValidManagedDaemon() {
		return nil, fmt.Errorf("%s is an invalid managed daemon", imageName)
	}

	for _, mp := range dm.managedDaemon.GetMountPoints() {
		if err := mkdirAllAndChown(mp.SourceVolumeHostPath, daemonMountPermission, daemonUID, os.Getegid()); err != nil {
			return nil, err
		}
		var bindOptions = dockermount.BindOptions{}

		if mp.PropagationShared {
			// https://github.com/moby/moby/blob/master/api/types/mount/mount.go#L52
			bindOptions = dockermount.BindOptions{Propagation: dockermount.PropagationShared}
		}
		logger.Info(fmt.Sprintf("bindMount Propagation: %s", bindOptions.Propagation),
			logger.Fields{
				field.Image: loadedImageRef,
			})
		typeBind := dockermount.TypeBind
		mountPoint := dockermount.Mount{}
		if mp.SourceVolumeType == "npipe" {
			typeBind = dockermount.TypeNamedPipe
			mountPoint = dockermount.Mount{
				Type:   typeBind,
				Source: mp.SourceVolumeHostPath,
				Target: mp.ContainerPath,
			}
		} else {
			mountPoint = dockermount.Mount{
				Type:        typeBind,
				Source:      mp.SourceVolumeHostPath,
				Target:      mp.ContainerPath,
				BindOptions: &bindOptions,
			}
		}
		dockerHostConfig.Mounts = append(dockerHostConfig.Mounts, mountPoint)
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
	// TODO update User in raw config to use either runAs user or runAsRoot from managed daemon config
	rawConfig = fmt.Sprintf(rawContainerConfigurationTemplate, string(rawHealthConfig))
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

func (dm *daemonManager) initDaemonDirectoryMounts(imageName string) error {
	// create logging directory
	logPathHost := filepath.Join(config.ManagedDaemonLogPathHostRoot, imageName)
	if err := mkdirAllAndChown(logPathHost, daemonLogPermission, daemonUID, os.Getegid()); err != nil {
		return err
	}
	// create socket path
	socketPathHost := filepath.Join(config.ManagedDaemonSocketPathHostRoot, imageName)
	if err := mkdirAllAndChown(socketPathHost, daemonMountPermission, daemonUID, os.Getegid()); err != nil {
		return err
	}
	return nil
}

// Returns true if the Daemon image is found on this host, false otherwise.
func (dm *daemonManager) ImageExists() (bool, error) {
	return utils.FileExists(dm.managedDaemon.GetImageTarPath())
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
