//+build integration

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dockerClient dockerapi.DockerClient

func init() {
	dockerClient, _ = dockerapi.NewDockerGoClient(clientFactory, &cfg)
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

	// Create a container to get the container id.
	container, err := createGremlin(client)
	require.NoError(t, err, "creating container failed")
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

	containerChangeEventStream := eventStream("TestStatsEngineWithExistingContainersWithoutHealth")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil)
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
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	validateInstanceMetrics(t, engine)
	validateEmptyTaskHealthMetrics(t, engine)

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
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

	container, err := createGremlin(client)
	require.NoError(t, err, "creating container failed")
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	containerChangeEventStream := eventStream("TestStatsEngineWithNewContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil)
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

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

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
	validateInstanceMetrics(t, engine)
	validateEmptyTaskHealthMetrics(t, engine)

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
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

	// Create a container to get the container id.
	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

	containerChangeEventStream := eventStream("TestStatsEngineWithExistingContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil)
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
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	assert.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	// Verify the metrics of the container
	validateInstanceMetrics(t, engine)

	// Verify the health metrics of container
	validateTaskHealthMetrics(t, engine)

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
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

	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	containerChangeEventStream := eventStream("TestStatsEngineWithNewContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil)

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

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err = engine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

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
	validateInstanceMetrics(t, engine)
	// Verify the health metrics of container
	validateTaskHealthMetrics(t, engine)

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
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

func TestStatsEngineWithDockerTaskEngine(t *testing.T) {
	containerChangeEventStream := eventStream("TestStatsEngineWithDockerTaskEngine")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream,
		nil, dockerstate.NewTaskEngineState(), nil, nil)
	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")

	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})
	unmappedContainer, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    unmappedContainer.ID,
		Force: true,
	})
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
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err = statsEngine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer statsEngine.removeAll()
	defer statsEngine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

	err = client.StartContainer(unmappedContainer.ID, nil)
	require.NoError(t, err, "starting container failed")
	defer client.StopContainer(unmappedContainer.ID, defaultDockerTimeoutSeconds)

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
	validateInstanceMetrics(t, statsEngine)
	validateTaskHealthMetrics(t, statsEngine)

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
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
		nil, dockerstate.NewTaskEngineState(), nil, nil)

	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})
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
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err = statsEngine.MustInit(ctx, taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer statsEngine.removeAll()
	defer statsEngine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")

	err = containerChangeEventStream.WriteToEventStream(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerRunning,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)
	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	require.NoError(t, err, "stopping container failed")
	err = client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})
	require.NoError(t, err, "removing container failed")

	time.Sleep(checkPointSleep)

	// Simulate tcs client invoking GetInstanceMetrics.
	_, _, err = statsEngine.GetInstanceMetrics()
	assert.Error(t, err, "expect error 'no task metrics tp report' when getting instance metrics")

	// Should not contain any metrics after cleanup.
	validateIdleContainerMetrics(t, statsEngine)
	validateEmptyTaskHealthMetrics(t, statsEngine)
}
