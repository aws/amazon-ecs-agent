//go:build !windows && unit
// +build !windows,unit

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

package platform

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	mock_ec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	mock_ecscni2 "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"
	mock_netwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

const (
	macAddress = "0a:1b:2c:3d:4e:5f"
)

func TestManagedLinux_TestConfigureInterface(t *testing.T) {
	t.Run("regular-eni", testManagedLinuxRegularENIConfiguration)
	t.Run("branch-eni", testManagedLinuxBranchENIConfiguration)
}

// testRegularENIConfiguration verifies the precise list of operations are invoked
// with the correct arguments while configuring a regular ENI on a host.
func testManagedLinuxRegularENIConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, osWrapper, cniClient, eni, managedLinuxPlatform := setupManagedLinuxTestConfigureInterface(ctrl, getTestRegularV4ENI)

	// When the ENI is the primary ENI.
	eniConfig := createENIPluginConfigs(netNSPath, eni)
	bridgeConfig := createBridgePluginConfig(netNSPath)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(managedLinuxPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Add(gomock.Any(), bridgeConfig).Return(nil, nil).Times(1),
		cniClient.EXPECT().Add(gomock.Any(), eniConfig).Return(nil, nil).Times(1),
	)
	err := managedLinuxPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)

	// Non-primary ENI case.
	eni.Default = false
	eniConfig = createENIPluginConfigs(netNSPath, eni)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(managedLinuxPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Add(gomock.Any(), eniConfig).Return(nil, nil).Times(1),
	)
	err = managedLinuxPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)

	// Delete workflow.
	eni.Default = true
	eni.DesiredStatus = status.NetworkDeleted
	eniConfig = createENIPluginConfigs(netNSPath, eni)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(managedLinuxPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Del(gomock.Any(), bridgeConfig).Return(nil).Times(1),
		cniClient.EXPECT().Del(gomock.Any(), eniConfig).Return(nil).Times(1),
	)
	err = managedLinuxPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)
}

func testManagedLinuxBranchENIConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, osWrapper, cniClient, eni, managedLinuxPlatform := setupManagedLinuxTestConfigureInterface(ctrl, getTestBranchV4ENI)

	eni.DesiredStatus = status.NetworkReadyPull
	bridgeConfig := createBridgePluginConfig(netNSPath)
	cniConfig := createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(managedLinuxPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Add(gomock.Any(), bridgeConfig).Return(nil, nil).Times(1),
		cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(nil, nil).Times(1),
	)
	err := managedLinuxPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)

	// Ready-Pull to Ready transition
	eni.DesiredStatus = status.NetworkReady
	osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(managedLinuxPlatform.stateDBDir, "eni-ipam.db"))
	err = managedLinuxPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)

	// Delete workflow.
	eni.DesiredStatus = status.NetworkDeleted
	osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(managedLinuxPlatform.stateDBDir, "eni-ipam.db"))
	cniClient.EXPECT().Del(gomock.Any(), bridgeConfig).Return(nil).Times(1)
	cniClient.EXPECT().Del(gomock.Any(), cniConfig).Return(nil).Times(1)
	err = managedLinuxPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)
}

func TestBuildDefaultNetworkNamespace(t *testing.T) {
	tests := []struct {
		name       string
		taskID     string
		setupMocks func(
			*mock_ec2.MockEC2MetadataClient,
			*mock_netwrapper.MockNet,
			*mock_netlinkwrapper.MockNetLink,
		)
		expectedError                error
		expectedIPAddress            string
		expectedSubnetGatewayAddress string
	}{
		{
			name:   "successful case for ipv4",
			taskID: "test-task-1",
			setupMocks: func(
				mockEC2Client *mock_ec2.MockEC2MetadataClient,
				mockNet *mock_netwrapper.MockNet,
				mockNetLink *mock_netlinkwrapper.MockNetLink) {
				mockEC2Client.EXPECT().GetMetadata(PrivateIPv4Address).Return("10.194.20.1", nil).Times(1)
				mockEC2Client.EXPECT().GetMetadata(MacResource).Return(macAddress, nil).Times(1)
				mockEC2Client.EXPECT().GetMetadata(InstanceIDResource).Return("i-1234567890abcdef0", nil).Times(1)
				mockEC2Client.EXPECT().GetMetadata(fmt.Sprintf(IPv4SubNetCidrBlock, macAddress)).
					Return("10.194.20.0/20", nil).
					Times(1)

				testMac, err := net.ParseMAC(macAddress)
				require.NoError(t, err)
				link1 := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{HardwareAddr: testMac}}
				mockNetLink.EXPECT().LinkList().Return([]netlink.Link{link1}, nil)
				routes := []netlink.Route{
					netlink.Route{
						Gw:        nil,
						Dst:       nil,
						LinkIndex: 0,
					},
					netlink.Route{
						Gw:        net.ParseIP("10.194.20.1"),
						Dst:       nil,
						LinkIndex: 0,
					},
				}
				mockNetLink.EXPECT().RouteList(link1, netlink.FAMILY_V4).Return(routes, nil).Times(1)
				mockNetLink.EXPECT().RouteList(link1, netlink.FAMILY_V6).Return(nil, nil).Times(1)

				testIface := []net.Interface{
					{
						HardwareAddr: testMac,
						Name:         "eth1",
					},
				}
				mockNet.EXPECT().Interfaces().Return(testIface, nil).Times(1)
			},
			expectedIPAddress:            "10.194.20.1",
			expectedSubnetGatewayAddress: "10.194.20.0/20",
			expectedError:                nil,
		},
		{
			name:   "successful case for ipv6",
			taskID: "test-task-1",
			setupMocks: func(
				mockEC2Client *mock_ec2.MockEC2MetadataClient,
				mockNet *mock_netwrapper.MockNet,
				mockNetLink *mock_netlinkwrapper.MockNetLink) {
				mockEC2Client.EXPECT().GetMetadata(PrivateIPv6Address).Return("fe80::406:baff:fef9:4305", nil).Times(1)
				mockEC2Client.EXPECT().GetMetadata(MacResource).Return(macAddress, nil).Times(1)
				mockEC2Client.EXPECT().GetMetadata(InstanceIDResource).Return("i-1234567890abcdef0", nil).Times(1)
				mockEC2Client.EXPECT().GetMetadata(fmt.Sprintf(IPv6SubNetCidrBlock, macAddress)).
					Return("fe80::406:baff:fef9:4305/60", nil).
					Times(1)

				testMac, err := net.ParseMAC(macAddress)
				require.NoError(t, err)
				link1 := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{HardwareAddr: testMac}}
				mockNetLink.EXPECT().LinkList().Return([]netlink.Link{link1}, nil)
				routes := []netlink.Route{
					netlink.Route{
						Gw:        nil,
						Dst:       nil,
						LinkIndex: 0,
					},
					netlink.Route{
						Gw:        net.ParseIP("10.194.20.1"),
						Dst:       nil,
						LinkIndex: 0,
					},
				}
				mockNetLink.EXPECT().RouteList(link1, netlink.FAMILY_V4).Return(nil, nil).Times(1)
				mockNetLink.EXPECT().RouteList(link1, netlink.FAMILY_V6).Return(routes, nil).Times(1)

				testIface := []net.Interface{
					{
						HardwareAddr: testMac,
						Name:         "eth1",
					},
				}
				mockNet.EXPECT().Interfaces().Return(testIface, nil).Times(1)
			},
			expectedIPAddress:            "fe80::406:baff:fef9:4305",
			expectedSubnetGatewayAddress: "fe80::406:baff:fef9:4305/60",
			expectedError:                nil,
		},
		{
			name:   "metadata client error",
			taskID: "test-task-1",
			setupMocks: func(
				mockEC2Client *mock_ec2.MockEC2MetadataClient,
				mockNet *mock_netwrapper.MockNet,
				mockNetLink *mock_netlinkwrapper.MockNetLink) {
				mockEC2Client.EXPECT().GetMetadata(gomock.Any()).
					Return("", fmt.Errorf("metadata client error")).
					AnyTimes()

				testMac, err := net.ParseMAC(macAddress)
				require.NoError(t, err)
				testIface := []net.Interface{
					{
						HardwareAddr: testMac,
						Name:         "eth1",
					},
				}
				mockNet.EXPECT().Interfaces().Return(testIface, nil).Times(1)
			},
			expectedError: fmt.Errorf("metadata client error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
			mockNet := mock_netwrapper.NewMockNet(ctrl)
			netLink := mock_netlinkwrapper.NewMockNetLink(ctrl)
			tt.setupMocks(mockMetadataClient, mockNet, netLink)

			commonPlatform := &common{
				net:     mockNet,
				netlink: netLink,
			}
			ml := &managedLinux{
				client: mockMetadataClient,
				common: *commonPlatform,
			}

			namespaces, err := ml.buildDefaultNetworkNamespace(tt.taskID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
				assert.Nil(t, namespaces)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, namespaces)
				assert.Len(t, namespaces, 1)

				ns := namespaces[0]
				// Verify namespace properties
				assert.Equal(t, status.NetworkReady, ns.KnownState)
				assert.Equal(t, status.NetworkReady, ns.DesiredState)

				// Verify network interface properties
				netInt := ns.NetworkInterfaces[0]
				assert.True(t, netInt.Default)
				assert.Equal(t, status.NetworkReady, netInt.DesiredStatus)
				assert.Equal(t, status.NetworkReady, netInt.KnownStatus)
				assert.Equal(t, "i-1234567890abcdef0", netInt.ID)
				assert.Equal(t, 1, len(netInt.IPV4Addresses)+len(netInt.IPV6Addresses))
				var ipAddr string
				for _, addr := range netInt.IPV4Addresses {
					ipAddr = addr.Address
				}
				for _, addr := range netInt.IPV6Addresses {
					ipAddr = addr.Address
				}
				assert.Equal(t, tt.expectedIPAddress, ipAddr)
				var subnetGatewayAddr string
				if netInt.SubnetGatewayIPV4Address != "" {
					subnetGatewayAddr = netInt.SubnetGatewayIPV4Address
				}
				if netInt.SubnetGatewayIPV6Address != "" {
					subnetGatewayAddr = netInt.SubnetGatewayIPV6Address
				}
				assert.Equal(t, tt.expectedSubnetGatewayAddress, subnetGatewayAddr)
			}
		})
	}
}

// setupManagedLinuxTestConfigureInterface provisions all the resources needed to facilitate the two
// subtests in TestManagedLinux_TestConfigureInterface.
func setupManagedLinuxTestConfigureInterface(
	ctrl *gomock.Controller, getTestENI func() *networkinterface.NetworkInterface) (
	context.Context, *mock_oswrapper.MockOS, *mock_ecscni2.MockCNI, *networkinterface.NetworkInterface, *managedLinux) {
	ctx := context.TODO()
	osWrapper := mock_oswrapper.NewMockOS(ctrl)
	cniClient := mock_ecscni2.NewMockCNI(ctrl)
	eni := getTestENI()
	managedLinuxPlatform := &managedLinux{
		common: common{
			os:         osWrapper,
			cniClient:  cniClient,
			stateDBDir: "dummy-db-dir",
		},
		client: nil,
	}
	return ctx, osWrapper, cniClient, eni, managedLinuxPlatform
}
