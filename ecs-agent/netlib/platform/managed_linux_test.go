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
	"fmt"
	"io/fs"
	"testing"

	mock_ecscni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_nsutil"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper/mocks"
	mock_volume "github.com/aws/amazon-ecs-agent/ecs-agent/volume/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestManagedLinux_CreateDNSConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	netNSName := "netns-name"
	netNSPath := "/etc/netns/" + netNSName
	iface := getTestInterface()

	netns := &tasknetworkconfig.NetworkNamespace{
		Name:              netNSName,
		Path:              netNSPath,
		NetworkInterfaces: []*networkinterface.NetworkInterface{iface},
	}
	ioutil := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	nsUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
	osWrapper := mock_oswrapper.NewMockOS(ctrl)
	//mockFile := mock_oswrapper.NewMockFile(ctrl)
	volumeAccessor := mock_volume.NewMockTaskVolumeAccessor(ctrl)
	commonPlatform := &common{
		ioutil:            ioutil,
		nsUtil:            nsUtil,
		os:                osWrapper,
		dnsVolumeAccessor: volumeAccessor,
	}

	managedPlatform := &managedLinux{
		common: *commonPlatform,
	}

	resolvData := fmt.Sprintf("nameserver %s\nsearch %s\n",
		"8.8.8.8",
		"us-west-2.compute.internal",
	)
	//hostnameData := fmt.Sprintf("%s\n", iface.GetHostname())

	// Test creation of hosts file.
	/*hostsData := fmt.Sprintf("%s\n%s %s\n%s %s\n%s %s\n",
		HostsLocalhostEntry,
		ipv4Addr, dnsName,
		addr, hostName,
		addr2, hostName2,
	)*/

	taskID := "taskID"

	// Creation of resolv.conf file.
	nsUtil.EXPECT().BuildResolvConfig([]string{"8.8.8.8"}, []string{"us-west-2.compute.internal"}).Return(resolvData).Times(1)
	ioutil.EXPECT().WriteFile(netNSPath+"/resolv.conf", []byte(resolvData), fs.FileMode(0644))
	ioutil.EXPECT().WriteFile(netNSPath+"/hosts", gomock.Any(), gomock.Any())
	ioutil.EXPECT().ReadFile("/etc/hosts")

	// CopyToVolume created files into task volume.
	volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/hosts", "hosts", fs.FileMode(0644)).Return(nil).Times(1)
	volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/resolv.conf", "resolv.conf", fs.FileMode(0644)).Return(nil).Times(1)
	volumeAccessor.EXPECT().CopyToVolume(taskID, netNSPath+"/hostname", "hostname", fs.FileMode(0644)).Return(nil).Times(1)

	err := managedPlatform.CreateDNSConfig(taskID, netns)
	require.NoError(t, err)

}
