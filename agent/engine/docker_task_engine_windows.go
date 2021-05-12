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
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils/retry"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"

	"github.com/containernetworking/cni/pkg/types/current"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/docker/docker/api/types"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// Constants for CNI timeout during setup and cleanup on Windows.
	cniSetupTimeout   = 3 * time.Minute
	cniCleanupTimeout = 2 * time.Minute
	// containerAdminUser is the admin username for any container on Windows.
	containerAdminUser = "ContainerAdministrator"
	// Constants for creating backoff while querying state of command execution inside pause namespace.
	commandExecBackoffMin      = time.Second * 2
	commandExecBackoffMax      = time.Second * 10
	commandExecBackoffJitter   = 0.2
	commandExecBackoffMultiple = 1.3
	commandExecMaxRetryCount   = 3
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

// invokeCommandsForTaskNamespaceSetup invokes the necessary commands to setup various constructs of awsvpc-network mode for the task.
func (engine *DockerTaskEngine) invokeCommandsForTaskNamespaceSetup(ctx context.Context, task *apitask.Task,
	config *ecscni.Config, result *current.Result) error {
	ecsBridgeSubnetIPAddress := &net.IPNet{
		IP:   result.IPs[0].Address.IP.Mask(result.IPs[0].Address.Mask),
		Mask: result.IPs[0].Address.Mask,
	}
	ecsBridgeEndpointName := fmt.Sprintf(ecscni.ECSBridgeEndpointNameFormat, ecscni.ECSBridgeNetworkName, config.ContainerID)

	// Prepare the commands to be executed inside pause namespace to setup the ECS Bridge.
	defaultRouteDeletionCmd := fmt.Sprintf(ecscni.ECSBridgeDefaultRouteDeleteCmdFormat, ecsBridgeEndpointName)
	defaultSubnetRouteDeletionCmd := fmt.Sprintf(ecscni.ECSBridgeSubnetRouteDeleteCmdFormat, ecsBridgeSubnetIPAddress.String(),
		ecsBridgeEndpointName)
	credentialsAddressRouteAdditionCmd := fmt.Sprintf(ecscni.ECSBridgeCredentialsRouteAddCmdFormat, ecsBridgeEndpointName)
	commands := []string{defaultRouteDeletionCmd, defaultSubnetRouteDeletionCmd, credentialsAddressRouteAdditionCmd}

	// Invoke the generated commands inside the pause namespace.
	err := engine.invokeCommandsInsideContainer(ctx, task, config, commands, " && ")
	if err != nil {
		return errors.Wrapf(err, "failed to execute commands inside pause namespace")
	}

	// Create firewall rule if IMDS has to be disabled for the task.
	if config.BlockInstanceMetadata {
		eni := engine.getTaskENI(task)
		if eni == nil {
			return errors.New("could not find the task eni")
		}

		checkExistingFirewallRule := fmt.Sprintf(ecscni.ValidateExistingFirewallRuleCmdFormat, eni.GetPrimaryIPv4Address())
		blockIMDSFirewallRuleCreationCmd := fmt.Sprintf(ecscni.BlockIMDSFirewallAddRuleCmdFormat,
			eni.GetPrimaryIPv4Address(), eni.GetPrimaryIPv4Address())
		err = engine.invokeCommandsOnHost(task, []string{checkExistingFirewallRule, blockIMDSFirewallRuleCreationCmd}, " || ")
		if err != nil {
			return errors.Wrapf(err, "failed to create firewall rule to disable imds")
		}
	}

	return nil
}

// invokeCommandsForTaskNamespaceCleanup invokes the necessary commands to cleanup the constructs of awsvpc-network mode for the task.
func (engine *DockerTaskEngine) invokeCommandsForTaskNamespaceCleanup(task *apitask.Task, config *ecscni.Config) error {
	if config.BlockInstanceMetadata {
		eni := engine.getTaskENI(task)
		if eni == nil {
			return errors.New("could not find the task eni")
		}

		// Delete the firewall rule created for blocking IMDS access by the task.
		checkExistingFirewallRule := fmt.Sprintf(ecscni.ValidateExistingFirewallRuleCmdFormat, eni.GetPrimaryIPv4Address())
		blockIMDSFirewallRuleDeletionCmd := fmt.Sprintf(ecscni.BlockIMDSFirewallDeleteRuleCmdFormat, eni.GetPrimaryIPv4Address())
		err := engine.invokeCommandsOnHost(task, []string{checkExistingFirewallRule, blockIMDSFirewallRuleDeletionCmd}, " && ")
		if err != nil {
			return errors.Wrapf(err, "failed to delete firewall rule to disable imds")
		}
	}

	return nil
}

// invokeCommands executes a set of commands inside the container namespace.
func (engine *DockerTaskEngine) invokeCommandsInsideContainer(ctx context.Context, task *apitask.Task,
	config *ecscni.Config, commands []string, separator string) error {

	seelog.Infof("Task [%s]: Executing commands inside pause namespace: %v", task.Arn, commands)

	// Concatenate all the commands into a single command.
	execCommands := strings.Join(commands, separator)
	// Prepare the config command.
	cfgCommand := []string{"cmd", "/C", execCommands}

	execCfg := types.ExecConfig{
		Detach: false,
		Cmd:    cfgCommand,
		User:   containerAdminUser,
	}

	execRes, err := engine.client.CreateContainerExec(ctx, config.ContainerID, execCfg, dockerclient.ContainerExecCreateTimeout)
	if err != nil {
		seelog.Errorf("Task [%s]: Failed to execute command in pause namespace [create]: %v", task.Arn, err)
		return err
	}

	err = engine.client.StartContainerExec(ctx, execRes.ID, dockerclient.ContainerExecStartTimeout)
	if err != nil {
		seelog.Errorf("Task [%s]: Failed to execute command in pause namespace [pre-start]: %v", task.Arn, err)
		return err
	}

	// Query the exec container using retry to determine if the commands succeeded.
	backoff := retry.NewExponentialBackoff(commandExecBackoffMin, commandExecBackoffMax,
		commandExecBackoffJitter, commandExecBackoffMultiple)
	for count := 0; count < commandExecMaxRetryCount; count++ {
		inspect, err := engine.client.InspectContainerExec(ctx, execRes.ID, dockerclient.ContainerExecInspectTimeout)
		if err != nil {
			seelog.Errorf("Task [%s]: Failed to execute command in pause namespace [inspect]: %v", task.Arn, err)
			return err
		}

		// If the commands succeeded then return nil.
		if !inspect.Running && inspect.ExitCode == 0 {
			return nil
		}

		if count < commandExecMaxRetryCount-1 {
			time.Sleep(backoff.Duration())
		}
	}

	// If the commands did not succeed then return error.
	return errors.Errorf("failed to execute command in pause namespace: %v", commands)
}

// invokeCommandsOnHost invokes given commands on the host instance.
func (engine *DockerTaskEngine) invokeCommandsOnHost(task *apitask.Task, commands []string, separator string) error {
	seelog.Infof("Task [%s]: Executing commands on host: %v", task.Arn, commands)

	// Concatenate all the commands into a single command.
	execCommands := strings.Join(commands, separator)

	cmd := exec.Command("cmd", "/C", execCommands)
	err := cmd.Run()
	if err != nil {
		seelog.Errorf("Task [%s]: Failed to execute powershell command on host: %v", task.Arn, err)
		return err
	}

	return nil
}

// getTaskENI returns the primary eni of the task.
func (engine *DockerTaskEngine) getTaskENI(task *apitask.Task) *apieni.ENI {
	for _, eni := range task.GetTaskENIs() {
		if eni.InterfaceAssociationProtocol == "" || eni.InterfaceAssociationProtocol == apieni.DefaultInterfaceAssociationProtocol {
			return eni
		}
	}
	return nil
}
