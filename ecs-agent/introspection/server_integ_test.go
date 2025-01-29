//go:build integration
// +build integration

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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1/handlers"
	mock_v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSetup(t *testing.T) (*gomock.Controller, *mock_v1.MockAgentState,
	*mock_metrics.MockEntryFactory, *http.Server,
) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	agentState := mock_v1.NewMockAgentState(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

	server, err := NewServer(agentState, metricsFactory)
	require.NoError(t, err)

	return ctrl, agentState, metricsFactory, server
}

func performMockRequest(t *testing.T, server *http.Server, path string) *httptest.ResponseRecorder {
	// Create the request
	req, err := http.NewRequest("GET", path, nil)
	require.NoError(t, err)

	// Send the request and record the response
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, req)

	return recorder
}

func TestRoutes(t *testing.T) {
	t.Run("agent metadata - happy case", func(t *testing.T) {
		_, mockAgentState, _, server := testSetup(t)

		testAgentMetadata := &v1.AgentMetadataResponse{
			Cluster:              "cluster",
			ContainerInstanceArn: aws.String("some/arn"),
			Version:              "1.0.0",
		}

		mockAgentState.EXPECT().
			GetAgentMetadata().
			Return(testAgentMetadata, nil)

		recorder := performMockRequest(t, server, handlers.V1AgentMetadataPath)

		testAgentMetadataJSON, _ := json.Marshal(testAgentMetadata)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, string(testAgentMetadataJSON), recorder.Body.String())
	})

	t.Run("agent metadata - fetch failed", func(t *testing.T) {
		mockCtrl, mockAgentState, mockMetricsFactory, server := testSetup(t)

		testErr := v1.NewErrorFetchFailure("some error")

		mockAgentState.EXPECT().
			GetAgentMetadata().
			Return(nil, testErr)

		mockEntry := mock_metrics.NewMockEntry(mockCtrl)
		mockEntry.EXPECT().Done(testErr)
		mockMetricsFactory.EXPECT().
			New(metrics.IntrospectionFetchFailure).Return(mockEntry)

		recorder := performMockRequest(t, server, handlers.V1AgentMetadataPath)

		emptyMetadataResponse, _ := json.Marshal(v1.AgentMetadataResponse{})

		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyMetadataResponse), recorder.Body.String())
	})

	t.Run("license - happy case", func(t *testing.T) {
		_, mockAgentState, _, server := testSetup(t)

		licenseText := "some license text"

		mockAgentState.EXPECT().
			GetLicenseText().
			Return(licenseText, nil)

		recorder := performMockRequest(t, server, licensePath)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, string(licenseText), recorder.Body.String())
	})

	t.Run("license - internal error", func(t *testing.T) {
		mockCtrl, mockAgentState, mockMetricsFactory, server := testSetup(t)

		internalErr := errors.New("some internal error")

		mockAgentState.EXPECT().
			GetLicenseText().
			Return("", internalErr)

		mockEntry := mock_metrics.NewMockEntry(mockCtrl)
		mockEntry.EXPECT().Done(internalErr)
		mockMetricsFactory.EXPECT().
			New(metrics.IntrospectionInternalServerError).Return(mockEntry)

		recorder := performMockRequest(t, server, licensePath)

		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, "", recorder.Body.String())
	})

	t.Run("tasks - happy case", func(t *testing.T) {
		_, mockAgentState, _, server := testSetup(t)

		tasksResponse := &v1.TasksResponse{
			Tasks: []*v1.TaskResponse{
				{
					Arn: "task/arn/123",
				},
			},
		}

		mockAgentState.EXPECT().
			GetTasksMetadata().
			Return(tasksResponse, nil)

		recorder := performMockRequest(t, server, handlers.V1TasksMetadataPath)

		testAgentMetadataJSON, _ := json.Marshal(tasksResponse)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, string(testAgentMetadataJSON), recorder.Body.String())
	})
}
