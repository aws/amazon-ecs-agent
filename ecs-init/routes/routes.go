// Copyright 2015-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package routes

import (
	"fmt"
	"net"

	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"

	"github.com/vishvananda/netlink"
)

// Equivalent of `ip route add <IP_ADDR> dev lo`.
//
// Creates a local route to route traffic destined to an IP address to loopback interface.
//
// On IPv6-only instances there is no route on the host that would apply to
// non-loopback local traffic such as TMDS. This function helps in such cases
// by creating an applicable route via loopback interface.
func AddRouteToRedirectToLo(nl netlinkwrapper.NetLink, addr net.IP) error {
	// Get the loopback interface.
	lo, err := nl.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("error getting lo interface: %v\n", err)
	}

	// Create the route.
	dst := &net.IPNet{
		IP:   addr,
		Mask: net.CIDRMask(32, 32),
	}
	route := &netlink.Route{
		Dst:       dst,
		LinkIndex: lo.Attrs().Index,
		Scope:     netlink.SCOPE_LINK, // Route is local to the link (interface)
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("error adding route %+v: %w", route, err)
	}
	return nil
}
