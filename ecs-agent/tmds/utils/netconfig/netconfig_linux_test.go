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

package netconfig

import (
	"net"
	"testing"

	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

func TestDefaultNetInterfaceName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	_, allIpNet, err := net.ParseCIDR("0.0.0.0/0")
	assert.NoError(t, err)
	_, randomIpNet, err := net.ParseCIDR("192.168.1.0/24")
	assert.NoError(t, err)

	tcs := []struct {
		name                            string
		routes                          []netlink.Route
		link                            netlink.Link
		expectedDefaultNetInterfaceName string
		expectedErrMsg                  string
	}{
		{
			name: "no default route 1",
			routes: []netlink.Route{
				netlink.Route{
					Gw:        nil,
					Dst:       nil,
					LinkIndex: 0,
				},
			},
			link: &netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					Index: 0,
					Name:  "eni-0",
				},
			},
			expectedDefaultNetInterfaceName: "",
			expectedErrMsg:                  "",
		},
		{
			name: "no default route 2",
			routes: []netlink.Route{
				netlink.Route{
					Gw:        net.ParseIP("10.194.20.1"),
					Dst:       randomIpNet,
					LinkIndex: 0,
				},
			},
			link: &netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					Index: 0,
					Name:  "eni-0",
				},
			},
			expectedDefaultNetInterfaceName: "",
			expectedErrMsg:                  "",
		},
		{
			name: "one default route 1",
			routes: []netlink.Route{
				netlink.Route{
					Gw:        net.ParseIP("10.194.20.1"),
					Dst:       nil,
					LinkIndex: 0,
				},
			},
			link: &netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					Index: 0,
					Name:  "eni-0",
				},
			},
			expectedDefaultNetInterfaceName: "eni-0",
			expectedErrMsg:                  "",
		},
		{
			name: "one default route 2",
			routes: []netlink.Route{
				netlink.Route{
					Gw:        net.ParseIP("10.194.20.1"),
					Dst:       allIpNet,
					LinkIndex: 1,
				},
			},
			link: &netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					Index: 1,
					Name:  "eni-1",
				},
			},
			expectedDefaultNetInterfaceName: "eni-1",
			expectedErrMsg:                  "",
		},
		{
			name: "two default routes",
			routes: []netlink.Route{
				netlink.Route{
					Gw:        net.ParseIP("10.194.20.1"),
					Dst:       randomIpNet,
					LinkIndex: 0,
				},
				netlink.Route{
					Gw:        net.ParseIP("10.194.20.1"),
					Dst:       allIpNet,
					LinkIndex: 1,
				},
				netlink.Route{
					Gw:        net.ParseIP("10.194.20.1"),
					Dst:       nil,
					LinkIndex: 2,
				},
			},
			link: &netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					Index: 1,
					Name:  "eni-0",
				},
			},
			expectedDefaultNetInterfaceName: "eni-0",
			expectedErrMsg:                  "",
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			netLink := mock_netlinkwrapper.NewMockNetLink(ctrl)
			gomock.InOrder(
				netLink.EXPECT().RouteList(nil, netlink.FAMILY_ALL).Return(tc.routes, nil).AnyTimes(),
				netLink.EXPECT().LinkByIndex(tc.link.Attrs().Index).Return(tc.link, nil).AnyTimes(),
			)

			defaultNetInterfaceName, err := DefaultNetInterfaceName(netLink)
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			}

			assert.Equal(t, tc.expectedErrMsg, errMsg)
			assert.Equal(t, tc.expectedDefaultNetInterfaceName, defaultNetInterfaceName)
		})
	}
}
