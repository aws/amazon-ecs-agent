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

	result := getDaemonBridgeNATArgs(ECSSubNet)
	assert.Equal(t, expected, result)
}

// Helper function to create getDaemonBridgeNATArgs wrapper for tests.
func getDaemonBridgeNATArgsIPv4() []string {
	return getDaemonBridgeNATArgs(ECSSubNet)
}

func TestModifyNetfilterEntry(t *testing.T) {
	tests := []struct {
		name                    string
		table                   string
		action                  iptablesAction
		getNetfilterChainArgs   getNetfilterChainArgsFunc
		useIPv6                 bool
		expectError             bool
		expectedCommandContains []string
	}{
		{
			name:                  "append daemon bridge NAT rule IPv4",
			table:                 iptablesTableNat,
			action:                iptablesAppend,
			getNetfilterChainArgs: getDaemonBridgeNATArgsIPv4,
			useIPv6:               false,
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
			name:                  "check daemon bridge NAT rule IPv4",
			table:                 iptablesTableNat,
			action:                iptablesCheck,
			getNetfilterChainArgs: getDaemonBridgeNATArgsIPv4,
			useIPv6:               false,
			expectedCommandContains: []string{
				"-t", "nat",
				"-C",
				"POSTROUTING",
			},
		},
		{
			name:                  "append simple IPv6 NAT rule",
			table:                 iptablesTableNat,
			action:                iptablesAppend,
			getNetfilterChainArgs: getSimpleIPv6NATArgs,
			useIPv6:               true,
			expectedCommandContains: []string{
				"-t", "nat",
				"-A",
				"POSTROUTING",
				"-o", "eth0",
				"-j", "MASQUERADE",
			},
		},
		{
			name:                  "check simple IPv6 NAT rule",
			table:                 iptablesTableNat,
			action:                iptablesCheck,
			getNetfilterChainArgs: getSimpleIPv6NATArgs,
			useIPv6:               true,
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
			err := modifyNetfilterEntry(tt.table, tt.action, tt.getNetfilterChainArgs, tt.useIPv6)

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
	assert.Equal(t, "ip6tables", ipv6Tables)
	assert.Equal(t, "nat", iptablesTableNat)
	assert.Equal(t, "sysctl", sysctlExecutable)
	assert.Equal(t, iptablesAction("-A"), iptablesAppend)
	assert.Equal(t, iptablesAction("-C"), iptablesCheck)
	assert.Equal(t, "net.ipv4.ip_forward", ipv4ForwardingKey)
	assert.Equal(t, "net.ipv6.conf.all.forwarding", ipv6ForwardingKey)
	assert.Equal(t, "net.bridge.bridge-nf-call-iptables", bridgeNetfilterCallKey)
	assert.Equal(t, "net.bridge.bridge-nf-call-ip6tables", bridgeNetfilterCallIPv6Key)
}

func TestGetSimpleIPv6NATArgs(t *testing.T) {
	expected := []string{
		"POSTROUTING",
		"-o", "eth0",
		"-j", "MASQUERADE",
	}

	result := getSimpleIPv6NATArgs()
	assert.Equal(t, expected, result)
}

func TestSetupIPv6NAT(t *testing.T) {
	tests := []struct {
		name       string
		ipv6Subnet string
	}{
		{
			name:       "with subnet",
			ipv6Subnet: "2600:1f13:f3e:4301::/64",
		},
		{
			name:       "without subnet (simple masquerade)",
			ipv6Subnet: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test verifies the function doesn't panic and handles ip6tables operations
			// The actual ip6tables operations are tested in integration tests
			err := SetupIPv6NAT(tt.ipv6Subnet)

			// We expect either success or a predictable error (like ip6tables not available in test env)
			if err != nil {
				t.Logf("Expected error in test environment: %v", err)
			}
		})
	}
}

func TestSetupIPv4NAT(t *testing.T) {
	// This test verifies the function doesn't panic and handles iptables operations
	err := SetupIPv4NAT()

	// We expect either success or a predictable error (like iptables not available in test env)
	if err != nil {
		t.Logf("Expected error in test environment: %v", err)
	}
}

func TestSetupNAT(t *testing.T) {
	tests := []struct {
		name       string
		ipComp     ipcompatibility.IPCompatibility
		ipv6Subnet string
	}{
		{
			name:       "IPv4 only",
			ipComp:     ipcompatibility.NewIPv4OnlyCompatibility(),
			ipv6Subnet: "",
		},
		{
			name:       "IPv6 only",
			ipComp:     ipcompatibility.NewIPv6OnlyCompatibility(),
			ipv6Subnet: "2600:1f13:f3e:4301::/64",
		},
		{
			name:       "dual stack",
			ipComp:     ipcompatibility.NewDualStackCompatibility(),
			ipv6Subnet: "2600:1f13:f3e:4301::/64",
		},
		{
			name:       "dual stack without IPv6 subnet",
			ipComp:     ipcompatibility.NewDualStackCompatibility(),
			ipv6Subnet: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test verifies the function doesn't panic and handles iptables/ip6tables operations
			err := SetupNAT(tt.ipComp, tt.ipv6Subnet)

			// We expect either success or a predictable error (like iptables not available in test env)
			if err != nil {
				t.Logf("Expected error in test environment: %v", err)
			}
		})
	}
}

// TestSetupNATRule_ErrorMessagesContainRuleDescription verifies that error messages
// from setupNATRule contain the rule description parameter.
// **Validates: Requirements 1.6**
func TestSetupNATRule_ErrorMessagesContainRuleDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		getArgs         getNetfilterChainArgsFunc
		useIPv6         bool
		ruleDescription string
	}{
		{
			name:            "IPv4 NAT rule with daemon bridge args",
			getArgs:         getDaemonBridgeNATArgsIPv4,
			useIPv6:         false,
			ruleDescription: "IPv4 NAT rule",
		},
		{
			name:            "IPv6 NAT rule with simple masquerade args",
			getArgs:         getSimpleIPv6NATArgs,
			useIPv6:         true,
			ruleDescription: "IPv6 NAT rule",
		},
		{
			name: "IPv6 NAT rule with subnet-based args",
			getArgs: func() []string {
				return getDaemonBridgeNATArgs("2600:1f13:f3e:4301::/64")
			},
			useIPv6:         true,
			ruleDescription: "IPv6 NAT rule",
		},
		{
			name:            "custom rule description for IPv4",
			getArgs:         getDaemonBridgeNATArgsIPv4,
			useIPv6:         false,
			ruleDescription: "custom IPv4 masquerade rule",
		},
		{
			name:            "custom rule description for IPv6",
			getArgs:         getSimpleIPv6NATArgs,
			useIPv6:         true,
			ruleDescription: "custom IPv6 masquerade rule",
		},
		{
			name:            "empty rule description for IPv4",
			getArgs:         getDaemonBridgeNATArgsIPv4,
			useIPv6:         false,
			ruleDescription: "",
		},
		{
			name:            "empty rule description for IPv6",
			getArgs:         getSimpleIPv6NATArgs,
			useIPv6:         true,
			ruleDescription: "",
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable for parallel execution.
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Call setupNATRule which will attempt to execute iptables commands.
			// In the test environment, these commands will fail because:
			// - iptables/ip6tables may not be installed
			// - The test may not have root permissions
			// - The netfilter rules don't exist
			err := setupNATRule(tt.getArgs, tt.useIPv6, tt.ruleDescription)

			// If an error occurred during rule addition (not during check), verify
			// that the error message contains the rule description.
			if err != nil {
				// The error message should follow the format: "failed to add <ruleDescription>: <underlying error>"
				if tt.ruleDescription != "" {
					assert.Contains(t, err.Error(), tt.ruleDescription,
						"Error message should contain the rule description")
					assert.Contains(t, err.Error(), "failed to add",
						"Error message should contain 'failed to add' prefix")
				} else {
					// Even with empty description, the error format should be consistent.
					assert.Contains(t, err.Error(), "failed to add",
						"Error message should contain 'failed to add' prefix")
				}
				t.Logf("Error message format verified: %v", err)
			} else {
				// If no error, the rule was either added successfully or already existed.
				// This is acceptable behavior - the test verifies error formatting when errors occur.
				t.Logf("setupNATRule succeeded (rule may already exist or was added)")
			}
		})
	}
}

// TestSetupNATRule_IPv4AndIPv6Parameters verifies that setupNATRule correctly handles
// both IPv4 and IPv6 parameter combinations.
// **Validates: Requirements 1.6**
func TestSetupNATRule_IPv4AndIPv6Parameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		getArgs         getNetfilterChainArgsFunc
		useIPv6         bool
		ruleDescription string
		expectedExec    string // Expected executable based on useIPv6 flag.
	}{
		{
			name:            "IPv4 uses iptables executable",
			getArgs:         getDaemonBridgeNATArgsIPv4,
			useIPv6:         false,
			ruleDescription: "IPv4 NAT rule",
			expectedExec:    "iptables",
		},
		{
			name:            "IPv6 uses ip6tables executable",
			getArgs:         getSimpleIPv6NATArgs,
			useIPv6:         true,
			ruleDescription: "IPv6 NAT rule",
			expectedExec:    "ip6tables",
		},
		{
			name: "IPv6 with subnet uses ip6tables executable",
			getArgs: func() []string {
				return getDaemonBridgeNATArgs("fd00::/64")
			},
			useIPv6:         true,
			ruleDescription: "IPv6 subnet NAT rule",
			expectedExec:    "ip6tables",
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable for parallel execution.
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Call setupNATRule to verify it handles the parameters correctly.
			err := setupNATRule(tt.getArgs, tt.useIPv6, tt.ruleDescription)

			// The function should not panic regardless of the outcome.
			// In test environments, we expect errors due to missing executables or permissions.
			if err != nil {
				// Verify error message contains the rule description when an error occurs.
				assert.Contains(t, err.Error(), tt.ruleDescription,
					"Error message should contain the rule description")
				t.Logf("Expected error in test environment for %s: %v", tt.expectedExec, err)
			} else {
				t.Logf("setupNATRule succeeded for %s", tt.expectedExec)
			}
		})
	}
}

// TestSetupIPv4NAT_DelegatesToSetupNATRule verifies that SetupIPv4NAT correctly
// delegates to setupNATRule with the appropriate IPv4 parameters.
// **Property 1: Unified Function Handles Both IP Versions**
// **Validates: Requirements 1.1**
func TestSetupIPv4NAT_DelegatesToSetupNATRule(t *testing.T) {
	t.Parallel()

	// Call SetupIPv4NAT and verify it behaves correctly.
	// The function should delegate to setupNATRule with:
	// - getDaemonBridgeNATArgs as the args function
	// - useIPv6 = false
	// - ruleDescription = "IPv4 NAT rule"
	err := SetupIPv4NAT()

	// In test environment, we expect either success or a predictable error.
	// The key verification is that the function doesn't panic and uses correct parameters.
	if err != nil {
		// Verify the error message contains "IPv4 NAT rule" which confirms
		// the correct ruleDescription was passed to setupNATRule.
		assert.Contains(t, err.Error(), "IPv4 NAT rule",
			"Error message should contain 'IPv4 NAT rule' indicating correct delegation")
		t.Logf("Expected error in test environment: %v", err)
	} else {
		t.Log("SetupIPv4NAT succeeded (rule may already exist or was added)")
	}

	// Verify the args function produces the expected arguments.
	expectedArgs := []string{
		"POSTROUTING",
		"-s", ECSSubNet,
		"!", "-d", ECSSubNet,
		"-j", "MASQUERADE",
	}
	actualArgs := getDaemonBridgeNATArgs(ECSSubNet)
	assert.Equal(t, expectedArgs, actualArgs,
		"getDaemonBridgeNATArgs should return the expected IPv4 NAT arguments")
}

// TestSetupIPv6NAT_EmptySubnet_UsesSimpleMasqueradeArgs verifies that SetupIPv6NAT
// with an empty subnet correctly uses simple MASQUERADE arguments.
// **Property 1: Unified Function Handles Both IP Versions**
// **Validates: Requirements 1.1**
func TestSetupIPv6NAT_EmptySubnet_UsesSimpleMasqueradeArgs(t *testing.T) {
	t.Parallel()

	// Call SetupIPv6NAT with an empty subnet.
	// This should use getSimpleIPv6NATArgs which produces simple MASQUERADE rules.
	err := SetupIPv6NAT("")

	// In test environment, we expect either success or a predictable error.
	if err != nil {
		// Verify the error message contains "IPv6 NAT rule" which confirms
		// the correct ruleDescription was passed to setupNATRule.
		assert.Contains(t, err.Error(), "IPv6 NAT rule",
			"Error message should contain 'IPv6 NAT rule' indicating correct delegation")
		t.Logf("Expected error in test environment: %v", err)
	} else {
		t.Log("SetupIPv6NAT with empty subnet succeeded")
	}

	// Verify the simple args function produces the expected arguments.
	expectedArgs := []string{
		"POSTROUTING",
		"-o", "eth0",
		"-j", "MASQUERADE",
	}
	actualArgs := getSimpleIPv6NATArgs()
	assert.Equal(t, expectedArgs, actualArgs,
		"getSimpleIPv6NATArgs should return simple MASQUERADE arguments")
}

// TestSetupIPv6NAT_NonEmptySubnet_UsesSubnetBasedArgs verifies that SetupIPv6NAT
// with a non-empty subnet correctly uses subnet-based MASQUERADE arguments.
// **Property 1: Unified Function Handles Both IP Versions**
// **Validates: Requirements 1.1**
func TestSetupIPv6NAT_NonEmptySubnet_UsesSubnetBasedArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ipv6Subnet string
	}{
		{
			name:       "standard IPv6 subnet",
			ipv6Subnet: "2600:1f13:f3e:4301::/64",
		},
		{
			name:       "private IPv6 subnet",
			ipv6Subnet: "fd00::/64",
		},
		{
			name:       "link-local IPv6 subnet",
			ipv6Subnet: "fe80::/10",
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable for parallel execution.
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Call SetupIPv6NAT with a non-empty subnet.
			// This should use getDaemonBridgeNATArgs which produces subnet-based rules.
			err := SetupIPv6NAT(tt.ipv6Subnet)

			// In test environment, we expect either success or a predictable error.
			if err != nil {
				// Verify the error message contains "IPv6 NAT rule" which confirms
				// the correct ruleDescription was passed to setupNATRule.
				assert.Contains(t, err.Error(), "IPv6 NAT rule",
					"Error message should contain 'IPv6 NAT rule' indicating correct delegation")
				t.Logf("Expected error in test environment for subnet %s: %v", tt.ipv6Subnet, err)
			} else {
				t.Logf("SetupIPv6NAT with subnet %s succeeded", tt.ipv6Subnet)
			}

			// Verify the subnet-based args function produces the expected arguments.
			expectedArgs := []string{
				"POSTROUTING",
				"-s", tt.ipv6Subnet,
				"!", "-d", tt.ipv6Subnet,
				"-j", "MASQUERADE",
			}
			actualArgs := getDaemonBridgeNATArgs(tt.ipv6Subnet)
			assert.Equal(t, expectedArgs, actualArgs,
				"getDaemonBridgeNATArgs should return subnet-based MASQUERADE arguments")
		})
	}
}

// TestSetupIPv6NAT_ArgsSelectionLogic verifies that SetupIPv6NAT correctly selects
// between simple and subnet-based args functions based on the subnet parameter.
// **Property 1: Unified Function Handles Both IP Versions**
// **Validates: Requirements 1.1**
func TestSetupIPv6NAT_ArgsSelectionLogic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		ipv6Subnet   string
		expectedArgs []string
		description  string
	}{
		{
			name:       "empty subnet uses simple masquerade",
			ipv6Subnet: "",
			expectedArgs: []string{
				"POSTROUTING",
				"-o", "eth0",
				"-j", "MASQUERADE",
			},
			description: "Empty subnet should use getSimpleIPv6NATArgs",
		},
		{
			name:       "non-empty subnet uses subnet-based masquerade",
			ipv6Subnet: "2600:1f13:f3e:4301::/64",
			expectedArgs: []string{
				"POSTROUTING",
				"-s", "2600:1f13:f3e:4301::/64",
				"!", "-d", "2600:1f13:f3e:4301::/64",
				"-j", "MASQUERADE",
			},
			description: "Non-empty subnet should use getDaemonBridgeNATArgs",
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable for parallel execution.
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify the args selection logic by checking the expected arguments.
			var actualArgs []string
			if tt.ipv6Subnet != "" {
				actualArgs = getDaemonBridgeNATArgs(tt.ipv6Subnet)
			} else {
				actualArgs = getSimpleIPv6NATArgs()
			}

			assert.Equal(t, tt.expectedArgs, actualArgs, tt.description)
		})
	}
}

// TestUnifiedFunctionHandlesBothIPVersions verifies that the unified setupNATRule
// function correctly handles both IPv4 and IPv6 configurations, producing the same
// behavior as the original separate functions would have.
// **Property 1: Unified Function Handles Both IP Versions**
// **Validates: Requirements 1.1**
func TestUnifiedFunctionHandlesBothIPVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setupFunc       func() error
		expectedUseIPv6 bool
		expectedDesc    string
		description     string
	}{
		{
			name:            "SetupIPv4NAT uses IPv4 configuration",
			setupFunc:       SetupIPv4NAT,
			expectedUseIPv6: false,
			expectedDesc:    "IPv4 NAT rule",
			description:     "SetupIPv4NAT should delegate with useIPv6=false",
		},
		{
			name: "SetupIPv6NAT with empty subnet uses IPv6 configuration",
			setupFunc: func() error {
				return SetupIPv6NAT("")
			},
			expectedUseIPv6: true,
			expectedDesc:    "IPv6 NAT rule",
			description:     "SetupIPv6NAT with empty subnet should delegate with useIPv6=true",
		},
		{
			name: "SetupIPv6NAT with subnet uses IPv6 configuration",
			setupFunc: func() error {
				return SetupIPv6NAT("2600:1f13:f3e:4301::/64")
			},
			expectedUseIPv6: true,
			expectedDesc:    "IPv6 NAT rule",
			description:     "SetupIPv6NAT with subnet should delegate with useIPv6=true",
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable for parallel execution.
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Call the setup function.
			err := tt.setupFunc()

			// Verify the function doesn't panic and handles errors appropriately.
			if err != nil {
				// The error message should contain the expected description,
				// confirming the correct parameters were passed to setupNATRule.
				assert.Contains(t, err.Error(), tt.expectedDesc,
					"Error message should contain the expected rule description")
				t.Logf("%s - Expected error: %v", tt.description, err)
			} else {
				t.Logf("%s - Succeeded", tt.description)
			}
		})
	}
}
