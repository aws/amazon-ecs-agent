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
	"encoding/json"
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	serviceConnectConfigKey        = "ServiceConnectConfig"
	serviceConnectContainerNameKey = "ContainerName"
)

// ServiceConnectConfig represents the Service Connect configuration for a task.
type ServiceConnectConfig struct {
	ContainerName string               `json:"containerName"`
	IngressConfig []IngressConfigEntry `json:"ingressConfig,omitempty"`
	EgressConfig  *EgressConfig        `json:"egressConfig,omitempty"`
	DNSConfig     []DNSConfigEntry     `json:"dnsConfig,omitempty"`

	// Admin configuration for operating with AppNet Agent
	RuntimeConfig RuntimeConfig `json:"runtimeConfig"`
}

// RuntimeConfig contains the runtime information for administering AppNet Agent
type RuntimeConfig struct {
	// Host path for the administration socket
	AdminSocketPath string `json:"adminSocketPath"`
	// HTTP Path + Params to get statistical information
	StatsRequest string `json:"statsRequest"`
	// HTTP Path + Params to drain ServiceConnect connections
	DrainRequest string `json:"drainRequest"`
}

// IngressConfigEntry is the ingress configuration for a given SC service.
type IngressConfigEntry struct {
	// ListenerName is the name of the listener for an SC service.
	ListenerName string `json:"listenerName"`
	// ListenerPort is the port where Envoy listens for ingress traffic for a given SC service.
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
	IPV4CIDR string `json:"ipv4Cidr,omitempty"`
	IPV6CIDR string `json:"ipv6Cidr,omitempty"`
}

// DNSConfigEntry represents a mapping between a VIP in the SC VIP-CIDR and an upstream SC service.
// e.g. DummySCService.my.corp -> 169.254.1.1
type DNSConfigEntry struct {
	HostName string `json:"hostName"`
	Address  string `json:"address"`
}

// ParseServiceConnectAttachment parses the service connect container name and service connect config value
// from the given attachment.
func ParseServiceConnectAttachment(scAttachment *ecsacs.Attachment, networkMode string, ipv6Enabled bool) (*ServiceConnectConfig, error) {
	scConfigValue := &ServiceConnectConfig{}
	containerName := ""

	for _, property := range scAttachment.AttachmentProperties {
		switch aws.StringValue(property.Name) {
		case serviceConnectConfigKey:
			// extract service connect config value from the attachment property,
			// and translate the attachment property value to ServiceConnectConfig
			data := aws.StringValue(property.Value)
			if err := json.Unmarshal([]byte(data), scConfigValue); err != nil {
				return nil, fmt.Errorf("failed to unmarshal service connect attachment property value: %w", err)
			}
		case serviceConnectContainerNameKey:
			// extract service connect container name from the attachment property
			containerName = aws.StringValue(property.Value)
		default:
			logger.Warn("Received an unrecognized attachment property", logger.Fields{
				"attachmentProperty": property.String(),
			})
		}
	}

	scConfigValue.ContainerName = containerName

	return scConfigValue, nil
}
