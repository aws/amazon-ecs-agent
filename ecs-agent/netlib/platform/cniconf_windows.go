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

//go:build windows
// +build windows

package platform

import (
	"os"
	"path/filepath"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"github.com/containernetworking/cni/pkg/types"
)

const (
	VPCENIPluginName = "vpc-eni.exe"
	// taskNetworkNamePrefix is the prefix of the HNS network used for task ENI.
	taskNetworkNamePrefix = "task"
	// fargateBridgeNetworkName is the name of the HNS network which is used for task IAM roles endpoint.
	fargateBridgeNetworkName = "fargate-bridge"

	// ENISetupTimeout is the maximum duration that ENI manager waits before aborting ENI setup.
	// The reasoning for this timeout duration is explained below with nsSetupTimeoutDuration.
	ENISetupTimeout = 3 * time.Minute
	// nsCleanupTimeoutDuration is irrelevant for warmpool instances since they are recycled after each
	// task and therefore, DEL operation by CNI plugin is executed without any config.
	// This value has been made similar to the one present for Linux.
	nsCleanupTimeoutDuration = 2 * time.Second
	// Timeout duration for each network setup and cleanup operation before it is cancelled.
	// The creation of networking stack on Windows is dependent upon HNS, which is an inbox network orchestration
	// service provided by Windows. This can lead to failures and/or increased latency in networking setup.
	// The ideal network setup time would be less than 30 seconds.These network setup timeout values are selected
	// after many trails which allow the tasks to be launched comfortably. These values are similar to the ones
	// used by ECS agent during CNI plugin invocation.
	nsSetupTimeoutDuration = 45 * time.Second

	// Values for creating backoff while retrying network setup.
	// As mentioned above, the networking stack creation is dependent upon HNS. During our POCs, it was found that
	// HNS calls can fail many times while setting up the networking stack. To mitigate task failure due to
	// error response from HNS, we are using retries. These retries result in a maximum total delay of
	// approximately 146 seconds (2.5 minutes) which is less than the ENI setup timeout of 3 minutes.
	// >>> (45s + 4.8s) + (45s + 6.24s) + (45s) ~ 146 seconds
	//        try1             try2        try3
	// Similar retry values are used for ECS agent as well during networking stack creation for the tasks.
	setupNSBackoffMin      = 4 * time.Second
	setupNSBackoffMax      = time.Minute
	setupNSBackoffJitter   = 0.2
	setupNSBackoffMultiple = 1.3
	setupNSMaxRetryCount   = 3

	// windowsProxyIPAddress is the proxy IP address of the endpoint in the task namespace.
	// Since we have a single task running on any instance, we can assign a static IP Address without
	// any IP conflicts.
	windowsProxyIPAddress = "169.254.172.2/22"
)

// GetCNIPluginPath returns the path to the CNI plugin.
func GetCNIPluginPath() string {
	programFiles := os.Getenv("ProgramFiles")
	if len(programFiles) == 0 {
		programFiles = `C:\Program Files`
	}

	return filepath.Join(programFiles, `Amazon\Fargate\cni`)
}

// getCNIPluginLogfilePath returns the path of the CNI Plugin log file.
func getCNIPluginLogfilePath() string {
	programData, ok := os.LookupEnv("ProgramData")
	if !ok {
		programData = `C:\ProgramData`
	}

	return filepath.Join(programData, `Amazon\Fargate\log\cni\vpc-eni.log`)
}

// newVPCENIConfigForENI creates a new vpc-eni CNI plugin configuration for moving task ENI
// into the task namespace.
func newVPCENIConfigForENI(
	iface *networkinterface.NetworkInterface,
	netnsId string,
	networkName string,
) ecscni.PluginConfig {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netnsId,
		CNISpecVersion: cniSpecVersion,
		CNIPluginName:  VPCENIPluginName,
	}

	eniConfig := &ecscni.VPCENIConfig{
		CNIConfig:          cniConfig,
		Name:               networkName,
		UseExistingNetwork: false,
		BlockIMDS:          true,
	}

	eniConfig.DNS = types.DNS{
		Nameservers: iface.DomainNameServers,
	}
	eniConfig.ENIName = iface.DeviceName
	eniConfig.ENIMACAddress = iface.MacAddress
	eniConfig.ENIIPAddresses = []string{iface.GetPrimaryIPv4AddressWithPrefixLength()}
	eniConfig.GatewayIPAddresses = []string{iface.GetSubnetGatewayIPv4Address()}

	return eniConfig
}

// newVPCENIConfigForBridge creates a new vpc-eni CNI plugin configuration for setting up
// bridge to access task IAM roles.
func newVPCENIConfigForBridge(
	netnsId string,
	networkName string,
) ecscni.PluginConfig {
	cniConfig := ecscni.CNIConfig{
		NetNSPath:      netnsId,
		CNISpecVersion: cniSpecVersion,
		CNIPluginName:  VPCENIPluginName,
	}

	eniConfig := &ecscni.VPCENIConfig{
		CNIConfig:          cniConfig,
		Name:               networkName,
		ENIIPAddresses:     []string{windowsProxyIPAddress},
		UseExistingNetwork: true,
		BlockIMDS:          true,
	}

	return eniConfig
}
