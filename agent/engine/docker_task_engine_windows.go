//go:build windows
// +build windows

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
	"strings"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	dockercontainer "github.com/docker/docker/api/types/container"

	"github.com/pkg/errors"
)

const (
	// Constants for CNI timeout during setup and cleanup on Windows.
	cniSetupTimeout   = 3 * time.Minute
	cniCleanupTimeout = 2 * time.Minute
)

func (engine *DockerTaskEngine) updateTaskENIDependencies(task *apitask.Task) {
	if !task.IsNetworkModeAWSVPC() {
		return
	}
	task.UpdateTaskENIsLinkName()
}

// invokePluginForContainer is used to invoke the CNI plugin for the given container
func (engine *DockerTaskEngine) invokePluginsForContainer(task *apitask.Task, container *apicontainer.Container) error {
	containerInspectOutput, err := engine.inspectContainer(task, container)
	if err != nil {
		return errors.Wrapf(err, "error occurred while inspecting container %v", container.Name)
	}

	cniConfig, err := engine.buildCNIConfigFromTaskContainerAwsvpc(task, containerInspectOutput, false)
	if err != nil {
		return errors.Wrap(err, "unable to build cni configuration")
	}

	// Invoke the cni plugin for the container using libcni
	_, err = engine.cniClient.SetupNS(engine.ctx, cniConfig, cniSetupTimeout)
	if err != nil {
		logger.Error("Unable to configure container in the pause namespace", logger.Fields{
			field.TaskID:    task.GetID(),
			field.Container: container.Name,
			field.Error:     err,
		})
		return errors.Wrap(err, "failed to connect HNS endpoint to container")
	}

	return nil
}

func (engine *DockerTaskEngine) watchAppNetImage(ctx context.Context) {
}

func (engine *DockerTaskEngine) reloadAppNetImage() error {
	return nil
}

func (engine *DockerTaskEngine) restartInstanceTask() {
}

// updateCredentialSpecMapping is used to map the credentialspec local file location to docker security opts
func (engine *DockerTaskEngine) updateCredentialSpecMapping(taskID string, containerName string, desiredCredSpecInjection string, hostConfig *dockercontainer.HostConfig) {
	// Inject containers' hostConfig.SecurityOpt with the credentialspec resource
	logger.Info("Injecting container with credentialspec resource", logger.Fields{
		field.TaskID:     taskID,
		field.Container:  containerName,
		"credentialSpec": desiredCredSpecInjection,
	})

	if len(hostConfig.SecurityOpt) == 0 {
		hostConfig.SecurityOpt = []string{desiredCredSpecInjection}
	} else {
		for idx, opt := range hostConfig.SecurityOpt {
			if strings.HasPrefix(opt, "credentialspec:") {
				hostConfig.SecurityOpt[idx] = desiredCredSpecInjection
			}
		}
	}
}
