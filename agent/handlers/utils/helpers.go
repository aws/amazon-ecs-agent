// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"net/http"

	"github.com/cihub/seelog"
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
}

// ValueFromRequest returns the value of a field in the http request. The boolean value is
// set to true if the field exists in the query.
func ValueFromRequest(r *http.Request, field string) (string, bool) {
	values := r.URL.Query()
	_, exists := values[field]
	return values.Get(field), exists
}
