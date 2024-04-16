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

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	md "github.com/aws/amazon-ecs-agent/ecs-agent/manageddaemon"
	"github.com/docker/docker/api/types"
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
