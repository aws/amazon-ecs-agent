//go:build !windows
// +build !windows

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
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"

	"github.com/containernetworking/cni/pkg/types"
)

const (
	// CNIPluginPathDefault is the directory where CNI plugin binaries are located.
	CNIPluginPathDefault = "/usr/local/bin"

	// ENISetupTimeout is the maximum duration that ENI manager waits before aborting ENI setup.
	ENISetupTimeout = 1 * time.Minute

	BridgePluginName         = "ecs-bridge"
	ENIPluginName            = "ecs-eni"
	IPAMPluginName           = "ecs-ipam"
	AppMeshPluginName        = "aws-appmesh"
	ServiceConnectPluginName = "ecs-serviceconnect"

	VPCBranchENIPluginName        = "vpc-branch-eni"
	vpcBranchENICNISpecVersion    = "0.3.1"
	VPCBranchENIInterfaceTypeVlan = "vlan"
	VPCBranchENIInterfaceTypeTap  = "tap"

	VPCTunnelPluginName          = "vpc-tunnel"
	vpcTunnelCNISpecVersion      = "0.3.1"
	VPCTunnelInterfaceTypeGeneve = "geneve"
	VPCTunnelInterfaceTypeTap    = "tap"

	BridgeInterfaceName = "fargate-bridge"
	VethInterfaceType   = "veth"

	IPAMDataFileName = "eni-ipam.db"

	// Timeout duration for each network setup and cleanup operation before it is cancelled.
	nsSetupTimeoutDuration   = 1 * time.Minute
	nsCleanupTimeoutDuration = 30 * time.Second
)

// createENIPluginConfigs constructs the configuration object for eni plugin
func createENIPluginConfigs(netNSPath string, eni *networkinterface.NetworkInterface) ecscni.PluginConfig {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNISpecVersion: cniSpecVersion,
		CNIPluginName:  ENIPluginName,
	}

	// Tasks can have multiple ENIs where each ENI connects to a different VPC. These VPCs will have
	// conflicting configurations, so each interface representing an ENI needs to be placed in a
	// separate network namespace and configured accordingly. Currently, each task is given only one
	// network namespace configured for the primary ENI. Secondary ENI(s) are not brought up because
	// they wouldn't work in the primary ENI's namespace.
	stayDown := !eni.IsPrimary()

	eniConfig := ecscni.NewENIConfig(
		cniConfig,
		eni,
		blockInstanceMetadataDefault,
		stayDown,
		mtu)

	return eniConfig
}

// createBridgePluginConfig constructs the configuration object for bridge plugin
func createBridgePluginConfig(netNSPath string) ecscni.PluginConfig {
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

	return bridgeConfig
}

// createDaemonBridgePluginConfig constructs the configuration object for bridge plugin in daemon-bridge mode.
// It includes routes for ECS agent endpoint and default route for external traffic.
// The ipComp parameter determines whether to configure IPv4, IPv6, or both routes.
func createDaemonBridgePluginConfig(netNSPath string, ipComp ipcompatibility.IPCompatibility) (ecscni.PluginConfig, error) {
	// Validate that at least one IP version is compatible
	if !ipComp.IsIPv4Compatible() && !ipComp.IsIPv6Compatible() {
		return nil, errors.New("host is neither IPv4 nor IPv6 compatible")
	}

	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNISpecVersion: cniSpecVersion,
		CNIPluginName:  BridgePluginName,
	}

	// Always include ECS agent endpoint route (IPv4 link-local)
	_, agentRouteIPNet, _ := net.ParseCIDR(AgentEndpoint)
	agentRoute := &types.Route{
		Dst: *agentRouteIPNet,
	}

	var ipv4Routes []*types.Route
	var ipv6Routes []*types.Route

	// Always add agent endpoint to IPv4 routes
	ipv4Routes = append(ipv4Routes, agentRoute)

	// Add IPv4 default route if IPv4 compatible
	if ipComp.IsIPv4Compatible() {
		_, defaultNet, _ := net.ParseCIDR(DefaultRouteDestination)
		bridgeGW := net.ParseIP(DaemonBridgeGatewayIP)
		defaultRoute := &types.Route{
			Dst: *defaultNet,
			GW:  bridgeGW,
		}
		ipv4Routes = append(ipv4Routes, defaultRoute)
	}

	// Add IPv6 default route if IPv6 compatible
	if ipComp.IsIPv6Compatible() {
		_, defaultNetV6, _ := net.ParseCIDR(DefaultRouteDestinationIPv6)
		bridgeGWv6 := net.ParseIP(DaemonBridgeGatewayIPv6)
		defaultRouteV6 := &types.Route{
			Dst: *defaultNetV6,
			GW:  bridgeGWv6,
		}
		ipv6Routes = append(ipv6Routes, defaultRouteV6)
	}

	ipamConfig := &ecscni.IPAMConfig{
		CNIConfig: ecscni.CNIConfig{
			NetNSPath:      netNSPath,
			CNISpecVersion: cniSpecVersion,
			CNIPluginName:  IPAMPluginName,
		},
		IPV4Subnet: ECSSubNet,
		IPV4Routes: ipv4Routes,
		IPV6Routes: ipv6Routes,
		ID:         netNSPath,
	}

	// Invoke the bridge plugin and ipam plugin
	bridgeConfig := &ecscni.BridgeConfig{
		CNIConfig: cniConfig,
		Name:      BridgeInterfaceName,
		IPAM:      *ipamConfig,
	}

	return bridgeConfig, nil
}

func createAppMeshPluginConfig(
	netNSPath string,
	cfg *appmesh.AppMesh,
) ecscni.PluginConfig {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNISpecVersion: cniSpecVersion,
		CNIPluginName:  AppMeshPluginName,
	}

	return ecscni.NewAppMeshConfig(cniConfig, cfg)
}

// createBranchENIConfig creates a new vpc-branch-eni CNI plugin configuration.
func createBranchENIConfig(
	netNSPath string,
	iface *networkinterface.NetworkInterface,
	ifType string,
	blockInstanceMetadata bool,
) ecscni.PluginConfig {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNIPluginName:  VPCBranchENIPluginName,
		CNISpecVersion: vpcBranchENICNISpecVersion,
	}

	var ifName string
	if ifType == VPCBranchENIInterfaceTypeVlan {
		// For VLAN interfaces, use the VLAN formatted name ("eth1.vlanid") as interface name.
		ifName = iface.DeviceName
	} else {
		// For all others, including TAP interfaces, use the task ENI index for easy identification.
		ifName = fmt.Sprintf("eth%d", iface.Index)
	}

	eniConfig := &ecscni.VPCBranchENIConfig{
		CNIConfig:          cniConfig,
		TrunkMACAddress:    iface.InterfaceVlanProperties.TrunkInterfaceMacAddress,
		BranchVlanID:       iface.InterfaceVlanProperties.VlanID,
		BranchMACAddress:   iface.MacAddress,
		IPAddresses:        iface.GetIPAddressesWithPrefixLength(),
		GatewayIPAddresses: []string{},
		BlockIMDS:          blockInstanceMetadata,
		InterfaceType:      ifType,
		UID:                strconv.Itoa(int(iface.UserID)),
		GID:                strconv.Itoa(int(iface.UserID)),

		// PluginConfig passes IfName to CNI plugins as the CNI_IFNAME runtime argument.
		// This is used by vpc-branch-eni plugin for the name of the VLAN/TAP interface.
		IfName: ifName,
	}
	if iface.IPv6Only() {
		eniConfig.GatewayIPAddresses = []string{iface.GetSubnetGatewayIPv6Address()}
	} else {
		eniConfig.GatewayIPAddresses = []string{iface.GetSubnetGatewayIPv4Address()}
	}
	return eniConfig
}

// NewTunnelConfig creates a new vpc-tunnel CNI plugin configuration.
func NewTunnelConfig(
	netNSPath string,
	iface *networkinterface.NetworkInterface,
	ifType string,
) ecscni.PluginConfig {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNIPluginName:  VPCTunnelPluginName,
		CNISpecVersion: vpcTunnelCNISpecVersion,
	}
	dport := strconv.Itoa(int(iface.TunnelProperties.DestinationPort))

	var ifName string
	if ifType == VPCTunnelInterfaceTypeGeneve {
		// For Geneve interfaces, the naming pattern is gnv.<destination port>.
		ifName = iface.DeviceName
	} else {
		// For TAP interfaces, the naming pattern is eth<eni index>.
		ifName = fmt.Sprintf("eth%d", iface.Index)
	}

	return &ecscni.VPCTunnelConfig{
		CNIConfig:            cniConfig,
		DestinationIPAddress: iface.TunnelProperties.DestinationIPAddress,
		VNI:                  iface.TunnelProperties.ID,
		DestinationPort:      dport,
		Primary:              iface.IsPrimary(),
		IPAddresses:          iface.GetIPAddressesWithPrefixLength(),
		GatewayIPAddress:     iface.GetSubnetGatewayIPv4Address(),
		InterfaceType:        ifType,
		UID:                  strconv.Itoa(int(iface.UserID)),
		GID:                  strconv.Itoa(int(iface.UserID)),

		// PluginConfig passes IfName to CNI plugins as the CNI_IFNAME runtime argument.
		// This is used by vpc-tunnel plugin for the name of the VLAN/TAP interface.
		IfName: ifName,
	}
}

func createServiceConnectCNIConfig(
	iface *networkinterface.NetworkInterface,
	netNSPath string,
	scConfig *serviceconnect.ServiceConnectConfig,
) *ecscni.ServiceConnectCNIConfig {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netNSPath,
		CNISpecVersion: cniSpecVersion,
		CNIPluginName:  ServiceConnectPluginName,
	}

	enableIPV4 := len(iface.IPV4Addresses) > 0
	enableIPV6 := len(iface.IPV6Addresses) > 0
	return ecscni.NewServiceConnectCNIConfig(cniConfig, scConfig, enableIPV4, enableIPV6)
}
