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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	mock_state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state/mocks"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	endpointId  = "endpointId"
	port        = 1234
	protocol    = "tcp"
	trafficType = "ingress"
)

// Tests the path for Fault Network Faults API
func TestFaultBlackholeFaultPath(t *testing.T) {
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-blackhole-port", FaultNetworkFaultPath(types.BlackHolePortFaultType))
}

func TestFaultLatencyFaultPath(t *testing.T) {
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-latency", FaultNetworkFaultPath(types.LatencyFaultType))
}

func TestFaultPacketLossFaultPath(t *testing.T) {
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-packet-loss", FaultNetworkFaultPath(types.PacketLossFaultType))
}

type networkBlackHolePortTestCase struct {
	name                      string
	expectedStatusCode        int
	requestBody               interface{}
	expectedResponseBody      types.NetworkFaultInjectionResponse
	setAgentStateExpectations func(agentState *mock_state.MockAgentState)
}

func getNetworkBlackHolePortTestCases(name string, expectedHappyResponseBody string) []networkBlackHolePortTestCase {
	happyBlackHolePortReqBody := map[string]interface{}{
		"Port":        port,
		"Protocol":    protocol,
		"TrafficType": trafficType,
	}
	tcs := []networkBlackHolePortTestCase{
		{
			name:                 fmt.Sprintf("%s success", name),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse(expectedHappyResponseBody),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, nil)
			},
		},
		{
			name:               fmt.Sprintf("%s unknown request body", name),
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": trafficType,
				"Unknown":     "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse(expectedHappyResponseBody),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, nil)
			},
		},
		{
			name:               fmt.Sprintf("%s malformed request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":        "incorrect typing",
				"Protocol":    protocol,
				"TrafficType": trafficType,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal string into Go struct field NetworkBlackholePortRequest.Port of type uint16"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s incomplete request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":     port,
				"Protocol": protocol,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required parameter TrafficType is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s empty value request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required parameter TrafficType is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid port value request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    "invalid",
				"TrafficType": trafficType,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value invalid for parameter Protocol"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid traffic type value request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": "invalid",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value invalid for parameter TrafficType"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:                 fmt.Sprintf("%s task lookup fail", name),
			expectedStatusCode:   404,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("unable to lookup container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, state.NewErrorLookupFailure("task lookup failed"))
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata fetch fail", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("unable to obtain container metadata for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(
					"Unable to generate metadata for task"))
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata unknown fail", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState) {
				agentState.EXPECT().GetTaskMetadata(endpointId).Return(state.TaskResponse{}, errors.New("unknown error"))
			},
		},
	}
	return tcs
}

func TestStartNetworkBlackHolePort(t *testing.T) {
	tcs := getNetworkBlackHolePortTestCases("start blackhole port", "running")

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// Mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			agentState := mock_state.NewMockAgentState(ctrl)
			metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

			if tc.setAgentStateExpectations != nil {
				tc.setAgentStateExpectations(agentState)
			}

			router := mux.NewRouter()

			handler := FaultHandler{
				AgentState:     agentState,
				MetricsFactory: metricsFactory,
			}

			router.HandleFunc(
				FaultNetworkFaultPath(types.BlackHolePortFaultType),
				handler.StartNetworkBlackholePort(),
			).Methods("PUT")

			method := "PUT"
			var requestBody io.Reader
			if tc.requestBody != nil {
				reqBodyBytes, err := json.Marshal(tc.requestBody)
				require.NoError(t, err)
				requestBody = bytes.NewReader(reqBodyBytes)
			}
			req, err := http.NewRequest(method, fmt.Sprintf("/api/%s/fault/v1/network-blackhole-port", endpointId),
				requestBody)
			require.NoError(t, err)

			// Send the request and record the response
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			var actualResponseBody types.NetworkFaultInjectionResponse
			err = json.Unmarshal(recorder.Body.Bytes(), &actualResponseBody)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatusCode, recorder.Code)
			assert.Equal(t, tc.expectedResponseBody, actualResponseBody)

		})
	}
}

func TestStopNetworkBlackHolePort(t *testing.T) {
	tcs := getNetworkBlackHolePortTestCases("stop blackhole port", "stopped")
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// Mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			agentState := mock_state.NewMockAgentState(ctrl)
			metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

			if tc.setAgentStateExpectations != nil {
				tc.setAgentStateExpectations(agentState)
			}

			router := mux.NewRouter()

			handler := FaultHandler{
				AgentState:     agentState,
				MetricsFactory: metricsFactory,
			}

			router.HandleFunc(
				FaultNetworkFaultPath(types.BlackHolePortFaultType),
				handler.StopNetworkBlackHolePort(),
			).Methods("DELETE")

			method := "DELETE"
			var requestBody io.Reader
			if tc.requestBody != nil {
				reqBodyBytes, err := json.Marshal(tc.requestBody)
				require.NoError(t, err)
				requestBody = bytes.NewReader(reqBodyBytes)
			}
			req, err := http.NewRequest(method, fmt.Sprintf("/api/%s/fault/v1/network-blackhole-port", endpointId),
				requestBody)
			require.NoError(t, err)

			// Send the request and record the response
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			var actualResponseBody types.NetworkFaultInjectionResponse
			err = json.Unmarshal(recorder.Body.Bytes(), &actualResponseBody)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatusCode, recorder.Code)
			assert.Equal(t, tc.expectedResponseBody, actualResponseBody)

		})
	}
}

func TestCheckNetworkBlackHolePort(t *testing.T) {
	tcs := getNetworkBlackHolePortTestCases("check blackhole port", "running")

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// Mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			agentState := mock_state.NewMockAgentState(ctrl)
			metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

			router := mux.NewRouter()

			handler := FaultHandler{
				AgentState:     agentState,
				MetricsFactory: metricsFactory,
			}

			if tc.setAgentStateExpectations != nil {
				tc.setAgentStateExpectations(agentState)
			}

			router.HandleFunc(
				FaultNetworkFaultPath(types.BlackHolePortFaultType),
				handler.StartNetworkBlackholePort(),
			).Methods("GET")

			method := "GET"
			var requestBody io.Reader
			if tc.requestBody != nil {
				reqBodyBytes, err := json.Marshal(tc.requestBody)
				require.NoError(t, err)
				requestBody = bytes.NewReader(reqBodyBytes)
			}
			req, err := http.NewRequest(method, fmt.Sprintf("/api/%s/fault/v1/network-blackhole-port", endpointId),
				requestBody)
			require.NoError(t, err)

			// Send the request and record the response
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			var actualResponseBody types.NetworkFaultInjectionResponse
			err = json.Unmarshal(recorder.Body.Bytes(), &actualResponseBody)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatusCode, recorder.Code)
			assert.Equal(t, tc.expectedResponseBody, actualResponseBody)

		})
	}
}
