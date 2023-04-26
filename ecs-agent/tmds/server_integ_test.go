//go:build integration
// +build integration

// copyright amazon.com inc. or its affiliates. all rights reserved.
//
// licensed under the apache license, version 2.0 (the "license"). you may
// not use this file except in compliance with the license. a copy of the
// license is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. this file is distributed
// on an "as is" basis, without warranties or conditions of any kind, either
// express or implied. see the license for the specific language governing
// permissions and limitations under the license.
package tmds

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/audit/mocks"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that the server returned by NewServer() has request rate limiter set up
func TestRequestRateLimiter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup a simple router with a simple handler
	router := mux.NewRouter()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello world")
	})

	overLimitCallCount := 5

	// Setup a mock audit logger, audit logger is used for logging over limit calls
	auditLogger := mock_audit.NewMockAuditLogger(ctrl)
	auditLogger.EXPECT().
		Log(gomock.Any(), http.StatusTooManyRequests, "").
		Times(overLimitCallCount).
		Return()

	serverAddress := "127.0.0.1:3598"

	// Setup the server with a low rate limit for testing
	server, err := NewServer(auditLogger,
		WithRouter(router),
		WithListenAddress(serverAddress),
		WithSteadyStateRate(1),
		WithBurstRate(1),
	)
	require.NoError(t, err)

	// Start the server
	go func() {
		err := server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			require.NoError(t, err)
		}
	}()
	defer server.Close()

	client := http.DefaultClient

	// wait for the server to come up
	for i := 0; i < 5; i++ {
		_, err = client.Get("http://" + serverAddress)
		if err == nil {
			break // server is up now
		}
		time.Sleep(1 * time.Second)
	}

	// send quick requests to exceed the rate limit and assert that they fail with 429
	for i := 0; i < overLimitCallCount; i++ {
		res, err := client.Get("http://" + serverAddress)
		require.NoError(t, err)
		assert.Equal(t, http.StatusTooManyRequests, res.StatusCode)
	}
}
