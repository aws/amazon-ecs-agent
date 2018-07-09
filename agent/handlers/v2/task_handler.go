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
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// TaskContainerMetadataPath specifies the relative URI path for serving task metadata.
const TaskContainerMetadataPath = "/v2/metadata"

// TaskContainerMetadataHandler returns the handler method for handling task metadata requests.
func TaskContainerMetadataHandler(state dockerstate.TaskEngineState, cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := getTaskARN(r, state)
		if err != nil {
			responseJSON, _ := json.Marshal(
				fmt.Sprintf("Unable to get task arn from request: %s", err.Error()))
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeTaskMetadata)
			return
		}
		if containerID := getContainerID(r.URL, TaskContainerMetadataPath); containerID != "" {
			writeContainerResponse(w, containerID, state)
			return
		}

		writeTaskResponse(w, taskARN, cluster, state)
	}
}

func getContainerID(reqURL *url.URL, prefix string) string {
	if strings.HasPrefix(reqURL.Path, prefix+"/") {
		return reqURL.String()[len(prefix+"/"):]
	}

	return ""
}

func getTaskARN(r *http.Request, state dockerstate.TaskEngineState) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", fmt.Errorf("Unable to parse request's ip address: %v", err)
	}

	// Get task arn for the request by looking up the ip address
	taskARN, ok := state.GetTaskByIPAddress(ip)
	if !ok {
		return "", errors.Errorf("Unable to associate '%s' with task", ip)
	}

	return taskARN, nil
}

func writeContainerResponse(w http.ResponseWriter, containerID string, state dockerstate.TaskEngineState) {
	seelog.Infof("V2 metadata: handling request for container '%s'", containerID)
	containerResponse, err := NewContainerResponse(containerID, state)
	if err != nil {
		errResponseJSON, _ := json.Marshal("Unable to generate metadata for container '" + containerID + "'")
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeContainerMetadata)
		return
	}

	responseJSON, _ := json.Marshal(containerResponse)
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeContainerMetadata)
}

func writeTaskResponse(w http.ResponseWriter, taskARN string, cluster string, state dockerstate.TaskEngineState) {
	seelog.Infof("V2 metadata: handling request for task '%s'", taskARN)
	// Generate a response for the task
	taskResponse, err := NewTaskResponse(taskARN, state, cluster)
	if err != nil {
		errResponseJSON, _ := json.Marshal("Unable to generate metadata for task: '" + taskARN + "'")
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeTaskMetadata)
		return
	}

	responseJSON, _ := json.Marshal(taskResponse)
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeTaskMetadata)
}
