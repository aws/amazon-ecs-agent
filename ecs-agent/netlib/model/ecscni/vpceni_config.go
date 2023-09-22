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

	"github.com/containernetworking/cni/pkg/types"
)

// VPCENIConfig contains all the information required to invoke the vpc-eni plugin.
type VPCENIConfig struct {
	CNIConfig
	// Name is the network name to be used in network configuration.
	Name string `json:"name"`
	// DNS is used to pass DNS information to the plugin.
	DNS types.DNS `json:"dns"`
	// ENIName is the device name of the eni on the instance.
	ENIName string `json:"eniName"`
	// ENIMACAddress is the MAC address of the eni.
	ENIMACAddress string `json:"eniMACAddress"`
	// ENIIPAddresses is the is the ipv4 of eni.
	ENIIPAddresses []string `json:"eniIPAddresses"`
	// GatewayIPAddresses specifies the IPv4 address of the subnet gateway for the eni.
	GatewayIPAddresses []string `json:"gatewayIPAddresses"`
	// UseExistingNetwork specifies if existing network should be used instead of creating a new one.
	// For Task IAM roles, a pre-existing HNS network is available from which the HNS endpoint should be created.
	// This field specifies that an existing network of provided name should be used during the network setup by the plugin.
	UseExistingNetwork bool `json:"useExistingNetwork"`
	// BlockIMDS specified if the instance metadata endpoint should be blocked for the tasks.
	BlockIMDS bool `json:"blockInstanceMetadata"`
}

func (ec *VPCENIConfig) String() string {
	return fmt.Sprintf("%s, eni: %s, mac: %s, ipAddrs: %v, gateways: %v, "+
		"useExistingNetwork: %t, BlockIMDS: %t, dns: %v",
		ec.CNIConfig.String(), ec.ENIName, ec.ENIMACAddress, ec.ENIIPAddresses,
		ec.GatewayIPAddresses, ec.UseExistingNetwork, ec.BlockIMDS, ec.DNS)
}

// InterfaceName returns the veth pair name will be used inside the namespace.
// For this plugin, interface name is redundant and would be generated in the plugin itself.
func (ec *VPCENIConfig) InterfaceName() string {
	return DefaultInterfaceName
}

func (ec *VPCENIConfig) NSPath() string {
	return ec.NetNSPath
}

func (ec *VPCENIConfig) CNIVersion() string {
	return ec.CNISpecVersion
}

func (ec *VPCENIConfig) PluginName() string {
	return ec.CNIPluginName
}

func (ec *VPCENIConfig) NetworkName() string {
	// We are using this field in the Windows CNI plugin while generating a network name/endpoint name.
	// It is also used to find pre-existing network while setting up fargate-bridge for Task IAM roles.
	return ec.Name
}
