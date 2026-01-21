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
	"reflect"
	"testing"
	"testing/quick"

	"github.com/containernetworking/cni/pkg/types"
)

// =============================================================================
// Property-Based Tests for IPAMConfig IPv6 JSON Serialization
// =============================================================================

// Feature: daemon-bridge-ipv6-support, Property 1: IPAMConfig IPv6 JSON Serialization Round-Trip
//
// For any valid IPAMConfig with IPv6 fields populated (IPV6Subnet, IPV6Address, IPV6Gateway, IPV6Routes),
// serializing to JSON and deserializing back SHALL produce an equivalent IPAMConfig object.
//
// **Validates: Requirements 1.5**
func TestProperty_IPAMConfigIPv6JSONSerializationRoundTrip(t *testing.T) {
	f := func(
		// IPv6 subnet parameters (using uint16 for address components)
		subnetPrefix1, subnetPrefix2, subnetPrefix3, subnetPrefix4 uint16,
		subnetPrefixLen uint8,
		// IPv6 address parameters
		addrPrefix1, addrPrefix2, addrPrefix3, addrPrefix4 uint16,
		addrSuffix1, addrSuffix2, addrSuffix3, addrSuffix4 uint16,
		// IPv6 gateway parameters
		gwPrefix1, gwPrefix2, gwPrefix3, gwPrefix4 uint16,
		gwSuffix1, gwSuffix2, gwSuffix3, gwSuffix4 uint16,
		// Route parameters
		numRoutes uint8,
		routeDstPrefix1, routeDstPrefix2 uint16,
		routeGwSuffix uint16,
		// Other fields
		netNSPath string,
		id string,
	) bool {
		// Constrain prefix length to valid IPv6 range (0-128)
		subnetPrefixLen = subnetPrefixLen % 129

		// Constrain number of routes to a reasonable number (0-5)
		numRoutes = numRoutes % 6

		// Build IPv6 subnet
		ipv6Subnet := buildIPv6Address(subnetPrefix1, subnetPrefix2, subnetPrefix3, subnetPrefix4, 0, 0, 0, 0)
		ipv6SubnetStr := ""
		if ipv6Subnet != nil {
			ipv6SubnetStr = ipv6Subnet.String() + "/" + itoa(int(subnetPrefixLen))
		}

		// Build IPv6 address
		ipv6Addr := buildIPv6Address(addrPrefix1, addrPrefix2, addrPrefix3, addrPrefix4,
			addrSuffix1, addrSuffix2, addrSuffix3, addrSuffix4)
		ipv6AddrStr := ""
		if ipv6Addr != nil {
			ipv6AddrStr = ipv6Addr.String()
		}

		// Build IPv6 gateway
		ipv6Gw := buildIPv6Address(gwPrefix1, gwPrefix2, gwPrefix3, gwPrefix4,
			gwSuffix1, gwSuffix2, gwSuffix3, gwSuffix4)
		ipv6GwStr := ""
		if ipv6Gw != nil {
			ipv6GwStr = ipv6Gw.String()
		}

		// Build IPv6 routes
		var ipv6Routes []*types.Route
		for i := uint8(0); i < numRoutes; i++ {
			// Create a route destination with varying prefix
			routeDst := buildIPv6Address(routeDstPrefix1, routeDstPrefix2, uint16(i), 0, 0, 0, 0, 0)
			routeGw := buildIPv6Address(gwPrefix1, gwPrefix2, gwPrefix3, gwPrefix4, 0, 0, 0, routeGwSuffix+uint16(i))

			if routeDst != nil {
				_, dstNet, err := net.ParseCIDR(routeDst.String() + "/64")
				if err == nil && dstNet != nil {
					route := &types.Route{
						Dst: *dstNet,
					}
					if routeGw != nil {
						route.GW = routeGw
					}
					ipv6Routes = append(ipv6Routes, route)
				}
			}
		}

		// Create the original IPAMConfig with IPv6 fields
		original := &IPAMConfig{
			CNIConfig: CNIConfig{
				NetNSPath:      netNSPath,
				CNISpecVersion: "1.0.0",
				CNIPluginName:  "ecs-ipam",
			},
			IPV6Subnet:  ipv6SubnetStr,
			IPV6Address: ipv6AddrStr,
			IPV6Gateway: ipv6GwStr,
			IPV6Routes:  ipv6Routes,
			ID:          id,
		}

		// Serialize to JSON
		jsonData, err := json.Marshal(original)
		if err != nil {
			// If marshaling fails, this is a valid failure case
			// (e.g., invalid data that can't be serialized)
			return true
		}

		// Deserialize back to IPAMConfig
		var deserialized IPAMConfig
		err = json.Unmarshal(jsonData, &deserialized)
		if err != nil {
			// If unmarshaling fails after successful marshaling, this is a bug
			t.Logf("Failed to unmarshal JSON: %v", err)
			t.Logf("JSON data: %s", string(jsonData))
			return false
		}

		// Verify equivalence of IPv6 fields
		if original.IPV6Subnet != deserialized.IPV6Subnet {
			t.Logf("IPV6Subnet mismatch: original=%q, deserialized=%q",
				original.IPV6Subnet, deserialized.IPV6Subnet)
			return false
		}

		if original.IPV6Address != deserialized.IPV6Address {
			t.Logf("IPV6Address mismatch: original=%q, deserialized=%q",
				original.IPV6Address, deserialized.IPV6Address)
			return false
		}

		if original.IPV6Gateway != deserialized.IPV6Gateway {
			t.Logf("IPV6Gateway mismatch: original=%q, deserialized=%q",
				original.IPV6Gateway, deserialized.IPV6Gateway)
			return false
		}

		// Verify routes
		if !routesEqual(original.IPV6Routes, deserialized.IPV6Routes) {
			t.Logf("IPV6Routes mismatch: original=%v, deserialized=%v",
				original.IPV6Routes, deserialized.IPV6Routes)
			return false
		}

		// Verify other fields are preserved
		if original.ID != deserialized.ID {
			t.Logf("ID mismatch: original=%q, deserialized=%q",
				original.ID, deserialized.ID)
			return false
		}

		// Verify CNIConfig fields (note: NetNSPath has json:"-" tag, so it won't be serialized)
		if original.CNISpecVersion != deserialized.CNISpecVersion {
			t.Logf("CNISpecVersion mismatch: original=%q, deserialized=%q",
				original.CNISpecVersion, deserialized.CNISpecVersion)
			return false
		}

		if original.CNIPluginName != deserialized.CNIPluginName {
			t.Logf("CNIPluginName mismatch: original=%q, deserialized=%q",
				original.CNIPluginName, deserialized.CNIPluginName)
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

// TestProperty_IPAMConfigIPv6JSONSerializationRoundTrip_WithIPv4 tests that IPv6 fields
// serialize correctly alongside IPv4 fields (dual-stack scenario).
//
// Feature: daemon-bridge-ipv6-support, Property 1: IPAMConfig IPv6 JSON Serialization Round-Trip
// **Validates: Requirements 1.5**
func TestProperty_IPAMConfigIPv6JSONSerializationRoundTrip_WithIPv4(t *testing.T) {
	f := func(
		// IPv4 parameters
		ipv4Octet1, ipv4Octet2, ipv4Octet3, ipv4Octet4 uint8,
		ipv4PrefixLen uint8,
		// IPv6 parameters
		ipv6Word1, ipv6Word2, ipv6Word3, ipv6Word4 uint16,
		ipv6PrefixLen uint8,
		// ID
		id string,
	) bool {
		// Constrain prefix lengths
		ipv4PrefixLen = ipv4PrefixLen % 33  // 0-32 for IPv4
		ipv6PrefixLen = ipv6PrefixLen % 129 // 0-128 for IPv6

		// Build IPv4 fields
		ipv4Subnet := net.IPv4(ipv4Octet1, ipv4Octet2, ipv4Octet3, 0).String() + "/" + itoa(int(ipv4PrefixLen))
		ipv4Address := net.IPv4(ipv4Octet1, ipv4Octet2, ipv4Octet3, ipv4Octet4).String()
		ipv4Gateway := net.IPv4(ipv4Octet1, ipv4Octet2, ipv4Octet3, 1).String()

		// Build IPv6 fields
		ipv6Addr := buildIPv6Address(ipv6Word1, ipv6Word2, ipv6Word3, ipv6Word4, 0, 0, 0, 2)
		ipv6Subnet := ""
		ipv6Address := ""
		ipv6Gateway := ""
		if ipv6Addr != nil {
			ipv6Net := buildIPv6Address(ipv6Word1, ipv6Word2, ipv6Word3, ipv6Word4, 0, 0, 0, 0)
			if ipv6Net != nil {
				ipv6Subnet = ipv6Net.String() + "/" + itoa(int(ipv6PrefixLen))
			}
			ipv6Address = ipv6Addr.String()
			ipv6GwAddr := buildIPv6Address(ipv6Word1, ipv6Word2, ipv6Word3, ipv6Word4, 0, 0, 0, 1)
			if ipv6GwAddr != nil {
				ipv6Gateway = ipv6GwAddr.String()
			}
		}

		// Create dual-stack IPAMConfig
		original := &IPAMConfig{
			CNIConfig: CNIConfig{
				CNISpecVersion: "1.0.0",
				CNIPluginName:  "ecs-ipam",
			},
			// IPv4 fields
			IPV4Subnet:  ipv4Subnet,
			IPV4Address: ipv4Address,
			IPV4Gateway: ipv4Gateway,
			// IPv6 fields
			IPV6Subnet:  ipv6Subnet,
			IPV6Address: ipv6Address,
			IPV6Gateway: ipv6Gateway,
			ID:          id,
		}

		// Serialize to JSON
		jsonData, err := json.Marshal(original)
		if err != nil {
			return true // Skip invalid inputs
		}

		// Deserialize back
		var deserialized IPAMConfig
		err = json.Unmarshal(jsonData, &deserialized)
		if err != nil {
			t.Logf("Failed to unmarshal JSON: %v", err)
			return false
		}

		// Verify all fields match
		if original.IPV4Subnet != deserialized.IPV4Subnet ||
			original.IPV4Address != deserialized.IPV4Address ||
			original.IPV4Gateway != deserialized.IPV4Gateway ||
			original.IPV6Subnet != deserialized.IPV6Subnet ||
			original.IPV6Address != deserialized.IPV6Address ||
			original.IPV6Gateway != deserialized.IPV6Gateway ||
			original.ID != deserialized.ID {
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

// buildIPv6Address constructs an IPv6 address from 8 uint16 words
func buildIPv6Address(w1, w2, w3, w4, w5, w6, w7, w8 uint16) net.IP {
	ip := make(net.IP, 16)
	ip[0] = byte(w1 >> 8)
	ip[1] = byte(w1 & 0xff)
	ip[2] = byte(w2 >> 8)
	ip[3] = byte(w2 & 0xff)
	ip[4] = byte(w3 >> 8)
	ip[5] = byte(w3 & 0xff)
	ip[6] = byte(w4 >> 8)
	ip[7] = byte(w4 & 0xff)
	ip[8] = byte(w5 >> 8)
	ip[9] = byte(w5 & 0xff)
	ip[10] = byte(w6 >> 8)
	ip[11] = byte(w6 & 0xff)
	ip[12] = byte(w7 >> 8)
	ip[13] = byte(w7 & 0xff)
	ip[14] = byte(w8 >> 8)
	ip[15] = byte(w8 & 0xff)
	return ip
}

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

// routesEqual compares two slices of routes for equality
func routesEqual(a, b []*types.Route) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil && b[i] == nil {
			continue
		}
		if a[i] == nil || b[i] == nil {
			return false
		}
		// Compare Dst
		if !reflect.DeepEqual(a[i].Dst, b[i].Dst) {
			return false
		}
		// Compare GW
		if !a[i].GW.Equal(b[i].GW) {
			// Handle nil case
			if a[i].GW == nil && b[i].GW == nil {
				continue
			}
			return false
		}
	}
	return true
}
