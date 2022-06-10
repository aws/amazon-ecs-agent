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

package serviceconnect

import (
	"fmt"
	"net"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	BridgeNetworkMode         = "bridge"
	AWSVPCNetworkMode         = "awsvpc"
	invalidEgressConfigFormat = `no service connect %s in the egress config. %s`
	portCollisionFormat       = `%s port collision detected in the ingress config with the %s port=%d, and listener name=%s`
	invalidIngressPortFormat  = `the %s port=%d in the ingress config is not valid: %w`
	warningIngressPortFormat  = `Service connect config validation: %s port should not exist for %s mode in the ingress config`
	invalidDnsEntryFormat     = `no %s in the DNS config hostname=%s, address=%s`
)

// validateContainerName validates the service connect container name.
func validateContainerName(scContainerName string, taskContainers []*ecsacs.Container) error {
	// service connect container name is required
	if scContainerName == "" {
		return fmt.Errorf("missing service connect container name")
	}

	// validate the specified service connect container name exists in the task definition
	numOfFoundSCContainer := 0
	for _, container := range taskContainers {
		if aws.StringValue(container.Name) == scContainerName {
			numOfFoundSCContainer += 1
		}
	}

	if numOfFoundSCContainer == 0 {
		return fmt.Errorf("service connect container name=%s does not exist in the task", scContainerName)
	} else if numOfFoundSCContainer > 1 {
		return fmt.Errorf("found %d duplicate service connect container name=%s exist in the task", numOfFoundSCContainer, scContainerName)
	}

	return nil
}

// validateEgressConfig validates the service connect egress config.
func validateEgressConfig(scEgressConfig *EgressConfig, ipv6Enabled bool) error {
	// egress config can be empty for the first service since there are no other tasks that it can talk to
	if scEgressConfig == nil {
		return nil
	}

	// ListenerName is required if the egress config exists
	if scEgressConfig.ListenerName == "" {
		return fmt.Errorf(invalidEgressConfigFormat, "listener name", "")
	}

	// VIP is required if the egress config exists
	// IPV4CIDR should be always required because an IPv6-only mode is not supoorted at this moment
	if scEgressConfig.VIP.IPV4CIDR == "" {
		return fmt.Errorf(invalidEgressConfigFormat, "VIP IPv4CIDR", "")
	}

	// IPV6CIDR is required when IPv6 is enabled
	if ipv6Enabled && scEgressConfig.VIP.IPV6CIDR == "" {
		return fmt.Errorf(invalidEgressConfigFormat, "VIP IPv6CIDR", "It must not be empty when the task is IPv6 enabled")
	}

	// validate IPV4CIDR if it exists
	if scEgressConfig.VIP.IPV4CIDR != "" {
		trimmedIpv4cidr := strings.TrimSpace(scEgressConfig.VIP.IPV4CIDR)
		if err := validateCIDR(trimmedIpv4cidr, "IPv4"); err != nil {
			return err
		}
	}

	// validate IPV6CIDR if it exists
	if scEgressConfig.VIP.IPV6CIDR != "" {
		trimmedIpv6cidr := strings.TrimSpace(scEgressConfig.VIP.IPV6CIDR)
		if err := validateCIDR(trimmedIpv6cidr, "IPv6"); err != nil {
			return err
		}
	}

	return nil
}

// validateCIDR validates the passed CIDR is a valid IPv4/IPv6 CIDR based on the protocol.
func validateCIDR(cidr, protocol string) error {
	ip, _, err := net.ParseCIDR(cidr)
	if err == nil {
		if valid := getProtocol(ip, protocol); valid {
			return nil
		}
	}

	return fmt.Errorf("cidr=%s is not a valid %s CIDR", cidr, protocol)
}

// getProtocol returns validity of the given IP based on the target protocol.
func getProtocol(ip net.IP, protocol string) bool {
	switch protocol {
	case "IPv4":
		if ip.To4() != nil {
			return true
		}
	case "IPv6":
		if ip.To16() != nil {
			return true
		}
	default:
		return false
	}
	return false
}

// validateDnsConfig validates the service connnect DNS config.
func validateDnsConfig(scDnsConfligList []DNSConfigEntry, scEgressConfig *EgressConfig, ipv6Enabled bool) error {
	// DNS config associates to egress config
	if len(scDnsConfligList) == 0 && scEgressConfig != nil {
		return fmt.Errorf("no service connect DNS config. The DNS config is required when the egress config exists")
	}

	for _, dnsEntry := range scDnsConfligList {
		// HostName is required
		if dnsEntry.HostName == "" {
			return fmt.Errorf(invalidDnsEntryFormat, "hostname", dnsEntry.HostName, dnsEntry.Address)
		}

		// Address is required
		if dnsEntry.Address == "" {
			return fmt.Errorf(invalidDnsEntryFormat, "address", dnsEntry.HostName, dnsEntry.Address)
		}

		// validate the address is a valid IPv4/IPv6 address
		if err := validateAddress(dnsEntry.Address); err != nil {
			return fmt.Errorf("invalid address in the DNS config hostname=%s, address=%s: %w", dnsEntry.HostName, dnsEntry.Address, err)
		}
	}

	return nil
}

// validateAddress validates the passed address is a valid IPv4/IPv6 address.
func validateAddress(address string) error {
	if ip := net.ParseIP(address); ip == nil {
		return fmt.Errorf("address=%s is not a valid IP address", address)
	}
	return nil
}

// validateIngressConfig validates the service connect ingress config based on given network mode.
func validateIngressConfig(scIngressConfigList []IngressConfigEntry, taskNetworkMode string) error {
	// ingress config can be empty since an ECS service can only act as a client
	if len(scIngressConfigList) == 0 {
		return nil
	}

	switch taskNetworkMode {
	case BridgeNetworkMode, AWSVPCNetworkMode:
		if err := validateIngressConfigEntry(scIngressConfigList, taskNetworkMode); err != nil {
			return err
		}
	default:
		return fmt.Errorf("service connect does not support for %s newtork mode", taskNetworkMode)
	}

	return nil
}

// validateIngressConfigEntry validates the service connect ingress config entry based on given network mode.
func validateIngressConfigEntry(scIngressConfigList []IngressConfigEntry, networkMode string) error {
	interceptAndListenerPortsMap := map[uint16]bool{}
	hostPortsMap := map[uint16]bool{}
	listenerPortValue := uint16(0)
	interceptPortValue := uint16(0)
	hostPortValue := uint16(0)

	for _, entry := range scIngressConfigList {
		// show a warning message if
		// 1) a host port exists in the ingress config for awsvpc mode
		// 2) an intercept port exists in the ingress config for bridge mode
		if (entry.HostPort != nil && networkMode == AWSVPCNetworkMode) ||
			(entry.InterceptPort != nil && networkMode == BridgeNetworkMode) {
			invalidPort := "a host"
			if networkMode == BridgeNetworkMode {
				invalidPort = "an intercept"
			}
			warningMsg := fmt.Sprintf(warningIngressPortFormat, invalidPort, networkMode)
			logger.Warn(warningMsg, logger.Fields{
				"listenerName":  entry.ListenerName,
				"listenerPort":  entry.ListenerPort,
				"hostPort":      aws.Uint16Value(entry.HostPort),
				"interceptPort": aws.Uint16Value(entry.InterceptPort),
			})
		}

		// verify the intercept port for awsvpc mode
		if entry.InterceptPort != nil && networkMode == AWSVPCNetworkMode {
			interceptPortValue = aws.Uint16Value(entry.InterceptPort)
			if err := validateInterceptPort(interceptPortValue, entry.ListenerName, interceptAndListenerPortsMap); err != nil {
				return err
			}
			// Save the listener port value
			interceptAndListenerPortsMap[interceptPortValue] = true
		}

		// verify the listener port
		if entry.ListenerPort > uint16(0) {
			listenerPortValue = entry.ListenerPort
			if err := validateListenerPort(listenerPortValue, entry.ListenerName, interceptAndListenerPortsMap); err != nil {
				return err
			}
			// Save the listener port value
			interceptAndListenerPortsMap[listenerPortValue] = true
		}

		// verify the host port for bridge mode
		if entry.HostPort != nil && networkMode == BridgeNetworkMode {
			hostPortValue = aws.Uint16Value(entry.HostPort)
			if err := validateHostPort(hostPortValue, entry.ListenerName, hostPortsMap); err != nil {
				return err
			}
			// Save the host port value
			hostPortsMap[hostPortValue] = true
		}
	}

	return nil
}

// validateInterceptPort validates the intercept port is in the valid port range and does not have port collision.
func validateInterceptPort(interceptPortValue uint16, listenerName string, interceptAndListenerPortsMap map[uint16]bool) error {
	if err := validatePort(interceptPortValue); err != nil {
		return fmt.Errorf(invalidIngressPortFormat, "intercept", interceptPortValue, err)
	}

	if listenerName == "" {
		return fmt.Errorf("no listener name in the ingress config with the intercept port=%d", interceptPortValue)
	}

	if present := interceptAndListenerPortsMap[interceptPortValue]; present {
		return fmt.Errorf(portCollisionFormat, "intercept", "intercept", interceptPortValue, listenerName)
	}

	return nil
}

// validateListenerPort validates the listener port is in the valid port range and does not have port collision.
func validateListenerPort(listenerPortValue uint16, listenerName string, interceptAndListenerPortsMap map[uint16]bool) error {
	if err := validatePort(listenerPortValue); err != nil {
		return fmt.Errorf(invalidIngressPortFormat, "listener", listenerPortValue, err)
	}

	if present := interceptAndListenerPortsMap[listenerPortValue]; present {
		return fmt.Errorf(portCollisionFormat, "listener", "listener", listenerPortValue, listenerName)
	}

	return nil
}

// validateHostPort validates the host port is in the valid port range and does not have port collision.
func validateHostPort(hostPortValue uint16, listenerName string, hostPortsMap map[uint16]bool) error {
	if err := validatePort(hostPortValue); err != nil {
		return fmt.Errorf(invalidIngressPortFormat, "host", hostPortValue, err)
	}

	if present := hostPortsMap[hostPortValue]; present {
		return fmt.Errorf(portCollisionFormat, "host", "host", hostPortValue, listenerName)
	}

	return nil
}

// validatePort validates port is in valid range.
func validatePort(port uint16) error {
	// valid port range is 1~65535
	if port >= uint16(1) && port <= uint16(65535) {
		return nil
	}

	return fmt.Errorf("the port=%d is an invalid port. A valid port ranges from 1 through 65535", port)
}

// ValidateSCConfig validates service connect container name, config, egress config, and ingress config.
func ValidateServiceConnectConfig(scConfig *Config,
	taskContainers []*ecsacs.Container,
	taskNetworkMode string,
	ipv6Enabled bool) error {
	if err := validateContainerName(scConfig.ContainerName, taskContainers); err != nil {
		return err
	}

	// egress config and ingress config should not both be nil/empty
	if scConfig.EgressConfig == nil && len(scConfig.IngressConfig) == 0 {
		return fmt.Errorf("egress config and ingress config should not both be nil/empty")
	}

	if err := validateEgressConfig(scConfig.EgressConfig, ipv6Enabled); err != nil {
		return err
	}

	if err := validateDnsConfig(scConfig.DNSConfig, scConfig.EgressConfig, ipv6Enabled); err != nil {
		return err
	}

	if err := validateIngressConfig(scConfig.IngressConfig, taskNetworkMode); err != nil {
		return err
	}

	return nil
}
