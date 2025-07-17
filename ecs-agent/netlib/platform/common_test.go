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

package platform

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
)

const (
	hostName              = "db.app.com"
	hostName2             = "be.app.com"
	addr                  = "169.254.2.3"
	addr2                 = "169.254.2.4"
	dnsName               = "amazon.com"
	nameServer            = "10.1.0.2"
	nameServer2           = "10.2.0.2"
	ipv4Addr              = "10.1.0.196"
	ipv4Addr2             = "10.2.0.196"
	ipv6Addr              = "fe80::406:baff:fef9:4305"
	ipv6Addr2             = "fe80::406:baff:fef9:3305"
	searchDomainName      = "us-west-2.test.compute.internal"
	searchDomainName2     = "us-west-2.test2.compute.internal"
	trunkENIMac           = "f0:5c:89:a3:ab:03"
	geneveMac             = "f0:5c:89:a3:ab:04"
	deviceName            = "eth1"
	eniMAC                = "f0:5c:89:a3:ab:01"
	subnetGatewayIPv4CIDR = "10.1.0.1/24"
	subnetGatewayIPv6CIDR = "2600:1f14:30ab:6902::/60"
	primaryENIName        = "primary-eni"
	secondaryENIName      = "secondary-eni"
)

func getTestIPv4OnlyInterface() *networkinterface.NetworkInterface {
	return &networkinterface.NetworkInterface{
		PrivateDNSName:    dnsName,
		DomainNameServers: []string{nameServer, nameServer2},
		Default:           true,
		MacAddress:        eniMAC,
		IPV4Addresses: []*networkinterface.IPV4Address{
			{
				Address: ipv4Addr,
				Primary: true,
			},
			{
				Address: ipv4Addr2,
				Primary: false,
			},
		},
		DNSMappingList: []networkinterface.DNSMapping{
			{
				Hostname: hostName,
				Address:  addr,
			},
			{
				Hostname: hostName2,
				Address:  addr2,
			},
		},
		DomainNameSearchList:         []string{searchDomainName, searchDomainName2},
		DeviceName:                   deviceName,
		SubnetGatewayIPV4Address:     subnetGatewayIPv4CIDR,
		InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol,
		KnownStatus:                  status.NetworkNone,
		DesiredStatus:                status.NetworkReadyPull,
		Name:                         primaryENIName,
	}
}

func getTestIPv4OnlyInterfaceWithoutDNS() *networkinterface.NetworkInterface {
	iface := getTestIPv4OnlyInterface()
	iface.DomainNameServers = nil
	iface.DomainNameSearchList = nil
	return iface
}

func getTestIPv6OnlyInterface() *networkinterface.NetworkInterface {
	return &networkinterface.NetworkInterface{
		PrivateDNSName:    dnsName,
		DomainNameServers: []string{nameServer, nameServer2},
		Default:           true,
		MacAddress:        eniMAC,
		IPV6Addresses: []*networkinterface.IPV6Address{
			{
				Address: ipv6Addr,
				Primary: true,
			},
			{
				Address: ipv6Addr2,
				Primary: false,
			},
		},
		DNSMappingList: []networkinterface.DNSMapping{
			{
				Hostname: hostName,
				Address:  addr,
			},
			{
				Hostname: hostName2,
				Address:  addr2,
			},
		},
		DomainNameSearchList:         []string{searchDomainName, searchDomainName2},
		DeviceName:                   deviceName,
		SubnetGatewayIPV6Address:     subnetGatewayIPv6CIDR,
		InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol,
		KnownStatus:                  status.NetworkNone,
		DesiredStatus:                status.NetworkReadyPull,
		Name:                         primaryENIName,
	}
}

func getTestIPv6OnlyInterfaceWithoutDNS() *networkinterface.NetworkInterface {
	iface := getTestIPv6OnlyInterface()
	iface.DomainNameServers = nil
	iface.DomainNameSearchList = nil
	return iface
}

func getTestDualStackInterface() *networkinterface.NetworkInterface {
	return &networkinterface.NetworkInterface{
		PrivateDNSName:    dnsName,
		DomainNameServers: []string{nameServer, nameServer2},
		Default:           true,
		MacAddress:        eniMAC,
		IPV4Addresses: []*networkinterface.IPV4Address{
			{
				Address: ipv4Addr,
				Primary: true,
			},
			{
				Address: ipv4Addr2,
				Primary: false,
			},
		},
		IPV6Addresses: []*networkinterface.IPV6Address{
			{
				Address: ipv6Addr,
				Primary: true,
			},
			{
				Address: ipv6Addr2,
				Primary: false,
			},
		},
		DNSMappingList: []networkinterface.DNSMapping{
			{
				Hostname: hostName,
				Address:  addr,
			},
			{
				Hostname: hostName2,
				Address:  addr2,
			},
		},
		DomainNameSearchList:         []string{searchDomainName, searchDomainName2},
		DeviceName:                   deviceName,
		SubnetGatewayIPV4Address:     subnetGatewayIPv4CIDR,
		SubnetGatewayIPV6Address:     subnetGatewayIPv6CIDR,
		InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol,
		KnownStatus:                  status.NetworkNone,
		DesiredStatus:                status.NetworkReadyPull,
		Name:                         primaryENIName,
	}
}
