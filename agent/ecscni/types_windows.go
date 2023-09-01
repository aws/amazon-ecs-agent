//go:build windows
// +build windows

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
	"time"
)

const (
	// ECSVPCENIPluginExecutable is the name of vpc-eni executable.
	ECSVPCENIPluginExecutable = "vpc-eni.exe"
	// TaskHNSNetworkNamePrefix is the prefix of the HNS network used for task ENI.
	TaskHNSNetworkNamePrefix = "task"
	// ECSBridgeNetworkName is the name of the HNS network used as ecs-bridge.
	ECSBridgeNetworkName = "nat"
	// Starting with CNI plugin v0.8.0 (this PR https://github.com/containernetworking/cni/pull/698)
	// NetworkName has to be non-empty field for network config.
	// We do not actually make use of the field, hence passing in a placeholder string to fulfill the API spec
	defaultNetworkName = "network-name"
	// DefaultENIName is the name of eni interface name in the container namespace
	DefaultENIName = "eth0"
)

var (
	// Values for creating backoff while retrying setupNS.
	// These have not been made constant so that we can inject different values for unit tests.
	setupNSBackoffMin      = time.Second * 4
	setupNSBackoffMax      = time.Minute
	setupNSBackoffJitter   = 0.2
	setupNSBackoffMultiple = 2.0
	setupNSMaxRetryCount   = 5
)
