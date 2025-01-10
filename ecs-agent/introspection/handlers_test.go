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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	licenseText          = "Licensed under the Apache License ..."
	taskARN              = "t1"
	family               = "sleep"
	version              = "1"
	statusRunning        = "RUNNING"
	dockerId             = "c0123456789ABC"
	shortDockerId        = "c01234567890"
	dockerName           = "cname"
	containerName        = "sleepy"
	imageName            = "busybox"
	imageID              = "busyb0xID"
	networkModeAwsvpc    = "awsvpc"
	eniIPv4Address       = "10.0.0.2"
	port                 = 80
	protocol             = "tcp"
	volName              = "volume1"
	volSource            = "/var/lib/volume1"
	volDestination       = "/volume"
	agentVersion         = "1.0.0"
	cluster              = "test-cluster"
	conatinerInstanceArn = "test/arn"
	internalErrorText    = "some internal error"
	testEndpoint         = "test endpoint"
)

var testAgentMetadata = &AgentMetadataResponse{
	Cluster:              cluster,
	ContainerInstanceArn: aws.String(conatinerInstanceArn),
	Version:              agentVersion,
}

var testTask = &TaskResponse{
	Arn:           "t1",
	DesiredStatus: statusRunning,
	KnownStatus:   statusRunning,
	Family:        "sleep",
	Version:       "1",
	Containers: []ContainerResponse{
		{
			DockerID:   dockerId,
			DockerName: dockerName,
			Name:       containerName,
			Image:      imageName,
			ImageID:    imageID,
			Ports: []response.PortResponse{
				{
					ContainerPort: port,
					Protocol:      protocol,
				},
			},
			Networks: []response.Network{
				{
					NetworkMode:   networkModeAwsvpc,
					IPv4Addresses: []string{eniIPv4Address},
				},
			},
			Volumes: []response.VolumeResponse{
				{
					DockerName:  volName,
					Source:      volSource,
					Destination: volDestination,
				},
			},
		},
	},
}

var testTasks = &TasksResponse{
	Tasks: []*TaskResponse{
		testTask,
	},
}

type IntrospectionResponse interface {
	string |
		*AgentMetadataResponse |
		*TaskResponse |
		*TasksResponse
}

type IntrospectionTestCase[R IntrospectionResponse] struct {
	Path          string
	AgentResponse R
	Err           error
	MetricName    string
}

func testHandlerSetup[R IntrospectionResponse](t *testing.T, testCase IntrospectionTestCase[R]) (
	*gomock.Controller, *MockAgentState, *mock_metrics.MockEntryFactory, *http.Request, *httptest.ResponseRecorder) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	agentState := NewMockAgentState(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

	req, err := http.NewRequest("GET", testCase.Path, nil)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()

	return ctrl, agentState, metricsFactory, req, recorder
}

func TestAgentMetadataHandler(t *testing.T) {
	var performMockRequest = func(t *testing.T, testCase IntrospectionTestCase[*AgentMetadataResponse]) *httptest.ResponseRecorder {
		mockCtrl, mockAgentState, mockMetricsFactory, req, recorder := testHandlerSetup(t, testCase)
		mockAgentState.EXPECT().
			GetAgentMetadata().
			Return(testCase.AgentResponse, testCase.Err)
		if testCase.Err != nil {
			mockEntry := mock_metrics.NewMockEntry(mockCtrl)
			mockEntry.EXPECT().Done(testCase.Err)
			mockMetricsFactory.EXPECT().
				New(testCase.MetricName).Return(mockEntry)
		}
		agentMetadataHandler(mockAgentState, mockMetricsFactory)(recorder, req)
		return recorder
	}

	t.Run("happy case", func(t *testing.T) {
		recorder := performMockRequest(t, IntrospectionTestCase[*AgentMetadataResponse]{
			Path:          agentMetadataPath,
			AgentResponse: testAgentMetadata,
			Err:           nil,
		})
		assert.Equal(t, http.StatusOK, recorder.Code)
		testAgentMetadataJson, _ := json.Marshal(testAgentMetadata)
		assert.Equal(t, string(testAgentMetadataJson), recorder.Body.String())
	})

	emptyMetadataResponse, _ := json.Marshal(AgentMetadataResponse{})

	t.Run("bad request", func(t *testing.T) {
		recorder := performMockRequest(t, IntrospectionTestCase[*AgentMetadataResponse]{
			Path:          agentMetadataPath,
			AgentResponse: testAgentMetadata,
			Err:           NewErrorBadRequest(internalErrorText),
			MetricName:    metrics.IntrospectionBadRequest,
		})
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, string(emptyMetadataResponse), recorder.Body.String())
	})

	t.Run("not found", func(t *testing.T) {
		recorder := performMockRequest(t, IntrospectionTestCase[*AgentMetadataResponse]{
			Path:          agentMetadataPath,
			AgentResponse: testAgentMetadata,
			Err:           NewErrorNotFound(internalErrorText),
			MetricName:    metrics.IntrospectionNotFound,
		})
		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Equal(t, string(emptyMetadataResponse), recorder.Body.String())
	})

	t.Run("fetch failure", func(t *testing.T) {
		recorder := performMockRequest(t, IntrospectionTestCase[*AgentMetadataResponse]{
			Path:          agentMetadataPath,
			AgentResponse: testAgentMetadata,
			Err:           NewErrorFetchFailure(internalErrorText),
			MetricName:    metrics.IntrospectionFetchFailure,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyMetadataResponse), recorder.Body.String())
	})

	t.Run("unkown error", func(t *testing.T) {
		recorder := performMockRequest(t, IntrospectionTestCase[*AgentMetadataResponse]{
			Path:          agentMetadataPath,
			AgentResponse: testAgentMetadata,
			Err:           errors.New(internalErrorText),
			MetricName:    metrics.IntrospectionInternalServerError,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyMetadataResponse), recorder.Body.String())
	})
}

func TestTasksHandler(t *testing.T) {
	/* all tasks */
	var performMockTasksRequest = func(t *testing.T, testCase IntrospectionTestCase[*TasksResponse]) *httptest.ResponseRecorder {
		mockCtrl, mockAgentState, mockMetricsFactory, req, recorder := testHandlerSetup(t, testCase)
		mockAgentState.EXPECT().
			GetTasksMetadata().
			Return(testCase.AgentResponse, testCase.Err)
		if testCase.Err != nil {
			mockEntry := mock_metrics.NewMockEntry(mockCtrl)
			mockEntry.EXPECT().Done(testCase.Err)
			mockMetricsFactory.EXPECT().
				New(testCase.MetricName).Return(mockEntry)
		}
		tasksMetadataHandler(mockAgentState, mockMetricsFactory)(recorder, req)
		return recorder
	}
	t.Run("all tasks: happy case", func(t *testing.T) {
		recorder := performMockTasksRequest(t, IntrospectionTestCase[*TasksResponse]{
			Path:          tasksMetadataPath,
			AgentResponse: testTasks,
			Err:           nil,
		})
		assert.Equal(t, http.StatusOK, recorder.Code)
		testTasksJSON, _ := json.Marshal(testTasks)
		assert.Equal(t, string(testTasksJSON), recorder.Body.String())
	})

	emptyTasksResponse, _ := json.Marshal(TasksResponse{})

	t.Run("all tasks: not found", func(t *testing.T) {
		recorder := performMockTasksRequest(t, IntrospectionTestCase[*TasksResponse]{
			Path:          tasksMetadataPath,
			AgentResponse: nil,
			Err:           NewErrorNotFound(internalErrorText),
			MetricName:    metrics.IntrospectionNotFound,
		})
		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Equal(t, string(emptyTasksResponse), recorder.Body.String())
	})

	t.Run("all tasks: fetch failed", func(t *testing.T) {
		recorder := performMockTasksRequest(t, IntrospectionTestCase[*TasksResponse]{
			Path:          tasksMetadataPath,
			AgentResponse: nil,
			Err:           NewErrorFetchFailure(internalErrorText),
			MetricName:    metrics.IntrospectionFetchFailure,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyTasksResponse), recorder.Body.String())
	})

	t.Run("all tasks: unknown error", func(t *testing.T) {
		recorder := performMockTasksRequest(t, IntrospectionTestCase[*TasksResponse]{
			Path:          tasksMetadataPath,
			AgentResponse: nil,
			Err:           errors.New(internalErrorText),
			MetricName:    metrics.IntrospectionInternalServerError,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyTasksResponse), recorder.Body.String())
	})

	/* task by Id */
	performMockTaskRequest := func(t *testing.T, testCase IntrospectionTestCase[*TaskResponse]) *httptest.ResponseRecorder {
		mockCtrl, mockAgentState, mockMetricsFactory, req, recorder := testHandlerSetup(t, testCase)
		mockAgentState.EXPECT().
			GetTaskMetadataByID(dockerId).
			Return(testCase.AgentResponse, testCase.Err)
		if testCase.Err != nil {
			mockEntry := mock_metrics.NewMockEntry(mockCtrl)
			mockEntry.EXPECT().Done(testCase.Err)
			mockMetricsFactory.EXPECT().
				New(testCase.MetricName).Return(mockEntry)
		}
		tasksMetadataHandler(mockAgentState, mockMetricsFactory)(recorder, req)
		return recorder
	}

	t.Run("task by Id: happy case", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, dockerIDQueryField, dockerId),
			AgentResponse: testTask,
			Err:           nil,
		})
		assert.Equal(t, http.StatusOK, recorder.Code)
		testTaskJSON, _ := json.Marshal(testTask)
		assert.Equal(t, string(testTaskJSON), recorder.Body.String())
	})

	emptyTaskResponse, _ := json.Marshal(TaskResponse{})

	t.Run("task by id: not found", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, dockerIDQueryField, dockerId),
			AgentResponse: nil,
			Err:           NewErrorNotFound(internalErrorText),
			MetricName:    metrics.IntrospectionNotFound,
		})
		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	t.Run("task by id: fetch failed", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, dockerIDQueryField, dockerId),
			AgentResponse: nil,
			Err:           NewErrorFetchFailure(internalErrorText),
			MetricName:    metrics.IntrospectionFetchFailure,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	t.Run("task by id: unknown error", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, dockerIDQueryField, dockerId),
			AgentResponse: nil,
			Err:           errors.New(internalErrorText),
			MetricName:    metrics.IntrospectionInternalServerError,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	/* task by short Id */
	performMockTaskRequest = func(t *testing.T, testCase IntrospectionTestCase[*TaskResponse]) *httptest.ResponseRecorder {
		mockCtrl, mockAgentState, mockMetricsFactory, req, recorder := testHandlerSetup(t, testCase)
		mockAgentState.EXPECT().
			GetTaskMetadataByShortID(shortDockerId).
			Return(testCase.AgentResponse, testCase.Err)
		if testCase.Err != nil {
			mockEntry := mock_metrics.NewMockEntry(mockCtrl)
			mockEntry.EXPECT().Done(testCase.Err)
			mockMetricsFactory.EXPECT().
				New(testCase.MetricName).Return(mockEntry)
		}
		tasksMetadataHandler(mockAgentState, mockMetricsFactory)(recorder, req)
		return recorder
	}
	t.Run("task by short Id: happy case", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, dockerIDQueryField, shortDockerId),
			AgentResponse: testTask,
			Err:           nil,
		})
		assert.Equal(t, http.StatusOK, recorder.Code)
		testTaskJSON, _ := json.Marshal(testTask)
		assert.Equal(t, string(testTaskJSON), recorder.Body.String())
	})

	t.Run("task by short id: not found", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, dockerIDQueryField, shortDockerId),
			AgentResponse: nil,
			Err:           NewErrorNotFound(internalErrorText),
			MetricName:    metrics.IntrospectionNotFound,
		})
		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	t.Run("task by short id: fetch failed", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, dockerIDQueryField, shortDockerId),
			AgentResponse: nil,
			Err:           NewErrorFetchFailure(internalErrorText),
			MetricName:    metrics.IntrospectionFetchFailure,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	t.Run("task by short id: unknown error", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, dockerIDQueryField, shortDockerId),
			AgentResponse: nil,
			Err:           errors.New(internalErrorText),
			MetricName:    metrics.IntrospectionInternalServerError,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	/* task by arn */
	performMockTaskRequest = func(t *testing.T, testCase IntrospectionTestCase[*TaskResponse]) *httptest.ResponseRecorder {
		mockCtrl, mockAgentState, mockMetricsFactory, req, recorder := testHandlerSetup(t, testCase)
		mockAgentState.EXPECT().
			GetTaskMetadataByArn(taskARN).
			Return(testCase.AgentResponse, testCase.Err)
		if testCase.Err != nil {
			mockEntry := mock_metrics.NewMockEntry(mockCtrl)
			mockEntry.EXPECT().Done(testCase.Err)
			mockMetricsFactory.EXPECT().
				New(testCase.MetricName).Return(mockEntry)
		}
		tasksMetadataHandler(mockAgentState, mockMetricsFactory)(recorder, req)
		return recorder
	}
	t.Run("task by arn: happy case", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, taskARNQueryField, taskARN),
			AgentResponse: testTask,
			Err:           nil,
		})
		assert.Equal(t, http.StatusOK, recorder.Code)
		testTaskJSON, _ := json.Marshal(testTask)
		assert.Equal(t, string(testTaskJSON), recorder.Body.String())
	})

	t.Run("task by arn: not found", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, taskARNQueryField, taskARN),
			AgentResponse: nil,
			Err:           NewErrorNotFound(internalErrorText),
			MetricName:    metrics.IntrospectionNotFound,
		})
		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	t.Run("task by arn: fetch failed", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, taskARNQueryField, taskARN),
			AgentResponse: nil,
			Err:           NewErrorFetchFailure(internalErrorText),
			MetricName:    metrics.IntrospectionFetchFailure,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	t.Run("task by arn: unknown error", func(t *testing.T) {
		recorder := performMockTaskRequest(t, IntrospectionTestCase[*TaskResponse]{
			Path:          fmt.Sprintf("%s?%s=%s", tasksMetadataPath, taskARNQueryField, taskARN),
			AgentResponse: nil,
			Err:           errors.New(internalErrorText),
			MetricName:    metrics.IntrospectionInternalServerError,
		})
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

	performMockRequest := func(t *testing.T, testCase IntrospectionTestCase[string]) *httptest.ResponseRecorder {
		mockCtrl, mockAgentState, mockMetricsFactory, req, recorder := testHandlerSetup(t, testCase)
		if testCase.Err != nil {
			mockEntry := mock_metrics.NewMockEntry(mockCtrl)
			mockEntry.EXPECT().Done(testCase.Err)
			mockMetricsFactory.EXPECT().
				New(testCase.MetricName).Return(mockEntry)
		}
		tasksMetadataHandler(mockAgentState, mockMetricsFactory)(recorder, req)
		return recorder
	}
	t.Run("tasks: too many parameters", func(t *testing.T) {
		errorMsg := fmt.Sprintf("Request contains both %s and %s. Expect at most one of these.", dockerIDQueryField, taskARNQueryField)
		recorder := performMockRequest(t, IntrospectionTestCase[string]{
			Path:          fmt.Sprintf("%s?%s=%s&%s=%s", tasksMetadataPath, taskARNQueryField, taskARN, dockerIDQueryField, dockerId),
			AgentResponse: "",
			Err:           errors.New(errorMsg),
			MetricName:    metrics.IntrospectionBadRequest,
		})
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, string(emptyTaskResponse), recorder.Body.String())
	})

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

func TestGetErrorResponse(t *testing.T) {
	t.Run("bad request error", func(t *testing.T) {
		statusCode, metricName := getErrorResponse(NewErrorBadRequest(internalErrorText))
		assert.Equal(t, metrics.IntrospectionBadRequest, metricName)
		assert.Equal(t, http.StatusBadRequest, statusCode)
	})

	t.Run("not found error", func(t *testing.T) {
		statusCode, metricName := getErrorResponse(NewErrorNotFound(internalErrorText))
		assert.Equal(t, metrics.IntrospectionNotFound, metricName)
		assert.Equal(t, http.StatusNotFound, statusCode)
	})

	t.Run("fetch failed error", func(t *testing.T) {
		statusCode, metricName := getErrorResponse(NewErrorFetchFailure(internalErrorText))
		assert.Equal(t, metrics.IntrospectionFetchFailure, metricName)
		assert.Equal(t, http.StatusInternalServerError, statusCode)
	})

	t.Run("unknown error", func(t *testing.T) {
		statusCode, metricName := getErrorResponse(errors.New(internalErrorText))
		assert.Equal(t, metrics.IntrospectionInternalServerError, metricName)
		assert.Equal(t, http.StatusInternalServerError, statusCode)
	})
}
