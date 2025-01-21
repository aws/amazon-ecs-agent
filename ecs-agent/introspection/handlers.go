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
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	tmdsutils "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
)

const (
	dockerIDQueryField = "dockerid"
	taskARNQueryField  = "taskarn"
	dockerShortIDLen   = 12
	requestTypeAgent   = "introspection/agent"
	requestTypeTasks   = "introspection/tasks"
	requestTypeLicense = "introspection/license"
	licensePath        = "/license"
	agentMetadataPath  = "/v1/metadata"
	tasksMetadataPath  = "/v1/tasks"
)

// licenseHandler creates response for '/license' API.
func licenseHandler(agentState AgentState, metricsFactory metrics.EntryFactory) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		text, err := agentState.GetLicenseText()
		if err != nil {
			metricsFactory.New(metrics.IntrospectionInternalServerError).Done(err)
			tmdsutils.WriteStringToResponse(w, http.StatusInternalServerError, "", requestTypeLicense)
		} else {
			tmdsutils.WriteStringToResponse(w, http.StatusOK, text, requestTypeLicense)
		}
	}
}

// getHTTPErrorCode returns an appropriate HTTP response status code and body for the error.
func getHTTPErrorCode(err error) (int, string) {
	// There was something wrong with the request
	var errBadRequest *ErrorBadRequest
	if errors.As(err, &errBadRequest) {
		return errBadRequest.StatusCode(), errBadRequest.MetricName()
	}

	// The requested object was not found
	var errNotFound *ErrorNotFound
	if errors.As(err, &errNotFound) {
		return errNotFound.StatusCode(), errNotFound.MetricName()
	}

	// There was an error finding the object, but we have some info to return
	var errFetchFailure *ErrorFetchFailure
	if errors.As(err, &errFetchFailure) {
		return errFetchFailure.StatusCode(), errFetchFailure.MetricName()
	}

	// Some unkown error has occurred
	return http.StatusInternalServerError, metrics.IntrospectionInternalServerError
}

// agentMetadataHandler returns the HTTP handler function for handling agent metadata requests.
func agentMetadataHandler(
	agentState AgentState,
	metricsFactory metrics.EntryFactory,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		agentMetadata, err := agentState.GetAgentMetadata()
		if err != nil {
			responseCode, metricName := getHTTPErrorCode(err)
			metricsFactory.New(metricName).Done(err)
			tmdsutils.WriteJSONResponse(w, responseCode, AgentMetadataResponse{}, requestTypeAgent)
			return
		}
		tmdsutils.WriteJSONResponse(w, http.StatusOK, agentMetadata, requestTypeAgent)
	}
}

// tasksMetadataHandler returns the HTTP handler function for handling tasks metadata requests.
func tasksMetadataHandler(
	agentState AgentState,
	metricsFactory metrics.EntryFactory,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		dockerID, dockerIDExists := tmdsutils.ValueFromRequest(r, dockerIDQueryField)
		taskArn, taskARNExists := tmdsutils.ValueFromRequest(r, taskARNQueryField)
		if dockerIDExists && taskARNExists {
			errorMsg := fmt.Sprintf("Request contains both %s and %s. Expect at most one of these.", dockerIDQueryField, taskARNQueryField)
			metricsFactory.New(metrics.IntrospectionBadRequest).Done(errors.New(errorMsg))
			tmdsutils.WriteJSONResponse(w, http.StatusBadRequest, TaskResponse{}, requestTypeTasks)
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
			getTaskByArn(agentState, metricsFactory, taskArn, w)
			return
		} else {
			// Return all tasks.
			getTasksMetadata(agentState, metricsFactory, w)
		}
	}
}

// getTasksMetadata writes a list of metadata for all tasks to the response
func getTasksMetadata(
	agentState AgentState,
	metricsFactory metrics.EntryFactory,
	w http.ResponseWriter,
) {
	tasksMetadata, err := agentState.GetTasksMetadata()
	if err != nil {
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, TasksResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, tasksMetadata, requestTypeTasks)
}

// getTaskByArn writes metadata for the corresponding task to the response, or an
// error status if the agent cannot return the metadata
func getTaskByArn(
	agentState AgentState,
	metricsFactory metrics.EntryFactory,
	taskArn string,
	w http.ResponseWriter,
) {
	taskMetadata, err := agentState.GetTaskMetadataByArn(taskArn)
	if err != nil {
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, TaskResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, taskMetadata, requestTypeTasks)
}

// getTaskByID writes metadata for the corresponding task to the response, or an
// error status if the agent cannot return the metadata
func getTaskByID(
	agentState AgentState,
	metricsFactory metrics.EntryFactory,
	dockerID string,
	w http.ResponseWriter,
) {
	taskMetadata, err := agentState.GetTaskMetadataByID(dockerID)
	if err != nil {
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, TaskResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, taskMetadata, requestTypeTasks)
}

// getTaskByShortID writes metadata for the corresponding task to the response, or an
// error status if the agent cannot return the metadata
func getTaskByShortID(
	agentState AgentState,
	metricsFactory metrics.EntryFactory,
	shortDockerID string,
	w http.ResponseWriter,
) {
	taskMetadata, err := agentState.GetTaskMetadataByShortID(shortDockerID)
	if err != nil {
		responseCode, metricName := getHTTPErrorCode(err)
		metricsFactory.New(metricName).Done(err)
		tmdsutils.WriteJSONResponse(w, responseCode, TaskResponse{}, requestTypeTasks)
		return
	}
	tmdsutils.WriteJSONResponse(w, http.StatusOK, taskMetadata, requestTypeTasks)
}

// panicHandler handler will gracefully close the connection if a panic occurs, returning
// an internal server error to the client.
func panicHandler(next http.Handler, metricsFactory metrics.EntryFactory) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				var err error
				switch x := r.(type) {
				case string:
					err = errors.New(x)
				case error:
					err = x
				default:
					err = errors.New("unknown panic")
				}
				w.Header().Set("Connection", "close")
				w.WriteHeader(http.StatusInternalServerError)
				metricsFactory.New(metrics.IntrospectionCrash).Done(err)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
