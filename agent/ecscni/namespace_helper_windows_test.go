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

	cnitypes "github.com/containernetworking/cni/pkg/types"

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
	var tests = []struct {
		name      string
		blockIMDS bool
	}{
		{
			name:      "DisabledIMDS",
			blockIMDS: true,
		},
		{
			name:      "EnabledIMDS",
			blockIMDS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
			taskENI := getTaskENI()
			cniConfig := getCNIConfig()
			cniConfig.BlockInstanceMetadata = tt.blockIMDS

			cniConfig.AdditionalLocalRoutes = append(cniConfig.AdditionalLocalRoutes, cnitypes.IPNet{
				IP:   net.ParseIP("10.0.0.0"),
				Mask: net.CIDRMask(24, 32),
			})

			bridgeEpName := fmt.Sprintf(ecsBridgeEndpointNameFormat, ECSBridgeNetworkName, containerID)
			taskEpId := strings.Replace(strings.ToLower(taskENI.MacAddress), ":", "", -1)
			taskEPName := fmt.Sprintf(taskPrimaryEndpointNameFormat, TaskHNSNetworkNamePrefix, taskEpId, containerID)

			cmd1 := fmt.Sprintf(windowsRouteDeleteCmdFormat, windowsDefaultRoute, bridgeEpName)
			cmd2 := fmt.Sprintf(windowsRouteDeleteCmdFormat, "10.0.0.0/24", bridgeEpName)
			cmd3 := fmt.Sprintf(windowsRouteAddCmdFormat, credentialsEndpointRoute, bridgeEpName)

			var cmd4 string
			if cniConfig.BlockInstanceMetadata {
				cmd4 = fmt.Sprintf(windowsRouteAddCmdFormat, imdsEndpointIPAddress, loopbackInterfaceName)
			} else {
				cmd4 = fmt.Sprintf(windowsRouteAddCmdFormat, imdsEndpointIPAddress, taskEPName)
			}
			cmd5 := fmt.Sprintf(windowsRouteAddCmdFormat, "10.0.0.0/24", bridgeEpName)
			finalCmd := strings.Join([]string{cmd1, cmd2, cmd3, cmd4, cmd5}, " & ")

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
			err := nsHelper.ConfigureTaskNamespaceRouting(ctx, taskENI, cniConfig, getECSBridgeResult())
			assert.NoError(t, err)
		})
	}
}
