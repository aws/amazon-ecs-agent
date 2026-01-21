//go:build !windows && unit
// +build !windows,unit

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
	"testing"
	"testing/quick"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
)

// =============================================================================
// Property-Based Tests for Daemon Bridge Plugin Configuration
// =============================================================================

// Feature: daemon-bridge-ipv6-support, Property 2: ECS Agent Endpoint Route Invariant
//
// For any IP compatibility configuration (IPv4-only, IPv6-only, or dual-stack),
// the daemon bridge plugin configuration SHALL always include the IPv4 route to
// the ECS agent endpoint (169.254.170.2/32) in the IPv4Routes field.
//
// **Validates: Requirements 3.2, 5.1, 5.2, 5.3**
func TestProperty_ECSAgentEndpointRouteInvariant(t *testing.T) {
	// Parse the expected agent endpoint for comparison
	_, expectedAgentEndpointNet, err := net.ParseCIDR(AgentEndpoint)
	if err != nil {
		t.Fatalf("Failed to parse AgentEndpoint constant: %v", err)
	}

	f := func(
		// Parameters to generate different IP compatibility configurations
		ipv4Compatible bool,
		ipv6Compatible bool,
		// Parameter to generate different network namespace paths
		nsPathSuffix uint8,
	) bool {
		// Skip the case where neither IPv4 nor IPv6 is compatible
		// (this is an error case that returns an error, not a valid configuration)
		if !ipv4Compatible && !ipv6Compatible {
			return true
		}

		// Create IP compatibility based on generated values
		ipComp := ipcompatibility.NewIPCompatibility(ipv4Compatible, ipv6Compatible)

		// Generate a network namespace path
		netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

		// Call the function under test
		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err != nil {
			t.Logf("createDaemonBridgePluginConfig returned error: %v", err)
			return false
		}

		// Extract the BridgeConfig from the PluginConfig interface
		bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
		if !ok {
			t.Logf("Expected *ecscni.BridgeConfig, got %T", pluginConfig)
			return false
		}

		// Verify that IPv4Routes is not nil and contains at least one route
		if bridgeConfig.IPAM.IPV4Routes == nil || len(bridgeConfig.IPAM.IPV4Routes) == 0 {
			t.Logf("IPv4Routes is nil or empty for ipv4Compatible=%v, ipv6Compatible=%v",
				ipv4Compatible, ipv6Compatible)
			return false
		}

		// Check that the ECS agent endpoint route (169.254.170.2/32) is present
		agentEndpointFound := false
		for _, route := range bridgeConfig.IPAM.IPV4Routes {
			if route != nil && route.Dst.String() == expectedAgentEndpointNet.String() {
				agentEndpointFound = true
				break
			}
		}

		if !agentEndpointFound {
			t.Logf("ECS agent endpoint route (%s) not found in IPv4Routes for ipv4Compatible=%v, ipv6Compatible=%v",
				AgentEndpoint, ipv4Compatible, ipv6Compatible)
			t.Logf("IPv4Routes: %v", bridgeConfig.IPAM.IPV4Routes)
			return false
		}

		return true
	}

	// Run with at least 100 iterations as specified in the design
	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// TestProperty_ECSAgentEndpointRouteInvariant_AllConfigurations tests the property
// exhaustively for all three valid IP compatibility configurations.
//
// Feature: daemon-bridge-ipv6-support, Property 2: ECS Agent Endpoint Route Invariant
// **Validates: Requirements 3.2, 5.1, 5.2, 5.3**
func TestProperty_ECSAgentEndpointRouteInvariant_AllConfigurations(t *testing.T) {
	// Parse the expected agent endpoint for comparison
	_, expectedAgentEndpointNet, err := net.ParseCIDR(AgentEndpoint)
	if err != nil {
		t.Fatalf("Failed to parse AgentEndpoint constant: %v", err)
	}

	testCases := []struct {
		name   string
		ipComp ipcompatibility.IPCompatibility
	}{
		{
			name:   "IPv4-only",
			ipComp: ipcompatibility.NewIPv4OnlyCompatibility(),
		},
		{
			name:   "IPv6-only",
			ipComp: ipcompatibility.NewIPv6OnlyCompatibility(),
		},
		{
			name:   "Dual-stack",
			ipComp: ipcompatibility.NewDualStackCompatibility(),
		},
	}

	// Run property test for each configuration with multiple namespace paths
	f := func(nsPathSuffix uint8) bool {
		for _, tc := range testCases {
			netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

			pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, tc.ipComp)
			if err != nil {
				t.Logf("[%s] createDaemonBridgePluginConfig returned error: %v", tc.name, err)
				return false
			}

			bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
			if !ok {
				t.Logf("[%s] Expected *ecscni.BridgeConfig, got %T", tc.name, pluginConfig)
				return false
			}

			if bridgeConfig.IPAM.IPV4Routes == nil || len(bridgeConfig.IPAM.IPV4Routes) == 0 {
				t.Logf("[%s] IPv4Routes is nil or empty", tc.name)
				return false
			}

			agentEndpointFound := false
			for _, route := range bridgeConfig.IPAM.IPV4Routes {
				if route != nil && route.Dst.String() == expectedAgentEndpointNet.String() {
					agentEndpointFound = true
					break
				}
			}

			if !agentEndpointFound {
				t.Logf("[%s] ECS agent endpoint route (%s) not found in IPv4Routes",
					tc.name, AgentEndpoint)
				return false
			}
		}
		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// TestProperty_ECSAgentEndpointRouteInvariant_ErrorCase verifies that the function
// returns an error when neither IPv4 nor IPv6 is compatible.
//
// Feature: daemon-bridge-ipv6-support, Property 2: ECS Agent Endpoint Route Invariant
// **Validates: Requirements 3.2, 5.1, 5.2, 5.3**
func TestProperty_ECSAgentEndpointRouteInvariant_ErrorCase(t *testing.T) {
	f := func(nsPathSuffix uint8) bool {
		// Create IP compatibility with neither IPv4 nor IPv6 compatible
		ipComp := ipcompatibility.NewIPCompatibility(false, false)

		netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

		// Call the function under test - should return an error
		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err == nil {
			t.Logf("Expected error for neither IPv4 nor IPv6 compatible, but got nil error")
			t.Logf("pluginConfig: %v", pluginConfig)
			return false
		}

		// Verify the error message
		expectedErrMsg := "host is neither IPv4 nor IPv6 compatible"
		if err.Error() != expectedErrMsg {
			t.Logf("Expected error message %q, got %q", expectedErrMsg, err.Error())
			return false
		}

		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// itoa converts an integer to a string (simple implementation to avoid strconv import)
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// =============================================================================
// Property 3: IPv4 Default Route Presence
// =============================================================================

// Feature: daemon-bridge-ipv6-support, Property 3: IPv4 Default Route Presence
//
// For any IP compatibility configuration where IPv4 is compatible, the daemon
// bridge plugin configuration SHALL include an IPv4 default route (0.0.0.0/0)
// with gateway 169.254.172.1 in the IPv4Routes field.
//
// **Validates: Requirements 3.3, 4.3**
func TestProperty_IPv4DefaultRoutePresence(t *testing.T) {
	// Parse the expected default route destination and gateway for comparison
	_, expectedDefaultRouteNet, err := net.ParseCIDR(DefaultRouteDestination)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestination constant: %v", err)
	}
	expectedGateway := net.ParseIP(DaemonBridgeGatewayIP)
	if expectedGateway == nil {
		t.Fatalf("Failed to parse DaemonBridgeGatewayIP constant")
	}

	f := func(
		// Parameter to control IPv6 compatibility (IPv4 is always true for this test)
		ipv6Compatible bool,
		// Parameter to generate different network namespace paths
		nsPathSuffix uint8,
	) bool {
		// IPv4 is always compatible for this property test
		ipv4Compatible := true

		// Create IP compatibility based on generated values
		ipComp := ipcompatibility.NewIPCompatibility(ipv4Compatible, ipv6Compatible)

		// Generate a network namespace path
		netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

		// Call the function under test
		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err != nil {
			t.Logf("createDaemonBridgePluginConfig returned error: %v", err)
			return false
		}

		// Extract the BridgeConfig from the PluginConfig interface
		bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
		if !ok {
			t.Logf("Expected *ecscni.BridgeConfig, got %T", pluginConfig)
			return false
		}

		// Verify that IPv4Routes is not nil and contains at least one route
		if bridgeConfig.IPAM.IPV4Routes == nil || len(bridgeConfig.IPAM.IPV4Routes) == 0 {
			t.Logf("IPv4Routes is nil or empty for ipv4Compatible=%v, ipv6Compatible=%v",
				ipv4Compatible, ipv6Compatible)
			return false
		}

		// Check that the IPv4 default route (0.0.0.0/0) with gateway 169.254.172.1 is present
		defaultRouteFound := false
		for _, route := range bridgeConfig.IPAM.IPV4Routes {
			if route != nil &&
				route.Dst.String() == expectedDefaultRouteNet.String() &&
				route.GW != nil &&
				route.GW.Equal(expectedGateway) {
				defaultRouteFound = true
				break
			}
		}

		if !defaultRouteFound {
			t.Logf("IPv4 default route (%s) with gateway (%s) not found in IPv4Routes for ipv4Compatible=%v, ipv6Compatible=%v",
				DefaultRouteDestination, DaemonBridgeGatewayIP, ipv4Compatible, ipv6Compatible)
			t.Logf("IPv4Routes: %v", bridgeConfig.IPAM.IPV4Routes)
			return false
		}

		return true
	}

	// Run with at least 100 iterations as specified in the design
	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// TestProperty_IPv4DefaultRoutePresence_AllIPv4CompatibleConfigurations tests the property
// exhaustively for all IP compatibility configurations where IPv4 is compatible.
//
// Feature: daemon-bridge-ipv6-support, Property 3: IPv4 Default Route Presence
// **Validates: Requirements 3.3, 4.3**
func TestProperty_IPv4DefaultRoutePresence_AllIPv4CompatibleConfigurations(t *testing.T) {
	// Parse the expected default route destination and gateway for comparison
	_, expectedDefaultRouteNet, err := net.ParseCIDR(DefaultRouteDestination)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestination constant: %v", err)
	}
	expectedGateway := net.ParseIP(DaemonBridgeGatewayIP)
	if expectedGateway == nil {
		t.Fatalf("Failed to parse DaemonBridgeGatewayIP constant")
	}

	// Test cases for IPv4-compatible configurations only
	testCases := []struct {
		name   string
		ipComp ipcompatibility.IPCompatibility
	}{
		{
			name:   "IPv4-only",
			ipComp: ipcompatibility.NewIPv4OnlyCompatibility(),
		},
		{
			name:   "Dual-stack",
			ipComp: ipcompatibility.NewDualStackCompatibility(),
		},
	}

	// Run property test for each configuration with multiple namespace paths
	f := func(nsPathSuffix uint8) bool {
		for _, tc := range testCases {
			netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

			pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, tc.ipComp)
			if err != nil {
				t.Logf("[%s] createDaemonBridgePluginConfig returned error: %v", tc.name, err)
				return false
			}

			bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
			if !ok {
				t.Logf("[%s] Expected *ecscni.BridgeConfig, got %T", tc.name, pluginConfig)
				return false
			}

			if bridgeConfig.IPAM.IPV4Routes == nil || len(bridgeConfig.IPAM.IPV4Routes) == 0 {
				t.Logf("[%s] IPv4Routes is nil or empty", tc.name)
				return false
			}

			defaultRouteFound := false
			for _, route := range bridgeConfig.IPAM.IPV4Routes {
				if route != nil &&
					route.Dst.String() == expectedDefaultRouteNet.String() &&
					route.GW != nil &&
					route.GW.Equal(expectedGateway) {
					defaultRouteFound = true
					break
				}
			}

			if !defaultRouteFound {
				t.Logf("[%s] IPv4 default route (%s) with gateway (%s) not found in IPv4Routes",
					tc.name, DefaultRouteDestination, DaemonBridgeGatewayIP)
				return false
			}
		}
		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// TestProperty_IPv4DefaultRoutePresence_IPv6OnlyNoIPv4DefaultRoute verifies that
// IPv6-only configurations do NOT include the IPv4 default route (only the ECS agent endpoint).
//
// Feature: daemon-bridge-ipv6-support, Property 3: IPv4 Default Route Presence
// **Validates: Requirements 3.3, 4.3**
func TestProperty_IPv4DefaultRoutePresence_IPv6OnlyNoIPv4DefaultRoute(t *testing.T) {
	// Parse the expected default route destination for comparison
	_, expectedDefaultRouteNet, err := net.ParseCIDR(DefaultRouteDestination)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestination constant: %v", err)
	}

	f := func(nsPathSuffix uint8) bool {
		// Create IPv6-only compatibility
		ipComp := ipcompatibility.NewIPv6OnlyCompatibility()

		netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err != nil {
			t.Logf("createDaemonBridgePluginConfig returned error: %v", err)
			return false
		}

		bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
		if !ok {
			t.Logf("Expected *ecscni.BridgeConfig, got %T", pluginConfig)
			return false
		}

		// IPv4Routes should still exist (for ECS agent endpoint), but should NOT contain
		// the IPv4 default route (0.0.0.0/0)
		for _, route := range bridgeConfig.IPAM.IPV4Routes {
			if route != nil && route.Dst.String() == expectedDefaultRouteNet.String() {
				t.Logf("IPv4 default route (%s) should NOT be present for IPv6-only configuration",
					DefaultRouteDestination)
				return false
			}
		}

		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// =============================================================================
// Property 4: IPv6 Default Route Presence
// =============================================================================

// Feature: daemon-bridge-ipv6-support, Property 4: IPv6 Default Route Presence
//
// For any IP compatibility configuration where IPv6 is compatible, the daemon
// bridge plugin configuration SHALL include an IPv6 default route (::/0) with
// the IPv6 gateway in the IPv6Routes field.
//
// **Validates: Requirements 3.4, 4.4**
func TestProperty_IPv6DefaultRoutePresence(t *testing.T) {
	// Parse the expected default route destination and gateway for comparison
	_, expectedDefaultRouteNetV6, err := net.ParseCIDR(DefaultRouteDestinationIPv6)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestinationIPv6 constant: %v", err)
	}
	expectedGatewayV6 := net.ParseIP(DaemonBridgeGatewayIPv6)
	if expectedGatewayV6 == nil {
		t.Fatalf("Failed to parse DaemonBridgeGatewayIPv6 constant")
	}

	f := func(
		// Parameter to control IPv4 compatibility (IPv6 is always true for this test)
		ipv4Compatible bool,
		// Parameter to generate different network namespace paths
		nsPathSuffix uint8,
	) bool {
		// IPv6 is always compatible for this property test
		ipv6Compatible := true

		// Create IP compatibility based on generated values
		ipComp := ipcompatibility.NewIPCompatibility(ipv4Compatible, ipv6Compatible)

		// Generate a network namespace path
		netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

		// Call the function under test
		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err != nil {
			t.Logf("createDaemonBridgePluginConfig returned error: %v", err)
			return false
		}

		// Extract the BridgeConfig from the PluginConfig interface
		bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
		if !ok {
			t.Logf("Expected *ecscni.BridgeConfig, got %T", pluginConfig)
			return false
		}

		// Verify that IPv6Routes is not nil and contains at least one route
		if bridgeConfig.IPAM.IPV6Routes == nil || len(bridgeConfig.IPAM.IPV6Routes) == 0 {
			t.Logf("IPv6Routes is nil or empty for ipv4Compatible=%v, ipv6Compatible=%v",
				ipv4Compatible, ipv6Compatible)
			return false
		}

		// Check that the IPv6 default route (::/0) with the IPv6 gateway is present
		defaultRouteV6Found := false
		for _, route := range bridgeConfig.IPAM.IPV6Routes {
			if route != nil &&
				route.Dst.String() == expectedDefaultRouteNetV6.String() &&
				route.GW != nil &&
				route.GW.Equal(expectedGatewayV6) {
				defaultRouteV6Found = true
				break
			}
		}

		if !defaultRouteV6Found {
			t.Logf("IPv6 default route (%s) with gateway (%s) not found in IPv6Routes for ipv4Compatible=%v, ipv6Compatible=%v",
				DefaultRouteDestinationIPv6, DaemonBridgeGatewayIPv6, ipv4Compatible, ipv6Compatible)
			t.Logf("IPv6Routes: %v", bridgeConfig.IPAM.IPV6Routes)
			return false
		}

		return true
	}

	// Run with at least 100 iterations as specified in the design
	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// TestProperty_IPv6DefaultRoutePresence_AllIPv6CompatibleConfigurations tests the property
// exhaustively for all IP compatibility configurations where IPv6 is compatible.
//
// Feature: daemon-bridge-ipv6-support, Property 4: IPv6 Default Route Presence
// **Validates: Requirements 3.4, 4.4**
func TestProperty_IPv6DefaultRoutePresence_AllIPv6CompatibleConfigurations(t *testing.T) {
	// Parse the expected default route destination and gateway for comparison
	_, expectedDefaultRouteNetV6, err := net.ParseCIDR(DefaultRouteDestinationIPv6)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestinationIPv6 constant: %v", err)
	}
	expectedGatewayV6 := net.ParseIP(DaemonBridgeGatewayIPv6)
	if expectedGatewayV6 == nil {
		t.Fatalf("Failed to parse DaemonBridgeGatewayIPv6 constant")
	}

	// Test cases for IPv6-compatible configurations only
	testCases := []struct {
		name   string
		ipComp ipcompatibility.IPCompatibility
	}{
		{
			name:   "IPv6-only",
			ipComp: ipcompatibility.NewIPv6OnlyCompatibility(),
		},
		{
			name:   "Dual-stack",
			ipComp: ipcompatibility.NewDualStackCompatibility(),
		},
	}

	// Run property test for each configuration with multiple namespace paths
	f := func(nsPathSuffix uint8) bool {
		for _, tc := range testCases {
			netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

			pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, tc.ipComp)
			if err != nil {
				t.Logf("[%s] createDaemonBridgePluginConfig returned error: %v", tc.name, err)
				return false
			}

			bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
			if !ok {
				t.Logf("[%s] Expected *ecscni.BridgeConfig, got %T", tc.name, pluginConfig)
				return false
			}

			if bridgeConfig.IPAM.IPV6Routes == nil || len(bridgeConfig.IPAM.IPV6Routes) == 0 {
				t.Logf("[%s] IPv6Routes is nil or empty", tc.name)
				return false
			}

			defaultRouteV6Found := false
			for _, route := range bridgeConfig.IPAM.IPV6Routes {
				if route != nil &&
					route.Dst.String() == expectedDefaultRouteNetV6.String() &&
					route.GW != nil &&
					route.GW.Equal(expectedGatewayV6) {
					defaultRouteV6Found = true
					break
				}
			}

			if !defaultRouteV6Found {
				t.Logf("[%s] IPv6 default route (%s) with gateway (%s) not found in IPv6Routes",
					tc.name, DefaultRouteDestinationIPv6, DaemonBridgeGatewayIPv6)
				return false
			}
		}
		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// TestProperty_IPv6DefaultRoutePresence_IPv4OnlyNoIPv6DefaultRoute verifies that
// IPv4-only configurations do NOT include the IPv6 default route.
//
// Feature: daemon-bridge-ipv6-support, Property 4: IPv6 Default Route Presence
// **Validates: Requirements 3.4, 4.4**
func TestProperty_IPv6DefaultRoutePresence_IPv4OnlyNoIPv6DefaultRoute(t *testing.T) {
	f := func(nsPathSuffix uint8) bool {
		// Create IPv4-only compatibility
		ipComp := ipcompatibility.NewIPv4OnlyCompatibility()

		netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err != nil {
			t.Logf("createDaemonBridgePluginConfig returned error: %v", err)
			return false
		}

		bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
		if !ok {
			t.Logf("Expected *ecscni.BridgeConfig, got %T", pluginConfig)
			return false
		}

		// IPv6Routes should be nil or empty for IPv4-only configuration
		if bridgeConfig.IPAM.IPV6Routes != nil && len(bridgeConfig.IPAM.IPV6Routes) > 0 {
			t.Logf("IPv6Routes should be nil or empty for IPv4-only configuration, but got: %v",
				bridgeConfig.IPAM.IPV6Routes)
			return false
		}

		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// =============================================================================
// Property 5: Route Configuration Completeness (Dual-Stack)
// =============================================================================

// Feature: daemon-bridge-ipv6-support, Property 5: Route Configuration Completeness
//
// For any dual-stack IP compatibility configuration, the daemon bridge plugin
// configuration SHALL include both IPv4 and IPv6 default routes simultaneously.
//
// **Validates: Requirements 3.5, 4.5**
func TestProperty_DualStackRouteCompleteness(t *testing.T) {
	// Parse the expected IPv4 default route destination and gateway for comparison
	_, expectedDefaultRouteNetV4, err := net.ParseCIDR(DefaultRouteDestination)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestination constant: %v", err)
	}
	expectedGatewayV4 := net.ParseIP(DaemonBridgeGatewayIP)
	if expectedGatewayV4 == nil {
		t.Fatalf("Failed to parse DaemonBridgeGatewayIP constant")
	}

	// Parse the expected IPv6 default route destination and gateway for comparison
	_, expectedDefaultRouteNetV6, err := net.ParseCIDR(DefaultRouteDestinationIPv6)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestinationIPv6 constant: %v", err)
	}
	expectedGatewayV6 := net.ParseIP(DaemonBridgeGatewayIPv6)
	if expectedGatewayV6 == nil {
		t.Fatalf("Failed to parse DaemonBridgeGatewayIPv6 constant")
	}

	f := func(
		// Parameter to generate different network namespace paths
		nsPathSuffix uint8,
	) bool {
		// Create dual-stack IP compatibility (both IPv4 and IPv6 compatible)
		ipComp := ipcompatibility.NewDualStackCompatibility()

		// Generate a network namespace path
		netNSPath := "/var/run/netns/test-ns-" + itoa(int(nsPathSuffix))

		// Call the function under test
		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err != nil {
			t.Logf("createDaemonBridgePluginConfig returned error: %v", err)
			return false
		}

		// Extract the BridgeConfig from the PluginConfig interface
		bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
		if !ok {
			t.Logf("Expected *ecscni.BridgeConfig, got %T", pluginConfig)
			return false
		}

		// Verify that IPv4Routes is not nil and contains at least one route
		if bridgeConfig.IPAM.IPV4Routes == nil || len(bridgeConfig.IPAM.IPV4Routes) == 0 {
			t.Logf("IPv4Routes is nil or empty for dual-stack configuration")
			return false
		}

		// Verify that IPv6Routes is not nil and contains at least one route
		if bridgeConfig.IPAM.IPV6Routes == nil || len(bridgeConfig.IPAM.IPV6Routes) == 0 {
			t.Logf("IPv6Routes is nil or empty for dual-stack configuration")
			return false
		}

		// Check that the IPv4 default route (0.0.0.0/0) with gateway 169.254.172.1 is present
		ipv4DefaultRouteFound := false
		for _, route := range bridgeConfig.IPAM.IPV4Routes {
			if route != nil &&
				route.Dst.String() == expectedDefaultRouteNetV4.String() &&
				route.GW != nil &&
				route.GW.Equal(expectedGatewayV4) {
				ipv4DefaultRouteFound = true
				break
			}
		}

		if !ipv4DefaultRouteFound {
			t.Logf("IPv4 default route (%s) with gateway (%s) not found in IPv4Routes for dual-stack configuration",
				DefaultRouteDestination, DaemonBridgeGatewayIP)
			t.Logf("IPv4Routes: %v", bridgeConfig.IPAM.IPV4Routes)
			return false
		}

		// Check that the IPv6 default route (::/0) with the IPv6 gateway is present
		ipv6DefaultRouteFound := false
		for _, route := range bridgeConfig.IPAM.IPV6Routes {
			if route != nil &&
				route.Dst.String() == expectedDefaultRouteNetV6.String() &&
				route.GW != nil &&
				route.GW.Equal(expectedGatewayV6) {
				ipv6DefaultRouteFound = true
				break
			}
		}

		if !ipv6DefaultRouteFound {
			t.Logf("IPv6 default route (%s) with gateway (%s) not found in IPv6Routes for dual-stack configuration",
				DefaultRouteDestinationIPv6, DaemonBridgeGatewayIPv6)
			t.Logf("IPv6Routes: %v", bridgeConfig.IPAM.IPV6Routes)
			return false
		}

		return true
	}

	// Run with at least 100 iterations as specified in the design
	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// TestProperty_DualStackRouteCompleteness_WithVariedInputs tests the property
// with varied input parameters to ensure robustness across different scenarios.
//
// Feature: daemon-bridge-ipv6-support, Property 5: Route Configuration Completeness
// **Validates: Requirements 3.5, 4.5**
func TestProperty_DualStackRouteCompleteness_WithVariedInputs(t *testing.T) {
	// Parse the expected IPv4 default route destination and gateway for comparison
	_, expectedDefaultRouteNetV4, err := net.ParseCIDR(DefaultRouteDestination)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestination constant: %v", err)
	}
	expectedGatewayV4 := net.ParseIP(DaemonBridgeGatewayIP)
	if expectedGatewayV4 == nil {
		t.Fatalf("Failed to parse DaemonBridgeGatewayIP constant")
	}

	// Parse the expected IPv6 default route destination and gateway for comparison
	_, expectedDefaultRouteNetV6, err := net.ParseCIDR(DefaultRouteDestinationIPv6)
	if err != nil {
		t.Fatalf("Failed to parse DefaultRouteDestinationIPv6 constant: %v", err)
	}
	expectedGatewayV6 := net.ParseIP(DaemonBridgeGatewayIPv6)
	if expectedGatewayV6 == nil {
		t.Fatalf("Failed to parse DaemonBridgeGatewayIPv6 constant")
	}

	// Test with various namespace path patterns
	nsPathPatterns := []string{
		"/var/run/netns/test-ns-",
		"/var/run/netns/daemon-",
		"/var/run/netns/container-",
		"/proc/self/ns/net-",
	}

	f := func(
		// Parameter to select namespace path pattern
		patternIndex uint8,
		// Parameter to generate different namespace path suffixes
		nsPathSuffix uint16,
	) bool {
		// Create dual-stack IP compatibility using the explicit constructor
		ipComp := ipcompatibility.NewIPCompatibility(true, true)

		// Select a namespace path pattern
		pattern := nsPathPatterns[int(patternIndex)%len(nsPathPatterns)]
		netNSPath := pattern + itoa(int(nsPathSuffix))

		// Call the function under test
		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err != nil {
			t.Logf("createDaemonBridgePluginConfig returned error: %v", err)
			return false
		}

		// Extract the BridgeConfig from the PluginConfig interface
		bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
		if !ok {
			t.Logf("Expected *ecscni.BridgeConfig, got %T", pluginConfig)
			return false
		}

		// Verify both IPv4Routes and IPv6Routes are populated
		if bridgeConfig.IPAM.IPV4Routes == nil || len(bridgeConfig.IPAM.IPV4Routes) == 0 {
			t.Logf("IPv4Routes is nil or empty for dual-stack configuration with path %s", netNSPath)
			return false
		}

		if bridgeConfig.IPAM.IPV6Routes == nil || len(bridgeConfig.IPAM.IPV6Routes) == 0 {
			t.Logf("IPv6Routes is nil or empty for dual-stack configuration with path %s", netNSPath)
			return false
		}

		// Check that the IPv4 default route is present
		ipv4DefaultRouteFound := false
		for _, route := range bridgeConfig.IPAM.IPV4Routes {
			if route != nil &&
				route.Dst.String() == expectedDefaultRouteNetV4.String() &&
				route.GW != nil &&
				route.GW.Equal(expectedGatewayV4) {
				ipv4DefaultRouteFound = true
				break
			}
		}

		if !ipv4DefaultRouteFound {
			t.Logf("IPv4 default route not found for dual-stack configuration with path %s", netNSPath)
			return false
		}

		// Check that the IPv6 default route is present
		ipv6DefaultRouteFound := false
		for _, route := range bridgeConfig.IPAM.IPV6Routes {
			if route != nil &&
				route.Dst.String() == expectedDefaultRouteNetV6.String() &&
				route.GW != nil &&
				route.GW.Equal(expectedGatewayV6) {
				ipv6DefaultRouteFound = true
				break
			}
		}

		if !ipv6DefaultRouteFound {
			t.Logf("IPv6 default route not found for dual-stack configuration with path %s", netNSPath)
			return false
		}

		return true
	}

	// Run with at least 100 iterations as specified in the design
	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}

// TestProperty_DualStackRouteCompleteness_BothRoutesSimultaneous verifies that
// both IPv4 and IPv6 default routes are present simultaneously in the same
// configuration object for dual-stack compatibility.
//
// Feature: daemon-bridge-ipv6-support, Property 5: Route Configuration Completeness
// **Validates: Requirements 3.5, 4.5**
func TestProperty_DualStackRouteCompleteness_BothRoutesSimultaneous(t *testing.T) {
	// Parse the expected route destinations and gateways
	_, expectedDefaultRouteNetV4, _ := net.ParseCIDR(DefaultRouteDestination)
	expectedGatewayV4 := net.ParseIP(DaemonBridgeGatewayIP)
	_, expectedDefaultRouteNetV6, _ := net.ParseCIDR(DefaultRouteDestinationIPv6)
	expectedGatewayV6 := net.ParseIP(DaemonBridgeGatewayIPv6)

	f := func(nsPathSuffix uint8) bool {
		// Create dual-stack IP compatibility
		ipComp := ipcompatibility.NewDualStackCompatibility()

		netNSPath := "/var/run/netns/dual-stack-test-" + itoa(int(nsPathSuffix))

		pluginConfig, err := createDaemonBridgePluginConfig(netNSPath, ipComp)
		if err != nil {
			t.Logf("createDaemonBridgePluginConfig returned error: %v", err)
			return false
		}

		bridgeConfig, ok := pluginConfig.(*ecscni.BridgeConfig)
		if !ok {
			t.Logf("Expected *ecscni.BridgeConfig, got %T", pluginConfig)
			return false
		}

		// Both routes must be present simultaneously in the same configuration
		ipv4Found := false
		ipv6Found := false

		for _, route := range bridgeConfig.IPAM.IPV4Routes {
			if route != nil &&
				route.Dst.String() == expectedDefaultRouteNetV4.String() &&
				route.GW != nil &&
				route.GW.Equal(expectedGatewayV4) {
				ipv4Found = true
				break
			}
		}

		for _, route := range bridgeConfig.IPAM.IPV6Routes {
			if route != nil &&
				route.Dst.String() == expectedDefaultRouteNetV6.String() &&
				route.GW != nil &&
				route.GW.Equal(expectedGatewayV6) {
				ipv6Found = true
				break
			}
		}

		// Property: Both routes must be present simultaneously
		if !ipv4Found || !ipv6Found {
			t.Logf("Dual-stack completeness violated: IPv4 default route found=%v, IPv6 default route found=%v",
				ipv4Found, ipv6Found)
			return false
		}

		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, config); err != nil {
		t.Errorf("Property test failed: %v", err)
	}
}
