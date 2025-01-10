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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mock_utils "github.com/aws/amazon-ecs-agent/agent/handlers/mocks"
	v1 "github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type ServerTestConfig struct {
	readTimeout  time.Duration
	writeTimeout time.Duration
	runtimeStats bool
}

const defaultResponse = `{"AvailableCommands":["/v1/metadata","/v1/tasks","/license"]}`

func TestServerSetup(t *testing.T) {
	var performMockRequest = func(config ServerTestConfig, path string) *httptest.ResponseRecorder {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStateResolver := mock_utils.NewMockDockerStateResolver(ctrl)

		server, _ := introspection.NewServer(&v1.AgentStateImpl{
			ContainerInstanceArn: aws.String("test_container_instance_arn"),
			ClusterName:          "test_cluster_arn",
			TaskEngine:           mockStateResolver,
		}, metrics.NewNopEntryFactory(),
			introspection.WithReadTimeout(config.readTimeout),
			introspection.WithWriteTimeout(config.writeTimeout),
			introspection.WithRuntimeStats(config.runtimeStats),
		)

		requestHandler := server.Handler

		recorder := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		requestHandler.ServeHTTP(recorder, req)
		return recorder
	}

	t.Run("test runtime stats disabled", func(t *testing.T) {
		recorder := performMockRequest(ServerTestConfig{
			readTimeout:  readTimeout,
			writeTimeout: writeTimeout,
			runtimeStats: false,
		}, "/")
		assert.Equal(t, defaultResponse, recorder.Body.String())
	})
	t.Run("test runtime stats enabled", func(t *testing.T) {
		recorder := performMockRequest(ServerTestConfig{
			readTimeout:  readTimeout,
			writeTimeout: writeTimeout,
			runtimeStats: true,
		}, "/")
		// The response should contain the pprof endpoints in addition
		// to the default endpoints.
		assert.Contains(t, recorder.Body.String(), "pprof")
	})
}
