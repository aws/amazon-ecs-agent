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
package utils

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	netutils "github.com/aws/amazon-ecs-agent/ecs-agent/utils/net"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"

	"github.com/vishvananda/netlink"
)

type IPType int

const (
	IPv4 IPType = iota
	IPv6
)

func IsIPv4(s string) bool {
	parsedIP := net.ParseIP(s)
	return parsedIP != nil && parsedIP.To4() != nil
}

func IsIPv4CIDR(s string) bool {
	ip, _, err := net.ParseCIDR(s)
	if err != nil {
		return false
	}

	return ip.To4() != nil
}

func IsIPv6(s string) bool {
	parsedIP := net.ParseIP(s)
	return parsedIP != nil && parsedIP.To4() == nil
}

func IsIPv6CIDR(s string) bool {
	if !strings.Contains(s, "/") {
		return false
	}

	ip, _, err := net.ParseCIDR(s)
	if err != nil {
		return false
	}

	return ip.To4() == nil
}

// GetDefaultNetworkInterfaces returns network interfaces with the highest priority default routes
// for IPv4 and IPv6. If both default routes use the same interface, only one interface is returned.
// Returns an empty slice if no default routes are found.
func GetDefaultNetworkInterfaces(
	netlinkClient netlinkwrapper.NetLink,
) ([]state.NetworkInterface, error) {
	interfaces := []state.NetworkInterface{}

	defaultIPv4Interface, err := GetDefaultNetworkInterface(netlinkClient, netlink.FAMILY_V4)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if defaultIPv4Interface != nil {
		interfaces = append(interfaces, *defaultIPv4Interface)
	}

	defaultIPv6Interface, err := GetDefaultNetworkInterface(netlinkClient, netlink.FAMILY_V6)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if defaultIPv6Interface != nil {
		interfaces = append(interfaces, *defaultIPv6Interface)
	}

	if len(interfaces) == 2 && interfaces[0].DeviceName == interfaces[1].DeviceName {
		// Both interfaces are the same, so return just one of them
		return []state.NetworkInterface{interfaces[0]}, nil
	}

	return interfaces, nil
}

// GetDefaultNetworkInterface returns the network interface with the highest priority default route
// for the specified IP family, along with its IPv4 and IPv6 addresses.
//
// The ipFamily parameter must be either netlink.FAMILY_V4 or netlink.FAMILY_V6.
// It will return an error if netlink.FAMILY_ALL or any other value is provided.
//
// This function returns:
// - A pointer to state.NetworkInterface containing the interface name and its global IP addresses
// - os.ErrNotExist if no default route is found for the specified IP family
// - An error if there's a problem retrieving the default interface or its IP addresses
//
// The returned NetworkInterface will contain both IPv4 and IPv6 addresses regardless of
// the ipFamily specified, as long as they are configured on the interface.
func GetDefaultNetworkInterface(
	netlinkClient netlinkwrapper.NetLink, ipFamily int,
) (*state.NetworkInterface, error) {
	if ipFamily != netlink.FAMILY_V4 && ipFamily != netlink.FAMILY_V6 {
		return nil, fmt.Errorf("ipFamily must be FAMILY_V4 or FAMILY_V6, got FAMILY_ALL")
	}

	defaultLink, err := netutils.GetDefaultNetworkInterface(netlinkClient, ipFamily)
	if err != nil {
		return nil, err
	}

	ipv4Addrs, ipv6Addrs, err := netutils.GetLinkGlobalIPAddrs(netlinkClient, defaultLink)
	if err != nil {
		return nil, err
	}

	return &state.NetworkInterface{DeviceName: defaultLink.Attrs().Name,
		IPV4Addresses: netutils.StringifyIPAddrs(ipv4Addrs),
		IPV6Addresses: netutils.StringifyIPAddrs(ipv6Addrs),
	}, nil
}
