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
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	"github.com/cihub/seelog"
)

var TaskStatsPath = "/v4/" + utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/task/stats"

func TaskStatsHandler(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskArn, err := v3.GetTaskARNByRequest(r, state)
		if err != nil {
			errResponseJSON, err := json.Marshal(fmt.Sprintf("V4 task stats handler: unable to get task arn from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusNotFound, errResponseJSON, utils.RequestTypeTaskStats)
			return
		}
		WriteV4TaskStatsResponse(w, taskArn, state, statsEngine)
	}
}

// WriteV4TaskStatsResponse writes the task stats to response writer.
func WriteV4TaskStatsResponse(w http.ResponseWriter,
	taskARN string,
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine) {

	taskStatsResponse, err := NewV4TaskStatsResponse(taskARN, state, statsEngine)
	if err != nil {
		seelog.Warnf("Unable to get task stats for task '%s': %v", taskARN, err)
		errResponseJSON, err := json.Marshal("Unable to get task stats for: " + taskARN)
		if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
			return
		}
		utils.WriteJSONToResponse(w, http.StatusInternalServerError, errResponseJSON, utils.RequestTypeTaskStats)
		return
	}

	responseJSON, err := json.Marshal(taskStatsResponse)
	if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
		return
	}
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeTaskStats)
}
