//go:build unit && linux
// +build unit,linux

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
	"errors"
	"net"
	"testing"

	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/agent/utils/netlinkwrapper/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

func TestFindLinkByMac(t *testing.T) {
	// Parse test MAC addresses
	mac1, _ := net.ParseMAC("00:A5:00:00:00:01")
	mac2, _ := net.ParseMAC("00:BC:00:d1:00:02")

	// Create test links
	link1 := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{HardwareAddr: mac1}}
	link2 := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{HardwareAddr: mac2}}

	tests := []struct {
		name         string
		mac          string
		setupMock    func(*mock_netlinkwrapper.MockNetLink)
		expectedErr  error
		expectedLink netlink.Link
	}{
		{
			name: "Success - Found link with matching MAC",
			mac:  mac2.String(),
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().LinkList().Return([]netlink.Link{link1, link2}, nil)
			},
			expectedErr:  nil,
			expectedLink: link2,
		},
		{
			name: "Success - No matching MAC found",
			mac:  "00:00:00:00:00:03",
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().LinkList().Return([]netlink.Link{link1, link2}, nil)
			},
			expectedErr:  errors.New("link was not found"),
			expectedLink: nil,
		},
		{
			name: "Error - LinkList fails",
			mac:  mac1.String(),
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().LinkList().Return(nil, errors.New("link list error"))
			},
			expectedErr:  errors.New("failed to list all links: link list error"),
			expectedLink: nil,
		},
		{
			name: "Success - Empty link list",
			mac:  mac1.String(),
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().LinkList().Return([]netlink.Link{}, nil)
			},
			expectedErr:  errors.New("link was not found"),
			expectedLink: nil,
		},
		{
			name: "Success - Link with nil attrs",
			mac:  mac1.String(),
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				nilAttrsLink := &netlink.Dummy{}
				m.EXPECT().LinkList().Return([]netlink.Link{nilAttrsLink}, nil)
			},
			expectedErr:  errors.New("link was not found"),
			expectedLink: nil,
		},
		{
			name:        "Invalid mac provided",
			mac:         "invalid",
			setupMock:   func(m *mock_netlinkwrapper.MockNetLink) {},
			expectedErr: errors.New("failed to parse mac 'invalid': address invalid: invalid MAC address"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockNetlink := mock_netlinkwrapper.NewMockNetLink(ctrl)
			tt.setupMock(mockNetlink)

			actualLink, err := FindLinkByMac(mockNetlink, tt.mac)

			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
				assert.Nil(t, actualLink)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedLink, actualLink)
			}
		})
	}
}

// Tests for isDefaultRoute and isNotDefaultRoute functions
func TestDefaultRoute(t *testing.T) {
	// Setup some IP addresses for testing
	ipv4Gateway := net.ParseIP("192.168.1.1")
	ipv6Gateway := net.ParseIP("2001:db8::1")

	// Setup some CIDR blocks
	_, ipv4Network, _ := net.ParseCIDR("0.0.0.0/0")
	_, ipv6Network, _ := net.ParseCIDR("::/0")
	_, nonDefaultNetwork, _ := net.ParseCIDR("192.168.1.0/24")

	tests := []struct {
		name     string
		route    netlink.Route
		expected bool
	}{
		{
			name: "IPv4 default route with 0.0.0.0/0",
			route: netlink.Route{
				Gw:  ipv4Gateway,
				Dst: ipv4Network,
			},
			expected: true,
		},
		{
			name: "IPv6 default route with ::/0",
			route: netlink.Route{
				Gw:  ipv6Gateway,
				Dst: ipv6Network,
			},
			expected: true,
		},
		{
			name: "Default route with nil destination",
			route: netlink.Route{
				Gw:  ipv4Gateway,
				Dst: nil,
			},
			expected: true,
		},
		{
			name: "Non-default route - has gateway but specific network",
			route: netlink.Route{
				Gw:  ipv4Gateway,
				Dst: nonDefaultNetwork,
			},
			expected: false,
		},
		{
			name: "Non-default route - no gateway",
			route: netlink.Route{
				Gw:  nil,
				Dst: ipv4Network,
			},
			expected: false,
		},
		{
			name: "Non-default route - no gateway and no destination",
			route: netlink.Route{
				Gw:  nil,
				Dst: nil,
			},
			expected: false,
		},
		{
			name: "Non-default route - no gateway but specific network",
			route: netlink.Route{
				Gw:  nil,
				Dst: nonDefaultNetwork,
			},
			expected: false,
		},
		{
			name: "IPv4 mapped to IPv6",
			route: netlink.Route{
				Gw:  ipv6Gateway,
				Dst: &net.IPNet{IP: net.ParseIP("::ffff:0.0.0.0"), Mask: net.CIDRMask(0, 32)},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDefaultRoute(tt.route)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasDefaultRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup test data
	mockLink := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{}}
	_, ipv4Network, _ := net.ParseCIDR("0.0.0.0/0")
	_, specificNetwork, _ := net.ParseCIDR("192.168.1.0/24")

	// Create some test routes
	ipv4Gateway := net.ParseIP("192.168.1.1")
	defaultRoute := netlink.Route{Dst: ipv4Network, Gw: ipv4Gateway}
	specificRoute := netlink.Route{Dst: specificNetwork}

	tests := []struct {
		name          string
		ipFamily      int
		setupMock     func(*mock_netlinkwrapper.MockNetLink)
		expected      bool
		expectedError error
	}{
		{
			name:     "Success - Has default route",
			ipFamily: netlink.FAMILY_V4,
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(mockLink, netlink.FAMILY_V4).Return(
					[]netlink.Route{specificRoute, defaultRoute}, nil,
				)
			},
			expected:      true,
			expectedError: nil,
		},
		{
			name:     "Success - No default route",
			ipFamily: netlink.FAMILY_V4,
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(mockLink, netlink.FAMILY_V4).Return(
					[]netlink.Route{specificRoute}, nil,
				)
			},
			expected:      false,
			expectedError: nil,
		},
		{
			name:     "Success - Empty route list",
			ipFamily: netlink.FAMILY_V4,
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(mockLink, netlink.FAMILY_V4).Return(
					[]netlink.Route{}, nil,
				)
			},
			expected:      false,
			expectedError: nil,
		},
		{
			name:     "Error - RouteList fails",
			ipFamily: netlink.FAMILY_V4,
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(mockLink, netlink.FAMILY_V4).Return(
					nil, errors.New("route list error"),
				)
			},
			expected:      false,
			expectedError: errors.New("failed to list routes: route list error"),
		},
		{
			name:     "Success - Multiple default routes",
			ipFamily: netlink.FAMILY_V4,
			setupMock: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(mockLink, netlink.FAMILY_V4).Return(
					[]netlink.Route{defaultRoute, defaultRoute, specificRoute}, nil,
				)
			},
			expected:      true,
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockNetlink := mock_netlinkwrapper.NewMockNetLink(ctrl)
			tt.setupMock(mockNetlink)

			actual, err := HasDefaultRoute(mockNetlink, mockLink, tt.ipFamily)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}
