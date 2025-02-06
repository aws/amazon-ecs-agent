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
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1/handlers"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeIntrospectionHTTPEndpoint(t *testing.T) {
	var serverAddress = fmt.Sprintf("http://localhost:%d", introspection.Port)
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

	go ServeIntrospectionHTTPEndpoint(context.Background(), aws.String("test_container_instance_arn"), &engine.DockerTaskEngine{}, &config.Config{Cluster: clusterName})

	client := http.DefaultClient
	err := waitForServer(client, serverAddress)
	require.NoError(t, err)

	response, err := client.Get(fmt.Sprintf("%s%s", serverAddress, handlers.V1AgentMetadataPath))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)

	bodyBytes, err := io.ReadAll(response.Body)
	assert.NoError(t, err)
	// Agent metadata response should contain the cluster
	assert.Contains(t, string(bodyBytes), clusterName)
}
