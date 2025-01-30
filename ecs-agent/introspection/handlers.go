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
	"net/http"

	v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	tmdsutils "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
)

const (
	requestTypeLicense = "introspection/license"
	licensePath        = "/license"
)

// licenseHandler creates response for '/license' API.
func licenseHandler(agentState v1.AgentState, metricsFactory metrics.EntryFactory) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		text, err := agentState.GetLicenseText()
		if err != nil {
			logger.Error("Failed to get v1 license.", logger.Fields{
				field.Error: err,
			})
			metricsFactory.New(metrics.IntrospectionInternalServerError).Done(err)
			tmdsutils.WriteStringToResponse(w, http.StatusInternalServerError, "", requestTypeLicense)
		} else {
			tmdsutils.WriteStringToResponse(w, http.StatusOK, text, requestTypeLicense)
		}
	}
}

// panicHandler handler will gracefully close the connection if a panic occurs and emit a metric.
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
