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

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
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

	return DefaultInterfaceName
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

// SetV2NDstPortAndDeviceName assigns a destination port to the task ENI and assigns
// it a device name with the pattern gnv<vni><dst port>.
func SetV2NDstPortAndDeviceName(taskENI *networkinterface.NetworkInterface, dstPort uint16) {
	vni := taskENI.TunnelProperties.ID
	taskENI.TunnelProperties.DestinationPort = dstPort

	// Here the device name is set. Although we do not save it right here because it
	// will get saved eventually when the network setup completes and ENI manager
	// transitions to READY_PULL state.
	taskENI.DeviceName = fmt.Sprintf(networkinterface.GeneveInterfaceNamePattern, vni, dstPort)
}
