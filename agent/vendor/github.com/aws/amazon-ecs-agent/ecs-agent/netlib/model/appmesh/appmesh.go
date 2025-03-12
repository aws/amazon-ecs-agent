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

package appmesh

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acs/types"
)

const (
	appMesh                    = types.ProxyConfigurationTypeAppmesh
	splitter                   = ","
	ignoredUID                 = "IgnoredUID"
	ignoredGID                 = "IgnoredGID"
	proxyIngressPort           = "ProxyIngressPort"
	proxyEgressPort            = "ProxyEgressPort"
	appPorts                   = "AppPorts"
	egressIgnoredIPs           = "EgressIgnoredIPs"
	egressIgnoredPorts         = "EgressIgnoredPorts"
	taskMetadataEndpointIP     = "169.254.170.2"
	instanceMetadataEndpointIP = "169.254.169.254"
)

// AppMesh contains information of app mesh config
type AppMesh struct {
	// ContainerName is the proxy container name
	ContainerName string
	// IgnoredUID is egress traffic from the processes owned by the UID will be ignored
	IgnoredUID string
	// IgnoredGID specifies egress traffic from the processes owned by the GID will be ignored
	IgnoredGID string
	// ProxyIngressPort is the ingress port number that proxy is listening on
	ProxyIngressPort string
	// ProxyEgressPort is the egress port number that proxy is listening on
	ProxyEgressPort string
	// AppPorts is the port number that application is listening on
	AppPorts []string
	// EgressIgnoredIPs is the list of IPs for which egress traffic will be ignored
	EgressIgnoredIPs []string
	// EgressIgnoredPorts is the list of ports for which egress traffic will be ignored
	EgressIgnoredPorts []string
}

// AppMeshFromACS validates proxy config if it is app mesh type and creates AppMesh object
func AppMeshFromACS(proxyConfig *types.ProxyConfiguration) (*AppMesh, error) {

	if proxyConfig.Type != appMesh {
		return nil, fmt.Errorf("agent does not support proxy type other than app mesh")
	}

	return &AppMesh{
		ContainerName:      aws.ToString(proxyConfig.ContainerName),
		IgnoredUID:         proxyConfig.Properties[ignoredUID],
		IgnoredGID:         proxyConfig.Properties[ignoredGID],
		ProxyIngressPort:   proxyConfig.Properties[proxyIngressPort],
		ProxyEgressPort:    proxyConfig.Properties[proxyEgressPort],
		AppPorts:           buildAppPorts(proxyConfig),
		EgressIgnoredIPs:   buildEgressIgnoredIPs(proxyConfig),
		EgressIgnoredPorts: buildEgressIgnoredPorts(proxyConfig),
	}, nil
}

// buildAppPorts creates app ports from proxy config
func buildAppPorts(proxyConfig *types.ProxyConfiguration) []string {
	var inputAppPorts []string
	if appPortsValue, exists := proxyConfig.Properties[appPorts]; exists && appPortsValue != "" {
		inputAppPorts = strings.Split(appPortsValue, splitter)
	}
	return inputAppPorts
}

// buildEgressIgnoredIPs creates egress ignored IPs from proxy config
func buildEgressIgnoredIPs(proxyConfig *types.ProxyConfiguration) []string {
	var inputEgressIgnoredIPs []string
	if egressIgnoredIPsValues, exists := proxyConfig.Properties[egressIgnoredIPs]; exists && egressIgnoredIPsValues != "" {
		inputEgressIgnoredIPs = strings.Split(egressIgnoredIPsValues, splitter)
	}
	// append agent default egress ignored IPs
	return appendDefaultEgressIgnoredIPs(inputEgressIgnoredIPs)
}

// buildEgressIgnoredPorts creates egress ignored ports from proxy config
func buildEgressIgnoredPorts(proxyConfig *types.ProxyConfiguration) []string {
	var inputEgressIgnoredPorts []string
	if egressIgnoredPortsValues, exists := proxyConfig.Properties[egressIgnoredPorts]; exists && egressIgnoredPortsValues != "" {
		inputEgressIgnoredPorts = strings.Split(egressIgnoredPortsValues, splitter)
	}
	return inputEgressIgnoredPorts
}

// appendDefaultEgressIgnoredIPs append task metadata endpoint ip and
// instance metadata ip address to egress ignored IPs if does not exist
func appendDefaultEgressIgnoredIPs(egressIgnoredIPs []string) []string {
	hasTaskMetadataEndpointIP := false
	hasInstanceMetadataEndpointIP := false
	for _, egressIgnoredIP := range egressIgnoredIPs {
		if strings.TrimSpace(egressIgnoredIP) == taskMetadataEndpointIP {
			hasTaskMetadataEndpointIP = true
		}
		if strings.TrimSpace(egressIgnoredIP) == instanceMetadataEndpointIP {
			hasInstanceMetadataEndpointIP = true
		}
	}

	if !hasTaskMetadataEndpointIP {
		egressIgnoredIPs = append(egressIgnoredIPs, taskMetadataEndpointIP)
	}
	if !hasInstanceMetadataEndpointIP {
		egressIgnoredIPs = append(egressIgnoredIPs, instanceMetadataEndpointIP)
	}

	return egressIgnoredIPs
}
