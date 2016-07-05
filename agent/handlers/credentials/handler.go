// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package credentials

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit/request"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	log "github.com/cihub/seelog"
)

const (
	// readTimeout specifies the maximum duration before timing out read of the request.
	// The value is set to 5 seconds as per AWS SDK defaults.
	readTimeout = 5 * time.Second
	// writeTimeout specifies the maximum duration before timing out write of the response.
	// The value is set to 5 seconds as per AWS SDK defaults.
	writeTimeout = 5 * time.Second

	// Error Types
	NoIdInRequest            = "NoIdInRequest"
	InvalidIdInRequest       = "InvalidIdInRequest"
	NoCredentialsAssociated  = "NoCredentialsAssociated"
	CredentialsUninitialized = "CredentialsUninitialized"
	InternalServerError      = "InternalServerError"
)

// errorMessage is used to store the human-readable error Code and a descriptive Message
//  that describes the error. This struct is marshalled and returned in the HTTP response.
type errorMessage struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	httpErrorCode int
}

// ServeHttp serves IAM Role Credentials for Tasks being managed by the agent.
func ServeHttp(credentialsManager credentials.Manager, containerInstanceArn string, cfg *config.Config) {
	// Create and initialize the audit log
	// TODO Use seelog's programmatic configuration instead of xml.
	logger, err := log.LoggerFromConfigAsString(audit.AuditLoggerConfig(cfg))
	if err != nil {
		log.Errorf("Error initializing the audit log: %v", err)
		// If the logger cannot be initialized, use the provided dummy seelog.LoggerInterface, seelog.Disabled.
		logger = log.Disabled
	}

	auditLogger := audit.NewAuditLog(containerInstanceArn, cfg, logger)

	server := setupServer(credentialsManager, auditLogger)

	for {
		utils.RetryWithBackoff(utils.NewSimpleBackoff(time.Second, time.Minute, 0.2, 2), func() error {
			// TODO, make this cancellable and use the passed in context;
			err := server.ListenAndServe()
			if err != nil {
				log.Errorf("Error running http api: %v", err)
			}
			return err
		})
	}
}

// setupServer starts the HTTP server for serving IAM Role Credentials for Tasks.
func setupServer(credentialsManager credentials.Manager, auditLogger audit.AuditLogger) http.Server {
	serverMux := http.NewServeMux()
	serverMux.HandleFunc(credentials.CredentialsPath, credentialsV1RequestHandler(credentialsManager, auditLogger))

	// Log all requests and then pass through to serverMux
	loggingServeMux := http.NewServeMux()
	loggingServeMux.Handle("/", handlers.NewLoggingHandler(serverMux))

	server := http.Server{
		Addr:         ":" + strconv.Itoa(config.AgentCredentialsPort),
		Handler:      loggingServeMux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	return server
}

// credentialsV1RequestHandler creates response for the 'v1/credentials' API. It returns a JSON response
// containing credentials when found. The HTTP status code of 400 is returned otherwise.
func credentialsV1RequestHandler(credentialsManager credentials.Manager, auditLogger audit.AuditLogger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		jsonResponse, arn, errorMessage, err := processCredentialsV1Request(credentialsManager, r)
		if err != nil {
			jsonMsg, _ := json.Marshal(errorMessage)
			writeCredentialsV1RequestResponse(w, r, errorMessage.httpErrorCode, audit.GetCredentialsEventType(), arn, auditLogger, jsonMsg)
			return
		}

		writeCredentialsV1RequestResponse(w, r, http.StatusOK, audit.GetCredentialsEventType(), arn, auditLogger, jsonResponse)
	}
}

func writeCredentialsV1RequestResponse(w http.ResponseWriter, r *http.Request, httpStatusCode int, eventType string, arn string, auditLogger audit.AuditLogger, message []byte) {
	auditLogger.Log(request.LogRequest{Request: r, ARN: arn}, httpStatusCode, eventType)

	writeJsonToResponse(w, httpStatusCode, message)
}

// processCredentialsV1Request returns the response json containing credentials for the credentials id in the request
func processCredentialsV1Request(credentialsManager credentials.Manager, r *http.Request) ([]byte, string, *errorMessage, error) {
	credentialsId, ok := handlers.ValueFromRequest(r, credentials.CredentialsIdQueryParameterName)

	if !ok {
		errText := "CredentialsV1Request: No ID in the request"
		log.Infof("%s. Request IP Address: %s", errText, r.RemoteAddr)
		msg := &errorMessage{
			Code:          NoIdInRequest,
			Message:       errText,
			httpErrorCode: http.StatusBadRequest,
		}
		return nil, "", msg, errors.New(errText)
	}

	credentials, ok := credentialsManager.GetTaskCredentials(credentialsId)
	if !ok {
		errText := "CredentialsV1Request: ID not found"
		log.Infof("%s. Request IP Address: %s", errText, r.RemoteAddr)
		msg := &errorMessage{
			Code:          InvalidIdInRequest,
			Message:       errText,
			httpErrorCode: http.StatusBadRequest,
		}
		return nil, "", msg, errors.New(errText)
	}

	if credentials == nil {
		// This can happen when the agent is restarted and is reconciling its state.
		errText := "CredentialsV1Request: Credentials uninitialized for ID"
		log.Infof("%s. Request IP Address: %s", errText, r.RemoteAddr)
		msg := &errorMessage{
			Code:          CredentialsUninitialized,
			Message:       errText,
			httpErrorCode: http.StatusServiceUnavailable,
		}
		return nil, "", msg, errors.New(errText)
	}

	credentialsJSON, err := json.Marshal(credentials.IAMRoleCredentials)
	if err != nil {
		errText := "CredentialsV1Request: Error marshaling credentials"
		log.Errorf("%s. Request IP Address: %s", errText, r.RemoteAddr)
		msg := &errorMessage{
			Code:          InternalServerError,
			Message:       "Internal server error",
			httpErrorCode: http.StatusInternalServerError,
		}
		return nil, "", msg, errors.New(errText)
	}

	//Success
	return credentialsJSON, credentials.ARN, nil, nil
}

func writeJsonToResponse(w http.ResponseWriter, httpStatusCode int, jsonMessage []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatusCode)
	_, err := w.Write(jsonMessage)
	if err != nil {
		log.Error("handlers/credentials: Error writing json error message to ResponseWriter")
	}
}
