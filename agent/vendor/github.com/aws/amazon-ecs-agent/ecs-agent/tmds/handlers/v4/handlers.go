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
package v4

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/gorilla/mux"
)

// v3EndpointIDMuxName is the key that's used in gorilla/mux to get the v3 endpoint ID.
const (
	EndpointContainerIDMuxName = "endpointContainerIDMuxName"
	version                    = "v4"
)

// ContainerMetadataPath specifies the relative URI path for serving container metadata.
func ContainerMetadataPath() string {
	return "/v4/" + utils.ConstructMuxVar(EndpointContainerIDMuxName, utils.AnythingButSlashRegEx)
}

func TaskMetadataPath() string {
	return fmt.Sprintf(
		"/v4/%s/task",
		utils.ConstructMuxVar(EndpointContainerIDMuxName, utils.AnythingButSlashRegEx))
}

// ContainerMetadataHandler returns the HTTP handler function for handling container metadata requests.
func ContainerMetadataHandler(
	agentState state.AgentState,
	metricsFactory metrics.EntryFactory,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		endpointContainerID := mux.Vars(r)[EndpointContainerIDMuxName]
		containerMetadata, err := agentState.GetContainerMetadata(endpointContainerID)
		if err != nil {
			logger.Error("Failed to get v4 container metadata", logger.Fields{
				field.TMDSEndpointContainerID: endpointContainerID,
				field.Error:                   err,
			})

			responseCode, responseBody := getContainerErrorResponse(endpointContainerID, err)
			utils.WriteJSONResponse(w, responseCode, responseBody, utils.RequestTypeContainerMetadata)

			if utils.Is5XXStatus(responseCode) {
				metricsFactory.New(metrics.InternalServerErrorMetricName).Done(err)()
			}

			return
		}

		logger.Info("Writing response for v4 container metadata", logger.Fields{
			field.TMDSEndpointContainerID: endpointContainerID,
			field.Container:               containerMetadata.ID,
		})
		utils.WriteJSONResponse(w, http.StatusOK, containerMetadata, utils.RequestTypeContainerMetadata)
	}
}

// Returns an appropriate HTTP response status code and body for the error.
func getContainerErrorResponse(endpointContainerID string, err error) (int, string) {
	var errLookupFailure *state.ErrorLookupFailure
	if errors.As(err, &errLookupFailure) {
		return http.StatusNotFound, fmt.Sprintf("V4 container metadata handler: %s",
			errLookupFailure.ExternalReason())
	}

	var errMetadataFetchFailure *state.ErrorMetadataFetchFailure
	if errors.As(err, &errMetadataFetchFailure) {
		return http.StatusInternalServerError, errMetadataFetchFailure.ExternalReason()
	}

	logger.Error("Unknown error encountered when handling container metadata fetch failure",
		logger.Fields{field.Error: err})
	return http.StatusInternalServerError, "failed to get container metadata"
}

// TaskMetadataHandler returns the HTTP handler function for handling task metadata requests.
func TaskMetadataHandler(
	agentState state.AgentState,
	metricsFactory metrics.EntryFactory,
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		endpointContainerID := mux.Vars(r)[EndpointContainerIDMuxName]
		taskMetadata, err := agentState.GetTaskMetadata(endpointContainerID)
		if err != nil {
			logger.Error("Failed to get v4 task metadata", logger.Fields{
				field.TMDSEndpointContainerID: endpointContainerID,
				field.Error:                   err,
			})

			responseCode, responseBody := getTaskErrorResponse(endpointContainerID, err)
			utils.WriteJSONResponse(w, responseCode, responseBody, utils.RequestTypeTaskMetadata)

			if utils.Is5XXStatus(responseCode) {
				metricsFactory.New(metrics.InternalServerErrorMetricName).Done(err)()
			}

			return
		}

		logger.Info("Writing response for v4 task metadata", logger.Fields{
			field.TMDSEndpointContainerID: endpointContainerID,
			field.TaskARN:                 taskMetadata.TaskARN,
		})
		utils.WriteJSONResponse(w, http.StatusOK, taskMetadata, utils.RequestTypeContainerMetadata)
	}
}

// Returns an appropriate HTTP response status code and body for the task metadata error.
func getTaskErrorResponse(endpointContainerID string, err error) (int, string) {
	var errContainerLookupFailed *state.ErrorLookupFailure
	if errors.As(err, &errContainerLookupFailed) {
		return http.StatusNotFound, fmt.Sprintf("V4 task metadata handler: %s",
			errContainerLookupFailed.ExternalReason())
	}

	var errFailedToGetContainerMetadata *state.ErrorMetadataFetchFailure
	if errors.As(err, &errFailedToGetContainerMetadata) {
		return http.StatusInternalServerError, errFailedToGetContainerMetadata.ExternalReason()
	}

	logger.Error("Unknown error encountered when handling task metadata fetch failure",
		logger.Fields{field.Error: err})
	return http.StatusInternalServerError, "failed to get task metadata"
}
