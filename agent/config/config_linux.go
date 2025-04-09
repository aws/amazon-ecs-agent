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

package config

import (
	"fmt"

	netutils "github.com/aws/amazon-ecs-agent/agent/utils/net"
	"github.com/aws/amazon-ecs-agent/agent/utils/netlinkwrapper"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/containerd/cgroups/v3"
	"github.com/vishvananda/netlink"
)

var (
	nlWrapper = netlinkwrapper.New()
)

func init() {
	if cgroups.Mode() == cgroups.Unified {
		CgroupV2 = true
	}
}

// Determines and sets IPv4 and IPv6 compatibility config variables.
//
// This function determines IPv4 and IPv6 compatibility by checking if default routes
// exist for each. Mac address of a network interface can be provided optionally to restrict
// compatibility checks to that particular network interface. If no mac address is provided
// then compatibility checks are performed for all network interfaces on the instance.
func DetermineIPCompatibility(mac string) error {
	instanceIsIPv4Compatible, instanceIsIPv6Compatible, err := determineIPCompatibility(nlWrapper, mac)
	if err != nil {
		return err
	}
	logger.Info("IPv4 and IPv6 compatibility determined", logger.Fields{
		"IPv4": instanceIsIPv4Compatible,
		"IPv6": instanceIsIPv6Compatible,
	})
	return nil
}

func determineIPCompatibility(nlWrapper netlinkwrapper.NetLink, mac string) (bool, bool, error) {
	// Find link for the mac if provided
	var link netlink.Link
	if mac != "" {
		var err error
		link, err = netutils.FindLinkByMac(nlWrapper, mac)
		if err != nil {
			return false, false, fmt.Errorf("failed to find link for mac '%s': %w", mac, err)
		}
	}

	// Determine IPv4 compatibility
	ipv4Compatible, err := netutils.HasDefaultRoute(nlWrapper, link, netlink.FAMILY_V4)
	if err != nil {
		return false, false, fmt.Errorf("failed to determine IPv4 compatibility: %w", err)
	}

	// Determine IPv6 compatibility
	ipv6Compatible, err := netutils.HasDefaultRoute(nlWrapper, link, netlink.FAMILY_V6)
	if err != nil {
		return false, false, fmt.Errorf("failed to determine IPv6 compatibility: %w", err)
	}

	return ipv4Compatible, ipv6Compatible, nil
}
