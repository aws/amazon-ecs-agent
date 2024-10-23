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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	mock_state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils/netconfig"
	mock_execwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	endpointId                        = "endpointId"
	port                              = 1234
	protocol                          = "tcp"
	trafficType                       = "ingress"
	delayMilliseconds                 = 123456789
	jitterMilliseconds                = 4567
	lossPercent                       = 6
	taskARN                           = "taskArn"
	awsvpcNetworkMode                 = "awsvpc"
	deviceName                        = "eth0"
	invalidNetworkMode                = "invalid"
	iptablesChainNotFoundError        = "iptables: Bad rule (does a matching rule exist in that chain?)."
	tcLatencyFaultExistsCommandOutput = `[{"kind":"netem","handle":"10:","parent":"1:1","options":{"limit":1000,"delay":{"delay":123456789,"jitter":4567,"correlation":0},"ecn":false,"gap":0}}]`
	tcLossFaultExistsCommandOutput    = `[{"kind":"netem","handle":"10:","dev":"eth0","parent":"1:1","options":{"limit":1000,"loss-random":{"loss":0.06,"correlation":0},"ecn":false,"gap":0}}]`
	tcCommandEmptyOutput              = `[]`
)

var (
	noDeviceNameInNetworkInterfaces = []*state.NetworkInterface{
		{
			DeviceName: "",
		},
	}

	happyNetworkInterfaces = []*state.NetworkInterface{
		{
			DeviceName: deviceName,
		},
	}

	happyNetworkNamespaces = []*state.NetworkNamespace{
		{
			Path:              "/some/path",
			NetworkInterfaces: happyNetworkInterfaces,
		},
	}

	noPathInNetworkNamespaces = []*state.NetworkNamespace{
		{
			Path:              "",
			NetworkInterfaces: happyNetworkInterfaces,
		},
	}

	happyTaskNetworkConfig = state.TaskNetworkConfig{
		NetworkMode:       awsvpcNetworkMode,
		NetworkNamespaces: happyNetworkNamespaces,
	}

	happyTaskResponse = state.TaskResponse{
		TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
		TaskNetworkConfig:     &happyTaskNetworkConfig,
		FaultInjectionEnabled: true,
	}

	happyBlackHolePortReqBody = map[string]interface{}{
		"Port":        port,
		"Protocol":    protocol,
		"TrafficType": trafficType,
	}

	happyNetworkLatencyReqBody = map[string]interface{}{
		"DelayMilliseconds":  delayMilliseconds,
		"JitterMilliseconds": jitterMilliseconds,
		"Sources":            ipSources,
		"SourcesToFilter":    ipSourcesToFilter,
	}

	happyNetworkPacketLossReqBody = map[string]interface{}{
		"LossPercent":     lossPercent,
		"Sources":         ipSources,
		"SourcesToFilter": ipSourcesToFilter,
	}

	ipSources = []string{"52.95.154.1", "52.95.154.2"}

	ipSourcesToFilter = []string{"8.8.8.8"}

	startNetworkBlackHolePortTestPrefix = fmt.Sprintf(startFaultRequestType, types.BlackHolePortFaultType)
	stopNetworkBlackHolePortTestPrefix  = fmt.Sprintf(stopFaultRequestType, types.BlackHolePortFaultType)
	checkNetworkBlackHolePortTestPrefix = fmt.Sprintf(checkStatusFaultRequestType, types.BlackHolePortFaultType)
	startNetworkLatencyTestPrefix       = fmt.Sprintf(startFaultRequestType, types.LatencyFaultType)
	stopNetworkLatencyTestPrefix        = fmt.Sprintf(stopFaultRequestType, types.LatencyFaultType)
	checkNetworkLatencyTestPrefix       = fmt.Sprintf(checkStatusFaultRequestType, types.LatencyFaultType)
	startNetworkPacketLossTestPrefix    = fmt.Sprintf(startFaultRequestType, types.PacketLossFaultType)
	stopNetworkPacketLossTestPrefix     = fmt.Sprintf(stopFaultRequestType, types.PacketLossFaultType)
	checkNetworkPacketLossTestPrefix    = fmt.Sprintf(checkStatusFaultRequestType, types.PacketLossFaultType)

	ctxTimeoutDuration = requestTimeoutSeconds * time.Second
)

type networkFaultInjectionTestCase struct {
	name                      string
	expectedStatusCode        int
	requestBody               interface{}
	expectedResponseBody      types.NetworkFaultInjectionResponse
	setAgentStateExpectations func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient)
	setExecExpectations       func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller)
}

// Tests the path for Fault Network Faults API
func TestFaultBlackholeFaultPath(t *testing.T) {
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-blackhole-port/start", NetworkFaultPath(types.BlackHolePortFaultType, types.StartNetworkFaultPostfix))
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-blackhole-port/stop", NetworkFaultPath(types.BlackHolePortFaultType, types.StopNetworkFaultPostfix))
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-blackhole-port/status", NetworkFaultPath(types.BlackHolePortFaultType, types.CheckNetworkFaultPostfix))
}

func TestFaultLatencyFaultPath(t *testing.T) {
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-latency/start", NetworkFaultPath(types.LatencyFaultType, types.StartNetworkFaultPostfix))
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-latency/stop", NetworkFaultPath(types.LatencyFaultType, types.StopNetworkFaultPostfix))
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-latency/status", NetworkFaultPath(types.LatencyFaultType, types.CheckNetworkFaultPostfix))
}

func TestFaultPacketLossFaultPath(t *testing.T) {
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-packet-loss/start", NetworkFaultPath(types.PacketLossFaultType, types.StartNetworkFaultPostfix))
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-packet-loss/stop", NetworkFaultPath(types.PacketLossFaultType, types.StopNetworkFaultPostfix))
	assert.Equal(t, "/api/{endpointContainerIDMuxName:[^/]*}/fault/v1/network-packet-loss/status", NetworkFaultPath(types.PacketLossFaultType, types.CheckNetworkFaultPostfix))
}

// testNetworkFaultInjectionCommon will be used by unit tests for all 9 fault injection Network Fault APIs.
// Unit tests for all 9 APIs interact with the TMDS server and share similar logic.
// Thus, use a shared base method to reduce duplicated code.
func testNetworkFaultInjectionCommon(t *testing.T,
	tcs []networkFaultInjectionTestCase, tmdsEndpoint string) {
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// Mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			agentState := mock_state.NewMockAgentState(ctrl)
			metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

			router := mux.NewRouter()
			mockExec := mock_execwrapper.NewMockExec(ctrl)
			handler := New(agentState, metricsFactory, mockExec)
			networkConfigClient := netconfig.NewNetworkConfigClient()

			if tc.setAgentStateExpectations != nil {
				tc.setAgentStateExpectations(agentState, networkConfigClient)
			}
			if tc.setExecExpectations != nil {
				tc.setExecExpectations(mockExec, ctrl)
			}

			var handleMethod func(http.ResponseWriter, *http.Request)
			var tmdsAPI string
			switch tmdsEndpoint {
			case NetworkFaultPath(types.BlackHolePortFaultType, types.StartNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-blackhole-port/start"
				handleMethod = handler.StartNetworkBlackholePort()
			case NetworkFaultPath(types.BlackHolePortFaultType, types.StopNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-blackhole-port/stop"
				handleMethod = handler.StopNetworkBlackHolePort()
			case NetworkFaultPath(types.BlackHolePortFaultType, types.CheckNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-blackhole-port/status"
				handleMethod = handler.CheckNetworkBlackHolePort()
			case NetworkFaultPath(types.LatencyFaultType, types.StartNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-latency/start"
				handleMethod = handler.StartNetworkLatency()
			case NetworkFaultPath(types.LatencyFaultType, types.StopNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-latency/stop"
				handleMethod = handler.StopNetworkLatency()
			case NetworkFaultPath(types.LatencyFaultType, types.CheckNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-latency/status"
				handleMethod = handler.CheckNetworkLatency()
			case NetworkFaultPath(types.PacketLossFaultType, types.StartNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-packet-loss/start"
				handleMethod = handler.StartNetworkPacketLoss()
			case NetworkFaultPath(types.PacketLossFaultType, types.StopNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-packet-loss/stop"
				handleMethod = handler.StopNetworkPacketLoss()
			case NetworkFaultPath(types.PacketLossFaultType, types.CheckNetworkFaultPostfix):
				tmdsAPI = "/api/%s/fault/v1/network-packet-loss/status"
				handleMethod = handler.CheckNetworkPacketLoss()
			default:
				t.Error("Unrecognized TMDS Endpoint")
			}

			router.HandleFunc(
				tmdsEndpoint,
				handleMethod,
			).Methods(http.MethodPost)

			var requestBody io.Reader
			if tc.requestBody != nil {
				reqBodyBytes, err := json.Marshal(tc.requestBody)
				require.NoError(t, err)
				requestBody = bytes.NewReader(reqBodyBytes)
			}
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(tmdsAPI, endpointId), requestBody)
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

func generateCommonNetworkBlackHolePortTestCases(name string) []networkFaultInjectionTestCase {
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 fmt.Sprintf("%s no request body", name),
			expectedStatusCode:   400,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required request body is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
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
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
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
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
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
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
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
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
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
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:                 fmt.Sprintf("%s task lookup fail", name),
			expectedStatusCode:   404,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("task lookup failed"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(state.TaskResponse{}, state.NewErrorLookupFailure("task lookup failed")).
					Times(1)
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata fetch fail", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("Unable to generate metadata for v4 task: '%s'", taskARN)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(
					fmt.Sprintf("Unable to generate metadata for v4 task: '%s'", taskARN))).
					Times(1)
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata unknown fail", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(state.TaskResponse{}, errors.New("unknown error")).
					Times(1)
			},
		},
		{
			name:                 fmt.Sprintf("%s fault injection disabled", name),
			expectedStatusCode:   400,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(faultInjectionEnabledError, taskARN)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: false,
				}, nil).Times(1)
			},
		},
		{
			name:                 fmt.Sprintf("%s invalid network mode", name),
			expectedStatusCode:   400,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(invalidNetworkModeError, invalidNetworkMode)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig: &state.TaskNetworkConfig{
						NetworkMode:       invalidNetworkMode,
						NetworkNamespaces: happyNetworkNamespaces,
					},
				}, nil).Times(1)
			},
		},
		{
			name:               fmt.Sprintf("%s empty task network config", name),
			expectedStatusCode: 500,
			requestBody:        happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(
				fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig:     nil,
				}, nil).Times(1)
			},
		},
		{
			name:               fmt.Sprintf("%s no task network namespace", name),
			expectedStatusCode: 500,
			requestBody:        happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(
				fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig: &state.TaskNetworkConfig{
						NetworkMode:       awsvpcNetworkMode,
						NetworkNamespaces: nil,
					},
				}, nil).Times(1)
			},
		},
		{
			name:               fmt.Sprintf("%s no path in task network config", name),
			expectedStatusCode: 500,
			requestBody:        happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(
				fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig: &state.TaskNetworkConfig{
						NetworkMode:       awsvpcNetworkMode,
						NetworkNamespaces: noPathInNetworkNamespaces,
					},
				}, nil).Times(1)
			},
		},
		{
			name:               fmt.Sprintf("%s no device name in task network config", name),
			expectedStatusCode: 500,
			requestBody:        happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(
				fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig: &state.TaskNetworkConfig{
						NetworkMode: awsvpcNetworkMode,
						NetworkNamespaces: []*state.NetworkNamespace{
							{
								Path:              "/path",
								NetworkInterfaces: noDeviceNameInNetworkInterfaces,
							},
						},
					},
				}, nil).Times(1)
			},
		},
		{
			name:                 fmt.Sprintf("%s request timed out", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(requestTimedOutError, name)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), -1*time.Second)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, errors.New("signal: killed")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, false),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata obtain default network interface name fail", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("unable to obtain default network interface name on host"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterfaceName(
					"unable to obtain default network interface name on host")).
					Times(1)
			},
		},
	}
	return tcs
}

func generateStartBlackHolePortFaultTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkBlackHolePortTestCases(startNetworkBlackHolePortTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 fmt.Sprintf("%s success running", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name:               fmt.Sprintf("%s unknown request body", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": trafficType,
				"Unknown":     "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s success already running", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s fail append ACCEPT rule to chain", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("exit status 1")),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s fail append DROP rule to chain", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("exit status 1")),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s fail insert chain to table", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("exit status 1")),
				)
			},
		},
		{
			name:               "SourcesToFilter validation failure",
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":            port,
				"Protocol":        protocol,
				"TrafficType":     trafficType,
				"SourcesToFilter": aws.StringSlice([]string{"1.2.3.4", "bad"}),
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value bad for parameter SourcesToFilter"),
		},
		{
			name: "TMDS IP is added to SourcesToFilter if needed",
			requestBody: map[string]interface{}{
				"Port":        80,
				"Protocol":    protocol,
				"TrafficType": "egress",
			},
			expectedStatusCode:   200,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "egress-tcp-80",
						"-p", "tcp", "-d", "169.254.170.2", "--dport", "80", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "egress-tcp-80",
						"-p", "tcp", "-d", "0.0.0.0/0", "--dport", "80", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name: "Sources to filter are filtered",
			requestBody: map[string]interface{}{
				"Port":            443,
				"Protocol":        "udp",
				"TrafficType":     "ingress",
				"SourcesToFilter": []string{"1.2.3.4/20", "8.8.8.8"},
			},
			expectedStatusCode:   200,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "1.2.3.4/20", "--dport", "443", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "8.8.8.8", "--dport", "443", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "0.0.0.0/0", "--dport", "443", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name:               "Error when filtering a source",
			expectedStatusCode: 500,
			requestBody: map[string]interface{}{
				"Port":            443,
				"Protocol":        "udp",
				"TrafficType":     "ingress",
				"SourcesToFilter": []string{"1.2.3.4/20"},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "1.2.3.4/20", "--dport", "443", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("exit status 1")),
				)
			},
		},
	}

	return append(tcs, commonTcs...)
}

func generateStopBlackHolePortFaultTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkBlackHolePortTestCases(stopNetworkBlackHolePortTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 fmt.Sprintf("%s success running", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name:               fmt.Sprintf("%s unknown request body", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": trafficType,
				"Unknown":     "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s success already stopped", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s fail clear chain", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("exit status 1")),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s fail delete chain from table", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("exit status 1")),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s fail delete chain", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("exit status 1")),
				)
			},
		},
	}
	return append(tcs, commonTcs...)
}

func generateCheckBlackHolePortFaultStatusTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkBlackHolePortTestCases(checkNetworkBlackHolePortTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 fmt.Sprintf("%s success running", checkNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name:               fmt.Sprintf("%s unknown request body", checkNetworkBlackHolePortTestPrefix),
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": trafficType,
				"Unknown":     "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s success not running", checkNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("not-running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s failure", checkNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("exit 2")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, false),
				)
			},
		},
	}

	return append(tcs, commonTcs...)
}

func TestStartNetworkBlackHolePort(t *testing.T) {
	tcs := generateStartBlackHolePortFaultTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.BlackHolePortFaultType, types.StartNetworkFaultPostfix))
}

func TestStopNetworkBlackHolePort(t *testing.T) {
	tcs := generateStopBlackHolePortFaultTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.BlackHolePortFaultType, types.StopNetworkFaultPostfix))
}

func TestCheckNetworkBlackHolePort(t *testing.T) {
	tcs := generateCheckBlackHolePortFaultStatusTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.BlackHolePortFaultType, types.CheckNetworkFaultPostfix))
}

func generateCommonNetworkLatencyTestCases(name string) []networkFaultInjectionTestCase {
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 fmt.Sprintf("%s task lookup fail", name),
			expectedStatusCode:   404,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("task lookup failed"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorLookupFailure("task lookup failed"))
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata fetch fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("Unable to generate metadata for v4 task: '%s'", taskARN)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(
					fmt.Sprintf("Unable to generate metadata for v4 task: '%s'", taskARN)))
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata unknown fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, errors.New("unknown error"))
			},
		},
		{
			name:                 fmt.Sprintf("%s fault injection disabled", name),
			expectedStatusCode:   400,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(faultInjectionEnabledError, taskARN)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: false,
				}, nil)
			},
		},
		{
			name:                 fmt.Sprintf("%s invalid network mode", name),
			expectedStatusCode:   400,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(invalidNetworkModeError, invalidNetworkMode)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig: &state.TaskNetworkConfig{
						NetworkMode:       invalidNetworkMode,
						NetworkNamespaces: happyNetworkNamespaces,
					},
				}, nil)
			},
		},
		{
			name:                 fmt.Sprintf("%s empty task network config", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig:     nil,
				}, nil)
			},
		},
		{
			name:               "failed-to-unmarshal-json",
			expectedStatusCode: 500,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            ipSources,
				"Unknown":            "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte("["), nil)
			},
		},
		{
			name:                 "request timed out",
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(requestTimedOutError, name)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Do(func(_, _ interface{}) {
						// Sleep for ctxTimeoutDuration plus 1 second, to make sure the
						// ctx that we passed to the os/exec execution times out.
						time.Sleep(ctxTimeoutDuration + 1*time.Second)
					}).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata obtain default network interface name fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("unable to obtain default network interface name on host"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterfaceName(
					"unable to obtain default network interface name on host"))
			},
		},
	}
	return tcs
}

func generateStartNetworkLatencyTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkLatencyTestCases(startNetworkLatencyTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "no-existing-fault",
			expectedStatusCode:   200,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(5).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(5).Return([]byte(tcCommandEmptyOutput), nil)
			},
		},
		{
			name:                 "existing-network-latency-fault",
			expectedStatusCode:   409,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("There is already one network latency fault running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLatencyFaultExistsCommandOutput), nil)
			},
		},
		{
			name:                 "existing-network-packet-loss-fault",
			expectedStatusCode:   409,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("There is already one network packet loss fault running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLossFaultExistsCommandOutput), nil)
			},
		},
		{
			name:               "unknown-request-body-no-existing-fault",
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            ipSources,
				"SourcesToFilter":    []string{},
				"Unknown":            "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(4).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(4).Return([]byte(tcCommandEmptyOutput), nil)
			},
		},
		{
			name:                 fmt.Sprintf("%s no request body", startNetworkLatencyTestPrefix),
			expectedStatusCode:   400,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required request body is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s malformed request body 1", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  "incorrect-field",
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            ipSources,
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal string into Go struct field NetworkLatencyRequest.DelayMilliseconds of type uint64"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s malformed request body 2", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            "",
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal string into Go struct field NetworkLatencyRequest.Sources of type []*string"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s malformed request body 3", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            ipSources,
				"SourcesToFilter":    "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal string into Go struct field NetworkLatencyRequest.SourcesToFilter of type []*string"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 1", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            []string{},
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required parameter Sources is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 2", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required parameter Sources is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 3", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds": delayMilliseconds,
				"Sources":           []string{},
				"SourcesToFilter":   []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required parameter JitterMilliseconds is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid DelayMilliseconds in the request body 1", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  -1,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            ipSources,
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal number -1 into Go struct field NetworkLatencyRequest.DelayMilliseconds of type uint64"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid JitterMilliseconds in the request body 2", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": -1,
				"Sources":            ipSources,
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal number -1 into Go struct field NetworkLatencyRequest.JitterMilliseconds of type uint64"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid IP value in the request body 1", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            []string{"10.1.2.3.4"},
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value 10.1.2.3.4 for parameter Sources"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid IP CIDR block value in the request body 2", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            []string{"52.95.154.0/33"},
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value 52.95.154.0/33 for parameter Sources"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid IP CIDR block value in the request body 2", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"Sources":            ipSources,
				"SourcesToFilter":    []string{"52.95.154.0/33"},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value 52.95.154.0/33 for parameter SourcesToFilter"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
	}
	return append(tcs, commonTcs...)
}

func generateStopNetworkLatencyTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkLatencyTestCases(stopNetworkLatencyTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "no-existing-fault-empty-request-payload",
			expectedStatusCode:   200,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
		{
			name:                 "existing-network-latency-fault-happy-request-payload",
			expectedStatusCode:   200,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLatencyFaultExistsCommandOutput), nil),
				)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(2).Return([]byte(""), nil)
			},
		},
		{
			name:                 "existing-network-packet-loss-fault-empty-request-payload",
			expectedStatusCode:   200,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLossFaultExistsCommandOutput), nil)
			},
		},
		{
			name:               "unknown-request-body-no-existing-fault-invalid-request-payload",
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  "",
				"JitterMilliseconds": -1,
				"Unknown":            "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
	}
	return append(tcs, commonTcs...)
}

func generateCheckNetworkLatencyTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkLatencyTestCases(checkNetworkLatencyTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "no-existing-fault-empty-request-payload",
			expectedStatusCode:   200,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("not-running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
		{
			name:                 "existing-network-latency-fault-happy-request-payload",
			expectedStatusCode:   200,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLatencyFaultExistsCommandOutput), nil),
				)
			},
		},
		{
			name:                 "existing-network-packet-loss-fault-empty-request-payload",
			expectedStatusCode:   200,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("not-running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLossFaultExistsCommandOutput), nil),
				)
			},
		},
		{
			name:               "unknown-request-body-no-existing-fault-invalid-request-payload",
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  "",
				"JitterMilliseconds": -1,
				"Unknown":            "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("not-running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
	}
	return append(tcs, commonTcs...)
}

func TestStartNetworkLatency(t *testing.T) {
	tcs := generateStartNetworkLatencyTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.LatencyFaultType, types.StartNetworkFaultPostfix))
}

func TestStopNetworkLatency(t *testing.T) {
	tcs := generateStopNetworkLatencyTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.LatencyFaultType, types.StopNetworkFaultPostfix))
}

func TestCheckNetworkLatency(t *testing.T) {
	tcs := generateCheckNetworkLatencyTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.LatencyFaultType, types.CheckNetworkFaultPostfix))
}

func generateCommonNetworkPacketLossTestCases(name string) []networkFaultInjectionTestCase {
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 fmt.Sprintf("%s task lookup fail", name),
			expectedStatusCode:   404,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("task lookup failed"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorLookupFailure("task lookup failed"))
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata fetch fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("Unable to generate metadata for v4 task: '%s'", taskARN)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(
					fmt.Sprintf("Unable to generate metadata for v4 task: '%s'", taskARN)))
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata unknown fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, errors.New("unknown error"))
			},
		},
		{
			name:                 fmt.Sprintf("%s fault injection disabled", name),
			expectedStatusCode:   400,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(faultInjectionEnabledError, taskARN)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: false,
				}, nil)
			},
		},
		{
			name:                 fmt.Sprintf("%s invalid network mode", name),
			expectedStatusCode:   400,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(invalidNetworkModeError, invalidNetworkMode)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig: &state.TaskNetworkConfig{
						NetworkMode:       invalidNetworkMode,
						NetworkNamespaces: happyNetworkNamespaces,
					},
				}, nil)
			},
		},
		{
			name:                 fmt.Sprintf("%s empty task network config", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig:     nil,
				}, nil)
			},
		},
		{
			name:               "failed-to-unmarshal-json",
			expectedStatusCode: 500,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         ipSources,
				"Unknown":         "",
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte("["), nil)
			},
		},
		{
			name:                 "request timed out",
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(requestTimedOutError, name)),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Do(func(_, _ interface{}) {
						// Sleep for ctxTimeoutDuration plus 1 second, to make sure the
						// ctx that we passed to the os/exec execution times out.
						time.Sleep(ctxTimeoutDuration + 1*time.Second)
					}).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
		{
			name:                 fmt.Sprintf("%s task metadata obtain default network interface name fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("unable to obtain default network interface name on host"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterfaceName(
					"unable to obtain default network interface name on host"))
			},
		},
	}
	return tcs
}

func generateStartNetworkPacketLossTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkPacketLossTestCases(startNetworkPacketLossTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "no-existing-fault",
			expectedStatusCode:   200,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(5).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(5).Return([]byte(tcCommandEmptyOutput), nil)
			},
		},
		{
			name:                 "existing-network-latency-fault",
			expectedStatusCode:   409,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("There is already one network latency fault running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLatencyFaultExistsCommandOutput), nil)
			},
		},
		{
			name:                 "existing-network-packet-loss-fault",
			expectedStatusCode:   409,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("There is already one network packet loss fault running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLossFaultExistsCommandOutput), nil)
			},
		},
		{
			name:               "unknown-request-body-no-existing-fault-no-allowlist-filter",
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
				"Unknown":         "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(4).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(4).Return([]byte(tcCommandEmptyOutput), nil)
			},
		},
		{
			name:                 fmt.Sprintf("%s no request body", startNetworkPacketLossTestPrefix),
			expectedStatusCode:   400,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required request body is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s malformed request body 1", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     "incorrect-field",
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal string into Go struct field NetworkPacketLossRequest.LossPercent of type uint64"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s malformed request body 2", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         "",
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal string into Go struct field NetworkPacketLossRequest.Sources of type []*string"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s malformed request body 3", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         ipSources,
				"SourcesToFilter": "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal string into Go struct field NetworkPacketLossRequest.SourcesToFilter of type []*string"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 1", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         []string{},
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required parameter Sources is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 2", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required parameter Sources is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 3", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("required parameter LossPercent is missing"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid LossPercent in the request body 1", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     -1,
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("json: cannot unmarshal number -1 into Go struct field NetworkPacketLossRequest.LossPercent of type uint64"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid LossPercent in the request body 2", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     101,
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value 101 for parameter LossPercent"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid LossPercent in the request body 3", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     0,
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value 0 for parameter LossPercent"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid IP value in the request body 1", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         []string{"10.1.2.3.4"},
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value 10.1.2.3.4 for parameter Sources"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid IP CIDR block value in the request body 2", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         []string{"52.95.154.0/33"},
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value 52.95.154.0/33 for parameter Sources"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
		{
			name:               fmt.Sprintf("%s invalid IP CIDR block value in the request body 3", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         ipSources,
				"SourcesToFilter": []string{"52.95.154.0/33"},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse("invalid value 52.95.154.0/33 for parameter SourcesToFilter"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
		},
	}
	return append(tcs, commonTcs...)
}

func generateStopNetworkPacketLossTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkPacketLossTestCases(stopNetworkPacketLossTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "no-existing-fault-empty-request-payload",
			expectedStatusCode:   200,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
		{
			name:                 "existing-network-latency-fault-happy-request-payload",
			expectedStatusCode:   200,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLatencyFaultExistsCommandOutput), nil)
			},
		},
		{
			name:                 "existing-network-packet-loss-fault-empty-request-payload",
			expectedStatusCode:   200,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLossFaultExistsCommandOutput), nil),
				)
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(2).Return([]byte(""), nil)
			},
		},
		{
			name:               "unknown-request-body-no-existing-fault-invalid-request-payload",
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"LossPercent": -1,
				"Sources":     "",
				"Unknown":     "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
	}
	return append(tcs, commonTcs...)
}

func generateCheckNetworkPacketLossTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkPacketLossTestCases(checkNetworkPacketLossTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "no-existing-fault-empty-request-payload",
			expectedStatusCode:   200,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("not-running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
		{
			name:                 "existing-network-latency-fault-happy-request-payload",
			expectedStatusCode:   200,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("not-running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLatencyFaultExistsCommandOutput), nil),
				)
			},
		},
		{
			name:                 "existing-network-packet-loss-fault-empty-request-payload",
			expectedStatusCode:   200,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcLossFaultExistsCommandOutput), nil),
				)
			},
		},
		{
			name:               "unknown-request-body-no-existing-fault-invalid-request-payload",
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"LossPercent": -1,
				"Sources":     "",
				"Unknown":     "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("not-running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(happyTaskResponse, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Times(1).Return([]byte(tcCommandEmptyOutput), nil),
				)
			},
		},
	}
	return append(tcs, commonTcs...)
}

func TestStartNetworkPacketLoss(t *testing.T) {
	tcs := generateStartNetworkPacketLossTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.PacketLossFaultType, types.StartNetworkFaultPostfix))
}

func TestStopNetworkPacketLoss(t *testing.T) {
	tcs := generateStopNetworkPacketLossTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.PacketLossFaultType, types.StopNetworkFaultPostfix))
}

func TestCheckNetworkPacketLoss(t *testing.T) {
	tcs := generateCheckNetworkPacketLossTestCases()
	testNetworkFaultInjectionCommon(t, tcs, NetworkFaultPath(types.PacketLossFaultType, types.CheckNetworkFaultPostfix))
}
