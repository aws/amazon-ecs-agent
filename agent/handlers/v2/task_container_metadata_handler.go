// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package v2

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/cihub/seelog"
)

const (
	// metadataContainerIDMuxName is the key that's used in gorilla/mux to get the container ID
	// for container metadata.
	metadataContainerIDMuxName = "metadataContainerIDMuxName"

	// TaskMetadataPath specifies the relative URI path for serving task metadata.
	TaskMetadataPath = "/v2/metadata"

	// TaskWithTagsMetadataPath specifies the relative URI path for serving task metadata with Container Instance and Task Tags.
	TaskWithTagsMetadataPath = "/v2/metadataWithTags"

	// TaskMetadataPathWithSlash specifies the relative URI path for serving task metadata.
	TaskMetadataPathWithSlash = TaskMetadataPath + "/"

	// TaskWithTagsMetadataPath specifies the relative URI path for serving task metadata with Container Instance and Task Tags.
	TaskWithTagsMetadataPathWithSlash = TaskWithTagsMetadataPath + "/"
)

// ContainerMetadataPath specifies the relative URI path for serving container metadata.
var ContainerMetadataPath = TaskMetadataPathWithSlash + utils.ConstructMuxVar(metadataContainerIDMuxName, utils.AnythingButEmptyRegEx)

// TaskContainerMetadataHandler returns the handler method for handling task and container metadata requests.
func TaskContainerMetadataHandler(state dockerstate.TaskEngineState, ecsClient api.ECSClient, cluster, az, containerInstanceArn string, propagateTags bool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := getTaskARNByRequest(r, state)
		if err != nil {
			responseJSON, _ := json.Marshal(
				fmt.Sprintf("Unable to get task arn from request: %s", err.Error()))
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeTaskMetadata)
			return
		}
		if containerID, ok := utils.GetMuxValueFromRequest(r, metadataContainerIDMuxName); ok {
			seelog.Infof("V2 task/container metadata handler: writing response for container '%s'", containerID)
			WriteContainerMetadataResponse(w, containerID, state)
			return
		}

		seelog.Infof("V2 task/container metadata handler: writing response for task '%s'", taskARN)
		WriteTaskMetadataResponse(w, taskARN, cluster, state, ecsClient, az, containerInstanceArn, propagateTags)
	}
}

// WriteContainerMetadataResponse writes the container metadata to response writer.
func WriteContainerMetadataResponse(w http.ResponseWriter, containerID string, state dockerstate.TaskEngineState) {
	containerResponse, err := NewContainerResponse(containerID, state)
	if err != nil {
		errResponseJSON, _ := json.Marshal("Unable to generate metadata for container '" + containerID + "'")
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeContainerMetadata)
		return
	}

	responseJSON, _ := json.Marshal(containerResponse)
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeContainerMetadata)
}

// WriteTaskMetadataResponse writes the task metadata to response writer.
func WriteTaskMetadataResponse(w http.ResponseWriter, taskARN string, cluster string, state dockerstate.TaskEngineState, ecsClient api.ECSClient, az, containerInstanceArn string, propagateTags bool) {
	// Generate a response for the task
	taskResponse, err := NewTaskResponse(taskARN, state, ecsClient, cluster, az, containerInstanceArn, propagateTags)
	if err != nil {
		errResponseJSON, _ := json.Marshal("Unable to generate metadata for task: '" + taskARN + "'")
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskMetadata)
		return
	}

	responseJSON, _ := json.Marshal(taskResponse)
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeTaskMetadata)
}
