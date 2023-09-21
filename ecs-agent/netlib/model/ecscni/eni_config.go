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

// ENIConfig contains all the information needed to invoke the eni plugin
type ENIConfig struct {
	CNIConfig
	// ENIID is the id of ec2 eni
	ENIID string `json:"eni"`
	// MacAddress is the mac address of eni
	MACAddress string `json:"mac"`
	// IPAddresses is the set of IP addresses assigned to the ENI.
	IPAddresses []string `json:"ip-addresses"`
	// GatewayIPAddresses is the set of subnet gateway IP addresses for the ENI.
	GatewayIPAddresses []string `json:"gateway-ip-addresses"`
	// BlockInstanceMetadata specifies if InstanceMetadata endpoint should be blocked.
	BlockInstanceMetadata bool `json:"block-instance-metadata"`
	// StayDown specifies if the ENI device should be brought up and configured.
	StayDown bool `json:"stay-down"`
	// DeviceName is the name of the interface will be set inside the namespace
	// this was used as a parameter of the libcni, which is not part of the plugin
	// configuration, thus no need to marshal
	DeviceName string `json:"-"`
	// MTU is the mtu of the eni that should be set if not default value
	MTU int `json:"mtu"`
}

func NewENIConfig(
	cniConfig CNIConfig,
	eni *networkinterface.NetworkInterface,
	blockInstanceMetadata bool,
	stayDown bool,
	mtu int,
) *ENIConfig {
	return &ENIConfig{
		CNIConfig:             cniConfig,
		ENIID:                 eni.ID,
		MACAddress:            eni.MacAddress,
		IPAddresses:           eni.GetIPAddressesWithPrefixLength(),
		GatewayIPAddresses:    []string{eni.GetSubnetGatewayIPv4Address()},
		BlockInstanceMetadata: blockInstanceMetadata,
		StayDown:              stayDown,
		DeviceName:            eni.DeviceName,
		MTU:                   mtu,
	}
}

func (ec *ENIConfig) String() string {
	return fmt.Sprintf("%s, eni: %s, mac: %s, ipAddrs: %v, gateways: %v,"+
		" blockIMDS: %v, stay-down: %t, mtu: %d",
		ec.CNIConfig.String(), ec.ENIID, ec.MACAddress, ec.IPAddresses, ec.GatewayIPAddresses,
		ec.BlockInstanceMetadata, ec.StayDown, ec.MTU)
}

func (ec *ENIConfig) NSPath() string {
	return ec.NetNSPath
}

func (ec *ENIConfig) InterfaceName() string {
	if ec.DeviceName == "" {
		return DefaultENIName
	}
	return ec.DeviceName
}

func (ec *ENIConfig) CNIVersion() string {
	return ec.CNISpecVersion
}

func (ec *ENIConfig) PluginName() string {
	return ec.CNIPluginName
}
