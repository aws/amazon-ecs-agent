// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package taskmetadata

import (
	"net/http"

	"github.com/cihub/seelog"
)

const (
	requestTypeCreds    = "credentials"
	requestTypeMetadata = "metadata"
)

// errorMessage is used to store the human-readable error Code and a descriptive Message
//  that describes the error. This struct is marshalled and returned in the HTTP response.
type errorMessage struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	httpErrorCode int
}

func writeJSONToResponse(w http.ResponseWriter, httpStatusCode int, jsonMessage []byte, requestType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatusCode)
	_, err := w.Write(jsonMessage)
	if err != nil {
		seelog.Errorf(
			"Task metadatda handler[%s]: Unable to write json error message to ResponseWriter",
			requestType)
	}
}
