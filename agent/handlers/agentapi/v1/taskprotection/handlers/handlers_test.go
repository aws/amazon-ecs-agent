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
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

// Tests the path for PutTaskProtection API
func TestTaskProtectionPath(t *testing.T) {
	assert.Equal(t, "/api/v1/{v3EndpointIDMuxName:[^/]*}/task/protection", TaskProtectionPath())
}

// Helper function for running tests for PutTaskProtection handler
func testPutTaskProtectionHandler(t *testing.T, state dockerstate.TaskEngineState,
	v3EndpointID string,
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
	handler := http.HandlerFunc(PutTaskProtectionHandler(state, "cluster"))
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

// TestPutTaskProtectionHandlerInvalidExpiry tests PutTaskProtection handler
// with an invalid expiration value
func TestPutTaskProtectionHandlerInvalidExpiry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	testPutTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
		"endpointID",
		&TaskProtectionRequest{
			ProtectionEnabled: utils.BoolPtr(true),
			ExpiresInMinutes:  utils.IntPtr(-4),
		},
		"Invalid request: expiration duration must be greater than zero minutes for enabled task protection",
		http.StatusBadRequest)
}

// TestPutTaskProtectionHandlerInvalidExpiryType tests PutTaskProtection handler
// with a bad expiry type
func TestPutTaskProtectionHandlerInvalidExpiryType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	testPutTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
		"endpointID",
		map[string]interface{}{
			"ProtectionEnabled": true,
			"ExpiresInMinutes":  "badType",
		},
		"Failed to decode request",
		http.StatusBadRequest)
}

// TestPutTaskProtectionHandlerInvalidJSONRequest tests PutTaskProtection handler
// with a bad JSON in request body
func TestPutTaskProtectionHandlerInvalidJSONRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	testPutTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
		"endpointID", "", "Failed to decode request", http.StatusBadRequest)
}

// TestPutTaskProtectionHandlerNoBody tests that PutTaskProtection handler returns an
// appropriate error if protection type is missing in the request
func TestPutTaskProtectionHandlerNoBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	testPutTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
		"endpointID", nil, "Invalid request: does not contain 'ProtectionEnabled' field",
		http.StatusBadRequest)
}

// TestPutTaskProtectionHandlerUnknownFieldsInRequest tests that PutTaskProtection handler
// returns bad request error when the reqeust JSON contains an unknown field.
func TestPutTaskProtectionHandlerUnknownFieldsInRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	request := TaskProtectionRequest{ProtectionEnabled: utils.BoolPtr(false)}
	requestJSON, err := json.Marshal(request)
	assert.NoError(t, err)

	var rawRequest map[string]interface{}
	err = json.Unmarshal(requestJSON, &rawRequest)
	assert.NoError(t, err)
	rawRequest["UnknownField"] = 5

	testPutTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
		"endpointID", rawRequest, "Failed to decode request", http.StatusBadRequest)
}

// TestPutTaskProtectionHandlerTaskARNNotFound tests PutTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestPutTaskProtectionHandlerTaskARNNotFound(t *testing.T) {
	request := TaskProtectionRequest{ProtectionEnabled: utils.BoolPtr(false)}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return("", false)

	testPutTaskProtectionHandler(t, mockState, "endpointID", request,
		"Invalid request: no task was found",
		http.StatusBadRequest)
}

// TestPutTaskProtectionHandlerTaskNotFound tests PutTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestPutTaskProtectionHandlerTaskNotFound(t *testing.T) {
	request := TaskProtectionRequest{ProtectionEnabled: utils.BoolPtr(false)}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return("taskARN", true)
	mockState.EXPECT().TaskByArn(gomock.Eq("taskARN")).Return(nil, false)

	testPutTaskProtectionHandler(t, mockState, "endpointID", request,
		"Failed to find a task for the request",
		http.StatusInternalServerError)
}

// TestPutTaskProtectionHandlerHappy tests PutTaskProtection handler's
// behavior when request and state both are good.
func TestPutTaskProtectionHandlerHappy(t *testing.T) {
	request := TaskProtectionRequest{
		ProtectionEnabled: utils.BoolPtr(true),
		ExpiresInMinutes:  utils.IntPtr(5),
	}

	taskARN := "taskARN"
	task := task.Task{
		Arn:         taskARN,
		ServiceName: "myService",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return(taskARN, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(taskARN)).Return(&task, true)

	testPutTaskProtectionHandler(t, mockState, "endpointID", request, "Ok", http.StatusOK)
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
	handler := http.HandlerFunc(GetTaskProtectionHandler(state, "cluster"))
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
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return("", false)

	testGetTaskProtectionHandler(t, mockState, "endpointID",
		"Invalid request: no task was found",
		http.StatusBadRequest)
}

// TestGetTaskProtectionHandlerTaskNotFound tests GetTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestGetTaskProtectionHandlerTaskNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return("taskARN", true)
	mockState.EXPECT().TaskByArn(gomock.Eq("taskARN")).Return(nil, false)

	testGetTaskProtectionHandler(t, mockState, "endpointID",
		"Failed to find a task for the request",
		http.StatusInternalServerError)
}

// TestGetTaskProtectionHandlerHappy tests GetTaskProtection handler's
// behavior when request and state both are good.
func TestGetTaskProtectionHandlerHappy(t *testing.T) {
	taskARN := "taskARN"
	task := task.Task{
		Arn:         taskARN,
		ServiceName: "myService",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return(taskARN, true)
	mockState.EXPECT().TaskByArn(gomock.Eq(taskARN)).Return(&task, true)

	testGetTaskProtectionHandler(t, mockState, "endpointID", "Ok", http.StatusOK)
}
