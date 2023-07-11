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
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	response "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// NewV4TaskStatsResponse returns a new v4 task stats response object
func NewV4TaskStatsResponse(taskARN string,
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine) (map[string]*response.StatsResponse, error) {

	containerMap, ok := state.ContainerMapByArn(taskARN)
	if !ok {
		return nil, errors.Errorf(
			"v4 task stats response: unable to lookup containers for task %s",
			taskARN)
	}

	resp := make(map[string]*response.StatsResponse)
	for _, dockerContainer := range containerMap {
		containerID := dockerContainer.DockerID
		dockerStats, network_rate_stats, err := statsEngine.ContainerDockerStats(taskARN, containerID)
		if err != nil {
			seelog.Warnf("V4 task stats response: Unable to get stats for container '%s' for task '%s': %v",
				containerID, taskARN, err)
			resp[containerID] = &response.StatsResponse{}
			continue
		}

		statsResponse := response.StatsResponse{
			StatsJSON:          dockerStats,
			Network_rate_stats: network_rate_stats,
		}

		resp[containerID] = &statsResponse
	}

	return resp, nil
}
