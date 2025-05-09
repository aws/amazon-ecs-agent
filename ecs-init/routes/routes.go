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
	"os"

	netutils "github.com/aws/amazon-ecs-agent/ecs-agent/utils/net"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
	"github.com/cihub/seelog"

	"github.com/vishvananda/netlink"
)

type TMDSIPv6OnlyRouteManager struct{ nl netlinkwrapper.NetLink }

// Equivalent of `ip route add <IP_ADDR> dev lo`.
//
// Creates a local route to route traffic destined to an IP address to loopback interface.
//
// On IPv6-only instances there is no route on the host that would apply to
// non-loopback local traffic such as TMDS. This function helps in such cases
// by creating an applicable route via loopback interface.
func (t *TMDSIPv6OnlyRouteManager) Create(addr net.IP) error {
	hasIPv4DefaultRoutes, err := netutils.HasDefaultRoute(t.nl, nil, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("error when looking up IPv6 default routes: %w", err)
	}
	if hasIPv4DefaultRoutes {
		// Host is not IPv6-only
		return nil
	}

	route, err := getRouteToRedirectToLo(t.nl, addr)
	if err != nil {
		return err
	}

	if err := t.nl.RouteAdd(route); err != nil {
		if os.IsExist(err) {
			seelog.Infof("Route %+v already exists", route)
			return nil
		}
		return fmt.Errorf("error adding route %+v: %w", route, err)
	}

	return nil
}

func (t *TMDSIPv6OnlyRouteManager) Remove(addr net.IP) error {
	hasIPv4DefaultRoutes, err := netutils.HasDefaultRoute(t.nl, nil, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("error when looking up IPv6 default routes: %w", err)
	}
	if hasIPv4DefaultRoutes {
		// Host is not IPv6-only
		return nil
	}

	route, err := getRouteToRedirectToLo(t.nl, addr)
	if err != nil {
		return err
	}

	if err := t.nl.RouteDel(route); err != nil {
		return fmt.Errorf("error deleting route %+v: %w", route, err)
	}
	return nil
}

func getRouteToRedirectToLo(nl netlinkwrapper.NetLink, addr net.IP) (*netlink.Route, error) {
	// Get the loopback interface.
	lo, err := nl.LinkByName("lo")
	if err != nil {
		return nil, fmt.Errorf("error getting lo interface: %v\n", err)
	}

	dst := &net.IPNet{
		IP:   addr,
		Mask: net.CIDRMask(32, 32),
	}

	return &netlink.Route{
		Dst:       dst,
		LinkIndex: lo.Attrs().Index,
		Scope:     netlink.SCOPE_LINK, // Route is local to the link (interface)
	}, nil
}
