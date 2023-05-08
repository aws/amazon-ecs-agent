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
package tmds

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	mock_audit "github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/mocks"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that the request rate limits of a server created using NewServer() has effect.
func TestRequestRateLimiter(t *testing.T) {
	overLimitCallCount := 5 // number of requests that will exceed the rate limits
	serverAddress := "127.0.0.1:3598"

	// Setup a mock audit logger, audit logger is used for logging over limit calls
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	auditLogger := mock_audit.NewMockAuditLogger(ctrl)
	auditLogger.EXPECT().
		Log(gomock.Any(), http.StatusTooManyRequests, "").
		Times(overLimitCallCount).
		Return()

	// Setup a simple router with a simple handler
	router := mux.NewRouter()
	router.HandleFunc("/", helloWorldHandler())

	// Setup the server with a low rate limit for testing
	server, err := NewServer(auditLogger,
		WithHandler(router),
		WithListenAddress(serverAddress),
		WithSteadyStateRate(1),
		WithBurstRate(1),
	)
	require.NoError(t, err)

	// Start the server
	startServer(t, server)
	defer server.Close()

	client := http.DefaultClient
	err = waitForServer(client, serverAddress)
	require.NoError(t, err)

	// send quick requests to exceed the rate limit and assert that they fail with 429
	for i := 0; i < overLimitCallCount; i++ {
		res, err := client.Get("http://" + serverAddress)
		require.NoError(t, err)
		assert.Equal(t, http.StatusTooManyRequests, res.StatusCode)
	}
}

// Tests that the Write Timeout setting of a server created using NewServer() has effect.
func TestRequestWriteTimeout(t *testing.T) {
	// Setup a simple router with a simple hello world handler and a slow handler
	router := mux.NewRouter()
	router.HandleFunc("/", helloWorldHandler())
	router.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		// add fake delay to the handler
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello world")
	})

	serverAddress := "127.0.0.1:3599"

	// Setup the server with a low write timeout for testing
	server, err := NewServer(nil,
		WithHandler(router),
		WithListenAddress(serverAddress),
		WithWriteTimeout(50*time.Millisecond),
		WithSteadyStateRate(10),
		WithBurstRate(10),
	)
	require.NoError(t, err)

	// Start the server
	startServer(t, server)
	defer server.Close()

	client := http.DefaultClient
	err = waitForServer(client, serverAddress)
	require.NoError(t, err)

	// An EOF error is expected when write timeout is exceeded
	_, err = client.Get("http://" + serverAddress + "/slow")
	require.ErrorIs(t, err, io.EOF)
}

// Tests that the server initialized with NewServer() preserves the routes on the
// passed router instance.
func TestRoutes(t *testing.T) {
	// Setup a simple router with a couple of routes
	router := mux.NewRouter()
	router.HandleFunc("/", helloWorldHandler())
	router.HandleFunc("/products/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "id:"+mux.Vars(r)["id"])
	})

	// setup the server
	serverAddress := "127.0.0.1:3600"
	server, err := NewServer(nil,
		WithHandler(router),
		WithListenAddress(serverAddress),
		WithSteadyStateRate(10),
		WithBurstRate(10),
	)
	require.NoError(t, err)

	// Start the server
	startServer(t, server)
	defer server.Close()

	// wait for the server to come up
	client := http.DefaultClient
	err = waitForServer(client, serverAddress)
	require.NoError(t, err)

	// send a test request to /products/{id} endpoint and read the body
	res, err := client.Get("http://" + serverAddress + "/products/5")
	require.NoError(t, err)
	body, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)

	// assertions
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "id:5", string(body))
}

// Returns an HTTP handler that responds with "Hello world"
func helloWorldHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello world")
	}
}

// Waits for the server to come up. Checks if the server is up by sending
// repeated requests to it.
func waitForServer(client *http.Client, serverAddress string) error {
	var err error
	// wait for the server to come up
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		_, err = client.Get("http://" + serverAddress)
		if err == nil {
			return nil // server is up now
		}
	}
	return fmt.Errorf("timed out waiting for server %s to come up: %w", serverAddress, err)
}

// Starts the HTTP server in a new goroutine
func startServer(t *testing.T, server *http.Server) {
	go func() {
		err := server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			require.NoError(t, err)
		}
	}()
}
