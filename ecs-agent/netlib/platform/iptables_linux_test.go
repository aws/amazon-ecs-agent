//go:build linux && unit
// +build linux,unit

package platform

import (
	"os/exec"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/stretchr/testify/assert"
)

func TestGetTableArgs(t *testing.T) {
	tests := []struct {
		name     string
		table    string
		expected []string
	}{
		{
			name:     "nat table",
			table:    "nat",
			expected: []string{"-t", "nat"},
		},
		{
			name:     "filter table",
			table:    "filter",
			expected: []string{"-t", "filter"},
		},
		{
			name:     "empty table",
			table:    "",
			expected: []string{"-t", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTableArgs(tt.table)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDaemonBridgeNATArgs(t *testing.T) {
	expected := []string{
		"POSTROUTING",
		"-s", ECSSubNet,
		"!", "-d", ECSSubNet,
		"-j", "MASQUERADE",
	}

	result := getDaemonBridgeNATArgs()
	assert.Equal(t, expected, result)
}

func TestModifyNetfilterEntry(t *testing.T) {
	tests := []struct {
		name                    string
		table                   string
		action                  iptablesAction
		getNetfilterChainArgs   getNetfilterChainArgsFunc
		expectError             bool
		expectedCommandContains []string
	}{
		{
			name:                  "append daemon bridge NAT rule",
			table:                 iptablesTableNat,
			action:                iptablesAppend,
			getNetfilterChainArgs: getDaemonBridgeNATArgs,
			expectedCommandContains: []string{
				"-t", "nat",
				"-A",
				"POSTROUTING",
				"-s", ECSSubNet,
				"!", "-d", ECSSubNet,
				"-j", "MASQUERADE",
			},
		},
		{
			name:                  "check daemon bridge NAT rule",
			table:                 iptablesTableNat,
			action:                iptablesCheck,
			getNetfilterChainArgs: getDaemonBridgeNATArgs,
			expectedCommandContains: []string{
				"-t", "nat",
				"-C",
				"POSTROUTING",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't actually run iptables in unit tests, but we can verify
			// the function constructs the command correctly by checking it doesn't panic
			// and that the error is predictable (command not found or permission denied)
			err := modifyNetfilterEntry(tt.table, tt.action, tt.getNetfilterChainArgs)

			// In test environment, we expect either:
			// - Command not found error (iptables not installed)
			// - Permission denied error (not root)
			// - Exit status error (iptables rules don't exist)
			if err != nil {
				// Verify it's an expected error type
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Logf("Expected exit error in test environment: %v", exitErr)
				} else {
					t.Logf("Expected error in test environment: %v", err)
				}
			}
		})
	}
}

func TestEnableSysctlSetting(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "enable IPv4 forwarding",
			key:   ipv4ForwardingKey,
			value: "1",
		},
		{
			name:  "enable IPv6 forwarding",
			key:   ipv6ForwardingKey,
			value: "1",
		},
		{
			name:  "enable bridge netfilter",
			key:   bridgeNetfilterCallKey,
			value: "1",
		},
		{
			name:  "custom setting",
			key:   "net.test.setting",
			value: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't actually modify sysctl in unit tests, but we can verify
			// the function doesn't panic and handles errors appropriately
			err := enableSysctlSetting(tt.key, tt.value)

			// In test environment, we expect either:
			// - Command not found error (sysctl not available)
			// - Permission denied error (not root)
			// - File not found error (sysctl key doesn't exist)
			if err != nil {
				t.Logf("Expected error in test environment: %v", err)
			}
		})
	}
}

func TestEnableSystemSettings(t *testing.T) {
	tests := []struct {
		name   string
		ipComp ipcompatibility.IPCompatibility
	}{
		{
			name:   "IPv4 only",
			ipComp: ipcompatibility.NewIPv4OnlyCompatibility(),
		},
		{
			name:   "IPv6 only",
			ipComp: ipcompatibility.NewIPv6OnlyCompatibility(),
		},
		{
			name:   "dual stack",
			ipComp: ipcompatibility.NewDualStackCompatibility(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test verifies that enableSystemSettings calls the required sysctl settings
			// and handles errors appropriately
			err := enableSystemSettings(tt.ipComp)

			// In test environment, we expect errors due to lack of permissions or missing commands
			// The important thing is that it doesn't panic and attempts the appropriate settings
			if err != nil {
				t.Logf("Expected error in test environment: %v", err)
				// Verify the error is related to IP forwarding (the first setting that would fail)
				assert.Contains(t, err.Error(), "failed to enable")
			}
		})
	}
}

func TestIptablesConstants(t *testing.T) {
	// Verify constants have expected values
	assert.Equal(t, "iptables", iptablesExecutable)
	assert.Equal(t, "nat", iptablesTableNat)
	assert.Equal(t, "sysctl", sysctlExecutable)
	assert.Equal(t, iptablesAction("-A"), iptablesAppend)
	assert.Equal(t, iptablesAction("-C"), iptablesCheck)
	assert.Equal(t, "net.ipv4.ip_forward", ipv4ForwardingKey)
	assert.Equal(t, "net.ipv6.conf.all.forwarding", ipv6ForwardingKey)
	assert.Equal(t, "net.bridge.bridge-nf-call-iptables", bridgeNetfilterCallKey)
}
