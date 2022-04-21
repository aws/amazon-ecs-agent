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

// ServiceConnectConfig represents the Service Connect configuration for a task.
type ServiceConnectConfig struct {
	ContainerName string               `json:"containerName"`
	StatsConfig   StatsConfig          `json:"statsConfig"`
	IngressConfig []IngressConfigEntry `json:"ingressConfig"`
	EgressConfig  EgressConfig         `json:"egressConfig"`
	DNSConfig     DNSConfig            `json:"dnsConfig"`
}

// StatsConfig is the endpoint where SC stats can be retrieved from.
type StatsConfig struct {
	UrlPath string `json:"urlPath,omitempty"`
	Port    uint16 `json:"port,omitempty"`
}

// IngressConfigEntry is the ingress configuration for a given SC service.
type IngressConfigEntry struct {
	// ListenerName is the name of the listener for an SC service.
	ListenerName string `json:"listenerName"`
	// ListenerPort is the port where Envoy listens for ingress traffic for a given  SC service.
	ListenerPort uint16 `json:"listenerPort"`
	// InterceptPort is only relevant for awsvpc mode. If present, SC CNI Plugin will configure netfilter rules to redirect
	// traffic destined to this port to ListenerPort.
	InterceptPort *uint16 `json:"interceptPort,omitempty"`
	// HostPort is only relevant for bridge network mode non-default case, where SC ingress host port is predefined in
	// SC Service creation/modification time.
	HostPort *uint16 `json:"hostPort,omitempty"`
}

// EgressConfig is the egress configuration for a given SC service.
type EgressConfig struct {
	// ListenerName is the name of the listener for SC service with name ServiceName.
	ListenerName string `json:"listenerName"`
	// EgressPort represent the port number Envoy will bind to. This port is selected at random by ECS Agent during
	// task startup. Port will be in the ephemeral range.
	ListenerPort uint16 `json:"listenerPort,omitempty"`
	// VIP is the representation of an SC VIP-CIDR
	VIP VIP `json:"vip"`
}

// VIP is the representation of an SC VIP-CIDR
// e.g. 169.254.0.0/16
type VIP struct {
	IPV4 string `json:"ipv4,omitempty"`
	IPV6 string `json:"ipv6,omitempty"`
}

// DNSConfig is a list of DNSConfigEntry's
type DNSConfig []DNSConfigEntry

// DNSConfigEntry represents a mapping between a VIP in the SC VIP-CIDR and an upstream SC service.
// e.g. DummySCService.corp.local -> 169.254.1.1
type DNSConfigEntry struct {
	HostName    string `json:"hostName"`
	IPV4Address string `json:"ipv4Address,omitempty"`
	IPV6Address string `json:"ipv6Address,omitempty"`
}
