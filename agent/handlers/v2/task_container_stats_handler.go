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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/cihub/seelog"
)

const (
	// statsContainerIDMuxName is the key that's used in mux to get the container ID
	// for container stats.
	statsContainerIDMuxName = "statsContainerIDMuxName"

	// TaskStatsPath specifies the relative URI path for serving task stats.
	TaskStatsPath = "/v2/stats"

	// TaskStatsPathWithSlash specifies the relative URI path for serving task stats.
	TaskStatsPathWithSlash = TaskStatsPath + "/"
)

// ContainerStatsPath specifies the relative URI path for serving container stats.
var ContainerStatsPath = TaskStatsPathWithSlash + utils.ConstructMuxVar(statsContainerIDMuxName, utils.AnythingButEmptyRegEx)

// TaskContainerStatsHandler returns the handler method for handling task and container stats requests.
func TaskContainerStatsHandler(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := getTaskARNByRequest(r, state)
		if err != nil {
			errResponseJSON, err := json.Marshal(
				fmt.Sprintf("Unable to get task arn from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskStats)
			return
		}
		if containerID, ok := utils.GetMuxValueFromRequest(r, statsContainerIDMuxName); ok {
			seelog.Infof("V2 task/container stats handler: writing response for container '%s'", containerID)
			WriteContainerStatsResponse(w, taskARN, containerID, statsEngine)
			return
		}

		seelog.Infof("V2 task/container stats handler: writing response for task '%s'", taskARN)
		WriteTaskStatsResponse(w, taskARN, state, statsEngine)
	}
}

// WriteTaskStatsResponse writes the task stats to response writer.
func WriteTaskStatsResponse(w http.ResponseWriter,
	taskARN string,
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine) {

	taskStatsResponse, err := NewTaskStatsResponse(taskARN, state, statsEngine)
	if err != nil {
		seelog.Warnf("Unable to get task stats for task '%s': %v", taskARN, err)
		errResponseJSON, err := json.Marshal("Unable to get task stats for: " + taskARN)
		if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
			return
		}
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskStats)
		return
	}

	responseJSON, err := json.Marshal(taskStatsResponse)
	if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
		return
	}
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeTaskStats)
}

// WriteContainerStatsResponse writes the container stats to response writer.
func WriteContainerStatsResponse(w http.ResponseWriter,
	taskARN string,
	containerID string,
	statsEngine stats.Engine) {
	dockerStats, _, err := statsEngine.ContainerDockerStats(taskARN, containerID)
	if err != nil {
		errResponseJSON, err := json.Marshal("Unable to get container stats for: " + containerID)
		if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
			return
		}
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeContainerStats)
		return
	}

	responseJSON, err := json.Marshal(dockerStats)
	if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
		return
	}
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeContainerStats)
}
