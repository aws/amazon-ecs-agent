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

package v2

import (
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/secrets"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	"github.com/gorilla/mux"
)

const (
	// secretIDMuxName is the key used in gorilla/mux to extract the secret ID
	// from the request URL for secret endpoints.
	secretIDMuxName = "secretIDMuxName"

	// requestTypeSplunkToken is the request type identifier for logging.
	requestTypeSplunkToken = "splunk token"

	// requestTypeContainerEnv is the request type identifier for logging.
	requestTypeContainerEnv = "container env"
)

// SplunkTokenPath specifies the relative URI path for serving Splunk tokens.
// Uses AnythingRegEx to handle the case where secretID is empty (returns 400
// instead of 404).
var SplunkTokenPath = "/v2/splunk-token/" + utils.ConstructMuxVar(secretIDMuxName, utils.AnythingRegEx)

// ContainerEnvPath specifies the relative URI path for serving container
// environment variables.
var ContainerEnvPath = "/v2/container-env/" + utils.ConstructMuxVar(secretIDMuxName, utils.AnythingRegEx)

// SplunkTokenResponse is the JSON response body for the splunk token endpoint.
type SplunkTokenResponse struct {
	Token string `json:"token"`
}

// ContainerEnvResponse is the JSON response body for the container env endpoint.
type ContainerEnvResponse struct {
	Env map[string]string `json:"env"`
}

// SplunkTokenHandler returns an HTTP handler that serves Splunk tokens from the
// provided store. A missing store entry returns 404.
func SplunkTokenHandler(store secrets.SplunkTokenStore) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		secretID := mux.Vars(r)[secretIDMuxName]
		if secretID == "" {
			logger.Error("Empty secret ID in splunk token request", logger.Fields{
				"requestType": requestTypeSplunkToken,
			})
			utils.WriteJSONResponse(w, http.StatusBadRequest, utils.ErrorMessage{
				Code:    "BadRequest",
				Message: "secret ID is empty or malformed",
			}, requestTypeSplunkToken)
			return
		}

		token, ok := store.Get(secretID)
		if !ok {
			logger.Warn("Splunk token not found", logger.Fields{
				"secretID":    secretID,
				"requestType": requestTypeSplunkToken,
			})
			utils.WriteJSONResponse(w, http.StatusNotFound, utils.ErrorMessage{
				Code:    "NotFound",
				Message: fmt.Sprintf("no secret found for secret ID: %s", secretID),
			}, requestTypeSplunkToken)
			return
		}

		logger.Info("Successfully served splunk token", logger.Fields{
			"secretID":    secretID,
			"requestType": requestTypeSplunkToken,
		})
		utils.WriteJSONResponse(w, http.StatusOK, SplunkTokenResponse{Token: token}, requestTypeSplunkToken)
	}
}

// ContainerEnvHandler returns an HTTP handler that serves container environment
// variables from the provided store. A missing store entry returns 404.
func ContainerEnvHandler(store secrets.ContainerEnvStore) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		secretID := mux.Vars(r)[secretIDMuxName]
		if secretID == "" {
			logger.Error("Empty secret ID in container env request", logger.Fields{
				"requestType": requestTypeContainerEnv,
			})
			utils.WriteJSONResponse(w, http.StatusBadRequest, utils.ErrorMessage{
				Code:    "BadRequest",
				Message: "secret ID is empty or malformed",
			}, requestTypeContainerEnv)
			return
		}

		env, ok := store.Get(secretID)
		if !ok {
			logger.Warn("Container env not found", logger.Fields{
				"secretID":    secretID,
				"requestType": requestTypeContainerEnv,
			})
			utils.WriteJSONResponse(w, http.StatusNotFound, utils.ErrorMessage{
				Code:    "NotFound",
				Message: fmt.Sprintf("no secret found for secret ID: %s", secretID),
			}, requestTypeContainerEnv)
			return
		}

		logger.Info("Successfully served container env", logger.Fields{
			"secretID":    secretID,
			"requestType": requestTypeContainerEnv,
		})
		utils.WriteJSONResponse(w, http.StatusOK, ContainerEnvResponse{Env: env}, requestTypeContainerEnv)
	}
}
