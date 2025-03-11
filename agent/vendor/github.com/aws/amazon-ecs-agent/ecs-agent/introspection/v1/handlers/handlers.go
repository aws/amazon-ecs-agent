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

// defaultMetadataResponse returns an empty agent metadata response. The response will
// exclude the Version field if hideAgentVersion is true.
func defaultMetadataResponse(hideAgentVersion bool) any {
	if hideAgentVersion {
		return v1.UnversionedAgentMetadataResponse{}
	}
	return v1.AgentMetadataResponse{}
}

// prepareAgentMetadata strips the Version field from the provided agentMetadata response
// if hideAgentVersion is true. If not, it will return the metadata unchanged.
func prepareAgentMetadata(agentMetadata *v1.AgentMetadataResponse, hideAgentVersion bool) any {
	if hideAgentVersion {
		return &v1.UnversionedAgentMetadataResponse{
			Cluster:              agentMetadata.Cluster,
			ContainerInstanceArn: agentMetadata.ContainerInstanceArn,
		}
	}
	return agentMetadata
}

// AgentMetadataHandler returns the HTTP handler function for handling agent metadata requests.
func AgentMetadataHandler(
	agentState v1.AgentState,
	metricsFactory metrics.EntryFactory,
	hideAgentVersion bool,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		agentMetadata, err := agentState.GetAgentMetadata()
		if err != nil {
			logger.Error("Failed to get v1 agent metadata.", logger.Fields{
				field.Error: err,
			})
			responseCode, metricName := getHTTPErrorCode(err)
			metricsFactory.New(metricName).Done(err)
			tmdsutils.WriteJSONResponse(w, responseCode, defaultMetadataResponse(hideAgentVersion), requestTypeAgent)
			return
		}
		tmdsutils.WriteJSONResponse(w, http.StatusOK, prepareAgentMetadata(agentMetadata, hideAgentVersion), requestTypeAgent)
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
				getTaskMetadata(agentState.GetTaskMetadataByID, dockerID, field.DockerId, metricsFactory, w)
				return
			} else {
				getTaskMetadata(agentState.GetTaskMetadataByShortID, dockerID, field.DockerId, metricsFactory, w)
				return
			}
		} else if taskARNExists {
			// Find the task with a matching Arn.
			getTaskMetadata(agentState.GetTaskMetadataByArn, taskArn, field.TaskARN, metricsFactory, w)
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

func getTaskMetadata(
	lookupFn func(key string) (*v1.TaskResponse, error),
	key string,
	keyType string,
	metricsFactory metrics.EntryFactory,
	w http.ResponseWriter,
) {
	taskMetadata, err := lookupFn(key)
	if err != nil {
		logger.Error("Failed to get v1 task metadata.", logger.Fields{
			field.Error: err,
			keyType:     key,
		})
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, v1.TaskResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, taskMetadata, requestTypeTasks)
}
