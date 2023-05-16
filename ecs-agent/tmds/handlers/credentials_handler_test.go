//go:build unit
// +build unit

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
package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit"
	mock_audit "github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	v1 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v1"
	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	"github.com/gorilla/mux"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Function to make a URL path for credentials request
type MakePath = func(credsId string) string

// A function that creates a credentialse HTTP handler
type GetCredentialsHandler = func(credentials.Manager, audit.AuditLogger) http.Handler

// A structure representing a test case for credentials endpoint error handling tests.
type CredentialsErrorTestCase struct {
	Name               string
	Path               string
	GetHandler         func(*mock_credentials.MockManager, *mock_audit.MockAuditLogger) http.Handler
	ExpectedStatusCode int
	ExpectedResponse   utils.ErrorMessage
}

// MakePath function for credentials endpoint v1
var makePathV1 MakePath = func(credsId string) string {
	if credsId == "" {
		return "/credentials"
	}
	return "/credentials?id=" + credsId
}

// MakePath function for credentials endpoint v2
var makePathV2 MakePath = func(credsId string) string {
	return fmt.Sprintf("%s/%s", credentials.V2CredentialsPath, credsId)
}

// GetCredentialsHandler function for v1
var getCredentialsHandlerV1 GetCredentialsHandler = func(
	credManager credentials.Manager,
	auditLogger audit.AuditLogger,
) http.Handler {
	return http.HandlerFunc(v1.CredentialsHandler(credManager, auditLogger))
}

// GetCredentialsHandler function for v2
var getCredentialsHandlerV2 GetCredentialsHandler = func(
	credManager credentials.Manager,
	auditLogger audit.AuditLogger,
) http.Handler {
	router := mux.NewRouter()
	router.HandleFunc(v2.CredentialsPath, v2.CredentialsHandler(credManager, auditLogger))
	return router
}

// Creates a test case for "no credentials ID in request" error
func noCredentialsIDCase(
	makePath MakePath,
	makeHandler GetCredentialsHandler,
	errorPrefix string,
) CredentialsErrorTestCase {
	return CredentialsErrorTestCase{
		Name: "no credentials ID in the requeest",
		Path: makePath(""),
		GetHandler: func(
			credManager *mock_credentials.MockManager,
			auditLogger *mock_audit.MockAuditLogger,
		) http.Handler {
			auditLogger.EXPECT().Log(
				gomock.Any(),
				http.StatusBadRequest,
				audit.GetCredentialsInvalidRoleTypeEventType)

			return makeHandler(credManager, auditLogger)
		},
		ExpectedStatusCode: http.StatusBadRequest,
		ExpectedResponse: utils.ErrorMessage{
			Code:          v1.ErrNoIDInRequest,
			Message:       errorPrefix + ": No Credential ID in the request",
			HTTPErrorCode: http.StatusBadRequest,
		},
	}
}

// Creates a test case for "credentials not found" error
func credentialsNotFoundCase(
	makePath MakePath,
	makeHandler GetCredentialsHandler,
	errorPrefix string,
) CredentialsErrorTestCase {
	return CredentialsErrorTestCase{
		Name: "credentials not found",
		Path: makePath("credsid"),
		GetHandler: func(
			credManager *mock_credentials.MockManager,
			auditLogger *mock_audit.MockAuditLogger,
		) http.Handler {
			auditLogger.EXPECT().Log(
				gomock.Any(),
				http.StatusBadRequest,
				audit.GetCredentialsInvalidRoleTypeEventType)
			credManager.EXPECT().GetTaskCredentials("credsid").
				Return(credentials.TaskIAMRoleCredentials{}, false)

			return makeHandler(credManager, auditLogger)
		},
		ExpectedStatusCode: http.StatusBadRequest,
		ExpectedResponse: utils.ErrorMessage{
			Code:          v1.ErrInvalidIDInRequest,
			Message:       errorPrefix + ": Credentials not found",
			HTTPErrorCode: http.StatusBadRequest,
		},
	}
}

// Creates a test case for "credentials uninitialized" error
func credentialsUninitializedCase(
	makePath MakePath,
	makeHandler GetCredentialsHandler,
	errorPrefix string,
) CredentialsErrorTestCase {
	return CredentialsErrorTestCase{
		Name: "credentials uninitialized",
		Path: makePath("credsid"),
		GetHandler: func(
			credManager *mock_credentials.MockManager,
			auditLogger *mock_audit.MockAuditLogger,
		) http.Handler {
			auditLogger.EXPECT().Log(
				gomock.Any(),
				http.StatusServiceUnavailable,
				audit.GetCredentialsInvalidRoleTypeEventType)
			credManager.EXPECT().GetTaskCredentials("credsid").
				Return(credentials.TaskIAMRoleCredentials{}, true)

			return makeHandler(credManager, auditLogger)
		},
		ExpectedStatusCode: http.StatusServiceUnavailable,
		ExpectedResponse: utils.ErrorMessage{
			Code:          v1.ErrCredentialsUninitialized,
			Message:       errorPrefix + ": Credentials uninitialized for ID",
			HTTPErrorCode: http.StatusServiceUnavailable,
		},
	}
}

// Tests error cases for credentials endpoint v1
func TestCredentialsHandlerErrorV1(t *testing.T) {
	errorPrefix := "CredentialsV1Request"
	tcs := []CredentialsErrorTestCase{
		noCredentialsIDCase(makePathV1, getCredentialsHandlerV1, errorPrefix),
		credentialsNotFoundCase(makePathV1, getCredentialsHandlerV1, errorPrefix),
		credentialsUninitializedCase(makePathV1, getCredentialsHandlerV1, errorPrefix),
	}
	for _, tc := range tcs {
		t.Run(tc.Name, func(t *testing.T) {
			testCredentialsHandlerError(t, tc)
		})
	}
}

// Tests error cases for credentials endpoint v2
func TestCredentialsHandlerErrorV2(t *testing.T) {
	errorPrefix := "CredentialsV2Request"
	tcs := []CredentialsErrorTestCase{
		noCredentialsIDCase(makePathV2, getCredentialsHandlerV2, errorPrefix),
		credentialsNotFoundCase(makePathV2, getCredentialsHandlerV2, errorPrefix),
		credentialsUninitializedCase(makePathV2, getCredentialsHandlerV2, errorPrefix),
	}
	for _, tc := range tcs {
		t.Run(tc.Name, func(t *testing.T) {
			testCredentialsHandlerError(t, tc)
		})
	}
}

// Tests error handling of credentials endpoint for a given test case.
// This function works by sending a test request to the
// handler and asserting on the response code and response body.
// This function also creates mocks for credentials manager and audit logger interfaces
// which are passed to the provided test case. The test case is responsible for setting up
// expectations on the mocks.
func testCredentialsHandlerError(
	t *testing.T,
	tc CredentialsErrorTestCase,
) {
	// Set up mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	auditLogger := mock_audit.NewMockAuditLogger(ctrl)
	credManager := mock_credentials.NewMockManager(ctrl)

	// Create request handler using a function provided by the test case.
	// The test case is responsible for setting up expectations on the mocks.
	handler := tc.GetHandler(credManager, auditLogger)

	// Send request and read response
	recorder := recordCredentialsRequest(t, handler, tc.Path)
	var response utils.ErrorMessage
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	// Assert on response status code and body
	assert.Equal(t, tc.ExpectedStatusCode, recorder.Code)
	assert.Equal(t, tc.ExpectedResponse, response)
}

// Tests happy case for credentials endpoint v1
func TestCredentialsHandlerV1Success(t *testing.T) {
	testCredentialsHandlerSuccess(t, makePathV1, getCredentialsHandlerV1)
}

// Tests happy case for credentials endpoint v2
func TestCredentialsHandlerV2Success(t *testing.T) {
	testCredentialsHandlerSuccess(t, makePathV2, getCredentialsHandlerV2)
}

// Tests happy case for credentials endpoint by sending a request to the handler and
// asserting that 200-OK response is received with credentials in the body.
func testCredentialsHandlerSuccess(t *testing.T, makePath MakePath, makeHandler GetCredentialsHandler) {
	// Create mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	auditLogger := mock_audit.NewMockAuditLogger(ctrl)
	credManager := mock_credentials.NewMockManager(ctrl)

	// Some variables
	credsId := "credsid"
	taskArn := "taskArn"
	path := makePath(credsId)

	// Credentials that will be found by the credential manager
	creds := credentials.IAMRoleCredentials{
		CredentialsID:   credsId,
		RoleArn:         "rolearn",
		AccessKeyID:     "access_key_id",
		SecretAccessKey: "secret_access_key",
		SessionToken:    "session_token",
		Expiration:      "expiration",
		RoleType:        credentials.ApplicationRoleType,
	}

	// Expected response - not all credentials fields are sent in the response
	expectedCreds := credentials.IAMRoleCredentials{
		RoleArn:         "rolearn",
		AccessKeyID:     "access_key_id",
		SecretAccessKey: "secret_access_key",
		SessionToken:    "session_token",
		Expiration:      "expiration",
	}

	auditLogger.EXPECT().Log(
		gomock.Any(),
		http.StatusOK,
		audit.GetCredentialsEventType)
	credManager.EXPECT().GetTaskCredentials(credsId).Return(
		credentials.TaskIAMRoleCredentials{ARN: taskArn, IAMRoleCredentials: creds}, true)

	// Prepare and send a request
	handler := makeHandler(credManager, auditLogger)
	recorder := recordCredentialsRequest(t, handler, path)

	// Read the response
	var response credentials.IAMRoleCredentials
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	// Assert on status code and body
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, expectedCreds, response)
}

// Sends a request to the handler and records it
func recordCredentialsRequest(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	// Prepare and send a request
	req, err := http.NewRequest("GET", path, nil)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}
