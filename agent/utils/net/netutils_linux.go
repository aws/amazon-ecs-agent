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

	"github.com/aws/amazon-ecs-agent/agent/utils/netlinkwrapper"
	"github.com/vishvananda/netlink"
)

// Finds a link by provided mac address.
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

// Checks if the given link has a default route of the given IP family.
//
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

// Checks if the provided route is a default route for the specified IP Family.
//
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

// Checks if the provided IPNet is 0.0.0.0/0
func isFullRangeIPv4(ipnet *net.IPNet) bool {
	mask := ipnet.Mask
	ipv4 := ipnet.IP.To4()
	if ipv4 != nil {
		// IP and mask both should be zero to cover all destinations
		return ipv4.Equal(net.IPv4zero) && len(mask) == net.IPv4len && allZeros(mask)
	}
	return false
}

// Checks if the provided IPNet is ::/0
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
