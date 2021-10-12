//go:build windows

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
	"context"
	"fmt"
	"net"
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
	ecsBridgeEndpointNameFormat = "vEthernet (%s-ep-%s)"
	// taskPrimaryEndpointNameFormat is the name format of the primary endpoint in the task namespace.
	taskPrimaryEndpointNameFormat = "vEthernet (%sbr%s-ep-%s)"
	// windowsRouteAddCmdFormat is the format of command for adding route entry on Windows.
	windowsRouteAddCmdFormat = `netsh interface ipv4 add route prefix=%s interface="%s"`
	// windowsRouteDeleteCmdFormat is the format of command for deleting route entry on Windowsx.
	windowsRouteDeleteCmdFormat = `netsh interface ipv4 delete route prefix=%s interface="%s"`
)

// ConfigureTaskNamespaceRouting executes the commands required for setting up appropriate routing inside task namespace.
// The commands currently executed are-
// netsh interface ipv4 delete route prefix=0.0.0.0/0 interface="vEthernet (nat-ep-<container_id>)
// netsh interface ipv4 delete route prefix=<ecs-bridge-subnet-cidr> interface="vEthernet (nat-ep-<container_id>)
// netsh interface ipv4 add route prefix=169.254.170.2/32 interface="vEthernet (nat-ep-<container_id>)
// netsh interface ipv4 add route prefix=169.254.169.254/32 interface="vEthernet (task-br-<mac>-ep-<container_id>)
// netsh interface ipv4 add route prefix=169.254.169.254/32 interface="Loopback"
// netsh interface ipv4 add route prefix=<local-route> interface="vEthernet (nat-ep-<container_id>)
func (nsHelper *helper) ConfigureTaskNamespaceRouting(ctx context.Context, taskENI *apieni.ENI, config *Config, result *current.Result) error {
	// Obtain the ecs-bridge endpoint's subnet IP address from the CNI plugin execution result.
	ecsBridgeSubnetIPAddress := &net.IPNet{
		IP:   result.IPs[0].Address.IP.Mask(result.IPs[0].Address.Mask),
		Mask: result.IPs[0].Address.Mask,
	}
	ecsBridgeEndpointName := fmt.Sprintf(ecsBridgeEndpointNameFormat, ECSBridgeNetworkName, config.ContainerID)

	// Prepare the commands to be executed inside task namespace to setup the ECS Bridge.
	defaultRouteDeletionCmd := fmt.Sprintf(windowsRouteDeleteCmdFormat, windowsDefaultRoute, ecsBridgeEndpointName)
	defaultSubnetRouteDeletionCmd := fmt.Sprintf(windowsRouteDeleteCmdFormat, ecsBridgeSubnetIPAddress.String(),
		ecsBridgeEndpointName)
	credentialsAddressRouteAdditionCmd := fmt.Sprintf(windowsRouteAddCmdFormat, credentialsEndpointRoute, ecsBridgeEndpointName)
	commands := []string{defaultRouteDeletionCmd, defaultSubnetRouteDeletionCmd, credentialsAddressRouteAdditionCmd}

	// If IMDS is required, then create an explicit route through the primary interface of the task.
	if !config.BlockInstanceMetadata {
		// This naming convention is drawn from the way CNI plugin names the endpoints.
		// https://github.com/aws/amazon-vpc-cni-plugins/blob/master/plugins/vpc-eni/network/network_windows.go
		taskPrimaryEndpointId := strings.Replace(strings.ToLower(taskENI.MacAddress), ":", "", -1)
		taskPrimaryEndpointName := fmt.Sprintf(taskPrimaryEndpointNameFormat, TaskHNSNetworkNamePrefix,
			taskPrimaryEndpointId, config.ContainerID)
		imdsRouteAdditionCmd := fmt.Sprintf(windowsRouteAddCmdFormat, imdsEndpointIPAddress, taskPrimaryEndpointName)
		commands = append(commands, imdsRouteAdditionCmd)
	}

	// Add any additional route which needs to be routed via ecs-bridge.
	for _, route := range config.AdditionalLocalRoutes {
		ipRoute := &net.IPNet{
			IP:   route.IP,
			Mask: route.Mask,
		}
		additionalRouteAdditionCmd := fmt.Sprintf(windowsRouteAddCmdFormat, ipRoute.String(), ecsBridgeEndpointName)
		commands = append(commands, additionalRouteAdditionCmd)
	}

	// Invoke the generated commands inside the task namespace.
	// On Windows 2004 and 20H2, there are a few stdout messages which leads to the failure of the tasks.
	// By using &, we ensure that the commands are executed irrespectively. In case of error, the
	// other commands can be executed (all commands are independent of each other) and the error would be
	// reported to agent. Since the agent will kill the task, execution of other commands is immaterial.
	err := nsHelper.invokeCommandsInsideContainer(ctx, config.ContainerID, commands, " & ")
	if err != nil {
		return errors.Wrapf(err, "failed to execute commands inside task namespace")
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
