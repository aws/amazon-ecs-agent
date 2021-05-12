// +build !linux,!windows

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

package engine

import (
	"context"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/containernetworking/cni/pkg/types/current"
)

const (
	// Constants for CNI timeout during setup and cleanup.
	cniSetupTimeout   = 1 * time.Minute
	cniCleanupTimeout = 30 * time.Second
)

// updateTaskENIDependencies updates the task's dependencies for awsvpc networking mode.
// This method is used only on Windows platform.
func (engine *DockerTaskEngine) updateTaskENIDependencies(task *apitask.Task) {
}

// invokePluginForContainer is used to invoke the CNI plugin for the given container
// On non-windows platform, we will not invoke CNI plugins for non-pause containers
func (engine *DockerTaskEngine) invokePluginsForContainer(task *apitask.Task, container *apicontainer.Container) error {
	return nil
}

// invokeCommandsForTaskNamespaceSetup invokes the necessary commands to setup various constructs of awsvpc-network mode for the task.
// This is applicable only for Windows for now.
func (engine *DockerTaskEngine) invokeCommandsForTaskNamespaceSetup(ctx context.Context, task *apitask.Task,
	config *ecscni.Config, result *current.Result) error {
	return nil
}

// invokeCommandsForTaskNamespaceCleanup invokes the necessary commands to cleanup the constructs of awsvpc-network mode for the task.
// This is applicable only for Windows for now.
func (engine *DockerTaskEngine) invokeCommandsForTaskNamespaceCleanup(task *apitask.Task, config *ecscni.Config) error {
	return nil
}
