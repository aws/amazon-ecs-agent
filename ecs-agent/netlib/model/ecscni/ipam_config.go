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

// IPAMConfig defines the configuration required for ipam plugin
type IPAMConfig struct {
	CNIConfig
	// IPv4 fields
	// IPV4Subnet is the ip address range managed by ipam
	IPV4Subnet string `json:"ipv4-subnet,omitempty"`
	// IPV4Address is the ip address to deal with(assign or release) in ipam
	IPV4Address string `json:"ipv4-address,omitempty"`
	// IPV4Gateway is the gateway returned by ipam, defalut the '.1' in the subnet
	IPV4Gateway string `json:"ipv4-gateway,omitempty"`
	// IPV4Routes is the route to added in the container namespace
	IPV4Routes []*types.Route `json:"ipv4-routes,omitempty"`

	// IPv6 fields
	// IPV6Subnet is the IPv6 address range managed by ipam
	IPV6Subnet string `json:"ipv6-subnet,omitempty"`
	// IPV6Address is the IPv6 address to deal with(assign or release) in ipam
	IPV6Address string `json:"ipv6-address,omitempty"`
	// IPV6Gateway is the IPv6 gateway returned by ipam
	IPV6Gateway string `json:"ipv6-gateway,omitempty"`
	// IPV6Routes is the IPv6 route to added in the container namespace
	IPV6Routes []*types.Route `json:"ipv6-routes,omitempty"`

	// ID is the key stored with the assigned ip in ipam
	ID string `json:"id"`
}

func (ic *IPAMConfig) String() string {
	return fmt.Sprintf("%s, ipv4-subnet: %s, ipv4-address: %s, ipv4-gateway: %s, ipv4-routes: %s, ipv6-subnet: %s, ipv6-address: %s, ipv6-gateway: %s, ipv6-routes: %s",
		ic.CNIConfig.String(), ic.IPV4Subnet, ic.IPV4Address, ic.IPV4Gateway, ic.IPV4Routes,
		ic.IPV6Subnet, ic.IPV6Address, ic.IPV6Gateway, ic.IPV6Routes)
}

func (ic *IPAMConfig) InterfaceName() string {
	return "none"
}

func (ic *IPAMConfig) NSPath() string {
	return ic.NetNSPath
}

func (ic *IPAMConfig) CNIVersion() string {
	return ic.CNISpecVersion
}

func (ic *IPAMConfig) PluginName() string {
	return ic.CNIPluginName
}
