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

package v3

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	"github.com/cihub/seelog"
)

// ContainerStatsPath specifies the relative URI path for serving container stats.
var ContainerStatsPath = "/v3/" + utils.ConstructMuxVar(V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/stats"

// ContainerStatsHandler returns the handler method for handling container stats requests.
func ContainerStatsHandler(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := GetTaskARNByRequest(r, state)
		if err != nil {
			errResponseJSON, err := json.Marshal(
				fmt.Sprintf("V3 container stats handler: unable to get task arn from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskStats)
			return
		}

		containerID, err := GetContainerIDByRequest(r, state)
		if err != nil {
			responseJSON, err := json.Marshal(
				fmt.Sprintf("V3 container stats handler: unable to get container ID from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerStats)
			return
		}

		seelog.Infof("V3 container stats handler: writing response for container '%s'", containerID)

		// v3 handler shares the same container stats response format with v2 handler.
		v2.WriteContainerStatsResponse(w, taskARN, containerID, statsEngine)
	}
}
