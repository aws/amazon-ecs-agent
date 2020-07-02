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

import "github.com/containernetworking/cni/pkg/types"

const (
	// ECSVPCSharedENIPluginName is the name of the vpc-shared-eni plugin
	ECSVPCSharedENIPluginName = "vpc-shared-eni"
	// ECSVPCSharedENIPluginExecutable is the name of vpc-shared-eni executable
	ECSVPCSharedENIPluginExecutable = "vpc-shared-eni.exe"
	// TaskENIBridgeNetworkPrefix is the prefix added to the Task ENI bridge network name by the CNI plugin
	TaskENIBridgeNetworkPrefix = "task"
)

// TaskENIConfig defines the Task Networking specific data required by the plugin
type TaskENIConfig struct {
	PauseContainer bool `json:"pauseContainer"`
}

// BridgeForTaskENIConfig contains all the information to invoke the vpc-shared-eni plugin
// This config is used to invoke plugin for setting up the Task ENI in task compartment
type BridgeForTaskENIConfig struct {
	// Type is the cni plugin name
	Type string `json:"type,omitempty"`
	// CNIVersion is the cni spec version to use
	CNIVersion string `json:"cniVersion,omitempty"`
	// DNS is used to pass DNS information to the plugin
	DNS types.DNS `json:"dns"`

	// ENIName is the name/id of the ENI
	ENIName string `json:"eniName"`
	// ENIMACAddress is the MAC address of the eni
	ENIMACAddress string `json:"eniMACAddress"`
	// ENIIPAddress is the is the ipv4 of eni
	ENIIPAddress string `json:"eniIPAddress"`
	// GatewayIPAddress specifies the IPv4 address of the subnet gateway for the ENI
	GatewayIPAddress string `json:"gatewayIPAddress"`
	// IPAddress which needs to be allocated to pause container endpoint
	IPAddress string `json:"ipAddress"`
	// TaskENIConfig provides the Task ENI specific information to the plugin
	// For now, we will use it to signal plugin to delete network on invokation of DEL
	TaskENIConfig TaskENIConfig `json:"taskENIConfig"`
}
