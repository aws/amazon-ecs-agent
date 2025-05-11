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
	"io/fs"
	"os"
	"testing"

	mock_ecscni2 "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_nsutil"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper/mocks"
	mock_volume "github.com/aws/amazon-ecs-agent/ecs-agent/volume/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

// TestFirecracker_CreateDNSConfig checks if DNS config files gets created for
// both primary and secondary interfaces.
func TestFirecracker_CreateDNSConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskID := "task-id"
	iface := getTestIPv4OnlyInterface()
	primaryNetNSName := networkinterface.NetNSName(taskID, iface.Name)
	primaryNetNSPath := "/etc/netns/" + primaryNetNSName

	v2nIface := getTestV2NInterface()
	secondaryNetNSName := networkinterface.NetNSName(taskID, v2nIface.Name)
	secondaryNetNSPath := "/etc/netns/" + secondaryNetNSName

	netns := &tasknetworkconfig.NetworkNamespace{
		Name:              primaryNetNSName,
		Path:              primaryNetNSPath,
		NetworkInterfaces: []*networkinterface.NetworkInterface{iface, v2nIface},
	}

	ioutil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	nsUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
	osWrapper := mock_oswrapper.NewMockOS(ctrl)
	mockFile := mock_oswrapper.NewMockFile(ctrl)
	volumeAccessor := mock_volume.NewMockTaskVolumeAccessor(ctrl)
	commonPlatform := common{
		ioutil:            ioutil,
		nsUtil:            nsUtil,
		os:                osWrapper,
		dnsVolumeAccessor: volumeAccessor,
	}

	fc := &firecraker{
		common: commonPlatform,
	}

	// Test creation of hosts file.
	primaryHostsData := fmt.Sprintf("%s\n%s %s\n%s %s\n%s %s\n",
		HostsLocalhostEntryIPv4,
		ipv4Addr, dnsName,
		addr, hostName,
		addr2, hostName2,
	)
	primaryResolvData := fmt.Sprintf("nameserver %s\nnameserver %s\nsearch %s\n",
		nameServer,
		nameServer2,
		searchDomainName+" "+searchDomainName2,
	)
	primaryHostnameData := fmt.Sprintf("%s\n", iface.GetHostname())

	secondaryHostsData := fmt.Sprintf("%s\n%s %s\n",
		HostsLocalhostEntryIPv4,
		networkinterface.DefaultGeneveInterfaceIPAddress, "",
	)
	secondaryResolvData := fmt.Sprintf("nameserver %s\nsearch %s\n",
		nameServer,
		searchDomainName,
	)
	secondaryHostnameData := "\n"

	gomock.InOrder(
		// Creation of netns path.
		osWrapper.EXPECT().Stat(primaryNetNSPath).Return(nil, os.ErrNotExist).Times(1),
		osWrapper.EXPECT().IsNotExist(os.ErrNotExist).Return(true).Times(1),
		osWrapper.EXPECT().MkdirAll(primaryNetNSPath, fs.FileMode(0644)),

		// Creation of resolv.conf file for primary interface.
		nsUtil.EXPECT().BuildResolvConfig(iface.DomainNameServers, iface.DomainNameSearchList).Return(primaryResolvData).Times(1),
		ioutil.EXPECT().WriteFile(primaryNetNSPath+"/resolv.conf", []byte(primaryResolvData), fs.FileMode(0644)),

		// Creation of hostname file for primary interface.
		ioutil.EXPECT().WriteFile(primaryNetNSPath+"/hostname", []byte(primaryHostnameData), fs.FileMode(0644)),
		osWrapper.EXPECT().OpenFile("/etc/hostname", os.O_RDONLY|os.O_CREATE, fs.FileMode(0644)).Return(mockFile, nil).Times(1),

		// Creation of hosts file for primary interface.
		mockFile.EXPECT().Close().Times(1),
		ioutil.EXPECT().WriteFile(primaryNetNSPath+"/hosts", []byte(primaryHostsData), fs.FileMode(0644)),

		// CopyToVolume created files into task volume for primary interface.
		volumeAccessor.EXPECT().CopyToVolume(taskID, primaryNetNSPath+"/hosts", "hosts", fs.FileMode(0644)).Return(nil).Times(1),
		volumeAccessor.EXPECT().CopyToVolume(taskID, primaryNetNSPath+"/resolv.conf", "resolv.conf", fs.FileMode(0644)).Return(nil).Times(1),
		volumeAccessor.EXPECT().CopyToVolume(taskID, primaryNetNSPath+"/hostname", "hostname", fs.FileMode(0644)).Return(nil).Times(1),

		// Creation of secondary netns path.
		osWrapper.EXPECT().Stat(secondaryNetNSPath).Return(nil, os.ErrNotExist).Times(1),
		osWrapper.EXPECT().IsNotExist(os.ErrNotExist).Return(true).Times(1),
		osWrapper.EXPECT().MkdirAll(secondaryNetNSPath, fs.FileMode(0644)),

		// Creation of resolv.conf file for secondary interface.
		nsUtil.EXPECT().BuildResolvConfig(v2nIface.DomainNameServers, v2nIface.DomainNameSearchList).Return(secondaryResolvData).Times(1),
		ioutil.EXPECT().WriteFile(secondaryNetNSPath+"/resolv.conf", []byte(secondaryResolvData), fs.FileMode(0644)),

		// Creation of hostname file for secondary interface.
		ioutil.EXPECT().WriteFile(secondaryNetNSPath+"/hostname", []byte(secondaryHostnameData), fs.FileMode(0644)),
		osWrapper.EXPECT().OpenFile("/etc/hostname", os.O_RDONLY|os.O_CREATE, fs.FileMode(0644)).Return(mockFile, nil).Times(1),

		// Creation of hosts file for secondary interface.
		mockFile.EXPECT().Close().Times(1),
		ioutil.EXPECT().WriteFile(secondaryNetNSPath+"/hosts", []byte(secondaryHostsData), fs.FileMode(0644)),

		// CopyToVolume created files into task volume for secondary interface.
		volumeAccessor.EXPECT().CopyToVolume(taskID, secondaryNetNSPath+"/hosts", "hosts", fs.FileMode(0644)).Return(nil).Times(1),
		volumeAccessor.EXPECT().CopyToVolume(taskID, secondaryNetNSPath+"/resolv.conf", "resolv.conf", fs.FileMode(0644)).Return(nil).Times(1),
		volumeAccessor.EXPECT().CopyToVolume(taskID, secondaryNetNSPath+"/hostname", "hostname", fs.FileMode(0644)).Return(nil).Times(1),
	)
	err := fc.CreateDNSConfig(taskID, netns)
	require.NoError(t, err)
}

func TestFirecracker_BranchENIConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	cniClient := mock_ecscni2.NewMockCNI(ctrl)
	commonPlatform := common{
		cniClient: cniClient,
	}
	fc := &firecraker{
		common: commonPlatform,
	}

	branchENI := getTestBranchV4ENI()

	cniConfig := createBranchENIConfig(netNSPath, branchENI, VPCBranchENIInterfaceTypeVlan, false)
	cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(nil, nil).Times(1)
	err := fc.ConfigureInterface(ctx, netNSPath, branchENI, nil)
	require.NoError(t, err)

	branchENI.DesiredStatus = status.NetworkReady
	cniConfig = createBranchENIConfig(netNSPath, branchENI, VPCBranchENIInterfaceTypeTap, false)
	cniClient.EXPECT().Add(gomock.Any(), cniConfig).Return(nil, nil).Times(1)
	err = fc.ConfigureInterface(ctx, netNSPath, branchENI, nil)
	require.NoError(t, err)

	// Delete workflow.
	branchENI.DesiredStatus = status.NetworkDeleted
	cniConfig = createBranchENIConfig(netNSPath, branchENI, VPCBranchENIInterfaceTypeTap, false)
	cniClient.EXPECT().Del(gomock.Any(), cniConfig).Return(nil).Times(1)
	err = fc.ConfigureInterface(ctx, netNSPath, branchENI, nil)
	require.NoError(t, err)
}
