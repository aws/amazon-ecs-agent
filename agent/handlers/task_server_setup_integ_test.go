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
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This function starts the server and listens on a specified port
func startServer() *http.Server {
	router := mux.NewRouter()
	registerFaultHandlers(router, nil, nil)
	server := &http.Server{
		Addr:    ":5932",
		Handler: router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("ListenAndServe(): %s\n", err)
		}
	}()
	return server
}

// This function shuts down the server after the test
func stopServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("Server Shutdown Failed:%+v", err)
	} else {
		fmt.Println("Server Exited Properly")
	}
}

func TestRateLimiterIntegration(t *testing.T) {

	server := startServer()
	// Case 1: Test same network fault A1 with same method B1
	t.Run("Case 1: Same network faults A1 + same methods B1", func(t *testing.T) {
		resp, err := http.Get("http://localhost:5932/api/container123/fault/v1/network-blackhole-port")
		require.NoError(t, err)
		// Second request should be rate-limited
		resp, err = http.Get("http://localhost:5932/api/container123/fault/v1/network-blackhole-port")
		require.NoError(t, err)
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		stopServer(server)
	})

	// Case 2: Test same network faults A1 but with different method B1 (GET) & B2 (PUT)
	t.Run("Case 2: Same network fault A1 + different methods B1, B2", func(t *testing.T) {
		server = startServer()
		resp, err := http.Get("http://localhost:5932/api/container123/fault/v1/network-blackhole-port")
		require.NoError(t, err)
		// Second request using PUT should not be rate-limited
		req, err := http.NewRequest(http.MethodPut, "http://localhost:5932/api/container123/fault/v1/network-blackhole-port", nil)
		require.NoError(t, err)
		client := &http.Client{}
		resp, err = client.Do(req)
		require.NoError(t, err)
		assert.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
		stopServer(server)
	})

	// Case 3: Test different network faults A1, A2 with same methods B1
	t.Run("Case 3: Different network faults A1, A2 + same method B1", func(t *testing.T) {
		server = startServer()
		resp, err := http.Get("http://localhost:5932/api/container123/fault/v1/network-blackhole-port")
		require.NoError(t, err)
		// Second request for fault A2 with same method should not be rate-limited
		resp, err = http.Get("http://localhost:5932/api/testEndpoint/fault/v1/network-latency")
		require.NoError(t, err)
		assert.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
		stopServer(server)
	})

	// Case 4: Test different network faults A1, A3 with same methods B1
	t.Run("Case 4: Different network faults A1, A3 + same method B1", func(t *testing.T) {
		server = startServer()
		resp, err := http.Get("http://localhost:5932/api/testEndpoint/fault/v1/network-blackhole-port")
		require.NoError(t, err)
		// Second request for fault A3 with same method should not be rate-limited
		resp, err = http.Get("http://localhost:5932/api/testEndpoint/fault/v1/network-packet-loss")
		require.NoError(t, err)
		assert.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
		stopServer(server)
	})

	// Case 5: Test different network faults A2, A3 with same methods B1
	t.Run("Case 5: Different network faults A2, A3 + same method B1", func(t *testing.T) {
		server = startServer()
		resp, err := http.Get("http://localhost:5932/api/testEndpoint/fault/v1/network-latency")
		require.NoError(t, err)
		// Second request for fault A3 with same method should notbe rate-limited
		resp, err = http.Get("http://localhost:5932/api/testEndpoint/fault/v1/network-packet-loss")
		require.NoError(t, err)
		assert.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
		stopServer(server)
	})
}
