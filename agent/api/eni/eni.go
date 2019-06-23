// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package eni

import (
	"fmt"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

// ENI contains information of the eni
type ENI struct {
	// ID is the id of eni
	ID string `json:"ec2Id"`
	// InterfaceAssociationProtocol is the type of ENI, valid value: "default", "vlan"
	InterfaceAssociationProtocol string `json:",omitempty"`
	// IPV4Addresses is the ipv4 address associated with the eni
	IPV4Addresses []*ENIIPV4Address
	// IPV6Addresses is the ipv6 address associated with the eni
	IPV6Addresses []*ENIIPV6Address
	// MacAddress is the mac address of the eni
	MacAddress string
	// DomainNameServers specifies the nameserver IP addresses for the eni
	DomainNameServers []string `json:",omitempty"`
	// DomainNameSearchList specifies the search list for the domain
	// name lookup, for the eni
	DomainNameSearchList []string `json:",omitempty"`
	// InterfaceVlanProperties contains information for an interface
	// that is supposed to be used as a VLAN device
	InterfaceVlanProperties *InterfaceVlanProperties `json:",omitempty"`
	// PrivateDNSName is the dns name assigned by the vpc to this eni
	PrivateDNSName string `json:",omitempty"`
	// SubnetGatewayIPV4Address is the address to the subnet gateway for the eni
	SubnetGatewayIPV4Address string `json:",omitempty"`
}

// InterfaceVlanProperties contains information for an interface that
// is supposed to be used as a VLAN device
type InterfaceVlanProperties struct {
	VlanID                   string
	TrunkInterfaceMacAddress string
}

const (
	// DefaultInterfaceAssociationProtocol represents the standard ENI type.
	DefaultInterfaceAssociationProtocol = "default"

	// VLANInterfaceAssociationProtocol represents the ENI with trunking enabled.
	VLANInterfaceAssociationProtocol = "vlan"
)

// GetIPV4Addresses returns a list of ipv4 addresses allocated to the ENI
func (eni *ENI) GetIPV4Addresses() []string {
	var addresses []string
	for _, addr := range eni.IPV4Addresses {
		addresses = append(addresses, addr.Address)
	}

	return addresses
}

// GetPrimaryIPv4Address returns the primary IPv4 address associated with the ENI.
func (eni *ENI) GetPrimaryIPv4Address() string {
	var primaryAddr string
	for _, addr := range eni.IPV4Addresses {
		if addr.Primary {
			primaryAddr = addr.Address
			break
		}
	}

	return primaryAddr
}

// GetIPV6Addresses returns a list of ipv6 addresses allocated to the ENI
func (eni *ENI) GetIPV6Addresses() []string {
	var addresses []string
	for _, addr := range eni.IPV6Addresses {
		addresses = append(addresses, addr.Address)
	}

	return addresses
}

// GetHostname returns the hostname assigned to the ENI
func (eni *ENI) GetHostname() string {
	return eni.PrivateDNSName
}

// GetSubnetGatewayIPV4Address returns the subnet IPv4 gateway address assigned
// to the ENI
func (eni *ENI) GetSubnetGatewayIPV4Address() string {
	return eni.SubnetGatewayIPV4Address
}

// String returns a human readable version of the ENI object
func (eni *ENI) String() string {
	var ipv4Addresses []string
	for _, addr := range eni.IPV4Addresses {
		ipv4Addresses = append(ipv4Addresses, addr.Address)
	}
	var ipv6Addresses []string
	for _, addr := range eni.IPV6Addresses {
		ipv6Addresses = append(ipv6Addresses, addr.Address)
	}

	eniString := ""

	if len(eni.InterfaceAssociationProtocol) == 0 {
		eniString += fmt.Sprintf(" ,ENI type: [%s]", eni.InterfaceAssociationProtocol)
	}

	if eni.InterfaceVlanProperties != nil {
		eniString += fmt.Sprintf(" ,VLan ID: [%s], TrunkInterfaceMacAddress: [%s]",
			eni.InterfaceVlanProperties.VlanID, eni.InterfaceVlanProperties.TrunkInterfaceMacAddress)
	}

	return fmt.Sprintf(
		"eni id:%s, mac: %s, hostname: %s, ipv4addresses: [%s], ipv6addresses: [%s], dns: [%s], dns search: [%s],"+
			" gateway ipv4: [%s][%s]", eni.ID, eni.MacAddress, eni.GetHostname(), strings.Join(ipv4Addresses, ","),
		strings.Join(ipv6Addresses, ","), strings.Join(eni.DomainNameServers, ","),
		strings.Join(eni.DomainNameSearchList, ","), eni.SubnetGatewayIPV4Address, eniString)
}

// ENIIPV4Address is the ipv4 information of the eni
type ENIIPV4Address struct {
	// Primary indicates whether the ip address is primary
	Primary bool
	// Address is the ipv4 address associated with eni
	Address string
}

// ENIIPV6Address is the ipv6 information of the eni
type ENIIPV6Address struct {
	// Address is the ipv6 address associated with eni
	Address string
}

// ENIFromACS validates the given ACS ENI information and creates an ENI object from it.
func ENIFromACS(acsENI *ecsacs.ElasticNetworkInterface) (*ENI, error) {
	err := ValidateTaskENI(acsENI)
	if err != nil {
		return nil, err
	}

	var ipv4Addrs []*ENIIPV4Address
	var ipv6Addrs []*ENIIPV6Address

	// Read IPv4 address information of the ENI.
	for _, ec2Ipv4 := range acsENI.Ipv4Addresses {
		ipv4Addrs = append(ipv4Addrs, &ENIIPV4Address{
			Primary: aws.BoolValue(ec2Ipv4.Primary),
			Address: aws.StringValue(ec2Ipv4.PrivateAddress),
		})
	}

	// Read IPv6 address information of the ENI.
	for _, ec2Ipv6 := range acsENI.Ipv6Addresses {
		ipv6Addrs = append(ipv6Addrs, &ENIIPV6Address{
			Address: aws.StringValue(ec2Ipv6.Address),
		})
	}

	var interfaceVlanProperties InterfaceVlanProperties

	if aws.StringValue(acsENI.InterfaceAssociationProtocol) == VLANInterfaceAssociationProtocol {
		interfaceVlanProperties.TrunkInterfaceMacAddress = aws.StringValue(acsENI.InterfaceVlanProperties.TrunkInterfaceMacAddress)
		interfaceVlanProperties.VlanID = aws.StringValue(acsENI.InterfaceVlanProperties.VlanId)
	}

	eni := &ENI{
		ID:                           aws.StringValue(acsENI.Ec2Id),
		IPV4Addresses:                ipv4Addrs,
		IPV6Addresses:                ipv6Addrs,
		MacAddress:                   aws.StringValue(acsENI.MacAddress),
		PrivateDNSName:               aws.StringValue(acsENI.PrivateDnsName),
		SubnetGatewayIPV4Address:     aws.StringValue(acsENI.SubnetGatewayIpv4Address),
		InterfaceAssociationProtocol: aws.StringValue(acsENI.InterfaceAssociationProtocol),
		InterfaceVlanProperties:      &interfaceVlanProperties,
	}

	for _, nameserverIP := range acsENI.DomainNameServers {
		eni.DomainNameServers = append(eni.DomainNameServers, aws.StringValue(nameserverIP))
	}
	for _, nameserverDomain := range acsENI.DomainName {
		eni.DomainNameSearchList = append(eni.DomainNameSearchList, aws.StringValue(nameserverDomain))
	}

	return eni, nil
}

// ValidateTaskENI validates the ENI information sent from ACS.
func ValidateTaskENI(acsENI *ecsacs.ElasticNetworkInterface) error {
	// At least one IPv4 address should be associated with the ENI.
	if len(acsENI.Ipv4Addresses) < 1 {
		return errors.Errorf("eni message validation: no ipv4 addresses in the message")
	}

	if acsENI.MacAddress == nil {
		return errors.Errorf("eni message validation: empty eni mac address in the message")
	}

	if acsENI.Ec2Id == nil {
		return errors.Errorf("eni message validation: empty eni id in the message")
	}

	// The association protocol, if specified, must be a supported value.
	if (acsENI.InterfaceAssociationProtocol != nil) &&
		(aws.StringValue(acsENI.InterfaceAssociationProtocol) != VLANInterfaceAssociationProtocol) &&
		(aws.StringValue(acsENI.InterfaceAssociationProtocol) != DefaultInterfaceAssociationProtocol) {
		return errors.Errorf("invalid interface association protocol: %s",
			aws.StringValue(acsENI.InterfaceAssociationProtocol))
	}

	// If the interface association protocol is vlan, InterfaceVlanProperties must be specified.
	if aws.StringValue(acsENI.InterfaceAssociationProtocol) == VLANInterfaceAssociationProtocol {
		if acsENI.InterfaceVlanProperties == nil ||
			len(aws.StringValue(acsENI.InterfaceVlanProperties.VlanId)) == 0 ||
			len(aws.StringValue(acsENI.InterfaceVlanProperties.TrunkInterfaceMacAddress)) == 0 {
			return errors.New("vlan interface properties missing")
		}
	}

	return nil
}
