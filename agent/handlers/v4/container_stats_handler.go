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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/cihub/seelog"
)

// ContainerStatsPath specifies the relative URI path for serving container stats.
var ContainerStatsPath = "/v4/" + utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/stats"

// ContainerStatsHandler returns the handler method for handling container stats requests.
func ContainerStatsHandler(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskArn, err := v3.GetTaskARNByRequest(r, state)
		if err != nil {
			errResponseJSON, err := json.Marshal(fmt.Sprintf("V4 container handler: unable to get task arn from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusNotFound, errResponseJSON, utils.RequestTypeTaskStats)
			return
		}

		containerID, err := v3.GetContainerIDByRequest(r, state)
		if err != nil {
			responseJSON, err := json.Marshal(fmt.Sprintf("V4 container stats handler: unable to get container ID from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusNotFound, responseJSON, utils.RequestTypeContainerStats)
			return
		}

		seelog.Infof("V4 container stats handler: writing response for container '%s'", containerID)
		// v4 handler shares the same container states response format with v2 handler.
		WriteV4ContainerStatsResponse(w, taskArn, containerID, statsEngine)
	}
}

// WriteContainerStatsResponse writes the container stats to response writer.
func WriteV4ContainerStatsResponse(w http.ResponseWriter,
	taskARN string,
	containerID string,
	statsEngine stats.Engine) {
	dockerStats, network_rate_stats, err := statsEngine.ContainerDockerStats(taskARN, containerID)
	if err != nil {
		errResponseJSON, err := json.Marshal("Unable to get container stats for: " + containerID)
		if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
			return
		}
		utils.WriteJSONToResponse(w, http.StatusInternalServerError, errResponseJSON, utils.RequestTypeContainerStats)
		return
	}

	containerStatsResponse := StatsResponse{
		StatsJSON:          dockerStats,
		Network_rate_stats: network_rate_stats,
	}

	responseJSON, err := json.Marshal(containerStatsResponse)
	if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
		return
	}
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeContainerStats)
}
