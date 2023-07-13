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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/taskprotection/v1/types"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	v4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/gorilla/mux"
)

const (
	expectedProtectionResponseLength = 1
	ecsCallTimedOutError             = "Timed out calling ECS Task Protection API"
	taskMetadataFetchFailureMsg      = "Failed to find a task for the request"
)

// TaskProtectionPath Returns endpoint path for UpdateTaskProtection API
func TaskProtectionPath() string {
	return fmt.Sprintf(
		"/api/%s/task-protection/v1/state",
		utils.ConstructMuxVar(v4.EndpointContainerIDMuxName, utils.AnythingButSlashRegEx))
}

// TaskProtectionRequest is the Task protection request received from customers pending validation
type TaskProtectionRequest struct {
	ProtectionEnabled *bool
	ExpiresInMinutes  *int64
}

// GetTaskProtectionHandler returns a handler function for GetTaskProtection API
func GetTaskProtectionHandler(
	agentState state.AgentState,
	credentialsManager credentials.Manager,
	factory TaskProtectionClientFactoryInterface,
	cluster string,
	metricsFactory metrics.EntryFactory,
	ecsCallTimeout time.Duration,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		requestType := "api/GetTaskProtection/v1"

		// Initialize metrics
		successMetric := metricsFactory.New(metrics.GetTaskProtectionMetricName)

		// Find task metadata
		task, errResponseCode, errResponseBody := getTaskMetadata(r, agentState, requestType)
		if errResponseBody != nil {
			utils.WriteJSONResponse(w, errResponseCode, errResponseBody, requestType)
			if utils.Is5XXStatus(errResponseCode) {
				successMetric.WithCount(0).Done(nil)
			}
			return
		}
		logger.Info("GetTaskProtection endpoint was called", logger.Fields{
			field.Cluster: cluster,
			field.TaskARN: task.TaskARN,
		})

		// Find task role creds
		taskCreds, errResponseCode, errResponseBody := getTaskCredentials(credentialsManager, *task)
		if errResponseBody != nil {
			utils.WriteJSONResponse(w, errResponseCode, errResponseBody, requestType)
			successMetric.WithCount(0).Done(nil)
			return
		}

		// Call ECS TaskProtection API
		ecsClient := factory.NewTaskProtectionClient(*taskCreds)
		ctx, cancel := context.WithTimeout(r.Context(), ecsCallTimeout)
		defer cancel()
		responseBody, err := ecsClient.GetTaskProtectionWithContext(ctx, &ecs.GetTaskProtectionInput{
			Cluster: aws.String(cluster),
			Tasks:   aws.StringSlice([]string{task.TaskARN}),
		})
		if err != nil {
			errResponseCode, errResponseBody := logAndHandleECSError(err, *task, requestType)
			utils.WriteJSONResponse(w, errResponseCode, errResponseBody, requestType)
			successMetric.WithCount(0).Done(nil)
			return
		}

		// Validate ECS response
		errResponseCode, errResponseBody = logAndValidateECSResponse(
			responseBody.ProtectedTasks, responseBody.Failures, *task, requestType)
		if errResponseBody != nil {
			utils.WriteJSONResponse(w, errResponseCode, errResponseBody, requestType)
			successMetric.WithCount(0).Done(nil)
			return
		}

		// ECS call was successful
		utils.WriteJSONResponse(w, http.StatusOK,
			types.NewTaskProtectionResponseProtection(responseBody.ProtectedTasks[0]), requestType)
		successMetric.WithCount(1).Done(nil)
	}
}

// UpdateTaskProtectionHandler returns an HTTP request handler function for UpdateTaskProtection API
func UpdateTaskProtectionHandler(
	agentState state.AgentState,
	credentialsManager credentials.Manager,
	factory TaskProtectionClientFactoryInterface,
	cluster string,
	metricsFactory metrics.EntryFactory,
	ecsCallTimeout time.Duration,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		requestType := "api/UpdateTaskProtection/v1"

		// Decode the request
		var request TaskProtectionRequest
		jsonDecoder := json.NewDecoder(r.Body)
		jsonDecoder.DisallowUnknownFields()
		if err := jsonDecoder.Decode(&request); err != nil {
			logger.Error("UpdateTaskProtection: failed to decode request", logger.Fields{
				field.Error: err,
			})
			utils.WriteJSONResponse(w, http.StatusBadRequest,
				types.NewTaskProtectionResponseError(types.NewErrorResponsePtr(
					"",
					ecs.ErrCodeInvalidParameterException,
					"UpdateTaskProtection: failed to decode request",
				), nil),
				requestType)
			return
		}

		// Initialize metrics
		successMetric := metricsFactory.New(metrics.UpdateTaskProtectionMetricName)

		// Find task metadata
		task, errResponseCode, errResponseBody := getTaskMetadata(r, agentState, requestType)
		if errResponseBody != nil {
			utils.WriteJSONResponse(w, errResponseCode, errResponseBody, requestType)
			if utils.Is5XXStatus(errResponseCode) {
				successMetric.WithCount(0).Done(nil)
			}
			return
		}
		logger.Info("GetTaskProtection endpoint was called", logger.Fields{
			field.Cluster: cluster,
			field.TaskARN: task.TaskARN,
		})

		// Validate the request
		if request.ProtectionEnabled == nil {
			responseErr := types.NewErrorResponsePtr(task.TaskARN, ecs.ErrCodeInvalidParameterException,
				"Invalid request: does not contain 'ProtectionEnabled' field")
			response := types.NewTaskProtectionResponseError(responseErr, nil)
			utils.WriteJSONResponse(w, http.StatusBadRequest, response, requestType)
			return
		}

		// Prepare ECS request body
		taskProtection := types.NewTaskProtection(*request.ProtectionEnabled, request.ExpiresInMinutes)
		logger.Info("UpdateTaskProtection endpoint was called", logger.Fields{
			field.Cluster:        cluster,
			field.TaskARN:        task.TaskARN,
			field.TaskProtection: taskProtection,
			field.RequestType:    requestType,
		})

		// Find task role creds
		taskCreds, errResponseCode, errResponseBody := getTaskCredentials(credentialsManager, *task)
		if errResponseBody != nil {
			utils.WriteJSONResponse(w, errResponseCode, errResponseBody, requestType)
			successMetric.WithCount(0).Done(nil)
			return
		}

		// Call ECS TaskProtection API
		ecsClient := factory.NewTaskProtectionClient(*taskCreds)
		ctx, cancel := context.WithTimeout(r.Context(), ecsCallTimeout)
		defer cancel()
		response, err := ecsClient.UpdateTaskProtectionWithContext(ctx, &ecs.UpdateTaskProtectionInput{
			Cluster:           aws.String(cluster),
			ExpiresInMinutes:  taskProtection.GetExpiresInMinutes(),
			ProtectionEnabled: aws.Bool(taskProtection.GetProtectionEnabled()),
			Tasks:             aws.StringSlice([]string{task.TaskARN}),
		})
		if err != nil {
			errResponseCode, errResponseBody := logAndHandleECSError(err, *task, requestType)
			utils.WriteJSONResponse(w, errResponseCode, errResponseBody, requestType)
			successMetric.WithCount(0).Done(nil)
			return
		}

		// Validate ECS response
		errResponseCode, errResponseBody = logAndValidateECSResponse(
			response.ProtectedTasks, response.Failures, *task, requestType)
		if errResponseBody != nil {
			utils.WriteJSONResponse(w, errResponseCode, errResponseBody, requestType)
			successMetric.WithCount(0).Done(nil)
			return
		}

		// ECS call was successful
		utils.WriteJSONResponse(w, http.StatusOK,
			types.NewTaskProtectionResponseProtection(response.ProtectedTasks[0]), requestType)
		successMetric.WithCount(1).Done(nil)
	}
}

// Helper function for retrieving task metadata for the request
func getTaskMetadata(
	r *http.Request,
	agentState state.AgentState,
	requestType string,
) (*state.TaskResponse, int, *types.TaskProtectionResponse) {
	endpointContainerID := mux.Vars(r)[v4.EndpointContainerIDMuxName]
	task, err := agentState.GetTaskMetadata(endpointContainerID)
	if err != nil {
		logger.Error("Failed to get v4 task metadata", logger.Fields{
			field.TMDSEndpointContainerID: endpointContainerID,
			field.Error:                   err,
			field.RequestType:             requestType,
		})

		responseCode, responseBody := getTaskMetadataErrorResponse(
			endpointContainerID, err, requestType)
		return nil, responseCode, &responseBody
	}

	return &task, 0, nil
}

// Helper function for retrieving task role credentials
func getTaskCredentials(
	credentialsManager credentials.Manager,
	task state.TaskResponse,
) (*credentials.TaskIAMRoleCredentials, int, *types.TaskProtectionResponse) {
	taskRoleCredential, ok := credentialsManager.GetTaskCredentials(task.CredentialsID)
	if !ok {
		errMsg := "Invalid Request: no task IAM role credentials available for task"
		logger.Error(errMsg, logger.Fields{field.TaskARN: task.TaskARN})
		responseErr := types.NewErrorResponsePtr(task.TaskARN, ecs.ErrCodeAccessDeniedException, errMsg)
		response := types.NewTaskProtectionResponseError(responseErr, nil)
		return nil, http.StatusForbidden, &response
	}

	return &taskRoleCredential, 0, nil
}

// Helper function for logging and handling error that occurred when calling ECS TaskProtection API
func logAndHandleECSError(
	err error,
	task state.TaskResponse,
	requestType string,
) (int, types.TaskProtectionResponse) {
	errorCode, errorMsg, statusCode, reqId := getErrorCodeAndStatusCode(err)
	var requestIdString = ""
	if reqId != nil {
		requestIdString = *reqId
	}

	logger.Error("Got an exception when calling TaskProtection API", logger.Fields{
		field.Error:        err,
		"ErrorCode":        errorCode,
		"ExceptionMessage": errorMsg,
		"StatusCode":       statusCode,
		"RequestId":        requestIdString,
		field.RequestType:  requestType,
	})

	responseErr := types.NewErrorResponsePtr(task.TaskARN, errorCode, errorMsg)
	response := types.NewTaskProtectionResponseError(responseErr, reqId)

	return statusCode, response
}

// Helper function for logging and validating ECS TaskProtection API response
func logAndValidateECSResponse(
	protectedTasks []*ecs.ProtectedTask,
	failures []*ecs.Failure,
	task state.TaskResponse,
	requestType string,
) (int, *types.TaskProtectionResponse) {
	logger.Debug("getTaskProtection response:", logger.Fields{
		field.TaskProtection: protectedTasks,
		field.Reason:         failures,
	})

	if len(failures) > 0 {
		if len(failures) > expectedProtectionResponseLength {
			err := fmt.Errorf(
				"expect at most %v failure in response, get %v",
				expectedProtectionResponseLength, len(failures))
			logger.Error("Unexpected number of failures", logger.Fields{
				field.Error:       err,
				field.TaskARN:     task.TaskARN,
				field.RequestType: requestType,
			})
			responseErr := types.NewErrorResponsePtr(
				task.TaskARN, ecs.ErrCodeServerException, "Unexpected error occurred")
			response := types.NewTaskProtectionResponseError(responseErr, nil)
			return http.StatusInternalServerError, &response
		}

		response := types.NewTaskProtectionResponseFailure(failures[0])
		return http.StatusOK, &response
	}

	if len(protectedTasks) > expectedProtectionResponseLength {
		err := fmt.Errorf(
			"expect %v protectedTask in response when no failure, get %v",
			expectedProtectionResponseLength, len(protectedTasks))
		logger.Error("Unexpected number of protections", logger.Fields{
			field.Error:       err,
			field.TaskARN:     task.TaskARN,
			field.RequestType: requestType,
		})

		responseErr := types.NewErrorResponsePtr(
			task.TaskARN, ecs.ErrCodeServerException, "Unexpected error occurred")
		response := types.NewTaskProtectionResponseError(responseErr, nil)
		return http.StatusInternalServerError, &response
	}

	return 0, nil
}

// Returns an appropriate HTTP response status code and body for the task metadata fetch error.
func getTaskMetadataErrorResponse(
	endpointContainerID string,
	err error,
	requestType string,
) (int, types.TaskProtectionResponse) {
	var errContainerLookupFailed *state.ErrorLookupFailure
	if errors.As(err, &errContainerLookupFailed) {
		responseErr := types.NewErrorResponsePtr(
			"", ecs.ErrCodeResourceNotFoundException, taskMetadataFetchFailureMsg)
		return http.StatusNotFound, types.NewTaskProtectionResponseError(responseErr, nil)
	}

	var errFailedToGetContainerMetadata *state.ErrorMetadataFetchFailure
	if errors.As(err, &errFailedToGetContainerMetadata) {
		responseErr := types.NewErrorResponsePtr(
			"", ecs.ErrCodeServerException, taskMetadataFetchFailureMsg)
		return http.StatusInternalServerError, types.NewTaskProtectionResponseError(responseErr, nil)
	}

	logger.Error("Unknown error encountered when handling task metadata fetch failure", logger.Fields{
		field.Error:       err,
		field.RequestType: requestType,
	})

	responseErr := types.NewErrorResponsePtr("", ecs.ErrCodeServerException, taskMetadataFetchFailureMsg)
	return http.StatusInternalServerError, types.NewTaskProtectionResponseError(responseErr, nil)
}

// Helper function to parse error to get ErrorCode, ExceptionMessage, HttpStatusCode, RequestID.
// RequestID will be empty if the request is not able to reach AWS
func getErrorCodeAndStatusCode(err error) (string, string, int, *string) {
	msg := err.Error()
	// The error is a Generic AWS Error with Code, Message, and original error (if any)
	if awsErr, ok := err.(awserr.Error); ok {
		// The error is an AWS service error occurred
		msg = awsErr.Message()
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			reqId := reqErr.RequestID()
			return awsErr.Code(), msg, reqErr.StatusCode(), &reqId
		} else if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			return aerr.Code(), ecsCallTimedOutError, http.StatusGatewayTimeout, nil
		} else {
			logger.Error(fmt.Sprintf(
				"got an exception that does not implement RequestFailure interface but is an aws error. This should not happen, return statusCode 500 for whatever errorCode. Original err: %v.",
				err))
			return awsErr.Code(), msg, http.StatusInternalServerError, nil
		}
	} else {
		logger.Error(fmt.Sprintf("non aws error received: %v", err))
		return ecs.ErrCodeServerException, msg, http.StatusInternalServerError, nil
	}
}
