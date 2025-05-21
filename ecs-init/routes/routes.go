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

// Manages routes for TMDS access on IPv6-only hosts.
//
// IPv6-only hosts do not have IPv4 default routes but still need to access TMDS
// over IPv4. So, this manager exposes methods to create routes on the host to redirect
// TMDS traffic to loopback interface over IPv4.
type TMDSRouteManagerForIPv6Only struct {
	nl       netlinkwrapper.NetLink
	tmdsAddr net.IP
}

func NewTMDSRouteManagerForIPv6Only(tmdsAddr string) (*TMDSRouteManagerForIPv6Only, error) {
	addr := net.ParseIP(tmdsAddr)
	if addr == nil {
		return nil, fmt.Errorf("invalid IP address: %s", tmdsAddr)
	}
	return &TMDSRouteManagerForIPv6Only{nl: netlinkwrapper.New(), tmdsAddr: addr}, nil
}

// Creates a route to route TMDS traffic to loopback interface.
// No-op on instances with default IPv4 routes.
func (t *TMDSRouteManagerForIPv6Only) CreateRoute() error {
	hasIPv4DefaultRoutes, err := netutils.HasDefaultRoute(t.nl, nil, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("error when looking up IPv4 default routes: %w", err)
	}
	if hasIPv4DefaultRoutes {
		// Host is not IPv6-only
		return nil
	}

	seelog.Info("Detected IPv6-only instance: adding route to route TMDS traffic through loopback")
	return AddRouteToRedirectToLo(t.nl, t.tmdsAddr)
}

// Removes the route created by CreateTMDSRouteForIPv6Only.
// No-op on instances with default IPv4 routes.
func (t *TMDSRouteManagerForIPv6Only) RemoveRoute() error {
	hasIPv4DefaultRoutes, err := netutils.HasDefaultRoute(t.nl, nil, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("error when looking up IPv4 default routes: %w", err)
	}
	if hasIPv4DefaultRoutes {
		// Host is not IPv6-only
		return nil
	}

	return RemoveRouteToRedirectToLo(t.nl, t.tmdsAddr)
}

// Equivalent of `ip route add <IP_ADDR> dev lo`.
//
// Creates a local route to route traffic destined to an IP address to loopback interface.
//
// On IPv6-only instances there is no route on the host that would apply to
// non-loopback local traffic such as TMDS. This function helps in such cases
// by creating an applicable route via loopback interface.
func AddRouteToRedirectToLo(nl netlinkwrapper.NetLink, addr net.IP) error {
	route, err := getRouteToRedirectToLo(nl, addr)
	if err != nil {
		return err
	}

	if err := nl.RouteAdd(route); err != nil {
		if os.IsExist(err) {
			seelog.Infof("Route %+v already exists", route)
			return nil
		}
		return fmt.Errorf("error adding route %+v: %w", route, err)
	}

	return nil
}

// Removes a route created by AddRouteToRedirectToLo.
func RemoveRouteToRedirectToLo(nl netlinkwrapper.NetLink, addr net.IP) error {
	route, err := getRouteToRedirectToLo(nl, addr)
	if err != nil {
		return err
	}

	if err := nl.RouteDel(route); err != nil {
		return fmt.Errorf("error deleting route %+v: %w", route, err)
	}
	return nil
}

// getRouteToRedirectToLo returns a route that routes traffic via loopback interface
func getRouteToRedirectToLo(nl netlinkwrapper.NetLink, addr net.IP) (*netlink.Route, error) {
	// Get the loopback interface.
	lo, err := netutils.GetLoopbackInterface(nl)
	if err != nil {
		return nil, fmt.Errorf("error getting lo interface: %v", err)
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
