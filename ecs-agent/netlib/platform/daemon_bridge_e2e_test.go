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
Package platform provides end-to-end integration tests for daemon bridge network
namespace configuration with IPv4, IPv6, and dual-stack support.

# Overview

These tests validate the integration between the ECS Agent's netlib/platform code
(specifically managed_linux.go) and the CNI plugins from amazon-ecs-cni-plugins
(ecs-bridge and ecs-ipam). The tests execute actual CNI plugin binaries to create
daemon network namespaces and validate the complete networking stack.

The daemon network namespace does NOT configure an ENI - it connects to the primary
host interface via a bridge. Traffic routes to the outside world via the bridge,
with link-local traffic routed to the ECS subnet and all other traffic forwarded
to the primary host interface.

# What These Tests Validate

  - Bridge creation with correct IPv4/IPv6 addresses
  - Veth pair creation connecting daemon namespace to bridge
  - Route configuration for ECS credentials endpoint (IPv4 link-local) and external traffic
  - Proper cleanup via CNI DEL command
  - Support for IPv4-only, IPv6-only, and dual-stack host configurations

# Test Requirements

The following requirements must be met to run these tests:

  - Root privileges: Tests must be run as root (sudo) to create network namespaces
  - CNI plugins: ecs-bridge and ecs-ipam binaries must be available
  - CNI_PATH: Environment variable must point to the directory containing the CNI plugin binaries
  - Network capabilities: Host must have appropriate IP version support (IPv4, IPv6, or both)

# Environment Variables

The following environment variables control test behavior:

  - CNI_PATH (required): Path to the directory containing ecs-bridge and ecs-ipam plugin binaries.
    Example: CNI_PATH=/opt/cni/bin

  - ECS_PRESERVE_E2E_TEST_LOGS: Set to "true" to preserve CNI plugin logs after test completion.
    Useful for debugging test failures. Default: "false"

  - ECS_BRIDGE_PRESERVE_IPAM_DB: Set to "true" to preserve the IPAM database after test completion.
    Useful for debugging IP allocation issues. Default: "false"

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
	    sudo -E CNI_PATH=/opt/cni/bin go test -v -tags e2e -timeout 120s ./netlib/platform/..."

Step 3: Run tests on the IPv6 host (md-ipv6-2):

	ssh md-ipv6-2 "cd /home/ssm-user/go/src/github.com/aws/amazon-ecs-agent/amazon-ecs-agent/ecs-agent && \
	    sudo -E CNI_PATH=/opt/cni/bin go test -v -tags e2e -timeout 120s ./netlib/platform/..."

To preserve logs for debugging:

	ssh md-jan16-26 "cd /home/ssm-user/go/src/github.com/aws/amazon-ecs-agent/amazon-ecs-agent/ecs-agent && \
	    sudo -E CNI_PATH=/opt/cni/bin ECS_PRESERVE_E2E_TEST_LOGS=true go test -v -tags e2e -timeout 120s ./netlib/platform/..."

# Test Structure

The main test function TestDaemonBridgeNetworkNamespace contains three sub-tests:

  - IPv4Only: Tests IPv4-only configuration with ECS credentials endpoint route and IPv4 default route
  - IPv6Only: Tests IPv6-only configuration with ECS credentials endpoint route (IPv4 link-local) and IPv6 default route
  - DualStack: Tests dual-stack configuration with both IPv4 and IPv6 routes

Each sub-test follows this pattern:
 1. Setup: Create temp directories for IPAM database and logs
 2. Create namespaces: Test namespace (host simulation) and target namespace (daemon)
 3. Execute ADD: Invoke CNI plugin with appropriate configuration
 4. Validate ADD results: Check bridge, veth, addresses, routes
 5. Execute DEL: Invoke CNI plugin cleanup
 6. Validate DEL results: Check veth removed, bridge persists
 7. Cleanup: Remove namespaces and temp directories

# Key Invariant: ECS Credentials Endpoint

The ECS credentials endpoint (169.254.170.2) is ALWAYS accessed via IPv4 link-local,
even on IPv6-only hosts. This is a critical invariant that the tests validate.
The IPv6Only sub-test specifically verifies that the IPv4 route to the ECS credentials
endpoint exists even when the host only has IPv6 connectivity for external traffic.

# Debugging Test Failures

When a test fails, the following information is logged:
  - Current network state (interfaces, addresses, routes) via logNetworkStateE2E()
  - CNI plugin error output
  - CNI configuration being used

To get more detailed logs:
 1. Set ECS_PRESERVE_E2E_TEST_LOGS=true to preserve CNI plugin logs
 2. Set ECS_BRIDGE_PRESERVE_IPAM_DB=true to preserve the IPAM database
 3. Check the log directory printed at test start for CNI plugin logs
*/

package platform

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

func init() {
	// Lock OS thread for namespace operations
	runtime.LockOSThread()
}

// ============================================================================
// Network Namespace Helper Types and Functions
// ============================================================================

// netNSE2E represents a network namespace for E2E testing
type netNSE2E struct {
	file *os.File
	path string
}

// newNetNSE2E creates a new network namespace
func newNetNSE2E() (*netNSE2E, error) {
	runtime.LockOSThread()

	// Save the current namespace
	origns, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get current namespace: %v", err)
	}
	defer origns.Close()

	// Create a new namespace
	newns, err := netns.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create new namespace: %v", err)
	}

	// Get the path to the new namespace
	nsPath := fmt.Sprintf("/proc/%d/task/%d/ns/net", os.Getpid(), syscall.Gettid())

	// Create a bind mount to persist the namespace
	tmpDir, err := ioutil.TempDir("", "netns-e2e-")
	if err != nil {
		newns.Close()
		netns.Set(origns)
		return nil, fmt.Errorf("failed to create temp dir: %v", err)
	}

	nsFile := filepath.Join(tmpDir, "ns")
	f, err := os.Create(nsFile)
	if err != nil {
		os.RemoveAll(tmpDir)
		newns.Close()
		netns.Set(origns)
		return nil, fmt.Errorf("failed to create ns file: %v", err)
	}
	f.Close()

	if err := unix.Mount(nsPath, nsFile, "none", unix.MS_BIND, ""); err != nil {
		os.RemoveAll(tmpDir)
		newns.Close()
		netns.Set(origns)
		return nil, fmt.Errorf("failed to bind mount namespace: %v", err)
	}

	// Switch back to the original namespace
	if err := netns.Set(origns); err != nil {
		unix.Unmount(nsFile, unix.MNT_DETACH)
		os.RemoveAll(tmpDir)
		newns.Close()
		return nil, fmt.Errorf("failed to restore original namespace: %v", err)
	}
	newns.Close()

	// Open the bind-mounted namespace file
	nsFileHandle, err := os.Open(nsFile)
	if err != nil {
		unix.Unmount(nsFile, unix.MNT_DETACH)
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to open namespace file: %v", err)
	}

	return &netNSE2E{
		file: nsFileHandle,
		path: nsFile,
	}, nil
}

// Path returns the path to the network namespace
func (n *netNSE2E) Path() string {
	return n.path
}

// Close closes the network namespace and cleans up
func (n *netNSE2E) Close() error {
	if n.file != nil {
		n.file.Close()
	}
	unix.Unmount(n.path, unix.MNT_DETACH)
	os.RemoveAll(filepath.Dir(n.path))
	return nil
}

// Do executes a function in the network namespace
func (n *netNSE2E) Do(toRun func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Save the current namespace
	origns, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to get current namespace: %v", err)
	}
	defer origns.Close()

	// Get the target namespace
	targetns, err := netns.GetFromPath(n.path)
	if err != nil {
		return fmt.Errorf("failed to get target namespace: %v", err)
	}
	defer targetns.Close()

	// Switch to the target namespace
	if err := netns.Set(targetns); err != nil {
		return fmt.Errorf("failed to set target namespace: %v", err)
	}

	// Run the function
	err = toRun()

	// Switch back to the original namespace
	if err2 := netns.Set(origns); err2 != nil {
		if err != nil {
			return fmt.Errorf("failed to restore namespace (original error: %v): %v", err, err2)
		}
		return fmt.Errorf("failed to restore namespace: %v", err2)
	}

	return err
}

// ============================================================================
// Test Constants
// ============================================================================
//
// These constants define the expected network configuration for daemon bridge
// namespace tests. The values match the CNI configuration templates below and
// are used to validate that the CNI plugins correctly configure the network.

const (
	// daemonBridgeNameE2E is the name of the bridge interface created in the test namespace.
	// This bridge connects daemon containers to the host network.
	daemonBridgeNameE2E = "ecs-dmn-br-e2e"

	// daemonIfNameE2E is the name of the interface created inside the daemon namespace.
	// This is the veth endpoint that the daemon container uses for network access.
	daemonIfNameE2E = "eth0"

	// daemonContainerIDE2E is the container ID passed to CNI plugins.
	// CNI plugins use this to track network allocations.
	daemonContainerIDE2E = "daemon-bridge-e2e"

	// ECS Link-Local Subnet Configuration
	// -----------------------------------
	// The ECS link-local subnet (169.254.172.0/22) is used for internal ECS communication.
	// This subnet is ALWAYS IPv4, even on IPv6-only hosts, because the ECS credentials
	// endpoint (169.254.170.2) only supports IPv4.

	// ecsLinkLocalSubnetE2E is the IPv4 link-local subnet used for ECS internal communication.
	ecsLinkLocalSubnetE2E = "169.254.172.0/22"

	// ecsLinkLocalGatewayE2E is the gateway address on the bridge for the ECS link-local subnet.
	// This is the first usable address in the subnet and is assigned to the bridge interface.
	ecsLinkLocalGatewayE2E = "169.254.172.1"

	// ecsCredentialsEndpointDstE2E is the destination for the ECS credentials endpoint route.
	// Containers access task credentials via this IPv4 link-local address.
	// CRITICAL: This route must exist on ALL hosts, including IPv6-only hosts.
	ecsCredentialsEndpointDstE2E = "169.254.170.2/32"

	// Expected Bridge Addresses
	// -------------------------
	// These are the addresses that should be assigned to the bridge interface
	// after CNI ADD completes successfully.

	// expectedDaemonBridgeIPv4E2E is the expected IPv4 address on the bridge.
	// This is the gateway address for the ECS link-local subnet.
	expectedDaemonBridgeIPv4E2E = "169.254.172.1/22"

	// expectedDaemonBridgeIPv6E2E is the expected IPv6 address on the bridge.
	// This is the gateway address for the IPv6 subnet used for external traffic.
	expectedDaemonBridgeIPv6E2E = "2001:db8:1::1/64"

	// Expected Veth Addresses
	// -----------------------
	// These are the addresses that should be assigned to the veth interface
	// inside the daemon namespace after CNI ADD completes successfully.

	// expectedDaemonVethIPv4E2E is the expected IPv4 address on the veth interface.
	// This is the second address in the ECS link-local subnet (first is the gateway).
	expectedDaemonVethIPv4E2E = "169.254.172.2/22"

	// expectedDaemonVethIPv6E2E is the expected IPv6 address on the veth interface.
	// This is the second address in the IPv6 subnet (first is the gateway).
	expectedDaemonVethIPv6E2E = "2001:db8:1::2/64"

	// IPv6 External Traffic Configuration
	// ------------------------------------
	// These constants define the IPv6 subnet used for external traffic routing.
	// Note: We use a documentation prefix (2001:db8::/32) for testing purposes.

	// daemonIPv6SubnetE2E is the IPv6 subnet used for external traffic.
	daemonIPv6SubnetE2E = "2001:db8:1::/64"

	// daemonIPv6GatewayE2E is the IPv6 gateway address for external traffic.
	// This is the first address in the IPv6 subnet and is assigned to the bridge.
	daemonIPv6GatewayE2E = "2001:db8:1::1"
)

// ============================================================================
// CNI Configuration Templates
// ============================================================================
//
// These templates define the CNI configuration passed to the ecs-bridge plugin.
// Each template is designed for a specific IP compatibility scenario:
//
//   - IPv4Only: For hosts with only IPv4 connectivity
//   - IPv6Only: For hosts with only IPv6 connectivity (but still needs IPv4 link-local for ECS)
//   - DualStack: For hosts with both IPv4 and IPv6 connectivity
//
// The %s placeholder is replaced with the bridge name at runtime.
//
// CNI Configuration Structure:
//   - type: The CNI plugin to invoke (ecs-bridge)
//   - cniVersion: CNI specification version (0.3.0)
//   - bridge: Name of the bridge interface to create/use
//   - ipam: IP Address Management configuration (delegated to ecs-ipam plugin)
//     - type: The IPAM plugin to use (ecs-ipam)
//     - id: Unique identifier for this IPAM allocation
//     - ipv4-subnet: IPv4 subnet for address allocation
//     - ipv4-routes: IPv4 routes to configure in the namespace
//     - ipv6-subnet: IPv6 subnet for address allocation (optional)
//     - ipv6-routes: IPv6 routes to configure in the namespace (optional)

// netConfDaemonIPv4OnlyE2E is the CNI configuration for IPv4-only daemon namespace.
//
// This configuration is used on hosts that only have IPv4 connectivity.
// It provides:
//   - IPv4 link-local subnet (169.254.172.0/22) for ECS credentials endpoint access
//   - Route to ECS credentials endpoint (169.254.170.2/32) via the bridge gateway
//   - IPv4 default route (0.0.0.0/0) for external traffic via IPv4
//
// Network topology after CNI ADD:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│ Test Namespace (simulates host)                             │
//	│  ┌─────────────────────────────────────────────────────┐    │
//	│  │ Bridge: ecs-dmn-br-e2e                              │    │
//	│  │ IPv4: 169.254.172.1/22                              │    │
//	│  └──────────────────────┬──────────────────────────────┘    │
//	│                         │ veth pair                         │
//	└─────────────────────────┼───────────────────────────────────┘
//	                          │
//	┌─────────────────────────┼───────────────────────────────────┐
//	│ Target Namespace (daemon container)                         │
//	│  ┌──────────────────────┴──────────────────────────────┐    │
//	│  │ eth0 (veth endpoint)                                │    │
//	│  │ IPv4: 169.254.172.2/22                              │    │
//	│  │ Routes:                                             │    │
//	│  │   - 169.254.170.2/32 via 169.254.172.1 (ECS creds)  │    │
//	│  │   - 0.0.0.0/0 via 169.254.172.1 (default)           │    │
//	│  └─────────────────────────────────────────────────────┘    │
//	└─────────────────────────────────────────────────────────────┘
const netConfDaemonIPv4OnlyE2E = `
{
    "type":"ecs-bridge",
    "cniVersion":"0.3.0",
    "bridge":"%s",
    "ipam":{
        "type":"ecs-ipam",
        "id":"daemon-ipv4-e2e",
        "cniVersion":"0.3.0",
        "ipv4-subnet":"169.254.172.0/22",
        "ipv4-routes":[
            {"dst":"169.254.170.2/32"},
            {"dst":"0.0.0.0/0"}
        ]
    }
}`

// netConfDaemonIPv6OnlyE2E is the CNI configuration for IPv6-only daemon namespace.
//
// This is the KEY configuration for testing IPv6 support in the ecs-bridge implementation.
// It validates that the bridge correctly handles the case where:
//   - External traffic routes via IPv6 (the host's primary connectivity)
//   - ECS credentials are still accessed via IPv4 link-local (ECS requirement)
//
// This configuration provides:
//   - IPv4 link-local subnet (169.254.172.0/22) for ECS credentials endpoint access
//     (ECS credentials endpoint is ALWAYS IPv4 link-local, even on IPv6-only hosts)
//   - Route to ECS credentials endpoint (169.254.170.2/32) via IPv4 link-local
//   - IPv6 subnet (2001:db8:1::/64) for external traffic
//   - IPv6 default route (::/0) for external traffic via IPv6
//
// IMPORTANT: Note that there is NO IPv4 default route (0.0.0.0/0) in this configuration.
// This is intentional - IPv6-only hosts should not have IPv4 external connectivity.
//
// Network topology after CNI ADD:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│ Test Namespace (simulates host)                             │
//	│  ┌─────────────────────────────────────────────────────┐    │
//	│  │ Bridge: ecs-dmn-br-e2e                              │    │
//	│  │ IPv4: 169.254.172.1/22 (for ECS credentials)        │    │
//	│  │ IPv6: 2001:db8:1::1/64 (for external traffic)       │    │
//	│  └──────────────────────┬──────────────────────────────┘    │
//	│                         │ veth pair                         │
//	└─────────────────────────┼───────────────────────────────────┘
//	                          │
//	┌─────────────────────────┼───────────────────────────────────┐
//	│ Target Namespace (daemon container)                         │
//	│  ┌──────────────────────┴──────────────────────────────┐    │
//	│  │ eth0 (veth endpoint)                                │    │
//	│  │ IPv4: 169.254.172.2/22 (for ECS credentials)        │    │
//	│  │ IPv6: 2001:db8:1::2/64 (for external traffic)       │    │
//	│  │ Routes:                                             │    │
//	│  │   - 169.254.170.2/32 via 169.254.172.1 (ECS creds)  │    │
//	│  │   - ::/0 via 2001:db8:1::1 (IPv6 default)           │    │
//	│  └─────────────────────────────────────────────────────┘    │
//	└─────────────────────────────────────────────────────────────┘
const netConfDaemonIPv6OnlyE2E = `
{
    "type":"ecs-bridge",
    "cniVersion":"0.3.0",
    "bridge":"%s",
    "ipam":{
        "type":"ecs-ipam",
        "id":"daemon-ipv6-e2e",
        "cniVersion":"0.3.0",
        "ipv4-subnet":"169.254.172.0/22",
        "ipv4-routes":[
            {"dst":"169.254.170.2/32"}
        ],
        "ipv6-subnet":"2001:db8:1::/64",
        "ipv6-routes":[
            {"dst":"::/0"}
        ]
    }
}`

// netConfDaemonDualStackE2E is the CNI configuration for dual-stack daemon namespace.
//
// This configuration is for hosts that have both IPv4 and IPv6 connectivity.
// It provides the most complete networking setup with:
//   - IPv4 link-local subnet (169.254.172.0/22) for ECS credentials endpoint access
//   - Route to ECS credentials endpoint (169.254.170.2/32)
//   - IPv4 default route (0.0.0.0/0) for external IPv4 traffic
//   - IPv6 subnet (2001:db8:1::/64) for external IPv6 traffic
//   - IPv6 default route (::/0) for external IPv6 traffic
//
// Network topology after CNI ADD:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│ Test Namespace (simulates host)                             │
//	│  ┌─────────────────────────────────────────────────────┐    │
//	│  │ Bridge: ecs-dmn-br-e2e                              │    │
//	│  │ IPv4: 169.254.172.1/22                              │    │
//	│  │ IPv6: 2001:db8:1::1/64                              │    │
//	│  └──────────────────────┬──────────────────────────────┘    │
//	│                         │ veth pair                         │
//	└─────────────────────────┼───────────────────────────────────┘
//	                          │
//	┌─────────────────────────┼───────────────────────────────────┐
//	│ Target Namespace (daemon container)                         │
//	│  ┌──────────────────────┴──────────────────────────────┐    │
//	│  │ eth0 (veth endpoint)                                │    │
//	│  │ IPv4: 169.254.172.2/22                              │    │
//	│  │ IPv6: 2001:db8:1::2/64                              │    │
//	│  │ Routes:                                             │    │
//	│  │   - 169.254.170.2/32 via 169.254.172.1 (ECS creds)  │    │
//	│  │   - 0.0.0.0/0 via 169.254.172.1 (IPv4 default)      │    │
//	│  │   - ::/0 via 2001:db8:1::1 (IPv6 default)           │    │
//	│  └─────────────────────────────────────────────────────┘    │
//	└─────────────────────────────────────────────────────────────┘
const netConfDaemonDualStackE2E = `
{
    "type":"ecs-bridge",
    "cniVersion":"0.3.0",
    "bridge":"%s",
    "ipam":{
        "type":"ecs-ipam",
        "id":"daemon-dualstack-e2e",
        "cniVersion":"0.3.0",
        "ipv4-subnet":"169.254.172.0/22",
        "ipv4-routes":[
            {"dst":"169.254.170.2/32"},
            {"dst":"0.0.0.0/0"}
        ],
        "ipv6-subnet":"2001:db8:1::/64",
        "ipv6-routes":[
            {"dst":"::/0"}
        ]
    }
}`

// ============================================================================
// Host IP Version Detection
// ============================================================================
//
// These functions detect the host's IP version capabilities by examining
// network interfaces. They are used to:
//   1. Skip tests that require IP versions the host doesn't support
//   2. Log the host's capabilities at test start for debugging
//
// Detection criteria:
//   - IPv4: At least one non-loopback interface has a non-link-local IPv4 address
//   - IPv6: At least one non-loopback interface has a non-link-local IPv6 address
//
// Note: Link-local addresses (169.254.x.x for IPv4, fe80:: for IPv6) are excluded
// because they don't indicate external connectivity.

// instanceSupportsIPv4E2E checks if the instance has IPv4 support by looking for
// non-loopback IPv4 addresses on any interface.
//
// Returns true if the host has at least one non-loopback, non-link-local IPv4 address.
// This indicates the host can route IPv4 traffic to external networks.
func instanceSupportsIPv4E2E() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ipNet.IP.To4() != nil && !ipNet.IP.IsLinkLocalUnicast() {
				return true
			}
		}
	}
	return false
}

// instanceSupportsIPv6E2E checks if the instance has IPv6 support by looking for
// non-loopback, non-link-local IPv6 addresses on any interface.
//
// Returns true if the host has at least one non-loopback, non-link-local IPv6 address.
// This indicates the host can route IPv6 traffic to external networks.
//
// Note: Link-local IPv6 addresses (fe80::) are excluded because they are automatically
// assigned to all interfaces and don't indicate external IPv6 connectivity.
func instanceSupportsIPv6E2E() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if ipNet.IP.To4() == nil && ipNet.IP.To16() != nil && !ipNet.IP.IsLinkLocalUnicast() {
				return true
			}
		}
	}
	return false
}

// skipIfNoIPv4E2E skips the current test if the host doesn't have IPv4 support.
// Use this at the start of tests that require IPv4 connectivity.
func skipIfNoIPv4E2E(t *testing.T) {
	if !instanceSupportsIPv4E2E() {
		t.Skip("Skipping test: instance does not have IPv4 support")
	}
}

// skipIfNoIPv6E2E skips the current test if the host doesn't have IPv6 support.
// Use this at the start of tests that require IPv6 connectivity.
func skipIfNoIPv6E2E(t *testing.T) {
	if !instanceSupportsIPv6E2E() {
		t.Skip("Skipping test: instance does not have IPv6 support")
	}
}

// skipIfNoDualStackE2E skips the current test if the host doesn't have both IPv4 and IPv6 support.
// Use this at the start of tests that require dual-stack connectivity.
func skipIfNoDualStackE2E(t *testing.T) {
	if !instanceSupportsIPv4E2E() || !instanceSupportsIPv6E2E() {
		t.Skip("Skipping test: instance does not have dual-stack (IPv4 + IPv6) support")
	}
}

// skipIfNotIPv6OnlyE2E skips the current test if the host is not IPv6-only.
// Use this at the start of tests that require IPv6-only connectivity (no IPv4).
func skipIfNotIPv6OnlyE2E(t *testing.T) {
	if instanceSupportsIPv4E2E() {
		t.Skip("Skipping test: instance has IPv4 support (not IPv6-only)")
	}
	if !instanceSupportsIPv6E2E() {
		t.Skip("Skipping test: instance does not have IPv6 support")
	}
}

// ============================================================================
// Helper Functions
// ============================================================================
//
// These helper functions provide common functionality used across all sub-tests:
//   - Environment variable handling
//   - Interface discovery and verification
//   - Link existence validation

// getEnvOrDefaultE2E returns the value of an environment variable, or a default value if not set.
// This is used to handle optional configuration like log preservation flags.
func getEnvOrDefaultE2E(name string, fallback string) string {
	val := os.Getenv(name)
	if val == "" {
		return fallback
	}
	return val
}

// getVethAndVerifyLoE2E discovers the veth interface in the current namespace and verifies
// that the loopback interface exists.
//
// This function is called after CNI ADD to verify that:
//  1. The loopback interface (lo) exists - required for local communication
//  2. A veth interface exists - the endpoint connecting to the bridge
//
// Returns:
//   - veth: The veth interface link, or nil if not found
//   - found: true if a veth interface was found
//
// The function uses require.True() to fail the test if loopback is not found,
// as this indicates a fundamental problem with the namespace setup.
func getVethAndVerifyLoE2E(t *testing.T) (netlink.Link, bool) {
	links, err := netlink.LinkList()
	require.NoError(t, err, "Unable to list devices")
	loFound := false
	vethFound := false
	var veth netlink.Link
	for _, link := range links {
		switch link.Type() {
		case "device":
			if link.Attrs().Name == "lo" {
				loFound = true
			}
		case "veth":
			vethFound = true
			veth = link
		}
	}
	require.True(t, loFound, "localhost interface not found in netns")
	return veth, vethFound
}

// validateLinkDoesNotExistE2E verifies that a network interface with the given name
// does not exist in the current namespace.
//
// This function is called after CNI DEL to verify that:
//   - The veth interface has been properly removed from both namespaces
//
// The function fails the test if:
//   - The link exists (cleanup failed)
//   - An error other than LinkNotFoundError occurs (unexpected state)
func validateLinkDoesNotExistE2E(t *testing.T, name string) {
	_, err := netlink.LinkByName(name)
	require.Error(t, err, "Link %s should not exist", name)
	_, ok := err.(netlink.LinkNotFoundError)
	require.True(t, ok, "Error type is incorrect for link '%s': %v", name, err)
}

// ============================================================================
// Validation Functions
// ============================================================================
//
// These functions validate the network configuration after CNI operations.
// They are organized by what they validate:
//
// Bridge validation:
//   - validateDaemonBridgeIPv4E2E: Verifies bridge has correct IPv4 address
//   - validateDaemonBridgeIPv6E2E: Verifies bridge has correct IPv6 address
//
// Veth validation:
//   - validateDaemonVethIPv4E2E: Verifies veth has correct IPv4 address
//   - validateDaemonVethIPv6E2E: Verifies veth has correct IPv6 address
//
// Route validation:
//   - validateDaemonRoutesIPv4E2E: Verifies IPv4 routes (ECS credentials + default)
//   - validateDaemonRoutesIPv4ECSCredentialsOnlyE2E: Verifies only ECS credentials route (for IPv6-only)
//   - validateDaemonRoutesIPv6E2E: Verifies IPv6 default route
//
// Debugging:
//   - logNetworkStateE2E: Logs complete network state for debugging failures

// validateDaemonBridgeIPv4E2E validates that the bridge has the expected IPv4 link-local address.
//
// After CNI ADD, the bridge should have the gateway address (169.254.172.1/22) assigned.
// This address serves as the gateway for containers in the daemon namespace.
//
// **Validates: Requirements 3.1 (bridge has expected IPv4 address)**
func validateDaemonBridgeIPv4E2E(t *testing.T, bridge netlink.Link) {
	addrs, err := netlink.AddrList(bridge, netlink.FAMILY_V4)
	require.NoError(t, err, "Unable to list the IPv4 addresses of: %s", bridge.Attrs().Name)
	addressFound := false
	for _, addr := range addrs {
		if addr.IPNet.String() == expectedDaemonBridgeIPv4E2E {
			addressFound = true
		}
	}
	require.True(t, addressFound, "IPv4 address '%s' not assigned to daemon bridge: %s",
		expectedDaemonBridgeIPv4E2E, bridge.Attrs().Name)
}

// validateDaemonVethIPv4E2E validates that the veth has an IPv4 address from the ECS link-local subnet.
//
// After CNI ADD, the veth interface in the daemon namespace should have an IPv4 address
// allocated from the ECS link-local subnet (169.254.172.0/22). The expected address is
// 169.254.172.2/22 (the second address in the subnet, after the gateway).
//
// **Validates: Requirements 3.2 (veth has IPv4 address from ECS subnet)**
func validateDaemonVethIPv4E2E(t *testing.T, veth netlink.Link) {
	addrs, err := netlink.AddrList(veth, netlink.FAMILY_V4)
	require.NoError(t, err, "Unable to list IPv4 addresses of: %s", veth.Attrs().Name)
	addressFound := false
	for _, addr := range addrs {
		if addr.IPNet.String() == expectedDaemonVethIPv4E2E {
			addressFound = true
		}
	}
	require.True(t, addressFound, "IPv4 address '%s' not associated with daemon veth: %s",
		expectedDaemonVethIPv4E2E, veth.Attrs().Name)
}

// validateDaemonRoutesIPv4E2E validates IPv4 routes for daemon host namespace connectivity.
// This function verifies:
// 1. Route to ECS credentials endpoint (169.254.170.2/32) via the bridge gateway
// 2. Gateway route exists (either default route 0.0.0.0/0 or gateway-specific route)
//
// **Validates: Requirements 3.3, 3.4, 4.4, 5.4**
func validateDaemonRoutesIPv4E2E(t *testing.T, veth netlink.Link) {
	routes, err := netlink.RouteList(veth, netlink.FAMILY_V4)
	require.NoError(t, err, "Unable to list routes for: %s", veth.Attrs().Name)

	ecsCredentialsRouteFound := false
	gatewayRouteFound := false

	for _, route := range routes {
		// Check for ECS credentials endpoint route (169.254.170.2/32) via gateway
		if route.Dst != nil && route.Dst.String() == ecsCredentialsEndpointDstE2E &&
			route.Gw != nil && route.Gw.String() == ecsLinkLocalGatewayE2E {
			ecsCredentialsRouteFound = true
		}
		// Check for gateway route - either default route (0.0.0.0/0) or gateway-specific route
		// The default route has Dst == nil or Dst == "0.0.0.0/0"
		isDefaultRoute := route.Dst == nil || (route.Dst != nil && route.Dst.String() == "0.0.0.0/0")
		if isDefaultRoute && route.Gw != nil && route.Gw.String() == ecsLinkLocalGatewayE2E {
			gatewayRouteFound = true
		}
	}

	require.True(t, ecsCredentialsRouteFound,
		"Route to ECS credentials endpoint '%s' via gateway '%s' not found for: %s",
		ecsCredentialsEndpointDstE2E, ecsLinkLocalGatewayE2E, veth.Attrs().Name)
	require.True(t, gatewayRouteFound,
		"IPv4 default route (0.0.0.0/0) via gateway '%s' not found for: %s",
		ecsLinkLocalGatewayE2E, veth.Attrs().Name)
}

// validateDaemonRoutesIPv4ECSCredentialsOnlyE2E validates IPv4 routes for IPv6-only hosts.
// This function verifies only the ECS credentials endpoint route exists.
// For IPv6-only hosts, there should be NO IPv4 default route - only the ECS credentials route.
//
// **Validates: Requirements 4.4**
func validateDaemonRoutesIPv4ECSCredentialsOnlyE2E(t *testing.T, veth netlink.Link) {
	routes, err := netlink.RouteList(veth, netlink.FAMILY_V4)
	require.NoError(t, err, "Unable to list routes for: %s", veth.Attrs().Name)

	ecsCredentialsRouteFound := false

	for _, route := range routes {
		// Check for ECS credentials endpoint route (169.254.170.2/32) via gateway
		if route.Dst != nil && route.Dst.String() == ecsCredentialsEndpointDstE2E &&
			route.Gw != nil && route.Gw.String() == ecsLinkLocalGatewayE2E {
			ecsCredentialsRouteFound = true
		}
	}

	require.True(t, ecsCredentialsRouteFound,
		"Route to ECS credentials endpoint '%s' via gateway '%s' not found for: %s (IPv6-only hosts still need IPv4 link-local for ECS credentials)",
		ecsCredentialsEndpointDstE2E, ecsLinkLocalGatewayE2E, veth.Attrs().Name)
}

// validateDaemonBridgeIPv6E2E validates that the bridge has the expected IPv6 address.
//
// After CNI ADD with IPv6 configuration, the bridge should have the IPv6 gateway address
// (2001:db8:1::1/64) assigned. This address serves as the gateway for IPv6 traffic
// from containers in the daemon namespace.
//
// **Validates: Requirements 4.1 (bridge has expected IPv6 address)**
func validateDaemonBridgeIPv6E2E(t *testing.T, bridge netlink.Link) {
	addrs, err := netlink.AddrList(bridge, netlink.FAMILY_V6)
	require.NoError(t, err, "Unable to list the IPv6 addresses of: %s", bridge.Attrs().Name)
	addressFound := false
	for _, addr := range addrs {
		if addr.IPNet.String() == expectedDaemonBridgeIPv6E2E {
			addressFound = true
		}
	}
	require.True(t, addressFound, "IPv6 address '%s' not assigned to daemon bridge: %s",
		expectedDaemonBridgeIPv6E2E, bridge.Attrs().Name)
}

// validateDaemonVethIPv6E2E validates that the veth has an IPv6 address.
//
// After CNI ADD with IPv6 configuration, the veth interface in the daemon namespace
// should have an IPv6 address allocated from the configured subnet (2001:db8:1::/64).
// The expected address is 2001:db8:1::2/64 (the second address in the subnet).
//
// **Validates: Requirements 4.2 (veth has IPv6 address from configured subnet)**
func validateDaemonVethIPv6E2E(t *testing.T, veth netlink.Link) {
	addrs, err := netlink.AddrList(veth, netlink.FAMILY_V6)
	require.NoError(t, err, "Unable to list IPv6 addresses of: %s", veth.Attrs().Name)
	addressFound := false
	for _, addr := range addrs {
		if addr.IPNet.String() == expectedDaemonVethIPv6E2E {
			addressFound = true
		}
	}
	require.True(t, addressFound, "IPv6 address '%s' not associated with daemon veth: %s",
		expectedDaemonVethIPv6E2E, veth.Attrs().Name)
}

// validateDaemonRoutesIPv6E2E validates IPv6 default route for external traffic.
// This function verifies:
// 1. IPv6 default route (::/0) exists with the correct gateway (2001:db8:1::1)
//
// Note: The ECS credentials endpoint route is always IPv4 (169.254.170.2/32),
// even on IPv6-only hosts, so it's validated separately by validateDaemonRoutesIPv4E2E.
//
// **Validates: Requirements 4.3, 4.5, 5.4**
func validateDaemonRoutesIPv6E2E(t *testing.T, veth netlink.Link) {
	routes, err := netlink.RouteList(veth, netlink.FAMILY_V6)
	require.NoError(t, err, "Unable to list IPv6 routes for: %s", veth.Attrs().Name)

	defaultRouteFound := false

	for _, route := range routes {
		// Check for IPv6 default route (::/0) via the configured gateway
		// The default route has Dst == nil or Dst == "::/0"
		isDefaultRoute := route.Dst == nil || (route.Dst != nil && route.Dst.String() == "::/0")
		if isDefaultRoute && route.Gw != nil && route.Gw.String() == daemonIPv6GatewayE2E {
			defaultRouteFound = true
		}
	}

	require.True(t, defaultRouteFound,
		"IPv6 default route (::/0) with gateway '%s' not found for: %s",
		daemonIPv6GatewayE2E, veth.Attrs().Name)
}

// logNetworkStateE2E logs the current network configuration for debugging.
//
// This function is called:
//   - After successful CNI ADD to show the configured state
//   - After test failures to help diagnose what went wrong
//
// It logs:
//   - All network interfaces with their type and operational state
//   - IPv4 addresses assigned to each interface
//   - IPv6 addresses assigned to each interface
//   - All IPv4 routes in the namespace
//   - All IPv6 routes in the namespace
//
// **Validates: Requirements 8.1 (log network state on failure), 8.3 (log CNI configuration)**
func logNetworkStateE2E(t *testing.T, targetNS *netNSE2E, description string) {
	targetNS.Do(func() error {
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

// TestDaemonBridgeNetworkNamespace tests the ecs-bridge plugin's ability to create
// a daemon namespace with connectivity to both the ECS credentials endpoint
// (via IPv4 link-local) and external networks (via IPv4 or IPv6 depending
// on host support).
//
// This test executes the actual CNI plugin binaries (ecs-bridge and ecs-ipam)
// to validate the integration between the ECS agent's netlib/platform code
// and the amazon-ecs-cni-plugins.
//
// Test structure:
// - Common setup: log directory, IPAM database, plugin path discovery
// - IPv4Only sub-test: Tests IPv4-only configuration
// - IPv6Only sub-test: Tests IPv6-only configuration (key for exposing bugs)
// - DualStack sub-test: Tests dual-stack configuration
//
// **Validates: Requirements 1.1-1.6, 2.1-2.5, 3.1-3.3, 4.1-4.5, 5.1-5.5, 6.1-6.5**
func TestDaemonBridgeNetworkNamespace(t *testing.T) {
	// ========================================================================
	// Common Setup
	// ========================================================================

	hasIPv4 := instanceSupportsIPv4E2E()
	hasIPv6 := instanceSupportsIPv6E2E()

	t.Logf("Host IP version support: IPv4=%v, IPv6=%v", hasIPv4, hasIPv6)

	if !hasIPv4 && !hasIPv6 {
		t.Skip("Skipping test: host supports neither IPv4 nor IPv6")
	}

	// Ensure that the bridge plugin exists
	bridgePluginPath, err := invoke.FindInPath("ecs-bridge", []string{os.Getenv("CNI_PATH")})
	require.NoError(t, err, "Unable to find bridge plugin in path. Set CNI_PATH environment variable.")
	t.Logf("Using bridge plugin: %s", bridgePluginPath)

	// Create a directory for storing test logs
	testLogDir, err := ioutil.TempDir("", "ecs-bridge-e2e-daemon-bridge-")
	require.NoError(t, err, "Unable to create directory for storing test logs")

	os.Setenv("ECS_CNI_LOG_FILE", fmt.Sprintf("%s/bridge.log", testLogDir))
	t.Logf("Using %s for test logs", testLogDir)
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
		t.Logf("Running IPv4-only daemon bridge namespace test")

		ipamDir, err := ioutil.TempDir("", "ecs-ipam-daemon-bridge-ipv4-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		preserveIPAM, err := strconv.ParseBool(getEnvOrDefaultE2E("ECS_BRIDGE_PRESERVE_IPAM_DB", "false"))
		assert.NoError(t, err, "Unable to parse ECS_BRIDGE_PRESERVE_IPAM_DB env var")
		if !preserveIPAM {
			defer os.RemoveAll(ipamDir)
		}

		testNS, err := newNetNSE2E()
		require.NoError(t, err, "Unable to create the network namespace to run the test in")
		defer testNS.Close()

		targetNS, err := newNetNSE2E()
		require.NoError(t, err, "Unable to create the network namespace that represents the daemon namespace")
		defer targetNS.Close()

		execInvokeArgs := &invoke.Args{
			ContainerID: daemonContainerIDE2E,
			NetNS:       targetNS.Path(),
			IfName:      daemonIfNameE2E,
			Path:        os.Getenv("CNI_PATH"),
		}

		var vethTestNetNS netlink.Link
		var ok bool

		testNS.Do(func() error {
			execInvokeArgs.Command = "ADD"
			ctx := context.Background()
			_, err := invoke.ExecPluginWithResult(
				ctx,
				bridgePluginPath,
				[]byte(fmt.Sprintf(netConfDaemonIPv4OnlyE2E, daemonBridgeNameE2E)),
				execInvokeArgs,
				nil)
			require.NoError(t, err, "Unable to execute ADD command for ecs-bridge plugin with IPv4-only daemon config")

			bridge, err := netlink.LinkByName(daemonBridgeNameE2E)
			require.NoError(t, err, "Unable to find daemon bridge: %s", daemonBridgeNameE2E)
			validateDaemonBridgeIPv4E2E(t, bridge)

			vethTestNetNS, ok = getVethAndVerifyLoE2E(t)
			require.True(t, ok, "veth device not found in test netns")
			return nil
		})

		if t.Failed() {
			return
		}

		var vethTargetNetNS netlink.Link
		targetNS.Do(func() error {
			vethTargetNetNS, ok = getVethAndVerifyLoE2E(t)
			require.True(t, ok, "veth device not found in target netns")

			validateDaemonVethIPv4E2E(t, vethTargetNetNS)
			validateDaemonRoutesIPv4E2E(t, vethTargetNetNS)

			logNetworkStateE2E(t, targetNS, "After ADD - IPv4Only")
			return nil
		})

		if t.Failed() {
			return
		}

		testNS.Do(func() error {
			execInvokeArgs.Command = "DEL"
			ctx := context.Background()
			err := invoke.ExecPluginWithoutResult(
				ctx,
				bridgePluginPath,
				[]byte(fmt.Sprintf(netConfDaemonIPv4OnlyE2E, daemonBridgeNameE2E)),
				execInvokeArgs,
				nil)
			require.NoError(t, err, "Unable to execute DEL command for ecs-bridge plugin")

			validateLinkDoesNotExistE2E(t, vethTestNetNS.Attrs().Name)

			bridge, err := netlink.LinkByName(daemonBridgeNameE2E)
			require.NoError(t, err, "Bridge should still exist after DEL")
			validateDaemonBridgeIPv4E2E(t, bridge)
			return nil
		})

		targetNS.Do(func() error {
			validateLinkDoesNotExistE2E(t, vethTargetNetNS.Attrs().Name)
			return nil
		})

		t.Log("IPv4-only daemon bridge namespace test completed successfully")
	})

	// ========================================================================
	// IPv6-only sub-test (key test for IPv6 support)
	// ========================================================================
	t.Run("IPv6Only", func(t *testing.T) {
		skipIfNoIPv6E2E(t)
		t.Logf("Running IPv6-only daemon bridge namespace test")
		t.Logf("NOTE: This test uses IPv4 link-local for ECS credentials + IPv6 for external traffic")

		ipamDir, err := ioutil.TempDir("", "ecs-ipam-daemon-bridge-ipv6-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		preserveIPAM, err := strconv.ParseBool(getEnvOrDefaultE2E("ECS_BRIDGE_PRESERVE_IPAM_DB", "false"))
		assert.NoError(t, err, "Unable to parse ECS_BRIDGE_PRESERVE_IPAM_DB env var")
		if !preserveIPAM {
			defer os.RemoveAll(ipamDir)
		}

		testNS, err := newNetNSE2E()
		require.NoError(t, err, "Unable to create the network namespace to run the test in")
		defer testNS.Close()

		targetNS, err := newNetNSE2E()
		require.NoError(t, err, "Unable to create the network namespace that represents the daemon namespace")
		defer targetNS.Close()

		execInvokeArgs := &invoke.Args{
			ContainerID: daemonContainerIDE2E,
			NetNS:       targetNS.Path(),
			IfName:      daemonIfNameE2E,
			Path:        os.Getenv("CNI_PATH"),
		}

		var vethTestNetNS netlink.Link
		var ok bool

		testNS.Do(func() error {
			execInvokeArgs.Command = "ADD"
			ctx := context.Background()
			_, err := invoke.ExecPluginWithResult(
				ctx,
				bridgePluginPath,
				[]byte(fmt.Sprintf(netConfDaemonIPv6OnlyE2E, daemonBridgeNameE2E)),
				execInvokeArgs,
				nil)
			require.NoError(t, err, "Unable to execute ADD command for ecs-bridge plugin with IPv6-only daemon config")

			bridge, err := netlink.LinkByName(daemonBridgeNameE2E)
			require.NoError(t, err, "Unable to find daemon bridge: %s", daemonBridgeNameE2E)
			validateDaemonBridgeIPv4E2E(t, bridge)
			validateDaemonBridgeIPv6E2E(t, bridge)

			vethTestNetNS, ok = getVethAndVerifyLoE2E(t)
			require.True(t, ok, "veth device not found in test netns")
			return nil
		})

		if t.Failed() {
			testNS.Do(func() error {
				logNetworkStateE2E(t, testNS, "After ADD failure - IPv6Only (test namespace)")
				return nil
			})
			return
		}

		var vethTargetNetNS netlink.Link
		targetNS.Do(func() error {
			vethTargetNetNS, ok = getVethAndVerifyLoE2E(t)
			require.True(t, ok, "veth device not found in target netns")

			validateDaemonVethIPv4E2E(t, vethTargetNetNS)
			validateDaemonVethIPv6E2E(t, vethTargetNetNS)
			// For IPv6-only, we only check for ECS credentials route (no IPv4 default route)
			validateDaemonRoutesIPv4ECSCredentialsOnlyE2E(t, vethTargetNetNS)
			validateDaemonRoutesIPv6E2E(t, vethTargetNetNS)

			logNetworkStateE2E(t, targetNS, "After ADD - IPv6Only")
			return nil
		})

		if t.Failed() {
			return
		}

		testNS.Do(func() error {
			execInvokeArgs.Command = "DEL"
			ctx := context.Background()
			err := invoke.ExecPluginWithoutResult(
				ctx,
				bridgePluginPath,
				[]byte(fmt.Sprintf(netConfDaemonIPv6OnlyE2E, daemonBridgeNameE2E)),
				execInvokeArgs,
				nil)
			require.NoError(t, err, "Unable to execute DEL command for ecs-bridge plugin")

			validateLinkDoesNotExistE2E(t, vethTestNetNS.Attrs().Name)

			bridge, err := netlink.LinkByName(daemonBridgeNameE2E)
			require.NoError(t, err, "Bridge should still exist after DEL")
			validateDaemonBridgeIPv4E2E(t, bridge)
			validateDaemonBridgeIPv6E2E(t, bridge)
			return nil
		})

		targetNS.Do(func() error {
			validateLinkDoesNotExistE2E(t, vethTargetNetNS.Attrs().Name)
			return nil
		})

		t.Log("IPv6-only daemon bridge namespace test completed successfully")
	})

	// ========================================================================
	// Dual-stack sub-test
	// ========================================================================
	t.Run("DualStack", func(t *testing.T) {
		skipIfNoDualStackE2E(t)
		t.Logf("Running dual-stack daemon bridge namespace test")
		t.Logf("NOTE: This test uses IPv4 link-local for ECS credentials + both IPv4 and IPv6 for external traffic")

		ipamDir, err := ioutil.TempDir("", "ecs-ipam-daemon-bridge-dualstack-")
		require.NoError(t, err, "Unable to create a temp directory for the ipam db")
		os.Setenv("IPAM_DB_PATH", fmt.Sprintf("%s/ipam.db", ipamDir))
		defer os.Unsetenv("IPAM_DB_PATH")
		preserveIPAM, err := strconv.ParseBool(getEnvOrDefaultE2E("ECS_BRIDGE_PRESERVE_IPAM_DB", "false"))
		assert.NoError(t, err, "Unable to parse ECS_BRIDGE_PRESERVE_IPAM_DB env var")
		if !preserveIPAM {
			defer os.RemoveAll(ipamDir)
		}

		testNS, err := newNetNSE2E()
		require.NoError(t, err, "Unable to create the network namespace to run the test in")
		defer testNS.Close()

		targetNS, err := newNetNSE2E()
		require.NoError(t, err, "Unable to create the network namespace that represents the daemon namespace")
		defer targetNS.Close()

		execInvokeArgs := &invoke.Args{
			ContainerID: daemonContainerIDE2E,
			NetNS:       targetNS.Path(),
			IfName:      daemonIfNameE2E,
			Path:        os.Getenv("CNI_PATH"),
		}

		var vethTestNetNS netlink.Link
		var ok bool

		testNS.Do(func() error {
			execInvokeArgs.Command = "ADD"
			ctx := context.Background()
			_, err := invoke.ExecPluginWithResult(
				ctx,
				bridgePluginPath,
				[]byte(fmt.Sprintf(netConfDaemonDualStackE2E, daemonBridgeNameE2E)),
				execInvokeArgs,
				nil)
			require.NoError(t, err, "Unable to execute ADD command for ecs-bridge plugin with dual-stack daemon config")

			bridge, err := netlink.LinkByName(daemonBridgeNameE2E)
			require.NoError(t, err, "Unable to find daemon bridge: %s", daemonBridgeNameE2E)
			validateDaemonBridgeIPv4E2E(t, bridge)
			validateDaemonBridgeIPv6E2E(t, bridge)

			vethTestNetNS, ok = getVethAndVerifyLoE2E(t)
			require.True(t, ok, "veth device not found in test netns")
			return nil
		})

		if t.Failed() {
			testNS.Do(func() error {
				logNetworkStateE2E(t, testNS, "After ADD failure - DualStack (test namespace)")
				return nil
			})
			return
		}

		var vethTargetNetNS netlink.Link
		targetNS.Do(func() error {
			vethTargetNetNS, ok = getVethAndVerifyLoE2E(t)
			require.True(t, ok, "veth device not found in target netns")

			validateDaemonVethIPv4E2E(t, vethTargetNetNS)
			validateDaemonVethIPv6E2E(t, vethTargetNetNS)
			validateDaemonRoutesIPv4E2E(t, vethTargetNetNS)
			validateDaemonRoutesIPv6E2E(t, vethTargetNetNS)

			logNetworkStateE2E(t, targetNS, "After ADD - DualStack")
			return nil
		})

		if t.Failed() {
			return
		}

		testNS.Do(func() error {
			execInvokeArgs.Command = "DEL"
			ctx := context.Background()
			err := invoke.ExecPluginWithoutResult(
				ctx,
				bridgePluginPath,
				[]byte(fmt.Sprintf(netConfDaemonDualStackE2E, daemonBridgeNameE2E)),
				execInvokeArgs,
				nil)
			require.NoError(t, err, "Unable to execute DEL command for ecs-bridge plugin")

			validateLinkDoesNotExistE2E(t, vethTestNetNS.Attrs().Name)

			bridge, err := netlink.LinkByName(daemonBridgeNameE2E)
			require.NoError(t, err, "Bridge should still exist after DEL")
			validateDaemonBridgeIPv4E2E(t, bridge)
			validateDaemonBridgeIPv6E2E(t, bridge)
			return nil
		})

		targetNS.Do(func() error {
			validateLinkDoesNotExistE2E(t, vethTargetNetNS.Attrs().Name)
			return nil
		})

		t.Log("Dual-stack daemon bridge namespace test completed successfully")
	})
}
