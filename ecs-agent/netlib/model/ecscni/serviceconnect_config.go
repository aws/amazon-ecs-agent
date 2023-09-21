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
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
)

const redirectModeNat string = "nat"

type ServiceConnectCNIConfig struct {
	CNIConfig
	// IngressConfig (optional) specifies the netfilter rules to be set for incoming requests.
	IngressConfig []IngressConfig `json:"ingressConfig,omitempty"`
	// EgressConfig (optional) specifies the netfilter rules to be set for outgoing requests.
	EgressConfig EgressConfig `json:"egressConfig,omitempty"`
	// EnableIPv4 (optional) specifies whether to set the rules in IPv4 table. Note that this.
	EnableIPv4 bool `json:"enableIPv4,omitempty"`
	// EnableIPv6 (optional) specifies whether to set the rules in IPv6 table. Default value is false.
	EnableIPv6 bool `json:"enableIPv6,omitempty"`
}

// IngressConfig defines the ingress network config in JSON format for the ecs-serviceconnect CNI plugin.
type IngressConfig struct {
	ListenerPort  int64 `json:"listenerPort"`
	InterceptPort int64 `json:"interceptPort,omitempty"`
}

// EgressConfig defines the egress network config in JSON format for the ecs-serviceconnect CNI plugin.
type EgressConfig struct {
	ListenerPort int64     `json:"listenerPort"`
	VIP          vipConfig `json:"vip"`
	// RedirectMode dictates what mechanism the plugin should use for redirecting egress traffic.
	// For awsvpc mode the value is "nat" always.
	RedirectMode string `json:"redirectMode"`
}

// vipConfig defines the EgressVIP network config in JSON format for the ecs-serviceconnect CNI plugin.
type vipConfig struct {
	IPv4CIDR string `json:"ipv4Cidr,omitempty"`
	IPv6CIDR string `json:"ipv6Cidr,omitempty"`
}

func NewServiceConnectCNIConfig(
	cniConfig CNIConfig,
	scConfig *serviceconnect.ServiceConnectConfig,
	enableIPV4 bool,
	enableIPV6 bool,
) *ServiceConnectCNIConfig {
	var cniIngress []IngressConfig
	for _, scIngress := range scConfig.IngressConfigList {
		cniIngress = append(cniIngress, IngressConfig{
			ListenerPort:  scIngress.ListenerPort,
			InterceptPort: scIngress.InterceptPort,
		})
	}

	var vip vipConfig
	if scConfig.EgressConfig.ListenerName != "" {
		vip.IPv6CIDR = scConfig.EgressConfig.IPV6CIDR
		vip.IPv4CIDR = scConfig.EgressConfig.IPV4CIDR
	}

	return &ServiceConnectCNIConfig{
		CNIConfig:     cniConfig,
		IngressConfig: cniIngress,
		EgressConfig: EgressConfig{
			ListenerPort: scConfig.EgressConfig.ListenerPort,
			VIP:          vip,
			RedirectMode: redirectModeNat,
		},
		EnableIPv4: enableIPV4,
		EnableIPv6: enableIPV6,
	}
}

func (sc *ServiceConnectCNIConfig) String() string {
	return fmt.Sprintf("%s, ingressConfig: %v, egressConfig: %v, enableIPv4: %v, enableIPv6: %v",
		sc.CNIConfig.String(),
		sc.IngressConfig,
		sc.EgressConfig,
		sc.EnableIPv4,
		sc.EnableIPv6)
}

func (sc *ServiceConnectCNIConfig) InterfaceName() string {
	// Not required for service connect plugin as no particular interface is set up in this
	// plugin. The plugin sets up some iptables filters, that's all. However, CNI requires
	// us to set it with a non-empty string.
	return "eth0"
}

func (sc *ServiceConnectCNIConfig) NSPath() string {
	return sc.NetNSPath
}

func (sc *ServiceConnectCNIConfig) PluginName() string {
	return sc.CNIPluginName
}

func (sc *ServiceConnectCNIConfig) CNIVersion() string {
	return sc.CNISpecVersion
}
