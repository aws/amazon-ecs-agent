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
	"fmt"
	"strconv"
	"strings"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"
)

const (
	// Constants for CNI timeout during setup and cleanup on Windows.
	cniSetupTimeout   = 3 * time.Minute
	cniCleanupTimeout = 2 * time.Minute

	// logDriverTypeFirelens is the log driver type for containers that want to use the firelens container to send logs.
	logDriverTypeFirelens       = "awsfirelens"
	logDriverTypeFluentd        = "fluentd"
	logDriverTag                = "tag"
	logDriverFluentdAddress     = "fluentd-address"
	dataLogDriverPath           = "/data/firelens/"
	logDriverAsyncConnect       = "fluentd-async-connect"
	logDriverSubSecondPrecision = "fluentd-sub-second-precision"
	logDriverBufferLimit        = "fluentd-buffer-limit"

	// fluentTagDockerFormat is the format for the log tag, which is "containerName-firelens-taskID"
	fluentTagDockerFormat = "%s-firelens-%s"
	hostPortFormat        = "%s:%s"

	// Environment variables are needed for firelens
	fluentNetworkHost      = "FLUENT_HOST"
	fluentNetworkPort      = "FLUENT_PORT"
	FluentNetworkPortValue = "24224"
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

	cniConfig, err := engine.buildCNIConfigFromTaskContainer(task, containerInspectOutput, false)
	if err != nil {
		return errors.Wrap(err, "unable to build cni configuration")
	}

	// Invoke the cni plugin for the container using libcni
	_, err = engine.cniClient.SetupNS(engine.ctx, cniConfig, cniSetupTimeout)
	if err != nil {
		seelog.Errorf("Task engine [%s]: unable to configure container %v in the pause namespace: %v", task.Arn, container.Name, err)
		return errors.Wrap(err, "failed to connect HNS endpoint to container")
	}

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
	}

	// If the container is using a special log driver type "awsfirelens", it means the container wants to use
	// the firelens container to send logs. In this case, override the log driver type to be fluentd
	// and specify appropriate tag and fluentd-address, so that the logs are sent to and routed by the firelens container.
	// Update the environment variables FLUENT_HOST and FLUENT_PORT depending on the supported network modes - bridge
	// and awsvpc. For reference - https://docs.docker.com/config/containers/logging/fluentd/.
	if hostConfig.LogConfig.Type == logDriverTypeFirelens {
		hostConfig.LogConfig = getFirelensLogConfig(task, container, hostConfig, engine.cfg)
		if task.IsNetworkModeAWSVPC() {
			primaryENI := task.GetPrimaryENI()
			if primaryENI == nil {
				err := apierrors.DockerClientConfigError{Msg: "unable to get ENI for task in AWSVPC mode"}
				return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(&err)}
			}
			fluentIP := primaryENI.GetPrimaryIPv4Address()
			if fluentIP == "" {
				err := apierrors.DockerClientConfigError{Msg: "ENI does not have a primary IPv4 address"}
				return dockerapi.DockerContainerMetadata{Error: apierrors.NamedError(&err)}
			}
			container.MergeEnvironmentVariables(map[string]string{
				fluentNetworkHost: fluentIP,
				fluentNetworkPort: FluentNetworkPortValue,
			})
		} else if container.GetNetworkModeFromHostConfig() == "" || container.GetNetworkModeFromHostConfig() == apitask.BridgeNetworkMode {
			ipAddress := ""
			for _, network := range task.GetFirelensContainer().GetNetworkSettings().Networks {
				if network.IPAddress != "" {
					ipAddress = network.IPAddress
				}
			}
			if ipAddress == "" {
				ipAddress = task.GetFirelensContainer().GetNetworkSettings().DefaultNetworkSettings.IPAddress
			}
			if ipAddress == "" {
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
	fluentAddr := ""
	// ok := false
	if task.IsNetworkModeAWSVPC() {
		primaryENI := task.GetPrimaryENI()
		if primaryENI == nil {
			seelog.Error("unable to get ENI for task in AWSVPC mode")
		}
		fluentAddr = task.GetPrimaryENI().GetPrimaryIPv4Address()
	} else if container.GetNetworkModeFromHostConfig() == "" || container.GetNetworkModeFromHostConfig() == apitask.BridgeNetworkMode {
		// fluentAddr, ok = getContainerHostIP(task.GetFirelensContainer().GetNetworkSettings())
		for _, network := range task.GetFirelensContainer().GetNetworkSettings().Networks {
			if network.IPAddress != "" {
				fluentAddr = network.IPAddress
			}
		}
		if fluentAddr == "" {
			fluentAddr = task.GetFirelensContainer().GetNetworkSettings().DefaultNetworkSettings.IPAddress
		}
		if fluentAddr == "" {
			seelog.Error("unable to get BridgeIP of log router")
		}
	}
	fields := strings.Split(task.Arn, "/")
	taskID := fields[len(fields)-1]
	tag := fmt.Sprintf(fluentTagDockerFormat, container.Name, taskID)
	logConfig := hostConfig.LogConfig
	bufferLimit, bufferLimitExists := logConfig.Config[apitask.FirelensLogDriverBufferLimitOption]
	logConfig.Type = logDriverTypeFluentd
	logConfig.Config = make(map[string]string)
	logConfig.Config[logDriverTag] = tag
	logConfig.Config[logDriverFluentdAddress] = fmt.Sprintf(hostPortFormat, fluentAddr, FluentNetworkPortValue)
	logConfig.Config[logDriverAsyncConnect] = strconv.FormatBool(true)
	logConfig.Config[logDriverSubSecondPrecision] = strconv.FormatBool(true)
	if bufferLimitExists {
		logConfig.Config[logDriverBufferLimit] = bufferLimit
	}
	seelog.Debugf("Applying firelens log config for container %s: %v", container.Name, logConfig)
	return logConfig
}
