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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
)

// Implements AgentState interface for TMDS v4.
type TMDSAgentState struct {
	state                dockerstate.TaskEngineState
	statsEngine          stats.Engine
	ecsClient            api.ECSClient
	cluster              string
	availabilityZone     string
	vpcID                string
	containerInstanceARN string
}

func NewTMDSAgentState(
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine,
	ecsClient api.ECSClient,
	cluster string,
	availabilityZone string,
	vpcID string,
	containerInstanceARN string,
) *TMDSAgentState {
	return &TMDSAgentState{
		state:                state,
		statsEngine:          statsEngine,
		ecsClient:            ecsClient,
		cluster:              cluster,
		availabilityZone:     availabilityZone,
		vpcID:                vpcID,
		containerInstanceARN: containerInstanceARN,
	}
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
		logger.Error("Failed to get container metadata", logger.Fields{
			field.Container: containerID,
			field.Error:     err,
		})
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

// Returns task metadata in v4 format for the task identified by the provided endpointContainerID.
func (s *TMDSAgentState) GetTaskMetadata(v3EndpointID string) (tmdsv4.TaskResponse, error) {
	return s.getTaskMetadata(v3EndpointID, false)
}

// Returns task metadata including task and container instance tags in v4 format for the
// task identified by the provided endpointContainerID.
func (s *TMDSAgentState) GetTaskMetadataWithTags(v3EndpointID string) (tmdsv4.TaskResponse, error) {
	return s.getTaskMetadata(v3EndpointID, true)
}

// Returns task metadata in v4 format for the task identified by the provided endpointContainerID.
func (s *TMDSAgentState) getTaskMetadata(v3EndpointID string, includeTags bool) (tmdsv4.TaskResponse, error) {
	taskARN, ok := s.state.TaskARNByV3EndpointID(v3EndpointID)
	if !ok {
		return tmdsv4.TaskResponse{}, tmdsv4.NewErrorLookupFailure(fmt.Sprintf(
			"unable to get task arn from request: unable to get task Arn from v3 endpoint ID: %s",
			v3EndpointID))
	}

	task, ok := s.state.TaskByArn(taskARN)
	if !ok {
		logger.Error("Task not found in state", logger.Fields{field.TaskARN: taskARN})
		return tmdsv4.TaskResponse{}, tmdsv4.NewErrorMetadataFetchFailure(fmt.Sprintf(
			"Unable to generate metadata for v4 task: '%s'", taskARN))
	}

	taskResponse, err := NewTaskResponse(taskARN, s.state, s.ecsClient, s.cluster,
		s.availabilityZone, s.vpcID, s.containerInstanceARN, task.ServiceName, includeTags)
	if err != nil {
		logger.Error("Failed to get task metadata", logger.Fields{
			field.TaskARN: taskARN,
			field.Error:   err,
		})
		return tmdsv4.TaskResponse{}, tmdsv4.NewErrorMetadataFetchFailure(fmt.Sprintf(
			"Unable to generate metadata for v4 task: '%s'", taskARN))
	}

	taskResponse.CredentialsID = task.GetCredentialsID()

	// for non-awsvpc task mode
	if !task.IsNetworkModeAWSVPC() {
		// fill in non-awsvpc network details for container responses here
		responses := make([]tmdsv4.ContainerResponse, 0)
		for _, containerResponse := range taskResponse.Containers {
			networks, err := GetContainerNetworkMetadata(containerResponse.ID, s.state)
			if err != nil {
				logger.Warn("Error retrieving network metadata", logger.Fields{
					field.Container: containerResponse.ID,
					field.Error:     err,
				})
			}
			containerResponse.Networks = networks
			responses = append(responses, containerResponse)
		}
		taskResponse.Containers = responses
	}

	pulledContainers, _ := s.state.PulledContainerMapByArn(task.Arn)
	// Convert each pulled container into v4 container response
	// and append pulled containers to taskResponse.Containers
	for _, dockerContainer := range pulledContainers {
		taskResponse.Containers = append(taskResponse.Containers,
			NewPulledContainerResponse(dockerContainer, task.GetPrimaryENI()))
	}

	return *taskResponse, nil
}

func (s *TMDSAgentState) GetContainerStats(v3EndpointID string) (tmdsv4.StatsResponse, error) {
	taskARN, ok := s.state.TaskARNByV3EndpointID(v3EndpointID)
	if !ok {
		return tmdsv4.StatsResponse{}, tmdsv4.NewErrorStatsLookupFailure(fmt.Sprintf(
			"unable to get task arn from request: unable to get task Arn from v3 endpoint ID: %s",
			v3EndpointID))
	}

	containerID, ok := s.state.DockerIDByV3EndpointID(v3EndpointID)
	if !ok {
		return tmdsv4.StatsResponse{}, tmdsv4.NewErrorStatsLookupFailure(fmt.Sprintf(
			"unable to get container ID from request: unable to get docker ID from v3 endpoint ID: %s",
			v3EndpointID))
	}

	dockerStats, network_rate_stats, err := s.statsEngine.ContainerDockerStats(taskARN, containerID)
	if err != nil {
		return tmdsv4.StatsResponse{}, tmdsv4.NewErrorStatsFetchFailure(
			fmt.Sprintf("Unable to get container stats for: %s", containerID),
			err)
	}

	return tmdsv4.StatsResponse{
		StatsJSON:          dockerStats,
		Network_rate_stats: network_rate_stats,
	}, nil
}

func (s *TMDSAgentState) GetTaskStats(v3EndpointID string) (map[string]*tmdsv4.StatsResponse, error) {
	taskARN, ok := s.state.TaskARNByV3EndpointID(v3EndpointID)
	if !ok {
		return nil, tmdsv4.NewErrorStatsLookupFailure(fmt.Sprintf(
			"unable to get task arn from request: unable to get task Arn from v3 endpoint ID: %s",
			v3EndpointID))
	}

	taskStatsResponse, err := NewV4TaskStatsResponse(taskARN, s.state, s.statsEngine)
	if err != nil {
		return nil, tmdsv4.NewErrorStatsFetchFailure(
			fmt.Sprintf("Unable to get task stats for: %s", taskARN),
			err)
	}

	return taskStatsResponse, nil
}
