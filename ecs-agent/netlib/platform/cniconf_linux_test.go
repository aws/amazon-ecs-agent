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
	"encoding/json"
	"net"
	"strconv"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/stretchr/testify/require"
)

const (
	netNSPath         = "ns1"
	ipAddress         = "169.254.0.1"
	eniID             = "eni-abe4d"
	eniMAC            = "f0:5c:89:a3:ab:01"
	subnetGatewayCIDR = "10.1.0.1/24"
	deviceName        = "eth1"
)

func TestCreateBridgeConfig(t *testing.T) {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNISpecVersion: cniSpecVersion,
		CNIPluginName:  BridgePluginName,
	}

	_, routeIPNet, _ := net.ParseCIDR(AgentEndpoint)
	route := &types.Route{
		Dst: *routeIPNet,
	}

	ipamConfig := &ecscni.IPAMConfig{
		CNIConfig: ecscni.CNIConfig{
			NetNSPath:      netNSPath,
			CNISpecVersion: cniSpecVersion,
			CNIPluginName:  IPAMPluginName,
		},
		IPV4Subnet: ECSSubNet,
		IPV4Routes: []*types.Route{route},
		ID:         netNSPath,
	}

	// Invoke the bridge plugin and ipam plugin
	bridgeConfig := &ecscni.BridgeConfig{
		CNIConfig: cniConfig,
		Name:      BridgeInterfaceName,
		IPAM:      *ipamConfig,
	}

	expected, err := json.Marshal(bridgeConfig)
	require.NoError(t, err)
	actual, err := json.Marshal(createBridgePluginConfig(netNSPath))
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func TestCreateENIConfig(t *testing.T) {
	eni := getTestRegularENI()
	eniConfig := &ecscni.ENIConfig{
		CNIConfig: ecscni.CNIConfig{
			NetNSPath:      netNSPath,
			CNISpecVersion: cniSpecVersion,
			CNIPluginName:  ENIPluginName,
		},
		ENIID:                 eni.ID,
		MACAddress:            eni.MacAddress,
		IPAddresses:           eni.GetIPAddressesWithPrefixLength(),
		GatewayIPAddresses:    []string{eni.GetSubnetGatewayIPv4Address()},
		BlockInstanceMetadata: true,
		StayDown:              false,
		DeviceName:            eni.DeviceName,
		MTU:                   mtu,
	}

	expected, err := json.Marshal(eniConfig)
	require.NoError(t, err)
	actual, err := json.Marshal(createENIPluginConfigs(netNSPath, eni))
	require.NoError(t, err)

	require.Equal(t, expected, actual)

	// Non-primary interface case.
	eni.Default = false
	eniConfig.StayDown = true

	expected, err = json.Marshal(eniConfig)
	require.NoError(t, err)
	actual, err = json.Marshal(createENIPluginConfigs(netNSPath, eni))
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func TestCreateBranchENIConfig(t *testing.T) {
	eni := getTestBranchENI()

	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNIPluginName:  VPCBranchENIPluginName,
		CNISpecVersion: vpcBranchENICNISpecVersion,
	}

	expected := &ecscni.VPCBranchENIConfig{
		CNIConfig:          cniConfig,
		TrunkMACAddress:    eni.InterfaceVlanProperties.TrunkInterfaceMacAddress,
		BranchVlanID:       eni.InterfaceVlanProperties.VlanID,
		BranchMACAddress:   eni.MacAddress,
		IPAddresses:        eni.GetIPAddressesWithPrefixLength(),
		GatewayIPAddresses: []string{eni.GetSubnetGatewayIPv4Address()},
		InterfaceType:      VPCBranchENIInterfaceTypeVlan,
		UID:                strconv.Itoa(int(eni.UserID)),
		GID:                strconv.Itoa(int(eni.UserID)),
		IfName:             "eth1.13",
	}
	require.Equal(t, expected, createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeVlan))

	expected = &ecscni.VPCBranchENIConfig{
		CNIConfig:          cniConfig,
		TrunkMACAddress:    eni.InterfaceVlanProperties.TrunkInterfaceMacAddress,
		BranchVlanID:       eni.InterfaceVlanProperties.VlanID,
		BranchMACAddress:   eni.MacAddress,
		IPAddresses:        eni.GetIPAddressesWithPrefixLength(),
		GatewayIPAddresses: []string{eni.GetSubnetGatewayIPv4Address()},
		InterfaceType:      VPCBranchENIInterfaceTypeTap,
		UID:                strconv.Itoa(int(eni.UserID)),
		GID:                strconv.Itoa(int(eni.UserID)),
		IfName:             "eth0",
	}
	require.Equal(t, expected, createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeTap))
}

func getTestRegularENI() *networkinterface.NetworkInterface {
	return &networkinterface.NetworkInterface{
		InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol,
		ID:                           eniID,
		IPV4Addresses: []*networkinterface.IPV4Address{
			{
				Primary: true,
				Address: ipAddress,
			},
		},
		MacAddress:               eniMAC,
		SubnetGatewayIPV4Address: subnetGatewayCIDR,
		DeviceName:               deviceName,
		Default:                  true,
		DesiredStatus:            status.NetworkReadyPull,
	}
}

func getTestBranchENI() *networkinterface.NetworkInterface {
	return &networkinterface.NetworkInterface{
		InterfaceAssociationProtocol: networkinterface.VLANInterfaceAssociationProtocol,
		IPV4Addresses: []*networkinterface.IPV4Address{
			{
				Primary: true,
				Address: ipAddress,
			},
		},
		MacAddress:               eniMAC,
		SubnetGatewayIPV4Address: subnetGatewayCIDR,
		DeviceName:               "eth1.13",
		DesiredStatus:            status.NetworkReadyPull,
		Default:                  true,
		InterfaceVlanProperties: &networkinterface.InterfaceVlanProperties{
			VlanID:                   "13",
			TrunkInterfaceMacAddress: trunkENIMac,
		},
		UserID: uint32(1000),
		Index:  0,
	}
}
