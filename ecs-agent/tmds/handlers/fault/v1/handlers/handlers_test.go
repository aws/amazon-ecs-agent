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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	endpointId         = "endpointId"
	port               = 1234
	protocol           = "tcp"
	trafficType        = "ingress"
	delayMilliseconds  = 123456789
	jitterMilliseconds = 4567
	lossPercent        = 6
	taskARN            = "taskArn"
	awsvpcNetworkMode  = "awsvpc"
	hostNetworkMode    = "host"
	deviceName         = "eth0"
	deviceName2        = "eth1"
	ipv4Addr           = "10.0.0.1"
	ipv6Addr           = "2600:1f13:4d9:e602:6aea:cdb1:2b2b:8d62"
	invalidNetworkMode = "invalid"
	nspath             = "/some/path"
	nspathHost         = "host"
	// Fault injection tooling errors output
	iptablesChainNotFoundError        = "iptables: Bad rule (does a matching rule exist in that chain?)."
	tcLatencyFaultExistsCommandOutput = `[{"kind":"netem","handle":"10:","parent":"1:1","options":{"limit":1000,"delay":{"delay":123456789,"jitter":4567,"correlation":0},"ecn":false,"gap":0}}]`
	tcLossFaultExistsCommandOutput    = `[{"kind":"netem","handle":"10:","dev":"eth0","parent":"1:1","options":{"limit":1000,"loss-random":{"loss":0.06,"correlation":0},"ecn":false,"gap":0}}]`
	tcCommandEmptyOutput              = `[]`
	// Common Fault injection JSON responses
	happyFaultRunningResponse    = `{"Status":"running"}`
	happyFaultStoppedResponse    = `{"Status":"stopped"}`
	happyFaultNotRunningResponse = `{"Status":"not-running"}`
	errorResponse                = `{"Error":"%s"}`
	taskLookupFailError          = "task lookup failed"
	missingHostInterfaceError    = "unable to obtain default network interface name on host"
	jsonStringMarshalError       = "json: cannot unmarshal string into Go struct field %s of type %s"
	jsonIntMarshalError          = "json: cannot unmarshal number %s into Go struct field %s of type %s"
	defaultIfaceResolveErr       = "failed to resolve default host network interface"
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

	happyV4OnlyNetworkInterfaces = []*state.NetworkInterface{
		{
			DeviceName:    deviceName,
			IPV4Addresses: []string{ipv4Addr},
		},
	}

	happyV6OnlyNetworkInterfaces = []*state.NetworkInterface{
		{
			DeviceName:    deviceName,
			IPV6Addresses: []string{ipv6Addr},
		},
	}

	happyV4V6NetworkInterfaces = []*state.NetworkInterface{
		{
			DeviceName:    deviceName,
			IPV4Addresses: []string{ipv4Addr},
			IPV6Addresses: []string{ipv6Addr},
		},
	}

	happyTwoNetworkInterfaces = []*state.NetworkInterface{
		{
			DeviceName:    deviceName,
			IPV4Addresses: []string{ipv4Addr},
		},
		{
			DeviceName:    deviceName2,
			IPV6Addresses: []string{ipv6Addr},
		},
	}

	happyNetworkNamespaces = []*state.NetworkNamespace{
		{
			Path:              nspath,
			NetworkInterfaces: happyNetworkInterfaces,
		},
	}

	happyV4OnlyNetworkNamespaces = []*state.NetworkNamespace{
		{
			Path:              nspath,
			NetworkInterfaces: happyV4OnlyNetworkInterfaces,
		},
	}

	happyV6OnlyNetworkNamespaces = []*state.NetworkNamespace{
		{
			Path:              nspath,
			NetworkInterfaces: happyV6OnlyNetworkInterfaces,
		},
	}

	happyV4V6NetworkNamespaces = []*state.NetworkNamespace{
		{
			Path:              nspath,
			NetworkInterfaces: happyV4V6NetworkInterfaces,
		},
	}

	happyNetworkNamespacesTwoInterfaces = []*state.NetworkNamespace{
		{
			Path:              nspathHost,
			NetworkInterfaces: happyTwoNetworkInterfaces,
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

	happyV4OnlyTaskNetworkConfig = state.TaskNetworkConfig{
		NetworkMode:       awsvpcNetworkMode,
		NetworkNamespaces: happyV4OnlyNetworkNamespaces,
	}

	happyV6OnlyTaskNetworkConfig = state.TaskNetworkConfig{
		NetworkMode:       awsvpcNetworkMode,
		NetworkNamespaces: happyV6OnlyNetworkNamespaces,
	}

	happyV4V6TaskNetworkConfig = state.TaskNetworkConfig{
		NetworkMode:       awsvpcNetworkMode,
		NetworkNamespaces: happyV4V6NetworkNamespaces,
	}

	happyTaskNetworkConfigTwoInterfaces = state.TaskNetworkConfig{
		NetworkMode:       hostNetworkMode,
		NetworkNamespaces: happyNetworkNamespacesTwoInterfaces,
	}

	happyTaskResponse = state.TaskResponse{
		TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
		TaskNetworkConfig:     &happyTaskNetworkConfig,
		FaultInjectionEnabled: true,
	}

	happyV4OnlyTaskResponse = state.TaskResponse{
		TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
		TaskNetworkConfig:     &happyV4OnlyTaskNetworkConfig,
		FaultInjectionEnabled: true,
	}

	happyV6OnlyTaskResponse = state.TaskResponse{
		TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
		TaskNetworkConfig:     &happyV6OnlyTaskNetworkConfig,
		FaultInjectionEnabled: true,
	}

	happyV4V6TaskResponse = state.TaskResponse{
		TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
		TaskNetworkConfig:     &happyV4V6TaskNetworkConfig,
		FaultInjectionEnabled: true,
	}

	happyTaskResponseTwoInterfaces = state.TaskResponse{
		TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
		TaskNetworkConfig:     &happyTaskNetworkConfigTwoInterfaces,
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

	ipSources = []string{"52.95.154.1", "52.95.154.2", "1:2:3:4::"}

	ipSourcesToFilter = []string{"8.8.8.8", "5:5:5:5::"}

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

	// Common Task metdata errors
	taskMetdataFetchFailError      = fmt.Sprintf("Unable to generate metadata for v4 task: '%s'", taskARN)
	taskMetdataUnknownFailureError = fmt.Sprintf("failed to get task metadata due to internal server error for container: %s", endpointId)
)

// setStartLatencyFaultExpectations sets up all mock expectations for start network latency fault
func setStartLatencyFaultExpectations(
	exec *mock_execwrapper.MockExec,
	ctrl *gomock.Controller,
	interfaces []string,
	delayMs uint64,
	jitterMs uint64,
	sourcesToFilter []string,
	sources []string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
	mockCMD := mock_execwrapper.NewMockCmd(ctrl)

	// Set up context expectations
	expectations := []*gomock.Call{
		exec.EXPECT().
			NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).
			Return(ctx, cancel),
	}

	// Check existing faults for each interface
	for _, interfaceName := range interfaces {
		expectations = append(expectations,
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", interfaceName, "parent", "1:1"}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
		)
	}

	// Apply latency fault to each interface
	for _, interfaceName := range interfaces {
		expectations = append(expectations,
			// Create root qdisc
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc",
					[]string{
						"qdisc", "add", "dev", interfaceName, "root",
						"handle", "1:", "prio", "priomap",
						"2", "2", "2", "2",
						"2", "2", "2", "2",
						"2", "2", "2", "2",
						"2", "2", "2", "2",
					}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),

			// Add latency qdisc
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc",
					[]string{
						"qdisc", "add", "dev", interfaceName, "parent", "1:1",
						"handle", "10:", "netem", "delay",
						fmt.Sprintf("%dms", delayMs),
						fmt.Sprintf("%dms", jitterMs),
					}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
		)

		// Set up SourcesToFilter expectations
		for _, source := range sourcesToFilter {
			if isIPv4(source) {
				expectations = append(expectations,
					exec.EXPECT().
						CommandContext(gomock.Any(), "tc",
							[]string{
								"filter", "add", "dev", interfaceName,
								"protocol", "all", "parent", "1:0", "prio", "1",
								"u32", "match", "ip", "dst", source, "flowid", "1:3",
							}).
						Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
				)
			} else {
				expectations = append(expectations,
					exec.EXPECT().
						CommandContext(gomock.Any(), "tc",
							[]string{
								"filter", "add", "dev", interfaceName,
								"protocol", "all", "parent", "1:0", "prio", "1",
								"u32", "match", "ip6", "dst", source, "flowid", "1:3",
							}).
						Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
				)
			}
		}

		// Set up Sources expectations
		for _, source := range sources {
			if isIPv4(source) {
				expectations = append(expectations,
					exec.EXPECT().
						CommandContext(gomock.Any(), "tc",
							[]string{
								"filter", "add", "dev", interfaceName,
								"protocol", "all", "parent", "1:0", "prio", "2",
								"u32", "match", "ip", "dst", source, "flowid", "1:1",
							}).
						Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
				)
			} else {
				expectations = append(expectations,
					exec.EXPECT().
						CommandContext(gomock.Any(), "tc",
							[]string{
								"filter", "add", "dev", interfaceName,
								"protocol", "all", "parent", "1:0", "prio", "2",
								"u32", "match", "ip6", "dst", source, "flowid", "1:1",
							}).
						Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
				)
			}
		}
	}
	gomock.InOrder(expectations...)
}

// setStartPacketLossFaultExpectations sets up mock expectations for applying network packet loss fault to network interfaces
func setStartPacketLossFaultExpectations(
	exec *mock_execwrapper.MockExec,
	ctrl *gomock.Controller,
	interfaces []string,
	lossPercent uint64,
	sourcesToFilter []string,
	sources []string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
	mockCMD := mock_execwrapper.NewMockCmd(ctrl)

	// Set up context expectations
	expectations := []*gomock.Call{
		exec.EXPECT().
			NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).
			Return(ctx, cancel),
	}

	// Check existing faults for each interface
	for _, iface := range interfaces {
		expectations = append(expectations,
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", iface, "parent", "1:1"}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
		)
	}

	// Apply packet loss fault to each interface
	for _, iface := range interfaces {
		expectations = append(expectations,
			// Create root qdisc
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc",
					[]string{
						"qdisc", "add", "dev", iface, "root",
						"handle", "1:", "prio", "priomap",
						"2", "2", "2", "2",
						"2", "2", "2", "2",
						"2", "2", "2", "2",
						"2", "2", "2", "2",
					}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),

			// Add packet loss qdisc
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc",
					[]string{
						"qdisc", "add", "dev", iface, "parent", "1:1",
						"handle", "10:", "netem", "loss",
						fmt.Sprintf("%d%%", lossPercent),
					}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
		)

		// Set up SourcesToFilter expectations
		for _, source := range sourcesToFilter {
			if isIPv4(source) {
				expectations = append(expectations,
					exec.EXPECT().
						CommandContext(gomock.Any(), "tc",
							[]string{
								"filter", "add", "dev", iface,
								"protocol", "all", "parent", "1:0", "prio", "1",
								"u32", "match", "ip", "dst", source, "flowid", "1:3",
							}).
						Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
				)
			} else {
				expectations = append(expectations,
					exec.EXPECT().
						CommandContext(gomock.Any(), "tc",
							[]string{
								"filter", "add", "dev", iface,
								"protocol", "all", "parent", "1:0", "prio", "1",
								"u32", "match", "ip6", "dst", source, "flowid", "1:3",
							}).
						Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
				)
			}
		}

		// Set up Sources expectations
		for _, source := range sources {
			if isIPv4(source) {
				expectations = append(expectations,
					exec.EXPECT().
						CommandContext(gomock.Any(), "tc",
							[]string{
								"filter", "add", "dev", iface,
								"protocol", "all", "parent", "1:0", "prio", "2",
								"u32", "match", "ip", "dst", source, "flowid", "1:1",
							}).
						Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
				)
			} else {
				expectations = append(expectations,
					exec.EXPECT().
						CommandContext(gomock.Any(), "tc",
							[]string{
								"filter", "add", "dev", iface,
								"protocol", "all", "parent", "1:0", "prio", "2",
								"u32", "match", "ip6", "dst", source, "flowid", "1:1",
							}).
						Return(mockCMD),
					mockCMD.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil),
				)
			}
		}
	}

	gomock.InOrder(expectations...)
}

// setStopTCFaultExpectations sets up all mock expectations for stopping
// tc based faults - latency and packet loss
func setStopTCFaultExpectations(
	exec *mock_execwrapper.MockExec,
	ctrl *gomock.Controller,
	interfaces []string,
	lossExistsCommandOutput string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
	mockCMD := mock_execwrapper.NewMockCmd(ctrl)

	// Set up context expectations
	exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel)

	// Check existing fault on each interface
	expectations := []*gomock.Call{}
	for _, iface := range interfaces {
		expectations = append(expectations,
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", iface, "parent", "1:1"}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(lossExistsCommandOutput), nil),
		)
	}
	for _, iface := range interfaces {
		expectations = append(expectations,
			// Delete netem qdisc
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc", []string{"qdisc", "del", "dev", iface, "parent", "1:1", "handle", "10:"}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(""), nil),

			// Delete root qdisc
			exec.EXPECT().
				CommandContext(gomock.Any(), "tc", []string{"qdisc", "del", "dev", iface, "root", "handle", "1:", "prio"}).
				Return(mockCMD),
			mockCMD.EXPECT().CombinedOutput().Return([]byte(""), nil),
		)
	}
	gomock.InOrder(expectations...)
}

// Helper function to check if an IP address is IPv4
func isIPv4(ip string) bool {
	// Simple check - if it contains ':', it's IPv6
	for i := 0; i < len(ip); i++ {
		if ip[i] == ':' {
			return false
		}
	}
	return true
}

type networkFaultInjectionTestCase struct {
	name                      string
	expectedStatusCode        int
	requestBody               interface{}
	expectedResponseBody      types.NetworkFaultInjectionResponse
	setAgentStateExpectations func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient)
	setExecExpectations       func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller)
	expectedResponseJSON      string
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

			// Assert that the response JSON string is as expected.
			assert.Equal(t, tc.expectedResponseJSON, recorder.Body.String())

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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(types.MissingRequestBodyError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, types.MissingRequestBodyError),
		},
		{
			name:               fmt.Sprintf("%s malformed request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":        "incorrect typing",
				"Protocol":    protocol,
				"TrafficType": trafficType,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonStringMarshalError, "NetworkBlackholePortRequest.Port", "uint16")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonStringMarshalError, "NetworkBlackholePortRequest.Port", "uint16")),
		},
		{
			name:               fmt.Sprintf("%s incomplete request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":     port,
				"Protocol": protocol,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.MissingRequiredFieldError, "TrafficType")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.MissingRequiredFieldError, "TrafficType")),
		},
		{
			name:               fmt.Sprintf("%s empty value request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.MissingRequiredFieldError, "TrafficType")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.MissingRequiredFieldError, "TrafficType")),
		},
		{
			name:               fmt.Sprintf("%s invalid protocol value request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    "invalid",
				"TrafficType": trafficType,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "invalid", "Protocol")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "invalid", "Protocol")),
		},
		{
			name:               fmt.Sprintf("%s invalid traffic type value request body", name),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": "invalid",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "invalid", "TrafficType")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "invalid", "TrafficType")),
		},
		{
			name:                 fmt.Sprintf("%s task lookup fail", name),
			expectedStatusCode:   404,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskLookupFailError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(state.TaskResponse{}, state.NewErrorLookupFailure(taskLookupFailError)).
					Times(1)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskLookupFailError),
		},
		{
			name:                 fmt.Sprintf("%s task metadata fetch fail", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskMetdataFetchFailError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(
					taskMetdataFetchFailError)).
					Times(1)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataFetchFailError),
		},
		{
			name:                 fmt.Sprintf("%s task metadata unknown fail", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskMetdataUnknownFailureError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(state.TaskResponse{}, errors.New("unknown error")).
					Times(1)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(faultInjectionEnabledError, taskARN)),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(invalidNetworkModeError, invalidNetworkMode)),
		},
		{
			name:               fmt.Sprintf("%s empty task network config", name),
			expectedStatusCode: 500,
			requestBody:        happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(
				taskMetdataUnknownFailureError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig:     nil,
				}, nil).Times(1)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
		},
		{
			name:               fmt.Sprintf("%s no task network namespace", name),
			expectedStatusCode: 500,
			requestBody:        happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(
				taskMetdataUnknownFailureError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
		},
		{
			name:               fmt.Sprintf("%s no path in task network config", name),
			expectedStatusCode: 500,
			requestBody:        happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(
				taskMetdataUnknownFailureError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
		},
		{
			name:               fmt.Sprintf("%s no device name in task network config", name),
			expectedStatusCode: 500,
			requestBody:        happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(
				taskMetdataUnknownFailureError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
		},
		{
			name:                 "Agent failed to resolve default host network interface",
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(defaultIfaceResolveErr),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterface(nil)).
					Times(1)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, defaultIfaceResolveErr),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(requestTimedOutError, name)),
		},
		{
			name:                 fmt.Sprintf("%s task metadata obtain default network interface name fail", name),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(missingHostInterfaceError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterfaceName(
					missingHostInterfaceError)).
					Times(1)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, missingHostInterfaceError),
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
					// No existing chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					// Create the chain in IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Create the chain in IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 fmt.Sprintf("%s success running v4 only task", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyV4OnlyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					// No existing chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					// Create the chain in IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Fail the chain creation in IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, errors.New("fail the ipv6 table update")),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 fmt.Sprintf("%s success running dual stack task", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyV4V6TaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					// No existing chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					// Create the chain in IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Fail the chain creation in IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, errors.New("fail the ipv6 table update")),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 fmt.Sprintf("%s success running ipv6 only task", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   200,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyV6OnlyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					// No existing chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					// Create the chain in IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Create the chain in IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 fmt.Sprintf("%s fail with ipv6 table update", startNetworkBlackHolePortTestPrefix),
			expectedStatusCode:   500,
			requestBody:          happyBlackHolePortReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyV6OnlyTaskResponse, nil).
					Times(1)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				cmdExec := mock_execwrapper.NewMockCmd(ctrl)
				gomock.InOrder(
					exec.EXPECT().NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).Return(ctx, cancel),
					// No existing chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					// Create the chain in IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Create the chain in IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("fail the ipv6 table update")),
				)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
					// No existing chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					// Create the chain in IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Create the chain in IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
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
			expectedResponseJSON: happyFaultRunningResponse,
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
		},
		{
			name:               "SourcesToFilter validation failure with invalid IP",
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":            port,
				"Protocol":        protocol,
				"TrafficType":     trafficType,
				"SourcesToFilter": aws.StringSlice([]string{"1.2.3.4", "bad"}),
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "bad", "SourcesToFilter")),
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "bad", "SourcesToFilter")),
		},
		{
			name:               "SourcesToFilter validation failure with invalid IPv4",
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":            port,
				"Protocol":        protocol,
				"TrafficType":     trafficType,
				"SourcesToFilter": aws.StringSlice([]string{"1.2.3.4", "1.2.333.3"}),
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "1.2.333.3", "SourcesToFilter")),
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "1.2.333.3", "SourcesToFilter")),
		},
		{
			name:               "SourcesToFilter validation failure with invalid IPv6",
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Port":            port,
				"Protocol":        protocol,
				"TrafficType":     trafficType,
				"SourcesToFilter": aws.StringSlice([]string{"1.2.3.4", "2001::db8::1"}),
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "2001::db8::1", "SourcesToFilter")),
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "2001::db8::1", "SourcesToFilter")),
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
					// No existing chain
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-C", "egress-tcp-80",
						"-p", "tcp", "--dport", "80", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					// Create the chain in IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-N", "egress-tcp-80",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to protect TMDS
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "egress-tcp-80",
						"-p", "tcp", "-d", "169.254.170.2", "--dport", "80", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "egress-tcp-80",
						"-p", "tcp", "-d", "0.0.0.0/0", "--dport", "80", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-I", "OUTPUT", "-j", "egress-tcp-80",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Create the chain in IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-N", "egress-tcp-80",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-A", "egress-tcp-80",
						"-p", "tcp", "-d", "::/0", "--dport", "80", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-I", "OUTPUT", "-j", "egress-tcp-80",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name: "Sources to filter are filtered",
			requestBody: map[string]interface{}{
				"Port":        443,
				"Protocol":    "udp",
				"TrafficType": "ingress",
				"SourcesToFilter": []string{
					"1.2.3.4/20",
					"8.8.8.8",
					"2600:1f13:4d9:e611::/64",
					"2600:1f13:4d9:e611:9009:ac97:1ab4:17d9",
				},
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
					// No existing chain
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-C", "ingress-udp-443",
						"-p", "udp", "--dport", "443", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(iptablesChainNotFoundError), errors.New("exit status 1")),
					exec.EXPECT().ConvertToExitError(gomock.Any()).Times(1).Return(nil, true),
					exec.EXPECT().GetExitCode(gomock.Any()).Times(1).Return(1),
					// Create the chain in IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-N", "ingress-udp-443",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to allow packets
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "1.2.3.4/20", "--dport", "443", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to allow packets
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "8.8.8.8", "--dport", "443", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "0.0.0.0/0", "--dport", "443", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-I", "INPUT", "-j", "ingress-udp-443",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Create the chain in IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-N", "ingress-udp-443",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to allow packets
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "2600:1f13:4d9:e611::/64", "--dport", "443", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to allow packets
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "2600:1f13:4d9:e611:9009:ac97:1ab4:17d9", "--dport", "443", "-j", "ACCEPT",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the rule to drop packets
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-A", "ingress-udp-443",
						"-p", "udp", "-d", "::/0", "--dport", "443", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Insert the chain
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-I", "INPUT", "-j", "ingress-udp-443",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
					// Found existing chain
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-C", "ingress-tcp-1234",
						"-p", "tcp", "--dport", "1234", "-j", "DROP",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Clear rules in the chain of IPv4
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-F", "ingress-tcp-1234",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Remove the chain from the IPv4 table
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-D", "INPUT", "-j", "ingress-tcp-1234",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Delete the chain of IPv4
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "iptables", "-w", "5", "-X", "ingress-tcp-1234",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Clear rules in the chain of IPv6
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-F", "ingress-tcp-1234",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Remove the chain from the IPv6 table
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-D", "INPUT", "-j", "ingress-tcp-1234",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					// Delete the chain of IPv6
					exec.EXPECT().CommandContext(gomock.Any(),
						"nsenter", "--net=/some/path", "ip6tables", "-w", "5", "-X", "ingress-tcp-1234",
					).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultStoppedResponse,
		},
		{
			name:               fmt.Sprintf("%s success running v4 only task", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": trafficType,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyV4OnlyTaskResponse, nil).
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
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, errors.New("fail the ipv6 table update")),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultStoppedResponse,
		},
		{
			name:               fmt.Sprintf("%s success running v4v6 only task", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": trafficType,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyV4V6TaskResponse, nil).
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
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, errors.New("fail the ipv6 table update")),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultStoppedResponse,
		},
		{
			name:               fmt.Sprintf("%s success running v6 only task", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode: 200,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": trafficType,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyV6OnlyTaskResponse, nil).
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
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultStoppedResponse,
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
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte{}, nil),
				)
			},
			expectedResponseJSON: happyFaultStoppedResponse,
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
			expectedResponseJSON: happyFaultStoppedResponse,
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
		},
		{
			name:               fmt.Sprintf("%s fail with ipv6 table update", stopNetworkBlackHolePortTestPrefix),
			expectedStatusCode: 500,
			requestBody: map[string]interface{}{
				"Port":        port,
				"Protocol":    protocol,
				"TrafficType": trafficType,
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(internalError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyV6OnlyTaskResponse, nil).
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
					exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cmdExec),
					cmdExec.EXPECT().CombinedOutput().Times(1).Return([]byte(internalError), errors.New("fail the ipv6 table update")),
				)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
			expectedResponseJSON: happyFaultRunningResponse,
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
			expectedResponseJSON: happyFaultRunningResponse,
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
			expectedResponseJSON: happyFaultNotRunningResponse,
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskLookupFailError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorLookupFailure(taskLookupFailError))
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskLookupFailError),
		},
		{
			name:                 fmt.Sprintf("%s task metadata fetch fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskMetdataFetchFailError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(
					taskMetdataFetchFailError))
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataFetchFailError),
		},
		{
			name:                 fmt.Sprintf("%s task metadata unknown fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskMetdataUnknownFailureError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, errors.New("unknown error"))
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(faultInjectionEnabledError, taskARN)),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(invalidNetworkModeError, invalidNetworkMode)),
		},
		{
			name:                 fmt.Sprintf("%s empty task network config", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskMetdataUnknownFailureError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig:     nil,
				}, nil)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
		},
		{
			name:                 "Agent failed to resolve host default network interface",
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(defaultIfaceResolveErr),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterface(nil)).
					Times(1)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, defaultIfaceResolveErr),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(requestTimedOutError, name)),
		},
		{
			name:                 fmt.Sprintf("%s task metadata obtain default network interface name fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(missingHostInterfaceError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterfaceName(
					missingHostInterfaceError))
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, missingHostInterfaceError),
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
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(7).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(7).Return([]byte(tcCommandEmptyOutput), nil)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 "no-existing-fault - two interfaces",
			expectedStatusCode:   200,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().
					GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponseTwoInterfaces, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				setStartLatencyFaultExpectations(
					exec,
					ctrl,
					[]string{"eth0", "eth1"},
					delayMilliseconds,
					jitterMilliseconds,
					ipSourcesToFilter,
					ipSources,
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 "existing-network-latency-fault",
			expectedStatusCode:   409,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(latencyFaultAlreadyRunningError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, latencyFaultAlreadyRunningError),
		},
		{
			name:                 "existing-network-packet-loss-fault",
			expectedStatusCode:   409,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(packetLossFaultAlreadyRunningError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, packetLossFaultAlreadyRunningError),
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
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(5).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(5).Return([]byte(tcCommandEmptyOutput), nil)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 fmt.Sprintf("%s no request body", startNetworkLatencyTestPrefix),
			expectedStatusCode:   400,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(types.MissingRequestBodyError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, types.MissingRequestBodyError),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonStringMarshalError, "NetworkLatencyRequest.DelayMilliseconds", "uint64")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonStringMarshalError, "NetworkLatencyRequest.DelayMilliseconds", "uint64")),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonStringMarshalError, "NetworkLatencyRequest.Sources", "[]*string")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonStringMarshalError, "NetworkLatencyRequest.Sources", "[]*string")),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonStringMarshalError, "NetworkLatencyRequest.SourcesToFilter", "[]*string")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonStringMarshalError, "NetworkLatencyRequest.SourcesToFilter", "[]*string")),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.MissingRequiredFieldError, "Sources")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.MissingRequiredFieldError, "Sources")),
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 2", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds":  delayMilliseconds,
				"JitterMilliseconds": jitterMilliseconds,
				"SourcesToFilter":    []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.MissingRequiredFieldError, "Sources")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.MissingRequiredFieldError, "Sources")),
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 3", startNetworkLatencyTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"DelayMilliseconds": delayMilliseconds,
				"Sources":           []string{},
				"SourcesToFilter":   []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.MissingRequiredFieldError, "JitterMilliseconds")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.MissingRequiredFieldError, "JitterMilliseconds")),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonIntMarshalError, "-1", "NetworkLatencyRequest.DelayMilliseconds", "uint64")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonIntMarshalError, "-1", "NetworkLatencyRequest.DelayMilliseconds", "uint64")),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonIntMarshalError, "-1", "NetworkLatencyRequest.JitterMilliseconds", "uint64")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonIntMarshalError, "-1", "NetworkLatencyRequest.JitterMilliseconds", "uint64")),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "10.1.2.3.4", "Sources")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "10.1.2.3.4", "Sources")),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "52.95.154.0/33", "Sources")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "52.95.154.0/33", "Sources")),
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "52.95.154.0/33", "SourcesToFilter")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "52.95.154.0/33", "SourcesToFilter")),
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
			expectedResponseJSON: happyFaultStoppedResponse,
		},
		{
			name:                 "existing-network-latency-fault-empty-request-payload-two-interfaces",
			expectedStatusCode:   200,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().
					GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponseTwoInterfaces, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				setStopTCFaultExpectations(exec, ctrl, []string{"eth0", "eth1"}, tcLatencyFaultExistsCommandOutput)
			},
			expectedResponseJSON: happyFaultStoppedResponse,
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
			expectedResponseJSON: happyFaultStoppedResponse,
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
			expectedResponseJSON: happyFaultStoppedResponse,
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
			expectedResponseJSON: happyFaultStoppedResponse,
		},
	}
	return append(tcs, commonTcs...)
}

func generateCheckNetworkLatencyTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkLatencyTestCases(checkNetworkLatencyTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "existing-network-latency-fault-two-interfaces",
			expectedStatusCode:   200,
			requestBody:          happyNetworkLatencyReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().
					GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponseTwoInterfaces, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				expectations := []*gomock.Call{
					exec.EXPECT().
						NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).
						Return(ctx, cancel),
				}

				// Check existing fault on each interface
				for _, iface := range []string{"eth0", "eth1"} {
					expectations = append(expectations,
						exec.EXPECT().
							CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", iface, "parent", "1:1"}).
							Return(mockCMD),
						mockCMD.EXPECT().CombinedOutput().Return([]byte(tcLatencyFaultExistsCommandOutput), nil),
					)
				}
				gomock.InOrder(expectations...)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
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
			expectedResponseJSON: happyFaultNotRunningResponse,
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
			expectedResponseJSON: happyFaultRunningResponse,
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
			expectedResponseJSON: happyFaultNotRunningResponse,
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
			expectedResponseJSON: happyFaultNotRunningResponse,
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
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskLookupFailError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorLookupFailure("task lookup failed"))
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskLookupFailError),
		},
		{
			name:                 fmt.Sprintf("%s task metadata fetch fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskMetdataFetchFailError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorMetadataFetchFailure(
					taskMetdataFetchFailError))
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataFetchFailError),
		},
		{
			name:                 fmt.Sprintf("%s task metadata unknown fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskMetdataUnknownFailureError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, errors.New("unknown error"))
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(faultInjectionEnabledError, taskARN)),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(invalidNetworkModeError, invalidNetworkMode)),
		},
		{
			name:                 fmt.Sprintf("%s empty task network config", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(taskMetdataUnknownFailureError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{
					TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
					FaultInjectionEnabled: true,
					TaskNetworkConfig:     nil,
				}, nil)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, taskMetdataUnknownFailureError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, internalError),
		},
		{
			name:                 "Agent failed to resolve host default network interface",
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(defaultIfaceResolveErr),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterface(nil)).
					Times(1)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, defaultIfaceResolveErr),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(requestTimedOutError, name)),
		},
		{
			name:                 fmt.Sprintf("%s task metadata obtain default network interface name fail", name),
			expectedStatusCode:   500,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(missingHostInterfaceError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, state.NewErrorDefaultNetworkInterfaceName(
					missingHostInterfaceError))
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, missingHostInterfaceError),
		},
	}
	return tcs
}

func generateStartNetworkPacketLossTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkPacketLossTestCases(startNetworkPacketLossTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "no-existing-fault - two interfaces",
			expectedStatusCode:   200,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().
					GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponseTwoInterfaces, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				setStartPacketLossFaultExpectations(
					exec,
					ctrl,
					[]string{"eth0", "eth1"},
					lossPercent,
					ipSourcesToFilter,
					ipSources,
				)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
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
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(7).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(7).Return([]byte(tcCommandEmptyOutput), nil)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 "existing-network-latency-fault",
			expectedStatusCode:   409,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(latencyFaultAlreadyRunningError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, latencyFaultAlreadyRunningError),
		},
		{
			name:                 "existing-network-packet-loss-fault",
			expectedStatusCode:   409,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(packetLossFaultAlreadyRunningError),
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
			expectedResponseJSON: fmt.Sprintf(errorResponse, packetLossFaultAlreadyRunningError),
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
				exec.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Times(5).Return(mockCMD)
				mockCMD.EXPECT().CombinedOutput().Times(5).Return([]byte(tcCommandEmptyOutput), nil)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
		{
			name:                 fmt.Sprintf("%s no request body", startNetworkPacketLossTestPrefix),
			expectedStatusCode:   400,
			requestBody:          nil,
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(types.MissingRequestBodyError),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, types.MissingRequestBodyError),
		},
		{
			name:               fmt.Sprintf("%s malformed request body 1", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     "incorrect-field",
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonStringMarshalError, "NetworkPacketLossRequest.LossPercent", "uint64")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonStringMarshalError, "NetworkPacketLossRequest.LossPercent", "uint64")),
		},
		{
			name:               fmt.Sprintf("%s malformed request body 2", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         "",
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonStringMarshalError, "NetworkPacketLossRequest.Sources", "[]*string")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonStringMarshalError, "NetworkPacketLossRequest.Sources", "[]*string")),
		},
		{
			name:               fmt.Sprintf("%s malformed request body 3", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         ipSources,
				"SourcesToFilter": "",
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonStringMarshalError, "NetworkPacketLossRequest.SourcesToFilter", "[]*string")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonStringMarshalError, "NetworkPacketLossRequest.SourcesToFilter", "[]*string")),
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 1", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         []string{},
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.MissingRequiredFieldError, "Sources")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.MissingRequiredFieldError, "Sources")),
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 2", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.MissingRequiredFieldError, "Sources")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.MissingRequiredFieldError, "Sources")),
		},
		{
			name:               fmt.Sprintf("%s incomplete request body 3", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.MissingRequiredFieldError, "LossPercent")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.MissingRequiredFieldError, "LossPercent")),
		},
		{
			name:               fmt.Sprintf("%s invalid LossPercent in the request body 1", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     -1,
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(jsonIntMarshalError, "-1", "NetworkPacketLossRequest.LossPercent", "uint64")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(jsonIntMarshalError, "-1", "NetworkPacketLossRequest.LossPercent", "uint64")),
		},
		{
			name:               fmt.Sprintf("%s invalid LossPercent in the request body 2", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     101,
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "101", "LossPercent")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "101", "LossPercent")),
		},
		{
			name:               fmt.Sprintf("%s invalid LossPercent in the request body 3", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     0,
				"Sources":         ipSources,
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "0", "LossPercent")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "0", "LossPercent")),
		},
		{
			name:               fmt.Sprintf("%s invalid IP value in the request body 1", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         []string{"10.1.2.3.4"},
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "10.1.2.3.4", "Sources")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "10.1.2.3.4", "Sources")),
		},
		{
			name:               fmt.Sprintf("%s invalid IP CIDR block value in the request body 2", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         []string{"52.95.154.0/33"},
				"SourcesToFilter": []string{},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "52.95.154.0/33", "Sources")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "52.95.154.0/33", "Sources")),
		},
		{
			name:               fmt.Sprintf("%s invalid IP CIDR block value in the request body 3", startNetworkPacketLossTestPrefix),
			expectedStatusCode: 400,
			requestBody: map[string]interface{}{
				"LossPercent":     lossPercent,
				"Sources":         ipSources,
				"SourcesToFilter": []string{"52.95.154.0/33"},
			},
			expectedResponseBody: types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(types.InvalidValueError, "52.95.154.0/33", "SourcesToFilter")),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).Return(state.TaskResponse{}, nil).Times(0)
			},
			expectedResponseJSON: fmt.Sprintf(errorResponse, fmt.Sprintf(types.InvalidValueError, "52.95.154.0/33", "SourcesToFilter")),
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
			expectedResponseJSON: happyFaultStoppedResponse,
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
			expectedResponseJSON: happyFaultStoppedResponse,
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
			expectedResponseJSON: happyFaultStoppedResponse,
		},
		{
			name:                 "existing-network-packet-loss-fault-empty-request-payload-two-interfaces",
			expectedStatusCode:   200,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("stopped"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().
					GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponseTwoInterfaces, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				setStopTCFaultExpectations(exec, ctrl, []string{"eth0", "eth1"}, tcLossFaultExistsCommandOutput)
			},
			expectedResponseJSON: happyFaultStoppedResponse,
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
			expectedResponseJSON: happyFaultStoppedResponse,
		},
	}
	return append(tcs, commonTcs...)
}

func generateCheckNetworkPacketLossTestCases() []networkFaultInjectionTestCase {
	commonTcs := generateCommonNetworkPacketLossTestCases(checkNetworkPacketLossTestPrefix)
	tcs := []networkFaultInjectionTestCase{
		{
			name:                 "existing-network-packet-loss-fault-two-interfaces",
			expectedStatusCode:   200,
			requestBody:          happyNetworkPacketLossReqBody,
			expectedResponseBody: types.NewNetworkFaultInjectionSuccessResponse("running"),
			setAgentStateExpectations: func(agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
				agentState.EXPECT().
					GetTaskMetadataWithTaskNetworkConfig(endpointId, netConfigClient).
					Return(happyTaskResponseTwoInterfaces, nil)
			},
			setExecExpectations: func(exec *mock_execwrapper.MockExec, ctrl *gomock.Controller) {
				ctx, cancel := context.WithTimeout(context.Background(), ctxTimeoutDuration)
				mockCMD := mock_execwrapper.NewMockCmd(ctrl)
				expectations := []*gomock.Call{
					exec.EXPECT().
						NewExecContextWithTimeout(gomock.Any(), gomock.Any()).Times(1).
						Return(ctx, cancel),
				}

				// Check existing fault on each interface
				for _, iface := range []string{"eth0", "eth1"} {
					expectations = append(expectations,
						exec.EXPECT().
							CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", iface, "parent", "1:1"}).
							Return(mockCMD),
						mockCMD.EXPECT().CombinedOutput().Return([]byte(tcLossFaultExistsCommandOutput), nil),
					)
				}
				gomock.InOrder(expectations...)
			},
			expectedResponseJSON: happyFaultRunningResponse,
		},
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
			expectedResponseJSON: happyFaultNotRunningResponse,
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
			expectedResponseJSON: happyFaultNotRunningResponse,
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
			expectedResponseJSON: happyFaultRunningResponse,
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
			expectedResponseJSON: happyFaultNotRunningResponse,
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

// TestIsIPv6OnlyTask does focused testing for isIPv6OnlyTask function.
func TestIsIPv6OnlyTask(t *testing.T) {
	tests := []struct {
		name     string
		task     *state.TaskResponse
		expected bool
	}{
		{
			name:     "IPv6-only single interface",
			task:     &happyV6OnlyTaskResponse,
			expected: true,
		},
		{
			name:     "IPv4-only single interface",
			task:     &happyV4OnlyTaskResponse,
			expected: false,
		},
		{
			name:     "Dual-stack single interface",
			task:     &happyV4V6TaskResponse,
			expected: false,
		},
		{
			name:     "Multiple interfaces",
			task:     &happyTaskResponseTwoInterfaces,
			expected: false,
		},
		{
			name: "Single interface with no addresses",
			task: &state.TaskResponse{
				TaskNetworkConfig: &state.TaskNetworkConfig{
					NetworkMode: awsvpcNetworkMode,
					NetworkNamespaces: []*state.NetworkNamespace{
						{
							Path: nspath,
							NetworkInterfaces: []*state.NetworkInterface{
								{
									DeviceName: deviceName,
								},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isIPv6OnlyTask(tc.task)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCheckTCFault does focused testing for CheckTCFault function.
func TestCheckTCFault(t *testing.T) {
	createTestTaskMetadata := func(deviceNames []string) *state.TaskResponse {
		interfaces := make([]*state.NetworkInterface, len(deviceNames))
		for i, name := range deviceNames {
			interfaces[i] = &state.NetworkInterface{
				DeviceName: name,
			}
		}

		return &state.TaskResponse{
			TaskResponse: &v2.TaskResponse{TaskARN: "test-task"},
			TaskNetworkConfig: &state.TaskNetworkConfig{
				NetworkMode: "host",
				NetworkNamespaces: []*state.NetworkNamespace{
					{
						Path:              "host",
						NetworkInterfaces: interfaces,
					},
				},
			},
		}
	}

	tests := []struct {
		name                string
		deviceNames         []string
		expectedLatency     bool
		expectedPacketLoss  bool
		expectedError       error
		commandExpectations func(*mock_execwrapper.MockExec, *mock_execwrapper.MockCmd)
	}{
		{
			name:               "No faults exist",
			deviceNames:        []string{"eth0"},
			expectedLatency:    false,
			expectedPacketLoss: false,
			expectedError:      nil,
			commandExpectations: func(exec *mock_execwrapper.MockExec, cmd *mock_execwrapper.MockCmd) {
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth0", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil)
			},
		},
		{
			name:               "Latency fault exists",
			deviceNames:        []string{"eth0"},
			expectedLatency:    true,
			expectedPacketLoss: false,
			expectedError:      nil,
			commandExpectations: func(exec *mock_execwrapper.MockExec, cmd *mock_execwrapper.MockCmd) {
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth0", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcLatencyFaultExistsCommandOutput), nil)
			},
		},
		{
			name:               "Packet loss fault exists",
			deviceNames:        []string{"eth0"},
			expectedLatency:    false,
			expectedPacketLoss: true,
			expectedError:      nil,
			commandExpectations: func(exec *mock_execwrapper.MockExec, cmd *mock_execwrapper.MockCmd) {
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth0", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcLossFaultExistsCommandOutput), nil)
			},
		},
		{
			name:               "Command execution error",
			deviceNames:        []string{"eth0"},
			expectedLatency:    false,
			expectedPacketLoss: false,
			expectedError:      errors.New("failed to check existing network fault: 'tc -j q show dev eth0 parent 1:1' command failed with the following error: 'command failed'. std output: ''. TaskArn: test-task"),
			commandExpectations: func(exec *mock_execwrapper.MockExec, cmd *mock_execwrapper.MockCmd) {
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth0", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(""), errors.New("command failed"))
			},
		},
		{
			name:               "Invalid JSON output",
			deviceNames:        []string{"eth0"},
			expectedLatency:    false,
			expectedPacketLoss: false,
			expectedError:      errors.New("failed to check existing network fault: failed to unmarshal tc command output: invalid character 'i' looking for beginning of value. TaskArn: test-task"),
			commandExpectations: func(exec *mock_execwrapper.MockExec, cmd *mock_execwrapper.MockCmd) {
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth0", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte("invalid json"), nil)
			},
		},
		{
			name:               "Multiple network interfaces - no faults",
			deviceNames:        []string{"eth0", "eth1"},
			expectedLatency:    false,
			expectedPacketLoss: false,
			expectedError:      nil,
			commandExpectations: func(exec *mock_execwrapper.MockExec, cmd *mock_execwrapper.MockCmd) {
				// First interface check
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth0", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil)

				// Second interface check
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth1", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil)
			},
		},
		{
			name:               "Multiple network interfaces - latency fault on second interface",
			deviceNames:        []string{"eth0", "eth1"},
			expectedLatency:    true,
			expectedPacketLoss: false,
			expectedError:      nil,
			commandExpectations: func(exec *mock_execwrapper.MockExec, cmd *mock_execwrapper.MockCmd) {
				// First interface check - no fault
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth0", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil)

				// Second interface check - has fault
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth1", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcLatencyFaultExistsCommandOutput), nil)
			},
		},
		{
			name:               "Multiple network interfaces - packet loss fault on second interface",
			deviceNames:        []string{"eth0", "eth1"},
			expectedLatency:    false,
			expectedPacketLoss: true,
			expectedError:      nil,
			commandExpectations: func(exec *mock_execwrapper.MockExec, cmd *mock_execwrapper.MockCmd) {
				// First interface check - no fault
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth0", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcCommandEmptyOutput), nil)

				// Second interface check - has fault
				exec.EXPECT().CommandContext(gomock.Any(), "tc", []string{"-j", "q", "show", "dev", "eth1", "parent", "1:1"}).Return(cmd)
				cmd.EXPECT().CombinedOutput().Return([]byte(tcLossFaultExistsCommandOutput), nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockExec := mock_execwrapper.NewMockExec(ctrl)
			mockCmd := mock_execwrapper.NewMockCmd(ctrl)

			handler := &FaultHandler{
				osExecWrapper: mockExec,
			}

			tc.commandExpectations(mockExec, mockCmd)

			taskMetadata := createTestTaskMetadata(tc.deviceNames)
			latencyExists, packetLossExists, err := handler.checkTCFault(context.Background(), taskMetadata)

			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedLatency, latencyExists)
			assert.Equal(t, tc.expectedPacketLoss, packetLossExists)
		})
	}
}
