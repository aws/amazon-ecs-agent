//go:build !linux
// +build !linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package serviceconnect

import (
	"context"
	"fmt"
	"runtime"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/utils/loader"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
)

type manager struct {
}

func NewManager() Manager {
	return &manager{}
}

func (m *manager) AugmentTaskContainer(*apitask.Task, *apicontainer.Container, *dockercontainer.HostConfig) error {
	return fmt.Errorf("ServiceConnect is only supported on linux")
}
func (m *manager) CreateInstanceTask(config *config.Config) (*apitask.Task, error) {
	return nil, fmt.Errorf("ServiceConnect is only supported on linux")
}
func (m *manager) AugmentInstanceContainer(*apitask.Task, *apicontainer.Container, *dockercontainer.HostConfig) error {
	return fmt.Errorf("ServiceConnect is only supported on linux")
}

func (*manager) LoadImage(ctx context.Context, _ *config.Config, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	return nil, loader.NewUnsupportedPlatformError(fmt.Errorf(
		"appnetAgent container load: unsupported platform: %s/%s",
		runtime.GOOS, runtime.GOARCH))
}

func (*manager) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return false, loader.NewUnsupportedPlatformError(fmt.Errorf(
		"appnetAgent container isloaded: unsupported platform: %s/%s",
		runtime.GOOS, runtime.GOARCH))
}

func (m *manager) SetECSClient(api.ECSClient, string) {
}

func (*manager) GetLoadedImageName() (string, error) {
	return "", loader.NewUnsupportedPlatformError(fmt.Errorf(
		"appnetAgent container get image name: unsupported platform: %s/%s",
		runtime.GOOS, runtime.GOARCH))
}
