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

package execcmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	assert.Equal(t, HostBinDir, m.hostBinDir)
	assert.Equal(t, defaultInspectRetryTimeout, m.inspectRetryTimeout)
	assert.Equal(t, defaultRetryMinDelay, m.retryMinDelay)
	assert.Equal(t, defaultRetryMaxDelay, m.retryMaxDelay)
	assert.Equal(t, defaultStartRetryTimeout, m.startRetryTimeout)
}

func TestNewManagerWithBinDir(t *testing.T) {
	const customHostBinDir = "/test"
	m := NewManagerWithBinDir(customHostBinDir)
	assert.Equal(t, customHostBinDir, m.hostBinDir)
	assert.Equal(t, defaultInspectRetryTimeout, m.inspectRetryTimeout)
	assert.Equal(t, defaultRetryMinDelay, m.retryMinDelay)
	assert.Equal(t, defaultRetryMaxDelay, m.retryMaxDelay)
	assert.Equal(t, defaultStartRetryTimeout, m.startRetryTimeout)
}

func TestIsExecEnabledTask(t *testing.T) {
	var tests = []struct {
		name                string
		agentName           string
		expectedExecEnabled bool
	}{
		{
			name:      "test task not exec enabled when no exec command managed agent is present",
			agentName: "randomAgent",
		},
		{
			name:                "test task not exec enabled when no exec command managed agent is present",
			agentName:           "ExecuteCommandAgent",
			expectedExecEnabled: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := &apitask.Task{
				Containers: []*apicontainer.Container{{
					ManagedAgentsUnsafe: []apicontainer.ManagedAgent{
						{
							Name: tc.agentName,
						}}},
				},
			}
			enabled := IsExecEnabledTask(task)
			assert.Equal(t, tc.expectedExecEnabled, enabled)
		})
	}
}

func TestExecEnabledContainer(t *testing.T) {

	var tests = []struct {
		name                string
		agentName           string
		expectedExecEnabled bool
	}{
		{
			name:      "test container not exec enabled when no exec command managed agent is present",
			agentName: "randomAgent",
		},
		{
			name:                "test container not exec enabled when no exec command managed agent is present",
			agentName:           "ExecuteCommandAgent",
			expectedExecEnabled: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			container := &apicontainer.Container{
				ManagedAgentsUnsafe: []apicontainer.ManagedAgent{
					{
						Name: tc.agentName,
					}},
			}
			enabled := IsExecEnabledContainer(container)
			assert.Equal(t, tc.expectedExecEnabled, enabled)
		})
	}
}
