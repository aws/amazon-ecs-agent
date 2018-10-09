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

package v3

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	"github.com/cihub/seelog"
)

// v3EndpointIDMuxName is the key that's used in gorilla/mux to get the v3 endpoint ID.
const v3EndpointIDMuxName = "v3EndpointIDMuxName"

// TaskMetadataPath specifies the relative URI path for serving task metadata.
var TaskMetadataPath = "/v3/" + utils.ConstructMuxVar(v3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/task"

// TaskMetadataHandler returns the handler method for handling task metadata requests.
func TaskMetadataHandler(state dockerstate.TaskEngineState, cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := getTaskARNByRequest(r, state)
		if err != nil {
			responseJSON, _ := json.Marshal(
				fmt.Sprintf("V3 task metadata handler: unable to get task arn from request: %s", err.Error()))
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeTaskMetadata)
			return
		}

		seelog.Infof("V3 task metadata handler: writing response for task '%s'", taskARN)

		// v3 handler shares the same task metadata response format with v2 handler.
		v2.WriteTaskMetadataResponse(w, taskARN, cluster, state)
	}
}
