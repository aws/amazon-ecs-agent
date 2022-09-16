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

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/agentapi/v1/taskprotection/types"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/agent/logger/field"
)

// Returns endpoint path for PutTaskProtection API
func TaskProtectionPath() string {
	return fmt.Sprintf(
		"/api/%s/task-protection/v1/state",
		utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx))
}

// Task protection request received from customers pending validation
type TaskProtectionRequest struct {
	ProtectionEnabled *bool
	ExpiresInMinutes  *int
}

// PutTaskProtectionHandler returns an HTTP request handler function for
// PutTaskProtection API
func PutTaskProtectionHandler(state dockerstate.TaskEngineState,
	cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		putTaskProtectionRequestType := "api/v1/PutTaskProtection"

		var request TaskProtectionRequest
		jsonDecoder := json.NewDecoder(r.Body)
		jsonDecoder.DisallowUnknownFields()
		if err := jsonDecoder.Decode(&request); err != nil {
			logger.Error("PutTaskProtection: failed to decode request", logger.Fields{
				loggerfield.Error: err,
			})
			writeJSONResponse(w, http.StatusBadRequest,
				"Failed to decode request", putTaskProtectionRequestType)
			return
		}

		if request.ProtectionEnabled == nil {
			writeJSONResponse(w, http.StatusBadRequest,
				"Invalid request: does not contain 'ProtectionEnabled' field",
				putTaskProtectionRequestType)
			return
		}

		taskProtection, err := types.NewTaskProtection(*request.ProtectionEnabled, request.ExpiresInMinutes)
		if err != nil {
			writeJSONResponse(w, http.StatusBadRequest,
				fmt.Sprintf("Invalid request: %v", err),
				putTaskProtectionRequestType)
			return
		}

		task, responseCode, err := getTaskFromRequest(state, r)
		if err != nil {
			writeJSONResponse(w, responseCode, err.Error(), putTaskProtectionRequestType)
			return
		}

		// TODO: Call ECS
		logger.Info("PutTaskProtection endpoint was called", logger.Fields{
			loggerfield.Cluster:        cluster,
			loggerfield.TaskARN:        task.Arn,
			loggerfield.TaskProtection: taskProtection,
		})
		writeJSONResponse(w, http.StatusOK, "Ok", putTaskProtectionRequestType)
	}
}

// Helper function for finding task for the request
func getTaskFromRequest(state dockerstate.TaskEngineState, r *http.Request) (*apitask.Task, int, error) {
	taskARN, err := v3.GetTaskARNByRequest(r, state)
	if err != nil {
		logger.Error("Failed to find task ARN for task protection request", logger.Fields{
			loggerfield.Error: err,
		})
		return nil, http.StatusBadRequest, errors.New("Invalid request: no task was found")
	}

	task, found := state.TaskByArn(taskARN)
	if !found {
		logger.Critical("No task was found for taskARN for task protection request", logger.Fields{
			loggerfield.TaskARN: taskARN,
		})
		return nil, http.StatusInternalServerError, errors.New("Failed to find a task for the request")
	}

	return task, http.StatusOK, nil
}

// GetTaskProtectionHandler returns a handler function for GetTaskProtection API
func GetTaskProtectionHandler(state dockerstate.TaskEngineState,
	cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		getTaskProtectionRequestType := "api/v1/GetTaskProtection"

		task, responseCode, err := getTaskFromRequest(state, r)
		if err != nil {
			writeJSONResponse(w, responseCode, err.Error(), getTaskProtectionRequestType)
			return
		}

		// TODO: Call ECS
		logger.Info("GetTaskProtection endpoint was called", logger.Fields{
			loggerfield.Cluster: cluster,
			loggerfield.TaskARN: task.Arn,
		})
		writeJSONResponse(w, http.StatusOK, "Ok", getTaskProtectionRequestType)
	}
}

// Writes the provided response to the ResponseWriter and handles any errors
func writeJSONResponse(w http.ResponseWriter, responseCode int, response interface{},
	requestType string) {
	bytes, err := json.Marshal(response)
	if err != nil {
		logger.Error("Agent API V1 Task Protection: failed to marshal response as JSON", logger.Fields{
			"response": response,
			"error":    err,
		})
		utils.WriteJSONToResponse(w, http.StatusInternalServerError, []byte(`{}`),
			requestType)
	} else {
		utils.WriteJSONToResponse(w, responseCode, bytes, requestType)
	}
}
