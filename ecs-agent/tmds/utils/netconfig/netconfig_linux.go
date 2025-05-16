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

package netconfig

import (
	"net"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper"

	"github.com/vishvananda/netlink"
)

type NetworkConfigClient struct {
	NetlinkClient netlinkwrapper.NetLink
}

func NewNetworkConfigClient() *NetworkConfigClient {
	return &NetworkConfigClient{
		NetlinkClient: netlinkwrapper.New(),
	}
}

// DefaultNetInterfaceName returns the device name of the first default network interface
// available on the instance. If none exist, an empty string and nil will be returned.
func DefaultNetInterfaceName(netlinkClient netlinkwrapper.NetLink) (string, error) {
	routes, err := netlinkClient.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return "", err
	}

	// Iterate over all routes
	for _, route := range routes {
		logger.Debug("Found route", logger.Fields{"Route": route})
		if route.Gw == nil {
			// A default route has a gateway. If it doesn't, skip it.
			continue
		}

		if route.Dst == nil || route.Dst.String() == "0.0.0.0/0" || route.Dst.String() == "::/0" {
			// Get the link (interface) associated with the default route
			link, err := netlinkClient.LinkByIndex(route.LinkIndex)
			if err != nil {
				logger.Warn("Not able to get the associated network interface by the index", logger.Fields{
					field.Error: err,
					"LinkIndex": route.LinkIndex,
				})
			} else {
				logger.Debug("Found the associated network interface by the index", logger.Fields{
					"LinkName":  link.Attrs().Name,
					"LinkIndex": route.LinkIndex,
				})
				return link.Attrs().Name, nil
			}
		}
	}
	return "", nil
}

// GetInterfaceGlobalIPAddresses returns all global unicast IP addresses (both IPv4 and IPv6)
// assigned to the given network interface. It excludes link-local, loopback, multicast,
// and unspecified addresses. Returns an empty list if no global unicast addresses are found,
// or an error if the interface cannot be accessed.
func GetInterfaceGlobalIPAddresses(nw netwrapper.Net, ifaceName string) ([]string, error) {
	iface, err := nw.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}

	allAddrs, err := nw.Addrs(iface)
	if err != nil {
		return nil, err
	}

	ipAddrs := make([]string, 0)
	for _, addr := range allAddrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		if ipNet.IP.IsGlobalUnicast() {
			ipAddrs = append(ipAddrs, ipNet.IP.String())
		}
	}
	return ipAddrs, nil
}
