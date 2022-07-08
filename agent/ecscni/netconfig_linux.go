//go:build linux
// +build linux

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

package ecscni

import (
	"fmt"
	"net"

	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"

	"github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	"github.com/aws/amazon-ecs-agent/agent/api/eni"

	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
)

// NewBridgeNetworkConfig creates the config of bridge for ADD command, where
// bridge plugin acquires the IP and route information from IPAM.
func NewBridgeNetworkConfig(cfg *Config, includeIPAM bool) (string, *libcni.NetworkConfig, error) {
	// Create the bridge config first.
	bridgeName := defaultBridgeName
	if len(cfg.BridgeName) != 0 {
		bridgeName = cfg.BridgeName
	}

	bridgeConfig := BridgeConfig{
		Type:       ECSBridgePluginName,
		BridgeName: bridgeName,
	}

	// Create the IPAM config if requested.
	if includeIPAM {
		ipamConfig, err := newIPAMConfig(cfg)
		if err != nil {
			return "", nil, fmt.Errorf("NewBridgeNetworkConfig: create ipam configuration failed: %w", err)
		}

		bridgeConfig.IPAM = ipamConfig
	}

	networkConfig, err := newNetworkConfig(bridgeConfig, ECSBridgePluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, fmt.Errorf("NewBridgeNetworkConfig: construct bridge and ipam network configuration failed: %w", err)
	}

	return defaultVethName, networkConfig, nil
}

// NewIPAMNetworkConfig creates the IPAM configuration accepted by libcni.
func NewIPAMNetworkConfig(cfg *Config) (string, *libcni.NetworkConfig, error) {
	ipamConfig, err := newIPAMConfig(cfg)
	if err != nil {
		return defaultVethName, nil, fmt.Errorf("NewIPAMNetworkConfig: create ipam network configuration failed: %w", err)
	}

	ipamNetworkConfig := IPAMNetworkConfig{
		Name: ECSIPAMPluginName,
		Type: ECSIPAMPluginName,
		IPAM: ipamConfig,
	}

	networkConfig, err := newNetworkConfig(ipamNetworkConfig, ECSIPAMPluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, fmt.Errorf("NewIPAMNetworkConfig: construct ipam network configuration failed: %w", err)
	}

	return defaultVethName, networkConfig, nil
}

func newIPAMConfig(cfg *Config) (IPAMConfig, error) {
	_, dst, err := net.ParseCIDR(TaskIAMRoleEndpoint)
	if err != nil {
		return IPAMConfig{}, err
	}

	routes := []*cnitypes.Route{
		{
			Dst: *dst,
		},
	}

	for _, route := range cfg.AdditionalLocalRoutes {
		seelog.Debugf("[ECSCNI] Adding an additional route for %s", route)
		ipNetRoute := (net.IPNet)(route)
		routes = append(routes, &cnitypes.Route{Dst: ipNetRoute})
	}

	ipamConfig := IPAMConfig{
		Type:        ECSIPAMPluginName,
		IPV4Subnet:  ecsSubnet,
		IPV4Address: cfg.IPAMV4Address,
		ID:          cfg.ID,
		IPV4Routes:  routes,
	}

	return ipamConfig, nil
}

// NewENINetworkConfig creates a new ENI CNI network configuration.
func NewENINetworkConfig(eni *eni.ENI, cfg *Config) (string, *libcni.NetworkConfig, error) {
	eniConf := ENIConfig{
		Type:                  ECSENIPluginName,
		ENIID:                 eni.ID,
		MACAddress:            eni.MacAddress,
		IPAddresses:           eni.GetIPAddressesWithPrefixLength(),
		GatewayIPAddresses:    []string{eni.GetSubnetGatewayIPv4Address()},
		BlockInstanceMetadata: cfg.BlockInstanceMetadata,
	}

	networkConfig, err := newNetworkConfig(eniConf, ECSENIPluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, fmt.Errorf("cni config: failed to create configuration: %w", err)
	}

	return defaultENIName, networkConfig, nil
}

// NewBranchENINetworkConfig creates a new branch ENI CNI network configuration.
func NewBranchENINetworkConfig(eni *eni.ENI, cfg *Config) (string, *libcni.NetworkConfig, error) {
	eniConf := BranchENIConfig{
		Type:                  ECSBranchENIPluginName,
		TrunkMACAddress:       eni.InterfaceVlanProperties.TrunkInterfaceMacAddress,
		BranchVlanID:          eni.InterfaceVlanProperties.VlanID,
		BranchMACAddress:      eni.MacAddress,
		IPAddresses:           eni.GetIPAddressesWithPrefixLength(),
		GatewayIPAddresses:    []string{eni.GetSubnetGatewayIPv4Address()},
		BlockInstanceMetadata: cfg.BlockInstanceMetadata,
		InterfaceType:         vpcCNIPluginInterfaceType,
	}

	networkConfig, err := newNetworkConfig(eniConf, ECSBranchENIPluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, fmt.Errorf("NewBranchENINetworkConfig: construct the eni network configuration failed: %w", err)
	}

	return defaultENIName, networkConfig, nil
}

// NewAppMeshConfig creates a new AppMesh CNI network configuration.
func NewAppMeshConfig(appMesh *appmesh.AppMesh, cfg *Config) (string, *libcni.NetworkConfig, error) {
	appMeshConfig := AppMeshConfig{
		Type:               ECSAppMeshPluginName,
		IgnoredUID:         appMesh.IgnoredUID,
		IgnoredGID:         appMesh.IgnoredGID,
		ProxyIngressPort:   appMesh.ProxyIngressPort,
		ProxyEgressPort:    appMesh.ProxyEgressPort,
		AppPorts:           appMesh.AppPorts,
		EgressIgnoredPorts: appMesh.EgressIgnoredPorts,
		EgressIgnoredIPs:   appMesh.EgressIgnoredIPs,
	}

	networkConfig, err := newNetworkConfig(appMeshConfig, ECSAppMeshPluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, fmt.Errorf("NewAppMeshConfig: construct the app mesh network configuration failed: %w", err)
	}

	return defaultAppMeshIfName, networkConfig, nil
}

// NewServiceConnectNetworkConfig creates a new ServiceConnect CNI network configuration
func NewServiceConnectNetworkConfig(
	scConfig *serviceconnect.Config,
	redirectMode RedirectMode,
	shouldIncludeRedirectIP bool,
	enableIPv4 bool,
	enableIPv6 bool,
	cfg *Config) (string, *libcni.NetworkConfig, error) {
	var ingressConfig []IngressConfigJSONEntry
	for _, ic := range scConfig.IngressConfig {
		newEntry := IngressConfigJSONEntry{
			ListenerPort: ic.ListenerPort,
		}
		if ic.InterceptPort != nil {
			newEntry.InterceptPort = *ic.InterceptPort
		}
		ingressConfig = append(ingressConfig, newEntry)
	}

	var egressConfig *EgressConfigJSON
	if scConfig.EgressConfig != nil {
		egressConfig = &EgressConfigJSON{
			RedirectMode: string(redirectMode),
			VIP: VIPConfigJSON{
				IPv4CIDR: scConfig.EgressConfig.VIP.IPV4CIDR,
				IPv6CIDR: scConfig.EgressConfig.VIP.IPV6CIDR,
			},
		}
		switch redirectMode {
		case NAT:
			// NAT redirect mode is for awsvpc tasks, where the one and only pause container netns will have a NAT redirect rule.
			egressConfig.ListenerPort = scConfig.EgressConfig.ListenerPort
		case TPROXY: // bridge
			// TPROXY redirect mode is used for bridge-mode tasks. There are two use cases:
			// 1. SC pause container netns will set up TPROXY that requires the Egress port
			// 2. Other task pause container netns will add a route for traffic destined for SC VIP-CIDR to go to SC container.
			//    In that case the configuration requires the SC (pause) container IP.
			if shouldIncludeRedirectIP {
				scNetworkConfig := scConfig.NetworkConfig
				egressConfig.RedirectIP = &RedirectIPJson{
					IPv4: scNetworkConfig.SCPauseIPv4Addr,
					IPv6: scNetworkConfig.SCPauseIPv6Addr,
				}
			} else {
				// for sc pause container, pass egress listener port for setting up tproxy
				egressConfig.ListenerPort = scConfig.EgressConfig.ListenerPort
			}
		default:
			return "", nil, fmt.Errorf("NewServiceConnectNetworkConfig: unknown redirect mode %s", string(redirectMode))
		}
	}

	scNetworkConfig := ServiceConnectConfig{
		Name:          ECSServiceConnectPluginName,
		Type:          ECSServiceConnectPluginName,
		IngressConfig: ingressConfig,
		EgressConfig:  egressConfig,
		EnableIPv4:    enableIPv4,
		EnableIPv6:    enableIPv6,
	}
	networkConfig, err := newNetworkConfig(scNetworkConfig, ECSServiceConnectPluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, fmt.Errorf("NewServiceConnectNetworkConfig: construct the service connect network configuration failed: %w", err)
	}
	return defaultServiceConnectIfName, networkConfig, nil
}
