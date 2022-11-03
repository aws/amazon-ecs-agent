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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/ecsclient"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/agentapi/taskprotection/v1/types"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/agent/logger/field"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	ExpectedProtectionResponseLength = 1

	// timeout for ECS SDK calls
	// must be lower than server write timeout
	ecsCallTimeout       = 4 * time.Second
	ecsCallTimedOutError = "Timed out calling ECS Task Protection API"
)

// TaskProtectionPath Returns endpoint path for UpdateTaskProtection API
func TaskProtectionPath() string {
	return fmt.Sprintf(
		"/api/%s/task-protection/v1/state",
		utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx))
}

// TaskProtectionRequest is the Task protection request received from customers pending validation
type TaskProtectionRequest struct {
	ProtectionEnabled *bool
	ExpiresInMinutes  *int64
}

// TaskProtectionClientFactory implements TaskProtectionClientFactoryInterface
type TaskProtectionClientFactory struct {
	Region             string
	Endpoint           string
	AcceptInsecureCert bool
}

// UpdateTaskProtectionHandler returns an HTTP request handler function for
// UpdateTaskProtection API
func UpdateTaskProtectionHandler(state dockerstate.TaskEngineState, credentialsManager credentials.Manager,
	factory TaskProtectionClientFactoryInterface, cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		updateTaskProtectionRequestType := "api/UpdateTaskProtection/v1"

		var request TaskProtectionRequest
		jsonDecoder := json.NewDecoder(r.Body)
		jsonDecoder.DisallowUnknownFields()
		if err := jsonDecoder.Decode(&request); err != nil {
			logger.Error("UpdateTaskProtection: failed to decode request", logger.Fields{
				loggerfield.Error: err,
			})
			writeJSONResponse(w, http.StatusBadRequest,
				types.NewTaskProtectionResponseError(types.NewErrorResponsePtr("", ecs.ErrCodeInvalidParameterException,
					"UpdateTaskProtection: failed to decode request"), nil),
				updateTaskProtectionRequestType)
			return
		}

		task, statusCode, errorCode, err := getTaskFromRequest(state, r)
		if err != nil {
			writeJSONResponse(w, statusCode,
				types.NewTaskProtectionResponseError(types.NewErrorResponsePtr("", errorCode, err.Error()), nil),
				updateTaskProtectionRequestType)
			return
		}

		if request.ProtectionEnabled == nil {
			writeJSONResponse(w, http.StatusBadRequest,
				types.NewTaskProtectionResponseError(types.NewErrorResponsePtr(task.Arn, ecs.ErrCodeInvalidParameterException,
					"Invalid request: does not contain 'ProtectionEnabled' field"), nil),
				updateTaskProtectionRequestType)
			return
		}

		taskProtection := types.NewTaskProtection(*request.ProtectionEnabled, request.ExpiresInMinutes)

		logger.Info("UpdateTaskProtection endpoint was called", logger.Fields{
			loggerfield.Cluster:        cluster,
			loggerfield.TaskARN:        task.Arn,
			loggerfield.TaskProtection: taskProtection,
		})

		taskRoleCredential, ok := credentialsManager.GetTaskCredentials(task.GetCredentialsID())
		if !ok {
			err = fmt.Errorf("Invalid Request: no task IAM role credentials available for task")
			logger.Error(err.Error(), logger.Fields{
				loggerfield.TaskARN: task.Arn,
			})
			writeJSONResponse(w, http.StatusForbidden,
				types.NewTaskProtectionResponseError(types.NewErrorResponsePtr(task.Arn, ecs.ErrCodeAccessDeniedException, err.Error()), nil),
				updateTaskProtectionRequestType)
			return
		}
		ecsClient := factory.newTaskProtectionClient(taskRoleCredential)

		ctx, cancel := context.WithTimeout(r.Context(), ecsCallTimeout)
		defer cancel()
		response, err := ecsClient.UpdateTaskProtectionWithContext(ctx, &ecs.UpdateTaskProtectionInput{
			Cluster:           aws.String(cluster),
			ExpiresInMinutes:  taskProtection.GetExpiresInMinutes(),
			ProtectionEnabled: aws.Bool(taskProtection.GetProtectionEnabled()),
			Tasks:             aws.StringSlice([]string{task.Arn}),
		})

		if err != nil {
			errorCode, errorMsg, statusCode, reqId := getErrorCodeAndStatusCode(err)
			var requestIdString = ""
			if reqId != nil {
				requestIdString = *reqId
			}
			logger.Error("Got an exception when calling UpdateTaskProtection.", logger.Fields{
				loggerfield.Error:  err,
				"ErrorCode":        errorCode,
				"ExceptionMessage": errorMsg,
				"StatusCode":       statusCode,
				"RequestId":        requestIdString,
			})
			writeJSONResponse(w, statusCode, types.NewTaskProtectionResponseError(types.NewErrorResponsePtr(task.Arn, errorCode, errorMsg), reqId),
				updateTaskProtectionRequestType)
			return
		}

		logger.Debug("updateTaskProtection response:", logger.Fields{
			loggerfield.TaskProtection: response.ProtectedTasks,
			loggerfield.Reason:         response.Failures,
		})

		// there are no exceptions but there are failures when setting protection in scheduler
		if len(response.Failures) > 0 {
			if len(response.Failures) > ExpectedProtectionResponseLength {
				err := fmt.Errorf("expect at most %v failure in response, get %v", ExpectedProtectionResponseLength, len(response.Failures))
				logger.Error("Unexpected number of failures", logger.Fields{
					loggerfield.Error:   err,
					loggerfield.TaskARN: task.Arn,
				})
				writeJSONResponse(w, http.StatusInternalServerError, types.NewTaskProtectionResponseError(
					types.NewErrorResponsePtr(task.Arn, ecs.ErrCodeServerException, "Unexpected error occurred"), nil),
					updateTaskProtectionRequestType)
				return
			}
			writeJSONResponse(w, http.StatusOK, types.NewTaskProtectionResponseFailure(response.Failures[0]), updateTaskProtectionRequestType)
			return
		}
		if len(response.ProtectedTasks) > ExpectedProtectionResponseLength {
			err := fmt.Errorf("expect %v protectedTask in response when no failure, get %v", ExpectedProtectionResponseLength, len(response.ProtectedTasks))
			logger.Error("Unexpected number of protections", logger.Fields{
				loggerfield.Error:   err,
				loggerfield.TaskARN: task.Arn,
			})
			writeJSONResponse(w, http.StatusInternalServerError, types.NewTaskProtectionResponseError(
				types.NewErrorResponsePtr(task.Arn, ecs.ErrCodeServerException, "Unexpected error occurred"), nil),
				updateTaskProtectionRequestType)
			return
		}
		writeJSONResponse(w, http.StatusOK, types.NewTaskProtectionResponseProtection(response.ProtectedTasks[0]), updateTaskProtectionRequestType)
	}
}

// GetTaskProtectionHandler returns a handler function for GetTaskProtection API
func GetTaskProtectionHandler(state dockerstate.TaskEngineState, credentialsManager credentials.Manager,
	factory TaskProtectionClientFactoryInterface, cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		getTaskProtectionRequestType := "api/GetTaskProtection/v1"

		task, statusCode, errorCode, err := getTaskFromRequest(state, r)
		if err != nil {
			writeJSONResponse(w, statusCode,
				types.NewTaskProtectionResponseError(types.NewErrorResponsePtr("", errorCode, err.Error()), nil),
				getTaskProtectionRequestType)
			return
		}

		logger.Info("GetTaskProtection endpoint was called", logger.Fields{
			loggerfield.Cluster: cluster,
			loggerfield.TaskARN: task.Arn,
		})

		taskRoleCredential, ok := credentialsManager.GetTaskCredentials(task.GetCredentialsID())
		if !ok {
			err = fmt.Errorf("Invalid Request: no task IAM role credentials available for task")
			logger.Error(err.Error(), logger.Fields{
				loggerfield.TaskARN: task.Arn,
			})
			writeJSONResponse(w, http.StatusForbidden,
				types.NewTaskProtectionResponseError(types.NewErrorResponsePtr(task.Arn, ecs.ErrCodeAccessDeniedException, err.Error()), nil),
				getTaskProtectionRequestType)
			return
		}

		ecsClient := factory.newTaskProtectionClient(taskRoleCredential)

		ctx, cancel := context.WithTimeout(r.Context(), ecsCallTimeout)
		defer cancel()
		response, err := ecsClient.GetTaskProtectionWithContext(ctx, &ecs.GetTaskProtectionInput{
			Cluster: aws.String(cluster),
			Tasks:   aws.StringSlice([]string{task.Arn}),
		})

		if err != nil {
			errorCode, errorMsg, statusCode, reqId := getErrorCodeAndStatusCode(err)
			var requestIdString = ""
			if reqId != nil {
				requestIdString = *reqId
			}
			logger.Error("Got an exception when calling GetTaskProtection.", logger.Fields{
				loggerfield.Error:  err,
				"ErrorCode":        errorCode,
				"ExceptionMessage": errorMsg,
				"StatusCode":       statusCode,
				"RequestId":        requestIdString,
			})
			writeJSONResponse(w, statusCode, types.NewTaskProtectionResponseError(types.NewErrorResponsePtr(task.Arn, errorCode, errorMsg), reqId),
				getTaskProtectionRequestType)
			return
		}

		logger.Debug("getTaskProtection response:", logger.Fields{
			loggerfield.TaskProtection: response.ProtectedTasks,
			loggerfield.Reason:         response.Failures,
		})

		// there are no exceptions but there are failures when getting protection in scheduler
		if len(response.Failures) > 0 {
			if len(response.Failures) > ExpectedProtectionResponseLength {
				err := fmt.Errorf("expect at most %v failure in response, get %v", ExpectedProtectionResponseLength, len(response.Failures))
				logger.Error("Unexpected number of failures", logger.Fields{
					loggerfield.Error:   err,
					loggerfield.TaskARN: task.Arn,
				})
				writeJSONResponse(w, http.StatusInternalServerError, types.NewTaskProtectionResponseError(
					types.NewErrorResponsePtr(task.Arn, ecs.ErrCodeServerException, "Unexpected error occurred"), nil),
					getTaskProtectionRequestType)
				return
			}
			writeJSONResponse(w, http.StatusOK, types.NewTaskProtectionResponseFailure(response.Failures[0]), getTaskProtectionRequestType)
			return
		}

		if len(response.ProtectedTasks) > ExpectedProtectionResponseLength {
			err := fmt.Errorf("expect %v protectedTask in response when no failure, get %v", ExpectedProtectionResponseLength, len(response.ProtectedTasks))
			logger.Error("Unexpected number of protections", logger.Fields{
				loggerfield.Error:   err,
				loggerfield.TaskARN: task.Arn,
			})
			writeJSONResponse(w, http.StatusInternalServerError, types.NewTaskProtectionResponseError(
				types.NewErrorResponsePtr(task.Arn, ecs.ErrCodeServerException, "Unexpected error occurred"), nil),
				getTaskProtectionRequestType)
			return
		}
		writeJSONResponse(w, http.StatusOK, types.NewTaskProtectionResponseProtection(response.ProtectedTasks[0]), getTaskProtectionRequestType)
	}
}

// Helper function for retrieving credential from credentials manager and create ecs client
func (factory TaskProtectionClientFactory) newTaskProtectionClient(taskRoleCredential credentials.TaskIAMRoleCredentials) api.ECSTaskProtectionSDK {
	taskCredential := taskRoleCredential.GetIAMRoleCredentials()
	cfg := aws.NewConfig().
		WithCredentials(awscreds.NewStaticCredentials(taskCredential.AccessKeyID,
			taskCredential.SecretAccessKey,
			taskCredential.SessionToken)).
		WithRegion(factory.Region).
		WithHTTPClient(httpclient.New(ecsclient.RoundtripTimeout, factory.AcceptInsecureCert)).
		WithEndpoint(factory.Endpoint)

	ecsClient := ecs.New(session.Must(session.NewSession()), cfg)
	return ecsClient
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
			logger.Error(fmt.Sprintf("got an exception that does not implement RequestFailure interface but is an aws error. This should not happen, return statusCode 500 for whatever errorCode. Original err: %v.", err))
			return awsErr.Code(), msg, http.StatusInternalServerError, nil
		}
	} else {
		logger.Error(fmt.Sprintf("non aws error received: %v", err))
		return ecs.ErrCodeServerException, msg, http.StatusInternalServerError, nil
	}
}

// Helper function for finding task for the request
func getTaskFromRequest(state dockerstate.TaskEngineState, r *http.Request) (*apitask.Task, int, string, error) {
	taskARN, err := v3.GetTaskARNByRequest(r, state)
	if err != nil {
		logger.Error("Failed to find task ARN for task protection request", logger.Fields{
			loggerfield.Error: err,
		})
		return nil, http.StatusNotFound, ecs.ErrCodeResourceNotFoundException, errors.New("Invalid request: no task was found")
	}

	task, found := state.TaskByArn(taskARN)
	if !found {
		logger.Critical("No task was found for taskARN for task protection request", logger.Fields{
			loggerfield.TaskARN: taskARN,
		})
		return nil, http.StatusInternalServerError, ecs.ErrCodeServerException, errors.New("Failed to find a task for the request")
	}

	return task, http.StatusOK, "", nil
}

// Writes the provided response to the ResponseWriter and handles any errors
func writeJSONResponse(w http.ResponseWriter, statusCode int, response types.TaskProtectionResponse, requestType string) {
	bytes, err := json.Marshal(response)
	if err != nil {
		logger.Error("Agent API Task Protection V1: failed to marshal response as JSON", logger.Fields{
			"response":        response,
			loggerfield.Error: err,
		})
		utils.WriteJSONToResponse(w, http.StatusInternalServerError, []byte(`{}`),
			requestType)
	} else {
		utils.WriteJSONToResponse(w, statusCode, bytes, requestType)
	}
}
