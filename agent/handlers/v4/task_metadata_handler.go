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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/cihub/seelog"
)

// TaskMetadataPath specifies the relative URI path for serving task metadata.
var TaskMetadataPath = "/v4/" + utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/task"

// TaskWithTagsMetadataPath specifies the relative URI path for serving task metdata
// with Container Instance and Task Tags retrieved through the ECS API
var TaskWithTagsMetadataPath = "/v4/" + utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx) + "/taskWithTags"

// TaskMetadataHandler returns the handler method for handling task metadata requests.
func TaskMetadataHandler(state dockerstate.TaskEngineState, ecsClient api.ECSClient, cluster, az, containerInstanceArn string, propagateTags bool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var taskArn, err = v3.GetTaskARNByRequest(r, state)
		if err != nil {
			ResponseJSON, _ := json.Marshal(fmt.Sprintf("V4 task metadata handler: unable to get task arn from request: %s", err.Error()))
			utils.WriteJSONToResponse(w, http.StatusBadRequest, ResponseJSON, utils.RequestTypeTaskMetadata)
			return
		}

		seelog.Infof("V4 taskMetadata handler: Writing response for task '%s'", taskArn)

		taskResponse, err := NewTaskResponse(taskArn, state, ecsClient, cluster, az, containerInstanceArn, propagateTags)
		if err != nil {
			errResponseJson, _ := json.Marshal("Unable to generate metadata for v4 task: '" + taskArn + "'")
			utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJson, utils.RequestTypeTaskMetadata)
			return
		}

		task, _ := state.TaskByArn(taskArn)
		// for non-awsvpc task mode
		if !task.IsNetworkModeAWSVPC() {
			// fill in non-awsvpc network details for container responses here
			responses := make([]ContainerResponse, 0)
			for _, containerResponse := range taskResponse.Containers {
				networks, err := GetContainerNetworkMetadata(containerResponse.ID, state)
				if err != nil {
					errResponseJSON, _ := json.Marshal(err.Error())
					utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeContainerMetadata)
					return
				}
				containerResponse.Networks = networks
				responses = append(responses, containerResponse)
			}
			taskResponse.Containers = responses
		}

		responseJSON, _ := json.Marshal(taskResponse)
		utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeTaskMetadata)
	}
}
