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
	"context"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

// isolatedLinux implements the platform API for containers running inside a
// micro VM. It extends managedLinux with two differences:
//
//  1. No ecs-bridge CNI plugin — TMDS is guest-local, so no bridge is needed
//     to reach it from the task netns.
//  2. Permanent ARP neighbor for the VPC gateway — L2 broadcast is unavailable
//     inside the micro VM, so the gateway MAC must be installed statically.
type isolatedLinux struct {
	managedLinux
}

// ConfigureInterface configures a network interface in the task netns for the
// isolated platform.
func (il *isolatedLinux) ConfigureInterface(
	ctx context.Context,
	netNSPath string,
	iface *networkinterface.NetworkInterface,
	netDAO netlibdata.NetworkDataClient,
) error {
	var err error

	switch iface.InterfaceAssociationProtocol {
	case networkinterface.DefaultInterfaceAssociationProtocol:
		iface.DeviceName = networkinterface.DefaultTapDeviceName
		err = il.configureRegularENI(ctx, netNSPath, iface)
	case networkinterface.VLANInterfaceAssociationProtocol:
		iface.DeviceName = networkinterface.DefaultTapDeviceName
		err = il.configureBranchENI(ctx, netNSPath, iface)
	case networkinterface.V2NInterfaceAssociationProtocol:
		return il.common.configureGENEVEInterface(ctx, netNSPath, iface, netDAO)
	case networkinterface.VETHInterfaceAssociationProtocol:
		return nil
	default:
		return errors.New("invalid interface association protocol " + iface.InterfaceAssociationProtocol)
	}

	// Gateway neighbor is only installed on add (NetworkReadyPull). On delete
	// the netns is torn down entirely, so the neighbor entry is removed implicitly.
	if err != nil || iface.DesiredStatus != status.NetworkReadyPull {
		return err
	}
	return il.addGatewayNeighbor(netNSPath, iface)
}

// configureRegularENI configures a directly-attached ENI (not a branch,
// GENEVE, or VETH interface).
func (il *isolatedLinux) configureRegularENI(ctx context.Context, netNSPath string, eni *networkinterface.NetworkInterface) error {
	logger.Info("Configuring regular ENI", map[string]interface{}{
		"ENIName":   eni.Name,
		"NetNSPath": netNSPath,
	})

	var cniNetConf []ecscni.PluginConfig
	var add bool
	var err error

	il.common.os.Setenv(CNIPluginLogFileEnv, ecscni.PluginLogPath)
	il.common.os.Setenv(IPAMDataPathEnv, filepath.Join(il.common.stateDBDir, IPAMDataFileName))

	switch eni.DesiredStatus {
	case status.NetworkReadyPull:
		cniNetConf = append(cniNetConf, createENIPluginConfigs(netNSPath, eni))
		add = true
	case status.NetworkDeleted:
		cniNetConf = append(cniNetConf, createENIPluginConfigs(netNSPath, eni))
		add = false
	}

	_, err = il.common.executeCNIPlugin(ctx, add, cniNetConf...)
	if err != nil {
		err = errors.Wrap(err, "failed to setup regular eni")
	}

	return err
}

// configureBranchENI configures a branch ENI for the isolated platform.
func (il *isolatedLinux) configureBranchENI(ctx context.Context, netNSPath string, eni *networkinterface.NetworkInterface) error {
	logger.Info("Configuring branch ENI", map[string]interface{}{
		"ENIName":   eni.Name,
		"NetNSPath": netNSPath,
	})

	il.common.os.Setenv(IPAMDataPathEnv, filepath.Join(il.common.stateDBDir, IPAMDataFileName))

	var cniNetConf []ecscni.PluginConfig
	var err error
	add := true

	switch eni.DesiredStatus {
	case status.NetworkReadyPull:
		cniNetConf = append(cniNetConf, createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault))
	case status.NetworkDeleted:
		cniNetConf = append(cniNetConf, createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault))
		add = false
	}

	_, err = il.common.executeCNIPlugin(ctx, add, cniNetConf...)
	if err != nil {
		err = errors.Wrap(err, "failed to setup branch eni")
	}

	return err
}

var (
	// gatewayNeighborResolveTimeout bounds how long we wait for the task-netns
	// kernel to ARP-resolve the VPC gateway's MAC. It is set to match the
	// kernel's own worst-case resolution horizon for a new neighbor:
	// mcast_solicit (default 3) x retrans_time_ms (default 1000ms) = ~3s
	// (see man 7 arp, /proc/sys/net/ipv4/neigh/<iface>/).
	gatewayNeighborResolveTimeout = 3 * time.Second
	// gatewayNeighborResolveInterval is the poll/probe interval while waiting.
	gatewayNeighborResolveInterval = 100 * time.Millisecond
)

// gatewayProbePort is an arbitrary UDP port used only to nudge the kernel into
// ARP-resolving the gateway; nothing is expected to listen there.
const gatewayProbePort = "9"

// gatewayProbeFn prompts the kernel to resolve gwIP by emitting a single
// throwaway datagram from the current network namespace. It is a package var
// so unit tests can stub it. Errors are intentionally ignored: the datagram
// exists only to trigger ARP; the ARP reply (not the datagram's delivery) is
// what populates the neighbor table.
var gatewayProbeFn = func(gwIP net.IP) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(gwIP.String(), gatewayProbePort), gatewayNeighborResolveInterval)
	if err != nil {
		return
	}
	_, _ = conn.Write([]byte{0})
	_ = conn.Close()
}

// addGatewayNeighbor installs a permanent ARP entry and /32 link-scope route
// for the gateway in the task netns.
//
// The gateway MAC is resolved from *inside* the task netns, where the task ENI
// (carrying the task's own VPC IP) has L2 reachability to the gateway. On ECS
// managed instances the host instance's subnet may or may not match the task's
// subnet; when it differs, the host root netns ARP cache does not contain the
// task's gateway. Resolving in the task netns works in both cases, regardless
// of the host's subnet.
func (il *isolatedLinux) addGatewayNeighbor(netNSPath string, eni *networkinterface.NetworkInterface) error {
	// IPv6-only interfaces have no IPv4 gateway to pre-resolve, even when the
	// payload carries a subnet gateway IPv4 address. The guest resolves the
	// IPv6 gateway via NDP, which the TAP egress filters permit, so no
	// neighbor entry is needed for IPv6.
	if eni.IPv6Only() {
		return nil
	}

	gwIPStr := eni.GetSubnetGatewayIPv4Address()
	if gwIPStr == "" {
		return nil
	}

	gwIP := net.ParseIP(gwIPStr)
	if gwIP == nil {
		return fmt.Errorf("failed to parse gateway IP: %s", gwIPStr)
	}

	logger.Info("Installing gateway neighbor in task netns", map[string]interface{}{
		"GatewayIP":  gwIP.String(),
		"NetNSPath":  netNSPath,
		"DeviceName": eni.DeviceName,
	})

	return il.common.nsUtil.ExecInNSPath(netNSPath, func(_ cnins.NetNS) error {
		link, linkErr := il.common.netlink.LinkByName(eni.DeviceName)
		if linkErr != nil {
			return errors.Wrapf(linkErr, "failed to find device %s in task netns", eni.DeviceName)
		}

		gwMAC, err := il.resolveGatewayNeighbor(link, gwIP)
		if err != nil {
			return errors.Wrap(err, "gateway MAC not resolvable in task netns")
		}

		neigh := &netlink.Neigh{
			LinkIndex:    link.Attrs().Index,
			State:        netlink.NUD_PERMANENT,
			IP:           gwIP,
			HardwareAddr: gwMAC,
		}
		if err := il.common.netlink.NeighSet(neigh); err != nil {
			return errors.Wrapf(err, "failed to set permanent neighbor for %s", gwIP)
		}

		// A /32 link-scope route marks the gateway as directly reachable,
		// preventing the kernel from attempting ARP resolution.
		route := &netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst: &net.IPNet{
				IP:   gwIP,
				Mask: net.CIDRMask(32, 32),
			},
			Scope: netlink.SCOPE_LINK,
		}
		if err := il.common.netlink.RouteReplace(route); err != nil {
			return errors.Wrapf(err, "failed to add /32 link-scope route for gateway %s", gwIP)
		}

		return nil
	})
}

// resolveGatewayNeighbor resolves the gateway's MAC from within the current
// (task) network namespace. It polls the interface's neighbor table, nudging
// the kernel to ARP the gateway between polls, until an entry with a resolved
// MAC appears or the timeout elapses. It must be called inside the task netns
// (e.g. from within ExecInNSPath).
func (il *isolatedLinux) resolveGatewayNeighbor(link netlink.Link, gwIP net.IP) (net.HardwareAddr, error) {
	linkIndex := link.Attrs().Index
	deadline := time.Now().Add(gatewayNeighborResolveTimeout)

	for {
		mac, err := il.lookupNeighbor(linkIndex, gwIP)
		if err != nil {
			return nil, err
		}
		if mac != nil {
			return mac, nil
		}

		if time.Now().After(deadline) {
			break
		}

		// Nudge the kernel to resolve the gateway, then wait before re-checking.
		gatewayProbeFn(gwIP)
		time.Sleep(gatewayNeighborResolveInterval)
	}

	return nil, fmt.Errorf("no neighbor entry for %s in task netns after %s", gwIP, gatewayNeighborResolveTimeout)
}

// lookupNeighbor returns the resolved MAC for ip on the given interface, or nil
// if there is no usable (resolved, non-failed) entry yet.
func (il *isolatedLinux) lookupNeighbor(linkIndex int, ip net.IP) (net.HardwareAddr, error) {
	neighbors, err := il.common.netlink.NeighList(linkIndex, netlink.FAMILY_V4)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list neighbors")
	}

	for _, n := range neighbors {
		if n.IP.Equal(ip) && len(n.HardwareAddr) > 0 && n.State != netlink.NUD_FAILED {
			return n.HardwareAddr, nil
		}
	}

	return nil, nil
}

// CreateDNSConfig creates the task DNS config files and backfills the
// interface DNS fields from the host's resolv.conf.
func (il *isolatedLinux) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	// Create the DNS config files. resolv.conf is built from the ENI
	// payload's DNS servers when present, otherwise copied from the host.
	if err := il.managedLinux.CreateDNSConfig(taskID, netNS); err != nil {
		return err
	}

	// An empty DomainNameServers means resolv.conf was copied from the
	// host; backfill the interface's DNS fields from file.
	primaryIF := netNS.GetPrimaryInterface()
	if primaryIF == nil || len(primaryIF.DomainNameServers) > 0 {
		return nil
	}

	src := filepath.Join(il.resolvConfPath, ResolveConfFileName)
	contents, err := il.ioutil.ReadFile(src)
	if err != nil {
		return errors.Wrapf(err, "unable to read %s", src)
	}
	servers, searches := parseResolvConf(contents)
	primaryIF.DomainNameServers = servers
	primaryIF.DomainNameSearchList = searches
	return nil
}
