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

package tasknetworkconfig

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/stretchr/testify/assert"

	"testing"
)

func TestTaskNetworkConfig_GetPrimaryInterface(t *testing.T) {
	testNetConfig := getTestTaskNetworkConfig()
	assert.Equal(t, primaryInterfaceName, testNetConfig.GetPrimaryInterface().Name)

	testNetConfig = &TaskNetworkConfig{
		NetworkNamespaces: []*NetworkNamespace{},
	}
	assert.Nil(t, testNetConfig.GetPrimaryInterface())
}

func TestTaskNetworkConfig_GetPrimaryNetNS(t *testing.T) {
	testNetConfig := getTestTaskNetworkConfig()
	assert.Equal(t, primaryNetNSName, testNetConfig.GetPrimaryNetNS().Name)

	testNetConfig = &TaskNetworkConfig{}
	assert.Nil(t, testNetConfig.GetPrimaryNetNS())
}

// TestNewTaskNetConfig tests creation of TaskNetworkConfig out of
// a given set of NetworkNamespace objects.
func TestNewTaskNetConfig(t *testing.T) {
	protos := []string{
		ecs.NetworkModeAwsvpc,
		ecs.NetworkModeHost,
		ecs.NetworkModeBridge,
		ecs.NetworkModeNone,
	}
	for _, proto := range protos {
		_, err := New(proto, nil)
		assert.NoError(t, err)
	}

	_, err := New("invalid-protocol", nil)
	assert.Error(t, err)

	primaryNetNS := "primary-netns"
	secondaryNetNS := "secondary-netns"
	netNSs := []*NetworkNamespace{
		{
			Name:  primaryNetNS,
			Index: 0,
		},
		{
			Name:  secondaryNetNS,
			Index: 1,
		},
	}

	taskNetConfig, err := New(
		ecs.NetworkModeAwsvpc,
		netNSs...)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(taskNetConfig.NetworkNamespaces))
	assert.Equal(t, *netNSs[0], *taskNetConfig.NetworkNamespaces[0])
	assert.Equal(t, *netNSs[1], *taskNetConfig.NetworkNamespaces[1])
}
