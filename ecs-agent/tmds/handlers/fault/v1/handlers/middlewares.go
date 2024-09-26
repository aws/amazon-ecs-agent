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

package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
)

// responseWriterWrapper is a wrapper of http.ResponseWriter.
type responseWriterWrapper struct {
	http.ResponseWriter
	status int
}

func (w *responseWriterWrapper) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func TelemetryMiddleware(
	next http.Handler,
	mf metrics.EntryFactory,
	faultOperation, faultType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		rw := &responseWriterWrapper{ResponseWriter: w}

		// Call the next handler in the chain
		next.ServeHTTP(rw, r)

		durationInMs := time.Since(startTime).Milliseconds()
		loggerFields := logger.Fields{
			"StatusCode":   rw.status,
			"DurationInMs": durationInMs,
			"Request":      r.URL.Path,
		}

		// Both the telemetry middleware hanle and other next handlers(including the corresponding
		// start/stop/check fault handlers) are finished.
		logger.Info("The telemetry middleware is complete", loggerFields)
		// Emit a duration metric.
		mf.New(fmt.Sprintf("MetadataServer.%s%sDuration", faultOperation, faultType)).
			WithFields(loggerFields).
			WithGauge(durationInMs).
			Done(nil)
		// Emit a metric when the request fails.
		if rw.status >= 500 && rw.status < 600 {
			mf.New(fmt.Sprintf("MetadataServer.%s%sServerError", faultOperation, faultType)).
				WithFields(loggerFields).
				Done(errors.New("fail to process the request due to a server error"))
		} else if rw.status >= 400 && rw.status < 500 {
			mf.New(fmt.Sprintf("MetadataServer.%s%sClientError", faultOperation, faultType)).
				WithFields(loggerFields).
				Done(errors.New("fail to process the request due to a client error"))
		}
	})
}
