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

//go:build windows && unit
// +build windows,unit

package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	mock_ecscni2 "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_nsutil"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const netNSID = "test-netns-id"

// TestConfigureENISuccessWithoutRetry tests ConfigureENI API when the CNI plugin invocation
// succeeds in the first attempt.
func TestConfigureENISuccessWithoutRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	mockCNIClient := mock_ecscni2.NewMockCNI(ctrl)
	os := mock_oswrapper.NewMockOS(ctrl)

	iface := getTestInterface()
	testC := &containerd{
		common{
			cniClient: mockCNIClient,
			os:        os,
		},
	}

	gomock.InOrder(
		os.EXPECT().Setenv(VPCCNIPluginLogFileEnv, `C:\ProgramData\Amazon\Fargate\log\cni\vpc-eni.log`).Times(1),
		// First Add invocation is to move task ENI into the task namespace.
		mockCNIClient.EXPECT().Add(gomock.Any(), gomock.Any()).Do(
			func(_ context.Context, config ecscni.PluginConfig) {
				pluginConfig, ok := config.(*ecscni.VPCENIConfig)
				assert.True(t, ok, "first plugin should be vpc-eni for task ENI setup")

				assert.Equal(t, netNSID, config.NSPath())
				assert.Equal(t, VPCENIPluginName, config.PluginName())
				assert.Equal(t, taskNetworkNamePrefix, config.NetworkName())
				assert.Len(t, pluginConfig.DNS.Nameservers, 2)
				assert.Equal(t, nameServer, pluginConfig.DNS.Nameservers[0])
				assert.Equal(t, deviceName, pluginConfig.ENIName)
				assert.Equal(t, eniMAC, pluginConfig.ENIMACAddress)
				assert.Len(t, pluginConfig.ENIIPAddresses, 1)
				assert.Equal(t, ipv4Addr, strings.Split(pluginConfig.ENIIPAddresses[0], "/")[0])
				assert.Len(t, pluginConfig.GatewayIPAddresses, 1)
				assert.Equal(t, strings.Split(subnetGatewayCIDR, "/")[0], pluginConfig.GatewayIPAddresses[0])
				assert.True(t, pluginConfig.BlockIMDS)
				assert.False(t, pluginConfig.UseExistingNetwork)
			}).Return(nil, nil),
		// Second Add invocation would be for setting up fargate-bridge endpoint in task namespace.
		mockCNIClient.EXPECT().Add(gomock.Any(), gomock.Any()).Do(
			func(_ context.Context, config ecscni.PluginConfig) {
				pluginConfig, ok := config.(*ecscni.VPCENIConfig)
				assert.True(t, ok, "second plugin should be vpc-eni for task iam role setup")

				assert.Equal(t, netNSID, config.NSPath())
				assert.Equal(t, VPCENIPluginName, config.PluginName())
				assert.Equal(t, fargateBridgeNetworkName, config.NetworkName())
				assert.Len(t, pluginConfig.ENIIPAddresses, 1)
				assert.Equal(t, windowsProxyIPAddress, pluginConfig.ENIIPAddresses[0])
				assert.True(t, pluginConfig.BlockIMDS)
				assert.True(t, pluginConfig.UseExistingNetwork)
			}).Return(nil, nil),
	)

	err := testC.ConfigureInterface(ctx, netNSID, iface, nil)
	assert.NoError(t, err)
}

// TestConfigureInterfaceSuccessWithRetry tests ConfigureENI API when the CNI plugin invocation fails initially.
// The networking for the task is setup after retry.
func TestConfigureInterfaceSuccessWithRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	cniClient := mock_ecscni2.NewMockCNI(ctrl)
	os := mock_oswrapper.NewMockOS(ctrl)

	iface := getTestInterface()
	testC := &containerd{
		common{
			cniClient: cniClient,
			os:        os,
		},
	}

	gomock.InOrder(
		os.EXPECT().Setenv(VPCCNIPluginLogFileEnv, `C:\ProgramData\Amazon\Fargate\log\cni\vpc-eni.log`).Times(1),
		// First invocation of CNI plugin fails.
		cniClient.EXPECT().Add(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("failed")),
		// During the retry the CNI plugin calls succeed.
		cniClient.EXPECT().Add(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2),
	)

	err := testC.ConfigureInterface(ctx, "test-netNSID", iface, nil)
	assert.NoError(t, err)
}

// TestContainerd_CreateNetNS verifies the right set of functions are executed while
// creating the task network namespace.
func TestContainerd_CreateNetNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
	testC := &containerd{
		common{
			nsUtil: nsUtil,
		},
	}

	// Positive case.
	gomock.InOrder(
		nsUtil.EXPECT().NSExists(netNSID).Return(false, nil).Times(1),
		nsUtil.EXPECT().NewNetNS(netNSID).Return(nil).Times(1),
	)
	err := testC.CreateNetNS(netNSID)
	require.NoError(t, err)

	// NetNS exists case.
	gomock.InOrder(
		nsUtil.EXPECT().NSExists(netNSID).Return(true, nil).Times(1),
	)
	err = testC.CreateNetNS(netNSID)
	require.NoError(t, err)

	// Negative case.
	err = errors.New("invalid operation")
	gomock.InOrder(
		nsUtil.EXPECT().NSExists(netNSID).Return(false, err).Times(1),
	)
	err = testC.CreateNetNS(netNSID)
	require.Error(t, err)
}

// TestContainerd_DeleteNetNS verifies the right set of functions are executed while
// creating the task network namespace.
func TestContainerd_DeleteNetNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := mock_ecscni.NewMockNetNSUtil(ctrl)
	testC := &containerd{
		common{
			nsUtil: nsUtil,
		},
	}

	// Positive case.
	gomock.InOrder(
		nsUtil.EXPECT().NSExists(netNSID).Return(false, nil).Times(1),
		nsUtil.EXPECT().DelNetNS(netNSID).Return(nil).Times(1),
	)
	err := testC.DeleteNetNS(netNSID)
	require.NoError(t, err)

	// NetNS exists case.
	gomock.InOrder(
		nsUtil.EXPECT().NSExists(netNSID).Return(true, nil).Times(1),
	)
	err = testC.DeleteNetNS(netNSID)
	require.NoError(t, err)

	// Negative case.
	err = errors.New("invalid operation")
	gomock.InOrder(
		nsUtil.EXPECT().NSExists(netNSID).Return(false, err).Times(1),
	)
	err = testC.DeleteNetNS(netNSID)
	require.Error(t, err)
}
