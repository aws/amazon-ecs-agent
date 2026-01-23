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
Package platform provides end-to-end integration tests for the managed Linux platform
APIs that configure daemon network namespaces.

# Overview

These tests validate the integration between the ECS Agent's managed Linux platform APIs
(BuildTaskNetworkConfiguration, ConfigureDaemonNetNS, StopDaemonNetNS) and the CNI plugins
from amazon-ecs-cni-plugins (ecs-bridge and ecs-ipam). Unlike daemon_bridge_e2e_test.go
which directly invokes CNI plugins, this test exercises the higher-level platform APIs
to validate the complete daemon network namespace configuration flow.

The test uses real EC2 metadata from the Instance Metadata Service (IMDS) to retrieve
actual host ENI information (MAC address, IP addresses, subnets). This means tests must
run on actual EC2 instances with access to the metadata service.

# What These Tests Validate

  - BuildTaskNetworkConfiguration() correctly builds network config for "daemon-bridge" mode
  - ConfigureDaemonNetNS() creates daemon namespace with correct bridge, veth, and routes
  - StopDaemonNetNS() properly cleans up veth while preserving bridge
  - Support for IPv4-only, IPv6-only, and dual-stack host configurations
  - ECS credentials endpoint route (169.254.170.2) exists for ALL configurations

# Test Requirements

The following requirements must be met to run these tests:

  - Root privileges: Tests must be run as root (sudo) to create network namespaces
  - CNI plugins: ecs-bridge and ecs-ipam binaries must be available
  - CNI_PATH: Environment variable must point to the directory containing the CNI plugin binaries
  - EC2 instance: Tests must run on an EC2 instance with access to IMDS
  - Network capabilities: Host must have appropriate IP version support (IPv4, IPv6, or both)

# Environment Variables

The following environment variables control test behavior:

  - CNI_PATH (required): Path to the directory containing ecs-bridge and ecs-ipam plugin binaries.
    Example: CNI_PATH=/opt/cni/bin

  - ECS_PRESERVE_E2E_TEST_LOGS: Set to "true" to preserve CNI plugin logs after test completion.
    Useful for debugging test failures. Default: "false"

  - ECS_CNI_LOG_FILE: Automatically set by the test to a temporary directory.
    Override to specify a custom log file location.

  - IPAM_DB_PATH: Automatically set by the test to a temporary directory.
    Override to specify a custom IPAM database location.

# Running Tests Remotely

These tests are designed to run on remote EC2 instances via SSH. Two remote hosts
are available for testing different IP configurations:

  - md-jan16-26: IPv4 host for testing IPv4-only and dual-stack configurations
  - md-ipv6-2: IPv6 host for testing IPv6-only configurations

Step 1: Sync code to the remote host using the build-remote.sh script:

	./build-remote.sh

Step 2: Run tests on the IPv4 host (md-jan16-26):

	ssh md-jan16-26 "cd /home/ssm-user/go/src/github.com/aws/amazon-ecs-agent/amazon-ecs-agent/ecs-agent && \
	    sudo -E CNI_PATH=/opt/cni/bin go test -v -tags e2e -timeout 120s -run TestManagedLinuxDaemonNetworkNamespace ./netlib/platform/..."

Step 3: Run tests on the IPv6 host (md-ipv6-2):

	ssh md-ipv6-2 "cd /home/ssm-user/go/src/github.com/aws/amazon-ecs-agent/amazon-ecs-agent/ecs-agent && \
	    sudo -E CNI_PATH=/opt/cni/bin go test -v -tags e2e -timeout 120s -run TestManagedLinuxDaemonNetworkNamespace ./netlib/platform/..."

To preserve logs for debugging:

	ssh md-jan16-26 "cd /home/ssm-user/go/src/github.com/aws/amazon-ecs-agent/amazon-ecs-agent/ecs-agent && \
	    sudo -E CNI_PATH=/opt/cni/bin ECS_PRESERVE_E2E_TEST_LOGS=true go test -v -tags e2e -timeout 120s -run TestManagedLinuxDaemonNetworkNamespace ./netlib/platform/..."

# Test Structure

The main test function TestManagedLinuxDaemonNetworkNamespace contains three sub-tests:

  - IPv4Only: Tests IPv4-only configuration with ECS credentials endpoint route and IPv4 default route
  - IPv6Only: Tests IPv6-only configuration with ECS credentials endpoint route (IPv4 link-local) and IPv6 default route
  - DualStack: Tests dual-stack configuration with both IPv4 and IPv6 routes

Each sub-test follows this pattern:
 1. Setup: Create temp directories for IPAM database and logs
 2. Create managedLinux platform with real EC2 metadata client
 3. Build task network configuration with "daemon-bridge" mode
 4. Execute ConfigureDaemonNetNS to configure the namespace
 5. Validate network state: bridge, veth, addresses, routes
 6. Execute StopDaemonNetNS for cleanup
 7. Validate cleanup: veth removed, bridge persists
 8. Cleanup: Remove temp directories

# Key Invariant: ECS Credentials Endpoint

The ECS credentials endpoint (169.254.170.2) is ALWAYS accessed via IPv4 link-local,
even on IPv6-only hosts. This is a critical invariant that the tests validate.
The IPv6Only sub-test specifically verifies that the IPv4 route to the ECS credentials
endpoint exists even when the host only has IPv6 connectivity for external traffic.

# Correctness Properties Validated

  - Property 1: ECS Credentials Route Invariant - route to 169.254.170.2/32 exists for all configs
  - Property 2: Bridge Address Consistency - bridge has 169.254.172.1/22 (and IPv6 if applicable)
  - Property 3: Veth Pair Creation - veth has IPv4 from ECS subnet (and IPv6 if applicable)
  - Property 4: Default Route Configuration - correct default routes based on IP compatibility
  - Property 5: Cleanup Preserves Bridge - veth removed, bridge persists after StopDaemonNetNS
  - Property 6: Loopback Interface State - loopback is UP in daemon namespace
*/
package platform

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
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
// Test Constants
// ============================================================================

const (
	// managedDaemonBridgeNameE2E is the name of the bridge interface created by the platform APIs.
	// This matches the BridgeInterfaceName constant in cniconf_linux.go.
	managedDaemonBridgeNameE2E = "fargate-bridge"

	// managedDaemonIfNameE2E is the name of the interface created inside the daemon namespace.
	managedDaemonIfNameE2E = "eth0"

	// ECS Link-Local Subnet Configuration
	// -----------------------------------
	// The ECS link-local subnet (169.254.172.0/22) is used for internal ECS communication.

	// managedECSLinkLocalGatewayE2E is the gateway address on the bridge for the ECS link-local subnet.
	managedECSLinkLocalGatewayE2E = "169.254.172.1"

	// managedECSCredentialsEndpointDstE2E is the destination for the ECS credentials endpoint route.
	managedECSCredentialsEndpointDstE2E = "169.254.170.2/32"

	// Expected Bridge Addresses
	// -------------------------

	// managedExpectedBridgeIPv4E2E is the expected IPv4 address on the bridge.
	managedExpectedBridgeIPv4E2E = "169.254.172.1/22"
)

// ============================================================================
// Helper Function: Create Test ManagedLinux Platform
// ============================================================================

// newTestManagedLinuxPlatformE2E creates a managedLinux platform instance with a real EC2 metadata client.
// This function is used by E2E tests to create a platform that can interact with the actual
// EC2 Instance Metadata Service (IMDS) to retrieve host ENI information.
//
// Parameters:
//   - t: The testing.T instance for logging and assertions
//   - stateDBDir: Directory path for storing IPAM database and other state
//
// Returns:
//   - *managedLinux: A configured managedLinux platform instance
//   - error: Any error encountered during creation
//
// The function will skip the test if the EC2 metadata service is not available.
//
// **Validates: Requirements 1.2, 1.7, 2.1**
func newTestManagedLinuxPlatformE2E(t *testing.T, stateDBDir string) (*managedLinux, error) {
	// Create real EC2 metadata client
	ec2Client, err := ec2.NewEC2MetadataClient(nil)
	if err != nil {
		t.Skipf("EC2 metadata service not available, skipping test: %v", err)
		return nil, err
	}

	// Verify we can actually reach IMDS by making a test request
	_, err = ec2Client.GetMetadata(MacResource)
	if err != nil {
		t.Skipf("Cannot reach EC2 metadata service, skipping test: %v", err)
		return nil, err
	}

	// Create the common platform struct with real implementations
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

// validateBridgeExistsE2E verifies that the bridge interface exists.
// Returns the bridge link for further validation.
//
// **Validates: Requirements 3.2, 4.2, 5.2**
func validateBridgeExistsE2E(t *testing.T, bridgeName string) netlink.Link {
	bridge, err := netlink.LinkByName(bridgeName)
	require.NoError(t, err, "Bridge interface '%s' should exist", bridgeName)
	require.NotNil(t, bridge, "Bridge interface should not be nil")
	return bridge
}

// validateBridgeIPv4AddressE2E verifies that the bridge has the expected IPv4 address.
//
// **Validates: Requirements 3.2 (bridge has expected IPv4 address)**
// **Feature: managed-linux-daemon-e2e-test, Property 2: Bridge Address Consistency**
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

// validateBridgeIPv6AddressE2E verifies that the bridge has an IPv6 address assigned.
// For IPv6-compatible hosts, the bridge should have an IPv6 gateway address.
//
// **Validates: Requirements 4.2, 5.2 (bridge has IPv6 address)**
// **Feature: managed-linux-daemon-e2e-test, Property 2: Bridge Address Consistency**
func validateBridgeIPv6AddressE2E(t *testing.T, bridge netlink.Link) {
	addrs, err := netlink.AddrList(bridge, netlink.FAMILY_V6)
	require.NoError(t, err, "Unable to list the IPv6 addresses of: %s", bridge.Attrs().Name)

	// Filter out link-local addresses (fe80::)
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

// validateVethExistsInNSE2E verifies that a veth interface exists in the specified namespace.
// Returns the veth link for further validation.
//
// **Validates: Requirements 3.3, 4.3, 5.3**
// **Feature: managed-linux-daemon-e2e-test, Property 3: Veth Pair Creation**
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

// validateVethIPv4AddressE2E verifies that the veth has an IPv4 address from the ECS subnet.
//
// **Validates: Requirements 3.3 (veth has IPv4 address from ECS subnet)**
// **Feature: managed-linux-daemon-e2e-test, Property 3: Veth Pair Creation**
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

		// Check that at least one IPv4 address is from the ECS subnet (169.254.172.0/22)
		addressFound := false
		for _, addr := range addrs {
			ip := addr.IP.To4()
			if ip != nil && ip[0] == 169 && ip[1] == 254 && ip[2] >= 172 && ip[2] <= 175 {
				addressFound = true
				break
			}
		}
		require.True(t, addressFound, "Veth '%s' should have an IPv4 address from ECS subnet (169.254.172.0/22)", ifName)
		return nil
	})
	require.NoError(t, err, "Failed to validate veth IPv4 address")
}

// validateVethIPv6AddressE2E verifies that the veth has an IPv6 address.
//
// **Validates: Requirements 4.3, 5.3 (veth has IPv6 address)**
// **Feature: managed-linux-daemon-e2e-test, Property 3: Veth Pair Creation**
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

		// Filter out link-local addresses
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

// validateLoopbackUpE2E verifies that the loopback interface is UP in the namespace.
//
// **Validates: Requirements 3.6**
// **Feature: managed-linux-daemon-e2e-test, Property 6: Loopback Interface State**
func validateLoopbackUpE2E(t *testing.T, nsPath string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		lo, err := netlink.LinkByName("lo")
		if err != nil {
			return fmt.Errorf("loopback interface not found: %v", err)
		}

		// Check if the interface is UP
		if lo.Attrs().Flags&0x1 == 0 { // IFF_UP = 0x1
			return fmt.Errorf("loopback interface is not UP")
		}
		return nil
	})
	require.NoError(t, err, "Loopback interface should be UP in namespace %s", nsPath)
}

// newNetNSFromPathE2E opens an existing network namespace from a path.
// This is a helper to work with namespaces created by the platform APIs.
// Unlike netNSE2E created by newNetNSE2E, this does NOT clean up the namespace
// when Close() is called, since the namespace is managed by the platform APIs.
func newNetNSFromPathE2E(nsPath string) (*managedNetNSE2E, error) {
	// Check if the namespace exists using Stat
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

// managedNetNSE2E represents a network namespace that is managed by the platform APIs.
// Unlike netNSE2E, this does NOT clean up the namespace when Close() is called.
type managedNetNSE2E struct {
	path string
}

// Path returns the path to the network namespace
func (n *managedNetNSE2E) Path() string {
	return n.path
}

// Close is a no-op for managed namespaces since they are managed by the platform APIs.
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

// validateECSCredentialsRouteE2E verifies that the route to ECS credentials endpoint exists.
// This is the CRITICAL invariant - ECS credentials are always accessed via IPv4 link-local.
//
// **Validates: Requirements 3.4, 4.4, 5.4**
// **Feature: managed-linux-daemon-e2e-test, Property 1: ECS Credentials Route Invariant**
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
			// Check for ECS credentials endpoint route (169.254.170.2/32) via gateway
			if route.Dst != nil && route.Dst.String() == managedECSCredentialsEndpointDstE2E &&
				route.Gw != nil && route.Gw.String() == managedECSLinkLocalGatewayE2E {
				ecsCredentialsRouteFound = true
				break
			}
		}

		require.True(t, ecsCredentialsRouteFound,
			"Route to ECS credentials endpoint '%s' via gateway '%s' not found for: %s",
			managedECSCredentialsEndpointDstE2E, managedECSLinkLocalGatewayE2E, ifName)
		return nil
	})
	require.NoError(t, err, "Failed to validate ECS credentials route")
}

// validateIPv4DefaultRouteE2E verifies that an IPv4 default route exists.
//
// **Validates: Requirements 3.5, 5.5**
// **Feature: managed-linux-daemon-e2e-test, Property 4: Default Route Configuration**
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
			// Check for default route (0.0.0.0/0) via gateway
			isDefaultRoute := route.Dst == nil || (route.Dst != nil && route.Dst.String() == "0.0.0.0/0")
			if isDefaultRoute && route.Gw != nil && route.Gw.String() == managedECSLinkLocalGatewayE2E {
				defaultRouteFound = true
				break
			}
		}

		require.True(t, defaultRouteFound,
			"IPv4 default route (0.0.0.0/0) via gateway '%s' not found for: %s",
			managedECSLinkLocalGatewayE2E, ifName)
		return nil
	})
	require.NoError(t, err, "Failed to validate IPv4 default route")
}

// validateIPv6DefaultRouteE2E verifies that an IPv6 default route exists.
//
// **Validates: Requirements 4.5, 5.5**
// **Feature: managed-linux-daemon-e2e-test, Property 4: Default Route Configuration**
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
			// Check for IPv6 default route (::/0)
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

// validateNoIPv4DefaultRouteE2E verifies that NO IPv4 default route exists.
// This is used for IPv6-only hosts where external traffic should only route via IPv6.
//
// **Validates: Requirements 4.6**
// **Feature: managed-linux-daemon-e2e-test, Property 4: Default Route Configuration**
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
			// Check for default route (0.0.0.0/0)
			isDefaultRoute := route.Dst == nil || (route.Dst != nil && route.Dst.String() == "0.0.0.0/0")
			if isDefaultRoute && route.Gw != nil {
				t.Logf("Found unexpected IPv4 default route: dst=%v gw=%v", route.Dst, route.Gw)
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

// validateVethRemovedE2E verifies that the veth interface no longer exists in the namespace.
//
// **Validates: Requirements 6.2**
// **Feature: managed-linux-daemon-e2e-test, Property 5: Cleanup Preserves Bridge**
func validateVethRemovedE2E(t *testing.T, nsPath string, ifName string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	require.NoError(t, err, "Failed to open network namespace at %s", nsPath)
	defer ns.Close()

	err = ns.Do(func() error {
		_, err := netlink.LinkByName(ifName)
		if err == nil {
			return fmt.Errorf("veth interface '%s' should have been removed", ifName)
		}
		// LinkNotFoundError is expected
		_, ok := err.(netlink.LinkNotFoundError)
		if !ok {
			return fmt.Errorf("unexpected error type when checking for removed veth: %v", err)
		}
		return nil
	})
	require.NoError(t, err, "Veth interface should be removed after cleanup")
}

// validateBridgePersistsE2E verifies that the bridge still exists with its addresses after cleanup.
//
// **Validates: Requirements 6.3, 6.4**
// **Feature: managed-linux-daemon-e2e-test, Property 5: Cleanup Preserves Bridge**
func validateBridgePersistsE2E(t *testing.T, bridgeName string) {
	bridge, err := netlink.LinkByName(bridgeName)
	require.NoError(t, err, "Bridge '%s' should still exist after cleanup", bridgeName)
	require.NotNil(t, bridge, "Bridge should not be nil")

	// Verify bridge still has IPv4 address
	addrs, err := netlink.AddrList(bridge, netlink.FAMILY_V4)
	require.NoError(t, err, "Unable to list IPv4 addresses of bridge after cleanup")
	require.NotEmpty(t, addrs, "Bridge should retain IPv4 addresses after cleanup")
}

// ============================================================================
// Debugging Helper Function
// ============================================================================

// logNetworkStateInNSE2E logs all interfaces, addresses, and routes in a namespace.
// This is called on test failure for diagnostics.
//
// **Validates: Requirements 8.1, 8.3**
func logNetworkStateInNSE2E(t *testing.T, nsPath string, description string) {
	ns, err := newNetNSFromPathE2E(nsPath)
	if err != nil {
		t.Logf("=== Network State: %s (failed to open namespace: %v) ===", description, err)
		return
	}
	defer ns.Close()

	ns.Do(func() error {
		t.Logf("=== Network State: %s ===", description)

		links, err := netlink.LinkList()
		if err != nil {
			t.Logf("Error listing interfaces: %v", err)
			return nil
		}

		for _, link := range links {
			t.Logf("Interface: %s (type: %s, state: %s)",
				link.Attrs().Name, link.Type(), link.Attrs().OperState)

			addrsV4, err := netlink.AddrList(link, netlink.FAMILY_V4)
			if err != nil {
				t.Logf("  Error listing IPv4 addresses: %v", err)
			} else {
				for _, addr := range addrsV4 {
					t.Logf("  IPv4: %s", addr.IPNet.String())
				}
			}

			addrsV6, err := netlink.AddrList(link, netlink.FAMILY_V6)
			if err != nil {
				t.Logf("  Error listing IPv6 addresses: %v", err)
			} else {
				for _, addr := range addrsV6 {
					t.Logf("  IPv6: %s", addr.IPNet.String())
				}
			}
		}

		routesV4, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err != nil {
			t.Logf("Error listing IPv4 routes: %v", err)
		} else {
			t.Logf("IPv4 Routes:")
			for _, route := range routesV4 {
				t.Logf("  dst=%v gw=%v", route.Dst, route.Gw)
			}
		}

		routesV6, err := netlink.RouteList(nil, netlink.FAMILY_V6)
		if err != nil {
			t.Logf("Error listing IPv6 routes: %v", err)
		} else {
			t.Logf("IPv6 Routes:")
			for _, route := range routesV6 {
				t.Logf("  dst=%v gw=%v", route.Dst, route.Gw)
			}
		}

		return nil
	})
}

// ============================================================================
// Main Test Function
// ============================================================================

// TestManagedLinuxDaemonNetworkNamespace tests the managed Linux platform APIs for
// daemon network namespace configuration.
//
// This test exercises the higher-level platform APIs:
//   - BuildTaskNetworkConfiguration() with network mode "daemon-bridge"
//   - ConfigureDaemonNetNS() to configure the daemon network namespace
//   - StopDaemonNetNS() to clean up the daemon network namespace
//
// The test uses real EC2 metadata from IMDS to retrieve actual host ENI information.
//
// Test structure:
//   - Common setup: log directory, IPAM database, plugin path discovery, IP compatibility detection
//   - IPv4Only sub-test: Tests IPv4-only configuration
//   - IPv6Only sub-test: Tests IPv6-only configuration
//   - DualStack sub-test: Tests dual-stack configuration
//
// **Validates: Requirements 1.1-1.7, 2.1-2.5, 3.1-3.6, 4.1-4.6, 5.1-5.5, 6.1-6.5, 7.1-7.5, 8.1-8.4**
func TestManagedLinuxDaemonNetworkNamespace(t *testing.T) {
	// ========================================================================
	// Common Setup
	// ========================================================================

	// Detect host IP compatibility using existing helper functions
	// **Validates: Requirements 1.4**
	hasIPv4 := instanceSupportsIPv4E2E()
	hasIPv6 := instanceSupportsIPv6E2E()

	t.Logf("Host IP version support: IPv4=%v, IPv6=%v", hasIPv4, hasIPv6)

	if !hasIPv4 && !hasIPv6 {
		t.Skip("Skipping test: host supports neither IPv4 nor IPv6")
	}

	// Ensure that the bridge plugin exists
	// **Validates: Requirements 1.1, 1.6**
	bridgePluginPath, err := invoke.FindInPath("ecs-bridge", []string{os.Getenv("CNI_PATH")})
	if err != nil {
		t.Skipf("Unable to find bridge plugin in path. Set CNI_PATH environment variable: %v", err)
	}
	t.Logf("Using bridge plugin: %s", bridgePluginPath)

	// Create a directory for storing test logs
	// **Validates: Requirements 1.3**
	testLogDir, err := ioutil.TempDir("", "managed-linux-daemon-e2e-")
	require.NoError(t, err, "Unable to create directory for storing test logs")

	os.Setenv("ECS_CNI_LOG_FILE", fmt.Sprintf("%s/bridge.log", testLogDir))
	t.Logf("Using %s for test logs", testLogDir)
	defer os.Unsetenv("ECS_CNI_LOG_FILE")

	// **Validates: Requirements 1.5, 8.2**
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
		t.Logf("Running IPv4-only managed Linux daemon namespace test")

		// Create temp directory for IPAM database
		ipamDir, err := ioutil.TempDir("", "managed-linux-ipam-ipv4-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		defer os.RemoveAll(ipamDir)

		// Create test managedLinux platform with real EC2 client
		// **Validates: Requirements 1.2, 1.7, 2.1**
		platform, err := newTestManagedLinuxPlatformE2E(t, ipamDir)
		require.NoError(t, err, "Failed to create test platform")

		// Build task payload with NetworkMode "daemon-bridge"
		// **Validates: Requirements 2.1, 2.2**
		taskPayload := &ecsacs.Task{
			NetworkMode: aws.String("daemon-bridge"),
		}

		// Call BuildTaskNetworkConfiguration()
		taskNetConfig, err := platform.BuildTaskNetworkConfiguration("test-task-ipv4", taskPayload)
		require.NoError(t, err, "BuildTaskNetworkConfiguration should succeed")
		require.NotNil(t, taskNetConfig, "TaskNetworkConfig should not be nil")
		require.Len(t, taskNetConfig.NetworkNamespaces, 1, "Should have exactly one network namespace")

		netNS := taskNetConfig.NetworkNamespaces[0]
		t.Logf("Network namespace path: %s", netNS.Path)

		// Call ConfigureDaemonNetNS() to configure the namespace
		// **Validates: Requirements 2.3, 2.4**
		err = platform.ConfigureDaemonNetNS(netNS)
		require.NoError(t, err, "ConfigureDaemonNetNS should succeed")

		// Validate bridge exists with IPv4 address
		// **Feature: managed-linux-daemon-e2e-test, Property 2: Bridge Address Consistency**
		// **Validates: Requirements 3.2**
		bridge := validateBridgeExistsE2E(t, managedDaemonBridgeNameE2E)
		validateBridgeIPv4AddressE2E(t, bridge, managedExpectedBridgeIPv4E2E)

		// Validate veth exists with IPv4 address from ECS subnet
		// **Feature: managed-linux-daemon-e2e-test, Property 3: Veth Pair Creation**
		// **Validates: Requirements 3.3**
		validateVethExistsInNSE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv4AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate ECS credentials route exists
		// **Feature: managed-linux-daemon-e2e-test, Property 1: ECS Credentials Route Invariant**
		// **Validates: Requirements 3.4**
		validateECSCredentialsRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate IPv4 default route exists
		// **Feature: managed-linux-daemon-e2e-test, Property 4: Default Route Configuration**
		// **Validates: Requirements 3.5**
		validateIPv4DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate loopback is UP
		// **Feature: managed-linux-daemon-e2e-test, Property 6: Loopback Interface State**
		// **Validates: Requirements 3.6**
		validateLoopbackUpE2E(t, netNS.Path)

		// Log network state for debugging
		logNetworkStateInNSE2E(t, netNS.Path, "After ConfigureDaemonNetNS - IPv4Only")

		// Call StopDaemonNetNS() for cleanup
		// **Validates: Requirements 6.1**
		ctx := context.Background()
		err = platform.StopDaemonNetNS(ctx, netNS)
		require.NoError(t, err, "StopDaemonNetNS should succeed")

		// Validate veth removed, bridge persists
		// **Feature: managed-linux-daemon-e2e-test, Property 5: Cleanup Preserves Bridge**
		// **Validates: Requirements 6.2, 6.3, 6.4**
		validateVethRemovedE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateBridgePersistsE2E(t, managedDaemonBridgeNameE2E)

		t.Log("IPv4-only managed Linux daemon namespace test completed successfully")
	})

	// ========================================================================
	// IPv6-only sub-test
	// ========================================================================
	t.Run("IPv6Only", func(t *testing.T) {
		skipIfNotIPv6OnlyE2E(t)
		t.Logf("Running IPv6-only managed Linux daemon namespace test")
		t.Logf("NOTE: This test uses IPv4 link-local for ECS credentials + IPv6 for external traffic")

		// Create temp directory for IPAM database
		ipamDir, err := ioutil.TempDir("", "managed-linux-ipam-ipv6-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		defer os.RemoveAll(ipamDir)

		// Create test managedLinux platform with real EC2 client
		// **Validates: Requirements 1.2, 1.7, 2.1**
		platform, err := newTestManagedLinuxPlatformE2E(t, ipamDir)
		require.NoError(t, err, "Failed to create test platform")

		// Build task payload with NetworkMode "daemon-bridge"
		// **Validates: Requirements 2.1, 2.2**
		taskPayload := &ecsacs.Task{
			NetworkMode: aws.String("daemon-bridge"),
		}

		// Call BuildTaskNetworkConfiguration()
		taskNetConfig, err := platform.BuildTaskNetworkConfiguration("test-task-ipv6", taskPayload)
		require.NoError(t, err, "BuildTaskNetworkConfiguration should succeed")
		require.NotNil(t, taskNetConfig, "TaskNetworkConfig should not be nil")
		require.Len(t, taskNetConfig.NetworkNamespaces, 1, "Should have exactly one network namespace")

		netNS := taskNetConfig.NetworkNamespaces[0]
		t.Logf("Network namespace path: %s", netNS.Path)

		// Call ConfigureDaemonNetNS() to configure the namespace
		// **Validates: Requirements 2.3, 2.4**
		err = platform.ConfigureDaemonNetNS(netNS)
		require.NoError(t, err, "ConfigureDaemonNetNS should succeed")

		// Validate bridge exists with both IPv4 and IPv6 addresses
		// **Feature: managed-linux-daemon-e2e-test, Property 2: Bridge Address Consistency**
		// **Validates: Requirements 4.2**
		bridge := validateBridgeExistsE2E(t, managedDaemonBridgeNameE2E)
		validateBridgeIPv4AddressE2E(t, bridge, managedExpectedBridgeIPv4E2E)
		validateBridgeIPv6AddressE2E(t, bridge)

		// Validate veth exists with both IPv4 and IPv6 addresses
		// **Feature: managed-linux-daemon-e2e-test, Property 3: Veth Pair Creation**
		// **Validates: Requirements 4.3**
		validateVethExistsInNSE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv4AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv6AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate ECS credentials route exists (IPv4 link-local)
		// **Feature: managed-linux-daemon-e2e-test, Property 1: ECS Credentials Route Invariant**
		// **Validates: Requirements 4.4**
		validateECSCredentialsRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate IPv6 default route exists
		// **Feature: managed-linux-daemon-e2e-test, Property 4: Default Route Configuration**
		// **Validates: Requirements 4.5**
		validateIPv6DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate NO IPv4 default route exists (IPv6-only host)
		// **Feature: managed-linux-daemon-e2e-test, Property 4: Default Route Configuration**
		// **Validates: Requirements 4.6**
		validateNoIPv4DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate loopback is UP
		// **Feature: managed-linux-daemon-e2e-test, Property 6: Loopback Interface State**
		validateLoopbackUpE2E(t, netNS.Path)

		// Log network state for debugging
		logNetworkStateInNSE2E(t, netNS.Path, "After ConfigureDaemonNetNS - IPv6Only")

		// Call StopDaemonNetNS() for cleanup
		// **Validates: Requirements 6.1**
		ctx := context.Background()
		err = platform.StopDaemonNetNS(ctx, netNS)
		require.NoError(t, err, "StopDaemonNetNS should succeed")

		// Validate veth removed, bridge persists
		// **Feature: managed-linux-daemon-e2e-test, Property 5: Cleanup Preserves Bridge**
		// **Validates: Requirements 6.2, 6.3, 6.4**
		validateVethRemovedE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateBridgePersistsE2E(t, managedDaemonBridgeNameE2E)

		t.Log("IPv6-only managed Linux daemon namespace test completed successfully")
	})

	// ========================================================================
	// Dual-stack sub-test
	// ========================================================================
	t.Run("DualStack", func(t *testing.T) {
		skipIfNoDualStackE2E(t)
		t.Logf("Running dual-stack managed Linux daemon namespace test")
		t.Logf("NOTE: This test uses IPv4 link-local for ECS credentials + both IPv4 and IPv6 for external traffic")

		// Create temp directory for IPAM database
		ipamDir, err := ioutil.TempDir("", "managed-linux-ipam-dualstack-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		defer os.RemoveAll(ipamDir)

		// Create test managedLinux platform with real EC2 client
		// **Validates: Requirements 1.2, 1.7, 2.1**
		platform, err := newTestManagedLinuxPlatformE2E(t, ipamDir)
		require.NoError(t, err, "Failed to create test platform")

		// Build task payload with NetworkMode "daemon-bridge"
		// **Validates: Requirements 2.1, 2.2**
		taskPayload := &ecsacs.Task{
			NetworkMode: aws.String("daemon-bridge"),
		}

		// Call BuildTaskNetworkConfiguration()
		taskNetConfig, err := platform.BuildTaskNetworkConfiguration("test-task-dualstack", taskPayload)
		require.NoError(t, err, "BuildTaskNetworkConfiguration should succeed")
		require.NotNil(t, taskNetConfig, "TaskNetworkConfig should not be nil")
		require.Len(t, taskNetConfig.NetworkNamespaces, 1, "Should have exactly one network namespace")

		netNS := taskNetConfig.NetworkNamespaces[0]
		t.Logf("Network namespace path: %s", netNS.Path)

		// Call ConfigureDaemonNetNS() to configure the namespace
		// **Validates: Requirements 2.3, 2.4**
		err = platform.ConfigureDaemonNetNS(netNS)
		require.NoError(t, err, "ConfigureDaemonNetNS should succeed")

		// Validate bridge exists with both IPv4 and IPv6 addresses
		// **Feature: managed-linux-daemon-e2e-test, Property 2: Bridge Address Consistency**
		// **Validates: Requirements 5.2**
		bridge := validateBridgeExistsE2E(t, managedDaemonBridgeNameE2E)
		validateBridgeIPv4AddressE2E(t, bridge, managedExpectedBridgeIPv4E2E)
		validateBridgeIPv6AddressE2E(t, bridge)

		// Validate veth exists with both IPv4 and IPv6 addresses
		// **Feature: managed-linux-daemon-e2e-test, Property 3: Veth Pair Creation**
		// **Validates: Requirements 5.3**
		validateVethExistsInNSE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv4AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateVethIPv6AddressE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate ECS credentials route exists
		// **Feature: managed-linux-daemon-e2e-test, Property 1: ECS Credentials Route Invariant**
		// **Validates: Requirements 5.4**
		validateECSCredentialsRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate both IPv4 and IPv6 default routes exist
		// **Feature: managed-linux-daemon-e2e-test, Property 4: Default Route Configuration**
		// **Validates: Requirements 5.5**
		validateIPv4DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateIPv6DefaultRouteE2E(t, netNS.Path, managedDaemonIfNameE2E)

		// Validate loopback is UP
		// **Feature: managed-linux-daemon-e2e-test, Property 6: Loopback Interface State**
		validateLoopbackUpE2E(t, netNS.Path)

		// Log network state for debugging
		logNetworkStateInNSE2E(t, netNS.Path, "After ConfigureDaemonNetNS - DualStack")

		// Call StopDaemonNetNS() for cleanup
		// **Validates: Requirements 6.1**
		ctx := context.Background()
		err = platform.StopDaemonNetNS(ctx, netNS)
		require.NoError(t, err, "StopDaemonNetNS should succeed")

		// Validate veth removed, bridge persists
		// **Feature: managed-linux-daemon-e2e-test, Property 5: Cleanup Preserves Bridge**
		// **Validates: Requirements 6.2, 6.3, 6.4**
		validateVethRemovedE2E(t, netNS.Path, managedDaemonIfNameE2E)
		validateBridgePersistsE2E(t, managedDaemonBridgeNameE2E)

		t.Log("Dual-stack managed Linux daemon namespace test completed successfully")
	})
}
