// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/cihub/seelog"
)

// StatsPath specifies the relative URI path for serving task and container stats.
const StatsPath = "/v2/stats"

// StatsHandler creates response for 'v2/stats' API.
func StatsHandler(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := getTaskARN(r, state)
		if err != nil {
			errResponseJSON, _ := json.Marshal(
				fmt.Sprintf("Unable to get task arn from request: %s", err.Error()))
			utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskStats)
			return
		}
		if containerID := getContainerID(r.URL, StatsPath); containerID != "" {
			writeContainerStatsResponse(w, taskARN, containerID, statsEngine)
			return
		}

		writeTaskStatsResponse(w, taskARN, state, statsEngine)
	}
}

func writeTaskStatsResponse(w http.ResponseWriter,
	taskARN string,
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine) {

	taskStatsResponse, err := NewTaskStatsResponse(taskARN, state, statsEngine)
	if err != nil {
		seelog.Warnf("Unable to get task stats for task '%s': %v", taskARN, err)
		errResponseJSON, _ := json.Marshal("Unable to get task stats for: " + taskARN)
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskStats)
		return
	}

	responseJSON, _ := json.Marshal(taskStatsResponse)
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeTaskStats)
}

func writeContainerStatsResponse(w http.ResponseWriter,
	taskARN string,
	containerID string,
	statsEngine stats.Engine) {
	dockerStats, err := statsEngine.ContainerDockerStats(taskARN, containerID)
	if err != nil {
		errResponseJSON, _ := json.Marshal("Unable to get container stats for: " + containerID)
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeContainerStats)
		return
	}

	responseJSON, _ := json.Marshal(dockerStats)
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeContainerStats)
}
