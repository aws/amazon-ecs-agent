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

// Package handlers deals with the agent introspection api.
package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	v1 "github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/cihub/seelog"
)

// ServeIntrospectionHTTPEndpoint serves information about this agent/containerInstance and tasks running on it.
func ServeIntrospectionHTTPEndpoint(ctx context.Context, containerInstanceArn *string, taskEngine engine.TaskEngine, cfg *config.Config) {
	// Is this the right level to type assert, assuming we'd abstract multiple taskengines here?
	// Revisit if we ever add another type..
	dockerTaskEngine := taskEngine.(*engine.DockerTaskEngine)
	agentState := &v1.AgentStateImpl{
		ContainerInstanceArn: containerInstanceArn,
		ClusterName:          cfg.Cluster,
		TaskEngine:           dockerTaskEngine,
	}

	server, err := introspection.NewServer(
		agentState,
		metrics.NewNopEntryFactory(),
		introspection.WithReadTimeout(readTimeout),
		introspection.WithWriteTimeout(writeTimeout),
		introspection.WithRuntimeStats(cfg.EnableRuntimeStats.Enabled()),
	)

	if err != nil {
		seelog.Criticalf("Failed to set up Introspection Server: %v", err)
		return
	}

	go func() {
		<-ctx.Done()
		if err := server.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout:
			seelog.Infof("HTTP server Shutdown: %v", err)
		}
	}()

	for {
		retry.RetryWithBackoff(retry.NewExponentialBackoff(time.Second, time.Minute, 0.2, 2), func() error {
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				seelog.Errorf("Error running introspection endpoint: %v", err)
				return err
			}
			// server was cleanly closed via context
			return nil
		})
	}
}
