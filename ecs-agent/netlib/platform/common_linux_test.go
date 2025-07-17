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

	mock_data "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data/mocks"
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

	currentCNITypes "github.com/containernetworking/cni/pkg/types/100"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

func TestNewPlatform(t *testing.T) {
	_, err := NewPlatform(Config{Name: WarmpoolPlatform}, nil, "", nil)
	assert.NoError(t, err)

	_, err = NewPlatform(Config{Name: "invalid-platform"}, nil, "", nil)
	assert.Error(t, err)
}

// TestCommon_CreateNetNS verifies the precise set of operations are executed
// in order to create a network namespace on the host.
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

// TestCommon_CreateDNSFiles creates a dummy interface which has IP and DNS
// configurations and verifies the precise list of operations are executed
// in order to configure DNS of the network namespace.
func TestCommon_CreateDNSFiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, cs := range []struct {
		iface             *networkinterface.NetworkInterface
		expectedHostsData string
	}{
		{
			iface:             getTestIPv4OnlyInterface(),
			expectedHostsData: getIPv4HostsData(),
		},
		{
			iface:             getTestIPv6OnlyInterface(),
			expectedHostsData: getIPv6HostsData(),
		},
		{
			iface:             getTestDualStackInterface(),
			expectedHostsData: getIPv4HostsData(),
		},
	} {
		netNSName := "netns-name"
		netNSPath := "/etc/netns/" + netNSName
		netns := &tasknetworkconfig.NetworkNamespace{
			Name:              netNSName,
			Path:              netNSPath,
			NetworkInterfaces: []*networkinterface.NetworkInterface{cs.iface},
		}

		ioutil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
		nsUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
		osWrapper := mock_oswrapper.NewMockOS(ctrl)
		mockFile := mock_oswrapper.NewMockFile(ctrl)
		volumeAccessor := mock_volume.NewMockTaskVolumeAccessor(ctrl)
		commonPlatform := &common{
			ioutil:            ioutil,
			nsUtil:            nsUtil,
			os:                osWrapper,
			dnsVolumeAccessor: volumeAccessor,
		}

		resolvData := fmt.Sprintf("nameserver %s\nnameserver %s\nsearch %s\n",
			nameServer,
			nameServer2,
			searchDomainName+" "+searchDomainName2,
		)
		hostnameData := fmt.Sprintf("%s\n", cs.iface.GetHostname())

		taskID := "taskID"
		gomock.InOrder(
			// Creation of netns path.
			osWrapper.EXPECT().Stat(netNSPath).Return(nil, os.ErrNotExist).Times(1),
			osWrapper.EXPECT().IsNotExist(os.ErrNotExist).Return(true).Times(1),
			osWrapper.EXPECT().MkdirAll(netNSPath, fs.FileMode(0644)),

			// Creation of resolv.conf file.
			nsUtil.EXPECT().BuildResolvConfig(cs.iface.DomainNameServers, cs.iface.DomainNameSearchList).Return(resolvData).Times(1),
			ioutil.EXPECT().WriteFile(netNSPath+"/resolv.conf", []byte(resolvData), fs.FileMode(0644)),

			// Creation of hostname file.
			ioutil.EXPECT().WriteFile(netNSPath+"/hostname", []byte(hostnameData), fs.FileMode(0644)),
			osWrapper.EXPECT().OpenFile("/etc/hostname", os.O_RDONLY|os.O_CREATE, fs.FileMode(0644)).Return(mockFile, nil).Times(1),

			// Creation of hosts file.
			mockFile.EXPECT().Close().Times(1),
			ioutil.EXPECT().WriteFile(netNSPath+"/hosts", []byte(cs.expectedHostsData), fs.FileMode(0644)),

			// CopyToVolume created files into task volume.
			volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/hosts", "hosts", fs.FileMode(0644)).Return(nil).Times(1),
			volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/resolv.conf", "resolv.conf", fs.FileMode(0644)).Return(nil).Times(1),
			volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/hostname", "hostname", fs.FileMode(0644)).Return(nil).Times(1),
		)
		err := commonPlatform.createDNSConfig(taskID, false, netns)
		require.NoError(t, err)
	}
}

func getIPv4HostsData() string {
	return fmt.Sprintf("%s\n%s %s\n%s %s\n%s %s\n",
		HostsLocalhostEntryIPv4,
		ipv4Addr, dnsName,
		addr, hostName,
		addr2, hostName2,
	)
}

func getIPv6HostsData() string {
	return fmt.Sprintf("%s\n%s %s\n%s %s\n%s %s\n",
		HostsLocalhostEntryIPv6,
		ipv6Addr, dnsName,
		addr, hostName,
		addr2, hostName2,
	)
}

func TestCommon_CreateDNSFilesForDebug(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, testCase := range []struct {
		name           string
		resolvConfPath string
		iface          *networkinterface.NetworkInterface
	}{
		{
			name:           "al-ipv4",
			resolvConfPath: "/etc",
			iface:          getTestIPv4OnlyInterface(),
		},
		{
			name:           "al-ipv4-withoutdns",
			resolvConfPath: "/etc",
			iface:          getTestIPv4OnlyInterfaceWithoutDNS(),
		},
		{
			name:           "br-ipv4",
			resolvConfPath: "/run/netdog",
			iface:          getTestIPv4OnlyInterface(),
		},
		{
			name:           "br-ipv4-withoutdns",
			resolvConfPath: "/run/netdog",
			iface:          getTestIPv4OnlyInterfaceWithoutDNS(),
		},
		{
			name:           "al-ipv6",
			resolvConfPath: "/etc",
			iface:          getTestIPv6OnlyInterface(),
		},
		{
			name:           "al-ipv6-withoutdns",
			resolvConfPath: "/etc",
			iface:          getTestIPv6OnlyInterfaceWithoutDNS(),
		},
		{
			name:           "br-ipv6",
			resolvConfPath: "/run/netdog",
			iface:          getTestIPv6OnlyInterface(),
		},
		{
			name:           "br-ipv6-withoutdns",
			resolvConfPath: "/run/netdog",
			iface:          getTestIPv6OnlyInterfaceWithoutDNS(),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			netNSName := "netns-name"
			netNSPath := "/etc/netns/" + netNSName
			netns := &tasknetworkconfig.NetworkNamespace{
				Name:              netNSName,
				Path:              netNSPath,
				NetworkInterfaces: []*networkinterface.NetworkInterface{testCase.iface},
			}

			ioutil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
			nsUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
			osWrapper := mock_oswrapper.NewMockOS(ctrl)
			mockFile := mock_oswrapper.NewMockFile(ctrl)
			volumeAccessor := mock_volume.NewMockTaskVolumeAccessor(ctrl)
			commonPlatform := &common{
				ioutil:            ioutil,
				nsUtil:            nsUtil,
				os:                osWrapper,
				dnsVolumeAccessor: volumeAccessor,
				resolvConfPath:    testCase.resolvConfPath,
			}

			hostnameData := fmt.Sprintf("%s\n", testCase.iface.GetHostname())

			taskID := "taskID"
			gomock.InOrder(
				// Read hostname file.
				osWrapper.EXPECT().OpenFile("/etc/hostname", os.O_RDONLY|os.O_CREATE, fs.FileMode(0644)).Return(mockFile, nil).Times(1),
				mockFile.EXPECT().Close().Times(1),

				// Creation of netns path.
				osWrapper.EXPECT().Stat(netNSPath).Return(nil, os.ErrNotExist).Times(1),
				osWrapper.EXPECT().IsNotExist(os.ErrNotExist).Return(true).Times(1),
				osWrapper.EXPECT().MkdirAll(netNSPath, fs.FileMode(0644)),

				// Write hostname file.
				ioutil.EXPECT().WriteFile(netNSPath+"/hostname", []byte(hostnameData), fs.FileMode(0644)),
			)

			// Copy resolv.conf file if no DNS resolver information in the payload, otherwise create it manually.
			if len(testCase.iface.DomainNameServers) == 0 {
				gomock.InOrder(
					ioutil.EXPECT().ReadFile(testCase.resolvConfPath+"/resolv.conf"),
					ioutil.EXPECT().WriteFile(netNSPath+"/resolv.conf", gomock.Any(), gomock.Any()),
				)
			} else {
				gomock.InOrder(
					nsUtil.EXPECT().BuildResolvConfig(testCase.iface.DomainNameServers, testCase.iface.DomainNameSearchList),
					ioutil.EXPECT().WriteFile(netNSPath+"/resolv.conf", gomock.Any(), gomock.Any()),
				)
			}

			gomock.InOrder(
				// Creation of hosts file.
				ioutil.EXPECT().ReadFile("/etc/hosts"),
				ioutil.EXPECT().WriteFile(netNSPath+"/hosts", gomock.Any(), gomock.Any()),

				// CopyToVolume created files into task volume.
				volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/hosts", "hosts", fs.FileMode(0644)).Return(nil).Times(1),
				volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/resolv.conf", "resolv.conf", fs.FileMode(0644)).Return(nil).Times(1),
				volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/hostname", "hostname", fs.FileMode(0644)).Return(nil).Times(1),
			)
			err := commonPlatform.createDNSConfig(taskID, true, netns)
			require.NoError(t, err)
		})
	}

}

func TestCommon_ConfigureInterface(t *testing.T) {
	t.Run("regular-eni", testRegularENIConfiguration)
	t.Run("branch-eni", testBranchENIConfiguration)
	t.Run("geneve-interface", testGeneveInterfaceConfiguration)
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

// testRegularENIConfiguration verifies the precise list of operations are invoked
// with the correct arguments while configuring a regular ENI on a host.
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

	eni := getTestRegularV4ENI()

	// When the ENI is the primary ENI.
	eniConfig := createENIPluginConfigs(netNSPath, eni)
	bridgeConfig := createBridgePluginConfig(netNSPath)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Add(gomock.Any(), bridgeConfig).Return(nil, nil).Times(1),
		cniClient.EXPECT().Add(gomock.Any(), eniConfig).Return(nil, nil).Times(1),
	)
	err := commonPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)

	// Non-primary ENI case.
	eni.Default = false
	eniConfig = createENIPluginConfigs(netNSPath, eni)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Add(gomock.Any(), eniConfig).Return(nil, nil).Times(1),
	)
	err = commonPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)

	// Delete workflow.
	eni.Default = true
	eni.DesiredStatus = status.NetworkDeleted
	eniConfig = createENIPluginConfigs(netNSPath, eni)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("ECS_CNI_LOG_FILE", ecscni.PluginLogPath).Times(1),
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db")),
	)
	err = commonPlatform.configureInterface(ctx, netNSPath, eni, nil)
	require.NoError(t, err)
}

func testBranchENIConfiguration(t *testing.T) {
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

	branchENI := getTestBranchV4ENI()
	branchENI.DesiredStatus = status.NetworkReadyPull
	bridgeConfig := createBridgePluginConfig(netNSPath)
	cniConfig := createBranchENIConfig(netNSPath, branchENI, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault)
	gomock.InOrder(
		osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db")),
		cniClient.EXPECT().Add(gomock.Any(), bridgeConfig).Return(nil, nil).Times(1),
		cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(nil, nil).Times(1),
	)
	err := commonPlatform.configureInterface(ctx, netNSPath, branchENI, nil)
	require.NoError(t, err)

	// Ready-Pull to Ready transition
	branchENI.DesiredStatus = status.NetworkReady
	osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db"))
	err = commonPlatform.configureInterface(ctx, netNSPath, branchENI, nil)
	require.NoError(t, err)

	// Delete workflow.
	branchENI.DesiredStatus = status.NetworkDeleted
	osWrapper.EXPECT().Setenv("IPAM_DB_PATH", filepath.Join(commonPlatform.stateDBDir, "eni-ipam.db"))
	cniClient.EXPECT().Del(gomock.Any(), cniConfig).Return(nil).Times(1)
	err = commonPlatform.configureInterface(ctx, netNSPath, branchENI, nil)
	require.NoError(t, err)
}

func testGeneveInterfaceConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	cniClient := mock_ecscni2.NewMockCNI(ctrl)
	netDAO := mock_data.NewMockNetworkDataClient(ctrl)
	commonPlatform := &common{
		cniClient: cniClient,
	}

	v2nIface := getTestV2NInterface()

	cniResult := &currentCNITypes.Result{
		Interfaces: []*currentCNITypes.Interface{
			{
				Mac: geneveMac,
			},
		},
	}

	cniConfig := NewTunnelConfig(netNSPath, v2nIface, VPCTunnelInterfaceTypeGeneve)
	netDAO.EXPECT().AssignGeneveDstPort(v2nIface.TunnelProperties.ID).
		Return(uint16(v2nIface.TunnelProperties.DestinationPort), nil).Times(1)
	cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(cniResult, nil).Times(1)
	err := commonPlatform.configureInterface(ctx, netNSPath, v2nIface, netDAO)
	require.NoError(t, err)

	v2nIface.DesiredStatus = status.NetworkReady
	cniConfig = NewTunnelConfig(netNSPath, v2nIface, VPCTunnelInterfaceTypeTap)
	cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(nil, nil).Times(1)
	err = commonPlatform.configureInterface(ctx, netNSPath, v2nIface, netDAO)
	require.NoError(t, err)

	v2nIface.DesiredStatus = status.NetworkDeleted
	cniConfig = NewTunnelConfig(netNSPath, v2nIface, VPCTunnelInterfaceTypeTap)
	cniClient.EXPECT().Del(gomock.Any(), cniConfig).Return(nil).Times(1)
	netDAO.EXPECT().ReleaseGeneveDstPort(v2nIface.TunnelProperties.DestinationPort, v2nIface.TunnelProperties.ID).
		Return(nil).Times(1)
	err = commonPlatform.configureInterface(ctx, netNSPath, v2nIface, netDAO)
	require.NoError(t, err)
}
