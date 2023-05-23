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

// TaskStatsPath specifies the relative URI path for serving task stats.
var TaskStatsPath = "/v3/" + utils.ConstructMuxVar(V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/task/stats"

// TaskStatsHandler returns the handler method for handling task stats requests.
func TaskStatsHandler(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := GetTaskARNByRequest(r, state)
		if err != nil {
			errResponseJSON, err := json.Marshal(
				fmt.Sprintf("V3 task stats handler: unable to get task arn from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskStats)
			return
		}

		seelog.Infof("V3 task stats handler: writing response for task '%s'", taskARN)

		// v3 handler shares the same task stats response format with v2 handler.
		v2.WriteTaskStatsResponse(w, taskARN, state, statsEngine)
	}
}
