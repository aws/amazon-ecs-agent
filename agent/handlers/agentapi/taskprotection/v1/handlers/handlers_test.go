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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/handlers/agentapi/taskprotection/v1/types"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

const (
	testAccessKey              = "accessKey"
	testSecretKey              = "secretKey"
	testSessionToken           = "sessionToken"
	testCluster                = "cluster"
	testRegion                 = "region"
	testECSEndpoint            = "endpoint"
	testTaskCredentialsId      = "taskCredentialsId"
	testV3EndpointId           = "endpointId"
	testTaskArn                = "taskArn"
	testServiceName            = "serviceName"
	testAcceptInsecureCert     = false
	protectionEnabledFieldName = "ProtectionEnabled"
	expiresInMinutesFieldName  = "ExpiresInMinutes"
	testExpiresInMinutes       = 5
	testProtectionEnabled      = true
	testRequestID              = "requestID"
	testFailureReason          = "failureReason"
)

// Tests the path for UpdateTaskProtection API
func TestTaskProtectionPath(t *testing.T) {
	assert.Equal(t, "/api/{v3EndpointIDMuxName:[^/]*}/task-protection/v1/state", TaskProtectionPath())
}

// TestGetECSClientHappyCase tests newTaskProtectionClient uses credential in credentials manager and
// returns an ECS client with correct status code and error
func TestGetECSClientHappyCase(t *testing.T) {

	testIAMRoleCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     testAccessKey,
			SecretAccessKey: testSecretKey,
			SessionToken:    testSessionToken,
		},
	}

	factory := TaskProtectionClientFactory{
		Region: testRegion, Endpoint: testECSEndpoint, AcceptInsecureCert: testAcceptInsecureCert,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ret := factory.newTaskProtectionClient(testIAMRoleCredentials)
	_, ok := ret.(api.ECSTaskProtectionSDK)

	// Assert response
	assert.True(t, ok)
}

func getRequestWithUnknownFields(t *testing.T) map[string]interface{} {
	request := TaskProtectionRequest{ProtectionEnabled: utils.BoolPtr(false)}
	requestJSON, err := json.Marshal(request)
	assert.NoError(t, err)

	var rawRequest map[string]interface{}
	err = json.Unmarshal(requestJSON, &rawRequest)
	assert.NoError(t, err)
	rawRequest["UnknownField"] = 5
	return rawRequest
}

// Helper function for running tests for UpdateTaskProtection handler
func testUpdateTaskProtectionHandler(t *testing.T, state dockerstate.TaskEngineState,
	v3EndpointID string, credentialsManager credentials.Manager, factory TaskProtectionClientFactoryInterface,
	request interface{}, expectedResponse interface{}, expectedResponseCode int) {
	// Prepare request
	requestBytes, err := json.Marshal(request)
	assert.NoError(t, err)
	bodyReader := bytes.NewReader(requestBytes)
	req, err := http.NewRequest("PUT", "", bodyReader)
	assert.NoError(t, err)
	req = mux.SetURLVars(req, map[string]string{v3.V3EndpointIDMuxName: v3EndpointID})

	// Call handler
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(UpdateTaskProtectionHandler(state, credentialsManager, factory, testCluster))
	handler.ServeHTTP(rr, req)

	expectedResponseJSON, err := json.Marshal(expectedResponse)
	assert.NoError(t, err, "Expected response must be JSON encodable")

	// Assert response
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Equal(t, expectedResponseCode, rr.Code)
	responseBody, err := io.ReadAll(rr.Body)
	assert.NoError(t, err, "Failed to read response body")
	assert.Equal(t, string(expectedResponseJSON), string(responseBody))
}

func generateRequestIdPtr() *string {
	requestIdString := testRequestID
	return &requestIdString
}

// TestUpdateTaskProtectionHandler_InputValidationsDecodeError tests UpdateTaskProtection handler's
// behavior with different invalid inputs with decode error
func TestUpdateTaskProtectionHandler_InputValidationsDecodeError(t *testing.T) {
	testCases := []struct {
		name          string
		request       interface{}
		expectedError *types.ErrorResponse
	}{
		{
			name: "InvalidTypes",
			request: &map[string]interface{}{
				protectionEnabledFieldName: true,
				expiresInMinutesFieldName:  "badType",
			},
			expectedError: &types.ErrorResponse{Code: ecs.ErrCodeInvalidParameterException, Message: "UpdateTaskProtection: failed to decode request"},
		},
		{
			name:          "UnknownFieldsInRequest",
			request:       getRequestWithUnknownFields(t),
			expectedError: &types.ErrorResponse{Code: ecs.ErrCodeInvalidParameterException, Message: "UpdateTaskProtection: failed to decode request"},
		},
		{
			name:          "InvalidJSONRequest",
			request:       "",
			expectedError: &types.ErrorResponse{Code: ecs.ErrCodeInvalidParameterException, Message: "UpdateTaskProtection: failed to decode request"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			expectedResponse := types.TaskProtectionResponse{Error: tc.expectedError}
			testUpdateTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
				testV3EndpointId, nil, nil, tc.request, expectedResponse, http.StatusBadRequest)
		})
	}
}

// TestUpdateTaskProtectionHandlerTaskARNNotFound tests UpdateTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestUpdateTaskProtectionHandlerTaskARNNotFound(t *testing.T) {
	request := TaskProtectionRequest{ProtectionEnabled: utils.BoolPtr(false)}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return("", false)

	expectedResponse := types.TaskProtectionResponse{
		Error: &types.ErrorResponse{
			Code:    ecs.ErrCodeResourceNotFoundException,
			Message: "Invalid request: no task was found",
		},
	}
	testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, nil, nil, request,
		expectedResponse, http.StatusNotFound)
}

// TestUpdateTaskProtectionHandlerTaskNotFound tests UpdateTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestUpdateTaskProtectionHandlerTaskNotFound(t *testing.T) {
	request := TaskProtectionRequest{ProtectionEnabled: utils.BoolPtr(false)}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(nil, false)

	expectedResponse := types.TaskProtectionResponse{
		Error: &types.ErrorResponse{
			Code:    ecs.ErrCodeServerException,
			Message: "Failed to find a task for the request",
		},
	}

	testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, nil, nil, request,
		expectedResponse, http.StatusInternalServerError)
}

// TestUpdateTaskProtectionHandler_EmptyRequest tests UpdateTaskProtection handler's behavior with empty inputs
func TestUpdateTaskProtectionHandler_EmptyRequest(t *testing.T) {
	expectedError := &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeInvalidParameterException, Message: "Invalid request: does not contain 'ProtectionEnabled' field"}
	testTask := task.Task{
		Arn:         testTaskArn,
		ServiceName: testServiceName,
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(&testTask, true)
	expectedResponse := types.TaskProtectionResponse{Error: expectedError}
	testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, nil, nil,
		nil, expectedResponse, http.StatusBadRequest)
}

// TestUpdateTaskProtectionHandlerTaskRoleCredentialsNotFound tests UpdateTaskProtection handler's
// behavior when task IAM role credential is not found for the request.
func TestUpdateTaskProtectionHandlerTaskRoleCredentialsNotFound(t *testing.T) {
	request := TaskProtectionRequest{
		ProtectionEnabled: utils.BoolPtr(true),
	}

	testTask := task.Task{
		Arn:         testTaskArn,
		ServiceName: testServiceName,
	}
	testTask.SetCredentialsID(testTaskCredentialsId)

	factory := TaskProtectionClientFactory{
		Region: testRegion, Endpoint: testECSEndpoint, AcceptInsecureCert: testAcceptInsecureCert,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockManager := mock_credentials.NewMockManager(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(&testTask, true)
	mockManager.EXPECT().GetTaskCredentials(gomock.Eq(testTaskCredentialsId)).Return(credentials.TaskIAMRoleCredentials{}, false)

	expectedResponse := types.TaskProtectionResponse{
		Error: &types.ErrorResponse{
			Arn:     testTaskArn,
			Code:    ecs.ErrCodeAccessDeniedException,
			Message: "Invalid Request: no task IAM role credentials available for task",
		},
	}

	testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, mockManager, factory, request,
		expectedResponse, http.StatusForbidden)
}

// TestUpdateTaskProtectionHandler_PostCall tests UpdateTaskProtection handler's
// behavior when request successfully reached ECS and get response
func TestUpdateTaskProtectionHandler_PostCall(t *testing.T) {
	testCases := []struct {
		name               string
		ecsError           error
		ecsResponse        *ecs.UpdateTaskProtectionOutput
		expectedProtection *ecs.ProtectedTask
		expectedFailure    *ecs.Failure
		expectedError      *types.ErrorResponse
		expectedRequestId  *string
		expectedStatusCode int
		time               time.Time
	}{
		{
			name:               "RequestFailure_ServerException",
			ecsError:           awserr.NewRequestFailure(awserr.New(ecs.ErrCodeServerException, "error message", nil), http.StatusInternalServerError, testRequestID),
			ecsResponse:        &ecs.UpdateTaskProtectionOutput{},
			expectedError:      &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeServerException, Message: "error message"},
			expectedStatusCode: http.StatusInternalServerError,
			expectedRequestId:  generateRequestIdPtr(),
		},
		{
			name:               "RequestFailure_OtherExceptions",
			ecsError:           awserr.NewRequestFailure(awserr.New(ecs.ErrCodeAccessDeniedException, "error message", nil), http.StatusBadRequest, testRequestID),
			ecsResponse:        &ecs.UpdateTaskProtectionOutput{},
			expectedError:      &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeAccessDeniedException, Message: "error message"},
			expectedStatusCode: http.StatusBadRequest,
			expectedRequestId:  generateRequestIdPtr(),
		},
		{
			name:               "NonRequestFailureAwsError",
			ecsError:           awserr.New(ecs.ErrCodeInvalidParameterException, "error message", nil),
			ecsResponse:        &ecs.UpdateTaskProtectionOutput{},
			expectedError:      &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeInvalidParameterException, Message: "error message"},
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:        "Agent timeout",
			ecsError:    awserr.New(request.CanceledErrorCode, "request cancelled", nil),
			ecsResponse: &ecs.UpdateTaskProtectionOutput{},
			expectedError: &types.ErrorResponse{
				Arn:     testTaskArn,
				Code:    request.CanceledErrorCode,
				Message: ecsCallTimedOutError,
			},
			expectedStatusCode: http.StatusGatewayTimeout,
		},
		{
			name:               "NonAwsError",
			ecsError:           fmt.Errorf("error message"),
			ecsResponse:        &ecs.UpdateTaskProtectionOutput{},
			expectedError:      &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeServerException, Message: "error message"},
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:     "Failure",
			ecsError: nil,
			ecsResponse: &ecs.UpdateTaskProtectionOutput{
				Failures: []*ecs.Failure{{
					Arn:    aws.String(testTaskArn),
					Reason: aws.String(testFailureReason),
				}},
				ProtectedTasks: []*ecs.ProtectedTask{},
			},
			expectedFailure: &ecs.Failure{
				Arn:    aws.String(testTaskArn),
				Reason: aws.String(testFailureReason),
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:     "SuccessProtected",
			ecsError: nil,
			ecsResponse: &ecs.UpdateTaskProtectionOutput{
				Failures: []*ecs.Failure{},
				ProtectedTasks: []*ecs.ProtectedTask{{
					ProtectionEnabled: aws.Bool(true),
					ExpirationDate:    aws.Time(time.UnixMilli(0)),
					TaskArn:           aws.String(testTaskArn),
				}},
			},
			expectedProtection: &ecs.ProtectedTask{
				ProtectionEnabled: aws.Bool(true),
				ExpirationDate:    aws.Time(time.UnixMilli(0)),
				TaskArn:           aws.String(testTaskArn),
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:     "SuccessNotProtected",
			ecsError: nil,
			ecsResponse: &ecs.UpdateTaskProtectionOutput{
				Failures: []*ecs.Failure{},
				ProtectedTasks: []*ecs.ProtectedTask{{
					ProtectionEnabled: aws.Bool(false),
					ExpirationDate:    nil,
					TaskArn:           aws.String(testTaskArn),
				}},
			},
			expectedProtection: &ecs.ProtectedTask{
				ProtectionEnabled: aws.Bool(false),
				ExpirationDate:    nil,
				TaskArn:           aws.String(testTaskArn),
			},
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			request := TaskProtectionRequest{
				ProtectionEnabled: utils.BoolPtr(testProtectionEnabled),
				ExpiresInMinutes:  utils.Int64Ptr(testExpiresInMinutes),
			}

			testTask := task.Task{
				Arn:         testTaskArn,
				ServiceName: testServiceName,
			}
			testTask.SetCredentialsID(testTaskCredentialsId)

			mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
			mockManager := mock_credentials.NewMockManager(ctrl)
			mockFactory := NewMockTaskProtectionClientFactoryInterface(ctrl)
			mockECSClient := mock_api.NewMockECSTaskProtectionSDK(ctrl)

			mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
			mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(&testTask, true)
			mockManager.EXPECT().GetTaskCredentials(gomock.Eq(testTaskCredentialsId)).Return(credentials.TaskIAMRoleCredentials{}, true)
			mockFactory.EXPECT().newTaskProtectionClient(gomock.Eq(credentials.TaskIAMRoleCredentials{})).Return(mockECSClient)
			mockECSClient.EXPECT().
				UpdateTaskProtectionWithContext(gomock.Any(), gomock.Any()).
				Return(tc.ecsResponse, tc.ecsError)

			expectedResponse := types.TaskProtectionResponse{
				Protection: tc.expectedProtection,
				Failure:    tc.expectedFailure,
				Error:      tc.expectedError,
				RequestID:  tc.expectedRequestId,
			}

			testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, mockManager, mockFactory, request, expectedResponse, tc.expectedStatusCode)
		})
	}
}

func testGetTaskProtectionHandler(t *testing.T, state dockerstate.TaskEngineState,
	v3EndpointID string, credentialsManager credentials.Manager, factory TaskProtectionClientFactoryInterface, expectedResponse interface{}, expectedResponseCode int) {
	// Prepare request
	bodyReader := bytes.NewReader([]byte{})
	req, err := http.NewRequest("GET", "", bodyReader)
	assert.NoError(t, err)
	req = mux.SetURLVars(req, map[string]string{v3.V3EndpointIDMuxName: v3EndpointID})

	// Call handler
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(GetTaskProtectionHandler(state, credentialsManager, factory, testCluster))
	handler.ServeHTTP(rr, req)

	expectedResponseJSON, err := json.Marshal(expectedResponse)
	assert.NoError(t, err, "Expected response must be JSON encodable")

	// Assert response
	assert.Equal(t, expectedResponseCode, rr.Code)
	responseBody, err := io.ReadAll(rr.Body)
	assert.NoError(t, err, "Failed to read response body")
	assert.Equal(t, string(expectedResponseJSON), string(responseBody))
}

// TestGetTaskProtectionHandlerTaskARNNotFound tests GetTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestGetTaskProtectionHandlerTaskARNNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return("", false)

	expectedResponse := types.TaskProtectionResponse{
		Error: &types.ErrorResponse{
			Code:    ecs.ErrCodeResourceNotFoundException,
			Message: "Invalid request: no task was found",
		},
	}
	testGetTaskProtectionHandler(t, mockState, testV3EndpointId, nil, nil,
		expectedResponse, http.StatusNotFound)
}

// TestGetTaskProtectionHandlerTaskNotFound tests GetTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestGetTaskProtectionHandlerTaskNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(nil, false)

	expectedResponse := types.TaskProtectionResponse{
		Error: &types.ErrorResponse{
			Code:    ecs.ErrCodeServerException,
			Message: "Failed to find a task for the request",
		},
	}

	testGetTaskProtectionHandler(t, mockState, testV3EndpointId, nil, nil,
		expectedResponse, http.StatusInternalServerError)
}

// TestGetTaskProtectionHandlerTaskRoleCredentialsNotFound tests GetTaskProtection handler's
// behavior when task IAM role credential is not found for the request.
func TestGetTaskProtectionHandlerTaskRoleCredentialsNotFound(t *testing.T) {
	testTask := task.Task{
		Arn:         testTaskArn,
		ServiceName: testServiceName,
	}
	testTask.SetCredentialsID(testTaskCredentialsId)

	factory := TaskProtectionClientFactory{
		Region: testRegion, Endpoint: testECSEndpoint, AcceptInsecureCert: testAcceptInsecureCert,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockManager := mock_credentials.NewMockManager(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(&testTask, true)
	mockManager.EXPECT().GetTaskCredentials(gomock.Eq(testTaskCredentialsId)).Return(credentials.TaskIAMRoleCredentials{}, false)

	expectedResponse := types.TaskProtectionResponse{
		Error: &types.ErrorResponse{
			Arn:     testTaskArn,
			Code:    ecs.ErrCodeAccessDeniedException,
			Message: "Invalid Request: no task IAM role credentials available for task",
		},
	}

	testGetTaskProtectionHandler(t, mockState, testV3EndpointId, mockManager, factory,
		expectedResponse, http.StatusForbidden)
}

// TestGetTaskProtectionHandler_PostCall tests GetTaskProtection handler's
// behavior when request successfully reached ECS and get response
func TestGetTaskProtectionHandler_PostCall(t *testing.T) {
	testCases := []struct {
		name               string
		ecsError           error
		ecsResponse        *ecs.GetTaskProtectionOutput
		expectedProtection *ecs.ProtectedTask
		expectedFailure    *ecs.Failure
		expectedError      *types.ErrorResponse
		expectedRequestId  *string
		expectedStatusCode int
		time               time.Time
	}{
		{
			name:               "RequestFailure_ServerException",
			ecsError:           awserr.NewRequestFailure(awserr.New(ecs.ErrCodeServerException, "error message", nil), http.StatusInternalServerError, testRequestID),
			ecsResponse:        &ecs.GetTaskProtectionOutput{},
			expectedError:      &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeServerException, Message: "error message"},
			expectedStatusCode: http.StatusInternalServerError,
			expectedRequestId:  generateRequestIdPtr(),
		},
		{
			name:               "RequestFailure_OtherExceptions",
			ecsError:           awserr.NewRequestFailure(awserr.New(ecs.ErrCodeAccessDeniedException, "error message", nil), http.StatusBadRequest, testRequestID),
			ecsResponse:        &ecs.GetTaskProtectionOutput{},
			expectedError:      &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeAccessDeniedException, Message: "error message"},
			expectedStatusCode: http.StatusBadRequest,
			expectedRequestId:  generateRequestIdPtr(),
		},
		{
			name:               "NonRequestFailureAwsError",
			ecsError:           awserr.New(ecs.ErrCodeInvalidParameterException, "error message", nil),
			ecsResponse:        &ecs.GetTaskProtectionOutput{},
			expectedError:      &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeInvalidParameterException, Message: "error message"},
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:        "Agent timeout",
			ecsError:    awserr.New(request.CanceledErrorCode, "request cancelled", nil),
			ecsResponse: &ecs.GetTaskProtectionOutput{},
			expectedError: &types.ErrorResponse{
				Arn:     testTaskArn,
				Code:    request.CanceledErrorCode,
				Message: ecsCallTimedOutError,
			},
			expectedStatusCode: http.StatusGatewayTimeout,
		},
		{
			name:               "NonAwsError",
			ecsError:           fmt.Errorf("error message"),
			ecsResponse:        &ecs.GetTaskProtectionOutput{},
			expectedError:      &types.ErrorResponse{Arn: testTaskArn, Code: ecs.ErrCodeServerException, Message: "error message"},
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:     "Failure",
			ecsError: nil,
			ecsResponse: &ecs.GetTaskProtectionOutput{
				Failures: []*ecs.Failure{{
					Arn:    aws.String(testTaskArn),
					Reason: aws.String(testFailureReason),
				}},
				ProtectedTasks: []*ecs.ProtectedTask{},
			},
			expectedFailure: &ecs.Failure{
				Arn:    aws.String(testTaskArn),
				Reason: aws.String(testFailureReason),
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:     "SuccessProtected",
			ecsError: nil,
			ecsResponse: &ecs.GetTaskProtectionOutput{
				Failures: []*ecs.Failure{},
				ProtectedTasks: []*ecs.ProtectedTask{{
					ProtectionEnabled: aws.Bool(true),
					ExpirationDate:    aws.Time(time.UnixMilli(0)),
					TaskArn:           aws.String(testTaskArn),
				}},
			},
			expectedProtection: &ecs.ProtectedTask{
				ProtectionEnabled: aws.Bool(true),
				ExpirationDate:    aws.Time(time.UnixMilli(0)),
				TaskArn:           aws.String(testTaskArn),
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:     "SuccessNotProtected",
			ecsError: nil,
			ecsResponse: &ecs.GetTaskProtectionOutput{
				Failures: []*ecs.Failure{},
				ProtectedTasks: []*ecs.ProtectedTask{{
					ProtectionEnabled: aws.Bool(false),
					ExpirationDate:    nil,
					TaskArn:           aws.String(testTaskArn),
				}},
			},
			expectedProtection: &ecs.ProtectedTask{
				ProtectionEnabled: aws.Bool(false),
				ExpirationDate:    nil,
				TaskArn:           aws.String(testTaskArn),
			},
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			testTask := task.Task{
				Arn:         testTaskArn,
				ServiceName: testServiceName,
			}
			testTask.SetCredentialsID(testTaskCredentialsId)

			mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
			mockManager := mock_credentials.NewMockManager(ctrl)
			mockFactory := NewMockTaskProtectionClientFactoryInterface(ctrl)
			mockECSClient := mock_api.NewMockECSTaskProtectionSDK(ctrl)

			mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
			mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(&testTask, true)
			mockManager.EXPECT().GetTaskCredentials(gomock.Eq(testTaskCredentialsId)).Return(credentials.TaskIAMRoleCredentials{}, true)
			mockFactory.EXPECT().newTaskProtectionClient(gomock.Eq(credentials.TaskIAMRoleCredentials{})).Return(mockECSClient)
			mockECSClient.EXPECT().
				GetTaskProtectionWithContext(gomock.Any(), gomock.Any()).
				Return(tc.ecsResponse, tc.ecsError)

			expectedResponse := types.TaskProtectionResponse{
				Protection: tc.expectedProtection,
				Failure:    tc.expectedFailure,
				Error:      tc.expectedError,
				RequestID:  tc.expectedRequestId,
			}

			testGetTaskProtectionHandler(t, mockState, testV3EndpointId, mockManager, mockFactory, expectedResponse, tc.expectedStatusCode)
		})
	}
}
