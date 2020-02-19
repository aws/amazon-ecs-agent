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
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/cihub/seelog"
)

var TaskStatsPath = "/v4/" + utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/task/stats"

func TaskStatsHandler(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskArn, err := v3.GetTaskARNByRequest(r, state)
		if err != nil {
			errResponseJSON, _ := json.Marshal(fmt.Sprintf("V4 task stats handler: unable to get task arn from request: %s", err.Error()))
			utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskStats)
			return
		}
		seelog.Infof("V4 tasks stats handler: writing response for task '%s'", taskArn)
		// v4 handler shares with the same task response format with v2 handler
		v2.WriteTaskStatsResponse(w, taskArn, state, statsEngine)
	}
}
