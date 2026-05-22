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
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	mock_state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils/netconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"

	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	endpointId         = "endpoint"
	taskARN            = "t1"
	startEndpoint      = "/api/%s/fault/v1/%s/start"
	stopEndpoint       = "/api/%s/fault/v1/%s/stop"
	statusEndpoint     = "/api/%s/fault/v1/%s/status"
	addSourcesEndpoint = "/api/%s/fault/v1/%s/add-sources"
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
		NetworkMode: string(ecstypes.NetworkModeHost),
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

	// Registering the add-sources fault injection handlers for both fault types
	router.HandleFunc(
		fmt.Sprintf(addSourcesEndpoint, endpointId, types.LatencyFaultType),
		handler.AddSourcesNetworkLatency(),
	).Methods(http.MethodPost)
	router.HandleFunc(
		fmt.Sprintf(addSourcesEndpoint, endpointId, types.PacketLossFaultType),
		handler.AddSourcesNetworkPacketLoss(),
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

// ===========================================================================
// FIS-driven add-sources endpoint integration tests.
//
// Covers the happy paths, 404 / 400 contract responses, and the
// concurrency / idempotency expectations of the add-sources endpoint for
// both latency and packet-loss fault types.
//
// IPv6 rejection cases are intentionally omitted because the implementation
// accepts IPv6 addresses and CIDR blocks, and a v6 filter is installed
// successfully end to end. host-vs-awsvpc network mode and
// multiple-interface permutations are omitted because the sudo-style
// harness only models a single host-mode interface; those network-mode
// cases are covered by handler-level unit tests in handlers_test.go. Rate
// limit and telemetry cases are omitted because this harness wires
// handlers without the tollbooth limiter or telemetry middleware that
// would make the observable behavior meaningful.
// ===========================================================================

// addSourcesRequestBody constructs the JSON shape the SSM script sends to
// the add-sources endpoint. A nil list is omitted from the encoded JSON
// entirely; pass an empty []string{} to send the field with a `[]` value
// (used by the empty-request 400 case). The two arguments are independent.
func addSourcesRequestBody(sources, sourcesToFilter []string) map[string]interface{} {
	body := map[string]interface{}{}
	if sources != nil {
		body["Sources"] = sources
	}
	if sourcesToFilter != nil {
		body["SourcesToFilter"] = sourcesToFilter
	}
	return body
}

// postFault sends a POST to the named fault endpoint on the test server and
// returns the status code and response body. body=nil sends an empty
// request; body=string sends the raw string verbatim (used for the
// malformed-JSON case); any other value is JSON-marshalled. The response
// body is fully read and the underlying connection closed before this
// helper returns.
func postFault(t *testing.T, port int, urlFormat, faultType string, body interface{}) (int, []byte) {
	t.Helper()
	var reader io.Reader
	switch v := body.(type) {
	case nil:
		reader = nil
	case string:
		reader = strings.NewReader(v)
	default:
		buf, err := json.Marshal(v)
		require.NoError(t, err)
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(http.MethodPost,
		getURL(port, fmt.Sprintf(urlFormat, endpointId, faultType)),
		reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, respBody
}

// startLatencyFault issues a /start request that brings the latency tc
// hierarchy up against the host's default interface. The caller is
// responsible for cleanup (typically via defer cleanupLatencyAndPacketLossFaults).
func startLatencyFault(t *testing.T, port int) {
	t.Helper()
	body := getNetworkLatencyRequestBody(100, 0,
		[]string{"203.0.113.1"}, nil)
	status, respBody := postFault(t, port, startEndpoint, types.LatencyFaultType, body)
	require.Equal(t, http.StatusOK, status,
		"latency /start failed: %s", string(respBody))
}

// startPacketLossFault is the packet-loss twin of startLatencyFault.
func startPacketLossFault(t *testing.T, port int) {
	t.Helper()
	body := getNetworkPacketLossRequestBody(50,
		[]string{"203.0.113.1"}, nil)
	status, respBody := postFault(t, port, startEndpoint, types.PacketLossFaultType, body)
	require.Equal(t, http.StatusOK, status,
		"packet-loss /start failed: %s", string(respBody))
}

// hostInterface returns the host's default network interface name. Tests
// that need to introspect real tc state shell out against this device.
func hostInterface(t *testing.T) string {
	t.Helper()
	dev, err := netconfig.DefaultNetInterfaceName(netconfig.NewNetworkConfigClient().NetlinkClient)
	require.NoError(t, err)
	require.NotEmpty(t, dev)
	return dev
}

// tcShowTimeout caps how long any tc inspection helper waits for a real tc
// invocation. 5s is the same budget the fault handler uses for tc commands;
// keeping the test side identical avoids spurious skews between observed
// behavior and the production timeout.
const tcShowTimeout = 5 * time.Second

// tcFilterShow runs `tc filter show dev <iface>` and returns its output.
// Used by tests that assert on which IPs ended up installed by add-sources.
func tcFilterShow(t *testing.T, iface string) string {
	t.Helper()
	cmdCtx, cancel := context.WithTimeout(context.Background(), tcShowTimeout)
	defer cancel()
	out, err := exec.CommandContext(cmdCtx, "tc", "filter", "show", "dev", iface).CombinedOutput()
	require.NoError(t, err, "tc filter show failed: %s", string(out))
	return string(out)
}

// tcQdiscShow runs `tc qdisc show dev <iface>` and returns the output.
// Used by the StopAfterAdd test to assert the qdisc hierarchy is fully gone.
func tcQdiscShow(t *testing.T, iface string) string {
	t.Helper()
	cmdCtx, cancel := context.WithTimeout(context.Background(), tcShowTimeout)
	defer cancel()
	out, err := exec.CommandContext(cmdCtx, "tc", "qdisc", "show", "dev", iface).CombinedOutput()
	require.NoError(t, err, "tc qdisc show failed: %s", string(out))
	return string(out)
}

// hexForIPv4 converts a dotted-quad IPv4 address into the lowercase 8-char
// hex representation tc filter show uses in its `match <hex>/ffffffff at 16`
// output. tc renders the destination IP as a single 32-bit big-endian word.
// The test fails fast on a malformed input so callers don't have to guard
// against silent empty-string returns.
func hexForIPv4(t *testing.T, ip string) string {
	t.Helper()
	parts := strings.Split(ip, ".")
	require.Lenf(t, parts, 4, "hexForIPv4: %q is not a dotted-quad IPv4 address", ip)
	out := make([]byte, 0, 8)
	const hex = "0123456789abcdef"
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		require.NoErrorf(t, err, "hexForIPv4: octet %q in %q is not an integer", p, ip)
		require.Truef(t, n >= 0 && n <= 255,
			"hexForIPv4: octet %d in %q out of range", n, ip)
		out = append(out, hex[(n>>4)&0xf], hex[n&0xf])
	}
	return string(out)
}

// assertFilterContainsIP fails the test with a focused message if the tc
// filter table doesn't include the hex-encoded form of `ip`. Replaces ad-hoc
// `assert.Contains(filterOutput, hexForIPv4(t, ip))` calls so failures cite
// the specific IP that was missing rather than dumping the full tc table
// with no hint of which assertion failed.
func assertFilterContainsIP(t *testing.T, filterOutput, ip string) {
	t.Helper()
	assert.Containsf(t, filterOutput, hexForIPv4(t, ip),
		"tc filter table missing %s:\n%s", ip, filterOutput)
}

// ---------------------------------------------------------------------------
// Happy-path tests.
// ---------------------------------------------------------------------------

// TestAddSourcesLatencyHappyPath: with a latency
// fault running, posting two new IPs to /add-sources returns 200 and both
// IPs land on the impaired flowid (1:1) in the tc filter table.
func TestAddSourcesLatencyHappyPath(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	const ip1, ip2 = "203.0.113.10", "203.0.113.11"
	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{ip1, ip2}, nil))
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"Status":"running"}`, string(body))

	// Verify the kernel actually installed filters for both IPs on flowid 1:1.
	filterOutput := tcFilterShow(t, hostInterface(t))
	assertFilterContainsIP(t, filterOutput, ip1)
	assertFilterContainsIP(t, filterOutput, ip2)
	assert.Contains(t, filterOutput, "flowid 1:1",
		"impaired flow not present in filter table")
}

// TestAddSourcesPacketLossHappyPath is the packet-loss twin of
// TestAddSourcesLatencyHappyPath.
func TestAddSourcesPacketLossHappyPath(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startPacketLossFault(t, port)

	const ip = "203.0.113.20"
	status, body := postFault(t, port, addSourcesEndpoint, types.PacketLossFaultType,
		addSourcesRequestBody([]string{ip}, nil))
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"Status":"running"}`, string(body))

	filterOutput := tcFilterShow(t, hostInterface(t))
	assertFilterContainsIP(t, filterOutput, ip)
}

// TestAddSourcesWithSourcesToFilter: a request
// mixing Sources (impaired, flowid 1:1) and SourcesToFilter (allowlist,
// flowid 1:3) installs both filter shapes.
func TestAddSourcesWithSourcesToFilter(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	const impairedIP = "203.0.113.30"
	const allowlistIP = "10.0.0.99"
	status, _ := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{impairedIP}, []string{allowlistIP}))
	assert.Equal(t, http.StatusOK, status)

	filterOutput := tcFilterShow(t, hostInterface(t))
	assertFilterContainsIP(t, filterOutput, impairedIP)
	assertFilterContainsIP(t, filterOutput, allowlistIP)
	assert.Contains(t, filterOutput, "flowid 1:1")
	assert.Contains(t, filterOutput, "flowid 1:3")
}

// TestAddSourcesOnlySourcesToFilter: Sources empty,
// SourcesToFilter populated. Only the allowlist filter is installed.
func TestAddSourcesOnlySourcesToFilter(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	// Snapshot the impaired-filter set installed by /start so we can prove
	// add-sources didn't add another impaired filter on top of it.
	pre := tcFilterShow(t, hostInterface(t))
	preFlow11 := strings.Count(pre, "flowid 1:1")

	const allowlistIP = "10.0.0.55"
	status, _ := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody(nil, []string{allowlistIP}))
	assert.Equal(t, http.StatusOK, status)

	post := tcFilterShow(t, hostInterface(t))
	assertFilterContainsIP(t, post, allowlistIP)
	assert.Contains(t, post, "flowid 1:3")
	assert.Equal(t, preFlow11, strings.Count(post, "flowid 1:1"),
		"impaired-filter count must not change when only SourcesToFilter is sent")
}

// TestAddSourcesCIDRBlock: a CIDR block in Sources
// is accepted and translated into a tc filter for the prefix.
func TestAddSourcesCIDRBlock(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	// Use a /28 to keep the filter mask distinct from a single-host /32.
	const cidr = "203.0.113.0/28"
	status, _ := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{cidr}, nil))
	assert.Equal(t, http.StatusOK, status)

	filterOutput := tcFilterShow(t, hostInterface(t))
	// /28 = 0xfffffff0 mask, base 203.0.113.0 = cb007100 in hex.
	assert.Contains(t, filterOutput, "cb007100/fffffff0",
		"tc filter table missing CIDR-shaped match: %s", filterOutput)
}

// ---------------------------------------------------------------------------
// 404 paths.
// ---------------------------------------------------------------------------

// TestAddSourcesNoFaultRunningReturns404: with no
// prior /start, the qdisc hierarchy is absent and add-sources returns 404
// with the not-running contract message.
func TestAddSourcesNoFaultRunningReturns404(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{"203.0.113.10"}, nil))
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, string(body), latencyFaultNotRunningError)
}

// TestAddSourcesWrongFaultTypeReturns404: latency
// is running, but the request hits the packet-loss endpoint and
// faultExistsSelector returns false so the response is 404.
func TestAddSourcesWrongFaultTypeReturns404(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	status, body := postFault(t, port, addSourcesEndpoint, types.PacketLossFaultType,
		addSourcesRequestBody([]string{"203.0.113.10"}, nil))
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, string(body), packetLossFaultNotRunningError)
}

// TestAddSourcesAfterStopReturns404: a /start
// followed by /stop tears down the qdisc; the next add-sources observes an
// empty tc state and returns 404.
func TestAddSourcesAfterStopReturns404(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)
	stopStatus, _ := postFault(t, port, stopEndpoint, types.LatencyFaultType, map[string]interface{}{})
	require.Equal(t, http.StatusOK, stopStatus)

	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{"203.0.113.10"}, nil))
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, string(body), latencyFaultNotRunningError)
}

// ---------------------------------------------------------------------------
// 400 validation paths.
// ---------------------------------------------------------------------------

// TestAddSourcesEmptyRequestReturns400 verifies that a body with both
// Sources and SourcesToFilter empty is rejected with 400.
func TestAddSourcesEmptyRequestReturns400(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)

	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		map[string]interface{}{"Sources": []string{}, "SourcesToFilter": []string{}})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, string(body), types.AddSourcesEmptyError)
}

// TestAddSourcesOverCapReturns400 verifies that a request exceeding
// AddSourcesMaxIPs is rejected with 400 and an error citing both counts.
func TestAddSourcesOverCapReturns400(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)

	overCap := types.AddSourcesMaxIPs + 1
	ips := make([]string, overCap)
	for i := range ips {
		ips[i] = fmt.Sprintf("10.0.0.%d", i+1)
	}
	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody(ips, nil))
	assert.Equal(t, http.StatusBadRequest, status)
	// The error message pins both the actual count and the cap.
	assert.Contains(t, string(body), strconv.Itoa(overCap))
	assert.Contains(t, string(body), strconv.Itoa(types.AddSourcesMaxIPs))
}

// TestAddSourcesAtCapBoundary: exactly
// AddSourcesMaxIPs entries across the two lists is accepted.
func TestAddSourcesAtCapBoundary(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	// Split the cap evenly between the two lists. types.AddSourcesMaxIPs is
	// even (16) by design; the test would need updating if the cap moved to
	// an odd value.
	half := types.AddSourcesMaxIPs / 2
	srcs := make([]string, half)
	filters := make([]string, half)
	// Start the srcs range at 203.0.113.10 to keep it disjoint from the
	// 203.0.113.1 filter installed by startLatencyFault; otherwise the
	// `assertFilterContainsIP(out, srcs[0])` check would pass even if the
	// boundary add-sources call were a no-op.
	for i := 0; i < half; i++ {
		srcs[i] = fmt.Sprintf("203.0.113.%d", i+10)
		filters[i] = fmt.Sprintf("10.0.0.%d", i+10)
	}
	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody(srcs, filters))
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"Status":"running"}`, string(body))

	// All AddSourcesMaxIPs entries should have landed in the filter table.
	// We verify the first and last of each list to keep the assertion focused.
	out := tcFilterShow(t, hostInterface(t))
	assertFilterContainsIP(t, out, srcs[0])
	assertFilterContainsIP(t, out, srcs[len(srcs)-1])
	assertFilterContainsIP(t, out, filters[0])
	assertFilterContainsIP(t, out, filters[len(filters)-1])
}

// TestAddSourcesWithinRequestOverlap verifies that an IP appearing in both
// Sources and SourcesToFilter within a single request is rejected with 400.
func TestAddSourcesWithinRequestOverlap(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)

	const ip = "203.0.113.10"
	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{ip}, []string{ip}))
	assert.Equal(t, http.StatusBadRequest, status)
	// Pin to the exact error format so accidental edits to the message
	// have to update the test alongside the production string.
	assert.Contains(t, string(body), fmt.Sprintf(types.AddSourcesOverlapError, ip))
}

// TestAddSourcesInvalidIP verifies that a malformed address in Sources is
// rejected with 400 and the canonical InvalidValueError message.
func TestAddSourcesInvalidIP(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)

	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{"not-an-ip"}, nil))
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, string(body),
		fmt.Sprintf(types.InvalidValueError, "not-an-ip", "Sources"))
}

// TestAddSourcesMalformedJSON sends a truncated
// JSON object; the decoder fails before validation runs.
func TestAddSourcesMalformedJSON(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)

	status, _ := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		`{"Sources":["1.1.1.1"`)
	assert.Equal(t, http.StatusBadRequest, status)
}

// TestAddSourcesMissingBody verifies that a request with no body is
// rejected with 400 and the MissingRequestBodyError contract message.
func TestAddSourcesMissingBody(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)

	status, body := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType, nil)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, string(body), types.MissingRequestBodyError)
}

// ---------------------------------------------------------------------------
// Concurrency / idempotency.
// ---------------------------------------------------------------------------

// TestAddSourcesDuplicateAcrossRequestsIdempotent:
// two sequential add-sources calls with the same IP both succeed.
//
// The handler does not deduplicate; the manual-test "Add same IP twice"
// observation shows the kernel installs a second filter and both calls
// return 200, which matches the expectation here.
func TestAddSourcesDuplicateAcrossRequestsIdempotent(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	body := addSourcesRequestBody([]string{"203.0.113.10"}, nil)
	status1, _ := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType, body)
	assert.Equal(t, http.StatusOK, status1)
	status2, _ := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType, body)
	assert.Equal(t, http.StatusOK, status2)
}

// TestAddSourcesConcurrentWithStopSerialized: an
// add-sources and a stop fired concurrently must both terminate, and the
// add-sources response must be either 200 (it raced ahead of the stop) or
// 404 (stop tore down the qdisc first). Neither must produce a 500.
func TestAddSourcesConcurrentWithStopSerialized(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	var wg sync.WaitGroup
	var addStatus, stopStatus int
	wg.Add(2)
	go func() {
		defer wg.Done()
		addStatus, _ = postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
			addSourcesRequestBody([]string{"203.0.113.10"}, nil))
	}()
	go func() {
		defer wg.Done()
		stopStatus, _ = postFault(t, port, stopEndpoint, types.LatencyFaultType,
			map[string]interface{}{})
	}()
	wg.Wait()

	assert.Equal(t, http.StatusOK, stopStatus,
		"stop must complete cleanly even when add-sources is in flight")
	assert.True(t,
		addStatus == http.StatusOK || addStatus == http.StatusNotFound,
		"add-sources concurrent with stop must be 200 or 404, got %d", addStatus)
	assert.NotEqual(t, http.StatusInternalServerError, addStatus,
		"add-sources concurrent with stop must not 500")
}

// TestAddSourcesConcurrentSerialized: two
// add-sources calls fired concurrently are serialized by the per-namespace
// lock; both return 200 and every requested filter ends up installed.
func TestAddSourcesConcurrentSerialized(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	const ipA, ipB = "203.0.113.10", "203.0.113.11"
	var wg sync.WaitGroup
	statuses := [2]int{}
	bodies := [2]map[string]interface{}{
		addSourcesRequestBody([]string{ipA}, nil),
		addSourcesRequestBody([]string{ipB}, nil),
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			statuses[idx], _ = postFault(t, port, addSourcesEndpoint,
				types.LatencyFaultType, bodies[idx])
		}(i)
	}
	wg.Wait()

	assert.Equal(t, http.StatusOK, statuses[0])
	assert.Equal(t, http.StatusOK, statuses[1])

	out := tcFilterShow(t, hostInterface(t))
	assertFilterContainsIP(t, out, ipA)
	assertFilterContainsIP(t, out, ipB)
}

// TestAddSourcesDoesNotMutateFaultParameters: the
// add-sources handler must not change the qdisc parameters established at
// /start. We assert by snapshotting `tc qdisc show` before and after.
func TestAddSourcesDoesNotMutateFaultParameters(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	startLatencyFault(t, port)

	iface := hostInterface(t)
	pre := tcQdiscShow(t, iface)

	status, _ := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{"203.0.113.10"}, nil))
	require.Equal(t, http.StatusOK, status)

	post := tcQdiscShow(t, iface)
	assert.Equal(t, pre, post,
		"qdisc state must not change across add-sources:\nbefore: %s\nafter:  %s", pre, post)
}

// TestAddSourcesStopAfterAddCleansEverything verifies that
// /add-sources installs new filters; /stop must remove the entire qdisc
// hierarchy regardless. We verify by snapshotting `tc qdisc show` before
// any fault and after the stop, and asserting the two are equal.
func TestAddSourcesStopAfterAddCleansEverything(t *testing.T) {
	skipForUnsupportedTc(t)
	server, port := startServer(t)
	defer shutdownServer(t, server)
	defer cleanupLatencyAndPacketLossFaults(t, port)

	iface := hostInterface(t)
	baseline := tcQdiscShow(t, iface)

	startLatencyFault(t, port)
	addStatus, _ := postFault(t, port, addSourcesEndpoint, types.LatencyFaultType,
		addSourcesRequestBody([]string{"203.0.113.10", "203.0.113.11"}, nil))
	require.Equal(t, http.StatusOK, addStatus)
	stopStatus, _ := postFault(t, port, stopEndpoint, types.LatencyFaultType,
		map[string]interface{}{})
	require.Equal(t, http.StatusOK, stopStatus)

	cleaned := tcQdiscShow(t, iface)
	assert.Equal(t, baseline, cleaned,
		"qdisc state must return to baseline after stop:\nbaseline: %s\nafter stop: %s",
		baseline, cleaned)
}
