//go:build linux
// +build linux

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

package net

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"

	"github.com/vishvananda/netlink"
)

// FindLinkByMac returns a link by provided mac address.
func FindLinkByMac(netlinkClient netlinkwrapper.NetLink, mac string) (netlink.Link, error) {
	parsedMac, err := net.ParseMAC(mac)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mac '%s': %w", mac, err)
	}

	links, err := netlinkClient.LinkList()
	if err != nil {
		return nil, fmt.Errorf("failed to list all links: %w", err)
	}

	for _, link := range links {
		attrs := link.Attrs()
		if attrs != nil && bytes.Equal(parsedMac, attrs.HardwareAddr) {
			return link, nil
		}
	}

	return nil, errors.New("link was not found")
}

// HasDefaultRoute checks if the given link has a default route of the given IP family.
// Link provided can be `nil`. In that case this function will look at routes for
// all links on the instance.
func HasDefaultRoute(
	netlinkClient netlinkwrapper.NetLink, link netlink.Link, ipFamily int,
) (bool, error) {
	routes, err := netlinkClient.RouteList(link, ipFamily)
	if err != nil {
		return false, fmt.Errorf("failed to list routes: %w", err)
	}

	for _, route := range routes {
		if isDefaultRoute(route, ipFamily) {
			return true, nil
		}
	}

	return false, nil
}

// isDefaultRoute checks if the provided route is a default route for the specified IP Family.
// A default route is defined here as a route with a Gateway and all IP addresses covered
// in its destination (0.0.0.0/0 or ::/0).
func isDefaultRoute(route netlink.Route, ipFamily int) bool {
	// Must have a gateway
	if route.Gw == nil {
		return false
	}

	// nil destination covers all destinations
	if route.Dst == nil {
		return true
	}

	switch ipFamily {
	case netlink.FAMILY_V4:
		return isFullRangeIPv4(route.Dst)
	case netlink.FAMILY_V6:
		return isFullRangeIPv6(route.Dst)
	default:
		return isFullRangeIPv4(route.Dst) || isFullRangeIPv6(route.Dst)
	}
}

// isFullRangeIPv4 checks if the provided IPNet is '0.0.0.0/0'.
func isFullRangeIPv4(ipnet *net.IPNet) bool {
	mask := ipnet.Mask
	ipv4 := ipnet.IP.To4()
	if ipv4 != nil {
		// IP and mask both should be zero to cover all destinations
		return ipv4.Equal(net.IPv4zero) && len(mask) == net.IPv4len && allZeros(mask)
	}
	return false
}

// isFullRangeIPv6 checks if the provided IPNet is '::/0'.
func isFullRangeIPv6(ipnet *net.IPNet) bool {
	mask := ipnet.Mask
	ipv6 := ipnet.IP.To16()
	if ipv6 != nil {
		// IP and mask both should be zero to cover all destinations
		return ipv6.Equal(net.IPv6zero) && len(mask) == net.IPv6len && allZeros(mask)
	}
	return false
}

// allZeros checks if all bytes in the given net.IPMask are zero.
func allZeros(mask net.IPMask) bool {
	for _, b := range mask {
		if b != 0 {
			return false
		}
	}
	return true
}

// DetermineIPCompatibility returns an object that helps determine IPv4 and IPv6
// compatibility of network interfaces configured via DHCP. The result depends on
// existing default routes for both IPv4 and IPv6.
// Mac address of a network interface can be provided optionally to restrict compatibility
// checks to that particular network interface. If no mac address is provided then
// compatibility checks are performed for all network interfaces on the instance.
func DetermineIPCompatibility(
	nlWrapper netlinkwrapper.NetLink, mac string,
) (ipcompatibility.IPCompatibility, error) {
	// Find link for the mac if provided
	var link netlink.Link
	if mac != "" {
		var err error
		link, err = FindLinkByMac(nlWrapper, mac)
		if err != nil {
			return ipcompatibility.NewIPCompatibility(false, false),
				fmt.Errorf("failed to find link for mac '%s': %w", mac, err)
		}
	}

	// Determine IPv4 compatibility
	ipv4Compatible, err := HasDefaultRoute(nlWrapper, link, netlink.FAMILY_V4)
	if err != nil {
		return ipcompatibility.NewIPCompatibility(false, false),
			fmt.Errorf("failed to determine IPv4 compatibility: %w", err)
	}

	// Determine IPv6 compatibility
	ipv6Compatible, err := HasDefaultRoute(nlWrapper, link, netlink.FAMILY_V6)
	if err != nil {
		return ipcompatibility.NewIPCompatibility(false, false),
			fmt.Errorf("failed to determine IPv6 compatibility: %w", err)
	}

	return ipcompatibility.NewIPCompatibility(ipv4Compatible, ipv6Compatible), nil
}

// GetLoopbackInterface finds and returns the loopback interface.
func GetLoopbackInterface(nlWrapper netlinkwrapper.NetLink) (netlink.Link, error) {
	links, err := nlWrapper.LinkList()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %v", err)
	}

	for _, link := range links {
		if link.Attrs().Flags&net.FlagLoopback != 0 {
			return link, nil
		}
	}

	return nil, fmt.Errorf("no loopback interface found")
}

// GetDefaultNetworkInterface returns the network link with the highest priority default route
// for the specified IP family.
//
// The ipFamily parameter must be either netlink.FAMILY_V4 or netlink.FAMILY_V6.
// Using netlink.FAMILY_ALL or any other value will return an error.
//
// A default route is identified by having:
// - For IPv4: a nil destination or 0.0.0.0/0
// - For IPv6: a nil destination or ::/0
// - A non-nil gateway address
// If multiple default routes exist, the one with the lowest Priority value is selected.
//
// Returns:
// - The network link of the highest priority default route
// - os.ErrNotExist if no default route is found
// - An error if ipFamily is invalid or if there's a problem accessing route information
func GetDefaultNetworkInterface(nlWrapper netlinkwrapper.NetLink, ipFamily int) (netlink.Link, error) {
	if ipFamily != netlink.FAMILY_V4 && ipFamily != netlink.FAMILY_V6 {
		return nil, fmt.Errorf("ipFamily must be FAMILY_V4 or FAMILY_V6, got FAMILY_ALL")
	}

	// Get all routes
	routes, err := nlWrapper.RouteList(nil, ipFamily)
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %w", err)
	}

	// Find the default route with highest priority
	var defaultRoute *netlink.Route
	for _, route := range routes {
		if isDefaultRoute(route, ipFamily) {
			if defaultRoute == nil || route.Priority < defaultRoute.Priority {
				defaultRoute = &route
			}
		}
	}

	if defaultRoute == nil {
		return nil, os.ErrNotExist
	}

	// Get the link associated with the default route
	link, err := nlWrapper.LinkByIndex(defaultRoute.LinkIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get link for default route: %w", err)
	}

	return link, nil
}

// AddrsToIPs translates a slice of netlink.Addrs to net.IPs
func AddrsToIPs(addrs []netlink.Addr) []net.IP {
	ips := []net.IP{}
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	return ips
}

// IsIPv4GlobalUnicast checks if the passed IP is an IPv4 Global Unicast address.
func IsIPv4GlobalUnicast(ipAddr net.IP) bool {
	return ipAddr.IsGlobalUnicast() && ipAddr.To4() != nil
}

// IsIPv6GlobalUnicast checks if the passed IP is an IPv6 Global Unicast address.
func IsIPv6GlobalUnicast(ipAddr net.IP) bool {
	return ipAddr.IsGlobalUnicast() && ipAddr.To4() == nil
}

// StringifyIPAddrs stringifies a slice of IP addresses.
func StringifyIPAddrs(addrs []net.IP) []string {
	var strs []string
	for _, addr := range addrs {
		strs = append(strs, addr.String())
	}
	return strs
}

// GetLinkGlobalIPAddrs returns global IPv4 and IPv6 addresses associated to a link.
func GetLinkGlobalIPAddrs(
	netlinkClient netlinkwrapper.NetLink, link netlink.Link,
) ([]net.IP, []net.IP, error) {
	addrs, err := netlinkClient.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list IP addresses for '%s': %w",
			link.Attrs().Name, err)
	}

	v4Addrs := FilterIPv4GlobalUnicast(AddrsToIPs(addrs))
	v6Addrs := FilterIPv6GlobalUnicast(AddrsToIPs(addrs))

	return v4Addrs, v6Addrs, nil
}

// FilterIPv4GlobalUnicast filters Global Unicast IPv4 addresses.
func FilterIPv4GlobalUnicast(ipAddrs []net.IP) []net.IP {
	ipv4Addrs := []net.IP{}
	for _, ipAddr := range ipAddrs {
		if IsIPv4GlobalUnicast(ipAddr) {
			ipv4Addrs = append(ipv4Addrs, ipAddr)
		}
	}
	return ipv4Addrs
}

// FilterIPv6GlobalUnicast filters Global Unicast IPv6 addresses.
func FilterIPv6GlobalUnicast(ipAddrs []net.IP) []net.IP {
	ipv6Addrs := []net.IP{}
	for _, ipAddr := range ipAddrs {
		if IsIPv6GlobalUnicast(ipAddr) {
			ipv6Addrs = append(ipv6Addrs, ipAddr)
		}
	}
	return ipv6Addrs
}
