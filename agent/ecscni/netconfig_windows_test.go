//go:build windows && unit
// +build windows,unit

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
	"encoding/json"
	"testing"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/stretchr/testify/assert"
)

const (
	linkName                = "Ethernet 4"
	validVPCGatewayCIDR     = "10.0.0.1/24"
	validVPCGatewayIPv4Addr = "10.0.0.1"
	validDNSServer          = "10.0.0.2"
	ipv4                    = "10.0.0.120"
	ipv4CIDR                = "10.0.0.120/24"
	mac                     = "02:7b:64:49:b1:40"
	cniMinSupportedVersion  = "1.0.0"
	invalidMACAddress       = "12:34;56-78"
)

func getTaskENI() *apieni.ENI {
	return &apieni.ENI{
		ID:                           "TestENI",
		LinkName:                     linkName,
		MacAddress:                   mac,
		InterfaceAssociationProtocol: apieni.DefaultInterfaceAssociationProtocol,
		SubnetGatewayIPV4Address:     validVPCGatewayCIDR,
		IPV4Addresses: []*apieni.ENIIPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		},
	}
}

func getCNIConfig() *Config {
	return &Config{
		MinSupportedCNIVersion:   cniMinSupportedVersion,
		ContainerID:              containerID,
		BlockInstanceMetadata:    false,
		InstanceENIDNSServerList: []string{validDNSServer},
	}
}

// TestNewVPCENIPluginConfigForTaskNSSetup tests the generated configuration when all parameters are valid.
func TestNewVPCENIPluginConfigForTaskNSSetup(t *testing.T) {
	taskENI := getTaskENI()
	cniConfig := getCNIConfig()
	config, err := NewVPCENIPluginConfigForTaskNSSetup(taskENI, cniConfig)

	netConfig := &VPCENIPluginConfig{}
	json.Unmarshal(config.Bytes, netConfig)

	assert.NoError(t, err)
	assert.EqualValues(t, ECSVPCENIPluginExecutable, config.Network.Type)
	assert.EqualValues(t, cniMinSupportedVersion, config.Network.CNIVersion)
	assert.EqualValues(t, []string{validDNSServer}, netConfig.DNS.Nameservers)
	assert.EqualValues(t, TaskHNSNetworkNamePrefix, config.Network.Name)
	assert.EqualValues(t, []string{ipv4CIDR}, netConfig.ENIIPAddresses)
	assert.EqualValues(t, mac, netConfig.ENIMACAddress)
	assert.EqualValues(t, []string{validVPCGatewayIPv4Addr}, netConfig.GatewayIPAddresses)
	assert.EqualValues(t, linkName, netConfig.ENIName)
	assert.False(t, netConfig.UseExistingNetwork)
	assert.EqualValues(t, cniConfig.BlockInstanceMetadata, netConfig.BlockIMDS)
}

func TestNewVPCENIPluginConfigForTaskNSSetupFailure(t *testing.T) {
	cniConfig := getCNIConfig()
	taskENI := getTaskENI()
	taskENI.MacAddress = invalidMACAddress

	config, err := NewVPCENIPluginConfigForTaskNSSetup(taskENI, cniConfig)

	assert.Nil(t, config)
	assert.Error(t, err)
}

// TestNewVPCENIPluginConfigForECSBridgeSetup tests the generated configuration for ecs-bridge setup.
func TestNewVPCENIPluginConfigForECSBridgeSetup(t *testing.T) {
	cniConfig := getCNIConfig()
	config, err := NewVPCENIPluginConfigForECSBridgeSetup(cniConfig)

	netConfig := &VPCENIPluginConfig{}
	json.Unmarshal(config.Bytes, netConfig)

	assert.NoError(t, err)
	assert.EqualValues(t, ECSVPCENIPluginExecutable, config.Network.Type)
	assert.EqualValues(t, cniMinSupportedVersion, config.Network.CNIVersion)
	assert.EqualValues(t, ECSBridgeNetworkName, config.Network.Name)
	assert.True(t, netConfig.UseExistingNetwork)
	assert.EqualValues(t, cniConfig.BlockInstanceMetadata, netConfig.BlockIMDS)
}
