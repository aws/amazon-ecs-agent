// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	appMesh            = "APPMESH"
	splitter           = ","
	ignoredUID         = "IgnoredUID"
	ignoredGID         = "IgnoredGID"
	proxyIngressPort   = "ProxyIngressPort"
	proxyEgressPort    = "ProxyEgressPort"
	appPorts           = "AppPorts"
	egressIgnoredIPs   = "EgressIgnoredIPs"
	egressIgnoredPorts = "EgressIgnoredPorts"
)

// AppMesh contains information of app mesh config
type AppMesh struct {
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
	// EgressIgnoredIPs is the list of ports for which egress traffic will be ignored
	EgressIgnoredIPs []string
	// EgressIgnoredPorts is the list of IPs for which egress traffic will be ignored
	EgressIgnoredPorts []string
}

// AppMeshFromACS validates proxy config if it is app mesh type and creates AppMesh object
func AppMeshFromACS(proxyConfig *ecsacs.ProxyConfiguration) (*AppMesh, error) {

	if *proxyConfig.Type != appMesh {
		return nil, fmt.Errorf("agent does not support proxy type other than app mesh")
	}

	var inputAppPorts []string
	var inputEgressIgnoredIPs []string
	var inputEgressIgnoredPorts []string

	if proxyConfig.Properties[appPorts] != nil {
		inputAppPorts = strings.Split(*proxyConfig.Properties[appPorts], splitter)
	}
	if proxyConfig.Properties[egressIgnoredIPs] != nil {
		inputEgressIgnoredIPs = strings.Split(*proxyConfig.Properties[egressIgnoredIPs], splitter)
	}
	if proxyConfig.Properties[egressIgnoredPorts] != nil {
		inputEgressIgnoredPorts = strings.Split(*proxyConfig.Properties[egressIgnoredPorts], splitter)
	}

	return &AppMesh{
		IgnoredUID:         aws.StringValue(proxyConfig.Properties[ignoredUID]),
		IgnoredGID:         aws.StringValue(proxyConfig.Properties[ignoredGID]),
		ProxyIngressPort:   aws.StringValue(proxyConfig.Properties[proxyIngressPort]),
		ProxyEgressPort:    aws.StringValue(proxyConfig.Properties[proxyEgressPort]),
		AppPorts:           inputAppPorts,
		EgressIgnoredIPs:   inputEgressIgnoredIPs,
		EgressIgnoredPorts: inputEgressIgnoredPorts,
	}, nil
}
