//go:build !windows
// +build !windows

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

package netlinkwrapper

import (
	"github.com/vishvananda/netlink"
)

type NetLink interface {
	LinkByName(name string) (netlink.Link, error)
	LinkSetUp(link netlink.Link) error
	RouteList(link netlink.Link, family int) ([]netlink.Route, error)
	LinkByIndex(index int) (netlink.Link, error)
	LinkList() ([]netlink.Link, error)
	RouteAdd(route *netlink.Route) error
	RouteDel(route *netlink.Route) error
}

type netLink struct{}

func New() NetLink {
	return &netLink{}
}

func (nl *netLink) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (nl *netLink) LinkSetUp(link netlink.Link) error {
	return netlink.LinkSetUp(link)
}

// RouteList gets a list of routes in the system. Equivalent to: `ip route show`.
// The list can be filtered by link and ip family.
func (nl *netLink) RouteList(link netlink.Link, family int) ([]netlink.Route, error) {
	return netlink.RouteList(link, family)
}

func (nl *netLink) LinkByIndex(index int) (netlink.Link, error) {
	return netlink.LinkByIndex(index)
}

// LinkList gets a list of link devices. Equivalent to: `ip link show`
func (nl *netLink) LinkList() ([]netlink.Link, error) {
	return netlink.LinkList()
}

func (nl *netLink) RouteAdd(route *netlink.Route) error {
	return netlink.RouteAdd(route)
}

func (nl *netLink) RouteDel(route *netlink.Route) error {
	return netlink.RouteDel(route)
}
