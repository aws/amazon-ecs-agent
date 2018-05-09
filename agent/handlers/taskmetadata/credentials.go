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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit/request"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

const (
	// Error Types

	// errnoIDInRequest is the error code indicating that no ID was specified
	errNoIDInRequest = "NoIdInRequest"
	// errinvalidIDInRequest is the error code indicating that the ID was invalid
	errInvalidIDInRequest = "InvalidIdInRequest"
	// errnoCredentialsAssociated is the error code indicating no credentials are
	// associated with the specified ID
	errNoCredentialsAssociated = "NoCredentialsAssociated"
	// errcredentialsUninitialized is the error code indicating that credentials were
	// not properly initialized.  This may happen immediately after the agent is
	// started, before it has completed state reconciliation.
	errCredentialsUninitialized = "CredentialsUninitialized"
	// errinternalServerError is the error indicating something generic went wrong
	errInternalServer = "InternalServerError"
)

// credentialsV1V2RequestHandler creates response for the 'v1/credentials' and 'v2/credentials' APIs. It returns a JSON response
// containing credentials when found. The HTTP status code of 400 is returned otherwise.
func credentialsV1V2RequestHandler(credentialsManager credentials.Manager, auditLogger audit.AuditLogger, idFunc func(*http.Request) string, apiVersion int) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		credentialsID := idFunc(r)
		jsonResponse, arn, roleType, errorMessage, err := processCredentialsV1V2Request(credentialsManager, r, credentialsID, apiVersion)
		if err != nil {
			jsonMsg, _ := json.Marshal(errorMessage)
			writeCredentialsV1V2RequestResponse(w, r, errorMessage.httpErrorCode, audit.GetCredentialsEventType(roleType), arn, auditLogger, jsonMsg)
			return
		}

		writeCredentialsV1V2RequestResponse(w, r, http.StatusOK, audit.GetCredentialsEventType(roleType), arn, auditLogger, jsonResponse)
	}
}

// processCredentialsV1V2Request returns the response json containing credentials for the credentials id in the request
func processCredentialsV1V2Request(credentialsManager credentials.Manager, r *http.Request, credentialsID string, apiVersion int) ([]byte, string, string, *errorMessage, error) {
	errPrefix := fmt.Sprintf("CredentialsV%dRequest: ", apiVersion)
	if credentialsID == "" {
		errText := errPrefix + "No ID in the request"
		seelog.Infof("%s. Request IP Address: %s", errText, r.RemoteAddr)
		msg := &errorMessage{
			Code:          errNoIDInRequest,
			Message:       errText,
			httpErrorCode: http.StatusBadRequest,
		}
		return nil, "", "", msg, errors.New(errText)
	}

	credentials, ok := credentialsManager.GetTaskCredentials(credentialsID)
	if !ok {
		errText := errPrefix + "ID not found"
		seelog.Infof("%s. Request IP Address: %s", errText, r.RemoteAddr)
		msg := &errorMessage{
			Code:          errInvalidIDInRequest,
			Message:       errText,
			httpErrorCode: http.StatusBadRequest,
		}
		return nil, "", "", msg, errors.New(errText)
	}

	if utils.ZeroOrNil(credentials) {
		// This can happen when the agent is restarted and is reconciling its state.
		errText := errPrefix + "Credentials uninitialized for ID"
		seelog.Infof("%s. Request IP Address: %s", errText, r.RemoteAddr)
		msg := &errorMessage{
			Code:          errCredentialsUninitialized,
			Message:       errText,
			httpErrorCode: http.StatusServiceUnavailable,
		}
		return nil, "", "", msg, errors.New(errText)
	}

	credentialsJSON, err := json.Marshal(credentials.IAMRoleCredentials)
	if err != nil {
		errText := errPrefix + "Error marshaling credentials"
		seelog.Errorf("%s. Request IP Address: %s", errText, r.RemoteAddr)
		msg := &errorMessage{
			Code:          errInternalServer,
			Message:       "Internal server error",
			httpErrorCode: http.StatusInternalServerError,
		}
		return nil, "", "", msg, errors.New(errText)
	}

	// Success
	return credentialsJSON, credentials.ARN, credentials.IAMRoleCredentials.RoleType, nil, nil
}

func writeCredentialsV1V2RequestResponse(w http.ResponseWriter, r *http.Request, httpStatusCode int, eventType string, arn string, auditLogger audit.AuditLogger, message []byte) {
	auditLogger.Log(request.LogRequest{Request: r, ARN: arn}, httpStatusCode, eventType)

	writeJSONToResponse(w, httpStatusCode, message, requestTypeCreds)
}

func getV1CredentialsID(r *http.Request) string {
	credentialsID, ok := handlers.ValueFromRequest(r, credentials.CredentialsIDQueryParameterName)
	if !ok {
		return ""
	}
	return credentialsID
}

func getV2CredentialsID(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, credentials.CredentialsPath+"/") {
		return r.URL.String()[len(credentials.V2CredentialsPath+"/"):]
	}
	return ""
}
