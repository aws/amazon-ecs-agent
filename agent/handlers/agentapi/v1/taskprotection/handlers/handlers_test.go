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
	"github.com/aws/amazon-ecs-agent/agent/handlers/agentapi/v1/taskprotection/types"
	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"
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
	assert.Equal(t, expectedResponseCode, rr.Code)
	responseBody, err := io.ReadAll(rr.Body)
	assert.NoError(t, err, "Failed to read response body")
	assert.Equal(t, string(expectedResponseJSON), string(responseBody))
}

// TestPutTaskProtectionHandlerInvalidRequest tests PutTaskProtection handler
// with a bad HTTP request
func TestPutTaskProtectionHandlerInvalidRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	request := taskProtectionRequest{ProtectionType: "badProtectionType"}
	testPutTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
		"endpointID", request,
		"Invalid request: protection type is invalid: unknown task protection type: badProtectionType",
		http.StatusBadRequest)
}

// TestPutTaskProtectionHandlerInvalidJSONRequest tests PutTaskProtection handler
// with a bad ProtectionTimeoutMinutes type
func TestPutTaskProtectionHandlerInvalidTimeoutType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	testPutTaskProtectionHandler(t, mock_dockerstate.NewMockTaskEngineState(ctrl),
		"endpointID",
		map[string]string{
			"ProtectionType":           string(types.TaskProtectionTypeScaleIn),
			"ProtectionTimeoutMinutes": "badType",
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
		"endpointID", nil, "Invalid request: protection type is missing or empty", http.StatusBadRequest)
}

// TestPutTaskProtectionHandlerUnknownFieldsInRequest tests that PutTaskProtection handler
// returns bad request error when the reqeust JSON contains an unknown field.
func TestPutTaskProtectionHandlerUnknownFieldsInRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	request := taskProtectionRequest{ProtectionType: string(types.TaskProtectionTypeScaleIn)}
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
	muxVars := make(map[string]string)
	muxVars[v3.V3EndpointIDMuxName] = "endpointID"
	request := taskProtectionRequest{ProtectionType: string(types.TaskProtectionTypeDisabled)}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return("", false)

	testPutTaskProtectionHandler(t, mockState, "endpointID", request,
		"Failed to find task: unable to get task ARN from request: unable to get task Arn from v3 endpoint ID: endpointID",
		http.StatusInternalServerError)
}

// TestPutTaskProtectionHandlerTaskNotFound tests PutTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestPutTaskProtectionHandlerTaskNotFound(t *testing.T) {
	request := taskProtectionRequest{ProtectionType: string(types.TaskProtectionTypeDisabled)}
	muxVars := make(map[string]string)
	muxVars[v3.V3EndpointIDMuxName] = "endpointID"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return("taskARN", true)
	mockState.EXPECT().TaskByArn(gomock.Eq("taskARN")).Return(nil, false)

	testPutTaskProtectionHandler(t, mockState, "endpointID", request,
		"Failed to find task: could not find task from task ARN 'taskARN'",
		http.StatusInternalServerError)
}

// TestPutTaskProtectionHandlerHappy tests PutTaskProtection handler's
// behavior when request and state both are good.
func TestPutTaskProtectionHandlerHappy(t *testing.T) {
	request := taskProtectionRequest{ProtectionType: string(types.TaskProtectionTypeDisabled)}
	muxVars := make(map[string]string)
	muxVars[v3.V3EndpointIDMuxName] = "endpointID"

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
	muxVars := make(map[string]string)
	muxVars[v3.V3EndpointIDMuxName] = "endpointID"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return("", false)

	testGetTaskProtectionHandler(t, mockState, "endpointID",
		"Failed to find task: unable to get task ARN from request: unable to get task Arn from v3 endpoint ID: endpointID",
		http.StatusInternalServerError)
}

// TestGetTaskProtectionHandlerTaskNotFound tests GetTaskProtection handler's
// behavior when task ARN was not found for the request.
func TestGetTaskProtectionHandlerTaskNotFound(t *testing.T) {
	muxVars := make(map[string]string)
	muxVars[v3.V3EndpointIDMuxName] = "endpointID"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockState.EXPECT().TaskARNByV3EndpointID(gomock.Eq("endpointID")).Return("taskARN", true)
	mockState.EXPECT().TaskByArn(gomock.Eq("taskARN")).Return(nil, false)

	testGetTaskProtectionHandler(t, mockState, "endpointID",
		"Failed to find task: could not find task from task ARN 'taskARN'",
		http.StatusInternalServerError)
}

// TestGetTaskProtectionHandlerHappy tests GetTaskProtection handler's
// behavior when request and state both are good.
func TestGetTaskProtectionHandlerHappy(t *testing.T) {
	muxVars := make(map[string]string)
	muxVars[v3.V3EndpointIDMuxName] = "endpointID"

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
