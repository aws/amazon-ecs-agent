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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	tmdsv2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	"github.com/cihub/seelog"
)

// v3EndpointIDMuxName is the key that's used in gorilla/mux to get the v3 endpoint ID.
const V3EndpointIDMuxName = "v3EndpointIDMuxName"

// TaskMetadataPath specifies the relative URI path for serving task metadata.
var TaskMetadataPath = "/v3/" + utils.ConstructMuxVar(V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/task"

// TaskWithTagsMetadataPath specifies the relative URI path for serving task metdata
// with Container Instance and Task Tags retrieved through the ECS API
var TaskWithTagsMetadataPath = "/v3/" + utils.ConstructMuxVar(V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/taskWithTags"

// TaskMetadataHandler returns the handler method for handling task metadata requests.
func TaskMetadataHandler(state dockerstate.TaskEngineState, ecsClient api.ECSClient, cluster, az, containerInstanceArn string, propagateTags bool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := GetTaskARNByRequest(r, state)
		if err != nil {
			responseJSON, err := json.Marshal(
				fmt.Sprintf("V3 task metadata handler: unable to get task arn from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusInternalServerError, responseJSON, utils.RequestTypeTaskMetadata)
			return
		}

		seelog.Infof("V3 task metadata handler: writing response for task '%s'", taskARN)

		taskResponse, err := v2.NewTaskResponse(taskARN, state, ecsClient, cluster, az, containerInstanceArn, propagateTags, false)
		if err != nil {
			errResponseJSON, err := json.Marshal("Unable to generate metadata for task: '" + taskARN + "'")
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusInternalServerError, errResponseJSON, utils.RequestTypeTaskMetadata)
			return
		}

		task, _ := state.TaskByArn(taskARN)
		if !task.IsNetworkModeAWSVPC() {
			// fill in non-awsvpc network details for container responses here
			responses := make([]tmdsv2.ContainerResponse, 0)
			for _, containerResponse := range taskResponse.Containers {
				networks, err := GetContainerNetworkMetadata(containerResponse.ID, state)
				if err != nil {
					errResponseJSON, err := json.Marshal(err.Error())
					if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
						return
					}
					utils.WriteJSONToResponse(w, http.StatusInternalServerError, errResponseJSON, utils.RequestTypeContainerMetadata)
					return
				}
				containerResponse.Networks = networks
				responses = append(responses, containerResponse)
			}
			taskResponse.Containers = responses
		}
		responseJSON, err := json.Marshal(taskResponse)
		if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
			return
		}
		utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeTaskMetadata)
	}
}
