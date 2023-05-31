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

package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	mock_audit "github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/request"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstructMuxVar(t *testing.T) {
	testCases := []struct {
		testName    string
		name        string
		pattern     string
		expectedVar string
	}{
		{
			testName:    "With anything but slash pattern",
			name:        "muxname",
			pattern:     AnythingButSlashRegEx,
			expectedVar: "{muxname:[^/]*}",
		},
		{
			testName:    "With anything pattern",
			name:        "muxname",
			pattern:     AnythingRegEx,
			expectedVar: "{muxname:.*}",
		},
		{
			testName:    "With anything but empty pattern",
			name:        "muxname",
			pattern:     AnythingButEmptyRegEx,
			expectedVar: "{muxname:.+}",
		},
		{
			testName:    "Without pattern",
			name:        "muxname",
			pattern:     "",
			expectedVar: "{muxname}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			assert.Equal(t, tc.expectedVar, ConstructMuxVar(tc.name, tc.pattern))
		})
	}
}

func TestWriteJSONToResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	responseJSON, _ := json.Marshal("Unable to get task arn from request")
	WriteJSONToResponse(recorder, http.StatusOK, responseJSON, RequestTypeTaskMetadata)

	bodyBytes, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	bodyString := string(bodyBytes)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, `"Unable to get task arn from request"`, bodyString)
}

// Tests that WriteJSONResponse marshals the provided response to JSON and writes it to
// the response writer.
func TestWriteJSONResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	res := response.PortResponse{ContainerPort: 8080, Protocol: "TCP", HostPort: 80, HostIp: "IP"}
	WriteJSONResponse(recorder, http.StatusOK, res, RequestTypeTaskMetadata)

	var actualResponse response.PortResponse
	err := json.Unmarshal(recorder.Body.Bytes(), &actualResponse)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, res, actualResponse)
}

// Tests that an empty JSON response is written by WriteJSONResponse if the provided response
// is not convertible to JSON.
func TestWriteJSONResponseError(t *testing.T) {
	recorder := httptest.NewRecorder()
	res := func(k string) string { return k }
	WriteJSONResponse(recorder, http.StatusOK, res, RequestTypeTaskMetadata)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "{}", recorder.Body.String())
}

func TestValueFromRequest(t *testing.T) {
	r, _ := http.NewRequest("GET", "/v1/credentials?id=credid", nil)
	val, ok := ValueFromRequest(r, "id")

	assert.True(t, ok)
	assert.Equal(t, "credid", val)
}

// Tests that an audit log is created by LimitReachHandler
func TestLimitReachHandler(t *testing.T) {
	// Prepare a request
	req, err := http.NewRequest("GET", "/endpoint", nil)
	require.NoError(t, err)

	// Set up mock audit logger with a log expectation
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	auditLogger := mock_audit.NewMockAuditLogger(ctrl)
	auditLogger.EXPECT().Log(request.LogRequest{Request: req}, http.StatusTooManyRequests, "")

	// Send the request, assertion is performed by the expectation on the mock audit logger
	handler := http.HandlerFunc(LimitReachedHandler(auditLogger))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
}

func TestIs5XXStatus(t *testing.T) {
	yes := []int{500, 501, 550, http.StatusInternalServerError, http.StatusServiceUnavailable, 580, 599}
	for _, y := range yes {
		t.Run(fmt.Sprintf("yes %d", y), func(t *testing.T) {
			assert.True(t, Is5XXStatus(y))
		})
	}

	no := []int{http.StatusTooEarly, http.StatusBadRequest, http.StatusTooManyRequests, 400, 450, 600, 200, 301}
	for _, n := range no {
		t.Run(fmt.Sprintf("no %d", n), func(t *testing.T) {
			assert.False(t, Is5XXStatus(n))
		})
	}
}
