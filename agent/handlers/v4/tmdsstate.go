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

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/cihub/seelog"
)

// Implements AgentState interface for TMDS v4.
type TMDSAgentState struct {
	state dockerstate.TaskEngineState
}

func NewTMDSAgentState(state dockerstate.TaskEngineState) *TMDSAgentState {
	return &TMDSAgentState{state: state}
}

// Returns container metadata in v4 format for the container identified by the provided
// v3EndpointID.
func (s *TMDSAgentState) GetContainerMetadata(v3EndpointID string) (tmdsv4.ContainerResponse, error) {
	// Get docker ID from the v3 endpoint ID.
	containerID, ok := s.state.DockerIDByV3EndpointID(v3EndpointID)
	if !ok {
		return tmdsv4.ContainerResponse{}, tmdsv4.NewErrorLookupFailure(fmt.Sprintf(
			"unable to get container ID from request: unable to get docker ID from v3 endpoint ID: %s",
			v3EndpointID))
	}

	containerResponse, err := NewContainerResponse(containerID, s.state)
	if err != nil {
		seelog.Errorf("Unable to get container metadata for container '%s'", containerID)
		return tmdsv4.ContainerResponse{}, tmdsv4.NewErrorMetadataFetchFailure(fmt.Sprintf(
			"unable to generate metadata for container '%s'", containerID))
	}

	// fill in network details if not set for NON AWSVPC Task
	if containerResponse.Networks == nil {
		if containerResponse.Networks, err = GetContainerNetworkMetadata(containerID, s.state); err != nil {
			return tmdsv4.ContainerResponse{}, tmdsv4.NewErrorMetadataFetchFailure(err.Error())
		}
	}

	return *containerResponse, nil
}
