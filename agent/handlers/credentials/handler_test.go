// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package credentials

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mock_audit "github.com/aws/amazon-ecs-agent/agent/logger/audit/mocks"
	"github.com/golang/mock/gomock"
)

const (
	roleArn         = "r1"
	accessKeyID     = "ak"
	secretAccessKey = "sk"
	credentialsId   = "credentialsId"
)

// TestInvalidPath tests if HTTP status code 404 is returned when invalid path is queried.
func TestInvalidPath(t *testing.T) {
	testErrorResponsesFromServer(t, "/", nil)
}

// TestCredentialsRequestWithNoArguments tests if HTTP status code 400 is returned when
// query parameters are not specified for the credentials endpoint.
func TestCredentialsRequestWithNoArguments(t *testing.T) {
	msg := &errorMessage{
		Code:          NoIdInRequest,
		Message:       "CredentialsV1Request: No ID in the request",
		httpErrorCode: http.StatusBadRequest,
	}
	testErrorResponsesFromServer(t, credentials.CredentialsPath, msg)
}

// TestCredentialsRequestWhenCredentialsIdNotFound tests if HTTP status code 400 is returned when
// the credentials manager does not contain the credentials id specified in the query.
func TestCredentialsRequestWhenCredentialsIdNotFound(t *testing.T) {
	expectedErrorMessage := &errorMessage{
		Code:          InvalidIdInRequest,
		Message:       fmt.Sprintf("CredentialsV1Request: ID not found"),
		httpErrorCode: http.StatusBadRequest,
	}
	_, err := getResponseForCredentialsRequestWithParameters(t, expectedErrorMessage.httpErrorCode,
		expectedErrorMessage, credentialsId, func() (*credentials.TaskIAMRoleCredentials, bool) { return nil, false })
	if err != nil {
		t.Fatalf("Error getting response body: %v", err)
	}
}

// TestCredentialsRequestWhenCredentialsUninitialized tests if HTTP status code 500 is returned when
// the credentials manager returns empty credentials.
func TestCredentialsRequestWhenCredentialsUninitialized(t *testing.T) {
	expectedErrorMessage := &errorMessage{
		Code:          CredentialsUninitialized,
		Message:       fmt.Sprintf("CredentialsV1Request: Credentials uninitialized for ID"),
		httpErrorCode: http.StatusServiceUnavailable,
	}
	_, err := getResponseForCredentialsRequestWithParameters(t, expectedErrorMessage.httpErrorCode,
		expectedErrorMessage, credentialsId, func() (*credentials.TaskIAMRoleCredentials, bool) { return nil, true })
	if err != nil {
		t.Fatalf("Error getting response body: %v", err)
	}
}

// TestCredentialsRequestWhenCredentialsFound tests if HTTP status code 200 is returned when
// the credentials manager contains the credentials id specified in the query.
func TestCredentialsRequestWhenCredentialsFound(t *testing.T) {
	creds := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			RoleArn:         roleArn,
			AccessKeyId:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		},
	}
	body, err := getResponseForCredentialsRequestWithParameters(t, http.StatusOK, nil, credentialsId, func() (*credentials.TaskIAMRoleCredentials, bool) { return &creds, true })
	if err != nil {
		t.Fatalf("Error retrieving credentials response: %v", err)
	}

	credentials, err := parseResponseBody(body)
	if err != nil {
		t.Fatalf("Error retrieving credentials: %v", err)
	}

	if roleArn != credentials.RoleArn {
		t.Fatalf("Incorrect credentials received. Expected arn: %s, got: %s", roleArn, credentials.RoleArn)
	}
	if accessKeyID != credentials.AccessKeyId {
		t.Fatalf("Incorrect credentials received. Expected Access Key: %s, got: %s", accessKeyID, credentials.AccessKeyId)
	}
	if secretAccessKey != credentials.SecretAccessKey {
		t.Fatalf("Incorrect credentials received. Expected Secret Key: %s, got: %s", secretAccessKey, credentials.SecretAccessKey)
	}
}

func testErrorResponsesFromServer(t *testing.T, path string, expectedErrorMessage *errorMessage) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	server := setupServer(credentialsManager, auditLog)

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	if path == credentials.CredentialsPath {
		auditLog.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any())
	}

	server.Handler.ServeHTTP(recorder, req)
	httpErrorCode := http.StatusNotFound
	if expectedErrorMessage != nil {
		httpErrorCode = expectedErrorMessage.httpErrorCode
	}
	if recorder.Code != httpErrorCode {
		t.Fatalf("Expected return code: %d. Got: %d", httpErrorCode, recorder.Code)
	}

	// Only paths that are equal to /v1/credentials will return valid error responses.
	if path == credentials.CredentialsPath {
		errorMessage := &errorMessage{}
		json.Unmarshal(recorder.Body.Bytes(), errorMessage)
		if errorMessage.Code != expectedErrorMessage.Code ||
			errorMessage.Message != expectedErrorMessage.Message {
			t.Fatalf("Unexpected values. Actual: %v| Expected: %v", errorMessage, expectedErrorMessage)
		}
	}
}

// getResponseForCredentialsRequestWithParameters queries credentials for the
// given id. The getCredentials function is used to simulate getting the
// credentials object from the CredentialsManager
func getResponseForCredentialsRequestWithParameters(t *testing.T, expectedStatus int,
	expectedErrorMessage *errorMessage, id string, getCredentials func() (*credentials.TaskIAMRoleCredentials, bool)) (*bytes.Buffer, error) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := mock_credentials.NewMockManager(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	server := setupServer(credentialsManager, auditLog)
	recorder := httptest.NewRecorder()

	creds, ok := getCredentials()
	credentialsManager.EXPECT().GetTaskCredentials(gomock.Any()).Return(creds, ok)
	auditLog.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any())

	params := make(url.Values)
	params[credentials.CredentialsIdQueryParameterName] = []string{credentialsId}

	req, _ := http.NewRequest("GET", credentials.CredentialsPath+"?"+params.Encode(), nil)
	server.Handler.ServeHTTP(recorder, req)

	if recorder.Code != expectedStatus {
		return nil, fmt.Errorf("Expected return code: %d. Got: %d", expectedStatus, recorder.Code)
	}

	if recorder.Code != http.StatusOK {
		errorMessage := &errorMessage{}
		json.Unmarshal(recorder.Body.Bytes(), errorMessage)

		if errorMessage.Code != expectedErrorMessage.Code || errorMessage.Message != expectedErrorMessage.Message {
			return nil, fmt.Errorf("Unexpected values. Actual: %v| Expected: %v", errorMessage, expectedErrorMessage)
		}
	}

	return recorder.Body, nil
}

// parseResponseBody parses the HTTP response body into an IAMRoleCredentials object.
func parseResponseBody(body *bytes.Buffer) (*credentials.IAMRoleCredentials, error) {
	response, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response body: %v", err)
	}
	var creds credentials.IAMRoleCredentials
	err = json.Unmarshal(response, &creds)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling: %v", err)
	}
	return &creds, nil
}
