// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
)

const (
	// versionCommand is the command used to get the version of plugin
	versionCommand = "--version"
	// capabilitiesCommand is the command used to get the capabilities of a
	// CNI plugin
	capabilitiesCommand = "--capabilities"
	// defaultVethName is the name of veth pair name in the container namespace
	defaultVethName = "ecs-eth0"
	// defaultENIName is the name of eni interface name in the container namespace
	defaultENIName = "eth0"
	// defaultBridgeName is the default name of bridge created for container to
	// communicate with ecs-agent
	defaultBridgeName = "ecs-bridge"
	// defaultAppMeshIfName is the default name of app mesh to setup iptable rules
	// for app mesh container. IfName is mandatory field to invoke CNI plugin.
	defaultAppMeshIfName = "aws-appmesh"
	// netnsFormat is used to construct the path to cotainer network namespace
	netnsFormat = "/host/proc/%s/ns/net"
	// ecsSubnet is the available ip addresses to use for task networking
	ecsSubnet = "169.254.172.0/22"

	// ECSIPAMPluginName is the binary of the ipam plugin
	ECSIPAMPluginName = "ecs-ipam"
	// ECSBridgePluginName is the binary of the bridge plugin
	ECSBridgePluginName = "ecs-bridge"
	// ECSENIPluginName is the binary of the eni plugin
	ECSENIPluginName = "ecs-eni"
	// ECSAppMeshPluginName is the binary of aws-appmesh plugin
	ECSAppMeshPluginName = "aws-appmesh"
	// ECSBranchENIPluginName is the binary of the branch-eni plugin
	ECSBranchENIPluginName = "vpc-branch-eni"
	// TaskIAMRoleEndpoint is the endpoint of ecs-agent exposes credentials for
	// task IAM role
	TaskIAMRoleEndpoint = "169.254.170.2/32"
	// CapabilityAWSVPCNetworkingMode is the capability string, which when
	// present in the output of the '--capabilities' command of a CNI plugin
	// indicates that the plugin can support the ECS "awsvpc" network mode
	CapabilityAWSVPCNetworkingMode = "awsvpc-network-mode"
)

//IPAMNetworkConfig is the config format accepted by the plugin
type IPAMNetworkConfig struct {
	Name       string     `json:"name,omitempty"`
	Type       string     `json:"type,omitempty"`
	CNIVersion string     `json:"cniVersion,omitempty"`
	IPAM       IPAMConfig `json:"ipam"`
}

// IPAMConfig contains all the information needed to invoke the ipam plugin
type IPAMConfig struct {
	// Type is the cni plugin name
	Type string `json:"type,omitempty"`
	// ID is the information stored in the ipam along with ip as key-value pair
	ID string `json:"id,omitempty"`
	// CNIVersion is the cni spec version to use
	CNIVersion string `json:"cniVersion,omitempty"`
	// IPV4Subnet is the ip address range managed by ipam
	IPV4Subnet string `json:"ipv4-subnet,omitempty"`
	// IPV4Address is the ip address to deal with(assign or release) in ipam
	IPV4Address *cnitypes.IPNet `json:"ipv4-address,omitempty"`
	// IPV4Gateway is the gateway returned by ipam, defalut the '.1' in the subnet
	IPV4Gateway string `json:"ipv4-gateway,omitempty"`
	// IPV4Routes is the route to added in the containerr namespace
	IPV4Routes []*cnitypes.Route `json:"ipv4-routes,omitempty"`
}

// BridgeConfig contains all the information needed to invoke the bridge plugin
type BridgeConfig struct {
	// Type is the cni plugin name
	Type string `json:"type,omitempty"`
	// CNIVersion is the cni spec version to use
	CNIVersion string `json:"cniVersion,omitempty"`
	// BridgeName is the name of bridge
	BridgeName string `json:"bridge"`
	// IsGw indicates whether the bridge act as a gateway, it determines whether
	// an ip address needs to assign to the bridge
	IsGW bool `json:"isGateway"`
	// IsDefaultGW indicates whether the bridge is the gateway of the container
	IsDefaultGW bool `json:"isDefaultGateway"`
	// ForceAddress indicates whether a new ip should be assigned if the bridge
	// has already a different ip
	ForceAddress bool `json:"forceAddress"`
	// IPMasq indicates whether to setup the IP Masquerade for traffic originating
	// from this network
	IPMasq bool `json:"ipMasq"`
	// MTU sets MTU of the bridge interface
	MTU int `json:"mtu"`
	// HairpinMode sets the hairpin mode of interface on the bridge
	HairpinMode bool `json:"hairpinMode"`
	// IPAM is the configuration to acquire ip/route from ipam plugin
	IPAM IPAMConfig `json:"ipam,omitempty"`
}

// ENIConfig contains all the information needed to invoke the eni plugin
type ENIConfig struct {
	// Type is the cni plugin name
	Type string `json:"type,omitempty"`
	// CNIVersion is the cni spec version to use
	CNIVersion string `json:"cniVersion,omitempty"`
	// ENIID is the id of ec2 eni
	ENIID string `json:"eni"`
	// IPV4Address is the ipv4 of eni
	IPV4Address string `json:"ipv4-address"`
	// IPV6Address is the ipv6 of eni
	IPV6Address string `json:"ipv6-address,omitempty"`
	// MacAddress is the mac address of eni
	MACAddress string `json:"mac"`
	// BlockInstanceMetadata specifies if InstanceMetadata endpoint should be blocked
	BlockInstanceMetadata bool `json:"block-instance-metadata"`
	// SubnetGatewayIPV4Address specifies the IPv4 address of the subnet gateway for the ENI
	SubnetGatewayIPV4Address string `json:"subnetgateway-ipv4-address"`
}

// AppMeshConfig contains all the information needed to invoke the app mesh plugin
type AppMeshConfig struct {
	// Type is the cni plugin name
	Type string `json:"type,omitempty"`
	// CNIVersion is the cni spec version to use
	CNIVersion string `json:"cniVersion,omitempty"`
	// IgnoredUID specifies egress traffic from the processes owned by the UID will be ignored
	IgnoredUID string `json:"ignoredUID,omitempty"`
	// IgnoredGID specifies egress traffic from the processes owned by the GID will be ignored
	IgnoredGID string `json:"ignoredGID,omitempty"`
	// ProxyIngressPort is the ingress port number that proxy is listening on
	ProxyIngressPort string `json:"proxyIngressPort"`
	// ProxyEgressPort is the egress port number that proxy is listening on
	ProxyEgressPort string `json:"proxyEgressPort"`
	// AppPorts specifies port numbers that application is listening on
	AppPorts []string `json:"appPorts"`
	// EgressIgnoredPorts is the list of ports for which egress traffic will be ignored
	EgressIgnoredPorts []string `json:"egressIgnoredPorts,omitempty"`
	// EgressIgnoredIPs is the list of IPs for which egress traffic will be ignored
	EgressIgnoredIPs []string `json:"egressIgnoredIPs,omitempty"`
}

// BranchENIConfig contains all the information needed to invoke the vpc-branch-eni plugin
type BranchENIConfig struct {
	// CNIVersion is the CNI spec version to use
	CNIVersion string `json:"cniVersion,omitempty"`
	// Name is the CNI network name
	Name string `json:"name,omitempty"`
	// Type is the CNI plugin name
	Type string `json:"type,omitempty"`

	// TrunkMACAddress is the MAC address of the trunk ENI
	TrunkMACAddress string `json:"trunkMACAddress,omitempty"`
	// BranchVlanID is the VLAN ID of the branch ENI
	BranchVlanID string `json:"branchVlanID,omitempty"`
	// BranchMacAddress is the MAC address of the branch ENI
	BranchMACAddress string `json:"branchMACAddress"`
	// BranchIPAddress is the IP address of the branch ENI
	BranchIPAddress string `json:"branchIPAddress"`
	// BranchGatewayIPAddress is the IP address of the branch ENI's default gateway.
	BranchGatewayIPAddress string `json:"branchGatewayIPAddress"`
	// InterfaceType is the type of the interface to connect the branch ENI to
	InterfaceType string `json:"interfaceType,omitempty"`
	// BlockInstanceMetdata specifies if InstanceMetadata endpoint should be blocked.
	BlockInstanceMetadata bool `json:"blockInstanceMetadata"`
}

// Config contains all the information to set up the container namespace using
// the plugins
type Config struct {
	// PluginsPath indicates the path where cni plugins are located
	PluginsPath string
	// MinSupportedCNIVersion is the minimum cni spec version supported
	MinSupportedCNIVersion string
	// ContainerID is the id of container of which to set up the network namespace
	ContainerID string
	// ContainerPID is the pid of the container
	ContainerPID string
	// BridgeName is the name used to create the bridge
	BridgeName string
	// IPAMV4Address is the ipv4 used to assign from ipam
	IPAMV4Address *cnitypes.IPNet
	// ID is the information associate with ip in ipam
	ID string
	// BlockInstanceMetadata specifies if InstanceMetadata endpoint should be blocked
	BlockInstanceMetadata bool
	// AdditionalLocalRoutes specifies additional routes to be added to the task namespace
	AdditionalLocalRoutes []cnitypes.IPNet
	// NetworkConfigs is the list of CNI network configurations to be invoked
	NetworkConfigs []*NetworkConfig
}

// NetworkConfig wraps CNI library's NetworkConfig object. It tracks the interface device
// name (the IfName param required to invoke AddNetwork) along with libcni's NetworkConfig
// object. The IfName is required to be set to invoke `AddNetwork` method when invoking
// plugins to set up the network namespace.
type NetworkConfig struct {
	// IfName is the name of the network interface device, to be set within the
	// network namespace.
	IfName string
	// CNINetworkConfig is the network configuration required to invoke the CNI plugin
	CNINetworkConfig *libcni.NetworkConfig
}
