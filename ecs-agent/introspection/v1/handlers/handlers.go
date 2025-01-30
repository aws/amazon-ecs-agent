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
	"errors"
	"fmt"
	"net/http"

	v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	tmdsutils "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
)

const (
	dockerIDQueryField = "dockerid"
	taskARNQueryField  = "taskarn"
	dockerShortIDLen   = 12
	requestTypeAgent   = "introspection/agent"
	requestTypeTasks   = "introspection/tasks"

	V1AgentMetadataPath = "/v1/metadata"
	V1TasksMetadataPath = "/v1/tasks"
)

// getHTTPErrorCode returns an appropriate HTTP response status code and metric name for a given error.
func getHTTPErrorCode(err error) (int, string) {

	// Multiple tasks were found, but we expect only one
	var errMultipleTasksFound *v1.ErrorMultipleTasksFound
	if errors.As(err, &errMultipleTasksFound) {
		return errMultipleTasksFound.StatusCode(), errMultipleTasksFound.MetricName()
	}

	// The requested object was not found
	var errNotFound *v1.ErrorNotFound
	if errors.As(err, &errNotFound) {
		return errNotFound.StatusCode(), errNotFound.MetricName()
	}

	// There was an error finding the object, but we have some info to return
	var errFetchFailure *v1.ErrorFetchFailure
	if errors.As(err, &errFetchFailure) {
		return errFetchFailure.StatusCode(), errFetchFailure.MetricName()
	}

	// Some unkown error has occurred
	return http.StatusInternalServerError, metrics.IntrospectionInternalServerError
}

// AgentMetadataHandler returns the HTTP handler function for handling agent metadata requests.
func AgentMetadataHandler(
	agentState v1.AgentState,
	metricsFactory metrics.EntryFactory,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		agentMetadata, err := agentState.GetAgentMetadata()
		if err != nil {
			logger.Error("Failed to get v1 agent metadata.", logger.Fields{
				field.Error: err,
			})
			responseCode, metricName := getHTTPErrorCode(err)
			metricsFactory.New(metricName).Done(err)
			tmdsutils.WriteJSONResponse(w, responseCode, v1.AgentMetadataResponse{}, requestTypeAgent)
			return
		}
		tmdsutils.WriteJSONResponse(w, http.StatusOK, agentMetadata, requestTypeAgent)
	}
}

// TasksMetadataHandler returns the HTTP handler function for handling tasks metadata requests.
func TasksMetadataHandler(
	agentState v1.AgentState,
	metricsFactory metrics.EntryFactory,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		dockerID, dockerIDExists := tmdsutils.ValueFromRequest(r, dockerIDQueryField)
		taskArn, taskARNExists := tmdsutils.ValueFromRequest(r, taskARNQueryField)
		if dockerIDExists && taskARNExists {
			errorMsg := fmt.Sprintf("request contains both %s and %s but expect at most one of these", dockerIDQueryField, taskARNQueryField)
			logger.Error("Bad request for v1 task metadata.", logger.Fields{
				field.Error: errorMsg,
			})
			metricsFactory.New(metrics.IntrospectionBadRequest).Done(errors.New(errorMsg))
			tmdsutils.WriteJSONResponse(w, http.StatusBadRequest, v1.TaskResponse{}, requestTypeTasks)
			return
		}
		if dockerIDExists {
			// Find the task that has a container with a matching docker ID.
			if len(dockerID) > dockerShortIDLen {
				getTaskByID(agentState, metricsFactory, dockerID, w)
				return
			} else {
				getTaskByShortID(agentState, metricsFactory, dockerID, w)
				return
			}
		} else if taskARNExists {
			// Find the task with a matching Arn.
			getTaskByARN(agentState, metricsFactory, taskArn, w)
			return
		} else {
			// Return all tasks.
			getTasksMetadata(agentState, metricsFactory, w)
		}
	}
}

// getTasksMetadata writes a list of metadata for all tasks to the response
func getTasksMetadata(
	agentState v1.AgentState,
	metricsFactory metrics.EntryFactory,
	w http.ResponseWriter,
) {
	tasksMetadata, err := agentState.GetTasksMetadata()
	if err != nil {
		logger.Error("Failed to get v1 tasks metadata.", logger.Fields{
			field.Error: err,
		})
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, v1.TasksResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, tasksMetadata, requestTypeTasks)
}

// getTaskByARN writes metadata for the corresponding task to the response, or an
// error status if the agent cannot return the metadata
func getTaskByARN(
	agentState v1.AgentState,
	metricsFactory metrics.EntryFactory,
	taskARN string,
	w http.ResponseWriter,
) {
	taskMetadata, err := agentState.GetTaskMetadataByArn(taskARN)
	if err != nil {
		logger.Error("Failed to get v1 task metadata.", logger.Fields{
			field.Error:   err,
			field.TaskARN: taskARN,
		})
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, v1.TaskResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, taskMetadata, requestTypeTasks)
}

// getTaskByID writes metadata for the corresponding task to the response, or an
// error status if the agent cannot return the metadata
func getTaskByID(
	agentState v1.AgentState,
	metricsFactory metrics.EntryFactory,
	dockerID string,
	w http.ResponseWriter,
) {
	taskMetadata, err := agentState.GetTaskMetadataByID(dockerID)
	if err != nil {
		logger.Error("Failed to get v1 task metadata.", logger.Fields{
			field.Error:    err,
			field.DockerId: dockerID,
		})
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, v1.TaskResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, taskMetadata, requestTypeTasks)
}

// getTaskByShortID writes metadata for the corresponding task to the response, or an
// error status if the agent cannot return the metadata
func getTaskByShortID(
	agentState v1.AgentState,
	metricsFactory metrics.EntryFactory,
	shortDockerID string,
	w http.ResponseWriter,
) {
	taskMetadata, err := agentState.GetTaskMetadataByShortID(shortDockerID)
	if err != nil {
		logger.Error("Failed to get v1 task metadata.", logger.Fields{
			field.Error:    err,
			field.DockerId: shortDockerID,
		})
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, v1.TaskResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, taskMetadata, requestTypeTasks)
}
