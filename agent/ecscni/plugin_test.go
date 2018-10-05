// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ecscni/mocks_libcni"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestSetupNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSENIPluginName, net.Network.Type, "second plugin should be eni")
			}),
		// Bridge plugin was called last
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "first plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
				assert.Len(t, bridgeConfig.IPAM.IPV4Routes, 3, "default route plus two extra routes")
			}),
	)

	_, err = ecscniClient.SetupNS(context.TODO(), &Config{AdditionalLocalRoutes: additionalRoutes}, time.Second)
	assert.NoError(t, err)
}

func TestSetupNSTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var wg sync.WaitGroup
	wg.Add(1)

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				wg.Wait()
			}).MaxTimes(1),
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).MaxTimes(1),
	)

	_, err := ecscniClient.SetupNS(context.TODO(), &Config{}, time.Millisecond)
	assert.Error(t, err)
	wg.Done()
}

func TestCleanupNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both bridge and eni plugin
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	err = ecscniClient.CleanupNS(context.TODO(), &Config{AdditionalLocalRoutes: additionalRoutes}, time.Second)
	assert.NoError(t, err)
}

func TestCleanupNSTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var wg sync.WaitGroup
	wg.Add(1)

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both bridge and eni plugin
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any()).Do(
		func(x interface{}, y interface{}) {
			wg.Wait()
		}).Return(nil).MaxTimes(2)

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.TODO(), 1*time.Millisecond)
	defer cancel()
	err = ecscniClient.CleanupNS(ctx, &Config{AdditionalLocalRoutes: additionalRoutes}, time.Millisecond)
	assert.Error(t, err)
	wg.Done()
}

func TestReleaseIPInIPAM(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any()).Return(nil)

	err := ecscniClient.ReleaseIPResource(&Config{})
	assert.NoError(t, err)
}

// TestConstructENINetworkConfig tests createENINetworkConfig creates the correct
// configuration for eni plugin
func TestConstructENINetworkConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	config := &Config{
		ENIID:                    "eni-12345678",
		ContainerID:              "containerid12",
		ContainerPID:             "pid",
		ENIIPV4Address:           "172.31.21.40",
		ENIIPV6Address:           "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		ENIMACAddress:            "02:7b:64:49:b1:40",
		BlockInstanceMetdata:     true,
		SubnetGatewayIPV4Address: "172.31.1.1/20",
	}

	_, eniNetworkConfig, err := ecscniClient.(*cniClient).createENINetworkConfig(config)
	assert.NoError(t, err, "construct eni network config failed")
	eniConfig := &ENIConfig{}
	err = json.Unmarshal(eniNetworkConfig.Bytes, eniConfig)
	assert.NoError(t, err, "unmarshal config from bytes failed")

	assert.Equal(t, config.ENIID, eniConfig.ENIID)
	assert.Equal(t, config.ENIIPV4Address, eniConfig.IPV4Address)
	assert.Equal(t, config.ENIIPV6Address, eniConfig.IPV6Address)
	assert.Equal(t, config.ENIMACAddress, eniConfig.MACAddress)
	assert.True(t, eniConfig.BlockInstanceMetdata)
	assert.Equal(t, config.SubnetGatewayIPV4Address, eniConfig.SubnetGatewayIPV4Address)
}

// TestConstructBridgeNetworkConfigWithoutIPAM tests createBridgeNetworkConfigWithoutIPAM creates the right configuration for bridge plugin
func TestConstructBridgeNetworkConfigWithoutIPAM(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	config := &Config{
		ContainerID:  "containerid12",
		ContainerPID: "pid",
		BridgeName:   "bridge-test1",
	}

	_, bridgeNetworkConfig, err := ecscniClient.(*cniClient).createBridgeNetworkConfigWithoutIPAM(config)
	assert.NoError(t, err, "construct bridge network config failed")
	bridgeConfig := &BridgeConfig{}
	err = json.Unmarshal(bridgeNetworkConfig.Bytes, bridgeConfig)

	assert.NoError(t, err, "unmarshal bridge config from bytes failed")
	assert.Equal(t, config.BridgeName, bridgeConfig.BridgeName)
	assert.Equal(t, IPAMConfig{}, bridgeConfig.IPAM)
}

// TestConstructBridgeNetworkConfigWithIPAM tests createBridgeNetworkConfigWithIPAM
// creates the correct configuration for bridge and ipam plugin
func TestConstructNetworkConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	config := &Config{
		ContainerID:           "containerid12",
		ContainerPID:          "pid",
		BridgeName:            "bridge-test1",
		AdditionalLocalRoutes: additionalRoutes,
	}

	_, bridgeNetworkConfig, err := ecscniClient.(*cniClient).createBridgeNetworkConfigWithIPAM(config)
	assert.NoError(t, err, "construct bridge plugins configuration failed")

	bridgeConfig := &BridgeConfig{}
	err = json.Unmarshal(bridgeNetworkConfig.Bytes, bridgeConfig)
	assert.NoError(t, err, "unmarshal bridge config from bytes failed: %s", string(bridgeNetworkConfig.Bytes))

	assert.Equal(t, config.BridgeName, bridgeConfig.BridgeName)
	assert.Equal(t, ecsSubnet, bridgeConfig.IPAM.IPV4Subnet)
	assert.Equal(t, TaskIAMRoleEndpoint, bridgeConfig.IPAM.IPV4Routes[0].Dst.String())
}

func TestCNIPluginVersion(t *testing.T) {
	testCases := []struct {
		version *cniPluginVersion
		str     string
	}{
		{
			version: &cniPluginVersion{
				Version: "1",
				Dirty:   false,
				Hash:    "hash",
			},
			str: "hash-1",
		},
		{
			version: &cniPluginVersion{
				Version: "1",
				Dirty:   true,
				Hash:    "hash",
			},
			str: "@hash-1",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("version string %s", tc.str), func(t *testing.T) {
			assert.Equal(t, tc.str, tc.version.str())
		})
	}
}
