//go:build unit
// +build unit

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
	"net/http"
	"net/http/httptest"
	"testing"

	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// TestMiddlewaresHandler verifies the expected metrics will be emitted.
func TestMiddlewaresHandler(t *testing.T) {
	tcs := []struct {
		Name                              string
		StatusCode                        int
		FaultType                         string
		FaultOperation                    string
		ExpectedEmittedDurationMetricName string
		ExpectedEmittedErrorMetricName    string
		ExpectedErrorMsgInMetricLogs      string
	}{
		{
			Name:                              "Server returns 200",
			StatusCode:                        200,
			FaultType:                         "AnyFault",
			FaultOperation:                    "Start",
			ExpectedEmittedDurationMetricName: "MetadataServer.StartAnyFaultDuration",
			ExpectedEmittedErrorMetricName:    "",
			ExpectedErrorMsgInMetricLogs:      "",
		},
		{
			Name:                              "Server returns 500",
			StatusCode:                        500,
			FaultType:                         "NetworkBlackholePort",
			FaultOperation:                    "Start",
			ExpectedEmittedDurationMetricName: "MetadataServer.StartNetworkBlackholePortDuration",
			ExpectedEmittedErrorMetricName:    "MetadataServer.StartNetworkBlackholePortServerError",
			ExpectedErrorMsgInMetricLogs:      "fail to process the request due to a server error",
		},
		{
			Name:                              "Server returns 400",
			StatusCode:                        400,
			FaultType:                         "NetworkLatency",
			FaultOperation:                    "Start",
			ExpectedEmittedDurationMetricName: "MetadataServer.StartNetworkLatencyDuration",
			ExpectedEmittedErrorMetricName:    "MetadataServer.StartNetworkLatencyClientError",
			ExpectedErrorMsgInMetricLogs:      "fail to process the request due to a client error",
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create a mock handler
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Write a response that the middleware should modify
				w.WriteHeader(tc.StatusCode)
				w.Write([]byte("Expected hello world from handler"))
			})

			// Create the mock metrics facotry
			metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
			// Verify the duration metric is emitted as expected
			durationMetricEntry := mock_metrics.NewMockEntry(ctrl)
			gomock.InOrder(
				metricsFactory.EXPECT().New(tc.ExpectedEmittedDurationMetricName).Return(durationMetricEntry).Times(1),
				durationMetricEntry.EXPECT().WithFields(gomock.Any()).Return(durationMetricEntry).Times(1),
				durationMetricEntry.EXPECT().WithGauge(gomock.Any()).Return(durationMetricEntry).Times(1),
				durationMetricEntry.EXPECT().Done(nil).Times(1),
			)
			// verify the error metric is emitted as expected
			if tc.StatusCode >= 400 {
				entry := mock_metrics.NewMockEntry(ctrl)
				gomock.InOrder(
					metricsFactory.EXPECT().New(tc.ExpectedEmittedErrorMetricName).Return(entry).Times(1),
					entry.EXPECT().WithFields(gomock.Any()).Return(entry).Times(1),
					entry.EXPECT().Done(errors.New(tc.ExpectedErrorMsgInMetricLogs)).Times(1),
				)
			}

			middleware := TelemetryMiddleware(
				nextHandler,
				metricsFactory,
				tc.FaultOperation,
				tc.FaultType,
			)

			// Create a test request
			req := httptest.NewRequest("GET", "/", nil)

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Serve the request through the middleware
			middleware.ServeHTTP(rr, req)

			// Check the response status code
			assert.Equal(t, rr.Code, tc.StatusCode)
			assert.Equal(t, "Expected hello world from handler", rr.Body.String())
		})
	}
}
