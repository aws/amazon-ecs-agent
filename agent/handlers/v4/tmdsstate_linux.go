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
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils/netconfig"
)

const (
	defaultNetworkInterfaceNameNotFoundError = "unable to obtain default network interface name on host from endpoint ID: %s"
)

// Returns task metadata including the task network configuration in v4 format for the
// task identified by the provided endpointContainerID.
func (s *TMDSAgentState) GetTaskMetadataWithTaskNetworkConfig(v3EndpointID string, networkConfigClient *netconfig.NetworkConfigClient) (tmdsv4.TaskResponse, error) {
	taskResponse, err := s.getTaskMetadata(v3EndpointID, false, true)
	if err == nil {
		if taskResponse.TaskNetworkConfig != nil && taskResponse.TaskNetworkConfig.NetworkMode == "host" {
			hostDeviceName, netErr := netconfig.DefaultNetInterfaceName(networkConfigClient.NetlinkClient)
			if netErr != nil {
				err = tmdsv4.NewErrorDefaultNetworkInterfaceName(fmt.Sprintf(defaultNetworkInterfaceNameNotFoundError, v3EndpointID))
				logger.Error("Unable to obtain default network interface on host", logger.Fields{
					field.TaskARN:  taskResponse.TaskARN,
					field.Error:    err,
					"netlinkError": netErr,
				})
			} else {
				logger.Info("Obtained default network interface name on host", logger.Fields{
					field.TaskARN:       taskResponse.TaskARN,
					"defaultDeviceName": hostDeviceName,
				})
			}
			taskResponse.TaskNetworkConfig.NetworkNamespaces[0].NetworkInterfaces[0].DeviceName = hostDeviceName
		}
	}
	return taskResponse, err
}
