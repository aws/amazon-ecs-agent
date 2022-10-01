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
	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	ExpectedProtectionResponseLength = 1
	FailureReasonMissing             = "MISSING"
	FailureReasonInput               = "INPUT"
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
			writeJSONResponse(w, http.StatusBadRequest, nil,
				generateFailure(nil, aws.String(FailureReasonInput), aws.String("Failed to decode request")),
				updateTaskProtectionRequestType)
			return
		}

		task, responseCode, err := getTaskFromRequest(state, r)
		if err != nil {
			writeJSONResponse(w, responseCode, nil,
				generateFailure(nil, aws.String(FailureReasonMissing), aws.String(err.Error())),
				updateTaskProtectionRequestType)
			return
		}

		if request.ProtectionEnabled == nil {
			writeJSONResponse(w, http.StatusBadRequest, nil,
				generateFailure(aws.String(task.Arn), aws.String(FailureReasonInput), aws.String("Invalid request: does not contain 'ProtectionEnabled' field")),
				updateTaskProtectionRequestType)
			return
		}

		taskProtection, err := types.NewTaskProtection(*request.ProtectionEnabled, request.ExpiresInMinutes)
		if err != nil {
			writeJSONResponse(w, http.StatusBadRequest, nil,
				generateFailure(aws.String(task.Arn), aws.String(FailureReasonInput), aws.String(fmt.Sprintf("Invalid request: %v", err))),
				updateTaskProtectionRequestType)
			return
		}

		logger.Info("UpdateTaskProtection endpoint was called", logger.Fields{
			loggerfield.Cluster:        cluster,
			loggerfield.TaskARN:        task.Arn,
			loggerfield.TaskProtection: taskProtection,
		})

		ecsClient, responseCode, err := factory.newTaskProtectionClient(credentialsManager, task)
		if err != nil {
			writeJSONResponse(w, responseCode, nil,
				generateFailure(aws.String(task.Arn), aws.String(FailureReasonInput), aws.String(err.Error())),
				updateTaskProtectionRequestType)
			return
		}

		response, err := ecsClient.UpdateTaskProtection(&ecs.UpdateTaskProtectionInput{
			Cluster:           aws.String(cluster),
			ExpiresInMinutes:  taskProtection.GetExpiresInMinutes(),
			ProtectionEnabled: aws.Bool(taskProtection.GetProtectionEnabled()),
			Tasks:             aws.StringSlice([]string{task.Arn}),
		})

		if err != nil {
			exceptionType, statusCode := getExceptionTypeAndStatusCode(err)
			logger.Error("Got an exception when calling UpdateTaskProtection.", logger.Fields{
				"ExceptionType":   exceptionType,
				loggerfield.Error: err,
			})
			writeJSONResponse(w, statusCode, nil,
				generateFailure(aws.String(task.Arn), aws.String(exceptionType), aws.String(err.Error())),
				updateTaskProtectionRequestType)
			return
		}

		logger.Debug("updateTaskProtection response:", logger.Fields{
			loggerfield.TaskProtection: response.ProtectedTasks,
			loggerfield.Reason:         response.Failures,
		})

		// there are no exceptions but there are failures when setting protection in scheduler
		if len(response.Failures) > 0 {
			writeJSONResponse(w, http.StatusBadRequest, nil, response.Failures[0], updateTaskProtectionRequestType)
			return
		}
		if len(response.ProtectedTasks) != ExpectedProtectionResponseLength {
			logger.Error(fmt.Sprintf("expect %v protectedTask in response, get %v", ExpectedProtectionResponseLength, len(response.ProtectedTasks)))
			writeJSONResponse(w, http.StatusInternalServerError, nil,
				generateFailure(aws.String(task.Arn), aws.String(FailureReasonInput), aws.String("An unexpected failure occurred.")),
				updateTaskProtectionRequestType)
			return
		}
		writeJSONResponse(w, http.StatusOK, response.ProtectedTasks[0], nil, updateTaskProtectionRequestType)
	}
}

// Helper function for retrieving credential from credentials manager and create ecs client
func (factory TaskProtectionClientFactory) newTaskProtectionClient(credentialsManager credentials.Manager, task *apitask.Task) (api.ECSTaskProtectionSDK, int, error) {
	taskRoleCredential, ok := credentialsManager.GetTaskCredentials(task.GetCredentialsID())
	if !ok {
		return nil, http.StatusBadRequest, errors.New("Invalid Request: no task IAM role credentials available for task")
	}
	taskCredential := taskRoleCredential.GetIAMRoleCredentials()
	cfg := aws.NewConfig().
		WithCredentials(awscreds.NewStaticCredentials(taskCredential.AccessKeyID,
			taskCredential.SecretAccessKey,
			taskCredential.SessionToken)).
		WithRegion(factory.Region).
		WithHTTPClient(httpclient.New(ecsclient.RoundtripTimeout, factory.AcceptInsecureCert)).
		WithEndpoint(factory.Endpoint)

	ecsClient := ecs.New(session.Must(session.NewSession()), cfg)
	return ecsClient, http.StatusOK, nil
}

// Helper function to parse error and get http status code
func getExceptionTypeAndStatusCode(err error) (string, int) {
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case ecs.ErrCodeServerException:
			return aerr.Code(), http.StatusInternalServerError
		case ecs.ErrCodeAccessDeniedException,
			ecs.ErrCodeClientException,
			ecs.ErrCodeClusterNotFoundException,
			ecs.ErrCodeInvalidParameterException,
			ecs.ErrCodeResourceNotFoundException,
			ecs.ErrCodeUnsupportedFeatureException:
			return aerr.Code(), http.StatusBadRequest
		default:
			logger.Error(fmt.Sprintf("errorType %s is not in expected exception types", aerr.Code()))
			return aerr.Code(), http.StatusInternalServerError
		}
	} else {
		logger.Error(fmt.Sprintf("non aws error received: %v", err))
		return "", http.StatusInternalServerError
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
func GetTaskProtectionHandler(state dockerstate.TaskEngineState, credentialsManager credentials.Manager,
	factory TaskProtectionClientFactoryInterface, cluster string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		getTaskProtectionRequestType := "api/GetTaskProtection/v1"

		task, responseCode, err := getTaskFromRequest(state, r)
		if err != nil {
			writeJSONResponse(w, responseCode, nil,
				generateFailure(nil, aws.String(FailureReasonMissing), aws.String(err.Error())),
				getTaskProtectionRequestType)
			return
		}

		logger.Info("GetTaskProtection endpoint was called", logger.Fields{
			loggerfield.Cluster: cluster,
			loggerfield.TaskARN: task.Arn,
		})

		ecsClient, responseCode, err := factory.newTaskProtectionClient(credentialsManager, task)
		if err != nil {
			writeJSONResponse(w, responseCode, nil,
				generateFailure(aws.String(task.Arn), aws.String(FailureReasonInput), aws.String(err.Error())),
				getTaskProtectionRequestType)
			return
		}

		response, err := ecsClient.GetTaskProtection(&ecs.GetTaskProtectionInput{
			Cluster: aws.String(cluster),
			Tasks:   aws.StringSlice([]string{task.Arn}),
		})

		if err != nil {
			exceptionType, statusCode := getExceptionTypeAndStatusCode(err)
			logger.Error("Got an exception when calling GetTaskProtection.", logger.Fields{
				"ExceptionType":   exceptionType,
				loggerfield.Error: err,
			})
			writeJSONResponse(w, statusCode, nil,
				generateFailure(aws.String(task.Arn), aws.String(exceptionType), aws.String(err.Error())),
				getTaskProtectionRequestType)
			return
		}

		logger.Debug("getTaskProtection response:", logger.Fields{
			loggerfield.TaskProtection: response.ProtectedTasks,
			loggerfield.Reason:         response.Failures,
		})

		// there are no exceptions but there are failures when getting protection in scheduler
		if len(response.Failures) > 0 {
			writeJSONResponse(w, http.StatusBadRequest, nil, response.Failures[0], getTaskProtectionRequestType)
			return
		}

		if len(response.ProtectedTasks) != ExpectedProtectionResponseLength {
			logger.Error(fmt.Sprintf("expect %v protectedTask in response, get %v", ExpectedProtectionResponseLength, len(response.ProtectedTasks)))
			writeJSONResponse(w, http.StatusInternalServerError, nil,
				generateFailure(aws.String(task.Arn), aws.String(FailureReasonInput), aws.String("An unexpected failure occurred.")),
				getTaskProtectionRequestType)
			return
		}
		writeJSONResponse(w, http.StatusOK, response.ProtectedTasks[0], nil, getTaskProtectionRequestType)
	}
}

// generateFailure generates a failure in ecs.Failure format for ECS exceptions and Agent input validations
func generateFailure(taskArn *string, reason *string, detail *string) *ecs.Failure {
	return &ecs.Failure{
		Arn:    taskArn,
		Reason: reason,
		Detail: detail,
	}
}

// Writes the provided response to the ResponseWriter and handles any errors
func writeJSONResponse(w http.ResponseWriter, responseCode int, protectedTask *ecs.ProtectedTask, failure *ecs.Failure,
	requestType string) {
	response := types.NewTaskProtectionResponse(protectedTask, failure)
	bytes, err := json.Marshal(response)
	if err != nil {
		logger.Error("Agent API Task Protection V1: failed to marshal response as JSON", logger.Fields{
			"response":        response,
			loggerfield.Error: err,
		})
		utils.WriteJSONToResponse(w, http.StatusInternalServerError, []byte(`{}`),
			requestType)
	} else {
		utils.WriteJSONToResponse(w, responseCode, bytes, requestType)
	}
}
