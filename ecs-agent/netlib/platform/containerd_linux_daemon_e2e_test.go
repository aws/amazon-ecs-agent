//go:build e2e
// +build e2e

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

/*
End-to-end tests for the containerd platform's daemon network namespace
configuration. See managed_linux_daemon_e2e_test.go for environment
requirements (root, CNI_PATH pointing at ecs-bridge/ecs-ipam binaries, a
Linux host). These tests do not use IMDS; a stand-in task namespace is
attached to the bridge directly.

TestContainerdDaemonNetNSPhaseSplit runs the create and configure phases, then
attaches a task namespace and verifies routing and TCP connectivity between
the two. TestContainerdDaemonNetNSNegativeControl attaches the task namespace
before the daemon namespace exists and verifies the resulting failures.
*/
package platform

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

const (
	containerdE2EDaemonNSName = "host-daemon"
	containerdE2ETaskNSName   = "e2e-containerd-task"
	containerdE2ETestPort     = "42424"
	// daemonSubnetRoute is the connected route the task namespace must have
	// when the daemon namespace existed at bridge-attach time.
	daemonSubnetRoute = "169.254.172.0/22"
)

func newTestContainerdPlatformE2E(t *testing.T, stateDBDir string) *containerd {
	if os.Getenv("CNI_PATH") == "" {
		t.Skip("CNI_PATH not set, skipping e2e test")
	}
	if os.Geteuid() != 0 {
		t.Skip("not running as root, skipping e2e test")
	}
	return &containerd{
		common: common{
			nsUtil:            ecscni.NewNetNSUtil(),
			dnsVolumeAccessor: volume.NewTmpAccessor("containerd-e2e"),
			os:                oswrapper.NewOS(),
			ioutil:            ioutilwrapper.NewIOUtil(),
			netlink:           netlinkwrapper.New(),
			stateDBDir:        stateDBDir,
			cniClient:         ecscni.NewCNIClient([]string{os.Getenv("CNI_PATH")}),
			net:               netwrapper.NewNet(),
			resolvConfPath:    "/etc",
		},
	}
}

func containerdDaemonNetNSE2E(known, desired status.NetworkStatus) *tasknetworkconfig.NetworkNamespace {
	return &tasknetworkconfig.NetworkNamespace{
		Name:         "host-daemon",
		Path:         "/var/run/netns/" + containerdE2EDaemonNSName,
		NetworkMode:  "daemon-bridge",
		KnownState:   known,
		DesiredState: desired,
	}
}

// attachTaskNSToBridge creates the stand-in task namespace and attaches it to
// the bridge exactly the way an awsvpc task's primary interface attachment
// does (bridge + ipam plugin pair from createBridgePluginConfig, whose
// daemon-awareness is the property under test).
func attachTaskNSToBridge(t *testing.T, c *containerd, taskNSPath string) {
	require.NoError(t, c.CreateNetNS(taskNSPath))
	// Mirror the production interface-configuration path: the IPAM DB and
	// plugin log location are provided via environment variables. Using the
	// platform's per-test stateDBDir isolates allocations between tests.
	c.os.Setenv(CNIPluginLogFileEnv, ecscni.PluginLogPath)
	c.os.Setenv(IPAMDataPathEnv, filepath.Join(c.stateDBDir, IPAMDataFileName))
	bridgeConfig := c.createBridgePluginConfig(taskNSPath,
		ipcompatibility.NewIPv4OnlyCompatibility())
	_, err := c.executeCNIPlugin(context.TODO(), true, bridgeConfig)
	require.NoError(t, err, "task namespace bridge attach failed")
}

// taskNSHasDaemonSubnetRoute reports whether the task namespace's routing
// table contains the /22 connected route covering the daemon subnet.
func taskNSHasDaemonSubnetRoute(t *testing.T, taskNSPath string) bool {
	found := false
	ns, err := newNetNSFromPathE2E(taskNSPath)
	require.NoError(t, err)
	defer ns.Close()
	require.NoError(t, ns.Do(func() error {
		routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		for _, r := range routes {
			if r.Dst != nil && r.Dst.String() == daemonSubnetRoute {
				found = true
			}
		}
		return nil
	}))
	return found
}

// dialFromTaskNS attempts a TCP connection from the task namespace to a
// listener inside the daemon namespace, exercising the forward path and,
// via the TCP handshake, the return path.
func dialFromTaskNS(t *testing.T, taskNSPath, daemonAddr string) error {
	ns, err := newNetNSFromPathE2E(taskNSPath)
	require.NoError(t, err)
	defer ns.Close()
	return ns.Do(func() error {
		conn, err := net.DialTimeout("tcp", daemonAddr, 3*time.Second)
		if err != nil {
			return err
		}
		return conn.Close()
	})
}

// listenInDaemonNS starts a TCP listener bound inside the daemon namespace
// and returns a stop function.
func listenInDaemonNS(t *testing.T, daemonNSPath, addr string) func() {
	ns, err := newNetNSFromPathE2E(daemonNSPath)
	require.NoError(t, err)
	var ln net.Listener
	require.NoError(t, ns.Do(func() error {
		var lerr error
		ln, lerr = net.Listen("tcp", addr)
		return lerr
	}))
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	return func() {
		ln.Close()
		ns.Close()
	}
}

// netNSInterfaceIPv4 returns the first IPv4 address on the named interface
// inside the namespace.
func netNSInterfaceIPv4(t *testing.T, nsPath, ifName string) string {
	var addr string
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err)
	defer ns.Close()
	require.NoError(t, ns.Do(func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}
		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if len(addrs) > 0 {
			addr = addrs[0].IP.String()
		}
		return nil
	}))
	return addr
}

// TestContainerdDaemonNetNSPhaseSplit validates the two-phase daemon namespace
// setup sequence and end-to-end connectivity between the task and daemon
// namespaces over the bridge.
func TestContainerdDaemonNetNSPhaseSplit(t *testing.T) {
	stateDBDir := t.TempDir()
	c := newTestContainerdPlatformE2E(t, stateDBDir)

	daemonNSPath := "/var/run/netns/" + containerdE2EDaemonNSName
	taskNSPath := "/var/run/netns/" + containerdE2ETaskNSName
	cleanupStaleDaemonNSE2E(t, daemonNSPath)
	cleanupStaleDaemonNSE2E(t, taskNSPath)
	// E2E_PRESERVE_NAMESPACES leaves the configured namespaces on the host
	// for manual inspection (routing, reachability probes) after the test.
	if getEnvOrDefaultE2E("E2E_PRESERVE_NAMESPACES", "false") != "true" {
		defer cleanupStaleDaemonNSE2E(t, daemonNSPath)
		defer cleanupStaleDaemonNSE2E(t, taskNSPath)
	}

	netNS := containerdDaemonNetNSE2E(status.NetworkNone, status.NetworkReadyPull)
	netNS.Path = daemonNSPath

	// Phase 1: create. The namespace must exist afterwards; no bridge yet.
	require.NoError(t, c.ConfigureDaemonNetNS(netNS))
	exists, err := c.nsUtil.NSExists(daemonNSPath)
	require.NoError(t, err)
	require.True(t, exists, "daemon namespace must exist after the create phase")

	// Phase 2: configure. The daemon namespace gets its bridge veth with the
	// static daemon address and the credentials endpoint route.
	netNS.KnownState = status.NetworkReadyPull
	netNS.DesiredState = status.NetworkReady
	require.NoError(t, c.ConfigureDaemonNetNS(netNS))
	validateVethExistsInNSE2E(t, daemonNSPath, DaemonInterfaceName)
	validateECSCredentialsRouteE2E(t, daemonNSPath, DaemonInterfaceName)

	// The daemon veth holds the static daemon address.
	daemonIP := strings.Split(DaemonBridgeIP, "/")[0]
	assert.Equal(t, daemonIP, netNSInterfaceIPv4(t, daemonNSPath, DaemonInterfaceName),
		"daemon veth must own the static daemon address")

	// Idempotency: a second configure run is a no-op.
	require.NoError(t, c.ConfigureDaemonNetNS(netNS))

	// Task attach happens AFTER the daemon claim. It must observe the wide
	// connected route (daemon namespace exists) and must receive a dynamic
	// address that is NOT the daemon's.
	attachTaskNSToBridge(t, c, taskNSPath)
	require.True(t, taskNSHasDaemonSubnetRoute(t, taskNSPath),
		"task namespace must have the %s connected route when the daemon namespace exists",
		daemonSubnetRoute)
	taskIP := netNSInterfaceIPv4(t, taskNSPath, "eth0")
	assert.NotEqual(t, daemonIP, taskIP,
		"task veth must never receive the daemon's static address")
	assert.Equal(t, "169.254.172.3", taskIP,
		"task veth receives the first free dynamic address after the daemon claim")

	// Connectivity: TCP from the task namespace to the daemon's static
	// address; the handshake exercises the return path as well.
	stop := listenInDaemonNS(t, daemonNSPath, net.JoinHostPort(daemonIP, containerdE2ETestPort))
	defer stop()
	assert.NoError(t, dialFromTaskNS(t, taskNSPath, net.JoinHostPort(daemonIP, containerdE2ETestPort)),
		"task namespace must reach the daemon's bridge address")
}

// TestContainerdDaemonNetNSNegativeControl runs the flows in the reversed
// order, task attach before daemon create, and verifies the task lacks the
// daemon subnet route and the daemon's configuration fails.
func TestContainerdDaemonNetNSNegativeControl(t *testing.T) {
	stateDBDir := t.TempDir()
	c := newTestContainerdPlatformE2E(t, stateDBDir)

	daemonNSPath := "/var/run/netns/" + containerdE2EDaemonNSName
	taskNSPath := "/var/run/netns/" + containerdE2ETaskNSName
	cleanupStaleDaemonNSE2E(t, daemonNSPath)
	cleanupStaleDaemonNSE2E(t, taskNSPath)
	defer cleanupStaleDaemonNSE2E(t, daemonNSPath)
	defer cleanupStaleDaemonNSE2E(t, taskNSPath)

	// Task attaches to the bridge FIRST - no daemon namespace exists.
	attachTaskNSToBridge(t, c, taskNSPath)

	// Without the daemon namespace, the narrow bridge config applies.
	assert.False(t, taskNSHasDaemonSubnetRoute(t, taskNSPath),
		"task namespace must NOT have the daemon subnet route when no daemon namespace existed")

	// The task's dynamic allocation has taken the first free address - the
	// daemon's static address.
	daemonIP := strings.Split(DaemonBridgeIP, "/")[0]
	assert.Equal(t, daemonIP, netNSInterfaceIPv4(t, taskNSPath, "eth0"),
		"in the broken order the task takes the daemon's address")

	// The daemon's configuration must now fail: its address is taken.
	netNS := containerdDaemonNetNSE2E(status.NetworkNone, status.NetworkReadyPull)
	netNS.Path = daemonNSPath
	require.NoError(t, c.ConfigureDaemonNetNS(netNS))
	netNS.KnownState = status.NetworkReadyPull
	netNS.DesiredState = status.NetworkReady
	assert.Error(t, c.ConfigureDaemonNetNS(netNS),
		"daemon configuration must fail when its static address was dynamically allocated")
}
