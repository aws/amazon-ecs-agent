//go:build unit
// +build unit

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
package networkinterface

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

func TestGetSubnetGatewayIPv6Address(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty address",
			input:    "",
			expected: "",
		},
		{
			name:     "ipv6 address with prefix",
			input:    "2001:db8:85a3::8a2e:370:7334/64",
			expected: "2001:db8:85a3::8a2e:370:7334",
		},
		{
			name:     "ipv6 address without prefix",
			input:    "2001:db8:85a3::8a2e:370:7334",
			expected: "2001:db8:85a3::8a2e:370:7334",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ni := &NetworkInterface{SubnetGatewayIPV6Address: tt.input}
			assert.Equal(t, tt.expected, ni.GetSubnetGatewayIPv6Address())
		})
	}
}

func TestValidateENI(t *testing.T) {
	t.Run("IPv6-only ENI with no IPv6 subnet gateway address", func(t *testing.T) {
		eni := &ecsacs.ElasticNetworkInterface{
			Ec2Id:                        aws.String("1"),
			MacAddress:                   aws.String(testconst.RandomMAC),
			InterfaceAssociationProtocol: aws.String(testconst.InterfaceProtocol),
			Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
				{Address: aws.String("1:2:3:4::")},
			},
		}
		err := ValidateENI(eni)
		assert.EqualError(t, err, "eni message validation: no subnet gateway ipv6 address in the message")
	})
	t.Run("Dual stack with no IPv4 subnet gateway address", func(t *testing.T) {
		eni := &ecsacs.ElasticNetworkInterface{
			Ec2Id:                        aws.String("1"),
			MacAddress:                   aws.String(testconst.RandomMAC),
			InterfaceAssociationProtocol: aws.String(testconst.InterfaceProtocol),
			Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
				{PrivateAddress: aws.String("1.2.3.4")},
			},
			Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
				{Address: aws.String("1:2:3:4::")},
			},
		}
		err := ValidateENI(eni)
		assert.EqualError(t, err, "eni message validation: no subnet gateway ipv4 address in the message")
	})
}

func TestGetIPv6SubnetCIDRBlock(t *testing.T) {
	tests := []struct {
		name     string
		ni       *NetworkInterface
		expected string
	}{
		{
			name: "IPv6 only interface with subnet gateway",
			ni: &NetworkInterface{
				IPV6Addresses: []*IPV6Address{
					{Address: "2001:db8:85a3::8a2e:370:7334"},
				},
				SubnetGatewayIPV6Address: "2001:db8:85a3::1/56",
			},
			expected: "2001:db8:85a3::/56",
		},
		{
			name: "IPv6 only interface without subnet gateway",
			ni: &NetworkInterface{
				IPV6Addresses: []*IPV6Address{
					{Address: "2001:db8:85a3::8a2e:370:7334"},
				},
			},
			expected: "2001:db8:85a3::/64", // Uses default prefix length
		},
		{
			name: "Dual-stack interface",
			ni: &NetworkInterface{
				IPV4Addresses: []*IPV4Address{
					{Address: "192.168.1.100"},
				},
				IPV6Addresses: []*IPV6Address{
					{Address: "2001:db8:85a3::8a2e:370:7334"},
				},
			},
			expected: "2001:db8:85a3::/64",
		},
		{
			name: "Dual-stack interface with subnet gateway",
			ni: &NetworkInterface{
				IPV4Addresses: []*IPV4Address{
					{Address: "192.168.1.100"},
				},
				IPV6Addresses: []*IPV6Address{
					{Address: "2001:db8:85a3::8a2e:370:7334"},
				},
				SubnetGatewayIPV6Address: "2001:db8:85a3::1/56",
			},
			expected: "2001:db8:85a3::/64", // Still uses /64 as it's dual-stack
		},
		{
			name: "No IPv6 addresses",
			ni: &NetworkInterface{
				IPV4Addresses: []*IPV4Address{
					{Address: "192.168.1.100"},
				},
			},
			expected: "",
		},
		{
			name: "Invalid IPv6 address",
			ni: &NetworkInterface{
				IPV6Addresses: []*IPV6Address{
					{Address: "invalid_ipv6_address"},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ni.GetIPv6SubnetCIDRBlock()
			if result != tt.expected {
				t.Errorf("GetIPv6SubnetCIDRBlock() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSetDeviceNameVLANInterface(t *testing.T) {
	ni := &NetworkInterface{
		InterfaceAssociationProtocol: VLANInterfaceAssociationProtocol,
		MacAddress:                   "06:9d:f1:5f:c5:83",
		InterfaceVlanProperties: &InterfaceVlanProperties{
			TrunkInterfaceMacAddress: "06:fb:61:8f:0e:63",
			VlanID:                   "1",
		},
	}

	macToName := map[string]string{
		"06:fb:61:8f:0e:63": "eth0", // trunk interface
	}

	err := ni.setDeviceName(macToName)
	assert.NoError(t, err)
	assert.Equal(t, "eth1", ni.DeviceName, "VLAN interface should be normalized to eth1")
}

func TestSetDeviceNameDefaultInterface(t *testing.T) {
	ni := &NetworkInterface{
		InterfaceAssociationProtocol: DefaultInterfaceAssociationProtocol,
		MacAddress:                   "06:9d:f1:5f:c5:83",
	}

	macToName := map[string]string{
		"06:9d:f1:5f:c5:83": "eth0", // interface maps to eth0
	}

	err := ni.setDeviceName(macToName)
	assert.NoError(t, err)
	assert.Equal(t, "eth0", ni.DeviceName, "Default interface should use actual device name from MAC mapping")
}

func TestSetDeviceNameDefaultInterfaceNotFound(t *testing.T) {
	ni := &NetworkInterface{
		InterfaceAssociationProtocol: DefaultInterfaceAssociationProtocol,
		MacAddress:                   "06:9d:f1:5f:c5:83",
	}

	macToName := map[string]string{
		"06:different:mac:address": "eth0", // MAC not matching
	}

	err := ni.setDeviceName(macToName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to find device name")
}

// getTestACSInterface returns a minimal ACS ENI that passes ValidateENI, for tests that only
// care about how InterfaceFromACS handles the DNS fields.
func getTestACSInterface() *ecsacs.ElasticNetworkInterface {
	return &ecsacs.ElasticNetworkInterface{
		Ec2Id:      aws.String("eni-1"),
		MacAddress: aws.String(testconst.RandomMAC),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{Primary: aws.Bool(true), PrivateAddress: aws.String("1.2.3.4")},
		},
		SubnetGatewayIpv4Address: aws.String("1.2.3.1/20"),
	}
}

// TestInterfaceFromACSTrimsWhitespaceFromDomainNameServers tests that surrounding whitespace on a
// nameserver from ACS is stripped. The VPC DHCP option set stores nameservers as typed, so an
// option set configured as "domain-name-servers  10.0.0.2, 10.0.0.3" yields a padded second value.
func TestInterfaceFromACSTrimsWhitespaceFromDomainNameServers(t *testing.T) {
	acsENI := getTestACSInterface()
	acsENI.DomainNameServers = []*string{aws.String("10.0.0.2"), aws.String(" 10.0.0.3 ")}

	ni, err := InterfaceFromACS(acsENI)

	assert.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.2", "10.0.0.3"}, ni.DomainNameServers)
}

// TestInterfaceFromACSTrimsWhitespaceFromDomainNameSearchList tests that surrounding whitespace on
// a search domain from ACS is stripped. Search domains come from the same DHCP option set as the
// nameservers and are stored just as literally.
func TestInterfaceFromACSTrimsWhitespaceFromDomainNameSearchList(t *testing.T) {
	acsENI := getTestACSInterface()
	acsENI.DomainName = []*string{aws.String(" us-west-2.compute.internal ")}

	ni, err := InterfaceFromACS(acsENI)

	assert.NoError(t, err)
	assert.Equal(t, []string{"us-west-2.compute.internal"}, ni.DomainNameSearchList)
}

// TestV2NTunnelFromACSTrimsWhitespaceFromDNSFields tests that a V2N tunnel interface gets the same
// whitespace trimming as a regular interface, since its DNS data comes from the same ACS payload.
func TestV2NTunnelFromACSTrimsWhitespaceFromDNSFields(t *testing.T) {
	acsENI := &ecsacs.ElasticNetworkInterface{
		Ec2Id:             aws.String("eni-1"),
		DomainNameServers: []*string{aws.String(" 10.0.0.2")},
		DomainName:        []*string{aws.String(" us-west-2.compute.internal")},
		InterfaceTunnelProperties: &ecsacs.NetworkInterfaceTunnelProperties{
			TunnelId:           aws.String("42"),
			InterfaceIpAddress: aws.String("10.1.2.3"),
		},
	}

	ni, err := v2nTunnelFromACS(acsENI)

	assert.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.2"}, ni.DomainNameServers)
	assert.Equal(t, []string{"us-west-2.compute.internal"}, ni.DomainNameSearchList)
}

// TestVETHPairFromACSTrimsWhitespaceFromDNSFields tests that a VETH interface gets the same
// whitespace trimming as its peer, whose DNS data it copies from the same ACS payload.
func TestVETHPairFromACSTrimsWhitespaceFromDNSFields(t *testing.T) {
	peer := &ecsacs.ElasticNetworkInterface{
		Ec2Id:             aws.String("eni-1"),
		Name:              aws.String("peer"),
		DomainNameServers: []*string{aws.String(" 10.0.0.2")},
		DomainName:        []*string{aws.String(" us-west-2.compute.internal")},
	}
	acsENI := &ecsacs.ElasticNetworkInterface{
		Ec2Id: aws.String("eni-2"),
		InterfaceVethProperties: &ecsacs.NetworkInterfaceVethProperties{
			PeerInterface: aws.String("peer"),
		},
	}

	ni, err := vethPairFromACS(acsENI, []*ecsacs.ElasticNetworkInterface{peer})

	assert.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.2"}, ni.DomainNameServers)
	assert.Equal(t, []string{"us-west-2.compute.internal"}, ni.DomainNameSearchList)
}
