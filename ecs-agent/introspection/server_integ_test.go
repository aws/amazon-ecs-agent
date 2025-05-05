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
package introspection

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1/handlers"
	mock_v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verify that the connection closes as expected and a metric is emitted if a panic occurs.
func TestPanicHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAgentState := mock_v1.NewMockAgentState(ctrl)
	mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

	mockEntry := mock_metrics.NewMockEntry(ctrl)
	mockEntry.EXPECT().Done(fmt.Errorf("panic!"))
	mockMetricsFactory.EXPECT().
		New(metrics.IntrospectionCrash).Return(mockEntry)

	mockAgentState.EXPECT().
		GetAgentMetadata().
		DoAndReturn(func() (interface{}, error) {
			panic("panic!")
		})

	response, err := performRequest(t, mockAgentState, mockMetricsFactory, handlers.V1AgentMetadataPath)
	require.NoError(t, err)

	bodyBytes, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
	assert.Equal(t, "", string(bodyBytes))
}

func TestWriteTimeout(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAgentState := mock_v1.NewMockAgentState(ctrl)
		mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

		testAgentMetadata := &v1.AgentMetadataResponse{
			Cluster:              "cluster",
			ContainerInstanceArn: aws.String("some/arn"),
			Version:              "1.0.0",
		}

		mockAgentState.EXPECT().
			GetAgentMetadata().
			DoAndReturn(func() (*v1.AgentMetadataResponse, error) {
				time.Sleep(100 * time.Millisecond)
				return testAgentMetadata, nil
			})

		response, err := performRequest(t, mockAgentState, mockMetricsFactory, handlers.V1AgentMetadataPath, WithWriteTimeout(time.Millisecond*150))
		require.NoError(t, err)

		bodyBytes, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		testAgentMetadataJSON, _ := json.Marshal(testAgentMetadata)

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, testAgentMetadataJSON, bodyBytes)
	})

	t.Run("timeout exceeded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAgentState := mock_v1.NewMockAgentState(ctrl)
		mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

		testAgentMetadata := &v1.AgentMetadataResponse{
			Cluster:              "cluster",
			ContainerInstanceArn: aws.String("some/arn"),
			Version:              "1.0.0",
		}

		mockAgentState.EXPECT().
			GetAgentMetadata().
			DoAndReturn(func() (*v1.AgentMetadataResponse, error) {
				time.Sleep(100 * time.Millisecond)
				return testAgentMetadata, nil
			})

		_, err := performRequest(t, mockAgentState, mockMetricsFactory, handlers.V1AgentMetadataPath, WithWriteTimeout(time.Millisecond*50))

		// An EOF error is expected when write timeout is exceeded
		require.ErrorIs(t, err, io.EOF)
	})
}

// Test that profiling endpoints are only available when explicitly enabled by passing WithRuntimeStats(true).
func TestPprof(t *testing.T) {
	t.Run("pprof enabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAgentState := mock_v1.NewMockAgentState(ctrl)
		mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

		response, err := performRequest(t, mockAgentState, mockMetricsFactory, "/", WithRuntimeStats(true))
		require.NoError(t, err)

		bodyBytes, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Contains(t, string(bodyBytes), "/pprof")
	})
	t.Run("pprof disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAgentState := mock_v1.NewMockAgentState(ctrl)
		mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

		response, err := performRequest(t, mockAgentState, mockMetricsFactory, "/", WithRuntimeStats(false))
		require.NoError(t, err)

		bodyBytes, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.NotContains(t, string(bodyBytes), "/pprof")
	})
	t.Run("default - pprof disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		response, err := performRequest(t, mock_v1.NewMockAgentState(ctrl), mock_metrics.NewMockEntryFactory(ctrl), "/")
		require.NoError(t, err)

		bodyBytes, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.NotContains(t, string(bodyBytes), "/pprof")
	})
}

func performRequest(t *testing.T, state v1.AgentState, metricsFactory metrics.EntryFactory, path string, options ...ConfigOpt) (*http.Response, error) {
	var waitForServer = func(client *http.Client, serverAddress string) error {
		var err error
		// wait for the server to come up
		for i := 0; i < 10; i++ {
			time.Sleep(100 * time.Millisecond)
			_, err = client.Get(serverAddress)
			if err == nil {
				return nil // server is up now
			}
		}
		return fmt.Errorf("timed out waiting for server %s to come up: %w", serverAddress, err)
	}
	var startServer = func(t *testing.T, server *http.Server) int {
		server.Addr = ":0"
		listener, err := net.Listen("tcp", server.Addr)
		assert.NoError(t, err)

		port := listener.Addr().(*net.TCPAddr).Port
		t.Logf("Server started on port: %d", port)

		go func() {
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				t.Logf("ListenAndServe(): %s\n", err)
			}
		}()

		return port
	}

	server, err := NewServer(state, metricsFactory, options...)

	port := startServer(t, server)
	defer server.Close()

	serverAddress := fmt.Sprintf("http://localhost:%d", port)

	client := http.DefaultClient
	err = waitForServer(client, serverAddress)
	assert.NoError(t, err)

	return client.Get(fmt.Sprintf("%s%s", serverAddress, path))
}
