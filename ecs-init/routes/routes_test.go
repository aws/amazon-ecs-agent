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
	"testing"

	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

func TestAddRouteToRedirectToLo(t *testing.T) {
	defaultIPAddr := "169.254.170.2"
	defaultLoLink := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Flags: net.FlagLoopback,
		},
	}
	defaultExpectedRoute := &netlink.Route{
		Dst: &net.IPNet{
			IP:   net.ParseIP(defaultIPAddr),
			Mask: net.CIDRMask(32, 32),
		},
		Scope: netlink.SCOPE_LINK,
	}

	testCases := []struct {
		name        string
		ipAddr      string
		mockSetup   func(*mock_netlinkwrapper.MockNetLink)
		expectedErr string
	}{
		{
			name:   "success - route added",
			ipAddr: defaultIPAddr,
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil)
				mock.EXPECT().RouteAdd(defaultExpectedRoute).Return(nil)
			},
		},
		{
			name:   "success - route already exists",
			ipAddr: defaultIPAddr,
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil)
				mock.EXPECT().RouteAdd(defaultExpectedRoute).Return(os.ErrExist)
			},
		},
		{
			name:   "error - failed to get loopback interface",
			ipAddr: defaultIPAddr,
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().LinkList().Return(nil, assert.AnError)
			},
			expectedErr: "error getting lo interface: failed to get network interfaces: " + assert.AnError.Error(),
		},
		{
			name:   "error - failed to add route",
			ipAddr: defaultIPAddr,
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil)
				mock.EXPECT().RouteAdd(defaultExpectedRoute).Return(assert.AnError)
			},
			expectedErr: fmt.Sprintf("error adding route %+v: "+assert.AnError.Error(), defaultExpectedRoute),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockNetLink := mock_netlinkwrapper.NewMockNetLink(ctrl)
			tc.mockSetup(mockNetLink)

			err := AddRouteToRedirectToLo(mockNetLink, net.ParseIP(tc.ipAddr))

			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveRouteToRedirectToLo(t *testing.T) {
	defaultIPAddr := "169.254.170.2"
	defaultLoLink := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Index: 1,
			Flags: net.FlagLoopback,
		},
	}
	defaultExpectedRoute := &netlink.Route{
		Dst: &net.IPNet{
			IP:   net.ParseIP(defaultIPAddr),
			Mask: net.CIDRMask(32, 32),
		},
		LinkIndex: 1,
		Scope:     netlink.SCOPE_LINK,
	}

	testCases := []struct {
		name        string
		ipAddr      string
		mockSetup   func(*mock_netlinkwrapper.MockNetLink)
		expectedErr string
	}{
		{
			name:   "success - route removed",
			ipAddr: defaultIPAddr,
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil)
				mock.EXPECT().RouteDel(defaultExpectedRoute).Return(nil)
			},
		},
		{
			name:   "error - failed to get loopback interface",
			ipAddr: defaultIPAddr,
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().LinkList().Return(nil, assert.AnError)
			},
			expectedErr: "error getting lo interface: failed to get network interfaces: " + assert.AnError.Error(),
		},
		{
			name:   "error - failed to delete route",
			ipAddr: defaultIPAddr,
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().LinkList().Return([]netlink.Link{defaultLoLink}, nil)
				mock.EXPECT().RouteDel(defaultExpectedRoute).Return(assert.AnError)
			},
			expectedErr: fmt.Sprintf("error deleting route %+v: "+assert.AnError.Error(), defaultExpectedRoute),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockNetLink := mock_netlinkwrapper.NewMockNetLink(ctrl)
			tc.mockSetup(mockNetLink)

			err := RemoveRouteToRedirectToLo(mockNetLink, net.ParseIP(tc.ipAddr))

			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateRoute(t *testing.T) {
	testCases := []struct {
		name        string
		mockSetup   func(*mock_netlinkwrapper.MockNetLink)
		expectedErr string
	}{
		{
			name: "success - IPv6-only host, route created",
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				// Simulate IPv6-only host by returning no IPv4 default routes
				mock.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return([]netlink.Route{}, nil)

				// Expect route creation
				mock.EXPECT().LinkList().Return(
					[]netlink.Link{&netlink.Dummy{
						LinkAttrs: netlink.LinkAttrs{Flags: net.FlagLoopback},
					}}, nil)
				mock.EXPECT().RouteAdd(gomock.Any()).Return(nil)
			},
		},
		{
			name: "success - host with IPv4 routes, no-op",
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				// Simulate host with IPv4 default route
				mock.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return([]netlink.Route{
					{Dst: nil, Gw: net.ParseIP("1.2.3.4")}, // default route
				}, nil)

				// Should not call route creation methods
			},
		},
		{
			name: "error - failed to check IPv4 routes",
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return(nil, assert.AnError)
			},
			expectedErr: "error when looking up IPv4 default routes: failed to list routes: " + assert.AnError.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockNetLink := mock_netlinkwrapper.NewMockNetLink(ctrl)
			tc.mockSetup(mockNetLink)

			manager, err := NewTMDSRouteManagerForIPv6Only("169.254.170.2")
			assert.NoError(t, err)
			manager.nl = mockNetLink

			err = manager.CreateRoute()
			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveRoute(t *testing.T) {
	testCases := []struct {
		name        string
		mockSetup   func(*mock_netlinkwrapper.MockNetLink)
		expectedErr string
	}{
		{
			name: "success - IPv6-only host, route removed",
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				// Simulate IPv6-only host by returning no IPv4 default routes
				mock.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return([]netlink.Route{}, nil)

				// Expect route removal
				mock.EXPECT().LinkList().Return([]netlink.Link{&netlink.Dummy{
					LinkAttrs: netlink.LinkAttrs{Flags: net.FlagLoopback},
				}, nil}, nil)
				mock.EXPECT().RouteDel(gomock.Any()).Return(nil)
			},
		},
		{
			name: "success - host with IPv4 routes, no-op",
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				// Simulate host with IPv4 default route
				mock.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return([]netlink.Route{
					{Dst: nil, Gw: net.ParseIP("1.2.3.4")}, // default route
				}, nil)

				// Should not call route removal methods
			},
		},
		{
			name: "error - failed to check IPv4 routes",
			mockSetup: func(mock *mock_netlinkwrapper.MockNetLink) {
				mock.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return(nil, assert.AnError)
			},
			expectedErr: "error when looking up IPv4 default routes: failed to list routes: " + assert.AnError.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockNetLink := mock_netlinkwrapper.NewMockNetLink(ctrl)
			tc.mockSetup(mockNetLink)

			manager, err := NewTMDSRouteManagerForIPv6Only("1.1.1.1")
			assert.NoError(t, err)
			manager.nl = mockNetLink

			err = manager.RemoveRoute()
			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
