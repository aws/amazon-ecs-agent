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
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/types/v2"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// metadataPath specifies the relative URI path for serving task metadata
	metadataPath = "/v2/metadata"
	statsPath    = "/v2/stats"
)

// metadataV2Handler returns the handler method for handling task metadata requests
func metadataV2Handler(state dockerstate.TaskEngineState, cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := getTaskARN(r, state)
		if err != nil {
			jsonMsg, _ := json.Marshal(
				fmt.Sprintf("Unable to get task arn from request: %s", err.Error()))
			writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
			return
		}
		if containerID := getContainerID(r.URL, metadataPath); containerID != "" {
			writeContainerResponse(w, containerID, state)
			return
		}

		writeTaskResponse(w, taskARN, cluster, state)
	}
}

func statsV2Handler(state dockerstate.TaskEngineState, statsEngine stats.Engine) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := getTaskARN(r, state)
		if err != nil {
			jsonMsg, _ := json.Marshal(
				fmt.Sprintf("Unable to get task arn from request: %s", err.Error()))
			writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
			return
		}
		if containerID := getContainerID(r.URL, statsPath); containerID != "" {
			writeContainerStatsResponse(w, taskARN, containerID, statsEngine)
			return
		}

		writeTaskStatsResponse(w, taskARN, state, statsEngine)
	}
}

func getTaskARN(r *http.Request, state dockerstate.TaskEngineState) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", fmt.Errorf("v2 handler: unable to parse request's ip address: %v", err)
	}

	// Get task arn for the request by looking up the ip address
	taskARN, ok := state.GetTaskByIPAddress(ip)
	if !ok {
		return "", errors.Errorf("v2 handler: unable to associate '%s' with task", ip)
	}

	return taskARN, nil
}

func getContainerID(reqURL *url.URL, prefix string) string {
	if strings.HasPrefix(reqURL.Path, prefix+"/") {
		return reqURL.String()[len(prefix+"/"):]
	}

	return ""
}

func writeContainerResponse(w http.ResponseWriter, containerID string, state dockerstate.TaskEngineState) {
	seelog.Infof("V2 metadata: handling request for container '%s'", containerID)
	containerResponse, err := v2.NewContainerResponse(containerID, state)
	if err != nil {
		jsonMsg, _ := json.Marshal("Unable to generate metadata for container '" + containerID + "'")
		writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
		return
	}

	jsonMsg, _ := json.Marshal(containerResponse)
	writeJSONToResponse(w, http.StatusOK, jsonMsg, requestTypeMetadata)
}

func writeTaskResponse(w http.ResponseWriter, taskARN string, cluster string, state dockerstate.TaskEngineState) {
	seelog.Infof("V2 metadata: handling request for task '%s'", taskARN)
	// Generate a response for the task
	taskResponse, err := v2.NewTaskResponse(taskARN, state, cluster)
	if err != nil {
		jsonMsg, _ := json.Marshal("Unable to generate metadata for task: '" + taskARN + "'")
		writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
		return
	}

	jsonMsg, _ := json.Marshal(taskResponse)
	writeJSONToResponse(w, http.StatusOK, jsonMsg, requestTypeMetadata)
}

func writeContainerStatsResponse(w http.ResponseWriter,
	taskARN string,
	containerID string,
	statsEngine stats.Engine) {
	dockerStats, err := statsEngine.ContainerDockerStats(taskARN, containerID)
	if err != nil {
		jsonMsg, _ := json.Marshal("Unable to get container stats for: " + containerID)
		writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
		return
	}

	jsonMsg, _ := json.Marshal(dockerStats)
	writeJSONToResponse(w, http.StatusOK, jsonMsg, requestTypeMetadata)
}

func writeTaskStatsResponse(w http.ResponseWriter,
	taskARN string,
	state dockerstate.TaskEngineState,
	statsEngine stats.Engine) {

	taskStatsResponse, err := v2.NewTaskStatsResponse(taskARN, state, statsEngine)
	if err != nil {
		seelog.Warnf("Unable to get task stats for task '%s': %v", taskARN, err)
		jsonMsg, _ := json.Marshal("Unable to get task stats for: " + taskARN)
		writeJSONToResponse(w, http.StatusBadRequest, jsonMsg, requestTypeMetadata)
		return
	}

	jsonMsg, _ := json.Marshal(taskStatsResponse)
	writeJSONToResponse(w, http.StatusOK, jsonMsg, requestTypeMetadata)
}
