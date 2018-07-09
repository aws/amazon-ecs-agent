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
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	"github.com/aws/amazon-ecs-agent/agent/logger/audit"
)

// Credentials API version.
const apiVersion = 2

// CredentialsHandler creates response for the 'v2/credentials' API.
func CredentialsHandler(credentialsManager credentials.Manager, auditLogger audit.AuditLogger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		credentialsID := getCredentialsID(r)
		errPrefix := fmt.Sprintf("CredentialsV%dRequest: ", apiVersion)
		v1.CredentialsHandlerImpl(w, r, auditLogger, credentialsManager, credentialsID, errPrefix)
	}
}

func getCredentialsID(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, credentials.CredentialsPath+"/") {
		return r.URL.String()[len(credentials.V2CredentialsPath+"/"):]
	}
	return ""
}
