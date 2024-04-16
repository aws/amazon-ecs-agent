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

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/docker/docker/api/types"
)

func (dm *daemonManager) CreateDaemonTask() (*apitask.Task, error) {
	return nil, errors.New("daemonmanager.CreateDaemonTask not implemented for Windows")
}

func (dm *daemonManager) LoadImage(ctx context.Context, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	return nil, errors.New("dameonmanager.LoadImage not implemented for Windows")
}

// isImageLoaded uses the image ref with its tag
func (dm *daemonManager) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return false, errors.New("daemonmanager.IsLoaded not implemented for Windows")
}

func (dm *daemonManager) ImageExists() (bool, error) {
	return false, errors.New("daemonmanager.ImageExists not implemented for Windows")
}
