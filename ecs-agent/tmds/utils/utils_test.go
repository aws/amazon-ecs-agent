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
package utils

import "testing"

func TestIsIPv4(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"Valid IPv4", "192.168.1.1", true},
		{"Valid IPv4 all zeros", "0.0.0.0", true},
		{"Valid IPv4 max values", "255.255.255.255", true},
		{"Invalid IPv4 with leading zeros", "192.168.001.001", false},
		{"Invalid IPv4 out of range", "256.256.256.256", false},
		{"Invalid IPv4 format", "192.168.1", false},
		{"IPv6 address", "2001:db8::1", false},
		{"Empty string", "", false},
		{"Invalid characters", "192.168.1.1a", false},
		{"Too many segments", "192.168.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIPv4(tt.ip)
			if result != tt.expected {
				t.Errorf("IsIPv4(%s) = %v; want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIsIPv4CIDR(t *testing.T) {
	tests := []struct {
		name     string
		cidr     string
		expected bool
	}{
		// Valid IPv4 CIDR cases
		{"Valid IPv4 CIDR /24", "192.168.1.0/24", true},
		{"Valid IPv4 CIDR /32", "192.168.1.1/32", true},
		{"Valid IPv4 CIDR /0", "0.0.0.0/0", true},
		{"Valid IPv4 CIDR /16", "172.16.0.0/16", true},

		// Invalid cases
		{"Valid IPv4 address", "192.168.1.0", false},
		{"Invalid IPv4 CIDR with leading zeros", "192.168.001.000/24", false},
		{"Missing prefix", "192.168.1.0/", false},
		{"Invalid prefix number", "192.168.1.0/33", false},
		{"Negative prefix", "192.168.1.0/-1", false},
		{"No prefix", "192.168.1.0", false},
		{"Invalid IP", "256.256.256.256/24", false},
		{"Empty string", "", false},
		{"Invalid format", "192.168.1/24", false},
		{"Invalid characters", "192.168.1.0a/24", false},
		{"IPv6 CIDR", "2001:db8::/32", false},
		{"IPv6 address with prefix", "2001:db8::1/128", false},
		{"Too many segments", "192.168.1.1.1/24", false},
		{"Invalid format with multiple slashes", "192.168.1.0/24/24", false},
		{"Only slash", "/", false},
		{"Only prefix", "/24", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIPv4CIDR(tt.cidr)
			if result != tt.expected {
				t.Errorf("IsIPv4CIDR(%s) = %v; want %v", tt.cidr, result, tt.expected)
			}
		})
	}
}

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"Valid IPv6", "2001:db8::1", true},
		{"Valid IPv6 full", "2001:0db8:0000:0000:0000:0000:0000:0001", true},
		{"Valid IPv6 compressed", "::", true},
		{"Valid IPv6 loopback", "::1", true},
		{"Valid IPv6 with zeros", "2001:db8:0:0:0:0:0:1", true},
		{"IPv4 address", "192.168.1.1", false},
		{"Invalid IPv6 too many segments", "2001:db8:0:0:0:0:0:1:1", false},
		{"Invalid IPv6 format", "2001::db8::1", false},
		{"Invalid characters", "2001:db8::1g", false},
		{"Empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIPv6(tt.ip)
			if result != tt.expected {
				t.Errorf("IsIPv6(%s) = %v; want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIsIPv6CIDR(t *testing.T) {
	tests := []struct {
		name     string
		cidr     string
		expected bool
	}{
		// Valid IPv6 CIDR cases
		{"Valid IPv6 CIDR /32", "2001:db8::/32", true},
		{"Valid IPv6 CIDR /128", "2001:db8::1/128", true},
		{"Valid IPv6 CIDR /0", "::/0", true},
		{"Valid IPv6 CIDR /48", "2001:db8:1::/48", true},
		{"Valid IPv6 CIDR full notation", "2001:0db8:0000:0000:0000:0000:0000:0000/64", true},
		{"Valid IPv6 CIDR with compressed zeros", "2001:db8:0:0:0:0:0:0/64", true},
		{"Valid IPv6 link-local", "fe80::/10", true},
		{"Valid IPv6 unique local", "fc00::/7", true},

		// Invalid cases
		{"Missing prefix", "2001:db8::/", false},
		{"Invalid prefix number", "2001:db8::/129", false},
		{"Negative prefix", "2001:db8::/-1", false},
		{"No prefix", "2001:db8::", false},
		{"Empty string", "", false},
		{"Invalid format", "2001:db8/64", false},
		{"Invalid hex characters", "2001:db8::g/64", false},
		{"IPv4 CIDR", "192.168.1.0/24", false},
		{"IPv4 address with prefix", "192.168.1.1/32", false},
		{"Double compression", "2001::db8::1/64", false},
		{"Invalid format with multiple slashes", "2001:db8::/64/64", false},
		{"Only slash", "/", false},
		{"Only prefix", "/64", false},
		{"Too many segments", "2001:db8:1:2:3:4:5:6:7/64", false},
		{"Invalid prefix format", "2001:db8::/64.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIPv6CIDR(tt.cidr)
			if result != tt.expected {
				t.Errorf("IsIPv6CIDR(%s) = %v; want %v", tt.cidr, result, tt.expected)
			}
		})
	}
}
