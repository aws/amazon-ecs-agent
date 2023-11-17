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
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	mock_ecscni2 "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_nsutil"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper/mocks"
	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"
	mock_netwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper/mocks"
	mock_volume "github.com/aws/amazon-ecs-agent/ecs-agent/volume/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

const (
	hostName          = "db.app.com"
	hostName2         = "be.app.com"
	addr              = "169.254.2.3"
	addr2             = "169.254.2.4"
	dnsName           = "amazon.com"
	nameServer        = "10.1.0.2"
	nameServer2       = "10.2.0.2"
	ipv4Addr          = "10.1.0.196"
	ipv4Addr2         = "10.2.0.196"
	searchDomainName  = "us-west-2.test.compute.internal"
	searchDomainName2 = "us-west-2.test2.compute.internal"
	trunkENIMac       = "f0:5c:89:a3:ab:03"
)

func TestNewPlatform(t *testing.T) {
	_, err := NewPlatform(WarmpoolPlatform, nil, "", nil)
	assert.NoError(t, err)

	_, err = NewPlatform("invalid-platform", nil, "", nil)
	assert.Error(t, err)
}

func TestCommon_CreateNetNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	netNSPath := "test-netns-path"

	nsUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
	netLink := mock_netlinkwrapper.NewMockNetLink(ctrl)
	commonPlatform := &common{
		nsUtil:  nsUtil,
		netlink: netLink,
	}
	var dummyLink netlink.Link

	// Happy path.
	gomock.InOrder(
		nsUtil.EXPECT().NSExists(netNSPath).Return(false, nil).Times(1),
		nsUtil.EXPECT().NewNetNS(netNSPath).Return(nil).Times(1),
		nsUtil.EXPECT().ExecInNSPath(netNSPath, gomock.Any()).Return(nil).Times(1),
		netLink.EXPECT().LinkByName("lo").Return(dummyLink, nil).Times(1),
		netLink.EXPECT().LinkSetUp(dummyLink).Return(nil).Times(1),
	)
	commonPlatform.CreateNetNS(netNSPath)
	commonPlatform.setUpLoFunc(netNSPath)(nil)

	// Negative cases.
	nsUtil.EXPECT().NSExists(netNSPath).Return(true, nil).Times(1)
	err := commonPlatform.CreateNetNS(netNSPath)
	require.NoError(t, err)

	nsUtil.EXPECT().NSExists(netNSPath).Return(false, nil).Times(1)
	nsUtil.EXPECT().NewNetNS(netNSPath).Return(errors.New("errrr")).Times(1)
	err = commonPlatform.CreateNetNS(netNSPath)
	require.Error(t, err)

	nsUtil.EXPECT().NSExists(netNSPath).Return(false, nil).Times(1)
	nsUtil.EXPECT().NewNetNS(netNSPath).Return(nil).Times(1)
	nsUtil.EXPECT().ExecInNSPath(netNSPath, gomock.Any()).
		Return(errors.New("errrr")).Times(1)
	err = commonPlatform.CreateNetNS(netNSPath)
	require.Error(t, err)
}

func TestCommon_CreateDNSFiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	netNSName := "netns-name"
	netNSPath := "/etc/netns/" + netNSName
	iface := &networkinterface.NetworkInterface{
		PrivateDNSName:    dnsName,
		DomainNameServers: []string{nameServer, nameServer2},
		Default:           true,
		IPV4Addresses: []*networkinterface.IPV4Address{
			{
				Address: ipv4Addr,
				Primary: true,
			},
			{
				Address: ipv4Addr2,
				Primary: false,
			},
		},
		DNSMappingList: []networkinterface.DNSMapping{
			{
				Hostname: hostName,
				Address:  addr,
			},
			{
				Hostname: hostName2,
				Address:  addr2,
			},
		},
		DomainNameSearchList: []string{searchDomainName, searchDomainName2},
	}

	netns := &tasknetworkconfig.NetworkNamespace{
		Name:              netNSName,
		Path:              netNSPath,
		NetworkInterfaces: []*networkinterface.NetworkInterface{iface},
	}

	ioutil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	nsUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
	osWrapper := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	volumeAccessor := mock_volume.NewMockVolumeAccessor(ctrl)
	commonPlatform := &common{
		ioutil:             ioutil,
		nsUtil:             nsUtil,
		os:                 osWrapper,
		taskVolumeAccessor: volumeAccessor,
	}

	// Test creation of hosts file.
	hostsData := fmt.Sprintf("%s\n%s %s\n%s %s\n%s %s\n",
		HostsLocalhostEntry,
		ipv4Addr, dnsName,
		addr, hostName,
		addr2, hostName2,
	)
	resolvData := fmt.Sprintf("nameserver %s\nnameserver %s\nsearch %s\n",
		nameServer,
		nameServer2,
		searchDomainName+" "+searchDomainName2,
	)
	hostnameData := fmt.Sprintf("%s\n", iface.GetHostname())

	gomock.InOrder(
		// Creation of netns path.
		osWrapper.EXPECT().Stat(netNSPath).Return(nil, os.ErrNotExist).Times(1),
		osWrapper.EXPECT().IsNotExist(os.ErrNotExist).Return(true).Times(1),
		osWrapper.EXPECT().MkdirAll(netNSPath, fs.FileMode(0644)),

		// Creation of resolv.conf file.
		nsUtil.EXPECT().BuildResolvConfig(iface.DomainNameServers, iface.DomainNameSearchList).Return(resolvData).Times(1),
		ioutil.EXPECT().WriteFile(netNSPath+"/resolv.conf", []byte(resolvData), fs.FileMode(0644)),

		// Creation of hostname file.
		ioutil.EXPECT().WriteFile(netNSPath+"/hostname", []byte(hostnameData), fs.FileMode(0644)),
		osWrapper.EXPECT().OpenFile("/etc/hostname", os.O_RDONLY|os.O_CREATE, fs.FileMode(0644)).Return(mockFile, nil).Times(1),

		// Creation of hosts file.
		mockFile.EXPECT().Close().Times(1),
		ioutil.EXPECT().WriteFile(netNSPath+"/hosts", []byte(hostsData), fs.FileMode(0644)),

		// CopyToVolume created files into task volume.
		volumeAccessor.EXPECT().CopyToVolume(netNSPath+"/hosts", "hosts", fs.FileMode(0644)).Return(nil).Times(1),
		volumeAccessor.EXPECT().CopyToVolume(netNSPath+"/resolv.conf", "resolv.conf", fs.FileMode(0644)).Return(nil).Times(1),
		volumeAccessor.EXPECT().CopyToVolume(netNSPath+"/hostname", "hostname", fs.FileMode(0644)).Return(nil).Times(1),
	)
	err := commonPlatform.createDNSConfig("taskID", false, netns)
	require.NoError(t, err)
}

func TestCommon_ConfigureInterface(t *testing.T) {
	t.Run("configure-regular-eni", testRegularENIConfiguration)
	t.Run("configure-branch-eni", testBranchENIConfiguration)
}

// TestInterfacesMACToName verifies interfacesMACToName behaves as expected.
func TestInterfacesMACToName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNet := mock_netwrapper.NewMockNet(ctrl)
	commonPlatform := &common{
		net: mockNet,
	}

	// Prepare test data.
	testMac, err := net.ParseMAC(trunkENIMac)
	require.NoError(t, err)
	testIface := []net.Interface{
		{
			HardwareAddr: testMac,
			Name:         "eth1",
		},
	}
	expected := map[string]string{
		trunkENIMac: "eth1",
	}

	// Positive case.
	mockNet.EXPECT().Interfaces().Return(testIface, nil).Times(1)
	actual, err := commonPlatform.interfacesMACToName()
	require.NoError(t, err)
	require.Equal(t, expected, actual)

	// Negative case.
	testErr := errors.New("no interfaces to chat with")
	mockNet.EXPECT().Interfaces().Return(testIface, testErr).Times(1)
	_, err = commonPlatform.interfacesMACToName()
	require.Error(t, err)
	require.Equal(t, testErr, err)
}

func testRegularENIConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	osWrapper := mock_oswrapper.NewMockOS(ctrl)
	cniClient := mock_ecscni2.NewMockCNI(ctrl)

	commonPlatform := &common{
		os:         osWrapper,
		cniClient:  cniClient,
		stateDBDir: "dummy-db-dir",
	}

	eni := getTestRegularENI()

	eniConfig := createENIPluginConfigs(netNSPath, eni)
	bridgeConfig := createBridgePluginConfig(netNSPath)

	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Add(gomock.Any(), bridgeConfig).Return(nil, nil).Times(1),
		cniClient.EXPECT().Add(gomock.Any(), eniConfig).Return(nil, nil).Times(1),
	)
	err := commonPlatform.configureInterface(ctx, netNSPath, eni)
	require.NoError(t, err)

	// Non-primary ENI case.
	eni.Default = false
	eniConfig = createENIPluginConfigs(netNSPath, eni)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Add(gomock.Any(), eniConfig).Return(nil, nil).Times(1),
	)
	err = commonPlatform.configureInterface(ctx, netNSPath, eni)
	require.NoError(t, err)

	// Delete workflow.
	eni.Default = true
	eni.DesiredStatus = status.NetworkDeleted
	eniConfig = createENIPluginConfigs(netNSPath, eni)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db")),
	)
	err = commonPlatform.configureInterface(ctx, netNSPath, eni)
	require.NoError(t, err)
}

func testBranchENIConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	cniClient := mock_ecscni2.NewMockCNI(ctrl)
	commonPlatform := &common{
		cniClient: cniClient,
	}

	branchENI := getTestBranchENI()

	cniConfig := createBranchENIConfig(netNSPath, branchENI, VPCBranchENIInterfaceTypeVlan)
	cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(nil, nil).Times(1)
	err := commonPlatform.configureInterface(ctx, netNSPath, branchENI)
	require.NoError(t, err)

	branchENI.DesiredStatus = status.NetworkReady
	cniConfig = createBranchENIConfig(netNSPath, branchENI, VPCBranchENIInterfaceTypeTap)
	cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(nil, nil).Times(1)
	err = commonPlatform.configureInterface(ctx, netNSPath, branchENI)
	require.NoError(t, err)

	// Delete workflow.
	branchENI.DesiredStatus = status.NetworkDeleted
	cniConfig = createBranchENIConfig(netNSPath, branchENI, VPCBranchENIInterfaceTypeTap)
	cniClient.EXPECT().Del(gomock.Any(), cniConfig).Return(nil).Times(1)
	err = commonPlatform.configureInterface(ctx, netNSPath, branchENI)
	require.NoError(t, err)
}
