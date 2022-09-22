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
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	testAccessKey              = "accessKey"
	testSecretKey              = "secretKey"
	testSessionToken           = "sessionToken"
	testCluster                = "cluster"
	testRegion                 = "region"
	testEndpoint               = "endpoint"
	testTaskCredentialsId      = "taskCredentialsId"
	testV3EndpointId           = "endpointId"
	testTaskArn                = "taskArn"
	testServiceName            = "serviceName"
	testAcceptInsecureCert     = false
	protectionEnabledFieldName = "ProtectionEnabled"
	expiresInMinutesFieldName  = "ExpiresInMinutes"
	testExpiresInMinutes       = 5
	testProtectionEnabled      = true
)

// MockTaskProtectionClientFactory is a mock of TaskProtectionClientFactory interface
type MockTaskProtectionClientFactory struct {
	ctrl     *gomock.Controller
	recorder *MockTaskProtectionClientFactoryMockRecorder
}

// MockECSClientMockRecorder is the mock recorder for MockECSClient
type MockTaskProtectionClientFactoryMockRecorder struct {
	mock *MockTaskProtectionClientFactory
}

func NewMockTaskProtectionClientFactory(ctrl *gomock.Controller) *MockTaskProtectionClientFactory {
	mock := &MockTaskProtectionClientFactory{ctrl: ctrl}
	mock.recorder = &MockTaskProtectionClientFactoryMockRecorder{mock}
	return mock
}

func (m *MockTaskProtectionClientFactory) EXPECT() *MockTaskProtectionClientFactoryMockRecorder {
	return m.recorder
}

// GetTaskCredentials mocks base method
func (m *MockTaskProtectionClientFactory) getECSClient(arg0 credentials.Manager, arg1 *task.Task) (api.ECSTaskProtectionSDK, int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getECSClient", arg0, arg1)
	ret0, _ := ret[0].(api.ECSTaskProtectionSDK)
	ret1, _ := ret[1].(int)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}
func (mr *MockTaskProtectionClientFactoryMockRecorder) getECSClient(arg0 interface{}, arg1 *task.Task) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getECSClient", reflect.TypeOf((*MockTaskProtectionClientFactory)(nil).getECSClient), arg0, arg1)
}

// Tests the path for UpdateTaskProtection API
func TestTaskProtectionPath(t *testing.T) {
	assert.Equal(t, "/api/{v3EndpointIDMuxName:[^/]*}/task-protection/v1/state", TaskProtectionPath())
}

// TestGetECSClientHappyCase tests getECSClient uses credential in credentials manager and
// returns a ECS client with correct status code and error
func TestGetECSClientHappyCase(t *testing.T) {
	testTask := task.Task{
		Arn:         testTaskArn,
		ServiceName: testServiceName,
	}
	testTask.SetCredentialsID(testTaskCredentialsId)

	testIAMRoleCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     testAccessKey,
			SecretAccessKey: testSecretKey,
			SessionToken:    testSessionToken,
		},
	}

	factory := TaskProtectionClientFactory{
		Region: testRegion, Endpoint: testEndpoint, AcceptInsecureCert: testAcceptInsecureCert,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockManager := mock_credentials.NewMockManager(ctrl)
	mockManager.EXPECT().GetTaskCredentials(gomock.Eq(testTaskCredentialsId)).Return(testIAMRoleCredentials, true)

	ret, statusCode, err := factory.getECSClient(mockManager, &testTask)
	_, ok := ret.(api.ECSTaskProtectionSDK)

	// Assert response
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
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

// TestUpdateTaskProtectionHandler_InputValidations tests UpdateTaskProtection handler's
// behavior with different invalid inputs
func TestUpdateTaskProtectionHandler_InputValidations(t *testing.T) {
	testCases := []struct {
		name             string
		request          interface{}
		expectedResponse string
	}{
		{
			name: "InvalidTypes",
			request: &map[string]interface{}{
				protectionEnabledFieldName: true,
				expiresInMinutesFieldName:  "badType",
			},
			expectedResponse: "Failed to decode request",
		},
		{
			name: "InvalidExpiry",
			request: &TaskProtectionRequest{
				ProtectionEnabled: utils.BoolPtr(true),
				ExpiresInMinutes:  utils.Int64Ptr(-4),
			},
			expectedResponse: "Invalid request: expiration duration must be greater than zero minutes for enabled task protection",
		},
		{
			name:             "InvalidJSONRequest",
			request:          "",
			expectedResponse: "Failed to decode request",
		},
		{
			name:             "EmptyRequest",
			request:          nil,
			expectedResponse: "Invalid request: does not contain 'ProtectionEnabled' field",
		},
		{
			name:             "UnknownFieldsInRequest",
			request:          getRequestWithUnknownFields(t),
			expectedResponse: "Failed to decode request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			testUpdateTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
				testEndpoint, nil, nil, tc.request, tc.expectedResponse, http.StatusBadRequest)
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

	testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, nil, nil, request,
		"invalid request: no task was found",
		http.StatusBadRequest)
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

	testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, nil, nil, request,
		"failed to find a task for the request",
		http.StatusInternalServerError)
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
		Region: testRegion, Endpoint: testEndpoint, AcceptInsecureCert: testAcceptInsecureCert,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockManager := mock_credentials.NewMockManager(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(&testTask, true)
	mockManager.EXPECT().GetTaskCredentials(gomock.Eq(testTaskCredentialsId)).Return(credentials.TaskIAMRoleCredentials{}, false)

	testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, mockManager, factory, request,
		"invalid Request: no task IAM role credentials available for task", http.StatusUnauthorized)
}

// TestUpdateTaskProtectionHandler_PostCall tests UpdateTaskProtection handler's
// behavior when request successfully reached ECS and get response
func TestUpdateTaskProtectionHandler_PostCall(t *testing.T) {
	testCases := []struct {
		name               string
		ecsError           error
		ecsResponse        *ecs.UpdateTaskProtectionOutput
		expectedResponse   interface{}
		expectedStatusCode int
		time               time.Time
	}{
		{
			name:               "ServerException",
			ecsError:           errors.Errorf("ServerException: error message"),
			ecsResponse:        &ecs.UpdateTaskProtectionOutput{},
			expectedResponse:   "ServerException: error message",
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:               "OtherExceptions",
			ecsError:           errors.Errorf("AccessDeniedException: error message"),
			ecsResponse:        &ecs.UpdateTaskProtectionOutput{},
			expectedResponse:   "AccessDeniedException: error message",
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:     "Failure",
			ecsError: nil,
			ecsResponse: &ecs.UpdateTaskProtectionOutput{
				Failures: []*ecs.Failure{{
					Arn:    aws.String(testTaskArn),
					Reason: aws.String("Failure Reason"),
				}},
				ProtectedTasks: []*ecs.ProtectedTask{},
			},
			expectedResponse: ecs.Failure{
				Arn:    aws.String(testTaskArn),
				Reason: aws.String("Failure Reason"),
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:     "Success",
			ecsError: nil,
			ecsResponse: &ecs.UpdateTaskProtectionOutput{
				Failures: []*ecs.Failure{},
				ProtectedTasks: []*ecs.ProtectedTask{{
					ProtectionEnabled: aws.Bool(true),
					ExpirationDate:    aws.Time(time.UnixMilli(0)),
					TaskArn:           aws.String(testTaskArn),
				}},
			},
			expectedResponse: ecs.ProtectedTask{
				ProtectionEnabled: aws.Bool(true),
				ExpirationDate:    aws.Time(time.UnixMilli(0)),
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
			mockFactory := NewMockTaskProtectionClientFactory(ctrl)
			mockECSClient := mock_api.NewMockECSTaskProtectionSDK(ctrl)

			mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
			mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(&testTask, true)
			mockFactory.EXPECT().getECSClient(mockManager, &testTask).Return(
				mockECSClient, http.StatusOK, nil)
			mockECSClient.EXPECT().UpdateTaskProtection(gomock.Any()).Return(tc.ecsResponse, tc.ecsError)

			testUpdateTaskProtectionHandler(t, mockState, testV3EndpointId, mockManager, mockFactory, request, tc.expectedResponse, tc.expectedStatusCode)
		})
	}
}

// Helper function for running tests for GetTaskProtection handler
func testGetTaskProtectionHandler(t *testing.T, state dockerstate.TaskEngineState,
	v3EndpointID string, expectedResponse interface{}, expectedResponseCode int) {
	// Prepare request
	bodyReader := bytes.NewReader([]byte{})
	req, err := http.NewRequest("GET", "", bodyReader)
	assert.NoError(t, err)
	req = mux.SetURLVars(req, map[string]string{v3.V3EndpointIDMuxName: v3EndpointID})

	// Call handler
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(GetTaskProtectionHandler(state, nil, testCluster, testRegion))
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

	testGetTaskProtectionHandler(t, mockState, testV3EndpointId,
		"invalid request: no task was found",
		http.StatusBadRequest)
}

// TestGetTaskProtectionHandlerTaskNotFound tests GetTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestGetTaskProtectionHandlerTaskNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(nil, false)

	testGetTaskProtectionHandler(t, mockState, testV3EndpointId,
		"failed to find a task for the request",
		http.StatusInternalServerError)
}

// TestGetTaskProtectionHandlerHappy tests GetTaskProtection handler's
// behavior when request and state both are good.
func TestGetTaskProtectionHandlerHappy(t *testing.T) {
	testTask := task.Task{
		Arn:         testTaskArn,
		ServiceName: testServiceName,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq(testV3EndpointId)).Return(testTaskArn, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(testTaskArn)).Return(&testTask, true)

	testGetTaskProtectionHandler(t, mockState, testV3EndpointId, "Ok", http.StatusOK)
}
