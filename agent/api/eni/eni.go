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

package eni

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/cihub/seelog"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

// ENI contains information of the eni
type ENI struct {
	// ID is the id of eni
	ID string `json:"ec2Id"`
	// LinkName is the name of the ENI on the instance.
	// Currently, this field is being used only for Windows and is used during task networking setup.
	LinkName string
	// MacAddress is the mac address of the eni
	MacAddress string
	// IPV4Addresses is the ipv4 address associated with the eni
	IPV4Addresses []*ENIIPV4Address
	// IPV6Addresses is the ipv6 address associated with the eni
	IPV6Addresses []*ENIIPV6Address
	// SubnetGatewayIPV4Address is the IPv4 address of the subnet gateway of the ENI
	SubnetGatewayIPV4Address string `json:",omitempty"`
	// DomainNameServers specifies the nameserver IP addresses for the eni
	DomainNameServers []string `json:",omitempty"`
	// DomainNameSearchList specifies the search list for the domain
	// name lookup, for the eni
	DomainNameSearchList []string `json:",omitempty"`
	// PrivateDNSName is the dns name assigned by the vpc to this eni
	PrivateDNSName string `json:",omitempty"`
	// InterfaceAssociationProtocol is the type of ENI, valid value: "default", "vlan"
	InterfaceAssociationProtocol string `json:",omitempty"`
	// InterfaceVlanProperties contains information for an interface
	// that is supposed to be used as a VLAN device
	InterfaceVlanProperties *InterfaceVlanProperties `json:",omitempty"`

	// Due to historical reasons, the IPv4 subnet prefix length is sent with IPv4 subnet gateway
	// address instead of the ENI's IP addresses. However, CNI plugins and many OS APIs expect it
	// the other way around. Instead of doing this conversion all the time for each CNI and TMDS
	// request, compute it once on demand and cache it here.
	ipv4SubnetPrefixLength string
	ipv4SubnetCIDRBlock    string
	ipv6SubnetCIDRBlock    string

	// guard protects access to fields of this struct.
	guard sync.RWMutex
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

	// IPv6SubnetPrefixLength is the IPv6 global unicast address prefix length, consisting of
	// global routing prefix and subnet ID lengths as specified in IPv6 addressing architecture
	// (RFC 4291 section 2.5.4) and IPv6 Global Unicast Address Format (RFC 3587).
	// The ACS ENI payload structure does not contain an IPv6 subnet prefix length because "/64" is
	// the only allowed length per RFCs above, and the only one that VPC supports.
	IPv6SubnetPrefixLength = "64"
)

var (
	// netInterfaces is the Interfaces() method of net package.
	netInterfaces = net.Interfaces
)

// GetIPV4Addresses returns the list of IPv4 addresses assigned to the ENI.
func (eni *ENI) GetIPV4Addresses() []string {
	var addresses []string
	for _, addr := range eni.IPV4Addresses {
		addresses = append(addresses, addr.Address)
	}

	return addresses
}

// GetIPV6Addresses returns the list of IPv6 addresses assigned to the ENI.
func (eni *ENI) GetIPV6Addresses() []string {
	var addresses []string
	for _, addr := range eni.IPV6Addresses {
		addresses = append(addresses, addr.Address)
	}

	return addresses
}

// GetPrimaryIPv4Address returns the primary IPv4 address assigned to the ENI.
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

// GetPrimaryIPv4AddressWithPrefixLength returns the primary IPv4 address assigned to the ENI with
// its subnet prefix length.
func (eni *ENI) GetPrimaryIPv4AddressWithPrefixLength() string {
	return eni.GetPrimaryIPv4Address() + "/" + eni.GetIPv4SubnetPrefixLength()
}

// GetIPAddressesWithPrefixLength returns the list of all IP addresses assigned to the ENI with
// their subnet prefix length.
func (eni *ENI) GetIPAddressesWithPrefixLength() []string {
	var addresses []string
	for _, addr := range eni.IPV4Addresses {
		addresses = append(addresses, addr.Address+"/"+eni.GetIPv4SubnetPrefixLength())
	}
	for _, addr := range eni.IPV6Addresses {
		addresses = append(addresses, addr.Address+"/"+IPv6SubnetPrefixLength)
	}

	return addresses
}

// GetIPv4SubnetPrefixLength returns the IPv4 prefix length of the ENI's subnet.
func (eni *ENI) GetIPv4SubnetPrefixLength() string {
	if eni.ipv4SubnetPrefixLength == "" && eni.SubnetGatewayIPV4Address != "" {
		eni.ipv4SubnetPrefixLength = strings.Split(eni.SubnetGatewayIPV4Address, "/")[1]
	}

	return eni.ipv4SubnetPrefixLength
}

// GetIPv4SubnetCIDRBlock returns the IPv4 CIDR block, if any, of the ENI's subnet.
func (eni *ENI) GetIPv4SubnetCIDRBlock() string {
	if eni.ipv4SubnetCIDRBlock == "" && eni.SubnetGatewayIPV4Address != "" {
		_, ipv4Net, err := net.ParseCIDR(eni.SubnetGatewayIPV4Address)
		if err == nil {
			eni.ipv4SubnetCIDRBlock = ipv4Net.String()
		}
	}

	return eni.ipv4SubnetCIDRBlock
}

// GetIPv6SubnetCIDRBlock returns the IPv6 CIDR block, if any, of the ENI's subnet.
func (eni *ENI) GetIPv6SubnetCIDRBlock() string {
	if eni.ipv6SubnetCIDRBlock == "" && len(eni.IPV6Addresses) > 0 {
		ipv6Addr := eni.IPV6Addresses[0].Address + "/" + IPv6SubnetPrefixLength
		_, ipv6Net, err := net.ParseCIDR(ipv6Addr)
		if err == nil {
			eni.ipv6SubnetCIDRBlock = ipv6Net.String()
		}
	}

	return eni.ipv6SubnetCIDRBlock
}

// GetSubnetGatewayIPv4Address returns the subnet gateway IPv4 address for the ENI.
func (eni *ENI) GetSubnetGatewayIPv4Address() string {
	var gwAddr string
	if eni.SubnetGatewayIPV4Address != "" {
		gwAddr = strings.Split(eni.SubnetGatewayIPV4Address, "/")[0]
	}

	return gwAddr
}

// GetHostname returns the hostname assigned to the ENI
func (eni *ENI) GetHostname() string {
	return eni.PrivateDNSName
}

// GetLinkName returns the name of the ENI on the instance.
func (eni *ENI) GetLinkName() string {
	eni.guard.Lock()
	defer eni.guard.Unlock()

	if eni.LinkName == "" {
		// Find all interfaces on the instance.
		ifaces, err := netInterfaces()
		if err != nil {
			seelog.Errorf("Failed to find link name: %v.", err)
			return ""
		}
		// Iterate over the list and find the interface with the ENI's MAC address.
		for _, iface := range ifaces {
			if strings.EqualFold(eni.MacAddress, iface.HardwareAddr.String()) {
				eni.LinkName = iface.Name
				break
			}
		}
		// If the ENI is not matched by MAC address above, we will fail to
		// assign the LinkName. Log that here since CNI will fail with the empty
		// name.
		if eni.LinkName == "" {
			seelog.Errorf("Failed to find LinkName for MAC %s", eni.MacAddress)
		}
	}

	return eni.LinkName
}

// IsStandardENI returns true if the ENI is a standard/regular ENI. That is, if it
// has its association protocol as standard. To be backwards compatible, if the
// association protocol is not set for an ENI, it's considered a standard ENI as well.
func (eni *ENI) IsStandardENI() bool {
	switch eni.InterfaceAssociationProtocol {
	case "", DefaultInterfaceAssociationProtocol:
		return true
	case VLANInterfaceAssociationProtocol:
		return false
	default:
		return false
	}
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

	// Read ENI association properties.
	var interfaceVlanProperties InterfaceVlanProperties

	if aws.StringValue(acsENI.InterfaceAssociationProtocol) == VLANInterfaceAssociationProtocol {
		interfaceVlanProperties.TrunkInterfaceMacAddress = aws.StringValue(acsENI.InterfaceVlanProperties.TrunkInterfaceMacAddress)
		interfaceVlanProperties.VlanID = aws.StringValue(acsENI.InterfaceVlanProperties.VlanId)
	}

	eni := &ENI{
		ID:                           aws.StringValue(acsENI.Ec2Id),
		MacAddress:                   aws.StringValue(acsENI.MacAddress),
		IPV4Addresses:                ipv4Addrs,
		IPV6Addresses:                ipv6Addrs,
		SubnetGatewayIPV4Address:     aws.StringValue(acsENI.SubnetGatewayIpv4Address),
		PrivateDNSName:               aws.StringValue(acsENI.PrivateDnsName),
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

	if acsENI.SubnetGatewayIpv4Address == nil {
		return errors.Errorf("eni message validation: no subnet gateway ipv4 address in the message")
	}
	gwIPv4Addr := aws.StringValue(acsENI.SubnetGatewayIpv4Address)
	s := strings.Split(gwIPv4Addr, "/")
	if len(s) != 2 {
		return errors.Errorf(
			"eni message validation: invalid subnet gateway ipv4 address %s", gwIPv4Addr)
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
