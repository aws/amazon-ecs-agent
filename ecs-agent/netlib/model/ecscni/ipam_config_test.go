//go:build unit
// +build unit

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

package ecscni

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Unit Tests for IPAMConfig IPv6 Fields
// =============================================================================
// These tests validate Requirements 1.1, 1.2, 1.3, 1.4, 1.5

// TestIPAMConfig_JSONMarshal_WithIPv6Fields tests that IPAMConfig with IPv6 fields
// serializes correctly to JSON with the expected field names.
// Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5
func TestIPAMConfig_JSONMarshal_WithIPv6Fields(t *testing.T) {
	// Create IPv6 route
	_, dstNet, err := net.ParseCIDR("2001:db8::/32")
	require.NoError(t, err)
	ipv6Route := &types.Route{
		Dst: *dstNet,
		GW:  net.ParseIP("2001:db8::1"),
	}

	config := &IPAMConfig{
		CNIConfig: CNIConfig{
			NetNSPath:      "/var/run/netns/test",
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		// IPv4 fields
		IPV4Subnet:  "169.254.172.0/22",
		IPV4Address: "169.254.172.10",
		IPV4Gateway: "169.254.172.1",
		// IPv6 fields
		IPV6Subnet:  "2001:db8::/64",
		IPV6Address: "2001:db8::10",
		IPV6Gateway: "2001:db8::1",
		IPV6Routes:  []*types.Route{ipv6Route},
		ID:          "test-id",
	}

	jsonData, err := json.Marshal(config)
	require.NoError(t, err)

	// Verify JSON contains expected IPv6 field names
	jsonStr := string(jsonData)
	assert.Contains(t, jsonStr, `"ipv6-subnet"`)
	assert.Contains(t, jsonStr, `"ipv6-address"`)
	assert.Contains(t, jsonStr, `"ipv6-gateway"`)
	assert.Contains(t, jsonStr, `"ipv6-routes"`)

	// Verify IPv6 values are present
	assert.Contains(t, jsonStr, "2001:db8::/64")
	assert.Contains(t, jsonStr, "2001:db8::10")
	assert.Contains(t, jsonStr, "2001:db8::1")
}

// TestIPAMConfig_JSONUnmarshal_WithIPv6Data tests that JSON with IPv6 data
// deserializes correctly into IPAMConfig struct.
// Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5
func TestIPAMConfig_JSONUnmarshal_WithIPv6Data(t *testing.T) {
	jsonData := `{
		"cniVersion": "1.0.0",
		"name": "ecs-ipam",
		"ipv4-subnet": "169.254.172.0/22",
		"ipv4-address": "169.254.172.10",
		"ipv4-gateway": "169.254.172.1",
		"ipv6-subnet": "2001:db8::/64",
		"ipv6-address": "2001:db8::10",
		"ipv6-gateway": "2001:db8::1",
		"ipv6-routes": [
			{
				"dst": "2001:db8:1::/48",
				"gw": "2001:db8::1"
			}
		],
		"id": "test-id"
	}`

	var config IPAMConfig
	err := json.Unmarshal([]byte(jsonData), &config)
	require.NoError(t, err)

	// Verify IPv6 fields are correctly populated
	assert.Equal(t, "2001:db8::/64", config.IPV6Subnet)
	assert.Equal(t, "2001:db8::10", config.IPV6Address)
	assert.Equal(t, "2001:db8::1", config.IPV6Gateway)

	// Verify IPv6 routes
	require.Len(t, config.IPV6Routes, 1)
	assert.Equal(t, "2001:db8:1::/48", config.IPV6Routes[0].Dst.String())
	assert.Equal(t, "2001:db8::1", config.IPV6Routes[0].GW.String())

	// Verify IPv4 fields are also correctly populated
	assert.Equal(t, "169.254.172.0/22", config.IPV4Subnet)
	assert.Equal(t, "169.254.172.10", config.IPV4Address)
	assert.Equal(t, "169.254.172.1", config.IPV4Gateway)
	assert.Equal(t, "test-id", config.ID)
}

// TestIPAMConfig_JSONMarshalUnmarshal_RoundTrip tests that IPAMConfig with IPv6 fields
// can be serialized and deserialized without data loss.
// Validates: Requirements 1.5
func TestIPAMConfig_JSONMarshalUnmarshal_RoundTrip(t *testing.T) {
	// Create IPv6 routes
	_, dstNet1, _ := net.ParseCIDR("2001:db8:1::/48")
	_, dstNet2, _ := net.ParseCIDR("::/0")
	ipv6Routes := []*types.Route{
		{
			Dst: *dstNet1,
			GW:  net.ParseIP("2001:db8::1"),
		},
		{
			Dst: *dstNet2,
			GW:  net.ParseIP("fe80::1"),
		},
	}

	// Create IPv4 routes
	_, ipv4DstNet, _ := net.ParseCIDR("0.0.0.0/0")
	ipv4Routes := []*types.Route{
		{
			Dst: *ipv4DstNet,
			GW:  net.ParseIP("169.254.172.1"),
		},
	}

	original := &IPAMConfig{
		CNIConfig: CNIConfig{
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		// IPv4 fields
		IPV4Subnet:  "169.254.172.0/22",
		IPV4Address: "169.254.172.10",
		IPV4Gateway: "169.254.172.1",
		IPV4Routes:  ipv4Routes,
		// IPv6 fields
		IPV6Subnet:  "2001:db8::/64",
		IPV6Address: "2001:db8::10",
		IPV6Gateway: "2001:db8::1",
		IPV6Routes:  ipv6Routes,
		ID:          "test-container-id",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var deserialized IPAMConfig
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.IPV4Subnet, deserialized.IPV4Subnet)
	assert.Equal(t, original.IPV4Address, deserialized.IPV4Address)
	assert.Equal(t, original.IPV4Gateway, deserialized.IPV4Gateway)
	assert.Equal(t, original.IPV6Subnet, deserialized.IPV6Subnet)
	assert.Equal(t, original.IPV6Address, deserialized.IPV6Address)
	assert.Equal(t, original.IPV6Gateway, deserialized.IPV6Gateway)
	assert.Equal(t, original.ID, deserialized.ID)

	// Verify IPv4 routes
	require.Len(t, deserialized.IPV4Routes, len(original.IPV4Routes))
	for i, route := range original.IPV4Routes {
		assert.Equal(t, route.Dst.String(), deserialized.IPV4Routes[i].Dst.String())
		assert.True(t, route.GW.Equal(deserialized.IPV4Routes[i].GW))
	}

	// Verify IPv6 routes
	require.Len(t, deserialized.IPV6Routes, len(original.IPV6Routes))
	for i, route := range original.IPV6Routes {
		assert.Equal(t, route.Dst.String(), deserialized.IPV6Routes[i].Dst.String())
		assert.True(t, route.GW.Equal(deserialized.IPV6Routes[i].GW))
	}
}

// TestIPAMConfig_String_IncludesIPv6Information tests that the String() method
// includes IPv6 field information in its output.
// Validates: Requirements 1.1, 1.2, 1.3, 1.4
func TestIPAMConfig_String_IncludesIPv6Information(t *testing.T) {
	// Create IPv6 route
	_, dstNet, _ := net.ParseCIDR("2001:db8::/32")
	ipv6Route := &types.Route{
		Dst: *dstNet,
		GW:  net.ParseIP("2001:db8::1"),
	}

	config := &IPAMConfig{
		CNIConfig: CNIConfig{
			NetNSPath:      "/var/run/netns/test",
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		IPV6Subnet:  "2001:db8::/64",
		IPV6Address: "2001:db8::10",
		IPV6Gateway: "2001:db8::1",
		IPV6Routes:  []*types.Route{ipv6Route},
		ID:          "test-id",
	}

	str := config.String()

	// Verify String() output contains IPv6 field labels
	assert.Contains(t, str, "ipv6-subnet:")
	assert.Contains(t, str, "ipv6-address:")
	assert.Contains(t, str, "ipv6-gateway:")
	assert.Contains(t, str, "ipv6-routes:")

	// Verify String() output contains IPv6 values
	assert.Contains(t, str, "2001:db8::/64")
	assert.Contains(t, str, "2001:db8::10")
	assert.Contains(t, str, "2001:db8::1")
}

// TestIPAMConfig_String_WithEmptyIPv6Fields tests that the String() method
// handles empty IPv6 fields gracefully.
// Validates: Requirements 1.1, 1.2, 1.3, 1.4
func TestIPAMConfig_String_WithEmptyIPv6Fields(t *testing.T) {
	config := &IPAMConfig{
		CNIConfig: CNIConfig{
			NetNSPath:      "/var/run/netns/test",
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		// Only IPv4 fields populated
		IPV4Subnet:  "169.254.172.0/22",
		IPV4Address: "169.254.172.10",
		IPV4Gateway: "169.254.172.1",
		ID:          "test-id",
	}

	str := config.String()

	// Verify String() output still contains IPv6 field labels (even if empty)
	assert.Contains(t, str, "ipv6-subnet:")
	assert.Contains(t, str, "ipv6-address:")
	assert.Contains(t, str, "ipv6-gateway:")
	assert.Contains(t, str, "ipv6-routes:")

	// Verify IPv4 values are present
	assert.Contains(t, str, "169.254.172.0/22")
	assert.Contains(t, str, "169.254.172.10")
	assert.Contains(t, str, "169.254.172.1")
}

// TestIPAMConfig_JSONMarshal_OmitsEmptyIPv6Fields tests that empty IPv6 fields
// are omitted from JSON output due to omitempty tag.
// Validates: Requirements 1.5
func TestIPAMConfig_JSONMarshal_OmitsEmptyIPv6Fields(t *testing.T) {
	config := &IPAMConfig{
		CNIConfig: CNIConfig{
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		// Only IPv4 fields populated
		IPV4Subnet:  "169.254.172.0/22",
		IPV4Address: "169.254.172.10",
		IPV4Gateway: "169.254.172.1",
		ID:          "test-id",
	}

	jsonData, err := json.Marshal(config)
	require.NoError(t, err)

	jsonStr := string(jsonData)

	// Verify empty IPv6 fields are omitted (due to omitempty)
	assert.NotContains(t, jsonStr, `"ipv6-subnet"`)
	assert.NotContains(t, jsonStr, `"ipv6-address"`)
	assert.NotContains(t, jsonStr, `"ipv6-gateway"`)
	assert.NotContains(t, jsonStr, `"ipv6-routes"`)

	// Verify IPv4 fields are present
	assert.Contains(t, jsonStr, `"ipv4-subnet"`)
	assert.Contains(t, jsonStr, `"ipv4-address"`)
	assert.Contains(t, jsonStr, `"ipv4-gateway"`)
}

// TestIPAMConfig_JSONMarshal_IPv6OnlyConfiguration tests JSON marshaling
// with only IPv6 fields populated (IPv6-only scenario).
// Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5
func TestIPAMConfig_JSONMarshal_IPv6OnlyConfiguration(t *testing.T) {
	// Create IPv6 default route
	_, defaultNet, _ := net.ParseCIDR("::/0")
	ipv6Route := &types.Route{
		Dst: *defaultNet,
		GW:  net.ParseIP("fe80::1"),
	}

	config := &IPAMConfig{
		CNIConfig: CNIConfig{
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		// Only IPv6 fields populated
		IPV6Subnet:  "2001:db8::/64",
		IPV6Address: "2001:db8::10",
		IPV6Gateway: "fe80::1",
		IPV6Routes:  []*types.Route{ipv6Route},
		ID:          "ipv6-only-test",
	}

	jsonData, err := json.Marshal(config)
	require.NoError(t, err)

	jsonStr := string(jsonData)

	// Verify IPv6 fields are present
	assert.Contains(t, jsonStr, `"ipv6-subnet":"2001:db8::/64"`)
	assert.Contains(t, jsonStr, `"ipv6-address":"2001:db8::10"`)
	assert.Contains(t, jsonStr, `"ipv6-gateway":"fe80::1"`)
	assert.Contains(t, jsonStr, `"ipv6-routes"`)

	// Verify empty IPv4 fields are omitted
	assert.NotContains(t, jsonStr, `"ipv4-subnet"`)
	assert.NotContains(t, jsonStr, `"ipv4-address"`)
	assert.NotContains(t, jsonStr, `"ipv4-gateway"`)
}

// TestIPAMConfig_JSONUnmarshal_MultipleIPv6Routes tests unmarshaling
// with multiple IPv6 routes.
// Validates: Requirements 1.4, 1.5
func TestIPAMConfig_JSONUnmarshal_MultipleIPv6Routes(t *testing.T) {
	jsonData := `{
		"cniVersion": "1.0.0",
		"name": "ecs-ipam",
		"ipv6-subnet": "2001:db8::/64",
		"ipv6-address": "2001:db8::10",
		"ipv6-gateway": "2001:db8::1",
		"ipv6-routes": [
			{
				"dst": "::/0",
				"gw": "fe80::1"
			},
			{
				"dst": "2001:db8:1::/48",
				"gw": "2001:db8::1"
			},
			{
				"dst": "2001:db8:2::/48"
			}
		],
		"id": "multi-route-test"
	}`

	var config IPAMConfig
	err := json.Unmarshal([]byte(jsonData), &config)
	require.NoError(t, err)

	// Verify all routes are parsed
	require.Len(t, config.IPV6Routes, 3)

	// Verify first route (default route)
	assert.Equal(t, "::/0", config.IPV6Routes[0].Dst.String())
	assert.Equal(t, "fe80::1", config.IPV6Routes[0].GW.String())

	// Verify second route
	assert.Equal(t, "2001:db8:1::/48", config.IPV6Routes[1].Dst.String())
	assert.Equal(t, "2001:db8::1", config.IPV6Routes[1].GW.String())

	// Verify third route (no gateway)
	assert.Equal(t, "2001:db8:2::/48", config.IPV6Routes[2].Dst.String())
	assert.Nil(t, config.IPV6Routes[2].GW)
}

// TestIPAMConfig_InterfaceMethods tests that IPAMConfig implements
// the PluginConfig interface methods correctly.
func TestIPAMConfig_InterfaceMethods(t *testing.T) {
	config := &IPAMConfig{
		CNIConfig: CNIConfig{
			NetNSPath:      "/var/run/netns/test",
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		IPV6Subnet: "2001:db8::/64",
		ID:         "test-id",
	}

	// Test InterfaceName() returns "none" for IPAM config
	assert.Equal(t, "none", config.InterfaceName())

	// Test NSPath() returns the network namespace path
	assert.Equal(t, "/var/run/netns/test", config.NSPath())

	// Test CNIVersion() returns the CNI spec version
	assert.Equal(t, "1.0.0", config.CNIVersion())

	// Test PluginName() returns the plugin name
	assert.Equal(t, "ecs-ipam", config.PluginName())
}

// TestIPAMConfig_DualStack_JSONRoundTrip tests JSON round-trip for
// a complete dual-stack configuration.
// Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5
func TestIPAMConfig_DualStack_JSONRoundTrip(t *testing.T) {
	// Create IPv4 routes
	_, ipv4DefaultNet, _ := net.ParseCIDR("0.0.0.0/0")
	_, ipv4AgentNet, _ := net.ParseCIDR("169.254.170.2/32")
	ipv4Routes := []*types.Route{
		{Dst: *ipv4AgentNet},
		{Dst: *ipv4DefaultNet, GW: net.ParseIP("169.254.172.1")},
	}

	// Create IPv6 routes
	_, ipv6DefaultNet, _ := net.ParseCIDR("::/0")
	ipv6Routes := []*types.Route{
		{Dst: *ipv6DefaultNet, GW: net.ParseIP("fe80::1")},
	}

	original := &IPAMConfig{
		CNIConfig: CNIConfig{
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		// IPv4 fields
		IPV4Subnet:  "169.254.172.0/22",
		IPV4Address: "169.254.172.10",
		IPV4Gateway: "169.254.172.1",
		IPV4Routes:  ipv4Routes,
		// IPv6 fields
		IPV6Subnet:  "2001:db8::/64",
		IPV6Address: "2001:db8::10",
		IPV6Gateway: "fe80::1",
		IPV6Routes:  ipv6Routes,
		ID:          "dual-stack-test",
	}

	// Marshal
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	var deserialized IPAMConfig
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, original.IPV4Subnet, deserialized.IPV4Subnet)
	assert.Equal(t, original.IPV4Address, deserialized.IPV4Address)
	assert.Equal(t, original.IPV4Gateway, deserialized.IPV4Gateway)
	assert.Equal(t, original.IPV6Subnet, deserialized.IPV6Subnet)
	assert.Equal(t, original.IPV6Address, deserialized.IPV6Address)
	assert.Equal(t, original.IPV6Gateway, deserialized.IPV6Gateway)
	assert.Equal(t, original.ID, deserialized.ID)

	// Verify route counts
	assert.Len(t, deserialized.IPV4Routes, 2)
	assert.Len(t, deserialized.IPV6Routes, 1)
}

// TestIPAMConfig_String_DualStack tests String() output for dual-stack config.
// Validates: Requirements 1.1, 1.2, 1.3, 1.4
func TestIPAMConfig_String_DualStack(t *testing.T) {
	config := &IPAMConfig{
		CNIConfig: CNIConfig{
			NetNSPath:      "/var/run/netns/test",
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		// IPv4 fields
		IPV4Subnet:  "169.254.172.0/22",
		IPV4Address: "169.254.172.10",
		IPV4Gateway: "169.254.172.1",
		// IPv6 fields
		IPV6Subnet:  "2001:db8::/64",
		IPV6Address: "2001:db8::10",
		IPV6Gateway: "fe80::1",
		ID:          "dual-stack-test",
	}

	str := config.String()

	// Verify both IPv4 and IPv6 information is present
	assert.Contains(t, str, "ipv4-subnet: 169.254.172.0/22")
	assert.Contains(t, str, "ipv4-address: 169.254.172.10")
	assert.Contains(t, str, "ipv4-gateway: 169.254.172.1")
	assert.Contains(t, str, "ipv6-subnet: 2001:db8::/64")
	assert.Contains(t, str, "ipv6-address: 2001:db8::10")
	assert.Contains(t, str, "ipv6-gateway: fe80::1")
}

// TestIPAMConfig_LinkLocalIPv6Gateway tests configuration with link-local IPv6 gateway.
// This is the expected configuration for daemon bridge IPv6 support.
// Validates: Requirements 1.3, 1.4
func TestIPAMConfig_LinkLocalIPv6Gateway(t *testing.T) {
	// Create IPv6 default route with link-local gateway (fe80::1)
	_, defaultNet, _ := net.ParseCIDR("::/0")
	ipv6Route := &types.Route{
		Dst: *defaultNet,
		GW:  net.ParseIP("fe80::1"),
	}

	config := &IPAMConfig{
		CNIConfig: CNIConfig{
			CNISpecVersion: "1.0.0",
			CNIPluginName:  "ecs-ipam",
		},
		IPV6Gateway: "fe80::1",
		IPV6Routes:  []*types.Route{ipv6Route},
		ID:          "link-local-gw-test",
	}

	// Marshal and unmarshal
	jsonData, err := json.Marshal(config)
	require.NoError(t, err)

	var deserialized IPAMConfig
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err)

	// Verify link-local gateway is preserved
	assert.Equal(t, "fe80::1", deserialized.IPV6Gateway)
	require.Len(t, deserialized.IPV6Routes, 1)
	assert.True(t, strings.HasPrefix(deserialized.IPV6Routes[0].GW.String(), "fe80::"))
}
