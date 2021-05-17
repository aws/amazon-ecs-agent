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
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

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
	// windowsDefaultRoute is the default route of any endpoint.
	windowsDefaultRoute = "0.0.0.0/0"
	// credentialsEndpointRoute is the route of credentials endpoint for accessing task iam roles/task metadata.
	credentialsEndpointRoute = "169.254.170.2/32"
	// imdsEndpointIPAddress is the IP address of the endpoint for accessing IMDS.
	imdsEndpointIPAddress = "169.254.169.254"
	// ecsBridgeEndpointNameFormat is the name format of the ecs-bridge endpoint in the task namespace.
	ecsBridgeEndpointNameFormat = "%s-ep-%s"
	// blockIMDSFirewallRuleNameFormat is the format of firewall rule name for blocking IMDS from task namespace.
	blockIMDSFirewallRuleNameFormat = "Disable IMDS for %s"
	// ecsBridgeRouteAddCmdFormat is the format of command for adding route entry through ECS Bridge.
	ecsBridgeRouteAddCmdFormat = `netsh interface ipv4 add route prefix=%s interface="vEthernet (%s)"`
	// ecsBridgeRouteDeleteCmdFormat is the format of command for deleting route entry of ECS bridge endpoint.
	ecsBridgeRouteDeleteCmdFormat = `netsh interface ipv4 delete route prefix=%s interface="vEthernet (%s)"`
	// checkExistingFirewallRuleCmdFormat is the format of the command to check if the firewall rule exists.
	checkExistingFirewallRuleCmdFormat = `netsh advfirewall firewall show rule name="%s" >nul`
	// addFirewallRuleCmdFormat is the format of command for creating firewall rule on Windows.
	addFirewallRuleCmdFormat = `netsh advfirewall firewall add rule name="%s" dir=out localip=%s remoteip=%s action=block`
	// deleteFirewallRuleCmdFormat is the format of the command to delete a firewall rule on Windows.
	deleteFirewallRuleCmdFormat = `netsh advfirewall firewall delete rule name="%s" dir=out`
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
	ecsBridgeEndpointName := fmt.Sprintf(ecsBridgeEndpointNameFormat, ecscni.ECSBridgeNetworkName, config.ContainerID)

	// Prepare the commands to be executed inside pause namespace to setup the ECS Bridge.
	defaultRouteDeletionCmd := fmt.Sprintf(ecsBridgeRouteDeleteCmdFormat, windowsDefaultRoute, ecsBridgeEndpointName)
	defaultSubnetRouteDeletionCmd := fmt.Sprintf(ecsBridgeRouteDeleteCmdFormat, ecsBridgeSubnetIPAddress.String(),
		ecsBridgeEndpointName)
	credentialsAddressRouteAdditionCmd := fmt.Sprintf(ecsBridgeRouteAddCmdFormat, credentialsEndpointRoute, ecsBridgeEndpointName)
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

		firewallRuleName := fmt.Sprintf(blockIMDSFirewallRuleNameFormat, eni.GetPrimaryIPv4Address())
		checkExistingFirewallRule := fmt.Sprintf(checkExistingFirewallRuleCmdFormat, firewallRuleName)
		blockIMDSFirewallRuleCreationCmd := fmt.Sprintf(addFirewallRuleCmdFormat, firewallRuleName,
			eni.GetPrimaryIPv4Address(), imdsEndpointIPAddress)

		// Invoke the generated command on the host to add the firewall rule.
		// Separator is "||" as either the firewall rule should exist or a new one should be created.
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

		firewallRuleName := fmt.Sprintf(blockIMDSFirewallRuleNameFormat, eni.GetPrimaryIPv4Address())

		// Delete the firewall rule created for blocking IMDS access by the task.
		checkExistingFirewallRule := fmt.Sprintf(checkExistingFirewallRuleCmdFormat, firewallRuleName)
		blockIMDSFirewallRuleDeletionCmd := fmt.Sprintf(deleteFirewallRuleCmdFormat, firewallRuleName)

		// The separator would be "&&" to ensure if the firewall rule exists then delete it.
		// An error at this point means that the firewall rule was not present and was therefore not deleted.
		// Hence, skip returning the error as it is redundant.
		engine.invokeCommandsOnHost(task, []string{checkExistingFirewallRule, blockIMDSFirewallRuleDeletionCmd}, " && ")
	}

	return nil
}

// invokeCommands executes a set of commands inside the container namespace.
func (engine *DockerTaskEngine) invokeCommandsInsideContainer(ctx context.Context, task *apitask.Task,
	config *ecscni.Config, commands []string, separator string) error {

	seelog.Debugf("Task [%s]: Executing commands inside pause namespace: %v", task.Arn, commands)

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

	err = engine.client.StartContainerExec(ctx, execRes.ID, types.ExecStartCheck{Detach: false, Tty: false},
		dockerclient.ContainerExecStartTimeout)
	if err != nil {
		seelog.Errorf("Task [%s]: Failed to execute command in pause namespace [pre-start]: %v", task.Arn, err)
		return err
	}

	// Query the exec container to determine if the commands succeeded.
	inspect, err := engine.client.InspectContainerExec(ctx, execRes.ID, dockerclient.ContainerExecInspectTimeout)
	if err != nil {
		seelog.Errorf("Task [%s]: Failed to execute command in pause namespace [inspect]: %v", task.Arn, err)
		return err
	}

	// If the commands succeeded then return nil.
	if !inspect.Running && inspect.ExitCode != 0 {
		return errors.Errorf("failed to execute command in pause namespace: %d", inspect.ExitCode)
	}

	return nil
}

// invokeCommandsOnHost invokes given commands on the host instance.
func (engine *DockerTaskEngine) invokeCommandsOnHost(task *apitask.Task, commands []string, separator string) error {
	seelog.Debugf("Task [%s]: Executing commands on host: %v", task.Arn, commands)

	// Concatenate all the commands into a single command.
	execCommands := strings.Join(commands, separator)

	cmd := exec.Command("cmd", "/C", execCommands)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		seelog.Errorf("Task [%s]: Failed to execute command on host: %v: %s", task.Arn, err, stdout.String())
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
