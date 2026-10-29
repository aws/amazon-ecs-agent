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

package platform

import (
	"errors"
	"net"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	mock_netwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper/mocks"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	testBuildTaskID    = "task-build"
	testBuildMAC       = "0a:11:22:33:44:55"
	testBuildSecondMAC = "0a:11:22:33:44:66"
	testBuildThirdMAC  = "0a:11:22:33:44:77"
	testBuildTrunkMAC  = "0a:aa:bb:cc:dd:ee"
)

func newBuildTestContainerd(t *testing.T, ctrl *gomock.Controller) (*containerd, *mock_netwrapper.MockNet) {
	t.Helper()
	mockNet := mock_netwrapper.NewMockNet(ctrl)
	return &containerd{common: common{
		nsUtil: ecscni.NewNetNSUtil(),
		net:    mockNet,
	}}, mockNet
}

func buildTestENI(id, mac string, index int64, addr string) *ecsacs.ElasticNetworkInterface {
	return &ecsacs.ElasticNetworkInterface{
		Ec2Id:                        aws.String(id),
		MacAddress:                   aws.String(mac),
		Index:                        aws.Int64(index),
		Name:                         aws.String("if-" + id),
		InterfaceAssociationProtocol: aws.String(DefaultArg),
		SubnetGatewayIpv4Address:     aws.String("10.0.0.1/24"),
		Ipv4Addresses:                []*ecsacs.IPv4AddressAssignment{{Primary: aws.Bool(true), PrivateAddress: aws.String(addr)}},
		DomainNameServers:            aws.StringSlice([]string{"10.0.0.2"}),
	}
}

func hostInterfaces(t *testing.T, devs map[string]string) []net.Interface {
	t.Helper()
	var out []net.Interface
	for mac, name := range devs {
		hw, err := net.ParseMAC(mac)
		require.NoError(t, err)
		out = append(out, net.Interface{Name: name, HardwareAddr: hw})
	}
	return out
}

// TestBuildAttachmentNetworkConfigurationPrimary verifies the primary
// interface establishes the task namespace, modelled exactly as the payload
// path would, alongside the daemon namespace.
func TestBuildAttachmentNetworkConfigurationPrimary(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, mockNet := newBuildTestContainerd(t, ctrl)
	mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{testBuildMAC: "eth1"}), nil)

	eni := buildTestENI("eni-primary", testBuildMAC, 0, "10.0.0.10")
	cfg, err := c.BuildAttachmentNetworkConfiguration(testBuildTaskID, eni)
	require.NoError(t, err)
	require.NotNil(t, cfg.TaskNetNS)
	require.NotNil(t, cfg.DaemonNetNS)

	netNS := cfg.TaskNetNS
	require.Equal(t, 0, netNS.Index)
	require.Equal(t, networkinterface.NetNSName(testBuildTaskID, "if-eni-primary"), netNS.Name)
	require.Equal(t, c.GetNetNSPath(netNS.Name), netNS.Path)
	require.Equal(t, status.NetworkNone, netNS.KnownState)
	require.Equal(t, status.NetworkReadyPull, netNS.DesiredState)
	require.Len(t, netNS.NetworkInterfaces, 1)
	iface := netNS.GetPrimaryInterface()
	require.NotNil(t, iface)
	require.True(t, iface.Default)
	require.Equal(t, "eth1", iface.DeviceName)
	require.Equal(t, "eni-primary", iface.ID)
}

// TestBuildAttachmentNetworkConfigurationSecondary verifies a non-primary
// interface establishes neither namespace: the task namespace takes its
// identity from the primary, and the daemon namespace mirrors the interface
// its egress leaves over, which is also the primary. An IPv6-only secondary
// must therefore not give the daemon namespace an IPv6-only model.
func TestBuildAttachmentNetworkConfigurationSecondary(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, _ := newBuildTestContainerd(t, ctrl)

	second := buildTestENI("eni-second", testBuildSecondMAC, 1, "10.0.0.11")
	second.Ipv4Addresses = nil
	second.Ipv6Addresses = []*ecsacs.IPv6AddressAssignment{{Address: aws.String("2600:1f14::11")}}
	cfg, err := c.BuildAttachmentNetworkConfiguration(testBuildTaskID, second)
	require.NoError(t, err)
	require.Nil(t, cfg.TaskNetNS)
	require.Nil(t, cfg.DaemonNetNS)
}

// TestBuildAttachmentNetworkConfigurationErrors verifies the attachment
// builder reports what it cannot model rather than returning a partial
// configuration: no interface, an interface whose device is not on the host,
// a host interface list that cannot be read, and a primary interface with no
// address family for the daemon namespace to mirror.
func TestBuildAttachmentNetworkConfigurationErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, mockNet := newBuildTestContainerd(t, ctrl)

	_, err := c.BuildAttachmentNetworkConfiguration(testBuildTaskID, nil)
	require.Error(t, err)

	mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{testBuildMAC: "eth1"}), nil)
	_, err = c.BuildAttachmentNetworkConfiguration(testBuildTaskID, buildTestENI("eni-unknown", testBuildTrunkMAC, 0, "10.0.0.12"))
	require.Error(t, err)

	mockNet.EXPECT().Interfaces().Return(nil, errors.New("netlink down"))
	_, err = c.BuildAttachmentNetworkConfiguration(testBuildTaskID, buildTestENI("eni-primary", testBuildMAC, 0, "10.0.0.10"))
	require.ErrorContains(t, err, "failed to list host interfaces")

	mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{testBuildMAC: "eth1"}), nil)
	noAddrs := buildTestENI("eni-primary", testBuildMAC, 0, "10.0.0.10")
	noAddrs.Ipv4Addresses = nil
	_, err = c.BuildAttachmentNetworkConfiguration(testBuildTaskID, noAddrs)
	require.Error(t, err, "an ENI with no addresses gives the daemon namespace no family")
}

// TestBuildAttachmentDaemonNamespaceTakesResolversFromTask verifies the
// daemon namespace resolves names the way the task does, while its device and
// address are the namespace's own.
func TestBuildAttachmentDaemonNamespaceTakesResolversFromTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, _ := newBuildTestContainerd(t, ctrl)

	netNS, err := c.buildDaemonNetworkNamespace(&ecsacs.ElasticNetworkInterface{
		Ipv4Addresses:     []*ecsacs.IPv4AddressAssignment{{Primary: aws.Bool(true), PrivateAddress: aws.String("10.0.0.5")}},
		DomainNameServers: aws.StringSlice([]string{"10.0.0.2"}),
		DomainName:        aws.StringSlice([]string{"us-west-2.compute.internal"}),
	})
	require.NoError(t, err)
	require.NotNil(t, netNS)
	require.Equal(t, DaemonBridgeNetNSName, netNS.Name)
	require.Equal(t, DaemonBridgeNetworkMode, netNS.NetworkMode)
	require.Equal(t, status.NetworkNone, netNS.KnownState)
	require.Equal(t, status.NetworkReadyPull, netNS.DesiredState)

	iface := netNS.GetPrimaryInterface()
	require.NotNil(t, iface)
	require.Equal(t, []string{"10.0.0.2"}, iface.DomainNameServers)
	require.Equal(t, []string{"us-west-2.compute.internal"}, iface.DomainNameSearchList)
	require.Equal(t, DaemonInterfaceName, iface.DeviceName)
	require.Equal(t, DefaultArg, iface.PrivateDNSName)
	require.Equal(t, "169.254.172.2", iface.GetPrimaryIPv4Address())
	require.True(t, iface.Default)

	// A task ENI with no resolvers yields a namespace with none, rather than
	// the host's.
	netNS, err = c.buildDaemonNetworkNamespace(&ecsacs.ElasticNetworkInterface{
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{{Primary: aws.Bool(true), PrivateAddress: aws.String("10.0.0.5")}},
	})
	require.NoError(t, err)
	require.Empty(t, netNS.GetPrimaryInterface().DomainNameServers)
}

// TestBuildAttachmentDaemonNamespaceFollowsTaskAddressFamilies verifies the
// daemon namespace speaks the address families of the ENI it will egress over.
func TestBuildAttachmentDaemonNamespaceFollowsTaskAddressFamilies(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, _ := newBuildTestContainerd(t, ctrl)

	v4 := []*ecsacs.IPv4AddressAssignment{{Primary: aws.Bool(true), PrivateAddress: aws.String("10.0.0.5")}}
	v6 := []*ecsacs.IPv6AddressAssignment{{Address: aws.String("2001:db8::5")}}
	tests := []struct {
		name   string
		eni    *ecsacs.ElasticNetworkInterface
		wantV4 string
		wantV6 string
	}{
		{name: "ipv4-only", eni: &ecsacs.ElasticNetworkInterface{Ipv4Addresses: v4}, wantV4: "169.254.172.2"},
		{name: "ipv6-only", eni: &ecsacs.ElasticNetworkInterface{Ipv6Addresses: v6}, wantV6: "fd00:ec2::172:2"},
		{name: "dual-stack", eni: &ecsacs.ElasticNetworkInterface{Ipv4Addresses: v4, Ipv6Addresses: v6},
			wantV4: "169.254.172.2", wantV6: "fd00:ec2::172:2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			netNS, err := c.buildDaemonNetworkNamespace(tc.eni)
			require.NoError(t, err)
			iface := netNS.GetPrimaryInterface()
			require.Equal(t, tc.wantV4, iface.GetPrimaryIPv4Address())
			require.Equal(t, tc.wantV6, iface.GetPrimaryIPv6Address())
		})
	}
}

// attachTimeNamespace models a namespace built from the primary attachment
// and driven to READY_PULL: its device has left the host namespace and its
// interface carries the state configuration recorded.
func attachTimeNamespace(t *testing.T, c *containerd, mockNet *mock_netwrapper.MockNet, eni *ecsacs.ElasticNetworkInterface, dev string) *tasknetworkconfig.NetworkNamespace {
	t.Helper()
	mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{aws.ToString(eni.MacAddress): dev}), nil)
	cfg, err := c.BuildAttachmentNetworkConfiguration(testBuildTaskID, eni)
	require.NoError(t, err)
	netNS := cfg.TaskNetNS
	netNS.KnownState = status.NetworkReadyPull
	netNS.NetworkInterfaces[0].KnownStatus = status.NetworkReadyPull
	netNS.Hosts = []tasknetworkconfig.Host{{IP: "10.0.0.10", Hostnames: []string{"ip-10-0-0-10"}}}
	return netNS
}

// TestBuildTaskNetworkConfigurationWithExistingSingleNamespace verifies the
// payload's remaining interfaces join the existing primary namespace when the
// task does not map interfaces to containers. The result is a fresh model
// under the existing namespace's identity; the held interface's device name is
// taken from the existing model since it is no longer on the host, and the
// existing model is left as it was.
func TestBuildTaskNetworkConfigurationWithExistingSingleNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, mockNet := newBuildTestContainerd(t, ctrl)

	primary := buildTestENI("eni-primary", testBuildMAC, 0, "10.0.0.10")
	second := buildTestENI("eni-second", testBuildSecondMAC, 1, "10.0.0.11")
	existing := attachTimeNamespace(t, c, mockNet, primary, "eth1")

	// The primary's device has moved into the namespace; only the second is
	// still on the host.
	mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{testBuildSecondMAC: "eth2"}), nil)
	payload := &ecsacs.Task{
		NetworkMode:              aws.String("awsvpc"),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{primary, second},
		Containers:               []*ecsacs.Container{{Name: aws.String("app")}},
		ProxyConfiguration: &ecsacs.ProxyConfiguration{
			Type: aws.String("APPMESH"),
			Properties: map[string]*string{
				"IgnoredUID":         aws.String("1337"),
				"ProxyIngressPort":   aws.String("15000"),
				"ProxyEgressPort":    aws.String("15001"),
				"AppPorts":           aws.String("8080"),
				"EgressIgnoredIPs":   aws.String("169.254.170.2"),
				"EgressIgnoredPorts": aws.String("22"),
			},
		},
	}
	cfg, err := c.BuildTaskNetworkConfiguration(testBuildTaskID, payload, existing)
	require.NoError(t, err)
	require.Len(t, cfg.NetworkNamespaces, 1)

	merged := cfg.NetworkNamespaces[0]
	require.NotSame(t, existing, merged)
	require.Equal(t, existing.Name, merged.Name)
	require.Equal(t, existing.Path, merged.Path)
	require.Equal(t, 0, merged.Index)
	require.NotNil(t, merged.AppMeshConfig, "task-level configuration comes from the payload")
	require.Len(t, merged.NetworkInterfaces, 2)

	held := merged.GetInterfaceByIndex(0)
	require.NotNil(t, held)
	require.Equal(t, "eni-primary", held.ID)
	require.Equal(t, "eth1", held.DeviceName, "device name of a held interface comes from the existing model")
	require.True(t, held.Default)
	require.Equal(t, status.NetworkReadyPull, held.KnownStatus, "a held interface keeps the status the existing model recorded")
	require.Equal(t, status.NetworkReadyPull, merged.KnownState, "the namespace carries the existing model's state")
	require.Equal(t, existing.Hosts, merged.Hosts, "the hosts entries the pull phase read back are carried over")
	require.NotSame(t, existing.NetworkInterfaces[0], held)

	added := merged.GetInterfaceByIndex(1)
	require.NotNil(t, added)
	require.Equal(t, "eni-second", added.ID)
	require.Equal(t, "eth2", added.DeviceName)
	require.False(t, added.Default)
	require.Equal(t, status.NetworkNone, added.KnownStatus, "an added interface is for Start to configure")

	require.Len(t, existing.NetworkInterfaces, 1, "the existing model is not modified")
	require.Nil(t, existing.AppMeshConfig)
}

// TestBuildTaskNetworkConfigurationWithExistingMappedNamespaces verifies that
// when the task maps interfaces to containers, the existing primary namespace
// keeps index 0 and the remaining interfaces form namespaces of their own
// behind it.
func TestBuildTaskNetworkConfigurationWithExistingMappedNamespaces(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, mockNet := newBuildTestContainerd(t, ctrl)

	primary := buildTestENI("eni-primary", testBuildMAC, 0, "10.0.0.10")
	second := buildTestENI("eni-second", testBuildSecondMAC, 1, "10.0.0.11")
	existing := attachTimeNamespace(t, c, mockNet, primary, "eth1")

	mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{testBuildSecondMAC: "eth2"}), nil)
	payload := &ecsacs.Task{
		NetworkMode:              aws.String("awsvpc"),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{primary, second},
		Containers: []*ecsacs.Container{
			{Name: aws.String("sidecar"), NetworkInterfaceNames: aws.StringSlice([]string{"if-eni-second"})},
			{Name: aws.String("app"), NetworkInterfaceNames: aws.StringSlice([]string{"if-eni-primary"})},
		},
	}
	cfg, err := c.BuildTaskNetworkConfiguration(testBuildTaskID, payload, existing)
	require.NoError(t, err)
	require.Len(t, cfg.NetworkNamespaces, 2)

	primaryNS := cfg.GetPrimaryNetNS()
	require.NotNil(t, primaryNS)
	require.Equal(t, existing.Name, primaryNS.Name)
	require.Len(t, primaryNS.NetworkInterfaces, 1)
	require.Equal(t, "eni-primary", primaryNS.NetworkInterfaces[0].ID)
	require.Equal(t, "eth1", primaryNS.NetworkInterfaces[0].DeviceName)

	var secondary *tasknetworkconfig.NetworkNamespace
	for _, ns := range cfg.NetworkNamespaces {
		if ns.Index == 1 {
			secondary = ns
		}
	}
	require.NotNil(t, secondary)
	require.Equal(t, networkinterface.NetNSName(testBuildTaskID, "if-eni-second"), secondary.Name)
	require.Len(t, secondary.NetworkInterfaces, 1)
	require.Equal(t, "eth2", secondary.NetworkInterfaces[0].DeviceName)
}

// TestBuildTaskNetworkConfigurationWithExistingComplete verifies a payload
// that declares exactly the interfaces the existing namespace holds yields
// the same namespace with nothing added, which is what a redelivered payload
// looks like.
func TestBuildTaskNetworkConfigurationWithExistingComplete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, mockNet := newBuildTestContainerd(t, ctrl)

	primary := buildTestENI("eni-primary", testBuildMAC, 0, "10.0.0.10")
	existing := attachTimeNamespace(t, c, mockNet, primary, "eth1")

	mockNet.EXPECT().Interfaces().Return(nil, nil)
	payload := &ecsacs.Task{
		NetworkMode:              aws.String("awsvpc"),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{primary},
		Containers:               []*ecsacs.Container{{Name: aws.String("app")}},
	}
	cfg, err := c.BuildTaskNetworkConfiguration(testBuildTaskID, payload, existing)
	require.NoError(t, err)
	require.Len(t, cfg.NetworkNamespaces, 1)
	require.Equal(t, existing.Name, cfg.NetworkNamespaces[0].Name)
	require.Len(t, cfg.NetworkNamespaces[0].NetworkInterfaces, 1)
	require.Equal(t, "eth1", cfg.NetworkNamespaces[0].NetworkInterfaces[0].DeviceName)
}

// TestBuildTaskNetworkConfigurationWithExistingMismatch verifies the builder
// rejects a payload that cannot be reconciled with what exists: an existing
// namespace holding an interface the task does not declare, and a task that
// maps the interfaces of one existing namespace into different namespaces.
func TestBuildTaskNetworkConfigurationWithExistingMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	c, mockNet := newBuildTestContainerd(t, ctrl)

	primary := buildTestENI("eni-primary", testBuildMAC, 0, "10.0.0.10")
	second := buildTestENI("eni-second", testBuildSecondMAC, 1, "10.0.0.11")
	third := buildTestENI("eni-third", testBuildThirdMAC, 2, "10.0.0.12")

	t.Run("undeclared interface", func(t *testing.T) {
		existing := attachTimeNamespace(t, c, mockNet, primary, "eth1")
		mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{testBuildSecondMAC: "eth2"}), nil)
		_, err := c.BuildTaskNetworkConfiguration(testBuildTaskID, &ecsacs.Task{
			NetworkMode:              aws.String("awsvpc"),
			ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{second},
			Containers:               []*ecsacs.Container{{Name: aws.String("app")}},
		}, existing)
		require.ErrorContains(t, err, "does not declare")
	})

	t.Run("split existing namespace", func(t *testing.T) {
		// An existing namespace already holding two interfaces, which the
		// payload maps to different containers.
		mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{testBuildMAC: "eth1", testBuildSecondMAC: "eth2"}), nil)
		full, err := c.BuildTaskNetworkConfiguration(testBuildTaskID, &ecsacs.Task{
			NetworkMode:              aws.String("awsvpc"),
			ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{primary, second},
			Containers:               []*ecsacs.Container{{Name: aws.String("app")}},
		})
		require.NoError(t, err)
		existing := full.NetworkNamespaces[0]

		mockNet.EXPECT().Interfaces().Return(hostInterfaces(t, map[string]string{testBuildThirdMAC: "eth3"}), nil)
		_, err = c.BuildTaskNetworkConfiguration(testBuildTaskID, &ecsacs.Task{
			NetworkMode:              aws.String("awsvpc"),
			ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{primary, second, third},
			Containers: []*ecsacs.Container{
				{Name: aws.String("a"), NetworkInterfaceNames: aws.StringSlice([]string{"if-eni-primary"})},
				{Name: aws.String("b"), NetworkInterfaceNames: aws.StringSlice([]string{"if-eni-second", "if-eni-third"})},
			},
		}, existing)
		require.ErrorContains(t, err, "splits")
	})
}
