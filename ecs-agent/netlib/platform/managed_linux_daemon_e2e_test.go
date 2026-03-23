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
Package platform provides end-to-end integration tests for the managed Linux
platform APIs that configure daemon network namespaces.

These tests are guarded by the "e2e" build tag so they are not included in the
default build or in automated CI unit-test runs. To execute them you must pass
"-tags e2e" explicitly to the go test command.

# Overview

These tests validate the integration between the ECS Agent's managed Linux
platform APIs (BuildTaskNetworkConfiguration, ConfigureDaemonNetNS,
StopDaemonNetNS) and the CNI plugins from amazon-ecs-cni-plugins (ecs-bridge
and ecs-ipam). Unlike daemon_bridge_e2e_test.go which directly invokes CNI
plugins, this test exercises the higher-level platform APIs to validate the
complete daemon network namespace configuration flow.

The test uses real EC2 metadata from the Instance Metadata Service (IMDS) to
retrieve actual host ENI information (MAC address, IP addresses, subnets).
This means tests must run on actual EC2 instances with access to the metadata
service.

# What These Tests Validate

  - BuildTaskNetworkConfiguration correctly builds network config for the
    daemon-bridge mode.
  - ConfigureDaemonNetNS creates a daemon namespace with the correct bridge,
    veth, and routes.
  - StopDaemonNetNS properly cleans up the veth while preserving the bridge.
  - IPv4-only, IPv6-only, and dual-stack host configurations are all supported.
  - The ECS credentials endpoint route (169.254.170.2) exists for every
    configuration.

# Test Requirements

The following requirements must be met to run these tests:

  - Root privileges: tests must be run as root (sudo) to create network
    namespaces.
  - CNI plugins: ecs-bridge and ecs-ipam binaries must be available.
  - CNI_PATH: environment variable must point to the directory containing the
    CNI plugin binaries.
  - EC2 instance: tests must run on an EC2 instance with access to IMDS.
  - Network capabilities: the host must have appropriate IP version support
    (IPv4, IPv6, or both).

# Environment Variables

The following environment variables control test behavior:

  - CNI_PATH (required): path to the directory containing ecs-bridge and
    ecs-ipam plugin binaries. Example: CNI_PATH=/opt/cni/bin
  - ECS_PRESERVE_E2E_TEST_LOGS: set to "true" to preserve CNI plugin logs
    after test completion. Useful for debugging test failures. Default: "false".
  - ECS_CNI_LOG_FILE: automatically set by the test to a temporary directory.
    Override to specify a custom log file location.
  - IPAM_DB_PATH: automatically set by the test to a temporary directory.
    Override to specify a custom IPAM database location.

# Running Tests Remotely

These tests are designed to run on remote EC2 instances via SSH. You need two
hosts: one with IPv4 connectivity for IPv4-only and dual-stack tests, and one
with IPv6-only connectivity for IPv6-only tests.

Step 1: Sync code to the remote host using the build-remote.sh script:

	./build-remote.sh

Step 2: Run tests on the IPv4 host:

	ssh <remote-ipv4-host> "cd /home/ssm-user/go/src/github.com/aws/amazon-ecs-agent/amazon-ecs-agent/ecs-agent && \
	    sudo -E CNI_PATH=/opt/cni/bin go test -v -tags e2e -timeout 120s -run TestManagedLinuxDaemonNetworkNamespace ./netlib/platform/..."

Step 3: Run tests on the IPv6 host:

	ssh <remote-ipv6-host> "cd /home/ssm-user/go/src/github.com/aws/amazon-ecs-agent/amazon-ecs-agent/ecs-agent && \
	    sudo -E CNI_PATH=/opt/cni/bin go test -v -tags e2e -timeout 120s -run TestManagedLinuxDaemonNetworkNamespace ./netlib/platform/..."

To preserve logs for debugging:

	ssh <remote-ipv4-host> "cd /home/ssm-user/go/src/github.com/aws/amazon-ecs-agent/amazon-ecs-agent/ecs-agent && \
	    sudo -E CNI_PATH=/opt/cni/bin ECS_PRESERVE_E2E_TEST_LOGS=true go test -v -tags e2e -timeout 120s -run TestManagedLinuxDaemonNetworkNamespace ./netlib/platform/..."

# Test Structure

The main test function TestManagedLinuxDaemonNetworkNamespace contains three
sub-tests:

  - IPv4Only: tests IPv4-only configuration with the ECS credentials endpoint
    route and an IPv4 default route.
  - IPv6Only: tests IPv6-only configuration with the ECS credentials endpoint
    route (IPv4 link-local) and an IPv6 default route.
  - DualStack: tests dual-stack configuration with both IPv4 and IPv6 routes.

Each sub-test follows this pattern:

 1. Setup: create temp directories for the IPAM database and logs.
 2. Create a managedLinux platform with a real EC2 metadata client.
 3. Build a task network configuration with the daemon-bridge mode.
 4. Execute ConfigureDaemonNetNS to configure the namespace.
 5. Validate network state: bridge, veth, addresses, routes.
 6. Execute StopDaemonNetNS for cleanup.
 7. Validate cleanup: veth removed, bridge persists.
 8. Cleanup: remove temp directories.

# Key Invariant: ECS Credentials Endpoint

The ECS credentials endpoint (169.254.170.2) is always accessed via IPv4
link-local, even on IPv6-only hosts. This is a critical invariant that the
tests validate. The IPv6Only sub-test specifically verifies that the IPv4
route to the ECS credentials endpoint exists even when the host only has IPv6
connectivity for external traffic.
*/
package platform

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"testing/quick"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/containernetworking/cni/pkg/invoke"
	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

// ============================================================================
// Real-time Logging Helper
// ============================================================================

// logf logs a message to both stdout for real-time visibility and t.Log for test output.
func logf(t *testing.T, format string, args ...interface{}) {
	t.Helper()
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	t.Log(msg)
}

// ============================================================================
// Stale Namespace Cleanup Helper
// ============================================================================

// cleanupStaleDaemonNSE2E removes a stale daemon network namespace left over
// from a previous test run. The isDaemonNamespaceConfigured check in
// ConfigureDaemonNetNS will skip CNI plugin setup if an eth0 veth already
// exists in the namespace, which causes the test to validate against stale
// network state. This function deletes the entire namespace so
// ConfigureDaemonNetNS recreates it fresh.
func cleanupStaleDaemonNSE2E(t *testing.T, nsPath string) {
	t.Helper()
	_, err := os.Stat(nsPath)
	if os.IsNotExist(err) {
		logf(t, "No stale namespace at %s, nothing to clean up", nsPath)
		return
	}
	if err != nil {
		logf(t, "Warning: failed to stat namespace %s: %v", nsPath, err)
		return
	}

	logf(t, "Cleaning up stale daemon namespace at %s", nsPath)

	// Unmount the bind-mounted namespace file.
	if err := syscall.Unmount(nsPath, syscall.MNT_DETACH); err != nil {
		logf(t, "Warning: failed to unmount stale namespace %s: %v (may already be unmounted)", nsPath, err)
	}

	// Remove the namespace file.
	if err := os.Remove(nsPath); err != nil && !os.IsNotExist(err) {
		logf(t, "Warning: failed to remove stale namespace file %s: %v", nsPath, err)
	}

	logf(t, "Stale daemon namespace cleaned up successfully")
}

// ============================================================================
// Test Constants
// ============================================================================

const (
	// managedDaemonBridgeNameE2E is the name of the bridge interface created
	// by the platform APIs. It matches the BridgeInterfaceName constant in
	// cniconf_linux.go.
	managedDaemonBridgeNameE2E = "fargate-bridge"

	// managedDaemonIfNameE2E is the name of the interface created inside the
	// daemon namespace.
	managedDaemonIfNameE2E = "eth0"

	// managedECSLinkLocalGatewayE2E is the gateway address on the bridge for
	// the ECS link-local subnet (169.254.172.0/22).
	managedECSLinkLocalGatewayE2E = "169.254.172.1"

	// managedECSCredentialsEndpointDstE2E is the destination for the ECS
	// credentials endpoint route.
	managedECSCredentialsEndpointDstE2E = "169.254.170.2/32"

	// managedExpectedBridgeIPv4E2E is the expected IPv4 address on the bridge.
	managedExpectedBridgeIPv4E2E = "169.254.172.1/22"
)

// ============================================================================
// Helper Function: Create Test ManagedLinux Platform
// ============================================================================

// newTestManagedLinuxPlatformE2E creates a managedLinux platform instance with
// a real EC2 metadata client. This function is used by E2E tests to create a
// platform that can interact with the actual EC2 Instance Metadata Service
// (IMDS) to retrieve host ENI information. The test is skipped if the EC2
// metadata service is not available.
func newTestManagedLinuxPlatformE2E(t *testing.T, stateDBDir string) (*managedLinux, error) {
	// Create a real EC2 metadata client.
	ec2Client, err := ec2.NewEC2MetadataClient(nil)
	if err != nil {
		t.Skipf("EC2 metadata service not available, skipping test: %v", err)
		return nil, err
	}

	// Verify we can actually reach IMDS by making a test request.
	_, err = ec2Client.GetMetadata(MacResource)
	if err != nil {
		t.Skipf("Cannot reach EC2 metadata service, skipping test: %v", err)
		return nil, err
	}

	// Create the common platform struct with real implementations.
	commonPlatform := common{
		nsUtil:            ecscni.NewNetNSUtil(),
		dnsVolumeAccessor: volume.NewTmpAccessor("managed-linux-e2e"),
		os:                oswrapper.NewOS(),
		ioutil:            ioutilwrapper.NewIOUtil(),
		netlink:           netlinkwrapper.New(),
		stateDBDir:        stateDBDir,
		cniClient:         ecscni.NewCNIClient([]string{os.Getenv("CNI_PATH")}),
		net:               netwrapper.NewNet(),
		resolvConfPath:    "/etc",
	}

	return &managedLinux{
		common: commonPlatform,
		client: ec2Client,
	}, nil
}

// ============================================================================
// Bridge Validation Helpers
// ============================================================================

// validateBridgeExistsE2E verifies that the bridge interface exists and returns
// the bridge link for further validation.
func validateBridgeExistsE2E(t *testing.T, bridgeName string) netlink.Link {
	bridge, err := netlink.LinkByName(bridgeName)
	require.NoError(t, err, "Bridge interface '%s' should exist", bridgeName)
	require.NotNil(t, bridge, "Bridge interface should not be nil")
	return bridge
}

// validateBridgeIPv4AddressE2E verifies that the bridge has the expected IPv4
// address assigned.
func validateBridgeIPv4AddressE2E(t *testing.T, bridge netlink.Link, expectedAddr string) {
	addrs, err := netlink.AddrList(bridge, netlink.FAMILY_V4)
	require.NoError(t, err, "Unable to list the IPv4 addresses of: %s", bridge.Attrs().Name)
	addressFound := false
	for _, addr := range addrs {
		if addr.IPNet.String() == expectedAddr {
			addressFound = true
			break
		}
	}
	require.True(t, addressFound, "IPv4 address '%s' not assigned to bridge: %s",
		expectedAddr, bridge.Attrs().Name)
}

// validateBridgeIPv6AddressE2E verifies that the bridge has at least one
// non-link-local IPv6 address assigned. For IPv6-compatible hosts, the bridge
// should have an IPv6 gateway address.
func validateBridgeIPv6AddressE2E(t *testing.T, bridge netlink.Link) {
	addrs, err := netlink.AddrList(bridge, netlink.FAMILY_V6)
	require.NoError(t, err, "Unable to list the IPv6 addresses of: %s", bridge.Attrs().Name)

	// Filter out link-local addresses (fe80::).
	var globalAddrs []netlink.Addr
	for _, addr := range addrs {
		if !addr.IP.IsLinkLocalUnicast() {
			globalAddrs = append(globalAddrs, addr)
		}
	}

	require.NotEmpty(t, globalAddrs, "Bridge '%s' should have at least one non-link-local IPv6 address",
		bridge.Attrs().Name)
}

// ============================================================================
// Veth and Namespace Validation Helpers
// ============================================================================

// validateVethExistsInNSE2E verifies that a veth interface exists in the
// specified namespace and returns the veth link for further validation.
func validateVethExistsInNSE2E(t *testing.T, nsPath string, ifName string) netlink.Link {
	var veth netlink.Link
	var vethFound bool

	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}
		if link.Type() == "veth" {
			veth = link
			vethFound = true
		}
		return nil
	})
	require.NoError(t, err, "Failed to find veth interface '%s' in namespace %s", ifName, nsPath)
	require.True(t, vethFound, "Interface '%s' is not a veth in namespace %s", ifName, nsPath)

	return veth
}

// validateVethIPv4AddressE2E verifies that the veth has the expected static
// IPv4 address. With the current IPAM configuration, the daemon veth gets a
// static IP of 169.254.172.2/32 from the ECS subnet (169.254.172.0/22).
func validateVethIPv4AddressE2E(t *testing.T, nsPath string, ifName string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}

		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}

		// Check that the veth has the expected static IP address (169.254.172.2/32).
		// The IPAM plugin assigns this address from the DaemonBridgeIP constant.
		addressFound := false
		for _, addr := range addrs {
			ip := addr.IP.To4()
			if ip != nil && ip[0] == 169 && ip[1] == 254 && ip[2] >= 172 && ip[2] <= 175 {
				addressFound = true
				logf(t, "Veth '%s' has IPv4 address: %s", ifName, addr.IPNet.String())
				break
			}
		}
		require.True(t, addressFound, "Veth '%s' should have an IPv4 address from ECS subnet (169.254.172.0/22)", ifName)
		return nil
	})
	require.NoError(t, err, "Failed to validate veth IPv4 address")
}

// validateVethIPv6AddressE2E verifies that the veth has at least one
// non-link-local IPv6 address.
func validateVethIPv6AddressE2E(t *testing.T, nsPath string, ifName string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}

		addrs, err := netlink.AddrList(link, netlink.FAMILY_V6)
		if err != nil {
			return err
		}

		// Filter out link-local addresses.
		var globalAddrs []netlink.Addr
		for _, addr := range addrs {
			if !addr.IP.IsLinkLocalUnicast() {
				globalAddrs = append(globalAddrs, addr)
			}
		}

		require.NotEmpty(t, globalAddrs, "Veth '%s' should have at least one non-link-local IPv6 address", ifName)
		return nil
	})
	require.NoError(t, err, "Failed to validate veth IPv6 address")
}

// validateLoopbackUpE2E verifies that the loopback interface is UP in the
// namespace.
func validateLoopbackUpE2E(t *testing.T, nsPath string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		lo, err := netlink.LinkByName("lo")
		if err != nil {
			return fmt.Errorf("loopback interface not found: %v", err)
		}

		// Check if the interface is UP.
		if lo.Attrs().Flags&0x1 == 0 { // IFF_UP = 0x1
			return fmt.Errorf("loopback interface is not UP")
		}
		return nil
	})
	require.NoError(t, err, "Loopback interface should be UP in namespace %s", nsPath)
}

// newNetNSFromPathE2E opens an existing network namespace from a path. This is
// a helper to work with namespaces created by the platform APIs. Unlike
// netNSE2E created by newNetNSE2E, this does not clean up the namespace when
// Close is called, since the namespace is managed by the platform APIs.
func newNetNSFromPathE2E(nsPath string) (*managedNetNSE2E, error) {
	// Check if the namespace exists using Stat.
	_, err := os.Stat(nsPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("namespace path does not exist: %s", nsPath)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat namespace path %s: %v", nsPath, err)
	}

	return &managedNetNSE2E{
		path: nsPath,
	}, nil
}

// managedNetNSE2E represents a network namespace that is managed by the
// platform APIs. Unlike netNSE2E, this does not clean up the namespace when
// Close is called.
type managedNetNSE2E struct {
	path string
}

// Path returns the path to the network namespace.
func (n *managedNetNSE2E) Path() string {
	return n.path
}

// Close is a no-op for managed namespaces since they are managed by the
// platform APIs.
func (n *managedNetNSE2E) Close() error {
	return nil
}

// Do executes a function in the network namespace using cnins.WithNetNSPath.
func (n *managedNetNSE2E) Do(toRun func() error) error {
	return cnins.WithNetNSPath(n.path, func(_ cnins.NetNS) error {
		return toRun()
	})
}

// ============================================================================
// Route Validation Helpers
// ============================================================================

// validateECSCredentialsRouteE2E verifies that the route to the ECS
// credentials endpoint exists. This is the critical invariant: ECS credentials
// are always accessed via IPv4 link-local. The route may appear with an
// explicit gateway (169.254.172.1) when the bridge plugin resolves it, or
// without a gateway when the IPAM plugin uses connected subnet routing with
// ConnectedSubnetMaskSize. Both forms are valid.
func validateECSCredentialsRouteE2E(t *testing.T, nsPath string, ifName string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}

		routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}

		ecsCredentialsRouteFound := false
		for _, route := range routes {
			// Check for ECS credentials endpoint route (169.254.170.2/32).
			// Accept the route with or without an explicit gateway. The bridge plugin
			// may resolve the gateway automatically, or the route may rely on the
			// connected subnet route added by ConnectedSubnetMaskSize in the IPAM config.
			if route.Dst != nil && route.Dst.String() == managedECSCredentialsEndpointDstE2E {
				ecsCredentialsRouteFound = true
				break
			}
		}

		require.True(t, ecsCredentialsRouteFound,
			"Route to ECS credentials endpoint '%s' not found for: %s",
			managedECSCredentialsEndpointDstE2E, ifName)
		return nil
	})
	require.NoError(t, err, "Failed to validate ECS credentials route")
}

// validateIPv4DefaultRouteE2E verifies that an IPv4 default route exists. The
// default route (0.0.0.0/0) should be present with a gateway pointing to the
// bridge gateway address.
func validateIPv4DefaultRouteE2E(t *testing.T, nsPath string, ifName string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}

		routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}

		defaultRouteFound := false
		for _, route := range routes {
			// Check for default route (0.0.0.0/0) via any gateway.
			isDefaultRoute := route.Dst == nil || (route.Dst != nil && route.Dst.String() == "0.0.0.0/0")
			if isDefaultRoute && route.Gw != nil {
				defaultRouteFound = true
				break
			}
		}

		require.True(t, defaultRouteFound,
			"IPv4 default route (0.0.0.0/0) not found for: %s", ifName)
		return nil
	})
	require.NoError(t, err, "Failed to validate IPv4 default route")
}

// validateIPv6DefaultRouteE2E verifies that an IPv6 default route exists. The
// default route (::/0) should be present with a gateway.
func validateIPv6DefaultRouteE2E(t *testing.T, nsPath string, ifName string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}

		routes, err := netlink.RouteList(link, netlink.FAMILY_V6)
		if err != nil {
			return err
		}

		defaultRouteFound := false
		for _, route := range routes {
			// Check for IPv6 default route (::/0).
			isDefaultRoute := route.Dst == nil || (route.Dst != nil && route.Dst.String() == "::/0")
			if isDefaultRoute && route.Gw != nil {
				defaultRouteFound = true
				break
			}
		}

		require.True(t, defaultRouteFound,
			"IPv6 default route (::/0) not found for: %s", ifName)
		return nil
	})
	require.NoError(t, err, "Failed to validate IPv6 default route")
}

// validateNoIPv4DefaultRouteE2E verifies that no IPv4 default route exists.
// This is used for IPv6-only hosts where external traffic should only route
// via IPv6.
func validateNoIPv4DefaultRouteE2E(t *testing.T, nsPath string, ifName string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return err
		}

		routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}

		for _, route := range routes {
			// Check for default route (0.0.0.0/0).
			isDefaultRoute := route.Dst == nil || (route.Dst != nil && route.Dst.String() == "0.0.0.0/0")
			if isDefaultRoute && route.Gw != nil {
				logf(t, "Found unexpected IPv4 default route: dst=%v gw=%v", route.Dst, route.Gw)
				require.Fail(t, "IPv4 default route should NOT exist on IPv6-only host")
			}
		}
		return nil
	})
	require.NoError(t, err, "Failed to validate absence of IPv4 default route")
}

// ============================================================================
// Cleanup Validation Helpers
// ============================================================================

// validateVethRemovedE2E verifies that the veth interface no longer exists in
// the namespace after cleanup.
func validateVethRemovedE2E(t *testing.T, nsPath string, ifName string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		_, err := netlink.LinkByName(ifName)
		if err == nil {
			return fmt.Errorf("veth interface '%s' should have been removed", ifName)
		}
		// LinkNotFoundError is expected.
		_, ok := err.(netlink.LinkNotFoundError)
		if !ok {
			return fmt.Errorf("unexpected error type when checking for removed veth: %v", err)
		}
		return nil
	})
	require.NoError(t, err, "Veth interface should be removed after cleanup")
}

// validateBridgePersistsE2E verifies that the bridge still exists with its
// addresses after cleanup.
func validateBridgePersistsE2E(t *testing.T, bridgeName string) {
	bridge, err := netlink.LinkByName(bridgeName)
	require.NoError(t, err, "Bridge '%s' should still exist after cleanup", bridgeName)
	require.NotNil(t, bridge, "Bridge should not be nil")

	// Verify that the bridge still has an IPv4 address.
	addrs, err := netlink.AddrList(bridge, netlink.FAMILY_V4)
	require.NoError(t, err, "Unable to list IPv4 addresses of bridge after cleanup")
	require.NotEmpty(t, addrs, "Bridge should retain IPv4 addresses after cleanup")
}

// ============================================================================
// Debugging Helper Function
// ============================================================================

// logNetworkStateInNSE2E logs all interfaces, addresses, and routes in a
// namespace. This is called on test failure for diagnostics.
func logNetworkStateInNSE2E(t *testing.T, nsPath string, description string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	if err != nil {
		logf(t, "=== Network State: %s (failed to open namespace: %v) ===", description, err)
		return
	}
	defer ns.Close()

	ns.Do(func() error {
		logf(t, "=== Network State: %s ===", description)

		links, err := netlink.LinkList()
		if err != nil {
			logf(t, "Error listing interfaces: %v", err)
			return nil
		}

		for _, link := range links {
			logf(t, "Interface: %s (type: %s, state: %s)",
				link.Attrs().Name, link.Type(), link.Attrs().OperState)

			addrsV4, err := netlink.AddrList(link, netlink.FAMILY_V4)
			if err != nil {
				logf(t, "  Error listing IPv4 addresses: %v", err)
			} else {
				for _, addr := range addrsV4 {
					logf(t, "  IPv4: %s", addr.IPNet.String())
				}
			}

			addrsV6, err := netlink.AddrList(link, netlink.FAMILY_V6)
			if err != nil {
				logf(t, "  Error listing IPv6 addresses: %v", err)
			} else {
				for _, addr := range addrsV6 {
					logf(t, "  IPv6: %s", addr.IPNet.String())
				}
			}
		}

		routesV4, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err != nil {
			logf(t, "Error listing IPv4 routes: %v", err)
		} else {
			logf(t, "IPv4 Routes:")
			for _, route := range routesV4 {
				logf(t, "  dst=%v gw=%v", route.Dst, route.Gw)
			}
		}

		routesV6, err := netlink.RouteList(nil, netlink.FAMILY_V6)
		if err != nil {
			logf(t, "Error listing IPv6 routes: %v", err)
		} else {
			logf(t, "IPv6 Routes:")
			for _, route := range routesV6 {
				logf(t, "  dst=%v gw=%v", route.Dst, route.Gw)
			}
		}

		return nil
	})
}

// ============================================================================
// Main Test Function
// ============================================================================

// TestManagedLinuxDaemonNetworkNamespace tests the managed Linux platform APIs
// for daemon network namespace configuration.
//
// This test exercises the higher-level platform APIs:
//   - BuildTaskNetworkConfiguration with network mode daemon-bridge.
//   - ConfigureDaemonNetNS to configure the daemon network namespace.
//   - StopDaemonNetNS to clean up the daemon network namespace.
//
// The test uses real EC2 metadata from IMDS to retrieve actual host ENI
// information.
//
// Test structure:
//   - Common setup: log directory, IPAM database, plugin path discovery, and
//     IP compatibility detection.
//   - IPv4Only sub-test: tests IPv4-only configuration.
//   - IPv6Only sub-test: tests IPv6-only configuration.
//   - DualStack sub-test: tests dual-stack configuration.
func TestManagedLinuxDaemonNetworkNamespace(t *testing.T) {
	// ========================================================================
	// Common Setup
	// ========================================================================

	// Detect host IP compatibility using existing helper functions.
	hasIPv4 := instanceSupportsIPv4E2E()
	hasIPv6 := instanceSupportsIPv6E2E()

	logf(t, "Host IP version support: IPv4=%v, IPv6=%v", hasIPv4, hasIPv6)

	if !hasIPv4 && !hasIPv6 {
		t.Skip("Skipping test: host supports neither IPv4 nor IPv6")
	}

	// Ensure that the bridge plugin exists.
	bridgePluginPath, err := invoke.FindInPath("ecs-bridge", []string{os.Getenv("CNI_PATH")})
	if err != nil {
		t.Skipf("Unable to find bridge plugin in path. Set CNI_PATH environment variable: %v", err)
	}
	logf(t, "Using bridge plugin: %s", bridgePluginPath)

	// Create a directory for storing test logs.
	testLogDir, err := ioutil.TempDir("", "managed-linux-daemon-e2e-")
	require.NoError(t, err, "Unable to create directory for storing test logs")

	os.Setenv("ECS_CNI_LOG_FILE", fmt.Sprintf("%s/bridge.log", testLogDir))
	logf(t, "Using %s for test logs", testLogDir)
	defer os.Unsetenv("ECS_CNI_LOG_FILE")

	preserveLogs, err := strconv.ParseBool(getEnvOrDefaultE2E("ECS_PRESERVE_E2E_TEST_LOGS", "false"))
	assert.NoError(t, err, "Unable to parse ECS_PRESERVE_E2E_TEST_LOGS env var")
	defer func(preserve bool) {
		if !t.Failed() && !preserve {
			os.RemoveAll(testLogDir)
		}
	}(preserveLogs)

	// ========================================================================
	// IPv4-only sub-test
	// ========================================================================
	t.Run("IPv4Only", func(t *testing.T) {
		skipIfNoIPv4E2E(t)
		logf(t, "Running IPv4-only managed Linux daemon namespace test")

		// Create a temp directory for the IPAM database.
		ipamDir, err := ioutil.TempDir("", "managed-linux-ipam-ipv4-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		defer os.RemoveAll(ipamDir)

		// Create a test managedLinux platform with a real EC2 client.
		platform, err := newTestManagedLinuxPlatformE2E(t, ipamDir)
		require.NoError(t, err, "Failed to create test platform")

		// Clean up any stale daemon namespace from previous test runs so that
		// isDaemonNamespaceConfigured returns false and ConfigureDaemonNetNS
		// runs the CNI plugin fresh.
		cleanupStaleDaemonNSE2E(t, platform.common.nsUtil.GetNetNSPath("host-daemon"))

		// Build a task payload with NetworkMode daemon-bridge.
		taskPayload := &ecsacs.Task{
			NetworkMode: aws.String("daemon-bridge"),
		}

		// Call BuildTaskNetworkConfiguration.
		taskNetConfig, err := platform.BuildTaskNetworkConfiguration("test-task-ipv4", taskPayload)
		require.NoError(t, err, "BuildTaskNetworkConfiguration should succeed")
		require.NotNil(t, taskNetConfig, "TaskNetworkConfig should not be nil")
		require.Len(t, taskNetConfig.NetworkNamespaces, 1, "Should have exactly one network namespace")

		netNS := taskNetConfig.NetworkNamespaces[0]
		logf(t, "Network namespace path: %s", netNS.Path)

		// Call ConfigureDaemonNetNS to configure the namespace.
		err = platform.ConfigureDaemonNetNS(netNS)
		require.NoError(t, err, "ConfigureDaemonNetNS should succeed")

		// Validate that the bridge exists with an IPv4 address.
		bridge := validateBridgeExistsE2E(t, managedDaemonBridgeNameE2E)
		validateBridgeIPv4AddressE2E(t, bridge, managedExpectedBridgeIPv4E2E)

		// Validate that the veth exists with an IPv4 address from the ECS subnet.
		validateVethExistsInNSE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv4AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that the ECS credentials route exists.
		validateECSCredentialsRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that the IPv4 default route exists.
		validateIPv4DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that the loopback interface is UP.
		validateLoopbackUpE2E(t, netNS.Path)

		// Log network state for debugging.
		logNetworkStateInNSE2E(t, netNS.Path, "After ConfigureDaemonNetNS - IPv4Only")

		// Call StopDaemonNetNS for cleanup.
		ctx := context.Background()
		err = platform.StopDaemonNetNS(ctx, netNS)
		require.NoError(t, err, "StopDaemonNetNS should succeed")

		// Validate that the veth was removed and the bridge persists.
		validateVethRemovedE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateBridgePersistsE2E(t, managedDaemonBridgeNameE2E)

		logf(t, "IPv4-only managed Linux daemon namespace test completed successfully")
	})

	// ========================================================================
	// IPv6-only sub-test
	// ========================================================================
	t.Run("IPv6Only", func(t *testing.T) {
		skipIfNotIPv6OnlyE2E(t)
		logf(t, "Running IPv6-only managed Linux daemon namespace test")
		logf(t, "NOTE: This test uses IPv4 link-local for ECS credentials + IPv6 for external traffic")

		// Create a temp directory for the IPAM database.
		ipamDir, err := ioutil.TempDir("", "managed-linux-ipam-ipv6-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		defer os.RemoveAll(ipamDir)

		// Create a test managedLinux platform with a real EC2 client.
		platform, err := newTestManagedLinuxPlatformE2E(t, ipamDir)
		require.NoError(t, err, "Failed to create test platform")

		// Clean up any stale daemon namespace from previous test runs so that
		// isDaemonNamespaceConfigured returns false and ConfigureDaemonNetNS
		// runs the CNI plugin fresh.
		cleanupStaleDaemonNSE2E(t, platform.common.nsUtil.GetNetNSPath("host-daemon"))

		// Build a task payload with NetworkMode daemon-bridge.
		taskPayload := &ecsacs.Task{
			NetworkMode: aws.String("daemon-bridge"),
		}

		// Call BuildTaskNetworkConfiguration.
		taskNetConfig, err := platform.BuildTaskNetworkConfiguration("test-task-ipv6", taskPayload)
		require.NoError(t, err, "BuildTaskNetworkConfiguration should succeed")
		require.NotNil(t, taskNetConfig, "TaskNetworkConfig should not be nil")
		require.Len(t, taskNetConfig.NetworkNamespaces, 1, "Should have exactly one network namespace")

		netNS := taskNetConfig.NetworkNamespaces[0]
		logf(t, "Network namespace path: %s", netNS.Path)

		// Call ConfigureDaemonNetNS to configure the namespace.
		err = platform.ConfigureDaemonNetNS(netNS)
		require.NoError(t, err, "ConfigureDaemonNetNS should succeed")

		// Validate that the bridge exists with both IPv4 and IPv6 addresses.
		bridge := validateBridgeExistsE2E(t, managedDaemonBridgeNameE2E)
		validateBridgeIPv4AddressE2E(t, bridge, managedExpectedBridgeIPv4E2E)
		validateBridgeIPv6AddressE2E(t, bridge)

		// Validate that the veth exists with both IPv4 and IPv6 addresses.
		validateVethExistsInNSE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv4AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv6AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that the ECS credentials route exists via IPv4 link-local.
		validateECSCredentialsRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that the IPv6 default route exists.
		validateIPv6DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that no IPv4 default route exists on this IPv6-only host.
		validateNoIPv4DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that the loopback interface is UP.
		validateLoopbackUpE2E(t, netNS.Path)

		// Log network state for debugging.
		logNetworkStateInNSE2E(t, netNS.Path, "After ConfigureDaemonNetNS - IPv6Only")

		// Call StopDaemonNetNS for cleanup.
		ctx := context.Background()
		err = platform.StopDaemonNetNS(ctx, netNS)
		require.NoError(t, err, "StopDaemonNetNS should succeed")

		// Validate that the veth was removed and the bridge persists.
		validateVethRemovedE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateBridgePersistsE2E(t, managedDaemonBridgeNameE2E)

		logf(t, "IPv6-only managed Linux daemon namespace test completed successfully")
	})

	// ========================================================================
	// Dual-stack sub-test
	// ========================================================================
	t.Run("DualStack", func(t *testing.T) {
		skipIfNoDualStackE2E(t)
		logf(t, "Running dual-stack managed Linux daemon namespace test")
		logf(t, "NOTE: This test uses IPv4 link-local for ECS credentials + both IPv4 and IPv6 for external traffic")

		// Create a temp directory for the IPAM database.
		ipamDir, err := ioutil.TempDir("", "managed-linux-ipam-dualstack-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		defer os.RemoveAll(ipamDir)

		// Create a test managedLinux platform with a real EC2 client.
		platform, err := newTestManagedLinuxPlatformE2E(t, ipamDir)
		require.NoError(t, err, "Failed to create test platform")

		// Clean up any stale daemon namespace from previous test runs so that
		// isDaemonNamespaceConfigured returns false and ConfigureDaemonNetNS
		// runs the CNI plugin fresh.
		cleanupStaleDaemonNSE2E(t, platform.common.nsUtil.GetNetNSPath("host-daemon"))

		// Build a task payload with NetworkMode daemon-bridge.
		taskPayload := &ecsacs.Task{
			NetworkMode: aws.String("daemon-bridge"),
		}

		// Call BuildTaskNetworkConfiguration.
		taskNetConfig, err := platform.BuildTaskNetworkConfiguration("test-task-dualstack", taskPayload)
		require.NoError(t, err, "BuildTaskNetworkConfiguration should succeed")
		require.NotNil(t, taskNetConfig, "TaskNetworkConfig should not be nil")
		require.Len(t, taskNetConfig.NetworkNamespaces, 1, "Should have exactly one network namespace")

		netNS := taskNetConfig.NetworkNamespaces[0]
		logf(t, "Network namespace path: %s", netNS.Path)

		// Call ConfigureDaemonNetNS to configure the namespace.
		err = platform.ConfigureDaemonNetNS(netNS)
		require.NoError(t, err, "ConfigureDaemonNetNS should succeed")

		// Validate that the bridge exists with both IPv4 and IPv6 addresses.
		bridge := validateBridgeExistsE2E(t, managedDaemonBridgeNameE2E)
		validateBridgeIPv4AddressE2E(t, bridge, managedExpectedBridgeIPv4E2E)
		validateBridgeIPv6AddressE2E(t, bridge)

		// Validate that the veth exists with both IPv4 and IPv6 addresses.
		validateVethExistsInNSE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv4AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv6AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that the ECS credentials route exists.
		validateECSCredentialsRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that both IPv4 and IPv6 default routes exist.
		validateIPv4DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateIPv6DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate that the loopback interface is UP.
		validateLoopbackUpE2E(t, netNS.Path)

		// Log network state for debugging.
		logNetworkStateInNSE2E(t, netNS.Path, "After ConfigureDaemonNetNS - DualStack")

		// Call StopDaemonNetNS for cleanup.
		ctx := context.Background()
		err = platform.StopDaemonNetNS(ctx, netNS)
		require.NoError(t, err, "StopDaemonNetNS should succeed")

		// Validate that the veth was removed and the bridge persists.
		validateVethRemovedE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateBridgePersistsE2E(t, managedDaemonBridgeNameE2E)

		logf(t, "Dual-stack managed Linux daemon namespace test completed successfully")
	})
}

// ============================================================================
// Concurrent Setup Race Condition Test
// ============================================================================

// TestConcurrentDaemonNetworkNamespaceSetup verifies that ConfigureDaemonNetNS
// handles concurrent invocations correctly. All daemon tasks share the same
// "host-daemon" namespace path, and this test launches multiple goroutines
// calling ConfigureDaemonNetNS simultaneously to confirm that the mutex
// serialization prevents CNI plugin conflicts on shared resources (veth,
// bridge, IMDS route).
func TestConcurrentDaemonNetworkNamespaceSetup(t *testing.T) {
	// Number of concurrent goroutines to launch. This matches the approximate
	// number of concurrent daemon tasks observed in production (~14).
	const numGoroutines = 10

	// Create a temporary directory for IPAM database and state.
	ipamDir, err := ioutil.TempDir("", "managed-linux-concurrent-race-")
	require.NoError(t, err, "Unable to create a temp directory for the ipam db")
	os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
	defer os.Unsetenv("IPAM_DB_PATH")
	defer os.RemoveAll(ipamDir)

	// Create a temporary directory for CNI plugin logs.
	testLogDir, err := ioutil.TempDir("", "managed-linux-concurrent-race-logs-")
	require.NoError(t, err, "Unable to create directory for storing test logs")
	os.Setenv("ECS_CNI_LOG_FILE", fmt.Sprintf("%s/bridge.log", testLogDir))
	defer os.Unsetenv("ECS_CNI_LOG_FILE")
	defer func() {
		if !t.Failed() {
			os.RemoveAll(testLogDir)
		}
	}()

	// Create a shared managedLinux platform instance. All goroutines share
	// this platform, just like in production where all daemon tasks on the
	// same instance share the same platform.
	platform, err := newTestManagedLinuxPlatformE2E(t, ipamDir)
	require.NoError(t, err, "Failed to create test platform")

	// Build N daemon task network configurations with unique task IDs. Each
	// configuration targets the same shared "host-daemon" namespace path,
	// which is the root cause of the race condition.
	taskPayload := &ecsacs.Task{
		NetworkMode: aws.String("daemon-bridge"),
	}

	// Build network configurations for all goroutines before starting the race.
	netNSList := make([]*tasknetworkconfig.NetworkNamespace, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		taskID := fmt.Sprintf("daemon-race-%d", i)
		taskNetConfig, err := platform.BuildTaskNetworkConfiguration(taskID, taskPayload)
		require.NoError(t, err, "BuildTaskNetworkConfiguration should succeed for task %s", taskID)
		require.NotNil(t, taskNetConfig, "TaskNetworkConfig should not be nil for task %s", taskID)
		require.Len(t, taskNetConfig.NetworkNamespaces, 1,
			"Should have exactly one network namespace for task %s", taskID)
		netNSList[i] = taskNetConfig.NetworkNamespaces[0]
	}

	logf(t, "Built %d daemon task network configurations, all targeting namespace: %s",
		numGoroutines, netNSList[0].Path)

	// Clean up any stale namespace from previous test runs so that
	// isDaemonNamespaceConfigured returns false and all goroutines attempt
	// the full CNI plugin setup path.
	cleanupStaleDaemonNSE2E(t, platform.common.nsUtil.GetNetNSPath("host-daemon"))

	// Launch N concurrent goroutines, each calling ConfigureDaemonNetNS on
	// its own netNS object. All netNS objects point to the same "host-daemon"
	// path, so the goroutines exercise the synchronized
	// isDaemonNamespaceConfigured check-then-act pattern.
	results := make([]error, numGoroutines)
	var mu sync.Mutex
	var wg sync.WaitGroup

	logf(t, "Launching %d concurrent goroutines calling ConfigureDaemonNetNS()...", numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := platform.ConfigureDaemonNetNS(netNSList[idx])
			mu.Lock()
			results[idx] = err
			mu.Unlock()
		}(i)
	}

	// Wait for all goroutines to complete.
	wg.Wait()

	// Count successes and failures, and log all errors for diagnostics.
	successCount := 0
	failureCount := 0
	for i, err := range results {
		if err != nil {
			failureCount++
			logf(t, "Goroutine %d (task daemon-race-%d) FAILED: %v", i, i, err)
		} else {
			successCount++
			logf(t, "Goroutine %d (task daemon-race-%d) SUCCEEDED", i, i)
		}
	}

	logf(t, "Concurrent setup results: %d succeeded, %d failed (out of %d total)",
		successCount, failureCount, numGoroutines)

	// Assert that all goroutines succeed now that the mutex serializes
	// concurrent calls to ConfigureDaemonNetNS. The first goroutine acquires
	// the lock and performs CNI setup; subsequent goroutines wait, then see
	// isDaemonNamespaceConfigured == true and skip setup.
	assert.Equal(t, 0, failureCount,
		"All goroutines should succeed after mutex serialization fix. "+
			"If any goroutine failed, the fix may be incomplete or the mutex scope is incorrect.")

	// Validate that the bridge interface exists after the concurrent setup.
	// At least one goroutine should succeed and leave the namespace in a
	// valid state with a bridge interface.
	validateBridgeExistsE2E(t, managedDaemonBridgeNameE2E)
	logf(t, "Bridge '%s' exists after concurrent setup (at least one goroutine succeeded)",
		managedDaemonBridgeNameE2E)

	// Call StopDaemonNetNS to clean up the namespace after the test.
	ctx := context.Background()
	err = platform.StopDaemonNetNS(ctx, netNSList[0])
	if err != nil {
		logf(t, "Warning: StopDaemonNetNS returned error during cleanup: %v", err)
	}

	// Validate that the bridge persists after cleanup, confirming the
	// immutable infrastructure pattern is preserved.
	validateBridgePersistsE2E(t, managedDaemonBridgeNameE2E)
	logf(t, "Bridge '%s' persists after StopDaemonNetNS cleanup", managedDaemonBridgeNameE2E)

	logf(t, "Concurrent daemon network namespace setup race condition test completed")
}

// ============================================================================
// Property-Based Bug Condition Exploration Test
// ============================================================================

// goroutineCount is a custom type for testing/quick that generates random
// goroutine counts in the range [2, 20]. This range covers the minimum
// concurrency needed to trigger the race (2) up to a value exceeding
// production levels (~14 concurrent daemon tasks).
type goroutineCount struct {
	N int
}

// Generate implements the quick.Generator interface for goroutineCount.
// It produces random values in the range [2, 20] to explore different
// concurrency levels that trigger the race condition.
func (goroutineCount) Generate(rand *rand.Rand, size int) reflect.Value {
	n := rand.Intn(19) + 2 // Range [2, 20].
	return reflect.ValueOf(goroutineCount{N: n})
}

// TestConcurrentDaemonNetworkNamespaceSetupProperty is a property-based test
// that verifies ConfigureDaemonNetNS behaves correctly under concurrent access.
// For any goroutine count N in [2, 20], launching N concurrent goroutines
// calling ConfigureDaemonNetNS on the same unconfigured "host-daemon" namespace
// should result in all goroutines returning without error, and the namespace
// should have a valid bridge and veth after completion.
func TestConcurrentDaemonNetworkNamespaceSetupProperty(t *testing.T) {
	// Create a temporary directory for IPAM database and state.
	ipamDir, err := ioutil.TempDir("", "managed-linux-concurrent-pbt-")
	require.NoError(t, err, "Unable to create a temp directory for the ipam db")
	os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
	defer os.Unsetenv("IPAM_DB_PATH")
	defer os.RemoveAll(ipamDir)

	// Create a temporary directory for CNI plugin logs.
	testLogDir, err := ioutil.TempDir("", "managed-linux-concurrent-pbt-logs-")
	require.NoError(t, err, "Unable to create directory for storing test logs")
	os.Setenv("ECS_CNI_LOG_FILE", fmt.Sprintf("%s/bridge.log", testLogDir))
	defer os.Unsetenv("ECS_CNI_LOG_FILE")
	defer func() {
		if !t.Failed() {
			os.RemoveAll(testLogDir)
		}
	}()

	// Use a small number of iterations because each iteration creates real
	// network namespaces and invokes CNI plugins, which is expensive.
	config := &quick.Config{
		MaxCount: 3,
	}

	logf(t, "Starting property-based exploration of concurrent ConfigureDaemonNetNS race condition")

	// The property function: for any goroutine count N in [2, 20], all N
	// concurrent goroutines calling ConfigureDaemonNetNS on the same
	// unconfigured namespace should succeed without error.
	property := func(gc goroutineCount) bool {
		numGoroutines := gc.N
		logf(t, "--- Property iteration: launching %d concurrent goroutines ---", numGoroutines)

		// Create a fresh platform instance for each iteration to avoid stale
		// network namespace context from previous iterations.
		iterPlatform, iterErr := newTestManagedLinuxPlatformE2E(t, ipamDir)
		if iterErr != nil {
			logf(t, "Failed to create platform for iteration: %v", iterErr)
			return false
		}

		// Clean up any stale namespace so isDaemonNamespaceConfigured returns
		// false and all goroutines attempt the full CNI plugin setup path.
		cleanupStaleDaemonNSE2E(t, iterPlatform.common.nsUtil.GetNetNSPath("host-daemon"))

		// Allow the kernel to finish cleaning up network namespace resources
		// before enumerating network links for BuildTaskNetworkConfiguration.
		time.Sleep(500 * time.Millisecond)

		// Build N daemon task network configurations targeting the same
		// shared "host-daemon" namespace path.
		taskPayload := &ecsacs.Task{
			NetworkMode: aws.String("daemon-bridge"),
		}

		netNSList := make([]*tasknetworkconfig.NetworkNamespace, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			taskID := fmt.Sprintf("daemon-pbt-%d", i)
			// Retry BuildTaskNetworkConfiguration up to 3 times to handle
			// transient "link was not found" errors from kernel namespace cleanup.
			var taskNetConfig *tasknetworkconfig.TaskNetworkConfig
			var buildErr error
			for attempt := 0; attempt < 3; attempt++ {
				taskNetConfig, buildErr = iterPlatform.BuildTaskNetworkConfiguration(taskID, taskPayload)
				if buildErr == nil {
					break
				}
				logf(t, "BuildTaskNetworkConfiguration attempt %d failed for task %s: %v (retrying)", attempt+1, taskID, buildErr)
				time.Sleep(500 * time.Millisecond)
			}
			if buildErr != nil {
				logf(t, "BuildTaskNetworkConfiguration failed for task %s after retries: %v", taskID, buildErr)
				return false
			}
			if len(taskNetConfig.NetworkNamespaces) != 1 {
				logf(t, "Expected 1 namespace, got %d for task %s", len(taskNetConfig.NetworkNamespaces), taskID)
				return false
			}
			netNSList[i] = taskNetConfig.NetworkNamespaces[0]
		}

		// Launch N concurrent goroutines calling ConfigureDaemonNetNS.
		results := make([]error, numGoroutines)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				err := iterPlatform.ConfigureDaemonNetNS(netNSList[idx])
				mu.Lock()
				results[idx] = err
				mu.Unlock()
			}(i)
		}

		wg.Wait()

		// Count successes and failures.
		successCount := 0
		failureCount := 0
		for i, err := range results {
			if err != nil {
				failureCount++
				logf(t, "  Goroutine %d FAILED: %v", i, err)
			} else {
				successCount++
			}
		}

		logf(t, "  Results: %d succeeded, %d failed (out of %d)", successCount, failureCount, numGoroutines)

		// Property assertion 1: all goroutines must return without error.
		if failureCount != 0 {
			logf(t, "  PROPERTY VIOLATED: %d out of %d goroutines failed (expected 0 failures)", failureCount, numGoroutines)
			return false
		}

		// Property assertion 2: bridge interface must exist after concurrent setup.
		bridge, err := netlink.LinkByName(managedDaemonBridgeNameE2E)
		if err != nil {
			logf(t, "  PROPERTY VIOLATED: bridge '%s' not found after concurrent setup: %v", managedDaemonBridgeNameE2E, err)
			return false
		}
		if bridge == nil {
			logf(t, "  PROPERTY VIOLATED: bridge '%s' is nil after concurrent setup", managedDaemonBridgeNameE2E)
			return false
		}

		// Property assertion 3: veth interface must exist in the daemon namespace.
		nsPath := netNSList[0].Path
		vethExists := false
		ns, nsErr := newNetNSFromPathE2E(nsPath)
		if nsErr != nil {
			logf(t, "  PROPERTY VIOLATED: failed to open namespace %s: %v", nsPath, nsErr)
			return false
		}
		defer ns.Close()

		doErr := ns.Do(func() error {
			link, err := netlink.LinkByName(managedDaemonIfNameE2E)
			if err != nil {
				return fmt.Errorf("veth '%s' not found: %v", managedDaemonIfNameE2E, err)
			}
			if link.Type() == "veth" {
				vethExists = true
			}
			return nil
		})
		if doErr != nil || !vethExists {
			logf(t, "  PROPERTY VIOLATED: veth '%s' not found in namespace %s: %v", managedDaemonIfNameE2E, nsPath, doErr)
			return false
		}

		logf(t, "  PROPERTY SATISFIED: all %d goroutines succeeded, bridge and veth exist", numGoroutines)

		// Clean up after this iteration so the next iteration starts fresh.
		ctx := context.Background()
		cleanupErr := iterPlatform.StopDaemonNetNS(ctx, netNSList[0])
		if cleanupErr != nil {
			logf(t, "  Warning: StopDaemonNetNS cleanup error: %v", cleanupErr)
		}

		return true
	}

	err = quick.Check(property, config)
	if err != nil {
		// Extract the counterexample from the quick.CheckError for diagnostics.
		logf(t, "Property-based test found counterexample: %v", err)
	}

	// Assert the property held for all iterations. On unfixed code, this fails
	// because the race condition causes concurrent goroutines to hit CNI
	// conflicts like "veth pair: eth0 already exists".
	assert.NoError(t, err,
		"Property violated: not all concurrent goroutines succeeded. "+
			"Concurrent goroutines hit CNI plugin conflicts on the unsynchronized "+
			"isDaemonNamespaceConfigured check-then-act pattern.")
}
