//go:build !windows && integration
// +build !windows,integration

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

package stats

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/docker/docker/api/types"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testStatsRestPath = "/get/stats"
	testStatsRestURL  = "http://testhost" + testStatsRestPath
)

const (
	stats = `# TYPE MetricFamily3 histogram
		MetricFamily3{dimensionX="value1", dimensionY="value2", le="0.5"} 1
		MetricFamily3{dimensionX="value1", dimensionY="value2", le="1"} 2
		MetricFamily3{dimensionX="value1", dimensionY="value2", le="5"} 3
		`
)

func TestStatsEngineWithNetworkStatsDifferentModes(t *testing.T) {
	var networkModes = []struct {
		NetworkMode string
		StatsEmpty  bool
	}{
		{"bridge", false},
		{"host", true},
		{"none", true},
	}
	for _, tc := range networkModes {
		testNetworkModeStats(t, tc.NetworkMode, tc.StatsEmpty)
	}
}

func TestStatsEngineWithServiceConnectMetrics(t *testing.T) {
	testUDSPath := filepath.Join(t.TempDir(), "test_stats_metrics.sock")

	// Create a new docker stats engine
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithServiceConnectMetrics"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Assign ContainerStop timeout to addressable variable
	timeout := defaultDockerTimeoutSeconds

	// Create a container to get the container id.
	container, err := createGremlin(client, "default")
	require.NoError(t, err, "creating container failed")
	defer client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")
	defer client.ContainerStop(ctx, container.ID, &timeout)

	containerChangeEventStream := eventStream("TestStatsEngineWithServiceConnectMetrics")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)
	testTask := createRunningTask()
	testTask.ServiceConnectConfig = &serviceconnect.Config{
		ContainerName: serviceConnectContainerName,
		RuntimeConfig: serviceconnect.RuntimeConfig{
			AdminSocketPath: testUDSPath,
			StatsRequest:    testStatsRestURL,
		},
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&apicontainer.DockerContainer{
			DockerID:   container.ID,
			DockerName: "gremlin",
			Container:  testTask.Containers[0],
		},
		testTask)

	// Simulate container start prior to listener initialization.
	time.Sleep(checkPointSleep)
	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	serviceConnectStats, err := newServiceConnectStats()
	require.NoError(t, err, "expected no error")
	engine.taskToServiceConnectStats[taskArn] = serviceConnectStats
	assert.Equal(t, 1, len(engine.taskToServiceConnectStats))

	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// simulate appnet server providing service connect metrics
	r := mux.NewRouter()
	r.HandleFunc(testStatsRestPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%v", stats)
	}))
	ts := httptest.NewUnstartedServer(r)
	l, err := net.Listen("unix", testUDSPath)
	require.NoError(t, err)

	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	defer ts.Close()

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	validateInstanceMetrics(t, engine, true)
	scStats := engine.taskToServiceConnectStats[taskArn]
	require.True(t, scStats.sent, "expected service connect metrics sent flag to be set")
	validateEmptyTaskHealthMetrics(t, engine)

	err = client.ContainerStop(ctx, container.ID, &timeout)
	require.NoError(t, err, "stopping container failed")

	err = engine.containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerStopped,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	validateIdleContainerMetrics(t, engine)
	validateEmptyTaskHealthMetrics(t, engine)
}
