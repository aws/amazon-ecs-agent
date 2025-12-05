package platform

import (
	"fmt"
	"os/exec"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
)

// iptablesAction enumerates different actions for the iptables command
type iptablesAction string

const (
	iptablesExecutable = "iptables"
	iptablesTableNat   = "nat"
	sysctlExecutable   = "sysctl"
	// iptablesAppend enumerates the 'append' action
	iptablesAppend iptablesAction = "-A"
	// iptablesCheck enumerates the 'check' action
	iptablesCheck iptablesAction = "-C"

	// sysctl configuration keys
	ipForwardingKey        = "net.ipv4.ip_forward"
	bridgeNetfilterCallKey = "net.bridge.bridge-nf-call-iptables"
)

// getNetfilterChainArgsFunc defines a function pointer type that returns
// a slice of arguments for modifying a netfilter chain
type getNetfilterChainArgsFunc func() []string

// modifyNetfilterEntry modifies an entry in the netfilter table based on
// the action and the function pointer to get arguments for modifying the chain
func modifyNetfilterEntry(table string, action iptablesAction, getNetfilterChainArgs getNetfilterChainArgsFunc) error {
	args := append(getTableArgs(table), string(action))
	args = append(args, getNetfilterChainArgs()...)
	cmd := exec.Command(iptablesExecutable, args...)

	logger.Info("Executing iptables command", logger.Fields{
		"executable": iptablesExecutable,
		"args":       args,
		"table":      table,
		"action":     string(action),
	})

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("iptables command failed", logger.Fields{
			"executable":      iptablesExecutable,
			"args":            args,
			"output":          string(output),
			loggerfield.Error: err,
		})
		return err
	}

	logger.Info("iptables command succeeded", logger.Fields{
		"executable": iptablesExecutable,
		"args":       args,
		"output":     string(output),
	})

	return nil
}

func getTableArgs(table string) []string {
	return []string{"-t", table}
}

// getDaemonBridgeNATArgs returns arguments for daemon-bridge MASQUERADE rule
func getDaemonBridgeNATArgs() []string {
	return []string{
		"POSTROUTING",
		"-s", ECSSubNet,
		"!", "-d", ECSSubNet,
		"-j", "MASQUERADE",
	}
}

// enableSysctlSetting enables a sysctl setting with the given key and value
func enableSysctlSetting(key string, value string) error {
	cmd := exec.Command(sysctlExecutable, "-w", fmt.Sprintf("%s=%s", key, value))
	return cmd.Run()
}

// enableSystemSettings enables required system settings for NAT
func enableSystemSettings() error {
	// Enable IP forwarding
	if err := enableSysctlSetting(ipForwardingKey, "1"); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	// Enable bridge forwarding (ignore errors if bridge module not loaded)
	enableSysctlSetting(bridgeNetfilterCallKey, "1")

	return nil
}
