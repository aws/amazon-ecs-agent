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
	"net"
	"path/filepath"
	"testing"

	mock_ecscni2 "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_nsutil"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper/mocks"

	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

func newIsolatedLinuxPlatform(ctrl *gomock.Controller) (*isolatedLinux, *mock_oswrapper.MockOS, *mock_ecscni2.MockCNI, *mock_ecscni.MockNetNSUtil, *mock_netlinkwrapper.MockNetLink) {
	osWrapper := mock_oswrapper.NewMockOS(ctrl)
	cniClient := mock_ecscni2.NewMockCNI(ctrl)
	mockNSUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
	mockNetLink := mock_netlinkwrapper.NewMockNetLink(ctrl)

	mockNSUtil.EXPECT().GetNetNSPath("host-daemon").Return("/var/run/netns/host-daemon").AnyTimes()
	mockNSUtil.EXPECT().NSExists("/var/run/netns/host-daemon").Return(false, nil).AnyTimes()

	p := &isolatedLinux{
		managedLinux: managedLinux{
			common: common{
				os:         osWrapper,
				cniClient:  cniClient,
				nsUtil:     mockNSUtil,
				netlink:    mockNetLink,
				stateDBDir: "dummy-db-dir",
			},
		},
	}
	return p, osWrapper, cniClient, mockNSUtil, mockNetLink
}

func TestIsolatedLinux_ConfigureInterface_SetsDeviceNameToEth0(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, osWrapper, cniClient, _, _ := newIsolatedLinuxPlatform(ctrl)
	eni := getTestRegularV4ENI()
	eni.DeviceName = "eth1"
	// Use NetworkDeleted so addGatewayNeighbor is not invoked.
	eni.DesiredStatus = status.NetworkDeleted

	osWrapper.EXPECT().Setenv(gomock.Any(), gomock.Any()).AnyTimes()
	cniClient.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	err := p.ConfigureInterface(context.TODO(), netNSPath, eni, nil)
	require.NoError(t, err)
	assert.Equal(t, networkinterface.DefaultTapDeviceName, eni.DeviceName)
}

func TestIsolatedLinux_ConfigureInterface_RegularENI(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, osWrapper, cniClient, _, _ := newIsolatedLinuxPlatform(ctrl)
	eni := getTestRegularV4ENI()
	eni.DesiredStatus = status.NetworkReadyPull

	eniConfig := createENIPluginConfigs(netNSPath, eni)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv(CNIPluginLogFileEnv, gomock.Any()),
		osWrapper.EXPECT().Setenv(IPAMDataPathEnv, filepath.Join("dummy-db-dir", IPAMDataFileName)),
		cniClient.EXPECT().Add(gomock.Any(), eniConfig).Return(nil, nil),
	)

	err := p.configureRegularENI(context.TODO(), netNSPath, eni)
	require.NoError(t, err)
}

func TestIsolatedLinux_ConfigureInterface_RegularENI_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, osWrapper, cniClient, _, _ := newIsolatedLinuxPlatform(ctrl)
	eni := getTestRegularV4ENI()
	eni.DesiredStatus = status.NetworkDeleted

	eniConfig := createENIPluginConfigs(netNSPath, eni)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv(CNIPluginLogFileEnv, gomock.Any()),
		osWrapper.EXPECT().Setenv(IPAMDataPathEnv, filepath.Join("dummy-db-dir", IPAMDataFileName)),
		cniClient.EXPECT().Del(gomock.Any(), eniConfig).Return(nil),
	)

	err := p.configureRegularENI(context.TODO(), netNSPath, eni)
	require.NoError(t, err)
}

func TestIsolatedLinux_ConfigureInterface_BranchENI(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, osWrapper, cniClient, _, _ := newIsolatedLinuxPlatform(ctrl)
	eni := getTestBranchV4ENI()
	eni.DesiredStatus = status.NetworkReadyPull

	cniConfig := createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv(IPAMDataPathEnv, filepath.Join("dummy-db-dir", IPAMDataFileName)),
		cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(nil, nil),
	)

	err := p.configureBranchENI(context.TODO(), netNSPath, eni)
	require.NoError(t, err)
}

func TestIsolatedLinux_ConfigureInterface_BranchENI_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, osWrapper, cniClient, _, _ := newIsolatedLinuxPlatform(ctrl)
	eni := getTestBranchV4ENI()
	eni.DesiredStatus = status.NetworkDeleted

	cniConfig := createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv(IPAMDataPathEnv, filepath.Join("dummy-db-dir", IPAMDataFileName)),
		cniClient.EXPECT().Del(gomock.Any(), cniConfig).Return(nil),
	)

	err := p.configureBranchENI(context.TODO(), netNSPath, eni)
	require.NoError(t, err)
}

func TestIsolatedLinux_ConfigureInterface_VETHDoesNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, _, _, _, _ := newIsolatedLinuxPlatform(ctrl)
	eni := &networkinterface.NetworkInterface{
		InterfaceAssociationProtocol: networkinterface.VETHInterfaceAssociationProtocol,
		DesiredStatus:                status.NetworkReadyPull,
	}

	err := p.ConfigureInterface(context.TODO(), netNSPath, eni, nil)
	require.NoError(t, err)
}

func TestIsolatedLinux_ConfigureInterface_InvalidProtocol(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, _, _, _, _ := newIsolatedLinuxPlatform(ctrl)
	eni := &networkinterface.NetworkInterface{
		InterfaceAssociationProtocol: "bogus",
		DesiredStatus:                status.NetworkReadyPull,
	}

	err := p.ConfigureInterface(context.TODO(), netNSPath, eni, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid interface association protocol")
}

func TestIsolatedLinux_AddGatewayNeighbor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	gwMAC, _ := net.ParseMAC("0a:1b:2c:3d:4e:5f")
	mockLink := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Index: 7, Name: "eth0"}}

	p, _, _, mockNSUtil, mockNetLink := newIsolatedLinuxPlatform(ctrl)
	eni := getTestRegularV4ENI()
	eni.DeviceName = "eth0"

	// resolveHostNeighbor: return gateway MAC
	mockNetLink.EXPECT().NeighList(0, netlink.FAMILY_V4).Return([]netlink.Neigh{
		{IP: net.ParseIP("10.1.0.1"), HardwareAddr: gwMAC},
	}, nil)

	// ExecInNSPath: execute the closure
	mockNSUtil.EXPECT().ExecInNSPath(netNSPath, gomock.Any()).DoAndReturn(
		func(path string, fn func(cnins.NetNS) error) error {
			return fn(nil)
		})

	mockNetLink.EXPECT().LinkByName("eth0").Return(mockLink, nil)
	mockNetLink.EXPECT().NeighSet(gomock.Any()).DoAndReturn(func(neigh *netlink.Neigh) error {
		assert.Equal(t, 7, neigh.LinkIndex)
		assert.Equal(t, netlink.NUD_PERMANENT, neigh.State)
		assert.True(t, neigh.IP.Equal(net.ParseIP("10.1.0.1")))
		assert.Equal(t, gwMAC, neigh.HardwareAddr)
		return nil
	})
	mockNetLink.EXPECT().RouteReplace(gomock.Any()).DoAndReturn(func(route *netlink.Route) error {
		assert.Equal(t, 7, route.LinkIndex)
		assert.Equal(t, "10.1.0.1/32", route.Dst.String())
		assert.Equal(t, netlink.SCOPE_LINK, route.Scope)
		return nil
	})

	err := p.addGatewayNeighbor(netNSPath, eni)
	require.NoError(t, err)
}

func TestIsolatedLinux_AddGatewayNeighbor_NoGateway(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, _, _, _, _ := newIsolatedLinuxPlatform(ctrl)
	eni := getTestRegularV4ENI()
	eni.SubnetGatewayIPV4Address = ""

	err := p.addGatewayNeighbor(netNSPath, eni)
	require.NoError(t, err)
}

func TestIsolatedLinux_AddGatewayNeighbor_HostNeighborNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	p, _, _, _, mockNetLink := newIsolatedLinuxPlatform(ctrl)
	eni := getTestRegularV4ENI()
	eni.DeviceName = "eth0"

	mockNetLink.EXPECT().NeighList(0, netlink.FAMILY_V4).Return([]netlink.Neigh{}, nil)

	err := p.addGatewayNeighbor(netNSPath, eni)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gateway MAC not in host neighbor cache")
}
