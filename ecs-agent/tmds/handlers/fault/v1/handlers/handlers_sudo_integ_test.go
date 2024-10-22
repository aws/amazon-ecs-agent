//go:build linux && sudo
// +build linux,sudo

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
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	mock_state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils/netconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	endpointId     = "endpoint"
	taskARN        = "t1"
	startEndpoint  = "/api/%s/fault/v1/%s/start"
	stopEndpoint   = "/api/%s/fault/v1/%s/stop"
	statusEndpoint = "/api/%s/fault/v1/%s/status"
)

var (
	agentStateExpectations = func(t *testing.T, agentState *mock_state.MockAgentState, netConfigClient *netconfig.NetworkConfigClient) {
		taskResponse := getFaultInjectionTaskResponse(t, netConfigClient)
		agentState.EXPECT().GetTaskMetadataWithTaskNetworkConfig(gomock.Any(), netConfigClient).Return(taskResponse, nil).AnyTimes()
	}
)

// Getter function to construct and return a task response with a task network config object
// which is mainly used as the return value of a GetTaskMetadataWithTaskNetworkConfig call
func getFaultInjectionTaskResponse(t *testing.T, netConfigClient *netconfig.NetworkConfigClient) state.TaskResponse {
	deviceName, err := getHostNetworkInterfaceName(netConfigClient)
	require.NoError(t, err)
	taskNetworkConfig := state.TaskNetworkConfig{
		NetworkMode: ecs.NetworkModeHost,
		NetworkNamespaces: []*state.NetworkNamespace{
			{
				Path: "/some/path",
				NetworkInterfaces: []*state.NetworkInterface{
					{
						DeviceName: deviceName,
					},
				},
			},
		},
	}
	taskResponse := state.TaskResponse{
		TaskResponse:          &v2.TaskResponse{TaskARN: taskARN},
		TaskNetworkConfig:     &taskNetworkConfig,
		FaultInjectionEnabled: true,
	}

	return taskResponse
}

// getHostNetworkInterfaceName obtains the default network interface name on the host via the DefaultNetInterfaceName method
func getHostNetworkInterfaceName(netConfigClient *netconfig.NetworkConfigClient) (string, error) {
	deviceName, err := netconfig.DefaultNetInterfaceName(netConfigClient.NetlinkClient)
	return deviceName, err
}

// Getter function to construct and return a request body for black hole port loss request
func getNetworkBlackHolePortRequestBody(port int, protocol, trafficType string) map[string]interface{} {
	return map[string]interface{}{
		"Port":        port,
		"Protocol":    protocol,
		"TrafficType": trafficType,
	}
}

// Getter function to construct and return a request body for network latency request
func getNetworkLatencyRequestBody(delayMilliseconds, jitterMilliseconds int, ipSources, ipSourcesToFilter []string) map[string]interface{} {
	return map[string]interface{}{
		"DelayMilliseconds":  delayMilliseconds,
		"JitterMilliseconds": jitterMilliseconds,
		"Sources":            ipSources,
		"SourcesToFilter":    ipSourcesToFilter,
	}
}

// Getter function to construct and return a request body for network packet loss request
func getNetworkPacketLossRequestBody(lossPercent int, ipSources, ipSourcesToFilter []string) map[string]interface{} {
	return map[string]interface{}{
		"LossPercent":     lossPercent,
		"Sources":         ipSources,
		"SourcesToFilter": ipSourcesToFilter,
	}
}

// Helper function that starts a HTTP server as well as set up the fault injection handlers and required mocks
func startServer(t *testing.T) (*http.Server, int) {
	// Mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	agentState := mock_state.NewMockAgentState(ctrl)
	nc := netconfig.NewNetworkConfigClient()
	agentStateExpectations(t, agentState, nc)

	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	router := mux.NewRouter()
	handler := New(agentState, metricsFactory, execwrapper.NewExec())

	// Registering the network black hole port fault injection handlers
	router.HandleFunc(
		fmt.Sprintf(startEndpoint, endpointId, types.BlackHolePortFaultType),
		handler.StartNetworkBlackholePort(),
	).Methods(http.MethodPost)
	router.HandleFunc(
		fmt.Sprintf(stopEndpoint, endpointId, types.BlackHolePortFaultType),
		handler.StopNetworkBlackHolePort(),
	).Methods(http.MethodPost)
	router.HandleFunc(
		fmt.Sprintf(statusEndpoint, endpointId, types.BlackHolePortFaultType),
		handler.CheckNetworkBlackHolePort(),
	).Methods(http.MethodPost)

	// Registering the network latency hole fault injection handlers
	router.HandleFunc(
		fmt.Sprintf(startEndpoint, endpointId, types.LatencyFaultType),
		handler.StartNetworkLatency(),
	).Methods(http.MethodPost)
	router.HandleFunc(
		fmt.Sprintf(stopEndpoint, endpointId, types.LatencyFaultType),
		handler.StopNetworkLatency(),
	).Methods(http.MethodPost)
	router.HandleFunc(
		fmt.Sprintf(statusEndpoint, endpointId, types.LatencyFaultType),
		handler.CheckNetworkLatency(),
	).Methods(http.MethodPost)

	// Registering the network packet loss fault injection handlers
	router.HandleFunc(
		fmt.Sprintf(startEndpoint, endpointId, types.PacketLossFaultType),
		handler.StartNetworkPacketLoss(),
	).Methods(http.MethodPost)
	router.HandleFunc(
		fmt.Sprintf(stopEndpoint, endpointId, types.PacketLossFaultType),
		handler.StopNetworkPacketLoss(),
	).Methods(http.MethodPost)
	router.HandleFunc(
		fmt.Sprintf(statusEndpoint, endpointId, types.PacketLossFaultType),
		handler.CheckNetworkPacketLoss(),
	).Methods(http.MethodPost)

	server := &http.Server{
		Addr:    ":0", // Lets the system allocate an available port
		Handler: router,
	}

	// Obtaining the port being used by the server
	listener, err := net.Listen("tcp", server.Addr)
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("ListenAndServe(): %s\n", err)
			require.NoError(t, err)
		}
	}()

	return server, port
}

// // Helper function to shut down a HTTP server
func shutdownServer(t *testing.T, server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Logf("Server Shutdown Failed:%+v", err)
	} else {
		t.Logf("Server Exited Properly")
	}
}

// Helper function that generates a local host URL with a certain port and path. This will be used to make HTTP requests to a HTTP server.
func getURL(port int, path string) string {
	return "http://localhost:" + fmt.Sprintf("%d", port) + path
}

// cleanupBlackHolePortFault will make requests to stop network black hole port requests within the server as a way to clean up after starting a black hole port fault injection on the host.
func cleanupBlackHolePortFault(t *testing.T, serverPort int, body interface{}) {
	client := &http.Client{}

	// Making a stop black hole port fault request to the server
	bodyBytes, _ := json.Marshal(body)
	bodyReader := bytes.NewReader(bodyBytes)
	req, err := http.NewRequest(http.MethodPost, getURL(serverPort, fmt.Sprintf(stopEndpoint, endpointId, types.BlackHolePortFaultType)), bodyReader)
	if err != nil {
		t.Logf("Unable to create stop black hole port request")
	}
	_, err = client.Do(req)
	if err != nil {
		t.Logf("Error occurred while cleaning up network black hole port fault injection: %v\n", err)
	}
}

// cleanupLatencyAndPacketLossFaults will make requests to stop both network latency and network packet loss requests within the server as a way to clean up
// after starting either a latency and/or packet loss fault injection on the host.
func cleanupLatencyAndPacketLossFaults(t *testing.T, serverPort int) {
	client := &http.Client{}

	// Making a stop network latency fault request to the server
	req1, err := http.NewRequest(http.MethodPost, getURL(serverPort, fmt.Sprintf(stopEndpoint, endpointId, "network-latency")), nil)
	if err != nil {
		t.Logf("Unable to create stop latency request")
	}
	_, err = client.Do(req1)
	if err != nil {
		t.Logf("Error occurred while cleaning up network latency fault injection: %v\n", err)
	}

	// Making a stop network packet loss fault request to the server
	req2, err := http.NewRequest(http.MethodPost, getURL(serverPort, fmt.Sprintf(stopEndpoint, endpointId, "network-packet-loss")), nil)
	if err != nil {
		t.Logf("Unable to create stop packet loss request")
	}
	_, err = client.Do(req2)
	if err != nil {
		t.Logf("Error occurred while cleaning up network packet loss fault injection: %v\n", err)
	}
}

// skipForUnsupportedTc will test whether or not the tc utility installed can use the required flag option(s) [e.g. -j] which is used to start/stop/check status of certain
// network faults such as latency and packet loss.
func skipForUnsupportedTc(t *testing.T) {
	cmdCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path, err := exec.LookPath("tc")
	if err != nil {
		t.Skipf("tc utility tool is not found on the host. Error: %v", err)
	}
	args := []string{"-j", "q", "show"}
	_, err = exec.CommandContext(cmdCtx, path, args[0:]...).CombinedOutput()
	if err != nil {
		t.Skipf("Current version of the tc utility does have the required flag/configuration. Error: %v", err)
	}
}

func TestParallelNetworkFaults(t *testing.T) {
	tcs := []struct {
		name          string
		faultType1    string
		faultType2    string
		body1         interface{}
		body2         interface{}
		responseCode1 int
		responseCode2 int
	}{
		{
			name:          "network black hole port same type",
			faultType1:    types.BlackHolePortFaultType,
			faultType2:    types.BlackHolePortFaultType,
			body1:         getNetworkBlackHolePortRequestBody(1234, "tcp", "ingress"),
			body2:         getNetworkBlackHolePortRequestBody(4321, "tcp", "egress"),
			responseCode1: http.StatusOK,
			responseCode2: http.StatusOK,
		},
		{
			name:          "network latency same type",
			faultType1:    types.LatencyFaultType,
			faultType2:    types.LatencyFaultType,
			body1:         getNetworkLatencyRequestBody(100, 200, []string{"1.1.1.1", "2.2.2.2"}, []string{"8.8.8.8"}),
			body2:         getNetworkLatencyRequestBody(200, 300, []string{"3.3.3.3", "4.4.4.4"}, []string{"9.9.9.9"}),
			responseCode1: http.StatusOK,
			responseCode2: http.StatusConflict,
		},
		{
			name:          "network packet loss same type",
			faultType1:    types.PacketLossFaultType,
			faultType2:    types.PacketLossFaultType,
			body1:         getNetworkPacketLossRequestBody(50, []string{"1.1.1.1", "2.2.2.2"}, []string{"8.8.8.8"}),
			body2:         getNetworkPacketLossRequestBody(70, []string{"3.3.3.3", "4.4.4.4"}, []string{"9.9.9.9"}),
			responseCode1: http.StatusOK,
			responseCode2: http.StatusConflict,
		},
		{
			name:          "network latency and packet loss different type",
			faultType1:    types.LatencyFaultType,
			faultType2:    types.PacketLossFaultType,
			body1:         getNetworkLatencyRequestBody(100, 200, []string{"1.1.1.1", "2.2.2.2"}, []string{"8.8.8.8"}),
			body2:         getNetworkPacketLossRequestBody(70, []string{"3.3.3.3", "4.4.4.4"}, []string{"9.9.9.9"}),
			responseCode1: http.StatusOK,
			responseCode2: http.StatusConflict,
		},
		{
			name:          "network black hole port and latency different type",
			faultType1:    types.BlackHolePortFaultType,
			faultType2:    types.LatencyFaultType,
			body1:         getNetworkBlackHolePortRequestBody(4321, "tcp", "ingress"),
			body2:         getNetworkLatencyRequestBody(100, 200, []string{"1.1.1.1", "2.2.2.2"}, []string{"8.8.8.8"}),
			responseCode1: http.StatusOK,
			responseCode2: http.StatusOK,
		},
		{
			name:          "network black hole port and packet loss different type",
			faultType1:    types.BlackHolePortFaultType,
			faultType2:    types.PacketLossFaultType,
			body1:         getNetworkBlackHolePortRequestBody(4321, "tcp", "ingress"),
			body2:         getNetworkPacketLossRequestBody(50, []string{"1.1.1.1", "2.2.2.2"}, []string{"8.8.8.8"}),
			responseCode1: http.StatusOK,
			responseCode2: http.StatusOK,
		},
	}
	skipForUnsupportedTc(t)
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			server, port := startServer(t)
			defer shutdownServer(t, server)
			// Cleaning up the fault injections afterwards
			defer func() {
				t.Logf("Cleaning up fault injections...")
				cleanupLatencyAndPacketLossFaults(t, port)
				cleanupBlackHolePortFault(t, port, tc.body1)
				// Sleeping for more than 5 seconds since we're only accepting 1 request per 5 seconds for each requests at a time
				time.Sleep(6 * time.Second)
				cleanupBlackHolePortFault(t, port, tc.body2)
			}()

			client := &http.Client{}

			// Making a start network fault request for the first fault type
			bodyBytes, _ := json.Marshal(tc.body1)
			bodyReader := bytes.NewReader(bodyBytes)
			req1, err := http.NewRequest(http.MethodPost, getURL(port, fmt.Sprintf(startEndpoint, endpointId, tc.faultType1)), bodyReader)
			require.NoError(t, err)
			res1, err := client.Do(req1)
			require.NoError(t, err)

			// Sleeping for more than 5 seconds since we're only accepting 1 request per 5 seconds for each requests at a time
			time.Sleep(6 * time.Second)

			// Making a start network fault request for the second fault type
			bodyBytes, _ = json.Marshal(tc.body2)
			bodyReader = bytes.NewReader(bodyBytes)
			req2, err := http.NewRequest(http.MethodPost, getURL(port, fmt.Sprintf(startEndpoint, endpointId, tc.faultType2)), bodyReader)
			require.NoError(t, err)
			res2, err := client.Do(req2)
			require.NoError(t, err)

			assert.Equal(t, tc.responseCode1, res1.StatusCode)
			assert.Equal(t, tc.responseCode2, res2.StatusCode)
		})
	}
}
