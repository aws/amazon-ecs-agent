// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"
	"net"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	"github.com/aws/amazon-ecs-agent/agent/api/eni"

	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
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
			return "", nil, errors.Wrap(err, "NewBridgeNetworkConfig: create ipam configuration failed")
		}

		bridgeConfig.IPAM = ipamConfig
	}

	networkConfig, err := newNetworkConfig(bridgeConfig, ECSBridgePluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, errors.Wrap(err, "NewBridgeNetworkConfig: construct bridge and ipam network configuration failed")
	}

	return defaultVethName, networkConfig, nil
}

// newNetworkConfig converts a network config to libcni's NetworkConfig.
func newNetworkConfig(netcfg interface{}, plugin string, cniVersion string) (*libcni.NetworkConfig, error) {
	configBytes, err := json.Marshal(netcfg)
	if err != nil {
		seelog.Errorf("[ECSCNI] Marshal configuration for plugin %s failed, error: %v", plugin, err)
		return nil, err
	}

	netConfig := &libcni.NetworkConfig{
		Network: &cnitypes.NetConf{
			Type:       plugin,
			CNIVersion: cniVersion,
		},
		Bytes: configBytes,
	}

	return netConfig, nil
}

// NewIPAMNetworkConfig creates the IPAM configuration accepted by libcni.
func NewIPAMNetworkConfig(cfg *Config) (string, *libcni.NetworkConfig, error) {
	ipamConfig, err := newIPAMConfig(cfg)
	if err != nil {
		return defaultVethName, nil, errors.Wrap(err, "NewIPAMNetworkConfig: create ipam network configuration failed")
	}

	ipamNetworkConfig := IPAMNetworkConfig{
		Name: ECSIPAMPluginName,
		Type: ECSIPAMPluginName,
		IPAM: ipamConfig,
	}

	networkConfig, err := newNetworkConfig(ipamNetworkConfig, ECSIPAMPluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, errors.Wrap(err, "NewIPAMNetworkConfig: construct ipam network configuration failed")
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
	ipv4Addr := eni.GetPrimaryIPv4Address()

	eniConf := ENIConfig{
		Type:                     ECSENIPluginName,
		ENIID:                    eni.ID,
		IPV4Address:              ipv4Addr,
		IPV6Address:              "",
		MACAddress:               eni.MacAddress,
		BlockInstanceMetadata:    cfg.BlockInstanceMetadata,
		SubnetGatewayIPV4Address: eni.SubnetGatewayIPV4Address,
	}

	networkConfig, err := newNetworkConfig(eniConf, ECSENIPluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, errors.Wrap(err, "cni config: failed to create configuration")
	}

	return defaultENIName, networkConfig, nil
}

// NewBranchENINetworkConfig creates a new branch ENI CNI network configuration.
func NewBranchENINetworkConfig(eni *eni.ENI, cfg *Config) (string, *libcni.NetworkConfig, error) {
	// ENIIPAddress does not have a prefix length while BranchIPAddress expects a prefix length.
	// SubnetGatewayIPV4Address has a prefix length while BranchGatewayIPAddress does not expect a prefix length.
	s := strings.Split(eni.SubnetGatewayIPV4Address, "/")
	branchIPv4Address := eni.GetPrimaryIPv4Address() + "/" + s[1]
	branchGatewayIPAddress := s[0]

	eniConf := BranchENIConfig{
		Type:                   ECSBranchENIPluginName,
		TrunkMACAddress:        eni.InterfaceVlanProperties.TrunkInterfaceMacAddress,
		BranchVlanID:           eni.InterfaceVlanProperties.VlanID,
		BranchMACAddress:       eni.MacAddress,
		BranchIPAddress:        branchIPv4Address,
		BranchGatewayIPAddress: branchGatewayIPAddress,
		InterfaceType:          vpcCNIPluginInterfaceType,
		BlockInstanceMetadata:  cfg.BlockInstanceMetadata,
	}

	networkConfig, err := newNetworkConfig(eniConf, ECSBranchENIPluginName, cfg.MinSupportedCNIVersion)
	if err != nil {
		return "", nil, errors.Wrap(err, "NewBranchENINetworkConfig: construct the eni network configuration failed")
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
		return "", nil, errors.Wrap(err, "NewAppMeshConfig: construct the app mesh network configuration failed")
	}

	return defaultAppMeshIfName, networkConfig, nil
}
