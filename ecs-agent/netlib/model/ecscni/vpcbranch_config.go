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
)

// VPCBranchENIConfig defines the configuration for vpc-branch-eni plugin
type VPCBranchENIConfig struct {
	CNIConfig
	TrunkName          string   `json:"trunkName"`
	TrunkMACAddress    string   `json:"trunkMACAddress"`
	BranchVlanID       string   `json:"branchVlanID"`
	BranchMACAddress   string   `json:"branchMACAddress"`
	IPAddresses        []string `json:"ipAddresses"`
	GatewayIPAddresses []string `json:"gatewayIPAddresses"`
	BlockIMDS          bool     `json:"blockInstanceMetadata"`
	InterfaceType      string   `json:"interfaceType"`
	UID                string   `json:"uid"`
	GID                string   `json:"gid"`

	// this was used as a parameter of the libcni, which is not part of the plugin
	// configuration, thus no need to marshal
	IfName string `json:"_"`
}

func (c *VPCBranchENIConfig) String() string {
	return fmt.Sprintf("%s, trunk: %s, trunkMAC: %s, branchMAC: %s "+
		"ipAddrs: %v, gateways: %v, vlanID: %s, uid: %s, gid: %s, interfaceType: %s",
		c.CNIConfig.String(), c.TrunkName,
		c.TrunkMACAddress, c.BranchMACAddress, c.IPAddresses, c.GatewayIPAddresses,
		c.BranchVlanID, c.UID, c.GID, c.InterfaceType)
}

func (c *VPCBranchENIConfig) InterfaceName() string {
	if c.IfName != "" {
		return c.IfName
	}

	return DefaultInterfaceName
}

func (c *VPCBranchENIConfig) NSPath() string {
	return c.NetNSPath
}

func (c *VPCBranchENIConfig) CNIVersion() string {
	return c.CNISpecVersion
}

func (c *VPCBranchENIConfig) PluginName() string {
	return c.CNIPluginName
}
