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
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
	cniTypes "github.com/containernetworking/cni/pkg/types"
)

const (
	// versionCommand is the command used to get the version of plugin
	versionCommand = "--version"
	// capabilitiesCommand is the command used to get the capabilities of a
	// CNI plugin
	capabilitiesCommand = "--capabilities"
	// ecsSubnet is the available ip addresses to use for task networking
	ecsSubnet = "169.254.172.0/22"
	// TaskIAMRoleEndpoint is the endpoint of ecs-agent exposes credentials for
	// task IAM role
	TaskIAMRoleEndpoint = "169.254.170.2/32"
	// CapabilityAWSVPCNetworkingMode is the capability string, which when
	// present in the output of the '--capabilities' command of a CNI plugin
	// indicates that the plugin can support the ECS "awsvpc" network mode
	CapabilityAWSVPCNetworkingMode = "awsvpc-network-mode"
	// VPCENIPluginName is the binary of the vpc-eni plugin
	VPCENIPluginName = "vpc-eni"
)

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
	// ContainerNetNS is the container namespace
	ContainerNetNS string
	// BridgeName is the name used to create the bridge
	BridgeName string
	// IPAMV4Address is the ipv4 used to assign from ipam
	IPAMV4Address *cniTypes.IPNet
	// ID is the information associate with ip in ipam
	ID string
	// BlockInstanceMetadata specifies if InstanceMetadata endpoint should be blocked
	BlockInstanceMetadata bool
	// AdditionalLocalRoutes specifies additional routes to be added to the task namespace
	AdditionalLocalRoutes []cniTypes.IPNet
	// NetworkConfigs is the list of CNI network configurations to be invoked
	NetworkConfigs []*NetworkConfig
	// InstanceENIDNSServerList stores the list of dns servers for the primary instance ENI.
	// Currently, this field is only populated for Windows and is used during task networking setup.
	InstanceENIDNSServerList []string
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

// VPCENIPluginConfig contains all the information required to invoke the vpc-eni plugin.
type VPCENIPluginConfig struct {
	// Type is the cni plugin name.
	Type string `json:"type,omitempty"`
	// CNIVersion is the cni spec version to use.
	CNIVersion string `json:"cniVersion,omitempty"`
	// DNS is used to pass DNS information to the plugin.
	DNS types.DNS `json:"dns"`
	// ENIName is the name of the eni on the instance.
	ENIName string `json:"eniName"`
	// ENIMACAddress is the MAC address of the eni.
	ENIMACAddress string `json:"eniMACAddress"`
	// ENIIPAddresses is the is the ipv4 of eni.
	ENIIPAddresses []string `json:"eniIPAddresses"`
	// GatewayIPAddresses specifies the IPv4 address of the subnet gateway for the eni.
	GatewayIPAddresses []string `json:"gatewayIPAddresses"`
	// UseExistingNetwork specifies if existing network should be used instead of creating a new one.
	UseExistingNetwork bool `json:"useExistingNetwork"`
	// BlockIMDS specifies if the IMDS should be blocked for the created endpoint.
	BlockIMDS bool `json:"blockInstanceMetadata"`
}
