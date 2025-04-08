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

package config

import (
	"errors"
	"fmt"
	"net"
	"testing"

	mocknl "github.com/aws/amazon-ecs-agent/agent/utils/netlinkwrapper/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

var linkNotFoundError = errors.New("link was not found")

func TestDetermineIPCompatibility(t *testing.T) {
	// Parse MAC address once
	testMACStr := "00:1A:2B:3C:4D:5E"
	testMAC, err := net.ParseMAC(testMACStr)
	if err != nil {
		t.Fatalf("Failed to parse MAC address: %v", err)
	}
	gw := net.ParseIP("1.2.0.0")

	tests := []struct {
		name          string
		mac           string
		setupMock     func(*mocknl.MockNetLink)
		expectedError error
		expectedIPv4  bool
		expectedIPv6  bool
	}{
		{
			name: "Success - both IPv4 and IPv6 compatible",
			mac:  testMACStr,
			setupMock: func(mock *mocknl.MockNetLink) {
				link := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{HardwareAddr: testMAC}}

				mock.EXPECT().LinkList().Return([]netlink.Link{link}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V4).Return([]netlink.Route{
					{Dst: nil, Gw: gw}, // default route
				}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V6).Return([]netlink.Route{
					{Dst: nil, Gw: gw}, // default route
				}, nil)
			},
			expectedIPv4: true,
			expectedIPv6: true,
		},
		{
			name: "Success - IPv4 only compatible",
			mac:  testMACStr,
			setupMock: func(mock *mocknl.MockNetLink) {
				link := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{HardwareAddr: testMAC}}

				mock.EXPECT().LinkList().Return([]netlink.Link{link}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V4).Return([]netlink.Route{
					{Dst: nil, Gw: gw},
				}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V6).Return([]netlink.Route{}, nil)
			},
			expectedIPv4: true,
			expectedIPv6: false,
		},
		{
			name: "Success - IPv6 only compatible",
			mac:  testMACStr,
			setupMock: func(mock *mocknl.MockNetLink) {
				link := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{HardwareAddr: testMAC}}

				mock.EXPECT().LinkList().Return([]netlink.Link{link}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V4).Return([]netlink.Route{}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V6).Return([]netlink.Route{
					{Dst: nil, Gw: gw}, // default route
				}, nil)
			},
			expectedIPv4: false,
			expectedIPv6: true,
		},
		{
			name: "Error - MAC not found",
			mac:  testMACStr,
			setupMock: func(mock *mocknl.MockNetLink) {
				mock.EXPECT().LinkList().Return([]netlink.Link{}, nil)
			},
			expectedError: fmt.Errorf("failed to find link for mac '%s': %w", testMACStr, linkNotFoundError),
		},
		{
			name: "Error - IPv4 route check fails",
			mac:  testMACStr,
			setupMock: func(mock *mocknl.MockNetLink) {
				link := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{HardwareAddr: testMAC}}

				mock.EXPECT().LinkList().Return([]netlink.Link{link}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V4).Return(
					nil, fmt.Errorf("some error"))
			},
			expectedError: fmt.Errorf("failed to determine IPv4 compatibility: failed to list routes: some error"),
		},
		{
			name: "Error - IPv6 route check fails",
			mac:  testMACStr,
			setupMock: func(mock *mocknl.MockNetLink) {
				link := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{HardwareAddr: testMAC}}

				mock.EXPECT().LinkList().Return([]netlink.Link{link}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V4).Return([]netlink.Route{
					{Dst: nil},
				}, nil)
				mock.EXPECT().RouteList(link, netlink.FAMILY_V6).Return(
					nil, fmt.Errorf("some error"))
			},
			expectedError: fmt.Errorf("failed to determine IPv6 compatibility: failed to list routes: some error"),
		},
		{
			name: "Success - no MAC provided",
			mac:  "",
			setupMock: func(mock *mocknl.MockNetLink) {
				mock.EXPECT().RouteList(nil, netlink.FAMILY_V4).Return([]netlink.Route{
					{Dst: nil, Gw: gw},
				}, nil)
				mock.EXPECT().RouteList(nil, netlink.FAMILY_V6).Return([]netlink.Route{
					{Dst: nil, Gw: gw},
				}, nil)
			},
			expectedIPv4: true,
			expectedIPv6: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockNL := mocknl.NewMockNetLink(ctrl)
			tt.setupMock(mockNL)

			ipv4Compatible, ipv6Compatible, err := determineIPCompatibility(mockNL, tt.mac)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
				assert.False(t, ipv4Compatible)
				assert.False(t, ipv6Compatible)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedIPv4, ipv4Compatible)
				assert.Equal(t, tt.expectedIPv6, ipv6Compatible)
			}
		})
	}
}
