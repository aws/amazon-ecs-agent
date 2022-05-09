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

package task

import (
	"fmt"
	"net"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/aws-sdk-go/aws"
)

// validateContainerName validates the service connect container name.
func validateContainerName(scContainerName string, taskContainers []*ecsacs.Container) error {
	// service connect container name is a required field
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
		return fmt.Errorf("found %d duplicate service connect container name=%s exists in the task", numOfFoundSCContainer, scContainerName)
	}

	return nil
}

// validateEgressConfig validates the service connect egress config.
func validateEgressConfig(scEgressConfig *EgressConfig, ipv6Enabled bool) error {
	// egress config can be empty for the first service since there are no other tasks that it can talk to
	if scEgressConfig == nil {
		return nil
	}

	// ListenerName is a required field if the egress config is not empty
	if scEgressConfig.ListenerName == "" {
		return fmt.Errorf("no service connect listener name")
	}

	// VIP is a required field if the egress config is not empty
	if !ipv6Enabled && scEgressConfig.VIP.IPV4CIDR == "" {
		return fmt.Errorf("no service connect VIP IPV4CIDR")
	}

	if ipv6Enabled && scEgressConfig.VIP.IPV6CIDR == "" {
		return fmt.Errorf("no service connect VIP IPV6CIDR. IPV6CIDR must not be empty when the task is IPV6 enabled")
	}

	// validate IPV4CIDR if it exists
	if scEgressConfig.VIP.IPV4CIDR != "" {
		trimmedIpv4cidr := strings.TrimSpace(scEgressConfig.VIP.IPV4CIDR)
		if err := validateCIDR(trimmedIpv4cidr, "IPv4 CIDR"); err != nil {
			return err
		}
	}

	// validate IPV6CIDR if it exists
	if scEgressConfig.VIP.IPV6CIDR != "" {
		trimmedIpv6cidr := strings.TrimSpace(scEgressConfig.VIP.IPV6CIDR)
		if err := validateCIDR(trimmedIpv6cidr, "IPv6 CIDR"); err != nil {
			return err
		}
	}

	return nil
}

// validateDnsConfig validates the service connnect DNS config.
func validateDnsConfig(scDnsConfligList []DNSConfigEntry, scEgressConfig *EgressConfig) error {
	if len(scDnsConfligList) == 0 && scEgressConfig != nil {
		// DNS config is a field which associates to egress config
		return fmt.Errorf("no service connect DNS config. The DNS config is required when the egress config is not empty")
	}

	for _, dnsEntry := range scDnsConfligList {
		// HostName is a required field
		if dnsEntry.HostName == "" {
			return fmt.Errorf("no hostname in the DNS config entry hostname=%s, address=%s", dnsEntry.HostName, dnsEntry.Address)
		}

		// Address is a required field
		if dnsEntry.Address == "" {
			return fmt.Errorf("no address in the DNS config entry hostname=%s, address=%s", dnsEntry.HostName, dnsEntry.Address)
		}

		// validate the address is a valid IPv4/IPv6 address
		if err := validateAddress(dnsEntry.Address); err != nil {
			return fmt.Errorf("invalid address in the DNS config entry hostname=%s, address=%s: %w", dnsEntry.HostName, dnsEntry.Address, err)
		}
	}

	return nil
}

// validateAddress validates the passed address is a valid IPv4/IPv6.
func validateAddress(address string) error {
	if ip := net.ParseIP(address); ip == nil {
		return fmt.Errorf("address=%s is not a valid textual representation of an IP address", address)
	}
	return nil
}

// validateCIDR validates the passed CIDR is a valid IPv4/IPv6 CIDR.
func validateCIDR(cidr, version string) error {
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return fmt.Errorf("cidr=%s is not a valid %s: %w", cidr, version, err)
	}
	return nil
}

// validateIngressConfig validates the service connect ingress config based on network mode.
func validateIngressConfig(scIngressConfigList []IngressConfigEntry, taskNetworkMode string) error {
	// ingress config can be empty since an ECS service can only act as a client
	if len(scIngressConfigList) == 0 {
		return nil
	}

	// AWSVPC network mode
	switch taskNetworkMode {
	case BridgeNetworkMode:
		if err := validateBridgeIngressConfig(scIngressConfigList); err != nil {
			return err
		}
	case AWSVPCNetworkMode:
		if err := validateAwsVpcIngressConfig(scIngressConfigList); err != nil {
			return err
		}
	default:
		return fmt.Errorf("service connect does not support for %s newtork mode", taskNetworkMode)
	}

	return nil
}

// validateAwsVpcIngressConfig validates the service connect ingress config for awsvpc network mode.
func validateAwsVpcIngressConfig(scIngressConfigList []IngressConfigEntry) error {
	portsMap := map[uint16]bool{}
	interceptPortValue := uint16(0)
	listenerPortValue := uint16(0)

	for _, entry := range scIngressConfigList {
		// show a warning message if a host port exists in the ingress config for awsvpc mode
		if entry.HostPort != nil {
			logger.Warn("Service connect config validation: a host port should not exist for awsvpc mode", logger.Fields{
				"listenerName":  entry.ListenerName,
				"listenerPort":  entry.ListenerPort,
				"hostPort":      aws.Uint16Value(entry.HostPort),
				"interceptPort": aws.Uint16Value(entry.InterceptPort),
			})
		}

		// verify the intercept port
		if entry.InterceptPort != nil {
			interceptPortValue = aws.Uint16Value(entry.InterceptPort)
			if err := validatePort(interceptPortValue); err != nil {
				return fmt.Errorf("the intercept port=%d in the ingress config entry is not valid: %w", interceptPortValue, err)
			}

			if entry.ListenerName == "" {
				return fmt.Errorf("missing listener name in the ingress config entry with the intercept port=%d", interceptPortValue)
			}

			if present := portsMap[interceptPortValue]; present {
				return fmt.Errorf("intercept port collision detected in the ingress config entry with the intercept port=%d, and listener name=%s", interceptPortValue, entry.ListenerName)
			}
			// Save the intercept port value
			portsMap[interceptPortValue] = true
		}

		// verify the listener port
		if entry.ListenerPort > uint16(0) {
			listenerPortValue = entry.ListenerPort
			if err := validatePort(listenerPortValue); err != nil {
				return fmt.Errorf("the listener port=%d in the ingress config entry is not valid: %w", listenerPortValue, err)
			}

			if present := portsMap[listenerPortValue]; present {
				return fmt.Errorf("listener port collision detected in the ingress config entry with the listener port=%d, and listener name=%s", listenerPortValue, entry.ListenerName)
			}
			// Save the listener port value
			portsMap[listenerPortValue] = true
		}
	}

	return nil
}

// validateBridgeIngressConfig validates the service connect ingress config for bridge network mode.
func validateBridgeIngressConfig(scIngressConfigList []IngressConfigEntry) error {
	hostPortsMap := map[uint16]bool{}
	hostPortValue := uint16(0)
	listenerPortsMap := map[uint16]bool{}
	listenerPortValue := uint16(0)

	for _, entry := range scIngressConfigList {
		// show a warnning messgae if an intercept port exists in the ingress config for bridge mode
		if entry.InterceptPort != nil {
			logger.Warn("Service connect config validation: an intercept port should not exist for bridge mode", logger.Fields{
				"listenerName":  entry.ListenerName,
				"listenerPort":  entry.ListenerPort,
				"hostPort":      aws.Uint16Value(entry.HostPort),
				"interceptPort": aws.Uint16Value(entry.InterceptPort),
			})
		}

		// verify the host port
		if entry.HostPort != nil {
			hostPortValue = aws.Uint16Value(entry.HostPort)
			if err := validatePort(hostPortValue); err != nil {
				return fmt.Errorf("the host port=%d in the ingress config entry is not valid: %w", hostPortValue, err)
			}

			if present := hostPortsMap[hostPortValue]; present {
				return fmt.Errorf("host port collision detected in the ingress config entry with the host port=%d, and listener name=%s", hostPortValue, entry.ListenerName)
			}
			// Save the host port value
			hostPortsMap[hostPortValue] = true
		}

		// verify the listener port
		if entry.ListenerPort > uint16(0) {
			listenerPortValue = entry.ListenerPort
			if err := validatePort(listenerPortValue); err != nil {
				return fmt.Errorf("the listener port=%d in the ingress config entry is not valid: %w", listenerPortValue, err)
			}

			if present := listenerPortsMap[listenerPortValue]; present {
				return fmt.Errorf("listener port collision detected in the ingress config entry with the listener port=%d, and listener name=%s", listenerPortValue, entry.ListenerName)
			}
			// Save the listener port value
			listenerPortsMap[listenerPortValue] = true
		}
	}

	return nil
}

//validatePort validates port is in valid range.
func validatePort(port uint16) error {
	// valid port range is 1~65535
	if port >= uint16(1) && port <= uint16(65535) {
		return nil
	}

	return fmt.Errorf("The port=%d is an invalid port. A valid port ranges from 1 through 65535", port)
}

// ValidateSCConfig validates service connect container name, config, egress config, and ingress config.
func ValidateServiceConnectConfig(scConfig *ServiceConnectConfig, taskContainers []*ecsacs.Container, taskNetworkMode string, ipv6Enabled bool) error {
	// container name is a required field
	if err := validateContainerName(scConfig.ContainerName, taskContainers); err != nil {
		return err
	}

	// egress config is a field that can be empty
	if err := validateEgressConfig(scConfig.EgressConfig, ipv6Enabled); err != nil {
		return err
	}

	// dns config is a field which associates to egress config
	if err := validateDnsConfig(scConfig.DNSConfig, scConfig.EgressConfig); err != nil {
		return err
	}

	// ingress config is a field that can be empty
	if err := validateIngressConfig(scConfig.IngressConfig, taskNetworkMode); err != nil {
		return err
	}

	return nil
}
