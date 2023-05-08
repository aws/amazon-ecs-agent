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
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/request"
	"github.com/cihub/seelog"
	"github.com/gorilla/mux"
)

const (
	// NetworkModeAWSVPC specifies the AWS VPC network mode.
	NetworkModeAWSVPC = "awsvpc"

	// RequestTypeCreds specifies the request type of CredentialsHandler.
	RequestTypeCreds = "credentials"

	// RequestTypeTaskMetadata specifies the task metadata request type of TaskContainerMetadataHandler.
	RequestTypeTaskMetadata = "task metadata"

	// RequestTypeContainerMetadata specifies the container metadata request type of TaskContainerMetadataHandler.
	RequestTypeContainerMetadata = "container metadata"

	// RequestTypeTaskStats specifies the task stats request type of StatsHandler.
	RequestTypeTaskStats = "task stats"

	// RequestTypeContainerStats specifies the container stats request type of StatsHandler.
	RequestTypeContainerStats = "container stats"

	// RequestTypeAgentMetadata specifies the Agent metadata request type of AgentMetadataHandler.
	RequestTypeAgentMetadata = "agent metadata"

	// RequestTypeContainerAssociations specifies the container associations request type of ContainerAssociationsHandler.
	RequestTypeContainerAssociations = "container associations"

	// RequestTypeContainerAssociation specifies the container association request type of ContainerAssociationHandler.
	RequestTypeContainerAssociation = "container association"

	// AnythingButSlashRegEx is a regex pattern that matches any string without slash.
	AnythingButSlashRegEx = "[^/]*"

	// AnythingRegEx is a regex pattern that matches anything.
	AnythingRegEx = ".*"

	// AnythingButEmptyRegEx is a regex pattern that matches anything but an empty string.
	AnythingButEmptyRegEx = ".+"
)

// ErrorMessage is used to store the human-readable error Code and a descriptive Message
// that describes the error. This struct is marshalled and returned in the HTTP response.
type ErrorMessage struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	HTTPErrorCode int
}

// WriteJSONToResponse writes the header, JSON response to a ResponseWriter, and
// log the error if necessary.
func WriteJSONToResponse(w http.ResponseWriter, httpStatusCode int, responseJSON []byte, requestType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatusCode)
	_, err := w.Write(responseJSON)
	if err != nil {
		seelog.Errorf(
			"Unable to write %s json response message to ResponseWriter",
			requestType)
	}

	if httpStatusCode >= 400 && httpStatusCode <= 599 {
		seelog.Errorf("HTTP response status code is '%d', request type is: %s, and response in JSON is %s", httpStatusCode, requestType, string(responseJSON))
	}
}

// WriteResponseIfMarshalError checks the 'err' response of the json.Marshal function.
// if this function returns an error, then it has already written a response to the
// http writer, and the calling function should return.
func WriteResponseIfMarshalError(w http.ResponseWriter, err error) error {
	if err != nil {
		WriteJSONToResponse(w, http.StatusInternalServerError, []byte(`{}`), RequestTypeAgentMetadata)
		seelog.Errorf("Error marshaling json: %s", err)
		return fmt.Errorf("json marshal error")
	}
	return nil
}

// ValueFromRequest returns the value of a field in the http request. The boolean value is
// set to true if the field exists in the query.
func ValueFromRequest(r *http.Request, field string) (string, bool) {
	values := r.URL.Query()
	_, exists := values[field]
	return values.Get(field), exists
}

// GetMuxValueFromRequest extracts the mux value from the request using a gorilla
// mux name
func GetMuxValueFromRequest(r *http.Request, gorillaMuxName string) (string, bool) {
	vars := mux.Vars(r)
	val, ok := vars[gorillaMuxName]
	return val, ok
}

// ConstructMuxVar constructs the mux var that is used in the gorilla/mux styled
// path, example: {id}, {id:[0-9]+}.
func ConstructMuxVar(name string, pattern string) string {
	if pattern == "" {
		return "{" + name + "}"
	}

	return "{" + name + ":" + pattern + "}"
}

// LimitReachedHandler logs the throttled request in the credentials audit log
func LimitReachedHandler(auditLogger audit.AuditLogger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		logRequest := request.LogRequest{
			Request: r,
		}
		auditLogger.Log(logRequest, http.StatusTooManyRequests, "")
	}
}
