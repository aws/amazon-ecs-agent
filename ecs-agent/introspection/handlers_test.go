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
package introspection

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1"
	mock_v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type IntrospectionResponse interface {
	string |
		*v1.AgentMetadataResponse |
		*v1.TaskResponse |
		*v1.TasksResponse
}

type IntrospectionTestCase[R IntrospectionResponse] struct {
	Path          string
	AgentResponse R
	Err           error
	MetricName    string
}

const (
	licenseText       = "Licensed under the Apache License ..."
	internalErrorText = "some internal error"
)

func testHandlerSetup[R IntrospectionResponse](t *testing.T, testCase IntrospectionTestCase[R]) (
	*gomock.Controller, *mock_v1.MockAgentState, *mock_metrics.MockEntryFactory, *http.Request, *httptest.ResponseRecorder) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	agentState := mock_v1.NewMockAgentState(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

	req, err := http.NewRequest("GET", testCase.Path, nil)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()

	return ctrl, agentState, metricsFactory, req, recorder
}

func TestLicenseHandler(t *testing.T) {
	var performMockRequest = func(t *testing.T, testCase IntrospectionTestCase[string]) *httptest.ResponseRecorder {
		mockCtrl, mockAgentState, mockMetricsFactory, req, recorder := testHandlerSetup(t, testCase)
		mockAgentState.EXPECT().
			GetLicenseText().
			Return(testCase.AgentResponse, testCase.Err)
		if testCase.Err != nil {
			mockEntry := mock_metrics.NewMockEntry(mockCtrl)
			mockEntry.EXPECT().Done(testCase.Err)
			mockMetricsFactory.EXPECT().
				New(testCase.MetricName).Return(mockEntry)
		}
		licenseHandler(mockAgentState, mockMetricsFactory)(recorder, req)
		return recorder
	}

	t.Run("happy case", func(t *testing.T) {
		recorder := performMockRequest(t, IntrospectionTestCase[string]{
			Path:          licensePath,
			AgentResponse: licenseText,
			Err:           nil,
		})
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, licenseText, recorder.Body.String())
	})

	t.Run("internal error", func(t *testing.T) {
		recorder := performMockRequest(t, IntrospectionTestCase[string]{
			Path:          licensePath,
			AgentResponse: "",
			Err:           errors.New(internalErrorText),
			MetricName:    metrics.IntrospectionInternalServerError,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, "", recorder.Body.String())
	})
}
