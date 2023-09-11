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

	return defaultInterfaceName
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
