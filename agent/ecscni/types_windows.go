// +build windows

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
	"time"

	"github.com/containernetworking/cni/pkg/types"
)

const (
	// ECSVPCENIPluginName is the name of the vpc-eni plugin.
	ECSVPCENIPluginName = "vpc-eni"
	// ECSVPCENIPluginExecutable is the name of vpc-eni executable.
	ECSVPCENIPluginExecutable = "vpc-eni.exe"
	// ECSBridgeNetworkName is the name of the HNS network used as ecs-bridge.
	ECSBridgeNetworkName = "nat"
	// Constants for creating backoff while retrying setupNS.
	setupNSBackoffMin      = time.Second * 4
	setupNSBackoffMax      = time.Minute
	setupNSBackoffJitter   = 0.2
	setupNSBackoffMultiple = 1.3
	setupNSMaxRetryCount   = 3
	// ECSBridgeEndpointNameFormat is the name format of the ecs-bridge endpoint in the task namespace.
	ECSBridgeEndpointNameFormat = "%s-ep-%s"
	// ECSBridgeDefaultRouteDeleteCmdFormat is the format of command for deleting default route of ECS bridge endpoint.
	ECSBridgeDefaultRouteDeleteCmdFormat = `netsh interface ipv4 delete route prefix=0.0.0.0/0 interface="vEthernet (%s)"`
	// ECSBridgeSubnetRouteDeleteCmdFormat is the format of command for deleting subnet route of ECS bridge endpoint.
	ECSBridgeSubnetRouteDeleteCmdFormat = `netsh interface ipv4 delete route prefix=%s interface="vEthernet (%s)"`
	// ECSBridgeCredentialsRouteAddCmdFormat is the format of command for adding route to credentials IP using ECS Bridge.
	ECSBridgeCredentialsRouteAddCmdFormat = `netsh interface ipv4 add route prefix=169.254.170.2/32 interface="vEthernet (%s)"`
	// ValidateExistingFirewallRuleCmdFormat is the format of the command to check if the firewall rule exists.
	ValidateExistingFirewallRuleCmdFormat = `netsh advfirewall firewall show rule name="Disable IMDS for %s" >nul`
	// BlockIMDSFirewallAddRuleCmdFormat is the format of command for creating firewall rule on Windows to block task's IMDS access.
	BlockIMDSFirewallAddRuleCmdFormat = `netsh advfirewall firewall add rule name="Disable IMDS for %s" ` +
		`dir=out localip=%s remoteip=169.254.169.254 action=block`
	// BlockIMDSFirewallDeleteRuleCmdFormat is the format of command to delete firewall rule on Windows to block task's IMDS access.
	BlockIMDSFirewallDeleteRuleCmdFormat = "netsh advfirewall firewall delete rule name=\"Disable IMDS for %s\" dir=out"
)

// VPCENIPluginConfig contains all the information required to invoke the vpc-eni plugin.
type VPCENIPluginConfig struct {
	// Type is the cni plugin name.
	Type string `json:"type,omitempty"`
	// CNIVersion is the cni spec version to use.
	CNIVersion string `json:"cniVersion,omitempty"`
	// DNS is used to pass DNS information to the plugin.
	DNS types.DNS `json:"dns"`

	// ENIName is the name of the eni on the instance.
	ENIName string `json:"eniName"`
	// ENIMACAddress is the MAC address of the eni.
	ENIMACAddress string `json:"eniMACAddress"`
	// ENIIPAddress is the is the ipv4 of eni.
	ENIIPAddress string `json:"eniIPAddress"`
	// GatewayIPAddress specifies the IPv4 address of the subnet gateway for the eni.
	GatewayIPAddress string `json:"gatewayIPAddress"`
	// NoInfraContainer specifies if HCN Namespace is being used for networking setup.
	NoInfraContainer bool `json:"noInfraContainer"`
	// UseExistingNetwork specifies if existing network should be used instead of creating a new one.
	UseExistingNetwork bool `json:"useExistingNetwork"`
}
