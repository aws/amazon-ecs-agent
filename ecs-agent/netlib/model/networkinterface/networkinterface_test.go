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
