package ecscni

import (
	"fmt"
)

// VPCTunnelConfig defines the configuration for vpc-tunnel plugin. This struct will
// be serialized and included as parameter while executing the CNI plugin.
type VPCTunnelConfig struct {
	CNIConfig
	DestinationIPAddress string   `json:"destinationIPAddress"`
	VNI                  string   `json:"vni"`
	DestinationPort      string   `json:"destinationPort"`
	Primary              bool     `json:"primary"`
	IPAddresses          []string `json:"ipAddresses"`
	GatewayIPAddress     string   `json:"gatewayIPAddress"`
	InterfaceType        string   `json:"interfaceType"`
	UID                  string   `json:"uid"`
	GID                  string   `json:"gid"`

	// this was used as a parameter of the libcni, which is not part of the plugin
	// configuration, thus no need to marshal
	IfName string `json:"_"`
}

func (c *VPCTunnelConfig) String() string {
	return fmt.Sprintf("%s, destinationIPAddress: %s, vni: %s, destinationPort: %s, "+
		"ipAddrs: %v, gateways: %v, uid: %s, gid: %s, interfaceType: %s, primary: %v, ifName: %s",
		c.CNIConfig.String(), c.DestinationIPAddress,
		c.VNI, c.DestinationPort, c.IPAddresses, c.GatewayIPAddress,
		c.UID, c.GID, c.InterfaceType, c.Primary, c.IfName)
}

func (c *VPCTunnelConfig) InterfaceName() string {
	if c.IfName != "" {
		return c.IfName
	}

	return defaultInterfaceName
}

func (c *VPCTunnelConfig) NSPath() string {
	return c.NetNSPath
}

func (c *VPCTunnelConfig) CNIVersion() string {
	return c.CNISpecVersion
}

func (c *VPCTunnelConfig) PluginName() string {
	return c.CNIPluginName
}
