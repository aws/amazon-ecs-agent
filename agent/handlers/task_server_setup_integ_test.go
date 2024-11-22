//go:build integration
// +build integration

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
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	agentV4 "github.com/aws/amazon-ecs-agent/agent/handlers/v4"
	mock_stats "github.com/aws/amazon-ecs-agent/agent/stats/mock"
	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_execwrapper "github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper/mocks"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	clusterName          = "default"
	availabilityzone     = "us-west-2b"
	vpcID                = "test-vpc-id"
	containerInstanceArn = "containerInstanceArn-test"
)

// This function starts the server and listens on a specified port
func startServer(t *testing.T) (*http.Server, int) {
	router := mux.NewRouter()

	// Mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)
	ecsClient := mock_ecs.NewMockECSClient(ctrl)

	agentState := agentV4.NewTMDSAgentState(state, statsEngine, ecsClient, clusterName, availabilityzone, vpcID, containerInstanceArn)
	metricsFactory := metrics.NewNopEntryFactory()
	execWrapper := mock_execwrapper.NewMockExec(ctrl)

	registerFaultHandlers(router, agentState, metricsFactory, execWrapper)

	server := &http.Server{
		Addr:    ":0", // Lets the system allocate an available port
		Handler: router,
	}

	listener, err := net.Listen("tcp", server.Addr)
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port
	t.Logf("Server started on port: %d", port)

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("ListenAndServe(): %s\n", err)
		}
	}()
	return server, port
}

// This function shuts down the server after the test
func stopServer(t *testing.T, server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Logf("Server Shutdown Failed:%+v", err)
	} else {
		t.Logf("Server Exited Properly")
	}
}

// Table-driven tests for rate limiter
func TestRateLimiterIntegration(t *testing.T) {

	testCases := []struct {
		name            string
		method1         string
		method2         string
		url1            string
		url2            string
		expectedStatus2 int
		assertNotEqual  bool
	}{
		{
			name:            "Same network faults A1 + same methods B1",
			method1:         "POST",
			method2:         "POST",
			url1:            "/api/container123/fault/v1/network-blackhole-port/status",
			url2:            "/api/container123/fault/v1/network-blackhole-port/status",
			expectedStatus2: http.StatusTooManyRequests,
			assertNotEqual:  false,
		},
		{
			name:            "Same network fault A1 + different methods B1, B2",
			method1:         "POST",
			method2:         "POST",
			url1:            "/api/container123/fault/v1/network-blackhole-port/status",
			url2:            "/api/container123/fault/v1/network-blackhole-port/start",
			expectedStatus2: http.StatusTooManyRequests,
			assertNotEqual:  true,
		},
		{
			name:            "Different network faults A1, A2 + same method B1",
			method1:         "POST",
			method2:         "POST",
			url1:            "/api/container123/fault/v1/network-blackhole-port/start",
			url2:            "/api/container123/fault/v1/network-latency/start",
			expectedStatus2: http.StatusTooManyRequests,
			assertNotEqual:  true,
		},
		{
			name:            "Different network faults A1, A3 + same method B1",
			method1:         "POST",
			method2:         "POST",
			url1:            "/api/container123/fault/v1/network-blackhole-port/start",
			url2:            "/api/container123/fault/v1/network-packet-loss/start",
			expectedStatus2: http.StatusTooManyRequests,
			assertNotEqual:  true,
		},
		{
			name:            "Different network faults A2, A3 + same methods B1",
			method1:         "POST",
			method2:         "POST",
			url1:            "/api/container123/fault/v1/network-latency/start",
			url2:            "/api/container123/fault/v1/network-packet-loss/start",
			expectedStatus2: http.StatusTooManyRequests,
			assertNotEqual:  true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server, port := startServer(t)
			client := &http.Client{}
			// First request
			req1, err := http.NewRequest(tc.method1, getURL(port, tc.url1), nil)
			require.NoError(t, err)
			_, err = client.Do(req1)
			require.NoError(t, err)

			// Second request
			req2, err := http.NewRequest(tc.method2, getURL(port, tc.url2), nil)
			require.NoError(t, err)
			resp2, err := client.Do(req2)
			require.NoError(t, err)
			if tc.assertNotEqual {
				assert.NotEqual(t, tc.expectedStatus2, resp2.StatusCode)
			} else {
				assert.Equal(t, tc.expectedStatus2, resp2.StatusCode)
			}
			stopServer(t, server)
		})
	}
}

// Utility function to generate a URL with a dynamic port
func getURL(port int, path string) string {
	return "http://localhost:" + fmt.Sprintf("%d", port) + path
}
