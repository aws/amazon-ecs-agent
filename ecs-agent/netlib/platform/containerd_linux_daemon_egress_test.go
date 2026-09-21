//go:build linux && unit

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

package platform

import (
	"net"
	"strings"
	"testing"

	mock_nsutil "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni/mocks_nsutil"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper/mocks"

	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

const (
	testEgressTaskNetNSPath = "/var/run/netns/task-egress"
	testEgressDeviceName    = "eth1"
	// The task's own addresses on the shared daemon bridge: the first
	// dynamic allocation after the daemon's static .2 / ::172:2.
	testTaskBridgeIPv4 = "169.254.172.3"
	testTaskBridgeIPv6 = "fd00:ec2::172:3"
)

// egressNetfilterCall records one iptables/ip6tables invocation.
type egressNetfilterCall struct {
	executable string
	args       string
}

// TestContainerdConfigureDaemonEgressByIPFamily verifies the forwarding
// settings, MASQUERADE rule and daemon default route installed for each
// address family the ENI carries, and that families the ENI lacks are left
// alone.
func TestContainerdConfigureDaemonEgressByIPFamily(t *testing.T) {
	v4Addr := netlink.Addr{IPNet: &net.IPNet{IP: net.ParseIP(testTaskBridgeIPv4), Mask: net.CIDRMask(22, 32)}}
	v6Addr := netlink.Addr{IPNet: &net.IPNet{IP: net.ParseIP(testTaskBridgeIPv6), Mask: net.CIDRMask(112, 128)}}

	tests := []struct {
		name       string
		ipv4       bool
		ipv6       bool
		wantV4NAT  bool
		wantV6NAT  bool
		wantV4Gw   string
		wantV6Gw   string
		wantSysctl []string
	}{
		{
			name: "ipv4-only", ipv4: true,
			wantV4NAT: true, wantV4Gw: testTaskBridgeIPv4,
			wantSysctl: []string{ipv4ForwardingKey},
		},
		{
			name: "ipv6-only", ipv6: true,
			wantV6NAT: true, wantV6Gw: testTaskBridgeIPv6,
			wantSysctl: []string{ipv6ForwardingKey},
		},
		{
			name: "dual-stack", ipv4: true, ipv6: true,
			wantV4NAT: true, wantV6NAT: true,
			wantV4Gw: testTaskBridgeIPv4, wantV6Gw: testTaskBridgeIPv6,
			wantSysctl: []string{ipv4ForwardingKey, ipv6ForwardingKey},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			nsUtil := mock_nsutil.NewMockNetNSUtil(ctrl)
			nl := mock_netlinkwrapper.NewMockNetLink(ctrl)

			// Both namespaces are entered for real: the closures do the work.
			nsUtil.EXPECT().ExecInNSPath(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ string, fn func(cnins.NetNS) error) error { return fn(nil) }).AnyTimes()
			nsUtil.EXPECT().GetNetNSPath(daemonBridgeNetNSName).Return(testDaemonNetNSPath).AnyTimes()

			// The task namespace holds one link carrying the task's bridge
			// address(es); the daemon namespace holds the bridge veth.
			taskLink := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Index: 7, Name: "ecs-eth0"}}
			daemonLink := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Index: 11, Name: "eth0"}}
			nl.EXPECT().LinkList().Return([]netlink.Link{taskLink}, nil).AnyTimes()
			nl.EXPECT().AddrList(taskLink, netlink.FAMILY_V4).DoAndReturn(
				func(netlink.Link, int) ([]netlink.Addr, error) {
					if tc.ipv4 {
						return []netlink.Addr{v4Addr}, nil
					}
					return nil, nil
				}).AnyTimes()
			nl.EXPECT().AddrList(taskLink, netlink.FAMILY_V6).DoAndReturn(
				func(netlink.Link, int) ([]netlink.Addr, error) {
					if tc.ipv6 {
						return []netlink.Addr{v6Addr}, nil
					}
					return nil, nil
				}).AnyTimes()
			nl.EXPECT().LinkByName("eth0").Return(daemonLink, nil).AnyTimes()

			var routes []*netlink.Route
			nl.EXPECT().RouteReplace(gomock.Any()).DoAndReturn(func(r *netlink.Route) error {
				routes = append(routes, r)
				return nil
			}).AnyTimes()

			var netfilter []egressNetfilterCall
			var sysctls []string
			origIptables, origSysctl := runIptablesCommand, runSysctlCommand
			defer func() { runIptablesCommand, runSysctlCommand = origIptables, origSysctl }()
			runIptablesCommand = func(executable string, args ...string) ([]byte, error) {
				joined := strings.Join(args, " ")
				// A check reports "absent" so the append is exercised.
				if strings.Contains(joined, " -C ") || strings.HasPrefix(joined, "-C ") {
					return nil, assert.AnError
				}
				netfilter = append(netfilter, egressNetfilterCall{executable: executable, args: joined})
				return nil, nil
			}
			runSysctlCommand = func(args ...string) ([]byte, error) {
				sysctls = append(sysctls, strings.Join(args, " "))
				return nil, nil
			}

			c := &containerd{common: common{nsUtil: nsUtil, netlink: nl}}
			eni := &networkinterface.NetworkInterface{DeviceName: testEgressDeviceName}
			if tc.ipv4 {
				eni.IPV4Addresses = []*networkinterface.IPV4Address{{Address: "10.0.0.5", Primary: true}}
			}
			if tc.ipv6 {
				eni.IPV6Addresses = []*networkinterface.IPV6Address{{Address: "2001:db8::5", Primary: true}}
			}

			c.configureDaemonEgress(testEgressTaskNetNSPath, eni)

			// Forwarding is enabled only for the families the ENI carries.
			for _, key := range tc.wantSysctl {
				assert.Truef(t, containsSubstring(sysctls, key+"=1"), "forwarding must be enabled for %s; sysctls: %v", key, sysctls)
			}
			if !tc.ipv4 {
				assert.Falsef(t, containsSubstring(sysctls, ipv4ForwardingKey), "no IPv4 forwarding on an IPv6-only ENI: %v", sysctls)
			}
			if !tc.ipv6 {
				assert.Falsef(t, containsSubstring(sysctls, ipv6ForwardingKey), "no IPv6 forwarding on an IPv4-only ENI: %v", sysctls)
			}
			// Only interfaces present in the task namespace are touched.
			assert.Falsef(t, containsSubstring(sysctls, "conf.fargate-bridge."), "fargate-bridge is not in the task namespace: %v", sysctls)
			assert.Falsef(t, containsSubstring(sysctls, "conf.eth0."), "eth0 is not the task's device here: %v", sysctls)
			if tc.ipv6 {
				assert.Truef(t, containsSubstring(sysctls, "net.ipv6.conf.default.forwarding=1"), "new interfaces must forward IPv6: %v", sysctls)
				assert.Truef(t, containsSubstring(sysctls, "net.ipv6.conf."+testEgressDeviceName+".forwarding=1"), "the ENI must forward IPv6: %v", sysctls)
			}

			// One MASQUERADE per family out the ENI.
			var v4NAT, v6NAT []string
			for _, call := range netfilter {
				switch call.executable {
				case iptablesExecutable:
					v4NAT = append(v4NAT, call.args)
				case ipv6Tables:
					v6NAT = append(v6NAT, call.args)
				default:
					t.Fatalf("unexpected netfilter executable %q", call.executable)
				}
			}
			if tc.wantV4NAT {
				require.Len(t, v4NAT, 1, "exactly one IPv4 MASQUERADE; got %v", v4NAT)
				assert.Contains(t, v4NAT[0], "-s "+DaemonBridgeIP)
				assert.Contains(t, v4NAT[0], "! -d "+ECSSubNet)
				assert.Contains(t, v4NAT[0], "-o "+testEgressDeviceName)
				assert.Contains(t, v4NAT[0], "MASQUERADE")
			} else {
				assert.Empty(t, v4NAT, "no IPv4 NAT without an IPv4 address")
			}
			if tc.wantV6NAT {
				require.Len(t, v6NAT, 1, "exactly one IPv6 MASQUERADE; got %v", v6NAT)
				assert.Contains(t, v6NAT[0], "-s "+DaemonBridgeIPv6)
				assert.Contains(t, v6NAT[0], "! -d "+ECSSubNetIPv6)
				assert.Contains(t, v6NAT[0], "-o "+testEgressDeviceName)
				assert.Contains(t, v6NAT[0], "MASQUERADE")
			} else {
				assert.Empty(t, v6NAT, "no IPv6 NAT without an IPv6 address")
			}

			// The daemon's default route of each family points at the task's
			// bridge address of that family, on the daemon's bridge veth.
			var v4Gw, v6Gw []string
			for _, r := range routes {
				assert.Equal(t, daemonLink.Attrs().Index, r.LinkIndex, "daemon route must use the daemon's bridge veth")
				require.NotNil(t, r.Gw)
				// The default route carries an explicit per-family destination.
				require.NotNil(t, r.Dst, "daemon egress route must carry an explicit destination")
				if r.Gw.To4() != nil {
					assert.Equal(t, "0.0.0.0/0", r.Dst.String(), "IPv4 gateway needs the IPv4 default destination")
					v4Gw = append(v4Gw, r.Gw.String())
				} else {
					assert.Equal(t, "::/0", r.Dst.String(), "IPv6 gateway needs the IPv6 default destination")
					v6Gw = append(v6Gw, r.Gw.String())
				}
			}
			if tc.wantV4Gw != "" {
				assert.Equal(t, []string{tc.wantV4Gw}, v4Gw, "IPv4 default route via the task's bridge address")
			} else {
				assert.Empty(t, v4Gw, "no IPv4 default route without an IPv4 address")
			}
			if tc.wantV6Gw != "" {
				assert.Equal(t, []string{tc.wantV6Gw}, v6Gw, "IPv6 default route via the task's bridge address")
			} else {
				assert.Empty(t, v6Gw, "no IPv6 default route without an IPv6 address")
			}
		})
	}
}

// TestContainerdConfigureDaemonEgressIPv6OnlyNeedsNoIPv4Address verifies
// that IPv6-only egress setup requires no IPv4 address.
func TestContainerdConfigureDaemonEgressIPv6OnlyNeedsNoIPv4Address(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := mock_nsutil.NewMockNetNSUtil(ctrl)
	nl := mock_netlinkwrapper.NewMockNetLink(ctrl)
	nsUtil.EXPECT().ExecInNSPath(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ string, fn func(cnins.NetNS) error) error { return fn(nil) }).AnyTimes()
	nsUtil.EXPECT().GetNetNSPath(daemonBridgeNetNSName).Return(testDaemonNetNSPath).AnyTimes()

	taskLink := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Index: 7, Name: "ecs-eth0"}}
	daemonLink := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Index: 11, Name: "eth0"}}
	nl.EXPECT().LinkList().Return([]netlink.Link{taskLink}, nil).AnyTimes()
	nl.EXPECT().AddrList(taskLink, netlink.FAMILY_V4).Return(nil, nil).AnyTimes()
	nl.EXPECT().AddrList(taskLink, netlink.FAMILY_V6).Return([]netlink.Addr{{
		IPNet: &net.IPNet{IP: net.ParseIP(testTaskBridgeIPv6), Mask: net.CIDRMask(112, 128)},
	}}, nil).AnyTimes()
	nl.EXPECT().LinkByName("eth0").Return(daemonLink, nil).AnyTimes()

	routed := false
	nl.EXPECT().RouteReplace(gomock.Any()).DoAndReturn(func(r *netlink.Route) error {
		routed = true
		return nil
	}).AnyTimes()

	origIptables, origSysctl := runIptablesCommand, runSysctlCommand
	defer func() { runIptablesCommand, runSysctlCommand = origIptables, origSysctl }()
	runIptablesCommand = func(string, ...string) ([]byte, error) { return nil, nil }
	runSysctlCommand = func(...string) ([]byte, error) { return nil, nil }

	c := &containerd{common: common{nsUtil: nsUtil, netlink: nl}}
	c.configureDaemonEgress(testEgressTaskNetNSPath, &networkinterface.NetworkInterface{
		DeviceName:    testEgressDeviceName,
		IPV6Addresses: []*networkinterface.IPV6Address{{Address: "2001:db8::5", Primary: true}},
	})

	assert.True(t, routed, "an IPv6-only task must still give its daemon a default route")
}

func containsSubstring(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
