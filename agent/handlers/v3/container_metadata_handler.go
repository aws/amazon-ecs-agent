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

// ContainerMetadataPath specifies the relative URI path for serving container metadata.
var ContainerMetadataPath = "/v3/" + utils.ConstructMuxVar(v3EndpointIDMuxName, utils.AnythingButSlashRegEx)

// ContainerMetadataHandler returns the handler method for handling container metadata requests.
func ContainerMetadataHandler(state dockerstate.TaskEngineState) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		containerID, err := getContainerIDByRequest(r, state)
		if err != nil {
			responseJSON, _ := json.Marshal(
				fmt.Sprintf("V3 container metadata handler: unable to get container ID from request: %s", err.Error()))
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerMetadata)
			return
		}

		seelog.Infof("V3 container metadata handler: writing response for container '%s'", containerID)

		// v3 handler shares the same container metadata response format with v2 handler.
		v2.WriteContainerMetadataResponse(w, containerID, state)
	}
}
