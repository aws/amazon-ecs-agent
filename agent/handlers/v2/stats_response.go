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

package v2

import (
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

// NewTaskStatsResponse returns a new task stats response object
func NewTaskStatsResponse(taskARN string,
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine) (map[string]*types.StatsJSON, error) {

	containerMap, ok := state.ContainerMapByArn(taskARN)
	if !ok {
		return nil, errors.Errorf(
			"v2 task stats response: unable to lookup containers for task %s",
			taskARN)
	}

	resp := make(map[string]*types.StatsJSON)
	for _, dockerContainer := range containerMap {
		containerID := dockerContainer.DockerID
		dockerStats, _, err := statsEngine.ContainerDockerStats(taskARN, containerID)
		if err != nil {
			seelog.Warnf("V2 task stats response: Unable to get stats for container '%s' for task '%s': %v",
				containerID, taskARN, err)
			resp[containerID] = nil
			continue
		}

		resp[containerID] = dockerStats
	}

	return resp, nil
}
