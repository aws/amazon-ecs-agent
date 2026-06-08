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

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"

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

// addGatewayNeighbor installs a permanent ARP entry and /32 link-scope route
// for the gateway in the task netns.
func (il *isolatedLinux) addGatewayNeighbor(netNSPath string, eni *networkinterface.NetworkInterface) error {
	gwIPStr := eni.GetSubnetGatewayIPv4Address()
	if gwIPStr == "" {
		return nil
	}
	// TODO: Install equivalent neighbor entry for IPv6 gateway.

	gwIP := net.ParseIP(gwIPStr)
	if gwIP == nil {
		return fmt.Errorf("failed to parse gateway IP: %s", gwIPStr)
	}

	// The gateway MAC must be resolved from the host before entering the task
	// netns, because the ENI has already been moved out of the host namespace.
	gwMAC, err := il.resolveHostNeighbor(gwIP)
	if err != nil {
		return errors.Wrap(err, "gateway MAC not in host neighbor cache")
	}

	logger.Info("Installing gateway neighbor in task netns", map[string]interface{}{
		"GatewayIP":  gwIP.String(),
		"GatewayMAC": gwMAC.String(),
		"NetNSPath":  netNSPath,
		"DeviceName": eni.DeviceName,
	})

	return il.common.nsUtil.ExecInNSPath(netNSPath, func(_ cnins.NetNS) error {
		link, linkErr := il.common.netlink.LinkByName(eni.DeviceName)
		if linkErr != nil {
			return errors.Wrapf(linkErr, "failed to find device %s in task netns", eni.DeviceName)
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

// resolveHostNeighbor looks up the MAC address for the given IP in the host's
// neighbor (ARP) table. Returns an error if no entry is found.
func (il *isolatedLinux) resolveHostNeighbor(ip net.IP) (net.HardwareAddr, error) {
	neighbors, err := il.common.netlink.NeighList(0, netlink.FAMILY_V4)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list host neighbors")
	}

	for _, n := range neighbors {
		if n.IP.Equal(ip) && len(n.HardwareAddr) > 0 {
			return n.HardwareAddr, nil
		}
	}

	return nil, fmt.Errorf("no neighbor entry for %s in host ARP table", ip)
}
