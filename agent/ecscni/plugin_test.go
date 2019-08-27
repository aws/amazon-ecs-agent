// +build linux,unit

// Copyright 2014-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	"github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_libcni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks_libcni"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	eniID                       = "eni-12345678"
	eniIPV4Address              = "172.31.21.40"
	eniMACAddress               = "02:7b:64:49:b1:40"
	eniSubnetGatewayIPV4Address = "172.31.1.1/20"
	trunkENIMACAddress          = "02:7b:64:49:b2:40"
	branchENIVLANID             = "42"
	branchIPV4Address           = "172.31.21.40/20"
	branchSubnetGatewayAddress  = "172.31.1.1"
)

func TestSetupNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSENIPluginName, net.Network.Type, "first plugin should be eni")
			}),
		// Bridge plugin was called second
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "second plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
				assert.Len(t, bridgeConfig.IPAM.IPV4Routes, 3, "default route plus two extra routes")
			}),
	)

	config := &Config{
		AdditionalLocalRoutes: additionalRoutes,
		NetworkConfigs:        []*NetworkConfig{},
	}
	config.NetworkConfigs = append(config.NetworkConfigs, eniNetworkConfig(config))
	config.NetworkConfigs = append(config.NetworkConfigs, bridgeConfigWithIPAM(config))

	_, err = ecscniClient.SetupNS(context.TODO(), config, time.Second)
	assert.NoError(t, err)
}

func eniNetworkConfig(config *Config) *NetworkConfig {
	_, eniNetworkConfig, _ := NewENINetworkConfig(
		&eni.ENI{
			ID: eniID,
			IPV4Addresses: []*eni.ENIIPV4Address{
				{Address: eniIPV4Address, Primary: true},
			},
			MacAddress:               eniMACAddress,
			SubnetGatewayIPV4Address: eniSubnetGatewayIPV4Address,
		},
		config,
	)
	return &NetworkConfig{CNINetworkConfig: eniNetworkConfig}
}

func bridgeConfigWithIPAM(config *Config) *NetworkConfig {
	_, bridgeNetworkConfig, _ := NewBridgeNetworkConfig(config, true)
	return &NetworkConfig{CNINetworkConfig: bridgeNetworkConfig}
}

func TestSetupNSTrunk(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBranchENIPluginName, net.Network.Type, "first plugin should be eni")
			}),
		// Bridge plugin was called last
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "second plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
				assert.Len(t, bridgeConfig.IPAM.IPV4Routes, 3, "default route plus two extra routes")
			}),
	)

	config := &Config{
		AdditionalLocalRoutes: additionalRoutes,
		NetworkConfigs:        []*NetworkConfig{},
	}
	config.NetworkConfigs = append(config.NetworkConfigs, branchENINetworkConfig(config))
	config.NetworkConfigs = append(config.NetworkConfigs, bridgeConfigWithIPAM(config))
	_, err = ecscniClient.SetupNS(context.TODO(), config, time.Second)
	assert.NoError(t, err)
}

func branchENINetworkConfig(config *Config) *NetworkConfig {
	_, eniNetworkConfig, _ := NewBranchENINetworkConfig(
		&eni.ENI{
			ID: eniID,
			IPV4Addresses: []*eni.ENIIPV4Address{
				{Address: eniIPV4Address, Primary: true},
			},
			MacAddress:               eniMACAddress,
			SubnetGatewayIPV4Address: eniSubnetGatewayIPV4Address,
			InterfaceVlanProperties: &eni.InterfaceVlanProperties{
				TrunkInterfaceMacAddress: trunkENIMACAddress,
				VlanID:                   branchENIVLANID,
			},
		},
		config)
	return &NetworkConfig{CNINetworkConfig: eniNetworkConfig}
}

func TestSetupNSAppMeshEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSENIPluginName, net.Network.Type, "first plugin should be eni")
			}),
		// Bridge plugin was called second
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "second plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
				assert.Len(t, bridgeConfig.IPAM.IPV4Routes, 3, "default route plus two extra routes")
			}),
		// AppMesh plugin was called third
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSAppMeshPluginName, net.Network.Type, "third plugin should be app mesh")
			}),
	)
	config := &Config{
		AdditionalLocalRoutes: additionalRoutes,
		NetworkConfigs:        []*NetworkConfig{},
	}
	config.NetworkConfigs = append(config.NetworkConfigs, eniNetworkConfig(config))
	config.NetworkConfigs = append(config.NetworkConfigs, bridgeConfigWithIPAM(config))
	config.NetworkConfigs = append(config.NetworkConfigs, appMeshNetworkConfig(config))
	_, err = ecscniClient.SetupNS(context.TODO(), config, time.Second)
	assert.NoError(t, err)
}

func appMeshNetworkConfig(config *Config) *NetworkConfig {
	_, appMeshNetworkConfig, _ := NewAppMeshConfig(&appmesh.AppMesh{
		IgnoredUID:       "1337",
		IgnoredGID:       "1448",
		ProxyIngressPort: "15000",
		ProxyEgressPort:  "15001",
		AppPorts: []string{
			"9000",
		},
		EgressIgnoredPorts: []string{
			"9001",
		},
		EgressIgnoredIPs: []string{
			"169.254.169.254",
		},
	}, config)
	return &NetworkConfig{CNINetworkConfig: appMeshNetworkConfig}
}

func TestSetupNSTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, errors.New("timeout")).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
			}).MaxTimes(1),
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).MaxTimes(1),
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).MaxTimes(1),
	)

	config := &Config{
		NetworkConfigs: []*NetworkConfig{},
	}
	config.NetworkConfigs = append(config.NetworkConfigs, eniNetworkConfig(config))
	config.NetworkConfigs = append(config.NetworkConfigs, bridgeConfigWithIPAM(config))
	config.NetworkConfigs = append(config.NetworkConfigs, appMeshNetworkConfig(config))
	_, err := ecscniClient.SetupNS(context.TODO(), config, time.Millisecond)
	assert.Error(t, err)
}

func TestCleanupNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both bridge and eni plugin
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	config := &Config{
		AdditionalLocalRoutes: additionalRoutes,
		NetworkConfigs:        []*NetworkConfig{},
	}
	config.NetworkConfigs = append(config.NetworkConfigs, eniNetworkConfig(config))
	config.NetworkConfigs = append(config.NetworkConfigs, bridgeConfigWithIPAM(config))
	err = ecscniClient.CleanupNS(context.TODO(), config, time.Second)
	assert.NoError(t, err)
}

func TestCleanupNSTrunk(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	gomock.InOrder(
		// Bridge plugin was called first
		libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "first plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
			}),
		// ENI plugin was called second
		libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBranchENIPluginName, net.Network.Type, "second plugin should be eni")
			}),
	)

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	config := &Config{
		AdditionalLocalRoutes: additionalRoutes,
		NetworkConfigs:        []*NetworkConfig{},
	}
	config.NetworkConfigs = append(config.NetworkConfigs, branchENINetworkConfig(config))
	config.NetworkConfigs = append(config.NetworkConfigs, bridgeConfigWithIPAM(config))
	err = ecscniClient.CleanupNS(context.TODO(), config, time.Second)
	assert.NoError(t, err)
}

func TestCleanupNSAppMeshEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both bridge and eni plugin
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(3)

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	config := &Config{
		AdditionalLocalRoutes: additionalRoutes,
		NetworkConfigs:        []*NetworkConfig{},
	}
	config.NetworkConfigs = append(config.NetworkConfigs, eniNetworkConfig(config))
	config.NetworkConfigs = append(config.NetworkConfigs, bridgeConfigWithIPAM(config))
	config.NetworkConfigs = append(config.NetworkConfigs, appMeshNetworkConfig(config))
	err = ecscniClient.CleanupNS(context.TODO(), config, time.Second)
	assert.NoError(t, err)
}

func TestCleanupNSTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both bridge and eni plugin
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(x interface{}, y interface{}, z interface{}) {
		}).Return(errors.New("timeout")).MaxTimes(3)

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.TODO(), 1*time.Millisecond)
	defer cancel()
	config := &Config{
		AdditionalLocalRoutes: additionalRoutes,
		NetworkConfigs:        []*NetworkConfig{},
	}
	config.NetworkConfigs = append(config.NetworkConfigs, eniNetworkConfig(config))
	config.NetworkConfigs = append(config.NetworkConfigs, bridgeConfigWithIPAM(config))
	err = ecscniClient.CleanupNS(ctx, config, time.Millisecond)
	assert.Error(t, err)
}

func TestReleaseIPInIPAM(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := ecscniClient.ReleaseIPResource(context.TODO(), &Config{}, time.Second)
	assert.NoError(t, err)
}

// TestConstructENINetworkConfig tests createENINetworkConfig creates the correct
// configuration for eni plugin
func TestConstructENINetworkConfig(t *testing.T) {
	config := &Config{
		ContainerID:           "containerid12",
		ContainerPID:          "pid",
		BlockInstanceMetadata: true,
	}

	eniName, eniNetworkConfig, err := NewENINetworkConfig(
		&eni.ENI{
			ID: eniID,
			IPV4Addresses: []*eni.ENIIPV4Address{
				{Address: eniIPV4Address, Primary: true},
			},
			MacAddress:               eniMACAddress,
			SubnetGatewayIPV4Address: eniSubnetGatewayIPV4Address,
		},
		config)
	require.NoError(t, err, "Failed to construct eni network config")
	assert.Equal(t, "eth0", eniName)
	eniConfig := &ENIConfig{}
	err = json.Unmarshal(eniNetworkConfig.Bytes, eniConfig)
	require.NoError(t, err, "unmarshal config from bytes failed")
	assert.Equal(t, &ENIConfig{
		Type:                     "ecs-eni",
		ENIID:                    eniID,
		IPV4Address:              eniIPV4Address,
		MACAddress:               eniMACAddress,
		BlockInstanceMetadata:    true,
		SubnetGatewayIPV4Address: eniSubnetGatewayIPV4Address,
	}, eniConfig)
}

// TestConstructBranchENINetworkConfig tests createBranchENINetworkConfig creates the correct
// configuration for eni plugin
func TestConstructBranchENINetworkConfig(t *testing.T) {
	config := &Config{
		ContainerID:           "containerid12",
		ContainerPID:          "pid",
		BlockInstanceMetadata: true,
	}

	eniName, eniNetworkConfig, err := NewBranchENINetworkConfig(
		&eni.ENI{
			ID: eniID,
			IPV4Addresses: []*eni.ENIIPV4Address{
				{Address: eniIPV4Address, Primary: true},
			},
			MacAddress:               eniMACAddress,
			SubnetGatewayIPV4Address: eniSubnetGatewayIPV4Address,
			InterfaceVlanProperties: &eni.InterfaceVlanProperties{
				TrunkInterfaceMacAddress: trunkENIMACAddress,
				VlanID:                   branchENIVLANID,
			},
		},
		config)
	require.NoError(t, err, "Failed to construct eni network config")
	assert.Equal(t, "eth0", eniName)
	branchENIConfig := &BranchENIConfig{}
	err = json.Unmarshal(eniNetworkConfig.Bytes, branchENIConfig)
	require.NoError(t, err, "unmarshal config from bytes failed")
	assert.Equal(t, &BranchENIConfig{
		Type:                   "vpc-branch-eni",
		BranchIPAddress:        branchIPV4Address,
		BranchMACAddress:       eniMACAddress,
		BlockInstanceMetadata:  true,
		BranchGatewayIPAddress: branchSubnetGatewayAddress,
		TrunkMACAddress:        trunkENIMACAddress,
		BranchVlanID:           branchENIVLANID,
		InterfaceType:          "vlan",
	}, branchENIConfig)
}

// TestConstructBridgeNetworkConfigWithoutIPAM tests createBridgeNetworkConfigWithoutIPAM creates the right configuration for bridge plugin
func TestConstructBridgeNetworkConfigWithoutIPAM(t *testing.T) {
	config := &Config{
		ContainerID:  "containerid12",
		ContainerPID: "pid",
		BridgeName:   "bridge-test1",
	}

	vethName, bridgeNetworkConfig, err := NewBridgeNetworkConfig(config, false)
	require.NoError(t, err, "Failed to construct bridge network config")
	assert.Equal(t, "ecs-eth0", vethName)
	bridgeConfig := &BridgeConfig{}
	err = json.Unmarshal(bridgeNetworkConfig.Bytes, bridgeConfig)
	require.NoError(t, err, "unmarshal bridge config from bytes failed")
	assert.Equal(t, config.BridgeName, bridgeConfig.BridgeName)
	assert.Equal(t, IPAMConfig{}, bridgeConfig.IPAM)
}

// TestConstructAppMeshNetworkConfig tests createAppMeshConfig creates the correct
// configuration for app mesh plugin
func TestConstructAppMeshNetworkConfig(t *testing.T) {
	config := &appmesh.AppMesh{
		IgnoredUID:       "1337",
		IgnoredGID:       "1448",
		ProxyIngressPort: "15000",
		ProxyEgressPort:  "15001",
		AppPorts: []string{
			"9000",
		},
		EgressIgnoredPorts: []string{
			"9001",
		},
		EgressIgnoredIPs: []string{
			"169.254.169.254",
		},
	}

	appMeshIfName, appMeshNetworkConfig, err := NewAppMeshConfig(config, &Config{})
	require.NoError(t, err, "Failed to construct app mesh network config")
	assert.Equal(t, "aws-appmesh", appMeshIfName)
	appMeshConfig := &AppMeshConfig{}
	err = json.Unmarshal(appMeshNetworkConfig.Bytes, appMeshConfig)
	require.NoError(t, err, "unmarshal config from bytes failed")

	assert.Equal(t, config.IgnoredUID, appMeshConfig.IgnoredUID)
	assert.Equal(t, config.IgnoredGID, appMeshConfig.IgnoredGID)
	assert.Equal(t, config.ProxyIngressPort, appMeshConfig.ProxyIngressPort)
	assert.Equal(t, config.ProxyEgressPort, appMeshConfig.ProxyEgressPort)
	assert.Equal(t, len(config.ProxyEgressPort), len(appMeshConfig.ProxyEgressPort))
	assert.Equal(t, config.ProxyEgressPort[0], appMeshConfig.ProxyEgressPort[0])
	assert.Equal(t, len(config.EgressIgnoredIPs), len(appMeshConfig.EgressIgnoredIPs))
	assert.Equal(t, config.EgressIgnoredIPs[0], appMeshConfig.EgressIgnoredIPs[0])
}

func TestConstructIPAMNetworkConfig(t *testing.T) {
	config := &Config{
		ID:                    eniMACAddress,
		ContainerID:           "containerid12",
		ContainerPID:          "pid",
		BlockInstanceMetadata: true,
	}

	vethName, networkConfig, err := NewIPAMNetworkConfig(config)
	require.NoError(t, err, "Failed to construct network config")
	assert.Equal(t, "ecs-eth0", vethName)
	ipamNetworkConfig := &IPAMNetworkConfig{}
	err = json.Unmarshal(networkConfig.Bytes, ipamNetworkConfig)
	require.NoError(t, err, "unmarshal config from bytes failed")
	_, dst, _ := net.ParseCIDR("169.254.170.2/32")
	expectedConfig := &IPAMNetworkConfig{
		Type: "ecs-ipam",
		Name: "ecs-ipam",
		IPAM: IPAMConfig{
			Type:       "ecs-ipam",
			ID:         eniMACAddress,
			IPV4Subnet: "169.254.172.0/22",
			IPV4Routes: []*cnitypes.Route{{Dst: *dst}},
		},
	}
	expectedConfigBytes, _ := json.Marshal(expectedConfig)
	assert.Equal(t, expectedConfigBytes, networkConfig.Bytes)
}

// TestConstructBridgeNetworkConfigWithIPAM tests createBridgeNetworkConfigWithIPAM
// creates the correct configuration for bridge and ipam plugin
func TestConstructNetworkConfig(t *testing.T) {
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

	vethName, bridgeNetworkConfig, err := NewBridgeNetworkConfig(config, true)
	require.NoError(t, err, "construct bridge plugins configuration failed")
	assert.Equal(t, "ecs-eth0", vethName)
	bridgeConfig := &BridgeConfig{}
	err = json.Unmarshal(bridgeNetworkConfig.Bytes, bridgeConfig)
	require.NoError(t, err, "unmarshal bridge config from bytes failed: %s",
		string(bridgeNetworkConfig.Bytes))
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

// Asserts that CNI plugin version matches the expected version
func TestCNIPluginVersionNumber(t *testing.T) {
	versionStr := getCNIVersionString(t)
	assert.Equal(t, currentECSCNIVersion, versionStr)
}

// Asserts that CNI plugin version is upgraded when new commits are made to CNI plugin submodule
func TestCNIPluginVersionUpgrade(t *testing.T) {
	versionStr := getCNIVersionString(t)
	cmd := exec.Command("git", "submodule")
	versionInfo, err := cmd.Output()
	assert.NoError(t, err, "Error running the command: git submodule")
	versionInfoStrList := strings.Split(string(versionInfo), "\n")
	// If a new commit is added, version should be upgraded
	if currentECSCNIGitHash != strings.Split(versionInfoStrList[0], " ")[1] {
		assert.NotEqual(t, currentECSCNIVersion, versionStr)
	}
	assert.Equal(t, currentVPCCNIGitHash, strings.Split(versionInfoStrList[1], " ")[1])
}

// Returns the version in CNI plugin VERSION file as a string
func getCNIVersionString(t *testing.T) string {
	// ../../amazon-ecs-cni-plugins/VERSION
	versionFilePath := filepath.Clean(filepath.Join("..", "..", "amazon-ecs-cni-plugins", "VERSION"))
	versionStr, err := ioutil.ReadFile(versionFilePath)
	assert.NoError(t, err, "Error reading the CNI plugin version file")
	return strings.TrimSpace(string(versionStr))
}
