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

package logging

import (
	"net/http"

	"github.com/cihub/seelog"
)

// LoggingHandler is used to log all requests for an endpoint.
type LoggingHandler struct{ h http.Handler }

// NewLoggingHandler creates a new LoggingHandler object.
func NewLoggingHandler(handler http.Handler) LoggingHandler {
	return LoggingHandler{h: handler}
}

// ServeHTTP logs the method and remote address of the request.
func (lh LoggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	seelog.Debug("Handling http request", "method", r.Method, "from", r.RemoteAddr)
	lh.h.ServeHTTP(w, r)
}
