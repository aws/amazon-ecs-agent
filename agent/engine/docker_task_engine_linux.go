// +build linux

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
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	// Constants for CNI timeout during setup and cleanup.
	cniSetupTimeout   = 1 * time.Minute
	cniCleanupTimeout = 30 * time.Second

	// logDriverTypeFirelens is the log driver type for containers that want to use the firelens container to send logs.
	logDriverTypeFirelens       = "awsfirelens"
	logDriverTypeFluentd        = "fluentd"
	logDriverTag                = "tag"
	logDriverFluentdAddress     = "fluentd-address"
	dataLogDriverPath           = "/data/firelens/"
	logDriverAsyncConnect       = "fluentd-async-connect"
	logDriverSubSecondPrecision = "fluentd-sub-second-precision"
	logDriverBufferLimit        = "fluentd-buffer-limit"
	dataLogDriverSocketPath     = "/socket/fluent.sock"
	socketPathPrefix            = "unix://"

	// fluentTagDockerFormat is the format for the log tag, which is "containerName-firelens-taskID"
	fluentTagDockerFormat = "%s-firelens-%s"

	// Environment variables are needed for firelens
	fluentNetworkHost      = "FLUENT_HOST"
	fluentNetworkPort      = "FLUENT_PORT"
	FluentNetworkPortValue = "24224"
	FluentAWSVPCHostValue  = "127.0.0.1"
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

func (engine *DockerTaskEngine) setupFireLensEnvironment(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) dockerapi.DockerContainerMetadata {
	firelensConfig := container.GetFirelensConfig()
	if firelensConfig != nil {
		err := task.AddFirelensContainerBindMounts(firelensConfig, hostConfig, engine.cfg)
		if err != nil {
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(err)}
		}

		cerr := task.PopulateSecretLogOptionsToFirelensContainer(container)
		if cerr != nil {
			return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(cerr)}
		}

		if firelensConfig.Type == firelens.FirelensConfigTypeFluentd {
			// For fluentd router, needs to specify FLUENT_UID to root in order for the fluentd process to access
			// the socket created by Docker.
			container.MergeEnvironmentVariables(map[string]string{
				"FLUENT_UID": "0",
			})
		}
	}

	// If the container is using a special log driver type "awsfirelens", it means the container wants to use
	// the firelens container to send logs. In this case, override the log driver type to be fluentd
	// and specify appropriate tag and fluentd-address, so that the logs are sent to and routed by the firelens container.
	// Update the environment variables FLUENT_HOST and FLUENT_PORT depending on the supported network modes - bridge
	// and awsvpc. For reference - https://docs.docker.com/config/containers/logging/fluentd/.
	if hostConfig.LogConfig.Type == logDriverTypeFirelens {
		hostConfig.LogConfig = getFirelensLogConfig(task, container, hostConfig, engine.cfg)
		if task.IsNetworkModeAWSVPC() {
			container.MergeEnvironmentVariables(map[string]string{
				fluentNetworkHost: FluentAWSVPCHostValue,
				fluentNetworkPort: FluentNetworkPortValue,
			})
		} else if container.GetNetworkModeFromHostConfig() == "" || container.GetNetworkModeFromHostConfig() == apitask.BridgeNetworkMode {
			ipAddress, ok := getContainerHostIP(task.GetFirelensContainer().GetNetworkSettings())
			if !ok {
				err := apierrors.DockerClientConfigError{Msg: "unable to get BridgeIP for task in bridge  mode"}
				return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(&err)}
			}
			container.MergeEnvironmentVariables(map[string]string{
				fluentNetworkHost: ipAddress,
				fluentNetworkPort: FluentNetworkPortValue,
			})
		}
	}
	return dockerapi.DockerContainerMetadata{}
}

func getFirelensLogConfig(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig, cfg *config.Config) dockercontainer.LogConfig {
	fields := strings.Split(task.Arn, "/")
	taskID := fields[len(fields)-1]
	tag := fmt.Sprintf(fluentTagDockerFormat, container.Name, taskID)
	fluentd := socketPathPrefix + filepath.Join(cfg.DataDirOnHost, dataLogDriverPath, taskID, dataLogDriverSocketPath)
	logConfig := hostConfig.LogConfig
	bufferLimit, bufferLimitExists := logConfig.Config[apitask.FirelensLogDriverBufferLimitOption]
	logConfig.Type = logDriverTypeFluentd
	logConfig.Config = make(map[string]string)
	logConfig.Config[logDriverTag] = tag
	logConfig.Config[logDriverFluentdAddress] = fluentd
	logConfig.Config[logDriverAsyncConnect] = strconv.FormatBool(true)
	logConfig.Config[logDriverSubSecondPrecision] = strconv.FormatBool(true)
	if bufferLimitExists {
		logConfig.Config[logDriverBufferLimit] = bufferLimit
	}
	seelog.Debugf("Applying firelens log config for container %s: %v", container.Name, logConfig)
	return logConfig
}
