//go:build linux
// +build linux

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

package v4

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	tmdsutils "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils/netconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
)

// Returns task metadata including the task network configuration in v4 format for the
// task identified by the provided endpointContainerID.
func (s *TMDSAgentState) GetTaskMetadataWithTaskNetworkConfig(
	v3EndpointID string, networkConfigClient *netconfig.NetworkConfigClient,
) (tmdsv4.TaskResponse, error) {
	taskResponse, err := s.getTaskMetadata(v3EndpointID, false, true)
	if err == nil {
		if taskResponse.TaskNetworkConfig != nil && taskResponse.TaskNetworkConfig.NetworkMode == "host" {
			networkInterfaces, netErr := tmdsutils.GetDefaultNetworkInterfaces(networkConfigClient.NetlinkClient)
			if netErr != nil {
				err := tmdsv4.NewErrorDefaultNetworkInterface(netErr)
				logger.Error("Unable to obtain default network interfaces on host", logger.Fields{
					field.TaskARN:  taskResponse.TaskARN,
					field.Error:    err,
					"netlinkError": netErr,
				})
				return taskResponse, err
			}
			taskResponse.TaskNetworkConfig.NetworkNamespaces[0].NetworkInterfaces = utils.ToPtrSlice(networkInterfaces)
		}
	}
	return taskResponse, err
}
