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
	"net"

	"github.com/containernetworking/cni/pkg/types"

	"github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/containernetworking/cni/libcni"
	"github.com/pkg/errors"
)

// NewVPCENIPluginConfigForTaskNSSetup is used to create the configuration of vpc-eni plugin for task namespace setup.
func NewVPCENIPluginConfigForTaskNSSetup(eni *eni.ENI, cfg *Config) (*libcni.NetworkConfig, error) {
	dns := types.DNS{
		Nameservers: eni.DomainNameServers,
	}

	if len(eni.DomainNameServers) == 0 && cfg.PrimaryIPv4VPCCIDR != nil {
		constructedDNS, err := constructDNSFromVPCCIDR(cfg.PrimaryIPv4VPCCIDR)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create vpc-eni network config")
		}
		dns.Nameservers = constructedDNS
	}

	eniConf := VPCENIPluginConfig{
		Type:               ECSVPCENIPluginName,
		DNS:                dns,
		ENIName:            eni.GetLinkName(),
		ENIMACAddress:      eni.MacAddress,
		ENIIPAddress:       eni.GetPrimaryIPv4AddressWithPrefixLength(),
		GatewayIPAddress:   eni.GetSubnetGatewayIPv4Address(),
		NoInfraContainer:   false,
		UseExistingNetwork: false,
	}

	networkConfig, err := newNetworkConfig(eniConf, ECSVPCENIPluginExecutable, cfg.MinSupportedCNIVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create vpc-eni plugin configuration for setting up task network namespace")
	}

	return networkConfig, nil
}

// NewVPCENIPluginConfigForECSBridgeSetup creates the configuration required by vpc-eni plugin to setup ecs-bridge endpoint for the task.
func NewVPCENIPluginConfigForECSBridgeSetup(cfg *Config) (*libcni.NetworkConfig, error) {
	bridgeConf := VPCENIPluginConfig{
		Type:               ECSVPCENIPluginName,
		NoInfraContainer:   false,
		UseExistingNetwork: true,
	}

	networkConfig, err := newNetworkConfig(bridgeConf, ECSVPCENIPluginExecutable, cfg.MinSupportedCNIVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create vpc-eni plugin configuration for setting up ecs-bridge endpoint of the task")
	}

	networkConfig.Network.Name = ECSBridgeNetworkName
	return networkConfig, nil
}

// constructDNSFromVPCCIDR is used to construct DNS server from the primary ipv4 cidr of the vpc.
func constructDNSFromVPCCIDR(vpcCIDR *net.IPNet) ([]string, error) {
	// The DNS server maps to a reserved IP address at the base of the VPC IPv4 network rage plus 2
	// https://docs.aws.amazon.com/vpc/latest/userguide/VPC_DHCP_Options.html#AmazonDNS

	if vpcCIDR == nil {
		return nil, errors.Errorf("unable to contruct dns from invalid vpc cidr")
	}
	mask := net.CIDRMask(24, 32)
	maskedIPv4 := vpcCIDR.IP.Mask(mask).To4()
	maskedIPv4[3] = 2

	return []string{maskedIPv4.String()}, nil
}
