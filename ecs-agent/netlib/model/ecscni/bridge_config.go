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
		return DefaultInterfaceName
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
