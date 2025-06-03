//go:build linux && unit
// +build linux,unit

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

package handlers

import (
	"errors"
	"fmt"
	"net"
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	agentV4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	mock_stats "github.com/aws/amazon-ecs-agent/agent/stats/mock"
	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	v4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils/netconfig"
	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

const (
	internalError                       = "internal error"
	defaultNetworkInterfaceErrorMessage = "failed to resolve default host network interface"
)

func TestV4GetTaskMetadataWithTaskNetworkConfig(t *testing.T) {

	tcs := []struct {
		name                      string
		setStateExpectations      func(state *mock_dockerstate.MockTaskEngineState)
		setNetLinkExpectations    func(netLink *mock_netlinkwrapper.MockNetLink)
		expectedTaskNetworkConfig *v4.TaskNetworkConfig
		shouldError               bool
		errorMessage              string
	}{
		{
			name: "happy case with awsvpc mode",
			setStateExpectations: func(state *mock_dockerstate.MockTaskEngineState) {
				task := standardTask()
				task.EnableFaultInjection = true
				task.NetworkNamespace = networkNamespace
				task.DefaultIfname = defaultIfname
				task.ENIs[0].IPV4Addresses = []*networkinterface.IPV4Address{{Address: "7.6.5.4"}}
				task.ENIs[0].IPV6Addresses = []*networkinterface.IPV6Address{{Address: "9:8:9:9::"}}
				gomock.InOrder(
					state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
					state.EXPECT().TaskByArn(taskARN).Return(task, true).Times(2),
					state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true).AnyTimes(),
					state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
					state.EXPECT().TaskByArn(taskARN).Return(task, true),
					state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true).AnyTimes(),
					state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
				)
			},
			expectedTaskNetworkConfig: state.NewTaskNetworkConfig(apitask.AWSVPCNetworkMode,
				networkNamespace,
				[]*state.NetworkInterface{
					{
						DeviceName:    defaultIfname,
						IPV4Addresses: []string{"7.6.5.4"},
						IPV6Addresses: []string{"9:8:9:9::"},
					},
				},
			),
		},
		{
			name: "happy case with host mode - single default ENI",
			setStateExpectations: func(state *mock_dockerstate.MockTaskEngineState) {
				hostTask := standardHostTask()
				hostTask.EnableFaultInjection = true
				hostTask.NetworkNamespace = networkNamespace
				hostTask.DefaultIfname = defaultIfname
				gomock.InOrder(
					state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
					state.EXPECT().TaskByArn(taskARN).Return(hostTask, true).Times(2),
					state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
					state.EXPECT().ContainerByID(containerID).Return(nil, false).AnyTimes(),
					state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
					state.EXPECT().ContainerByID(containerID).Return(nil, false).AnyTimes(),
				)
			},
			setNetLinkExpectations: func(netLink *mock_netlinkwrapper.MockNetLink) {
				v4Routes := []netlink.Route{
					{
						Gw:        net.ParseIP("10.194.20.1"),
						Dst:       nil,
						LinkIndex: 0,
					},
				}
				v6Routes := []netlink.Route{
					{
						Gw:        net.ParseIP("8:8:8:8::"),
						Dst:       nil,
						LinkIndex: 0,
					},
				}
				link := &netlink.Device{
					LinkAttrs: netlink.LinkAttrs{
						Index: 0,
						Name:  "eth0",
					},
				}
				addrs := []netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("1.2.3.4")}},
					{IPNet: &net.IPNet{IP: net.ParseIP("5:5:5:5::")}},
				}
				gomock.InOrder(
					netLink.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return(v4Routes, nil),
					netLink.EXPECT().LinkByIndex(link.Attrs().Index).Return(link, nil),
					netLink.EXPECT().AddrList(link, netlink.FAMILY_ALL).Return(addrs, nil),
					netLink.EXPECT().RouteList(nil, netlink.FAMILY_V6).Return(v6Routes, nil),
					netLink.EXPECT().LinkByIndex(link.Attrs().Index).Return(link, nil),
					netLink.EXPECT().AddrList(link, netlink.FAMILY_ALL).Return(addrs, nil),
				)
			},
			expectedTaskNetworkConfig: state.NewTaskNetworkConfig(apitask.HostNetworkMode,
				hostNetworkNamespace,
				[]*state.NetworkInterface{
					{
						DeviceName:    "eth0",
						IPV4Addresses: []string{"1.2.3.4"},
						IPV6Addresses: []string{"5:5:5:5::"},
					},
				},
			),
		},
		{
			name: "happy case with host mode - two default ENIs",
			setStateExpectations: func(state *mock_dockerstate.MockTaskEngineState) {
				hostTask := standardHostTask()
				hostTask.EnableFaultInjection = true
				hostTask.NetworkNamespace = networkNamespace
				hostTask.DefaultIfname = defaultIfname
				gomock.InOrder(
					state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
					state.EXPECT().TaskByArn(taskARN).Return(hostTask, true).Times(2),
					state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
					state.EXPECT().ContainerByID(containerID).Return(nil, false).AnyTimes(),
					state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
					state.EXPECT().ContainerByID(containerID).Return(nil, false).AnyTimes(),
				)
			},
			setNetLinkExpectations: func(netLink *mock_netlinkwrapper.MockNetLink) {
				v4Routes := []netlink.Route{
					{
						Gw:        net.ParseIP("10.194.20.1"),
						Dst:       nil,
						LinkIndex: 0,
					},
				}
				v6Routes := []netlink.Route{
					{
						Gw:        net.ParseIP("8:8:8:8::"),
						Dst:       nil,
						LinkIndex: 1,
					},
				}
				v4Link := &netlink.Device{
					LinkAttrs: netlink.LinkAttrs{
						Index: 0,
						Name:  "eth0",
					},
				}
				v4LinkAddrs := []netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("1.2.3.4")}},
				}
				v6Link := &netlink.Device{
					LinkAttrs: netlink.LinkAttrs{
						Index: 1,
						Name:  "eth1",
					},
				}
				v6LinkAddrs := []netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("5:5:5:5::")}},
				}
				gomock.InOrder(
					netLink.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return(v4Routes, nil),
					netLink.EXPECT().LinkByIndex(v4Link.Attrs().Index).Return(v4Link, nil),
					netLink.EXPECT().AddrList(v4Link, netlink.FAMILY_ALL).Return(v4LinkAddrs, nil),
					netLink.EXPECT().RouteList(nil, netlink.FAMILY_V6).Return(v6Routes, nil),
					netLink.EXPECT().LinkByIndex(v6Link.Attrs().Index).Return(v6Link, nil),
					netLink.EXPECT().AddrList(v6Link, netlink.FAMILY_ALL).Return(v6LinkAddrs, nil),
				)
			},
			expectedTaskNetworkConfig: state.NewTaskNetworkConfig(apitask.HostNetworkMode,
				hostNetworkNamespace,
				[]*state.NetworkInterface{
					{
						DeviceName:    "eth0",
						IPV4Addresses: []string{"1.2.3.4"},
					},
					{
						DeviceName:    "eth1",
						IPV6Addresses: []string{"5:5:5:5::"},
					},
				},
			),
		},
		{
			name: "happy bridge mode",
			setStateExpectations: func(state *mock_dockerstate.MockTaskEngineState) {
				gomock.InOrder(
					state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
					state.EXPECT().TaskByArn(taskARN).Return(bridgeTask, true).Times(2),
					state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToBridgeContainer, true),
					state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true).AnyTimes(),
					state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
					state.EXPECT().ContainerByID(containerID).Return(bridgeContainer, true).AnyTimes(),
				)
			},
			expectedTaskNetworkConfig: state.NewTaskNetworkConfig(bridgeMode, "", nil),
		},
		{
			name: "unhappy case with host mode",
			setStateExpectations: func(state *mock_dockerstate.MockTaskEngineState) {
				hostTask := standardHostTask()
				hostTask.EnableFaultInjection = true
				hostTask.NetworkNamespace = networkNamespace
				hostTask.DefaultIfname = defaultIfname
				gomock.InOrder(
					state.EXPECT().TaskARNByV3EndpointID(v3EndpointID).Return(taskARN, true),
					state.EXPECT().TaskByArn(taskARN).Return(hostTask, true).Times(2),
					state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
					state.EXPECT().ContainerByID(containerID).Return(nil, false).AnyTimes(),
					state.EXPECT().PulledContainerMapByArn(taskARN).Return(nil, true),
					state.EXPECT().ContainerByID(containerID).Return(nil, false).AnyTimes(),
				)
			},
			setNetLinkExpectations: func(netLink *mock_netlinkwrapper.MockNetLink) {
				gomock.InOrder(
					netLink.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return(nil, errors.New(internalError)).Times(1),
				)
			},
			expectedTaskNetworkConfig: state.NewTaskNetworkConfig(
				apitask.HostNetworkMode, hostNetworkNamespace,
				[]*state.NetworkInterface{{DeviceName: defaultIfname}}),
			shouldError: true,
			errorMessage: fmt.Sprintf("%s: %s: %s",
				defaultNetworkInterfaceErrorMessage, "failed to get routes", "internal error"),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			state := mock_dockerstate.NewMockTaskEngineState(ctrl)
			statsEngine := mock_stats.NewMockEngine(ctrl)
			ecsClient := mock_ecs.NewMockECSClient(ctrl)

			if tc.setStateExpectations != nil {
				tc.setStateExpectations(state)
			}
			tmdsAgentState := agentV4.NewTMDSAgentState(state, statsEngine, ecsClient, clusterName, availabilityzone, vpcID, containerInstanceArn)

			netConfigClient := netconfig.NewNetworkConfigClient()

			if tc.setNetLinkExpectations != nil {
				mock_netlinkwrapper := mock_netlinkwrapper.NewMockNetLink(ctrl)
				tc.setNetLinkExpectations(mock_netlinkwrapper)
				netConfigClient.NetlinkClient = mock_netlinkwrapper
			}

			actualTaskResponse, err := tmdsAgentState.GetTaskMetadataWithTaskNetworkConfig(v3EndpointID, netConfigClient)

			if tc.shouldError {
				var errDefaultNetworkInterfaceName *v4.ErrorDefaultNetworkInterface
				assert.Error(t, err)
				assert.ErrorAs(t, err, &errDefaultNetworkInterfaceName)
				assert.Equal(t, tc.errorMessage, err.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedTaskNetworkConfig, actualTaskResponse.TaskNetworkConfig)
		})
	}
}
