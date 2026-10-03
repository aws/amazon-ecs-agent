package platform

import (
	"fmt"
	"os/exec"
	"strconv"

	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
)

// iptablesAction enumerates different actions for the iptables command.
type iptablesAction string

const (
	iptablesExecutable  = "iptables"
	ipv6Tables          = "ip6tables"
	iptablesTableNat    = "nat"
	iptablesTableFilter = "filter"
	sysctlExecutable    = "sysctl"
	// iptablesAppend enumerates the 'append' action.
	iptablesAppend iptablesAction = "-A"
	// iptablesInsert enumerates the 'insert' action (inserts at the top of the
	// chain, i.e. before already-appended rules).
	iptablesInsert iptablesAction = "-I"
	// iptablesCheck enumerates the 'check' action.
	iptablesCheck iptablesAction = "-C"
	// iptablesDelete enumerates the 'delete' action.
	iptablesDelete iptablesAction = "-D"

	// iptablesWaitFlag makes iptables acquire the xtables lock (waiting rather
	// than failing) so concurrent iptables invocations don't spuriously error.
	iptablesWaitFlag = "-w"

	// sysctl configuration keys.
	ipv4ForwardingKey          = "net.ipv4.ip_forward"
	ipv6ForwardingKey          = "net.ipv6.conf.all.forwarding"
	bridgeNetfilterCallKey     = "net.bridge.bridge-nf-call-iptables"
	bridgeNetfilterCallIPv6Key = "net.bridge.bridge-nf-call-ip6tables"
)

// introspectionServerPort is the introspection server's TCP port as a string for
// iptables --dport args, derived from the canonical introspection.Port so the
// filter rules always target the same port the server binds.
var introspectionServerPort = strconv.Itoa(introspection.Port)

// getNetfilterChainArgsFunc defines a function pointer type that returns
// a slice of arguments for modifying a netfilter chain.
type getNetfilterChainArgsFunc func() []string

// runIptablesCommand executes the iptables/ip6tables binary with the given args
// and returns its combined output. It is a package-level var so unit tests can
// stub iptables execution to exercise error paths without invoking real
// iptables; production code never reassigns it.
var runIptablesCommand = func(executable string, args ...string) ([]byte, error) {
	// executable is a fixed iptables/ip6tables constant and args are built from
	// internal constants (chain, port, fixed bridge addresses); none are
	// user-controlled. exec.Command runs the binary directly (no shell), so there
	// is no shell-injection surface.
	// nosemgrep: command-injection-exec-variable
	return exec.Command(executable, args...).CombinedOutput()
}

// modifyNetfilterEntry modifies an entry in the netfilter table based on
// the action and the function pointer to get arguments for modifying the chain.
func modifyNetfilterEntry(table string, action iptablesAction, getNetfilterChainArgs getNetfilterChainArgsFunc, useIPv6 bool) error {
	executable := iptablesExecutable
	if useIPv6 {
		executable = ipv6Tables
	}

	args := buildIptablesArgs(table, action, getNetfilterChainArgs())

	logger.Info("Executing iptables command", logger.Fields{
		"executable": executable,
		"args":       args,
		"table":      table,
		"action":     string(action),
		"ipv6":       useIPv6,
	})

	output, err := runIptablesCommand(executable, args...)
	if err != nil {
		logger.Error("iptables command failed", logger.Fields{
			"executable":      executable,
			"args":            args,
			"output":          string(output),
			loggerfield.Error: err,
		})
		return err
	}

	logger.Info("iptables command succeeded", logger.Fields{
		"executable": executable,
		"args":       args,
		"output":     string(output),
	})

	return nil
}

func getTableArgs(table string) []string {
	return []string{"-t", table}
}

// buildIptablesArgs assembles the full iptables argument list for a command:
// the -w wait flag, the table selector, the action, then the chain args. -w
// makes iptables wait for the xtables lock instead of failing when another
// iptables invocation holds it (this code runs concurrently with NAT setup).
func buildIptablesArgs(table string, action iptablesAction, chainArgs []string) []string {
	args := append([]string{iptablesWaitFlag}, getTableArgs(table)...)
	args = append(args, string(action))
	args = append(args, chainArgs...)
	return args
}

// getDaemonBridgeNATArgs returns arguments for daemon-bridge MASQUERADE rule.
// The subnet parameter specifies the source network for NAT (e.g., ECSSubNet for IPv4 or ECSSubNetIPv6 for IPv6).
func getDaemonBridgeNATArgs(subnet string) []string {
	return []string{
		"POSTROUTING",
		"-s", subnet,
		"!", "-d", subnet,
		"-j", "MASQUERADE",
	}
}

// getSimpleIPv6NATArgs returns simple MASQUERADE rule for all IPv6 traffic.
// Use this if you don't want to restrict by source subnet.
func getSimpleIPv6NATArgs() []string {
	return []string{
		"POSTROUTING",
		"-o", "eth0", // Output interface.
		"-j", "MASQUERADE",
	}
}

// getIntrospectionAllowDaemonArgs returns a filter-table INPUT rule that accepts
// introspection traffic on the daemon bridge from the given source address
// (daemonAddr, e.g. DaemonBridgeIP for IPv4).
func getIntrospectionAllowDaemonArgs(daemonAddr string) []string {
	return []string{
		"INPUT",
		"-i", BridgeInterfaceName,
		"-p", "tcp",
		"--dport", introspectionServerPort,
		"-s", daemonAddr,
		"-j", "ACCEPT",
	}
}

// getIntrospectionBridgeDropArgs returns a filter-table INPUT rule that drops
// introspection traffic on the daemon bridge from any source.
func getIntrospectionBridgeDropArgs() []string {
	return []string{
		"INPUT",
		"-i", BridgeInterfaceName,
		"-p", "tcp",
		"--dport", introspectionServerPort,
		"-j", "DROP",
	}
}

// enableSysctlSetting enables a sysctl setting with the given key and value.
func enableSysctlSetting(key string, value string) error {
	cmd := exec.Command(sysctlExecutable, "-w", fmt.Sprintf("%s=%s", key, value))
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("sysctl command failed", logger.Fields{
			"key":             key,
			"value":           value,
			"output":          string(output),
			loggerfield.Error: err,
		})
		return err
	}
	logger.Info("sysctl setting enabled", logger.Fields{
		"key":   key,
		"value": value,
	})
	return nil
}

// enableSystemSettings enables required system settings for NAT based on IP compatibility.
// This is needed because daemon-bridge mode is similar to Docker bridge networking, where an isolated
// network namespace shares connectivity via a bridge interface. The Linux kernel must forward
// packets from the daemon namespace through the bridge to the host ENI. The iptables/ip6tables
// NAT rules (configured elsewhere) perform the address translation for external connectivity.
func enableSystemSettings(ipComp ipcompatibility.IPCompatibility) error {
	// Enable IPv4 forwarding if IPv4 compatible.
	if ipComp.IsIPv4Compatible() {
		if err := enableSysctlSetting(ipv4ForwardingKey, "1"); err != nil {
			return fmt.Errorf("failed to enable IPv4 forwarding: %w", err)
		}
	}

	// Enable IPv6 forwarding if IPv6 compatible.
	if ipComp.IsIPv6Compatible() {
		if err := enableSysctlSetting(ipv6ForwardingKey, "1"); err != nil {
			return fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
		}

		// Also enable forwarding on specific interfaces.
		if err := enableSysctlSetting("net.ipv6.conf.eth0.forwarding", "1"); err != nil {
			return fmt.Errorf("failed to enable IPv6 forwarding on eth0: %w", err)
		}
		if err := enableSysctlSetting("net.ipv6.conf.fargate-bridge.forwarding", "1"); err != nil {
			return fmt.Errorf("failed to enable IPv6 forwarding on fargate-bridge: %w", err)
		}
	}

	// Enable bridge forwarding (ignore errors if bridge module not loaded).
	enableSysctlSetting(bridgeNetfilterCallKey, "1")
	if ipComp.IsIPv6Compatible() {
		enableSysctlSetting(bridgeNetfilterCallIPv6Key, "1")
	}

	return nil
}

// setupNATRule sets up a NAT rule using the provided arguments function.
// It checks if the rule already exists before adding it, and logs appropriate messages.
// Parameters:
//   - getArgs: function that returns the netfilter chain arguments.
//   - useIPv6: whether to use ip6tables (true) or iptables (false).
//   - ruleDescription: human-readable description for log messages (e.g., "IPv4 NAT rule").
func setupNATRule(getArgs getNetfilterChainArgsFunc, useIPv6 bool, ruleDescription string) error {
	// Check if the rule already exists.
	if err := modifyNetfilterEntry(iptablesTableNat, iptablesCheck, getArgs, useIPv6); err != nil {
		// Rule doesn't exist, so add it.
		if err := modifyNetfilterEntry(iptablesTableNat, iptablesAppend, getArgs, useIPv6); err != nil {
			return fmt.Errorf("failed to add %s: %w", ruleDescription, err)
		}
		logger.Info(fmt.Sprintf("%s added successfully", ruleDescription))
	} else {
		logger.Info(fmt.Sprintf("%s already exists", ruleDescription))
	}

	return nil
}

// setupFilterRule sets up a filter-table rule using the provided arguments
// function, using the given action (append or insert). It checks for existence
// first so it is idempotent, mirroring setupNATRule.
func setupFilterRule(action iptablesAction, getArgs getNetfilterChainArgsFunc, useIPv6 bool, ruleDescription string) error {
	if err := modifyNetfilterEntry(iptablesTableFilter, iptablesCheck, getArgs, useIPv6); err != nil {
		if err := modifyNetfilterEntry(iptablesTableFilter, action, getArgs, useIPv6); err != nil {
			return fmt.Errorf("failed to add %s: %w", ruleDescription, err)
		}
		logger.Info(fmt.Sprintf("%s added successfully", ruleDescription))
	} else {
		logger.Info(fmt.Sprintf("%s already exists", ruleDescription))
	}
	return nil
}

// SetupIntrospectionFirewall appends a filter-table INPUT rule that drops all
// introspection traffic arriving on the daemon bridge. The rule is appended for
// IPv4, and additionally for IPv6 when ipv6Enabled is true — gating on
// ipv6Enabled both scopes the rule to hosts that use IPv6 and avoids invoking
// ip6tables on hosts without an IPv6 stack.
//
// The rule matches the bridge interface by name even before that interface
// exists (iptables allows this); it simply matches no traffic until the bridge
// is created. Idempotent: the rule is added only if not already present.
func SetupIntrospectionFirewall(ipv6Enabled bool) error {
	if err := setupFilterRule(iptablesAppend, getIntrospectionBridgeDropArgs, false, "IPv4 introspection bridge-drop rule"); err != nil {
		return err
	}
	if ipv6Enabled {
		if err := setupFilterRule(iptablesAppend, getIntrospectionBridgeDropArgs, true, "IPv6 introspection bridge-drop rule"); err != nil {
			return err
		}
	}
	return nil
}

// allowDaemonIntrospection inserts a filter-table INPUT rule that accepts
// introspection traffic on the daemon bridge from the daemon namespace's fixed
// source address. It is inserted at the top of the chain so it takes precedence
// over lower-priority rules. The rule is added for IPv4, and additionally for
// IPv6 when ipv6Enabled is true.
//
// Idempotent: the rule is added only if not already present.
func allowDaemonIntrospection(ipv6Enabled bool) error {
	allowV4 := func() []string { return getIntrospectionAllowDaemonArgs(DaemonBridgeIP) }
	if err := setupFilterRule(iptablesInsert, allowV4, false, "IPv4 introspection allow-daemon rule"); err != nil {
		return err
	}
	if ipv6Enabled {
		allowV6 := func() []string { return getIntrospectionAllowDaemonArgs(DaemonBridgeIPv6) }
		if err := setupFilterRule(iptablesInsert, allowV6, true, "IPv6 introspection allow-daemon rule"); err != nil {
			return err
		}
	}
	return nil
}

// deleteFilterRule removes a filter-table rule if present, checking existence
// first so it is idempotent (a -D on a missing rule would error). It is the
// inverse of setupFilterRule.
func deleteFilterRule(getArgs getNetfilterChainArgsFunc, useIPv6 bool, ruleDescription string) error {
	if err := modifyNetfilterEntry(iptablesTableFilter, iptablesCheck, getArgs, useIPv6); err != nil {
		// Rule not present; nothing to delete.
		logger.Info(fmt.Sprintf("%s not present, nothing to remove", ruleDescription))
		return nil
	}
	if err := modifyNetfilterEntry(iptablesTableFilter, iptablesDelete, getArgs, useIPv6); err != nil {
		return fmt.Errorf("failed to remove %s: %w", ruleDescription, err)
	}
	logger.Info(fmt.Sprintf("%s removed successfully", ruleDescription))
	return nil
}

// disallowDaemonIntrospection removes the filter-table INPUT rule that accepts
// introspection traffic from the daemon namespace's fixed source address, for a
// single address family (IPv6 when useIPv6 is true, else IPv4). The caller
// invokes it once per family so a failure on one family does not prevent the
// other from being cleaned up.
//
// Idempotent: removing an absent rule is a no-op, not an error.
func disallowDaemonIntrospection(useIPv6 bool) error {
	addr, description := DaemonBridgeIP, "IPv4 introspection allow-daemon rule"
	if useIPv6 {
		addr, description = DaemonBridgeIPv6, "IPv6 introspection allow-daemon rule"
	}
	allow := func() []string { return getIntrospectionAllowDaemonArgs(addr) }
	return deleteFilterRule(allow, useIPv6, description)
}

// SetupIPv6NAT sets up IPv6 NAT rules for the daemon bridge.
// ipv6Subnet should be something like "2600:1f13:f3e:4301::/64".
// If empty, it will use a simple MASQUERADE rule for all traffic.
// It delegates to the unified setupNATRule function with IPv6 parameters.
func SetupIPv6NAT(ipv6Subnet string) error {
	var getArgs getNetfilterChainArgsFunc

	if ipv6Subnet != "" {
		getArgs = func() []string {
			return getDaemonBridgeNATArgs(ipv6Subnet)
		}
	} else {
		getArgs = getSimpleIPv6NATArgs
	}

	return setupNATRule(getArgs, true, "IPv6 NAT rule")
}

// SetupIPv4NAT sets up IPv4 NAT rules for the daemon bridge.
// It delegates to the unified setupNATRule function with IPv4 parameters.
func SetupIPv4NAT() error {
	getArgs := func() []string {
		return getDaemonBridgeNATArgs(ECSSubNet)
	}
	return setupNATRule(getArgs, false, "IPv4 NAT rule")
}

// SetupNAT sets up both IPv4 and IPv6 NAT based on IP compatibility.
func SetupNAT(ipComp ipcompatibility.IPCompatibility, ipv6Subnet string) error {
	// Enable system settings first.
	if err := enableSystemSettings(ipComp); err != nil {
		return fmt.Errorf("failed to enable system settings: %w", err)
	}

	// Setup IPv4 NAT.
	if ipComp.IsIPv4Compatible() {
		if err := SetupIPv4NAT(); err != nil {
			return fmt.Errorf("failed to setup IPv4 NAT: %w", err)
		}
	}

	// Setup IPv6 NAT.
	if ipComp.IsIPv6Compatible() {
		if err := SetupIPv6NAT(ipv6Subnet); err != nil {
			return fmt.Errorf("failed to setup IPv6 NAT: %w", err)
		}
	}

	return nil
}
