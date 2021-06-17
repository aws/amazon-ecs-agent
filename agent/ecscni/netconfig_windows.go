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
	"github.com/containernetworking/cni/pkg/types"

	"github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/containernetworking/cni/libcni"
	"github.com/pkg/errors"
)

// NewVPCENIPluginConfigForTaskNSSetup is used to create the configuration of vpc-eni plugin for task namespace setup.
func NewVPCENIPluginConfigForTaskNSSetup(eni *eni.ENI, cfg *Config) (*libcni.NetworkConfig, error) {
	// Use the DNS server addresses of the instance ENI it would belong in the same VPC as
	// the task ENI and therefore, have same DNS configuration.
	dns := types.DNS{
		Nameservers: cfg.InstanceENIDNSServerList,
	}

	eniConf := VPCENIPluginConfig{
		Type:               ECSVPCENIPluginName,
		DNS:                dns,
		ENIName:            eni.GetLinkName(),
		ENIMACAddress:      eni.MacAddress,
		ENIIPAddresses:     []string{eni.GetPrimaryIPv4AddressWithPrefixLength()},
		GatewayIPAddresses: []string{eni.GetSubnetGatewayIPv4Address()},
		UseExistingNetwork: false,
	}

	networkConfig, err := newNetworkConfig(eniConf, ECSVPCENIPluginExecutable, cfg.MinSupportedCNIVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create vpc-eni plugin configuration for setting up task network namespace")
	}

	networkConfig.Network.Name = TaskHNSNetworkNamePrefix
	return networkConfig, nil
}

// NewVPCENIPluginConfigForECSBridgeSetup creates the configuration required by vpc-eni plugin to setup ecs-bridge endpoint for the task.
func NewVPCENIPluginConfigForECSBridgeSetup(cfg *Config) (*libcni.NetworkConfig, error) {
	bridgeConf := VPCENIPluginConfig{
		Type:               ECSVPCENIPluginName,
		UseExistingNetwork: true,
	}

	networkConfig, err := newNetworkConfig(bridgeConf, ECSVPCENIPluginExecutable, cfg.MinSupportedCNIVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create vpc-eni plugin configuration for setting up ecs-bridge endpoint of the task")
	}

	networkConfig.Network.Name = ECSBridgeNetworkName
	return networkConfig, nil
}
