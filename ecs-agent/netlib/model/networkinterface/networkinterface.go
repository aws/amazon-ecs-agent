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

//lint:file-ignore U1000 Ignore unused fields as some of them are only used by Fargate

package networkinterface

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

// NetworkInterface contains information of the network interface
type NetworkInterface struct {
	// ID is the id of eni
	ID string `json:"ec2Id"`
	// LinkName is the name of the NetworkInterface on the instance.
	// Currently, this field is being used only for Windows and is used during task networking setup.
	LinkName string
	// MacAddress is the mac address of the eni
	MacAddress string
	// IPV4Addresses is the ipv4 address associated with the eni
	IPV4Addresses []*IPV4Address
	// IPV6Addresses is the ipv6 address associated with the eni
	IPV6Addresses []*IPV6Address
	// SubnetGatewayIPV4Address is the IPv4 address of the subnet gateway of the NetworkInterface
	SubnetGatewayIPV4Address string `json:",omitempty"`
	// DomainNameServers specifies the nameserver IP addresses for the eni
	DomainNameServers []string `json:",omitempty"`
	// DomainNameSearchList specifies the search list for the domain
	// name lookup, for the eni
	DomainNameSearchList []string `json:",omitempty"`
	// PrivateDNSName is the dns name assigned by the vpc to this eni
	PrivateDNSName string `json:",omitempty"`
	// InterfaceAssociationProtocol is the type of NetworkInterface, valid value: "default", "vlan"
	InterfaceAssociationProtocol string `json:",omitempty"`

	Index          int64  `json:"Index,omitempty"`
	UserID         uint32 `json:"UserID,omitempty"`
	Name           string `json:"Name,omitempty"`
	NetNSName      string `json:"NetNSName,omitempty"`
	NetNSPath      string `json:"NetNSPath,omitempty"`
	DeviceName     string `json:"DeviceName,omitempty"`
	GuestNetNSName string `json:"GuestNetNSName,omitempty"`
	KnownStatus    Status `json:"KnownStatus,omitempty"`
	DesiredStatus  Status `json:"DesiredStatus,omitempty"`

	// InterfaceVlanProperties contains information for an interface
	// that is supposed to be used as a VLAN device
	InterfaceVlanProperties *InterfaceVlanProperties `json:",omitempty"`
	// TunnelProperties contains information for tunnel interface
	TunnelProperties *TunnelProperties `json:",omitempty"`
	// VETHProperties contains information for a virtual ethernet interface
	VETHProperties *VETHProperties `json:",omitempty"`
	// Certain tasks such as service connect tasks may require additional
	// domain name to IP address mapping defined in their /etc/hosts files.
	// DNSMappingList will contain this for each NetworkInterface since /etc/hosts file
	// is created per NetworkInterface.
	DNSMappingList []DNSMapping

	// Due to historical reasons, the IPv4 subnet prefix length is sent with IPv4 subnet gateway
	// address instead of the NetworkInterface's IP addresses. However, CNI plugins and many OS APIs expect it
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

// TunnelProperties holds ID (e.g. VNI), destination IP address and port for tunnel interfaces.
type TunnelProperties struct {
	ID                   string `json:"ID"`
	DestinationIPAddress string `json:"DestinationIPAddress"`
	DestinationPort      uint16 `json:"DestinationPort"`
}

// VETHProperties holds the properties for virtual ethernet interfaces.
type VETHProperties struct {
	PeerInterfaceName string `json:"PeerInterfaceName"`
}

// DNSMapping holds additional pre-defined DNS entries for containers.
// These additional entries will be written into /etc/hosts file eventually.
type DNSMapping struct {
	Hostname string
	Address  string
}

const (
	// DefaultInterfaceAssociationProtocol represents the standard NetworkInterface type.
	DefaultInterfaceAssociationProtocol = "default"

	// VLANInterfaceAssociationProtocol represents the NetworkInterface with trunking enabled.
	VLANInterfaceAssociationProtocol = "vlan"

	// IPv6SubnetPrefixLength is the IPv6 global unicast address prefix length, consisting of
	// global routing prefix and subnet ID lengths as specified in IPv6 addressing architecture
	// (RFC 4291 section 2.5.4) and IPv6 Global Unicast Address Format (RFC 3587).
	// The ACS NetworkInterface payload structure does not contain an IPv6 subnet prefix length because "/64" is
	// the only allowed length per RFCs above, and the only one that VPC supports.
	IPv6SubnetPrefixLength = "64"

	// TapDeviceNamePrefix holds the name prefix for interfaces attached to a MicroVM.
	// In a multi NetworkInterface task, there will be multiple tap ENIs attached to it.
	// They follow a naming pattern 'eth<eni index>'.
	TapDeviceNamePrefix = "eth"

	// DefaultTapDeviceName is the name of the tap device created by CNI plugin
	// which connects the MicroVM with the branch NetworkInterface.
	DefaultTapDeviceName = TapDeviceNamePrefix + "0"

	// Standard ENIs and branch ENIs are supported both in ECS EC2 instances and Fargate. Therefore
	// the processing of those common interface types are implemented in the ECS Agent's NetworkInterface package
	// (ecseni) and simply imported here. VETH and V2N interface types are supported only in Fargate
	// The constants below and functionality specific to those types are implemented in this file.

	// VETHInterfaceAssociationProtocol is the interface association protocol for veth interfaces.
	VETHInterfaceAssociationProtocol = "veth"

	// V2NInterfaceAssociationProtocol is the interface association protocol for V2N tunnel interfaces.
	V2NInterfaceAssociationProtocol = "tunnel"

	// GeneveInterfaceNamePattern holds pattern of GENEVE interface name:
	// 'gnv<v2nVNI><destination port>'.
	// We have both the VNI and destination port in the name because that is the only
	// guaranteed combination that can make the name of the interface unique.
	// It is important that the name is unique for all GENEVE interfaces because
	// the interface is always first created in the default network namespace
	// before moving it to a custom namespace.
	GeneveInterfaceNamePattern = "gnv%s%d"

	// DefaultGeneveInterfaceIPAddress is the IP address that will be assigned to the
	// GENEVE interface created for the V2N NetworkInterface. These IP addresses are chosen because
	// they come under the ECS reserved link-local IP range. By having the subnet mask as /31,
	// it means there are only 2 available IPs in this chosen subnet - 169.254.175.252 and
	// 169.254.175.253. We set 169.254.175.252 as the geneve interface IP and set
	// 169.254.175.253 as the default default gateway in the routing rules.
	// We also assign a place holder MAC address for the gateway in the ARP table. This
	// configuration ensures all traffic generated in the V2N NetworkInterface's netns will pass through
	// the GENEVE interface.
	DefaultGeneveInterfaceIPAddress = "169.254.175.252"
	DefaultGeneveInterfaceGateway   = "169.254.175.253/31"
)

var (
	// netInterfaces is the Interfaces() method of net package.
	netInterfaces = net.Interfaces
)

// GetIPV4Addresses returns the list of IPv4 addresses assigned to the NetworkInterface.
func (ni *NetworkInterface) GetIPV4Addresses() []string {
	var addresses []string
	for _, addr := range ni.IPV4Addresses {
		addresses = append(addresses, addr.Address)
	}

	return addresses
}

// GetIPV6Addresses returns the list of IPv6 addresses assigned to the NetworkInterface.
func (ni *NetworkInterface) GetIPV6Addresses() []string {
	var addresses []string
	for _, addr := range ni.IPV6Addresses {
		addresses = append(addresses, addr.Address)
	}

	return addresses
}

// GetPrimaryIPv4Address returns the primary IPv4 address assigned to the NetworkInterface.
func (ni *NetworkInterface) GetPrimaryIPv4Address() string {
	var primaryAddr string
	for _, addr := range ni.IPV4Addresses {
		if addr.Primary {
			primaryAddr = addr.Address
			break
		}
	}

	return primaryAddr
}

// GetPrimaryIPv4AddressWithPrefixLength returns the primary IPv4 address assigned to the NetworkInterface with
// its subnet prefix length.
func (ni *NetworkInterface) GetPrimaryIPv4AddressWithPrefixLength() string {
	return ni.GetPrimaryIPv4Address() + "/" + ni.GetIPv4SubnetPrefixLength()
}

// GetIPAddressesWithPrefixLength returns the list of all IP addresses assigned to the NetworkInterface with
// their subnet prefix length.
func (ni *NetworkInterface) GetIPAddressesWithPrefixLength() []string {
	var addresses []string
	for _, addr := range ni.IPV4Addresses {
		addresses = append(addresses, addr.Address+"/"+ni.GetIPv4SubnetPrefixLength())
	}
	for _, addr := range ni.IPV6Addresses {
		addresses = append(addresses, addr.Address+"/"+IPv6SubnetPrefixLength)
	}

	return addresses
}

// GetIPv4SubnetPrefixLength returns the IPv4 prefix length of the NetworkInterface's subnet.
func (ni *NetworkInterface) GetIPv4SubnetPrefixLength() string {
	if ni.ipv4SubnetPrefixLength == "" && ni.SubnetGatewayIPV4Address != "" {
		ni.ipv4SubnetPrefixLength = strings.Split(ni.SubnetGatewayIPV4Address, "/")[1]
	}

	return ni.ipv4SubnetPrefixLength
}

// GetIPv4SubnetCIDRBlock returns the IPv4 CIDR block, if any, of the NetworkInterface's subnet.
func (ni *NetworkInterface) GetIPv4SubnetCIDRBlock() string {
	if ni.ipv4SubnetCIDRBlock == "" && ni.SubnetGatewayIPV4Address != "" {
		_, ipv4Net, err := net.ParseCIDR(ni.SubnetGatewayIPV4Address)
		if err == nil {
			ni.ipv4SubnetCIDRBlock = ipv4Net.String()
		}
	}

	return ni.ipv4SubnetCIDRBlock
}

// GetIPv6SubnetCIDRBlock returns the IPv6 CIDR block, if any, of the NetworkInterface's subnet.
func (ni *NetworkInterface) GetIPv6SubnetCIDRBlock() string {
	if ni.ipv6SubnetCIDRBlock == "" && len(ni.IPV6Addresses) > 0 {
		ipv6Addr := ni.IPV6Addresses[0].Address + "/" + IPv6SubnetPrefixLength
		_, ipv6Net, err := net.ParseCIDR(ipv6Addr)
		if err == nil {
			ni.ipv6SubnetCIDRBlock = ipv6Net.String()
		}
	}

	return ni.ipv6SubnetCIDRBlock
}

// GetSubnetGatewayIPv4Address returns the subnet gateway IPv4 address for the NetworkInterface.
func (ni *NetworkInterface) GetSubnetGatewayIPv4Address() string {
	var gwAddr string
	if ni.SubnetGatewayIPV4Address != "" {
		gwAddr = strings.Split(ni.SubnetGatewayIPV4Address, "/")[0]
	}

	return gwAddr
}

// GetHostname returns the hostname assigned to the NetworkInterface
func (ni *NetworkInterface) GetHostname() string {
	return ni.PrivateDNSName
}

// GetLinkName returns the name of the NetworkInterface on the instance.
func (ni *NetworkInterface) GetLinkName() string {
	ni.guard.Lock()
	defer ni.guard.Unlock()

	if ni.LinkName == "" {
		// Find all interfaces on the instance.
		ifaces, err := netInterfaces()
		if err != nil {
			logger.Error("Failed to find link name:", logger.Fields{
				loggerfield.Error: err,
			})
			return ""
		}
		// Iterate over the list and find the interface with the NetworkInterface's MAC address.
		for _, iface := range ifaces {
			if strings.EqualFold(ni.MacAddress, iface.HardwareAddr.String()) {
				ni.LinkName = iface.Name
				break
			}
		}
		// If the NetworkInterface is not matched by MAC address above, we will fail to
		// assign the LinkName. Log that here since CNI will fail with the empty
		// name.
		if ni.LinkName == "" {
			logger.Error("Failed to find LinkName for given MAC", logger.Fields{
				"mac": ni.MacAddress,
			})
		}
	}

	return ni.LinkName
}

// IsStandardENI returns true if the NetworkInterface is a standard/regular NetworkInterface. That is, if it
// has its association protocol as standard. To be backwards compatible, if the
// association protocol is not set for an NetworkInterface, it's considered a standard NetworkInterface as well.
func (ni *NetworkInterface) IsStandardENI() bool {
	switch ni.InterfaceAssociationProtocol {
	case "", DefaultInterfaceAssociationProtocol:
		return true
	case VLANInterfaceAssociationProtocol:
		return false
	default:
		return false
	}
}

// String returns a human-readable version of the NetworkInterface object
func (ni *NetworkInterface) String() string {
	var ipv4Addresses []string
	for _, addr := range ni.IPV4Addresses {
		ipv4Addresses = append(ipv4Addresses, addr.Address)
	}
	var ipv6Addresses []string
	for _, addr := range ni.IPV6Addresses {
		ipv6Addresses = append(ipv6Addresses, addr.Address)
	}

	eniString := ""

	if len(ni.InterfaceAssociationProtocol) == 0 {
		eniString += fmt.Sprintf(" ,NetworkInterface type: [%s]", ni.InterfaceAssociationProtocol)
	}

	if ni.InterfaceVlanProperties != nil {
		eniString += fmt.Sprintf(" ,VLan ID: [%s], TrunkInterfaceMacAddress: [%s]",
			ni.InterfaceVlanProperties.VlanID, ni.InterfaceVlanProperties.TrunkInterfaceMacAddress)
	}

	return fmt.Sprintf(
		"eni id:%s, mac: %s, hostname: %s, ipv4addresses: [%s], ipv6addresses: [%s], dns: [%s], dns search: [%s],"+
			" gateway ipv4: [%s][%s]", ni.ID, ni.MacAddress, ni.GetHostname(), strings.Join(ipv4Addresses, ","),
		strings.Join(ipv6Addresses, ","), strings.Join(ni.DomainNameServers, ","),
		strings.Join(ni.DomainNameSearchList, ","), ni.SubnetGatewayIPV4Address, eniString)
}

// IPV4Address is the ipv4 information of the eni
type IPV4Address struct {
	// Primary indicates whether the ip address is primary
	Primary bool
	// Address is the ipv4 address associated with eni
	Address string
}

// IPV6Address is the ipv6 information of the eni
type IPV6Address struct {
	// Address is the ipv6 address associated with eni
	Address string
}

// ENIFromACS validates the given ACS NetworkInterface information and creates an NetworkInterface object from it.
func ENIFromACS(acsENI *ecsacs.ElasticNetworkInterface) (*NetworkInterface, error) {
	err := ValidateENI(acsENI)
	if err != nil {
		return nil, err
	}

	var ipv4Addrs []*IPV4Address
	var ipv6Addrs []*IPV6Address

	// Read IPv4 address information of the NetworkInterface.
	for _, ec2Ipv4 := range acsENI.Ipv4Addresses {
		ipv4Addrs = append(ipv4Addrs, &IPV4Address{
			Primary: aws.BoolValue(ec2Ipv4.Primary),
			Address: aws.StringValue(ec2Ipv4.PrivateAddress),
		})
	}

	// Read IPv6 address information of the NetworkInterface.
	for _, ec2Ipv6 := range acsENI.Ipv6Addresses {
		ipv6Addrs = append(ipv6Addrs, &IPV6Address{
			Address: aws.StringValue(ec2Ipv6.Address),
		})
	}

	// Read NetworkInterface association properties.
	var interfaceVlanProperties InterfaceVlanProperties

	if aws.StringValue(acsENI.InterfaceAssociationProtocol) == VLANInterfaceAssociationProtocol {
		interfaceVlanProperties.TrunkInterfaceMacAddress = aws.StringValue(acsENI.InterfaceVlanProperties.TrunkInterfaceMacAddress)
		interfaceVlanProperties.VlanID = aws.StringValue(acsENI.InterfaceVlanProperties.VlanId)
	}

	ni := &NetworkInterface{
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
		ni.DomainNameServers = append(ni.DomainNameServers, aws.StringValue(nameserverIP))
	}
	for _, nameserverDomain := range acsENI.DomainName {
		ni.DomainNameSearchList = append(ni.DomainNameSearchList, aws.StringValue(nameserverDomain))
	}

	return ni, nil
}

// ValidateENI validates the NetworkInterface information sent from ACS.
func ValidateENI(acsENI *ecsacs.ElasticNetworkInterface) error {
	// At least one IPv4 address should be associated with the NetworkInterface.
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

// New creates a new NetworkInterface model.
func New(
	acsENI *ecsacs.ElasticNetworkInterface,
	netNSName string,
	netNSPath string,
	guestNetNSName string,
	peerInterface *ecsacs.ElasticNetworkInterface,
) (*NetworkInterface, error) {
	var err error

	var networkInterface *NetworkInterface

	interfaceAssociationProtocol := aws.StringValue(acsENI.InterfaceAssociationProtocol)

	switch interfaceAssociationProtocol {

	case V2NInterfaceAssociationProtocol:
		// In case of V2N ENIs, network tunnel properties need to be included in the NetworkInterface object.
		// This will be used in creating the GENEVE interface to connect the baremetal host and the BigMac tunnel.
		networkInterface, err = v2nTunnelFromACS(acsENI)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal interface tunnel properties")
		}

	case VETHInterfaceAssociationProtocol:
		networkInterface, err = vethPairFromACS(acsENI, peerInterface)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal interface veth properties")
		}

	// Standard ENIs (Default) and branch ENIs (VLANInterfaceAssociationProtocol) are both processed
	// by the common NetworkInterface handler.
	default:
		// Acquire the NetworkInterface information from the payload.
		networkInterface, err = ENIFromACS(acsENI)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal eni")
		}

		// Historically, if there is no interface association protocol in the NetworkInterface payload, we assume
		// it is a standard NetworkInterface. For more context, see the `IsStandardENI()` method in the `ecseni` model.
		if networkInterface.InterfaceAssociationProtocol == "" {
			networkInterface.InterfaceAssociationProtocol = DefaultInterfaceAssociationProtocol
		}
	}

	networkInterface.Index = aws.Int64Value(acsENI.Index)
	networkInterface.Name = GetENIName(acsENI)
	networkInterface.KnownStatus = StatusNone
	networkInterface.DesiredStatus = StatusReadyPull
	networkInterface.NetNSName = netNSName
	networkInterface.NetNSPath = netNSPath
	networkInterface.GuestNetNSName = guestNetNSName

	return networkInterface, nil
}

// setDeviceName sets the device name for the NetworkInterface based on its type and MAC address.
func (ni *NetworkInterface) setDeviceName(macToName map[string]string) error {
	switch ni.InterfaceAssociationProtocol {
	case DefaultInterfaceAssociationProtocol:
		name, ok := macToName[ni.MacAddress]
		if !ok {
			// This should never happen in theory. ACS can only send the payload message once
			// Agent has acknowledged attachment on the instance. However, we have found that
			// in some instances, EC2 detaches the NetworkInterface after attaching it when there are
			// asynchornous errors in the NetworkInterface attachment workflow. We return a typed error in
			// such scenarios to ensure that we don't get paged when that happens.
			return NewUnableToFindENIError(ni.MacAddress, ni.InterfaceAssociationProtocol)
		}
		ni.DeviceName = name
	case VLANInterfaceAssociationProtocol:
		// We don't need to find the name for a branch NetworkInterface as it's just a vlan association.
		// Get the name of the trunk interface since we need it for constructing the
		// name of the branch interface.
		trunkName, ok := macToName[ni.InterfaceVlanProperties.TrunkInterfaceMacAddress]
		if !ok {
			// Same as above. Guard against edge-cases where we're unable to find the trunk
			// NetworkInterface because of internal EC2 errors.
			return NewUnableToFindENIError(
				ni.InterfaceVlanProperties.TrunkInterfaceMacAddress, ni.InterfaceAssociationProtocol)
		}
		// Name of the branch is based on the vlan id and the name of the trunk.
		// Example: eth1.24, where trunk is attached as `eth1` and vlan id is `24`.
		ni.DeviceName = fmt.Sprintf("%s.%s", trunkName, ni.InterfaceVlanProperties.VlanID)
	default:
		// Do nothing.
	}
	return nil
}

// IsPrimary returns whether the NetworkInterface is the primary NetworkInterface of the task.
func (ni *NetworkInterface) IsPrimary() bool {
	return ni.Index == 0
}

// ShouldGenerateNetworkConfigFiles can be used to check if network configuration files (hosts,
// hostname and resolv.conf) need to be generated using this eni's information. In case of warmpool,
// network config files should only be generated for primary ENIs. But as part of multi-NetworkInterface implementation
// it was decided that for firecracker platform the files had to be generated for secondary ENIs as well.
// Hence the NetworkInterface IsPrimary check was moved from here to warmpool specific APIs.
func (ni *NetworkInterface) ShouldGenerateNetworkConfigFiles() bool {
	return ni.DesiredStatus == StatusReadyPull
}

// GetENIName creates the NetworkInterface name from the NetworkInterface mac address in case it is empty in the ACS payload.
func GetENIName(acsENI *ecsacs.ElasticNetworkInterface) string {
	if acsENI.Name != nil {
		return aws.StringValue(acsENI.Name)
	}

	return strings.ReplaceAll(aws.StringValue(acsENI.MacAddress), ":", "")
}

// v2nTunnelFromACS creates an NetworkInterface model with V2N tunnel properties from the ACS NetworkInterface payload.
func v2nTunnelFromACS(acsENI *ecsacs.ElasticNetworkInterface) (*NetworkInterface, error) {
	// We only require the association protocol and the mac address.
	// Mac address is needed primarily because according to current logic, this mac address is assigned to the
	// interface that gets attached inside the MicroVM. The mac address is also used to find the interface inside
	// the MicroVM to move it to the desired network namespace when the network topology inside the MicroVM is setup.

	acsTunnelProperties := acsENI.InterfaceTunnelProperties
	if acsTunnelProperties == nil {
		return nil, errors.New("interface tunnel properties not found in payload")
	}
	if acsTunnelProperties.TunnelId == nil {
		return nil, errors.New("tunnel ID not found in payload")
	}
	if acsTunnelProperties.InterfaceIpAddress == nil {
		return nil, errors.New("tunnel interface IP not found in payload")
	}

	return &NetworkInterface{
			InterfaceAssociationProtocol: V2NInterfaceAssociationProtocol,
			SubnetGatewayIPV4Address:     DefaultGeneveInterfaceGateway,

			IPV4Addresses: []*IPV4Address{
				{
					Address: DefaultGeneveInterfaceIPAddress,
				},
			},

			DomainNameServers:    aws.StringValueSlice(acsENI.DomainNameServers),
			DomainNameSearchList: aws.StringValueSlice(acsENI.DomainName),
			TunnelProperties: &TunnelProperties{
				ID:                   aws.StringValue(acsTunnelProperties.TunnelId),
				DestinationIPAddress: aws.StringValue(acsTunnelProperties.InterfaceIpAddress),
			},
		},
		nil
}

// vethPairFromACS creates an NetworkInterface model with veth pair properties from the ACS NetworkInterface payload.
func vethPairFromACS(
	acsENI *ecsacs.ElasticNetworkInterface,
	peerInterface *ecsacs.ElasticNetworkInterface) (*NetworkInterface, error) {
	if acsENI.InterfaceVethProperties == nil ||
		acsENI.InterfaceVethProperties.PeerInterface == nil {
		return nil, errors.New("interface veth properties not found in payload")
	}
	if aws.StringValue(peerInterface.InterfaceAssociationProtocol) == VETHInterfaceAssociationProtocol {
		return nil, errors.New("peer interface cannot be veth")
	}

	return &NetworkInterface{
			InterfaceAssociationProtocol: VETHInterfaceAssociationProtocol,

			// DNS related data for VETH interface will be copied from the peer interface's DNS data.
			// This is because if default traffic of the container needs to use the VETH interface,
			// domain name resolution will be based on the DNS config of the peer interface.
			DomainNameServers:    aws.StringValueSlice(peerInterface.DomainNameServers),
			DomainNameSearchList: aws.StringValueSlice(peerInterface.DomainName),
			VETHProperties: &VETHProperties{
				PeerInterfaceName: aws.StringValue(acsENI.InterfaceVethProperties.PeerInterface),
			},
		},
		nil
}
