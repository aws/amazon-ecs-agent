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

package stats

import (
	"context"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dockerClient dockerapi.DockerClient

func init() {
	dockerClient, _ = dockerapi.NewDockerGoClient(sdkClientFactory, &cfg, ctx)
}

func createRunningTask() *apitask.Task {
	return &apitask.Task{
		Arn:                 taskArn,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		Family:              taskDefinitionFamily,
		Version:             taskDefinitionVersion,
		Containers: []*apicontainer.Container{
			{
				Name: containerName,
			},
		},
	}
}

func TestStatsEngineWithExistingContainersWithoutHealth(t *testing.T) {
	// Create a new docker stats engine
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithExistingContainersWithoutHealth"))
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

	containerChangeEventStream := eventStream("TestStatsEngineWithExistingContainersWithoutHealth")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)
	testTask := createRunningTask()
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
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	validateInstanceMetrics(t, engine, false)
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

func TestStatsEngineWithNewContainersWithoutHealth(t *testing.T) {
	// Create a new docker stats engine
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithNewContainers"))
	defer engine.removeAll()

	// Assign ContainerStop timeout to addressable variable
	timeout := defaultDockerTimeoutSeconds

	container, err := createGremlin(client, "default")
	require.NoError(t, err, "creating container failed")
	defer client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	containerChangeEventStream := eventStream("TestStatsEngineWithNewContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)
	testTask := createRunningTask()
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

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")
	defer client.ContainerStop(ctx, container.ID, &timeout)

	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerRunning,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	validateInstanceMetrics(t, engine, false)
	validateEmptyTaskHealthMetrics(t, engine)

	err = client.ContainerStop(ctx, container.ID, &timeout)
	require.NoError(t, err, "stopping container failed")
	// Write the container change event to event stream
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

func TestStatsEngineWithExistingContainers(t *testing.T) {
	// Create a new docker stats engine
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithExistingContainers"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Create a container to get the container id.
	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})

	// Assign ContainerStop timeout to addressable variable
	timeout := defaultDockerTimeoutSeconds

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")
	defer client.ContainerStop(ctx, container.ID, &timeout)

	containerChangeEventStream := eventStream("TestStatsEngineWithExistingContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)
	testTask := createRunningTask()
	// enable container health check for this container
	testTask.Containers[0].HealthCheckType = "docker"
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&apicontainer.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  testTask.Containers[0],
		},
		testTask)

	// Simulate container start prior to listener initialization.
	time.Sleep(checkPointSleep)
	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	assert.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	// Verify the metrics of the container
	validateInstanceMetrics(t, engine, false)

	// Verify the health metrics of container
	validateTaskHealthMetrics(t, engine)

	err = client.ContainerStop(ctx, container.ID, &timeout)
	require.NoError(t, err, "stopping container failed")

	err = engine.containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerStopped,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "write to container change event stream failed")

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	validateIdleContainerMetrics(t, engine)
	validateEmptyTaskHealthMetrics(t, engine)
}

func TestStatsEngineWithNewContainers(t *testing.T) {
	// Create a new docker stats engine
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithNewContainers"))
	defer engine.removeAll()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Assign ContainerStop timeout to addressable variable
	timeout := defaultDockerTimeoutSeconds

	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	containerChangeEventStream := eventStream("TestStatsEngineWithNewContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)

	testTask := createRunningTask()
	// enable health check of the container
	testTask.Containers[0].HealthCheckType = "docker"
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&apicontainer.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  testTask.Containers[0],
		},
		testTask)

	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")
	defer client.ContainerStop(ctx, container.ID, &timeout)

	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerRunning,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	validateInstanceMetrics(t, engine, false)
	// Verify the health metrics of container
	validateTaskHealthMetrics(t, engine)

	err = client.ContainerStop(ctx, container.ID, &timeout)
	require.NoError(t, err, "stopping container failed")

	// Write the container change event to event stream
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

func TestStatsEngineWithNewContainersWithPolling(t *testing.T) {
	// additional config fields to use polling instead of stream
	cfg.PollMetrics = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.PollingMetricsWaitDuration = 1 * time.Second
	// Create a new docker client with new config
	dockerClientForNewContainersWithPolling, _ := dockerapi.NewDockerGoClient(sdkClientFactory, &cfg, ctx)
	// Create a new docker stats engine
	engine := NewDockerStatsEngine(&cfg, dockerClientForNewContainersWithPolling, eventStream("TestStatsEngineWithNewContainers"))
	defer engine.removeAll()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Assign ContainerStop timeout to addressable variable
	timeout := defaultDockerTimeoutSeconds

	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	containerChangeEventStream := eventStream("TestStatsEngineWithNewContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)

	testTask := createRunningTask()
	// enable health check of the container
	testTask.Containers[0].HealthCheckType = "docker"
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&apicontainer.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  testTask.Containers[0],
		},
		testTask)

	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")
	defer client.ContainerStop(ctx, container.ID, &timeout)

	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerRunning,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Wait for the stats collection go routine to start.
	time.Sleep(10 * time.Second)
	validateInstanceMetrics(t, engine, false)
	// Verify the health metrics of container
	validateTaskHealthMetrics(t, engine)

	err = client.ContainerStop(ctx, container.ID, &timeout)
	require.NoError(t, err, "stopping container failed")

	// Write the container change event to event stream
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

	// reset cfg, currently cfg is shared by all tests.
	cfg.PollMetrics = config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled}
	cfg.PollingMetricsWaitDuration = config.DefaultPollingMetricsWaitDuration
}

func TestStatsEngineWithDockerTaskEngine(t *testing.T) {
	containerChangeEventStream := eventStream("TestStatsEngineWithDockerTaskEngine")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)
	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	defer client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})
	unmappedContainer, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.ContainerRemove(ctx, unmappedContainer.ID, types.ContainerRemoveOptions{Force: true})
	testTask := createRunningTask()
	// enable the health check of the container
	testTask.Containers[0].HealthCheckType = "docker"
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&apicontainer.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  testTask.Containers[0],
		},
		testTask)

	// Create a new docker stats engine
	statsEngine := NewDockerStatsEngine(&cfg, dockerClient, containerChangeEventStream)
	err = statsEngine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer statsEngine.removeAll()
	defer statsEngine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Assign ContainerStop timeout to addressable variable
	timeout := defaultDockerTimeoutSeconds

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")
	defer client.ContainerStop(ctx, container.ID, &timeout)

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")
	defer client.ContainerStop(ctx, unmappedContainer.ID, &timeout)

	err = containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerRunning,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	err = containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerRunning,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: unmappedContainer.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	validateInstanceMetrics(t, statsEngine, false)
	validateTaskHealthMetrics(t, statsEngine)

	err = client.ContainerStop(ctx, container.ID, &timeout)
	require.NoError(t, err, "stopping container failed")

	err = containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerStopped,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	validateIdleContainerMetrics(t, statsEngine)
	validateEmptyTaskHealthMetrics(t, statsEngine)
}

func TestStatsEngineWithDockerTaskEngineMissingRemoveEvent(t *testing.T) {
	containerChangeEventStream := eventStream("TestStatsEngineWithDockerTaskEngineMissingRemoveEvent")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})
	testTask := createRunningTask()
	// enable container health check of this container
	testTask.Containers[0].HealthCheckType = "docker"
	testTask.Containers[0].KnownStatusUnsafe = apicontainerstatus.ContainerStopped

	// Populate Tasks and Container map in the engine.
	dockerTaskEngine := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&apicontainer.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  testTask.Containers[0],
		},
		testTask)

	// Create a new docker stats engine
	statsEngine := NewDockerStatsEngine(&cfg, dockerClient, containerChangeEventStream)
	err = statsEngine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer statsEngine.removeAll()
	defer statsEngine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")

	err = containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerRunning,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Assign ContainerStop timeout to addressable variable
	timeout := defaultDockerTimeoutSeconds

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	err = client.ContainerStop(ctx, container.ID, &timeout)
	require.NoError(t, err, "stopping container failed")
	err = client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})
	require.NoError(t, err, "removing container failed")

	time.Sleep(checkPointSleep)

	// Simulate tcs client invoking GetInstanceMetrics.
	_, _, err = statsEngine.GetInstanceMetrics(false)
	assert.Error(t, err, "expect error 'no task metrics tp report' when getting instance metrics")

	// Should not contain any metrics after cleanup.
	validateIdleContainerMetrics(t, statsEngine)
	validateEmptyTaskHealthMetrics(t, statsEngine)
}

func TestStatsEngineWithNetworkStatsDefaultMode(t *testing.T) {
	testNetworkModeStats(t, "default", false)
}

func testNetworkModeStats(t *testing.T, networkMode string, statsEmpty bool) {
	// Create a new docker stats engine
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithNetworkStats"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Assign ContainerStop timeout to addressable variable
	timeout := defaultDockerTimeoutSeconds

	// Create a container to get the container id.
	container, err := createGremlin(client, networkMode)
	require.NoError(t, err, "creating container failed")
	defer client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	require.NoError(t, err, "starting container failed")
	defer client.ContainerStop(ctx, container.ID, &timeout)

	containerChangeEventStream := eventStream("TestStatsEngineWithNetworkStats")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)
	testTask := createRunningTask()

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

	// Inspect the container and populate the container's network mode
	// This is done as part of Task Engine
	// https://github.com/aws/amazon-ecs-agent/blob/d2456beb048d36bfe18159ad7f35ca6b78bb9ee9/agent/engine/docker_task_engine.go#L364
	dockerContainer, err := client.ContainerInspect(ctx, container.ID)
	require.NoError(t, err, "inspecting container failed")
	netMode := string(dockerContainer.HostConfig.NetworkMode)
	testTask.Containers[0].SetNetworkMode(netMode)

	// Simulate container start prior to listener initialization.
	time.Sleep(checkPointSleep)
	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	_, taskMetrics, err := engine.GetInstanceMetrics(false)
	assert.NoError(t, err, "getting instance metrics failed")
	taskMetric := taskMetrics[0]
	for _, containerMetric := range taskMetric.ContainerMetrics {
		if statsEmpty {
			assert.Nil(t, containerMetric.NetworkStatsSet, "network stats should be empty for %s network mode", networkMode)
		} else {
			assert.NotNil(t, containerMetric.NetworkStatsSet, "network stats should be non-empty for %s network mode", networkMode)
		}
	}

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

func TestStorageStats(t *testing.T) {
	// Create a new docker stats engine
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithStorageStats"))
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

	containerChangeEventStream := eventStream("TestStatsEngineWithStorageStats")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil, nil)
	testTask := createRunningTask()

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

	// Inspect the container and populate the container's network mode
	dockerContainer, err := client.ContainerInspect(ctx, container.ID)
	require.NoError(t, err, "inspecting container failed")
	// Using default network mode
	netMode := string(dockerContainer.HostConfig.NetworkMode)
	testTask.Containers[0].SetNetworkMode(netMode)

	// Simulate container start prior to listener initialization.
	time.Sleep(checkPointSleep)
	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	_, taskMetrics, err := engine.GetInstanceMetrics(false)
	assert.NoError(t, err, "getting instance metrics failed")
	taskMetric := taskMetrics[0]
	for _, containerMetric := range taskMetric.ContainerMetrics {
		assert.NotNil(t, containerMetric.StorageStatsSet, "storage stats should be non-empty")
	}

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
