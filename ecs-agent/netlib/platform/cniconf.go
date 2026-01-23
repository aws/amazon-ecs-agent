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

package platform

const (
	cniSpecVersion               = "0.3.0"
	blockInstanceMetadataDefault = true
	mtu                          = 9001

	ECSSubNet     = "169.254.172.0/22"
	AgentEndpoint = "169.254.170.2/32"

	// Daemon-bridge networking constants
	DaemonBridgeGatewayIP   = "169.254.172.1"
	DefaultRouteDestination = "0.0.0.0/0"

	// IPv6 daemon-bridge networking constants
	// Using fd00:ec2::172:0/112 as a unique local address (ULA) subnet for ECS internal communication.
	// This mirrors the IPv4 link-local subnet 169.254.172.0/22 in the IPv6 address space.
	// ULA addresses (fd00::/8) are routable within a site but not on the global internet.
	ECSSubNetIPv6               = "fd00:ec2::172:0/112"
	DaemonBridgeGatewayIPv6     = "fd00:ec2::172:1"
	DefaultRouteDestinationIPv6 = "::/0"

	CNIPluginLogFileEnv    = "ECS_CNI_LOG_FILE"
	VPCCNIPluginLogFileEnv = "VPC_CNI_LOG_FILE"
	IPAMDataPathEnv        = "IPAM_DB_PATH"
)
