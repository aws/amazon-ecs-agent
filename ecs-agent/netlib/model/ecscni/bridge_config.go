package ecscni

import (
	"fmt"
)

// BridgeConfig defines the configuration for bridge plugin
type BridgeConfig struct {
	CNIConfig
	// Name is the name of bridge
	Name string `json:"bridge"`
	// IPAM is the configuration to acquire ip/route from ipam plugin
	IPAM IPAMConfig `json:"ipam,omitempty"`
	// DeviceName is the name of the veth inside the namespace
	// this was used as a parameter of the libcni, thus don't need to be marshalled
	// in the plugin configuration
	DeviceName string `json:"-"`
}

func (bc *BridgeConfig) String() string {
	return fmt.Sprintf("%s, name: %s, ipam: %s", bc.CNIConfig.String(), bc.Name, bc.IPAM.String())
}

// InterfaceName returns the veth pair name will be used inside the namespace
func (bc *BridgeConfig) InterfaceName() string {
	if bc.DeviceName == "" {
		return defaultInterfaceName
	}

	return bc.DeviceName
}

func (bc *BridgeConfig) NSPath() string {
	return bc.NetNSPath
}

func (bc *BridgeConfig) CNIVersion() string {
	return bc.CNISpecVersion
}

func (bc *BridgeConfig) PluginName() string {
	return bc.CNIPluginName
}
