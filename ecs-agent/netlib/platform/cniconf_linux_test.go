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
	"fmt"
	"net"
	"strconv"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/stretchr/testify/require"
)

const (
	netNSPath       = "ns1"
	ipV4Address     = "169.254.0.1"
	ipV6Address     = "2600:1f13:4d9:e611:9009:ac97:1ab4:17d1"
	eniID           = "eni-abe4d"
	vni             = "ABC123"
	destinationIP   = "10.0.3.1"
	destinationPort = 6081
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
	for _, tc := range []struct {
		name      string
		eni       *networkinterface.NetworkInterface
		eniConfig *ecscni.ENIConfig
	}{
		{
			name: "ipv4 only",
			eni:  getTestRegularV4ENI(),
			eniConfig: &ecscni.ENIConfig{
				CNIConfig: ecscni.CNIConfig{
					NetNSPath:      netNSPath,
					CNISpecVersion: cniSpecVersion,
					CNIPluginName:  ENIPluginName,
				},
				ENIID:                 eniID,
				MACAddress:            eniMAC,
				IPAddresses:           []string{ipV4Address + "/24"},
				GatewayIPAddresses:    []string{"10.1.0.1"},
				BlockInstanceMetadata: true,
				StayDown:              false,
				DeviceName:            deviceName,
				MTU:                   mtu,
			},
		},
		{
			name: "dual stack",
			eni:  getTestRegularV4V6ENI(),
			eniConfig: &ecscni.ENIConfig{
				CNIConfig: ecscni.CNIConfig{
					NetNSPath:      netNSPath,
					CNISpecVersion: cniSpecVersion,
					CNIPluginName:  ENIPluginName,
				},
				ENIID:                 eniID,
				MACAddress:            eniMAC,
				IPAddresses:           []string{ipV4Address + "/24", ipV6Address + "/64"},
				GatewayIPAddresses:    []string{"10.1.0.1"},
				BlockInstanceMetadata: true,
				StayDown:              false,
				DeviceName:            deviceName,
				MTU:                   mtu,
			},
		},
		{
			name: "ipv6 only",
			eni:  getTestRegularV6ENI(),
			eniConfig: &ecscni.ENIConfig{
				CNIConfig: ecscni.CNIConfig{
					NetNSPath:      netNSPath,
					CNISpecVersion: cniSpecVersion,
					CNIPluginName:  ENIPluginName,
				},
				ENIID:                 eniID,
				MACAddress:            eniMAC,
				IPAddresses:           []string{ipV6Address + "/60"},
				GatewayIPAddresses:    []string{"2600:1f14:30ab:6902::"},
				BlockInstanceMetadata: true,
				StayDown:              false,
				DeviceName:            deviceName,
				MTU:                   mtu,
			},
		},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			expected, err := json.Marshal(tc.eniConfig)
			require.NoError(t, err)
			actual, err := json.Marshal(createENIPluginConfigs(netNSPath, tc.eni))
			require.NoError(t, err)
			require.Equal(t, expected, actual)

			// Non-primary interface case.
			tc.eni.Default = false
			tc.eniConfig.StayDown = true

			expected, err = json.Marshal(tc.eniConfig)
			require.NoError(t, err)
			actual, err = json.Marshal(createENIPluginConfigs(netNSPath, tc.eni))
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

func TestCreateBranchENIConfig(t *testing.T) {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNIPluginName:  VPCBranchENIPluginName,
		CNISpecVersion: vpcBranchENICNISpecVersion,
	}

	for _, tc := range []struct {
		name      string
		eni       *networkinterface.NetworkInterface
		eniConfig *ecscni.VPCBranchENIConfig
	}{
		{
			name: "ipv4 only",
			eni:  getTestBranchV4ENI(),
			eniConfig: &ecscni.VPCBranchENIConfig{
				CNIConfig:          cniConfig,
				TrunkMACAddress:    trunkENIMac,
				BranchVlanID:       "13",
				BranchMACAddress:   eniMAC,
				IPAddresses:        []string{ipV4Address + "/24"},
				GatewayIPAddresses: []string{"10.1.0.1"},
				InterfaceType:      VPCBranchENIInterfaceTypeVlan,
				BlockIMDS:          true,
				UID:                strconv.Itoa(1000),
				GID:                strconv.Itoa(1000),
				IfName:             "eth1.13",
			},
		},
		{
			name: "dual stack",
			eni:  getTestBranchV4V6ENI(),
			eniConfig: &ecscni.VPCBranchENIConfig{
				CNIConfig:          cniConfig,
				TrunkMACAddress:    trunkENIMac,
				BranchVlanID:       "13",
				BranchMACAddress:   eniMAC,
				IPAddresses:        []string{ipV4Address + "/24", ipV6Address + "/64"},
				GatewayIPAddresses: []string{"10.1.0.1"},
				InterfaceType:      VPCBranchENIInterfaceTypeVlan,
				BlockIMDS:          true,
				UID:                strconv.Itoa(1000),
				GID:                strconv.Itoa(1000),
				IfName:             "eth1.13",
			},
		},
		{
			name: "ipv6 only",
			eni:  getTestBranchV6ENI(),
			eniConfig: &ecscni.VPCBranchENIConfig{
				CNIConfig:          cniConfig,
				TrunkMACAddress:    trunkENIMac,
				BranchVlanID:       "13",
				BranchMACAddress:   eniMAC,
				IPAddresses:        []string{ipV6Address + "/60"},
				GatewayIPAddresses: []string{"2600:1f14:30ab:6902::"},
				InterfaceType:      VPCBranchENIInterfaceTypeVlan,
				BlockIMDS:          true,
				UID:                strconv.Itoa(1000),
				GID:                strconv.Itoa(1000),
				IfName:             "eth1.13",
			},
		},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.eniConfig, createBranchENIConfig(netNSPath, tc.eni, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault))

			// Test the new interface type
			tc.eniConfig = &ecscni.VPCBranchENIConfig{
				CNIConfig:          cniConfig,
				TrunkMACAddress:    tc.eni.InterfaceVlanProperties.TrunkInterfaceMacAddress,
				BranchVlanID:       tc.eni.InterfaceVlanProperties.VlanID,
				BranchMACAddress:   tc.eni.MacAddress,
				IPAddresses:        tc.eni.GetIPAddressesWithPrefixLength(),
				GatewayIPAddresses: []string{tc.eni.GetSubnetGatewayIPv4Address()},
				InterfaceType:      VPCBranchENIInterfaceTypeTap,
				BlockIMDS:          false,
				UID:                strconv.Itoa(int(tc.eni.UserID)),
				GID:                strconv.Itoa(int(tc.eni.UserID)),
				IfName:             "eth0",
			}
			if tc.eni.IPv6Only() {
				tc.eniConfig.GatewayIPAddresses = []string{tc.eni.GetSubnetGatewayIPv6Address()}
			}
			require.Equal(t, tc.eniConfig, createBranchENIConfig(netNSPath, tc.eni, VPCBranchENIInterfaceTypeTap, false))
		})
	}
}

func getTestRegularV4ENI() *networkinterface.NetworkInterface {
	return &networkinterface.NetworkInterface{
		InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol,
		ID:                           eniID,
		IPV4Addresses: []*networkinterface.IPV4Address{
			{
				Primary: true,
				Address: ipV4Address,
			},
		},
		MacAddress:               eniMAC,
		SubnetGatewayIPV4Address: subnetGatewayIPv4CIDR,
		DeviceName:               deviceName,
		Default:                  true,
		DesiredStatus:            status.NetworkReadyPull,
	}
}

func getTestRegularV4V6ENI() *networkinterface.NetworkInterface {
	eni := getTestRegularV4ENI()
	eni.IPV6Addresses = []*networkinterface.IPV6Address{
		{
			Primary: true,
			Address: ipV6Address,
		},
	}
	eni.SubnetGatewayIPV6Address = subnetGatewayIPv6CIDR
	return eni
}

func getTestRegularV6ENI() *networkinterface.NetworkInterface {
	eni := getTestRegularV4V6ENI()
	eni.IPV4Addresses = nil
	eni.SubnetGatewayIPV4Address = ""
	return eni
}

func getTestBranchV4ENI() *networkinterface.NetworkInterface {
	return &networkinterface.NetworkInterface{
		InterfaceAssociationProtocol: networkinterface.VLANInterfaceAssociationProtocol,
		IPV4Addresses: []*networkinterface.IPV4Address{
			{
				Primary: true,
				Address: ipV4Address,
			},
		},
		MacAddress:               eniMAC,
		SubnetGatewayIPV4Address: subnetGatewayIPv4CIDR,
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

func getTestBranchV4V6ENI() *networkinterface.NetworkInterface {
	eni := getTestBranchV4ENI()
	eni.IPV6Addresses = []*networkinterface.IPV6Address{
		{
			Primary: true,
			Address: ipV6Address,
		},
	}
	eni.SubnetGatewayIPV6Address = subnetGatewayIPv6CIDR
	return eni
}

func getTestBranchV6ENI() *networkinterface.NetworkInterface {
	eni := getTestBranchV4V6ENI()
	eni.IPV4Addresses = nil
	eni.SubnetGatewayIPV4Address = ""
	return eni
}

func getTestV2NInterface() *networkinterface.NetworkInterface {
	return &networkinterface.NetworkInterface{
		InterfaceAssociationProtocol: networkinterface.V2NInterfaceAssociationProtocol,
		SubnetGatewayIPV4Address:     networkinterface.DefaultGeneveInterfaceGateway,
		DesiredStatus:                status.NetworkReadyPull,
		IPV4Addresses: []*networkinterface.IPV4Address{
			{
				Address: networkinterface.DefaultGeneveInterfaceIPAddress,
				Primary: true,
			},
		},
		TunnelProperties: &networkinterface.TunnelProperties{
			ID:                   vni,
			DestinationIPAddress: destinationIP,
			DestinationPort:      destinationPort,
		},
		DeviceName:           fmt.Sprintf(networkinterface.GeneveInterfaceNamePattern, vni, destinationPort),
		Name:                 secondaryENIName,
		DomainNameServers:    []string{nameServer},
		DomainNameSearchList: []string{searchDomainName},
	}
}

func TestCreateDaemonBridgePluginConfig(t *testing.T) {
	// Common CNI config for all test cases
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNISpecVersion: cniSpecVersion,
		CNIPluginName:  BridgePluginName,
	}

	// Common routes
	_, agentRouteIPNet, _ := net.ParseCIDR(AgentEndpoint)
	agentRoute := &types.Route{
		Dst: *agentRouteIPNet,
	}

	_, defaultNet, _ := net.ParseCIDR(DefaultRouteDestination)
	bridgeGW := net.ParseIP(DaemonBridgeGatewayIP)
	defaultRouteIPv4 := &types.Route{
		Dst: *defaultNet,
		GW:  bridgeGW,
	}

	_, defaultNetV6, _ := net.ParseCIDR(DefaultRouteDestinationIPv6)
	bridgeGWv6 := net.ParseIP(DaemonBridgeGatewayIPv6)
	defaultRouteIPv6 := &types.Route{
		Dst: *defaultNetV6,
		GW:  bridgeGWv6,
	}

	for _, tc := range []struct {
		name           string
		ipComp         ipcompatibility.IPCompatibility
		expectedConfig *ecscni.BridgeConfig
		expectError    bool
		errorContains  string
	}{
		{
			name:   "IPv4-only configuration",
			ipComp: ipcompatibility.NewIPv4OnlyCompatibility(),
			expectedConfig: &ecscni.BridgeConfig{
				CNIConfig: cniConfig,
				Name:      BridgeInterfaceName,
				IPAM: ecscni.IPAMConfig{
					CNIConfig: ecscni.CNIConfig{
						NetNSPath:      netNSPath,
						CNISpecVersion: cniSpecVersion,
						CNIPluginName:  IPAMPluginName,
					},
					IPV4Subnet: ECSSubNet,
					IPV4Routes: []*types.Route{agentRoute, defaultRouteIPv4},
					ID:         netNSPath,
				},
			},
			expectError: false,
		},
		{
			name:   "IPv6-only configuration",
			ipComp: ipcompatibility.NewIPv6OnlyCompatibility(),
			expectedConfig: &ecscni.BridgeConfig{
				CNIConfig: cniConfig,
				Name:      BridgeInterfaceName,
				IPAM: ecscni.IPAMConfig{
					CNIConfig: ecscni.CNIConfig{
						NetNSPath:      netNSPath,
						CNISpecVersion: cniSpecVersion,
						CNIPluginName:  IPAMPluginName,
					},
					IPV4Subnet: ECSSubNet,
					IPV4Routes: []*types.Route{agentRoute}, // ECS agent endpoint always included
					IPV6Routes: []*types.Route{defaultRouteIPv6},
					ID:         netNSPath,
				},
			},
			expectError: false,
		},
		{
			name:   "Dual-stack configuration",
			ipComp: ipcompatibility.NewDualStackCompatibility(),
			expectedConfig: &ecscni.BridgeConfig{
				CNIConfig: cniConfig,
				Name:      BridgeInterfaceName,
				IPAM: ecscni.IPAMConfig{
					CNIConfig: ecscni.CNIConfig{
						NetNSPath:      netNSPath,
						CNISpecVersion: cniSpecVersion,
						CNIPluginName:  IPAMPluginName,
					},
					IPV4Subnet: ECSSubNet,
					IPV4Routes: []*types.Route{agentRoute, defaultRouteIPv4},
					IPV6Routes: []*types.Route{defaultRouteIPv6},
					ID:         netNSPath,
				},
			},
			expectError: false,
		},
		{
			name:          "Error when neither IPv4 nor IPv6 compatible",
			ipComp:        ipcompatibility.NewIPCompatibility(false, false),
			expectError:   true,
			errorContains: "host is neither IPv4 nor IPv6 compatible",
		},
	} {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			actualConfig, err := createDaemonBridgePluginConfig(netNSPath, tc.ipComp)

			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorContains)
				require.Nil(t, actualConfig)
			} else {
				require.NoError(t, err)
				expected, err := json.Marshal(tc.expectedConfig)
				require.NoError(t, err)
				actual, err := json.Marshal(actualConfig)
				require.NoError(t, err)
				require.Equal(t, expected, actual)
			}
		})
	}
}
