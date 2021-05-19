// +build windows,unit

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
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/golang/mock/gomock"
)

const (
	containerID     = "abc12345"
	containerExecID = "container1234"
)

// getECSBridgeResult returns an instance of current.Result.
func getECSBridgeResult() *current.Result {
	return &current.Result{
		IPs: []*current.IPConfig{{
			Address: net.IPNet{
				IP:   net.ParseIP(ipv4),
				Mask: net.CIDRMask(24, 32),
			},
		}},
	}
}

func TestConfigureTaskNamespaceRouting(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	cniConig := getCNIConfig()

	bridgeEpName := fmt.Sprintf(ecsBridgeEndpointNameFormat, ECSBridgeNetworkName, containerID)
	cmd1 := fmt.Sprintf(ecsBridgeRouteDeleteCmdFormat, windowsDefaultRoute, bridgeEpName)
	cmd2 := fmt.Sprintf(ecsBridgeRouteDeleteCmdFormat, "10.0.0.0/24", bridgeEpName)
	cmd3 := fmt.Sprintf(ecsBridgeRouteAddCmdFormat, credentialsEndpointRoute, bridgeEpName)
	finalCmd := strings.Join([]string{cmd1, cmd2, cmd3}, " && ")

	gomock.InOrder(
		dockerClient.EXPECT().CreateContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(_ context.Context, container string, execConfig types.ExecConfig, _ time.Duration) {
				assert.Equal(t, container, containerID)
				assert.Len(t, execConfig.Cmd, 3)
				assert.Equal(t, execConfig.Cmd[2], finalCmd)
				assert.Equal(t, execConfig.User, containerAdminUser)
			}).Return(&types.IDResponse{ID: containerExecID}, nil),
		dockerClient.EXPECT().StartContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(_ context.Context, execID string, execStartCheck types.ExecStartCheck, _ time.Duration) {
				assert.Equal(t, execID, containerExecID)
				assert.False(t, execStartCheck.Detach)
			}).Return(nil),
		dockerClient.EXPECT().InspectContainerExec(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			&types.ContainerExecInspect{
				ExitCode: 0,
				Running:  false,
			}, nil),
	)

	nsHelper := NewNamespaceHelper(dockerClient)
	err := nsHelper.ConfigureTaskNamespaceRouting(ctx, cniConig, getECSBridgeResult())
	assert.NoError(t, err)
}

func TestConfigureFirewallForTaskNSSetup(t *testing.T) {
	taskENI := getTaskENI()
	cniConfig := getCNIConfig()
	cniConfig.BlockInstanceMetadata = true

	firewallRuleName := fmt.Sprintf(blockIMDSFirewallRuleNameFormat, taskENI.GetPrimaryIPv4Address())
	checkExistingFirewallRule := fmt.Sprintf(checkExistingFirewallRuleCmdFormat, firewallRuleName)
	blockIMDSFirewallRuleCreationCmd := fmt.Sprintf(addFirewallRuleCmdFormat, firewallRuleName,
		taskENI.GetPrimaryIPv4Address(), imdsEndpointIPAddress)

	execCmdsOnHostFn = func(commands []string, separator string) error {
		assert.Equal(t, checkExistingFirewallRule, commands[0])
		assert.Equal(t, blockIMDSFirewallRuleCreationCmd, commands[1])
		assert.Equal(t, " || ", separator)
		return nil
	}

	nsHelper := NewNamespaceHelper(nil)
	err := nsHelper.ConfigureFirewallForTaskNSSetup(taskENI, cniConfig)
	assert.NoError(t, err)
}

func TestConfigureFirewallForTaskNSCleanup(t *testing.T) {
	taskENI := getTaskENI()
	cniConfig := getCNIConfig()
	cniConfig.BlockInstanceMetadata = true

	firewallRuleName := fmt.Sprintf(blockIMDSFirewallRuleNameFormat, taskENI.GetPrimaryIPv4Address())
	checkExistingFirewallRule := fmt.Sprintf(checkExistingFirewallRuleCmdFormat, firewallRuleName)
	blockIMDSFirewallRuleDeletionCmd := fmt.Sprintf(deleteFirewallRuleCmdFormat, firewallRuleName)

	execCmdsOnHostFn = func(commands []string, separator string) error {
		assert.Equal(t, checkExistingFirewallRule, commands[0])
		assert.Equal(t, blockIMDSFirewallRuleDeletionCmd, commands[1])
		assert.Equal(t, " && ", separator)
		return nil
	}
	nsHelper := NewNamespaceHelper(nil)
	err := nsHelper.ConfigureFirewallForTaskNSCleanup(taskENI, cniConfig)
	assert.NoError(t, err)
}
