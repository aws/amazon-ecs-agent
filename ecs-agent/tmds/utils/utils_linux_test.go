//go:build linux && unit
// +build linux,unit

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
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

func TestGetDefaultNetworkInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNetlink := mock_netlinkwrapper.NewMockNetLink(ctrl)
	link := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: "eth0",
		},
	}

	testCases := []struct {
		name        string
		ipFamily    int
		setupMocks  func(*mock_netlinkwrapper.MockNetLink)
		expected    *state.NetworkInterface
		expectedErr error
	}{
		{
			name:     "success with IPv4 family",
			ipFamily: netlink.FAMILY_V4,
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return([]netlink.Route{
					{LinkIndex: 1, Gw: net.ParseIP("192.168.1.1")},
				}, nil)
				m.EXPECT().LinkByIndex(1).Return(link, nil)
				m.EXPECT().AddrList(link, netlink.FAMILY_ALL).Return([]netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("192.168.1.2")}},
					{IPNet: &net.IPNet{IP: net.ParseIP("2001:db8::2")}},
				}, nil)
			},
			expected: &state.NetworkInterface{
				DeviceName:    "eth0",
				IPV4Addresses: []string{"192.168.1.2"},
				IPV6Addresses: []string{"2001:db8::2"},
			},
		},
		{
			name:     "success with IPv6 family",
			ipFamily: netlink.FAMILY_V6,
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V6).Return([]netlink.Route{
					{LinkIndex: 1, Gw: net.ParseIP("2001:db8::1")},
				}, nil)
				m.EXPECT().LinkByIndex(1).Return(link, nil)
				m.EXPECT().AddrList(link, netlink.FAMILY_ALL).Return([]netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("192.168.1.2")}},
					{IPNet: &net.IPNet{IP: net.ParseIP("2001:db8::2")}},
				}, nil)
			},
			expected: &state.NetworkInterface{
				DeviceName:    "eth0",
				IPV4Addresses: []string{"192.168.1.2"},
				IPV6Addresses: []string{"2001:db8::2"},
			},
		},
		{
			name:     "no default route found",
			ipFamily: netlink.FAMILY_V4,
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return([]netlink.Route{}, nil)
			},
			expectedErr: os.ErrNotExist,
		},
		{
			name:     "error getting addresses",
			ipFamily: netlink.FAMILY_V4,
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return([]netlink.Route{
					{LinkIndex: 1, Gw: net.ParseIP("192.168.1.1")},
				}, nil)
				m.EXPECT().LinkByIndex(1).Return(link, nil)
				m.EXPECT().AddrList(link, netlink.FAMILY_ALL).Return(nil, fmt.Errorf("AddrList failed"))
			},
			expectedErr: fmt.Errorf("failed to list IP addresses for 'eth0': AddrList failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks(mockNetlink)

			result, err := GetDefaultNetworkInterface(mockNetlink, tc.ipFamily)

			if tc.expectedErr != nil {
				assert.Error(t, err)
				if os.IsNotExist(tc.expectedErr) {
					assert.True(t, os.IsNotExist(err))
				} else {
					assert.EqualError(t, err, tc.expectedErr.Error())
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestGetDefaultNetworkInterfaces(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockNetlink := mock_netlinkwrapper.NewMockNetLink(ctrl)
	link1 := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: "eth0",
		},
	}
	link2 := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: "eth1",
		},
	}

	testCases := []struct {
		name        string
		setupMocks  func(*mock_netlinkwrapper.MockNetLink)
		expected    []state.NetworkInterface
		expectedErr string
	}{
		{
			name: "both IPv4 and IPv6 on same interface",
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				// IPv4 setup
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return([]netlink.Route{
					{LinkIndex: 1, Gw: net.ParseIP("192.168.1.1")},
				}, nil)
				m.EXPECT().LinkByIndex(1).Return(link1, nil)
				m.EXPECT().AddrList(link1, netlink.FAMILY_ALL).Return([]netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("192.168.1.2")}},
					{IPNet: &net.IPNet{IP: net.ParseIP("2001:db8::2")}},
				}, nil)

				// IPv6 setup
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V6).Return([]netlink.Route{
					{LinkIndex: 1, Gw: net.ParseIP("2001:db8::1")},
				}, nil)
				m.EXPECT().LinkByIndex(1).Return(link1, nil)
				m.EXPECT().AddrList(link1, netlink.FAMILY_ALL).Return([]netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("192.168.1.2")}},
					{IPNet: &net.IPNet{IP: net.ParseIP("2001:db8::2")}},
				}, nil)
			},
			expected: []state.NetworkInterface{
				{
					DeviceName:    "eth0",
					IPV4Addresses: []string{"192.168.1.2"},
					IPV6Addresses: []string{"2001:db8::2"},
				},
			},
		},
		{
			name: "different interfaces for IPv4 and IPv6",
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				// IPv4 setup
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return([]netlink.Route{
					{LinkIndex: 1, Gw: net.ParseIP("192.168.1.1")},
				}, nil)
				m.EXPECT().LinkByIndex(1).Return(link1, nil)
				m.EXPECT().AddrList(link1, netlink.FAMILY_ALL).Return([]netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("192.168.1.2")}},
				}, nil)

				// IPv6 setup
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V6).Return([]netlink.Route{
					{LinkIndex: 2, Gw: net.ParseIP("2001:db8::1")},
				}, nil)
				m.EXPECT().LinkByIndex(2).Return(link2, nil)
				m.EXPECT().AddrList(link2, netlink.FAMILY_ALL).Return([]netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("2001:db8::2")}},
				}, nil)
			},
			expected: []state.NetworkInterface{
				{
					DeviceName:    "eth0",
					IPV4Addresses: []string{"192.168.1.2"},
				},
				{
					DeviceName:    "eth1",
					IPV6Addresses: []string{"2001:db8::2"},
				},
			},
		},
		{
			name: "only IPv4 default route exists",
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				// IPv4 setup
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return([]netlink.Route{
					{LinkIndex: 1, Gw: net.ParseIP("192.168.1.1")},
				}, nil)
				m.EXPECT().LinkByIndex(1).Return(link1, nil)
				m.EXPECT().AddrList(link1, netlink.FAMILY_ALL).Return([]netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("192.168.1.2")}},
				}, nil)

				// IPv6 no route
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V6).Return([]netlink.Route{}, nil)
			},
			expected: []state.NetworkInterface{
				{
					DeviceName:    "eth0",
					IPV4Addresses: []string{"192.168.1.2"},
				},
			},
		},
		{
			name: "no default routes",
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return([]netlink.Route{}, nil)
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V6).Return([]netlink.Route{}, nil)
			},
			expected: []state.NetworkInterface{},
		},
		{
			name: "error getting IPv4 interface",
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return(nil, fmt.Errorf("RouteList failed"))
			},
			expectedErr: "failed to get routes: RouteList failed",
		},
		{
			name: "error getting IPv6 interface",
			setupMocks: func(m *mock_netlinkwrapper.MockNetLink) {
				// IPv4 setup succeeds
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V4).Return([]netlink.Route{
					{LinkIndex: 1, Gw: net.ParseIP("192.168.1.1")},
				}, nil)
				m.EXPECT().LinkByIndex(1).Return(link1, nil)
				m.EXPECT().AddrList(link1, netlink.FAMILY_ALL).Return([]netlink.Addr{
					{IPNet: &net.IPNet{IP: net.ParseIP("192.168.1.2")}},
				}, nil)

				// IPv6 fails
				m.EXPECT().RouteList(gomock.Any(), netlink.FAMILY_V6).Return(nil, fmt.Errorf("RouteList failed"))
			},
			expectedErr: "failed to get routes: RouteList failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks(mockNetlink)

			result, err := GetDefaultNetworkInterfaces(mockNetlink)

			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}
