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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestValueFromRequest(t *testing.T) {
	r, _ := http.NewRequest("GET", "/v1/credentials?id=credid", nil)
	val, ok := ValueFromRequest(r, "id")

	assert.True(t, ok)
	assert.Equal(t, "credid", val)
}
