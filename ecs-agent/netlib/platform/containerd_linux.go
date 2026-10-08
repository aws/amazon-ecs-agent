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
	"net"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"

	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
)

// containerd implements platform API methods for non-firecrakcer infrastructure.
type containerd struct {
	common
	// nsSetupMutex serializes work on the daemon namespace: its setup, so
	// concurrent callers cannot race on the exists/configured checks and CNI
	// execution, and the egress routes tasks install in it.
	nsSetupMutex sync.Mutex
}

// ConfigureDaemonNetNS drives the daemon network namespace through its
// phases. The NONE -> READY_PULL transition only creates the namespace: the
// namespace's existence is what concurrent task interface configuration
// observes to size the bridge's connected subnets, so it must be creatable
// before any interface exists. The CONFIGURED -> READY transition attaches
// the namespace to the bridge with a static bridge-local address.
func (c *containerd) ConfigureDaemonNetNS(netNS *tasknetworkconfig.NetworkNamespace) error {
	if netNS.DesiredState == status.NetworkDeleted {
		return errors.New("invalid transition state encountered: " + netNS.DesiredState.String())
	}

	c.nsSetupMutex.Lock()
	defer c.nsSetupMutex.Unlock()

	if netNS.KnownState == status.NetworkNone &&
		netNS.DesiredState == status.NetworkReadyPull {
		if err := c.CreateNetNS(netNS.Path); err != nil {
			return err
		}

		// A daemon container mounts this namespace's resolv.conf, hostname and
		// hosts files, so they have to exist before it starts. They are built
		// from the namespace's own interface rather than copied from the host:
		// the host's resolvers and hostname describe the host, not this
		// namespace.
		primaryIF := netNS.GetPrimaryInterface()
		if primaryIF == nil {
			return errors.New("daemon network namespace has no interface to build DNS config from")
		}
		return c.common.createNetworkConfigFiles(netNS.Name, primaryIF)
	}

	// The daemon namespace has the one interface it was built with, so the
	// READY_PULL -> CONFIGURED transition has nothing to do.
	if netNS.KnownState == status.NetworkConfigured &&
		netNS.DesiredState == status.NetworkReady {
		if c.isDaemonNamespaceConfigured(netNS.Path) {
			return nil
		}

		ipComp := c.getIPCompatibilityFromNetNS(netNS)

		bridgeConfig, err := createDaemonBridgePluginConfig(netNS.Path, ipComp)
		if err != nil {
			return errors.Wrap(err, "failed to create daemon bridge plugin config")
		}

		c.os.Setenv(CNIPluginLogFileEnv, ecscni.PluginLogPath)
		c.os.Setenv(IPAMDataPathEnv, filepath.Join(c.stateDBDir, IPAMDataFileName))

		_, err = c.executeCNIPlugin(context.Background(), true, bridgeConfig)
		if err != nil {
			return errors.Wrap(err, "failed to setup daemon network namespace bridge")
		}
	}

	return nil
}

// configureDaemonEgress carries the shared daemon namespace's traffic out over
// this task's ENI: the task namespace forwards on the daemon's behalf and
// masquerades to the ENI's own address, and the daemon's default route is
// pointed at this task's bridge address.
//
// Best-effort throughout: a task must not fail to launch because a daemon could
// not be given egress.
func (c *containerd) configureDaemonEgress(
	netNSPath string,
	eni *networkinterface.NetworkInterface,
) {
	logFields := logger.Fields{
		"NetNSPath":  netNSPath,
		"DeviceName": eni.DeviceName,
	}

	ipComp := ipcompatibility.NewIPCompatibility(
		len(eni.IPV4Addresses) > 0, len(eni.IPV6Addresses) > 0)

	// A family the ENI lacks is left untouched, so an IPv6-only task is not
	// made to look for an IPv4 address it does not have.
	type egressFamily struct {
		subnet  string
		daemon  string // the daemon's address within subnet, the only NAT source
		netlink int
		ipv6    bool
	}
	var families []egressFamily
	if ipComp.IsIPv4Compatible() {
		families = append(families, egressFamily{subnet: ECSSubNet, daemon: DaemonBridgeIP, netlink: netlink.FAMILY_V4})
	}
	if ipComp.IsIPv6Compatible() {
		families = append(families, egressFamily{subnet: ECSSubNetIPv6, daemon: DaemonBridgeIPv6, netlink: netlink.FAMILY_V6, ipv6: true})
	}

	taskBridgeIPs := make(map[string]net.IP, len(families))
	err := c.common.nsUtil.ExecInNSPath(netNSPath, func(_ cnins.NetNS) error {
		if err := enableTaskNamespaceForwarding(ipComp, eni.DeviceName); err != nil {
			return err
		}
		for _, family := range families {
			// The address the daemon namespace has to route through is this
			// task's own address on the bridge they share.
			taskBridgeIP, err := c.addressInSubnet(family.subnet, family.netlink)
			if err != nil {
				return err
			}
			taskBridgeIPs[family.subnet] = taskBridgeIP

			args := func() []string { return getDaemonEgressNATArgs(family.daemon, family.subnet, eni.DeviceName) }
			if err := modifyNetfilterEntry(iptablesTableNat, iptablesCheck, args, family.ipv6); err == nil {
				continue
			}
			if err := modifyNetfilterEntry(iptablesTableNat, iptablesAppend, args, family.ipv6); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logFields[loggerfield.Error] = err
		logger.Error("Failed to let the task namespace forward daemon traffic", logFields)
		return
	}

	if len(taskBridgeIPs) == 0 {
		logger.Warn("No task bridge address to route daemon egress through", logFields)
		return
	}
	for subnet, ip := range taskBridgeIPs {
		logFields["TaskBridgeIP:"+subnet] = ip.String()
	}

	// The daemon namespace's routes are shared state with its setup.
	c.nsSetupMutex.Lock()
	defer c.nsSetupMutex.Unlock()
	daemonNSPath := c.common.GetNetNSPath(DaemonBridgeNetNSName)
	err = c.common.nsUtil.ExecInNSPath(daemonNSPath, func(_ cnins.NetNS) error {
		link, err := c.common.netlink.LinkByName(ecscni.DefaultInterfaceName)
		if err != nil {
			return errors.Wrapf(err, "failed to find %s in the daemon namespace",
				ecscni.DefaultInterfaceName)
		}
		for _, family := range families {
			// The destination must be the family's own default. netlink reads a
			// nil Dst as 0.0.0.0/0, so an IPv6 gateway would otherwise install
			// an IPv4 default route and leave the daemon with no IPv6 route to
			// the VPC resolver or any IPv6 endpoint.
			_, defaultDst, err := net.ParseCIDR(DefaultRouteDestination)
			if family.ipv6 {
				_, defaultDst, err = net.ParseCIDR(DefaultRouteDestinationIPv6)
			}
			if err != nil {
				return errors.Wrapf(err, "failed to parse the default route destination for %s", family.subnet)
			}
			if err := c.common.netlink.RouteReplace(&netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       defaultDst,
				Gw:        taskBridgeIPs[family.subnet],
			}); err != nil {
				return errors.Wrapf(err, "failed to set the daemon default route for %s", family.subnet)
			}
		}
		return nil
	})
	if err != nil {
		logFields[loggerfield.Error] = err
		logger.Error("Failed to route daemon egress through the task", logFields)
		return
	}

	logger.Info("Daemon namespace egress now leaves over the task ENI", logFields)
}

// addressInSubnet returns this namespace's address within the given subnet,
// looked up among addresses of the given netlink family. Must be called inside
// the namespace being inspected.
func (c *containerd) addressInSubnet(subnet string, family int) (net.IP, error) {
	_, cidr, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, err
	}

	links, err := c.common.netlink.LinkList()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list links")
	}
	for _, link := range links {
		addrs, err := c.common.netlink.AddrList(link, family)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if addr.IP != nil && cidr.Contains(addr.IP) {
				return addr.IP, nil
			}
		}
	}
	return nil, errors.Errorf("no address found in %s", subnet)
}

func (c *containerd) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task,
	existing ...*tasknetworkconfig.NetworkNamespace,
) (*tasknetworkconfig.TaskNetworkConfig, error) {
	return c.common.buildTaskNetworkConfiguration(taskID, taskPayload, false, nil, existing)
}

// BuildAttachmentNetworkConfiguration adds the shared daemon namespace to the
// task namespace the attached interface establishes. Both are modelled from the
// task's primary interface only: the daemon namespace mirrors the address
// families and resolvers of the interface its egress leaves over, and that is
// the primary. For any other interface both namespaces are left nil.
func (c *containerd) BuildAttachmentNetworkConfiguration(
	taskID string,
	eni *ecsacs.ElasticNetworkInterface,
) (*tasknetworkconfig.AttachmentNetworkConfig, error) {
	cfg, err := c.common.BuildAttachmentNetworkConfiguration(taskID, eni)
	if err != nil || cfg.TaskNetNS == nil {
		return cfg, err
	}
	cfg.DaemonNetNS, err = c.buildDaemonNetworkNamespace(eni)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// buildDaemonNetworkNamespace returns the model of the shared daemon namespace
// as ConfigureDaemonNetNS will provision it. The namespace is the host's, so
// the device and the static bridge address are fixed for the host's lifetime;
// what varies is taken from the task ENI the daemons' egress leaves over: the
// address families, since a family the task cannot route has no business on the
// daemon, and the resolvers, since the daemon must resolve names the way the
// workload does and the host's resolvers describe service infrastructure the
// workload is not meant to see.
func (c *containerd) buildDaemonNetworkNamespace(taskENI *ecsacs.ElasticNetworkInterface) (*tasknetworkconfig.NetworkNamespace, error) {
	if len(taskENI.Ipv4Addresses) == 0 && len(taskENI.Ipv6Addresses) == 0 {
		return nil, errors.New("the task ENI carries no addresses to derive the daemon namespace's families from")
	}

	iface := &networkinterface.NetworkInterface{
		Name:                 DaemonBridgeNetNSName,
		DeviceName:           DaemonInterfaceName,
		Default:              true,
		PrivateDNSName:       DefaultArg,
		KnownStatus:          status.NetworkNone,
		DesiredStatus:        status.NetworkReadyPull,
		DomainNameServers:    aws.ToStringSlice(taskENI.DomainNameServers),
		DomainNameSearchList: aws.ToStringSlice(taskENI.DomainName),
	}
	if len(taskENI.Ipv4Addresses) > 0 {
		iface.IPV4Addresses = []*networkinterface.IPV4Address{{Primary: true, Address: stripPrefixLen(DaemonBridgeIP)}}
	}
	if len(taskENI.Ipv6Addresses) > 0 {
		iface.IPV6Addresses = []*networkinterface.IPV6Address{{Primary: true, Address: stripPrefixLen(DaemonBridgeIPv6)}}
	}

	return &tasknetworkconfig.NetworkNamespace{
		Name:              DaemonBridgeNetNSName,
		Path:              c.common.GetNetNSPath(DaemonBridgeNetNSName),
		NetworkMode:       DaemonBridgeNetworkMode,
		KnownState:        status.NetworkNone,
		DesiredState:      status.NetworkReadyPull,
		NetworkInterfaces: []*networkinterface.NetworkInterface{iface},
	}, nil
}

// stripPrefixLen returns the address part of a CIDR literal.
func stripPrefixLen(cidr string) string {
	if i := strings.IndexByte(cidr, '/'); i >= 0 {
		return cidr[:i]
	}
	return cidr
}

func (c *containerd) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	return c.common.createDNSConfig(taskID, false, netNS)
}

func (c *containerd) ConfigureInterface(
	ctx context.Context,
	netNSPath string,
	iface *networkinterface.NetworkInterface,
	netDAO netlibdata.NetworkDataClient,
) error {
	if err := c.common.configureInterface(ctx, netNSPath, iface, netDAO); err != nil {
		return err
	}

	// Daemon egress leaves over the task's primary interface, so this runs
	// once that interface is in place. Every namespace of a multi-namespace
	// task has a default interface of its own; the task's primary interface
	// is the one at index 0.
	if iface.IsPrimary() && iface.Index == 0 &&
		iface.DesiredStatus == status.NetworkReadyPull &&
		c.common.daemonNamespaceExists() {
		c.configureDaemonEgress(netNSPath, iface)
	}

	return nil
}

func (c *containerd) ConfigureAppMesh(ctx context.Context, netNSPath string, cfg *appmesh.AppMesh) error {
	return c.common.configureAppMesh(ctx, netNSPath, cfg)
}

func (c *containerd) ConfigureServiceConnect(
	ctx context.Context,
	netNSPath string,
	primaryIf *networkinterface.NetworkInterface,
	scConfig *serviceconnect.ServiceConnectConfig,
) error {
	return c.common.configureServiceConnect(ctx, netNSPath, primaryIf, scConfig)
}
