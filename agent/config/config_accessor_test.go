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

package config

import (
	"testing"

	ecsagentconfig "github.com/aws/amazon-ecs-agent/ecs-agent/config"
	"github.com/stretchr/testify/assert"
)

func TestAgentConfigAccessorErrorsWhenConfigIsNil(t *testing.T) {
	configAccessor, err := NewAgentConfigAccessor(nil)
	assert.Error(t, err)
	assert.Nil(t, configAccessor)
}

func TestAgentConfigAccessorImplements(t *testing.T) {
	configAccessor, err := NewAgentConfigAccessor(&Config{})
	assert.NoError(t, err)
	assert.Implements(t, (*ecsagentconfig.AgentConfigAccessor)(nil), configAccessor)
}

func TestAgentConfigAccessorMethods(t *testing.T) {
	cfg := &Config{
		AcceptInsecureCert: true,
		APIEndpoint:        "https://some-endpoint.com",
		AWSRegion:          "us-east-1",
		Cluster:            "myCluster",
		External:           BooleanDefaultFalse{Value: ExplicitlyEnabled},
		InstanceAttributes: map[string]string{"attribute1": "value1"},
		NoIID:              true,
		ReservedMemory:     uint16(20),
		ReservedPorts:      []uint16{22, 2375, 2376, 51678},
		ReservedPortsUDP:   []uint16{},
	}

	configAccessor, err := NewAgentConfigAccessor(cfg)
	assert.NoError(t, err)
	assert.Equal(t, cfg.AcceptInsecureCert, configAccessor.AcceptInsecureCert())
	assert.Equal(t, cfg.APIEndpoint, configAccessor.APIEndpoint())
	assert.Equal(t, cfg.AWSRegion, configAccessor.AWSRegion())
	assert.Equal(t, cfg.Cluster, configAccessor.Cluster())
	assert.Equal(t, DefaultClusterName, configAccessor.DefaultClusterName())
	assert.Equal(t, cfg.External.Enabled(), configAccessor.External())
	assert.Equal(t, cfg.NoIID, configAccessor.NoInstanceIdentityDocument())
	assert.Equal(t, GetOSFamily(), configAccessor.OSFamily())
	assert.Equal(t, OSType, configAccessor.OSType())
	assert.Equal(t, cfg.ReservedMemory, configAccessor.ReservedMemory())
	assert.Equal(t, cfg.ReservedPorts, configAccessor.ReservedPorts())
	assert.Equal(t, cfg.ReservedPortsUDP, configAccessor.ReservedPortsUDP())
	newCluster := "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"
	configAccessor.UpdateCluster(newCluster)
	assert.Equal(t, newCluster, cfg.Cluster)
	assert.Equal(t, newCluster, configAccessor.Cluster())
	assert.Equal(t, cfg.Cluster, configAccessor.Cluster())
}
