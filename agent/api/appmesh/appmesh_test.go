//go:build unit
// +build unit

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
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

const (
	mockEgressIgnoredIP1   = "128.0.0.1"
	mockEgressIgnoredIP2   = "171.1.3.24"
	mockAppPort1           = "8000"
	mockAppPort2           = "8001"
	mockEgressIgnoredPort1 = "13000"
	mockEgressIgnoredPort2 = "13001"
	mockIgnoredUID         = "1337"
	mockIgnoredGID         = "2339"
	mockProxyIngressPort   = "9000"
	mockProxyEgressPort    = "9001"
	mockAppPorts           = mockAppPort1 + splitter + mockAppPort2
	mockEgressIgnoredIPs   = mockEgressIgnoredIP1 + splitter + mockEgressIgnoredIP2
	mockEgressIgnoredPorts = mockEgressIgnoredPort1 + splitter + mockEgressIgnoredPort2
	mockContainerName      = "testEnvoyContainer"
)

func TestAppMeshFromACS(t *testing.T) {
	testProxyConfig := prepareProxyConfig()

	appMesh, err := AppMeshFromACS(&testProxyConfig)

	assert.NoError(t, err)
	assert.NotNil(t, appMesh)
	assert.Equal(t, mockContainerName, appMesh.ContainerName)
	assert.Equal(t, mockIgnoredUID, appMesh.IgnoredUID)
	assert.Equal(t, mockIgnoredGID, appMesh.IgnoredGID)
	assert.Equal(t, mockProxyEgressPort, appMesh.ProxyEgressPort)
	assert.Equal(t, mockProxyIngressPort, appMesh.ProxyIngressPort)
	assert.Equal(t, mockAppPort1, appMesh.AppPorts[0])
	assert.Equal(t, mockAppPort2, appMesh.AppPorts[1])
	assert.Equal(t, mockEgressIgnoredIP1, appMesh.EgressIgnoredIPs[0])
	assert.Equal(t, mockEgressIgnoredIP2, appMesh.EgressIgnoredIPs[1])
	assert.Equal(t, taskMetadataEndpointIP, appMesh.EgressIgnoredIPs[2])
	assert.Equal(t, instanceMetadataEndpointIP, appMesh.EgressIgnoredIPs[3])
	assert.Equal(t, mockEgressIgnoredPort1, appMesh.EgressIgnoredPorts[0])
	assert.Equal(t, mockEgressIgnoredPort2, appMesh.EgressIgnoredPorts[1])
}

func TestAppMeshFromACSContainsDefaultEgressIgnoredIP(t *testing.T) {
	testProxyConfig := prepareProxyConfig()
	egressIgnoredIPs := mockEgressIgnoredIPs + splitter + taskMetadataEndpointIP + splitter + instanceMetadataEndpointIP
	testProxyConfig.Properties[egressIgnoredIPs] = aws.String(egressIgnoredIPs)

	appMesh, err := AppMeshFromACS(&testProxyConfig)

	assert.NoError(t, err)
	assert.NotNil(t, appMesh)
	assert.Equal(t, mockIgnoredUID, appMesh.IgnoredUID)
	assert.Equal(t, mockIgnoredGID, appMesh.IgnoredGID)
	assert.Equal(t, mockProxyEgressPort, appMesh.ProxyEgressPort)
	assert.Equal(t, mockProxyIngressPort, appMesh.ProxyIngressPort)
	assert.Equal(t, mockAppPort1, appMesh.AppPorts[0])
	assert.Equal(t, mockAppPort2, appMesh.AppPorts[1])
	assert.Equal(t, mockEgressIgnoredIP1, appMesh.EgressIgnoredIPs[0])
	assert.Equal(t, mockEgressIgnoredIP2, appMesh.EgressIgnoredIPs[1])
	assert.Equal(t, taskMetadataEndpointIP, appMesh.EgressIgnoredIPs[2])
	assert.Equal(t, instanceMetadataEndpointIP, appMesh.EgressIgnoredIPs[3])
	assert.Equal(t, mockEgressIgnoredPort1, appMesh.EgressIgnoredPorts[0])
	assert.Equal(t, mockEgressIgnoredPort2, appMesh.EgressIgnoredPorts[1])
}

func TestAppMeshFromACSNonAppMeshProxyInput(t *testing.T) {
	someOtherProxyType := "fooProxy"
	testProxyConfig := prepareProxyConfig()
	testProxyConfig.Type = &someOtherProxyType

	_, err := AppMeshFromACS(&testProxyConfig)

	assert.Error(t, err)
}

func TestAppMeshEmptyAppPorts(t *testing.T) {
	emptyAppPorts := ""
	testProxyConfig := prepareProxyConfig()
	testProxyConfig.Properties[appPorts] = &emptyAppPorts

	appMesh, err := AppMeshFromACS(&testProxyConfig)

	assert.NoError(t, err)
	assert.Equal(t, 0, len(appMesh.AppPorts))
}

func TestAppMeshEmptyIgnoredIPs(t *testing.T) {
	emptyIgnoredIPs := ""
	testProxyConfig := prepareProxyConfig()
	testProxyConfig.Properties[egressIgnoredIPs] = &emptyIgnoredIPs

	appMesh, err := AppMeshFromACS(&testProxyConfig)

	assert.NoError(t, err)
	assert.Equal(t, 2, len(appMesh.EgressIgnoredIPs))
}

func TestAppMeshEmptyIgnoredPorts(t *testing.T) {
	emptyIgnoredPorts := ""
	testProxyConfig := prepareProxyConfig()
	testProxyConfig.Properties[egressIgnoredPorts] = &emptyIgnoredPorts

	appMesh, err := AppMeshFromACS(&testProxyConfig)

	assert.NoError(t, err)
	assert.Equal(t, 0, len(appMesh.EgressIgnoredPorts))
}

func prepareProxyConfig() ecsacs.ProxyConfiguration {

	return ecsacs.ProxyConfiguration{
		Type: aws.String(appMesh),
		Properties: map[string]*string{
			ignoredUID:         aws.String(mockIgnoredUID),
			ignoredGID:         aws.String(mockIgnoredGID),
			proxyIngressPort:   aws.String(mockProxyIngressPort),
			proxyEgressPort:    aws.String(mockProxyEgressPort),
			appPorts:           aws.String(mockAppPorts),
			egressIgnoredIPs:   aws.String(mockEgressIgnoredIPs),
			egressIgnoredPorts: aws.String(mockEgressIgnoredPorts),
		},
		ContainerName: aws.String(mockContainerName),
	}
}
