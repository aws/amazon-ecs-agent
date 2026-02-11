package platform

import (
	"fmt"
	"os/exec"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
)

// iptablesAction enumerates different actions for the iptables command.
type iptablesAction string

const (
	iptablesExecutable = "iptables"
	ipv6Tables         = "ip6tables"
	iptablesTableNat   = "nat"
	sysctlExecutable   = "sysctl"
	// iptablesAppend enumerates the 'append' action.
	iptablesAppend iptablesAction = "-A"
	// iptablesCheck enumerates the 'check' action.
	iptablesCheck iptablesAction = "-C"

	// sysctl configuration keys.
	ipv4ForwardingKey          = "net.ipv4.ip_forward"
	ipv6ForwardingKey          = "net.ipv6.conf.all.forwarding"
	bridgeNetfilterCallKey     = "net.bridge.bridge-nf-call-iptables"
	bridgeNetfilterCallIPv6Key = "net.bridge.bridge-nf-call-ip6tables"
)

// getNetfilterChainArgsFunc defines a function pointer type that returns
// a slice of arguments for modifying a netfilter chain.
type getNetfilterChainArgsFunc func() []string

// modifyNetfilterEntry modifies an entry in the netfilter table based on
// the action and the function pointer to get arguments for modifying the chain.
func modifyNetfilterEntry(table string, action iptablesAction, getNetfilterChainArgs getNetfilterChainArgsFunc, useIPv6 bool) error {
	executable := iptablesExecutable
	if useIPv6 {
		executable = ipv6Tables
	}

	args := append(getTableArgs(table), string(action))
	args = append(args, getNetfilterChainArgs()...)
	cmd := exec.Command(executable, args...)

	logger.Info("Executing iptables command", logger.Fields{
		"executable": executable,
		"args":       args,
		"table":      table,
		"action":     string(action),
		"ipv6":       useIPv6,
	})

	output, err := cmd.CombinedOutput()
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
