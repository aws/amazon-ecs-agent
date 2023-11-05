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

import "time"

const (
	cniSpecVersion               = "0.3.0"
	blockInstanceMetadataDefault = true
	mtu                          = 9001

	ECSSubNet     = "169.254.172.0/22"
	AgentEndpoint = "169.254.170.2/32"

	CNIPluginLogFileEnv    = "ECS_CNI_LOG_FILE"
	VPCCNIPluginLogFileEnv = "VPC_CNI_LOG_FILE"
	IPAMDataPathEnv        = "IPAM_DB_PATH"

	// Timeout duration for each network setup and cleanup operation before it is cancelled.
	nsSetupTimeoutDuration   = 1 * time.Minute
	nsCleanupTimeoutDuration = 30 * time.Second
)
