package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/agentapi/v1/taskprotection/types"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/cihub/seelog"
)

// Returns endpoint path for PutTaskProtection API
func TaskProtectionPath() string {
	return fmt.Sprintf(
		"/api/v1/%s/task/protection",
		utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx))
}

// Task protection request received from customers pending validation
type taskProtectionRequest struct {
	ProtectionType           string
	ProtectionTimeoutMinutes *int
}

// PutTaskProtectionHandler returns an HTTP request handler function for
// PutTaskProtection API
func PutTaskProtectionHandler(state dockerstate.TaskEngineState,
	cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		putTaskProtectionRequestType := "v1/PutTaskProtection"

		var request taskProtectionRequest
		jsonDecoder := json.NewDecoder(r.Body)
		jsonDecoder.DisallowUnknownFields()
		if err := jsonDecoder.Decode(&request); err != nil {
			seelog.Errorf("PutTaskProtection: failed to decode request: %v", err)
			writeJSONResponse(w, http.StatusBadRequest,
				"Failed to decode request", putTaskProtectionRequestType)
			return
		}

		if request.ProtectionType == "" {
			writeJSONResponse(w, http.StatusBadRequest,
				"Invalid request: protection type is missing or empty",
				putTaskProtectionRequestType)
			return
		}

		taskProtection, err := types.NewTaskProtection(request.ProtectionType, request.ProtectionTimeoutMinutes)
		if err != nil {
			writeJSONResponse(w, http.StatusBadRequest,
				fmt.Sprintf("Invalid request: %v", err),
				putTaskProtectionRequestType)
			return
		}

		task, err := getTaskFromRequest(state, r)
		if err != nil {
			writeJSONResponse(w, http.StatusInternalServerError,
				fmt.Sprintf("Failed to find task: %v", err), putTaskProtectionRequestType)
			return
		}

		// TODO: Call ECS
		seelog.Infof("Would have called ECS.PutTaskProtection(%s, %s, %s, %s, %v)\n",
			cluster, task.ServiceName, task.Arn, taskProtection.GetProtectionType(),
			taskProtection.GetProtectionTimeoutMinutes())
		writeJSONResponse(w, http.StatusOK, "Ok", putTaskProtectionRequestType)
	}
}

// Helper function for finding task for the request
func getTaskFromRequest(state dockerstate.TaskEngineState, r *http.Request) (*apitask.Task, error) {
	taskARN, err := v3.GetTaskARNByRequest(r, state)
	if err != nil {
		return nil, fmt.Errorf("unable to get task ARN from request: %w", err)
	}

	task, found := state.TaskByArn(taskARN)
	if !found {
		return nil, fmt.Errorf("could not find task from task ARN '%v'", taskARN)
	}

	return task, nil
}

// GetTaskProtectionHandler returns a handler function for GetTaskProtection API
func GetTaskProtectionHandler(state dockerstate.TaskEngineState,
	cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		getTaskProtectionRequestType := "api/v1/GetTaskProtection"

		task, err := getTaskFromRequest(state, r)
		if err != nil {
			writeJSONResponse(w, http.StatusInternalServerError,
				fmt.Sprintf("Failed to find task: %v", err), getTaskProtectionRequestType)
			return
		}

		// TODO: Call ECS
		seelog.Infof("Would have called ECS.GetTaskProtection(%s, %s, %s)",
			cluster, task.ServiceName, task)
		writeJSONResponse(w, http.StatusOK, "Ok", getTaskProtectionRequestType)
	}
}

// Writes the provided response to the ResponseWriter and handles any errors
func writeJSONResponse(w http.ResponseWriter, responseCode int, response interface{},
	requestType string) {
	bytes, err := json.Marshal(response)
	if err != nil {
		seelog.Errorf("Agent API V1 failed to marshal response '%v' as JSON: %v",
			response, err)
		utils.WriteJSONToResponse(w, http.StatusInternalServerError, []byte(`{}`),
			requestType)
	} else {
		utils.WriteJSONToResponse(w, responseCode, bytes, requestType)
	}
}
