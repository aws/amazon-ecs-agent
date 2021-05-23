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

package ecscni

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

const (
	// containerAdminUser is the admin username for any container on Windows.
	containerAdminUser = "ContainerAdministrator"
	// windowsDefaultRoute is the default route of any endpoint.
	windowsDefaultRoute = "0.0.0.0/0"
	// credentialsEndpointRoute is the route of credentials endpoint for accessing task iam roles/task metadata.
	credentialsEndpointRoute = "169.254.170.2/32"
	// imdsEndpointIPAddress is the IP address of the endpoint for accessing IMDS.
	imdsEndpointIPAddress = "169.254.169.254/32"
	// ecsBridgeEndpointNameFormat is the name format of the ecs-bridge endpoint in the task namespace.
	ecsBridgeEndpointNameFormat = "%s-ep-%s"
	// taskPrimaryEndpointNameFormat is the name format of the primary endpoint in the task namespace.
	taskPrimaryEndpointNameFormat = "task-br-%s-ep-%s"
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

var execCmdExecutorFn execCmdExecutorFnType = execCmdExecutor

// ConfigureTaskNamespaceRouting executes the commands required for setting up appropriate routing inside task namespace.
func (nsHelper *helper) ConfigureTaskNamespaceRouting(ctx context.Context, taskENI *apieni.ENI, config *Config, result *current.Result) error {
	// Obtain the ecs-bridge endpoint's subnet IP address from the CNI plugin execution result.
	ecsBridgeSubnetIPAddress := &net.IPNet{
		IP:   result.IPs[0].Address.IP.Mask(result.IPs[0].Address.Mask),
		Mask: result.IPs[0].Address.Mask,
	}
	ecsBridgeEndpointName := fmt.Sprintf(ecsBridgeEndpointNameFormat, ECSBridgeNetworkName, config.ContainerID)

	// Prepare the commands to be executed inside task namespace to setup the ECS Bridge.
	defaultRouteDeletionCmd := fmt.Sprintf(ecsBridgeRouteDeleteCmdFormat, windowsDefaultRoute, ecsBridgeEndpointName)
	defaultSubnetRouteDeletionCmd := fmt.Sprintf(ecsBridgeRouteDeleteCmdFormat, ecsBridgeSubnetIPAddress.String(),
		ecsBridgeEndpointName)
	credentialsAddressRouteAdditionCmd := fmt.Sprintf(ecsBridgeRouteAddCmdFormat, credentialsEndpointRoute, ecsBridgeEndpointName)
	commands := []string{defaultRouteDeletionCmd, defaultSubnetRouteDeletionCmd, credentialsAddressRouteAdditionCmd}

	if !config.BlockInstanceMetadata {
		// This naming convention is drawn from the way CNI plugin names the endpoints.
		taskPrimaryEndpointId := strings.Replace(strings.ToLower(taskENI.MacAddress), ":", "", -1)
		taskPrimaryEndpointName := fmt.Sprintf(taskPrimaryEndpointNameFormat, taskPrimaryEndpointId, config.ContainerID)
		imdsRouteAdditionCmd := fmt.Sprintf(ecsBridgeRouteAddCmdFormat, imdsEndpointIPAddress, taskPrimaryEndpointName)
		commands = append(commands, imdsRouteAdditionCmd)
	}

	// Add any additional route which needs to be routed via ecs-bridge.
	if len(config.AdditionalLocalRoutes) != 0 {
		for _, route := range config.AdditionalLocalRoutes {
			ipRoute := &net.IPNet{
				IP:   route.IP,
				Mask: route.Mask,
			}
			additionalRouteAdditionCmd := fmt.Sprintf(ecsBridgeRouteAddCmdFormat, ipRoute.String(), ecsBridgeEndpointName)
			commands = append(commands, additionalRouteAdditionCmd)
		}
	}

	// Invoke the generated commands inside the task namespace.
	err := nsHelper.invokeCommandsInsideContainer(ctx, config.ContainerID, commands, " && ")
	if err != nil {
		return errors.Wrapf(err, "failed to execute commands inside task namespace")
	}
	return nil
}

// ConfigureFirewallForTaskNSSetup executes the commands, if required, to setup firewall rules for disabling IMDS access from task.
func (nsHelper *helper) ConfigureFirewallForTaskNSSetup(taskENI *apieni.ENI, config *Config) error {
	if config.BlockInstanceMetadata {
		if taskENI == nil {
			return errors.New("failed to configure firewall due to invalid task eni")
		}

		firewallRuleName := fmt.Sprintf(blockIMDSFirewallRuleNameFormat, taskENI.GetPrimaryIPv4Address())

		checkExistingFirewallRule := fmt.Sprintf(checkExistingFirewallRuleCmdFormat, firewallRuleName)
		blockIMDSFirewallRuleCreationCmd := fmt.Sprintf(addFirewallRuleCmdFormat, firewallRuleName,
			taskENI.GetPrimaryIPv4Address(), imdsEndpointIPAddress)

		// Invoke the generated command on the host to add the firewall rule.
		// Separator is "||" as either the firewall rule should exist or a new one should be created.
		err := nsHelper.invokeCommandsOnHost([]string{checkExistingFirewallRule, blockIMDSFirewallRuleCreationCmd}, " || ")
		if err != nil {
			return errors.Wrapf(err, "failed to create firewall rule to disable imds")
		}
	}

	return nil
}

// ConfigureFirewallForTaskNSCleanup executes the commands, if required, to cleanup the firewall rules created during setup.
func (nsHelper *helper) ConfigureFirewallForTaskNSCleanup(taskENI *apieni.ENI, config *Config) error {
	if config.BlockInstanceMetadata {
		if taskENI == nil {
			return errors.New("failed to configure firewall due to invalid task eni")
		}

		firewallRuleName := fmt.Sprintf(blockIMDSFirewallRuleNameFormat, taskENI.GetPrimaryIPv4Address())

		// Delete the firewall rule created for blocking IMDS access by the task.
		checkExistingFirewallRule := fmt.Sprintf(checkExistingFirewallRuleCmdFormat, firewallRuleName)
		blockIMDSFirewallRuleDeletionCmd := fmt.Sprintf(deleteFirewallRuleCmdFormat, firewallRuleName)

		// The separator would be "&&" to ensure if the firewall rule exists then delete it.
		// An error at this point means that the firewall rule was not present and was therefore not deleted.
		// Hence, skip returning the error as it is redundant.
		nsHelper.invokeCommandsOnHost([]string{checkExistingFirewallRule, blockIMDSFirewallRuleDeletionCmd}, " && ")
	}

	return nil
}

// invokeCommandsInsideContainer executes a set of commands inside the container namespace.
func (nsHelper *helper) invokeCommandsInsideContainer(ctx context.Context, containerID string, commands []string, separator string) error {

	seelog.Debugf("[ECSCNI] Executing commands inside container %s namespace: %v", containerID, commands)

	// Concatenate all the commands into a single command.
	execCommands := strings.Join(commands, separator)
	// Prepare the config command.
	cfgCommand := []string{"cmd", "/C", execCommands}

	execCfg := types.ExecConfig{
		Detach: false,
		Cmd:    cfgCommand,
		User:   containerAdminUser,
	}

	execRes, err := nsHelper.dockerClient.CreateContainerExec(ctx, containerID, execCfg, dockerclient.ContainerExecCreateTimeout)
	if err != nil {
		seelog.Errorf("[ECSCNI] Failed to execute command in container %s namespace [create]: %v", containerID, err)
		return err
	}

	err = nsHelper.dockerClient.StartContainerExec(ctx, execRes.ID, types.ExecStartCheck{Detach: false, Tty: false},
		dockerclient.ContainerExecStartTimeout)
	if err != nil {
		seelog.Errorf("[ECSCNI] Failed to execute command in container %s namespace [pre-start]: %v", containerID, err)
		return err
	}

	// Query the exec container to determine if the commands succeeded.
	inspect, err := nsHelper.dockerClient.InspectContainerExec(ctx, execRes.ID, dockerclient.ContainerExecInspectTimeout)
	if err != nil {
		seelog.Errorf("[ECSCNI] Failed to execute command in container %s namespace [inspect]: %v", containerID, err)
		return err
	}
	// If the commands fail then return error.
	if !inspect.Running && inspect.ExitCode != 0 {
		return errors.Errorf("commands failed inside container %s namespace: %d", containerID, inspect.ExitCode)
	}

	return nil
}

// invokeCommandsOnHost invokes given commands on the host instance using the executeFn.
func (nsHelper *helper) invokeCommandsOnHost(commands []string, separator string) error {
	return nsHelper.execCmdExecutor(commands, separator)
}

// execCmdExecutor invokes given commands on the host instance.
func execCmdExecutor(commands []string, separator string) error {
	seelog.Debugf("[ECSCNI] Executing commands on host: %v", commands)

	// Concatenate all the commands into a single command.
	execCommands := strings.Join(commands, separator)

	cmd := exec.Command("cmd", "/C", execCommands)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		seelog.Errorf("[ECSCNI] Failed to execute command on host: %v: %s", err, stdout.String())
		return err
	}

	return nil
}
