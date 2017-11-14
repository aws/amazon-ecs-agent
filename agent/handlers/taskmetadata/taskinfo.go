// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package taskmetadata

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/types/v2"
)

const (
	// metadataPath specifies the relative URI path for serving task metadata
	metadataPath = "/v2/metadata"
)

// metadataV2Handler returns the handler method for handling task metadata requests
func metadataV2Handler(state dockerstate.TaskEngineState, cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get request's ip address
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			jsonMsg, _ := json.Marshal("Unable to get request's ip address")
			writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
			return
		}

		// Get task arn for the request by looking up the ip address
		taskARN, ok := state.GetTaskByIPAddress(ip)
		if !ok {
			jsonMsg, _ := json.Marshal("Unable to assoicate '" + ip + "' with a task")
			writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
			return
		}

		// Generate a response for the task
		taskResponse, err := v2.NewTaskResponse(taskARN, state, cluster)
		if err != nil {
			jsonMsg, _ := json.Marshal("Unable to generate metadata for '" + ip + "'")
			writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
			return
		}

		jsonMsg, _ := json.Marshal(taskResponse)
		writeJSONToResponse(w, http.StatusOK, jsonMsg, requestTypeMetadata)
	}
}
