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

package handlers

import (
	"testing"

	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	"github.com/stretchr/testify/assert"
)

func TestValidateTaskNetworkConfig(t *testing.T) {
	testCases := []struct {
		name          string
		config        *state.TaskNetworkConfig
		expectedError string
	}{
		{
			name:          "nil config",
			config:        nil,
			expectedError: "TaskNetworkConfig is empty within task metadata",
		},
		{
			name:          "empty network namespaces",
			config:        &state.TaskNetworkConfig{},
			expectedError: "empty network namespaces within task network config",
		},
		{
			name: "empty network namespace path",
			config: &state.TaskNetworkConfig{
				NetworkNamespaces: []*state.NetworkNamespace{{}},
			},
			expectedError: "no path in the network namespace within task network config",
		},
		{
			name: "empty network interfaces",
			config: &state.TaskNetworkConfig{
				NetworkNamespaces: []*state.NetworkNamespace{
					{
						Path: "/proc/1234/ns/net",
					},
				},
			},
			expectedError: "empty network interfaces within task network config",
		},
		{
			name: "first interface missing device name",
			config: &state.TaskNetworkConfig{
				NetworkNamespaces: []*state.NetworkNamespace{
					{
						Path: "/proc/1234/ns/net",
						NetworkInterfaces: []*state.NetworkInterface{
							{},
							{
								DeviceName: "eth1",
							},
						},
					},
				},
			},
			expectedError: "no ENI device name for network interface 0 in the network namespace within task network config",
		},
		{
			name: "second interface missing device name",
			config: &state.TaskNetworkConfig{
				NetworkNamespaces: []*state.NetworkNamespace{
					{
						Path: "/proc/1234/ns/net",
						NetworkInterfaces: []*state.NetworkInterface{
							{
								DeviceName: "eth0",
							},
							{},
						},
					},
				},
			},
			expectedError: "no ENI device name for network interface 1 in the network namespace within task network config",
		},
		{
			name: "valid config with single interface",
			config: &state.TaskNetworkConfig{
				NetworkNamespaces: []*state.NetworkNamespace{
					{
						Path: "/proc/1234/ns/net",
						NetworkInterfaces: []*state.NetworkInterface{
							{
								DeviceName: "eth0",
							},
						},
					},
				},
			},
			expectedError: "",
		},
		{
			name: "valid config with multiple interfaces",
			config: &state.TaskNetworkConfig{
				NetworkNamespaces: []*state.NetworkNamespace{
					{
						Path: "/proc/1234/ns/net",
						NetworkInterfaces: []*state.NetworkInterface{
							{
								DeviceName: "eth0",
							},
							{
								DeviceName: "eth1",
							},
						},
					},
				},
			},
			expectedError: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTaskNetworkConfig(tc.config)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}
