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
	// IPV4Subnet is the ip address range managed by ipam
	IPV4Subnet string `json:"ipv4-subnet,omitempty"`
	// IPV4Address is the ip address to deal with(assign or release) in ipam
	IPV4Address string `json:"ipv4-address,omitempty"`
	// IPV4Gateway is the gateway returned by ipam, defalut the '.1' in the subnet
	IPV4Gateway string `json:"ipv4-gateway,omitempty"`
	// IPV4Routes is the route to added in the container namespace
	IPV4Routes []*types.Route `json:"ipv4-routes,omitempty"`
	// ID is the key stored with the assigned ip in ipam
	ID string `json:"id"`
}

func (ic *IPAMConfig) String() string {
	return fmt.Sprintf("%s, subnet: %s, ip: %s, gw: %s, route: %s",
		ic.CNIConfig.String(), ic.IPV4Subnet, ic.IPV4Address, ic.IPV4Gateway, ic.IPV4Routes)
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
