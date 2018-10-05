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

package v2

import (
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
	"github.com/gorilla/mux"
)

const (
	// Credentials API version.
	apiVersion = 2

	// credentialsIDMuxName is the key that's used in gorilla/mux to get the credentials ID.
	credentialsIDMuxName = "credentialsIDMuxName"
)

// CredentialsPath specifies the relative URI path for serving task IAM credentials.
// Use "AnythingRegEx" regex to handle the case where the "credentialsIDMuxName" is
// empty, this is because the name that's used to extract dynamic value in gorilla cannot
// be empty by default. If we don't do this, we will get 404 error when we access "/v2/credentials/",
// but it should be 400 error.
var CredentialsPath = credentials.V2CredentialsPath + "/" + utils.ConstructMuxVar(credentialsIDMuxName, utils.AnythingRegEx)

// CredentialsHandler creates response for the 'v2/credentials' API.
func CredentialsHandler(credentialsManager credentials.Manager, auditLogger audit.AuditLogger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		credentialsID := getCredentialsID(r)
		errPrefix := fmt.Sprintf("CredentialsV%dRequest: ", apiVersion)
		v1.CredentialsHandlerImpl(w, r, auditLogger, credentialsManager, credentialsID, errPrefix)
	}
}

func getCredentialsID(r *http.Request) string {
	vars := mux.Vars(r)
	return vars[credentialsIDMuxName]
}
