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
	"net"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultDNS         = "169.254.169.253"
	customDNS          = "10.0.0.2"
	customSearchDomain = "us-west-2.compute.internal"

	linkName                 = "eth1"
	macAddr                  = "02:22:ea:8c:81:dc"
	ipv4Addr                 = "1.2.3.4"
	ipv4Gw                   = "1.2.3.1"
	ipv4SubnetPrefixLength   = "20"
	ipv4Subnet               = "1.2.0.0"
	ipv4AddrWithPrefixLength = ipv4Addr + "/" + ipv4SubnetPrefixLength
	ipv4GwWithPrefixLength   = ipv4Gw + "/" + ipv4SubnetPrefixLength
	ipv4SubnetCIDRBlock      = ipv4Subnet + "/" + ipv4SubnetPrefixLength
	ipv6Addr                 = "abcd:dcba:1234:4321::"
	ipv6SubnetPrefixLength   = "64"
	ipv6SubnetCIDRBlock      = ipv6Addr + "/" + ipv6SubnetPrefixLength
	ipv6AddrWithPrefixLength = ipv6Addr + "/" + ipv6SubnetPrefixLength
	vethPeerInterfaceName    = "veth1-peer"
	v2nVNI                   = "ABCDE"
	v2nDestinationIP         = "10.0.2.129"
	v2nDnsIP                 = "10.3.0.2"
	v2nDnsSearch             = "us-west-2.test.compute.internal"
)

var (
	testENI = &NetworkInterface{
		ID:                           "eni-123",
		InterfaceAssociationProtocol: DefaultInterfaceAssociationProtocol,
		IPV4Addresses: []*IPV4Address{
			{
				Primary: true,
				Address: ipv4Addr,
			},
		},
		IPV6Addresses: []*IPV6Address{
			{
				Address: ipv6Addr,
			},
		},
		SubnetGatewayIPV4Address: ipv4GwWithPrefixLength,
	}
	// validNetInterfacesFunc represents a mock of valid response from net.Interfaces() method.
	validNetInterfacesFunc = func() ([]net.Interface, error) {
		parsedMAC, _ := net.ParseMAC(macAddr)
		return []net.Interface{
			net.Interface{
				Name:         linkName,
				HardwareAddr: parsedMAC,
			},
		}, nil
	}
	// invalidNetInterfacesFunc represents a mock of error response from net.Interfaces() method.
	invalidNetInterfacesFunc = func() ([]net.Interface, error) {
		return nil, errors.New("failed to find interfaces")
	}
)

func TestIsStandardENI(t *testing.T) {
	testCases := []struct {
		protocol   string
		isStandard bool
	}{
		{
			protocol:   "",
			isStandard: true,
		},
		{
			protocol:   DefaultInterfaceAssociationProtocol,
			isStandard: true,
		},
		{
			protocol:   VLANInterfaceAssociationProtocol,
			isStandard: false,
		},
		{
			protocol:   "invalid",
			isStandard: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.protocol, func(t *testing.T) {
			ni := &NetworkInterface{
				InterfaceAssociationProtocol: tc.protocol,
			}
			assert.Equal(t, tc.isStandard, ni.IsStandardENI())
		})
	}
}

func TestGetIPV4Addresses(t *testing.T) {
	assert.Equal(t, []string{ipv4Addr}, testENI.GetIPV4Addresses())
}

func TestGetIPV6Addresses(t *testing.T) {
	assert.Equal(t, []string{ipv6Addr}, testENI.GetIPV6Addresses())
}

func TestGetPrimaryIPv4Address(t *testing.T) {
	assert.Equal(t, ipv4Addr, testENI.GetPrimaryIPv4Address())
}

func TestGetPrimaryIPv4AddressWithPrefixLength(t *testing.T) {
	assert.Equal(t, ipv4AddrWithPrefixLength, testENI.GetPrimaryIPv4AddressWithPrefixLength())
}

func TestGetIPAddressesWithPrefixLength(t *testing.T) {
	assert.Equal(t, []string{ipv4AddrWithPrefixLength, ipv6AddrWithPrefixLength}, testENI.GetIPAddressesWithPrefixLength())
}

func TestGetIPv4SubnetPrefixLength(t *testing.T) {
	assert.Equal(t, ipv4SubnetPrefixLength, testENI.GetIPv4SubnetPrefixLength())
}

func TestGetIPv4SubnetCIDRBlock(t *testing.T) {
	assert.Equal(t, ipv4SubnetCIDRBlock, testENI.GetIPv4SubnetCIDRBlock())
}

func TestGetIPv6SubnetCIDRBlock(t *testing.T) {
	assert.Equal(t, ipv6SubnetCIDRBlock, testENI.GetIPv6SubnetCIDRBlock())
}

func TestGetSubnetGatewayIPv4Address(t *testing.T) {
	assert.Equal(t, ipv4Gw, testENI.GetSubnetGatewayIPv4Address())
}

// TestGetLinkNameSuccess tests the retrieval of ENIs name on the instance.
func TestGetLinkNameSuccess(t *testing.T) {
	netInterfaces = validNetInterfacesFunc
	ni := &NetworkInterface{
		MacAddress: macAddr,
	}

	eniLinkName := ni.GetLinkName()
	assert.EqualValues(t, linkName, eniLinkName)
}

// TestGetLinkNameFailure tests the retrieval of Network Interface Name in case of failure.
func TestGetLinkNameFailure(t *testing.T) {
	netInterfaces = invalidNetInterfacesFunc
	ni := &NetworkInterface{
		MacAddress: macAddr,
	}

	eniLinkName := ni.GetLinkName()
	assert.EqualValues(t, "", eniLinkName)
}

func TestENIToString(t *testing.T) {
	expectedStr := `eni id:eni-123, mac: , hostname: , ipv4addresses: [1.2.3.4], ipv6addresses: [abcd:dcba:1234:4321::], dns: [], dns search: [], gateway ipv4: [1.2.3.1/20][]`
	assert.Equal(t, expectedStr, testENI.String())
}

// TestENIFromACS tests the eni information was correctly read from the acs
func TestENIFromACS(t *testing.T) {
	acsENI := getTestACSENI()
	eni, err := ENIFromACS(acsENI)
	assert.NoError(t, err)
	assert.NotNil(t, eni)
	assert.Equal(t, aws.StringValue(acsENI.Ec2Id), eni.ID)
	assert.Len(t, eni.IPV4Addresses, 1)
	assert.Len(t, eni.GetIPV4Addresses(), 1)
	assert.Equal(t, aws.StringValue(acsENI.Ipv4Addresses[0].PrivateAddress), eni.IPV4Addresses[0].Address)
	assert.Equal(t, aws.BoolValue(acsENI.Ipv4Addresses[0].Primary), eni.IPV4Addresses[0].Primary)
	assert.Equal(t, aws.StringValue(acsENI.MacAddress), eni.MacAddress)
	assert.Len(t, eni.IPV6Addresses, 1)
	assert.Len(t, eni.GetIPV6Addresses(), 1)
	assert.Equal(t, aws.StringValue(acsENI.Ipv6Addresses[0].Address), eni.IPV6Addresses[0].Address)
	assert.Len(t, eni.DomainNameServers, 2)
	assert.Equal(t, defaultDNS, eni.DomainNameServers[0])
	assert.Equal(t, customDNS, eni.DomainNameServers[1])
	assert.Len(t, eni.DomainNameSearchList, 1)
	assert.Equal(t, customSearchDomain, eni.DomainNameSearchList[0])
	assert.Equal(t, aws.StringValue(acsENI.PrivateDnsName), eni.PrivateDNSName)
}

// TestValidateENIFromACS tests the validation of enis from acs
func TestValidateENIFromACS(t *testing.T) {
	acsENI := getTestACSENI()
	err := ValidateENI(acsENI)
	assert.NoError(t, err)

	acsENI.Ipv6Addresses = nil
	err = ValidateENI(acsENI)
	assert.NoError(t, err)

	acsENI.Ipv4Addresses = nil
	err = ValidateENI(acsENI)
	assert.Error(t, err)
}

func TestInvalidENIInterfaceVlanPropertyMissing(t *testing.T) {
	acsENI := &ecsacs.ElasticNetworkInterface{
		InterfaceAssociationProtocol: aws.String(VLANInterfaceAssociationProtocol),
		AttachmentArn:                aws.String("arn"),
		Ec2Id:                        aws.String("ec2id"),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String("ipv4"),
			},
		},
		SubnetGatewayIpv4Address: aws.String(ipv4GwWithPrefixLength),
		Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
			{
				Address: aws.String("ipv6"),
			},
		},
		MacAddress: aws.String("mac"),
	}

	err := ValidateENI(acsENI)
	assert.Error(t, err)

}

func TestInvalidENIInvalidInterfaceAssociationProtocol(t *testing.T) {
	acsENI := &ecsacs.ElasticNetworkInterface{
		InterfaceAssociationProtocol: aws.String("no-eni"),
		AttachmentArn:                aws.String("arn"),
		Ec2Id:                        aws.String("ec2id"),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String("ipv4"),
			},
		},
		SubnetGatewayIpv4Address: aws.String(ipv4GwWithPrefixLength),
		Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
			{
				Address: aws.String("ipv6"),
			},
		},
		MacAddress: aws.String("mac"),
	}
	err := ValidateENI(acsENI)
	assert.Error(t, err)
}

func TestInvalidSubnetGatewayAddress(t *testing.T) {
	acsENI := getTestACSENI()
	acsENI.SubnetGatewayIpv4Address = aws.String(ipv4Addr)
	_, err := ENIFromACS(acsENI)
	assert.Error(t, err)
}

func getTestACSENI() *ecsacs.ElasticNetworkInterface {
	return &ecsacs.ElasticNetworkInterface{
		AttachmentArn: aws.String("arn"),
		Ec2Id:         aws.String("ec2id"),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String(ipv4Addr),
			},
		},
		SubnetGatewayIpv4Address: aws.String(ipv4GwWithPrefixLength),
		Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
			{
				Address: aws.String(ipv6Addr)},
		},
		MacAddress:        aws.String("mac"),
		DomainNameServers: []*string{aws.String(defaultDNS), aws.String(customDNS)},
		DomainName:        []*string{aws.String(customSearchDomain)},
		PrivateDnsName:    aws.String("ip.region.compute.internal"),
	}
}

// TestV2NTunnelFromACS tests the ENI model created from ACS V2N interface payload.
func TestV2NTunnelFromACS(t *testing.T) {
	v2nTunnelACS := &ecsacs.ElasticNetworkInterface{
		DomainNameServers: []*string{
			aws.String(v2nDnsIP),
		},
		DomainName: []*string{
			aws.String(v2nDnsSearch),
		},
		InterfaceTunnelProperties: &ecsacs.NetworkInterfaceTunnelProperties{
			TunnelId:           aws.String(v2nVNI),
			InterfaceIpAddress: aws.String(v2nDestinationIP),
		},
	}

	// Test success case.
	v2nEni, err := v2nTunnelFromACS(v2nTunnelACS)
	require.NoError(t, err)

	assert.Equal(t, V2NInterfaceAssociationProtocol, v2nEni.InterfaceAssociationProtocol)
	assert.Equal(t, DefaultGeneveInterfaceGateway, v2nEni.SubnetGatewayIPV4Address)
	assert.Equal(t, DefaultGeneveInterfaceIPAddress, v2nEni.IPV4Addresses[0].Address)
	assert.Equal(t, v2nDnsIP, v2nEni.DomainNameServers[0])
	assert.Equal(t, v2nDnsSearch, v2nEni.DomainNameSearchList[0])

	assert.Equal(t, v2nVNI, v2nEni.TunnelProperties.ID)
	assert.Equal(t, v2nDestinationIP, v2nEni.TunnelProperties.DestinationIPAddress)

	// Test failure cases.
	v2nTunnelACS.InterfaceTunnelProperties.TunnelId = nil
	_, err = v2nTunnelFromACS(v2nTunnelACS)
	require.Error(t, err)
	require.Equal(t, "tunnel ID not found in payload", err.Error())

	v2nTunnelACS.InterfaceTunnelProperties.TunnelId = aws.String(v2nVNI)
	v2nTunnelACS.InterfaceTunnelProperties.InterfaceIpAddress = nil
	_, err = v2nTunnelFromACS(v2nTunnelACS)
	require.Error(t, err)
	require.Equal(t, "tunnel interface IP not found in payload", err.Error())

	v2nTunnelACS.InterfaceTunnelProperties = nil
	_, err = v2nTunnelFromACS(v2nTunnelACS)
	require.Error(t, err)
	assert.Equal(t, "interface tunnel properties not found in payload", err.Error())
}

// TestVETHPairFromACS tests the ENI model created from ACS VETH interface payload.
// It tests if the ENI model inherits the DNS config data from the peer interface
// and also verifies that an error is returned if the peer interface is also veth.
func TestVETHPairFromACS(t *testing.T) {
	peerInterface := &ecsacs.ElasticNetworkInterface{
		Name:              aws.String(vethPeerInterfaceName),
		DomainNameServers: []*string{aws.String("10.0.23.2")},
		DomainName:        []*string{aws.String("amazon.com")},
	}

	vethACS := &ecsacs.ElasticNetworkInterface{
		InterfaceVethProperties: &ecsacs.NetworkInterfaceVethProperties{
			PeerInterface: aws.String(vethPeerInterfaceName),
		},
	}

	vethInterface, err := vethPairFromACS(vethACS, peerInterface)
	require.NoError(t, err)

	assert.Equal(t, VETHInterfaceAssociationProtocol, vethInterface.InterfaceAssociationProtocol)
	assert.Equal(t, vethPeerInterfaceName, vethInterface.VETHProperties.PeerInterfaceName)
	assert.Equal(t, vethInterface.DomainNameServers[0], "10.0.23.2")
	assert.Equal(t, vethInterface.DomainNameSearchList[0], "amazon.com")

	peerInterface.InterfaceAssociationProtocol = aws.String(VETHInterfaceAssociationProtocol)
	_, err = vethPairFromACS(vethACS, peerInterface)
	require.Error(t, err)
	assert.Equal(t, "peer interface cannot be veth", err.Error())
}
