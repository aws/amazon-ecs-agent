//+build !windows,integration
// Disabled on Windows until Stats are actually supported
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
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"

	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dockerClient ecsengine.DockerClient

func init() {
	dockerClient, _ = ecsengine.NewDockerGoClient(clientFactory, &cfg)
}

func (resolver *IntegContainerMetadataResolver) addToMap(containerID string) {
	resolver.containerIDToTask[containerID] = &api.Task{
		Arn:     taskArn,
		Family:  taskDefinitionFamily,
		Version: taskDefinitionVersion,
	}
	resolver.containerIDToDockerContainer[containerID] = &api.DockerContainer{
		DockerID:  containerID,
		Container: &api.Container{},
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

	// Wait for containers from previous tests to transition states.
	time.Sleep(checkPointSleep)

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

	containerChangeEventStream := eventStream("TestStatsEngineWithExistingContainersWithoutHealth")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream, nil, dockerstate.NewTaskEngineState(), nil)
	containers := []*api.Container{
		{
			Name: "gremlin",
		},
	}
	testTask := &api.Task{
		Arn:                 "gremlin-task",
		DesiredStatusUnsafe: api.TaskRunning,
		KnownStatusUnsafe:   api.TaskRunning,
		Family:              "docker-gremlin",
		Version:             "1",
		Containers:          containers,
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&api.DockerContainer{
			DockerID:   container.ID,
			DockerName: "gremlin",
			Container:  containers[0],
		},
		testTask)

	// Simulate container start prior to listener initialization.
	time.Sleep(checkPointSleep)
	err = engine.MustInit(taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	assert.NoError(t, err, "gettting instance metrics failed")
	err = validateMetricsMetadata(metadata)
	assert.NoError(t, validateMetricsMetadata(metadata), "validating metadata failed")
	assert.Len(t, taskMetrics, 1, "incorrect number of tasks")

	taskMetric := taskMetrics[0]
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionFamily), taskDefinitionFamily, "unexpected task definition family")
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionVersion), taskDefinitionVersion, "unexcpected task definition version")
	assert.NoError(t, validateContainerMetrics(taskMetric.ContainerMetrics, 1), "validating container metrics failed")

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	assert.NoError(t, err, "stopping container failed")

	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	assert.NoError(t, validateIdleContainerMetrics(engine), "validating idle metrics failed")
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

	// Wait for containers from previous tests to transition states.
	time.Sleep(checkPointSleep * 2)
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	containerChangeEventStream := eventStream("TestStatsEngineWithNewContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream, nil, dockerstate.NewTaskEngineState(), nil)
	containers := []*api.Container{
		{
			Name: "gremlin",
		},
	}
	testTask := &api.Task{
		Arn:                 "gremlin-task",
		DesiredStatusUnsafe: api.TaskRunning,
		KnownStatusUnsafe:   api.TaskRunning,
		Family:              "docker-gremlin",
		Version:             "1",
		Containers:          containers,
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&api.DockerContainer{
			DockerID:   container.ID,
			DockerName: "gremlin",
			Container:  containers[0],
		},
		testTask)

	err = engine.MustInit(taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	require.NoError(t, err, "starting container failed")

	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	assert.NoError(t, err, "gettting instance metrics failed")

	assert.NoError(t, validateMetricsMetadata(metadata), "validating metadata failed")
	assert.Len(t, taskMetrics, 1, "incorrect number of tasks")
	taskMetric := taskMetrics[0]
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionFamily), taskDefinitionFamily, "unexcpected task definition family")
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionVersion), taskDefinitionVersion, "unexcpected task definition version")

	assert.NoError(t, validateContainerMetrics(taskMetric.ContainerMetrics, 1), "validating container metrics failed")

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	assert.NoError(t, err, "stopping container failed")
	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	assert.NoError(t, validateIdleContainerMetrics(engine), "validating idle metrics failed")
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

	// Wait for containers from previous tests to transition states.
	time.Sleep(checkPointSleep)

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

	containerChangeEventStream := eventStream("TestStatsEngineWithExistingContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream, nil, dockerstate.NewTaskEngineState(), nil)
	containers := []*api.Container{
		{
			Name:            "container-health",
			HealthCheckType: "docker",
		},
	}
	testTask := &api.Task{
		Arn:                 "container-health-task",
		DesiredStatusUnsafe: api.TaskRunning,
		KnownStatusUnsafe:   api.TaskRunning,
		Family:              "docker-container-health",
		Version:             "1",
		Containers:          containers,
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&api.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  containers[0],
		},
		testTask)

	// Simulate container start prior to listener initialization.
	time.Sleep(checkPointSleep)
	err = engine.MustInit(taskEngine, defaultCluster, defaultContainerInstance)
	assert.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	// Verify the metrics of the container
	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	assert.NoError(t, err, "error gettting instance metrics")
	assert.NoError(t, validateMetricsMetadata(metadata), "error validating metadata")
	assert.Len(t, taskMetrics, 1, "incorrect number of tasks")

	taskMetric := taskMetrics[0]
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionFamily), "docker-container-health", "task definition family not expected")
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionVersion), taskDefinitionVersion, "task definition family version not expected")
	assert.NoError(t, validateContainerMetrics(taskMetric.ContainerMetrics, 1), "error validating container metrics")

	// Verify the health metrics of container
	healthMetadata, healthMetrics, err := engine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health metrics failed")
	require.Len(t, healthMetrics, 1)
	assert.NoError(t, validateHealthMetricsMetadata(healthMetadata))
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskArn), testTask.Arn, "task arn not expected")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskDefinitionFamily), "docker-container-health", "task definition family not expected")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskDefinitionVersion), taskDefinitionVersion, "task definition version not expected")
	assert.NoError(t, validateTaskHealth(healthMetrics[0].Containers, 1))

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	assert.NoError(t, err, "stopping container failed")

	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "write to container change event stream failed")

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(engine)
	assert.NoError(t, err, "validating idle metrics failed")

	_, healthMetrics, err = engine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health failed")
	assert.Len(t, healthMetrics, 0, "no health metrics was expected")
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

	// Wait for containers from previous tests to transition states.
	time.Sleep(checkPointSleep * 2)
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	containerChangeEventStream := eventStream("TestStatsEngineWithNewContainers")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream, nil, dockerstate.NewTaskEngineState(), nil)
	containers := []*api.Container{
		{
			Name:            "container-health",
			HealthCheckType: "docker",
		},
	}
	testTask := &api.Task{
		Arn:                 "container-health-task",
		DesiredStatusUnsafe: api.TaskRunning,
		KnownStatusUnsafe:   api.TaskRunning,
		Family:              "docker-container-health",
		Version:             "1",
		Containers:          containers,
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(testTask)
	dockerTaskEngine.State().AddContainer(
		&api.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  containers[0],
		},
		testTask)

	err = engine.MustInit(taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	require.NoError(t, err, "starting container failed")

	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	assert.NoError(t, err, "gettting instance metrics failed")

	err = validateMetricsMetadata(metadata)
	assert.NoError(t, err, "validating metadata failed")

	assert.Len(t, taskMetrics, 1)
	taskMetric := taskMetrics[0]
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionFamily), "docker-container-health", "task defnition family not expected")
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionVersion), taskDefinitionVersion, "task definition version not expected")

	err = validateContainerMetrics(taskMetric.ContainerMetrics, 1)
	assert.NoError(t, err, "validating container metrics failed")

	// Verify the health metrics of container
	healthMetadata, healthMetrics, err := engine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health metrics failed")
	require.Len(t, healthMetrics, 1)
	assert.NoError(t, validateHealthMetricsMetadata(healthMetadata))
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskArn), testTask.Arn, "task arn not expected")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskDefinitionFamily), "docker-container-health", "task definition family not expected")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskDefinitionVersion), taskDefinitionVersion, "task definition version not expected")
	assert.NoError(t, validateTaskHealth(healthMetrics[0].Containers, 1))

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	assert.NoError(t, err, "stopping container failed")

	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(engine)
	assert.NoError(t, err, "validating idle metrics failed")

	_, healthMetrics, err = engine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health failed")
	assert.Len(t, healthMetrics, 0, "no health metrics was expected")
}

func TestStatsEngineWithDockerTaskEngine(t *testing.T) {
	containerChangeEventStream := eventStream("TestStatsEngineWithDockerTaskEngine")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream, nil, dockerstate.NewTaskEngineState(), nil)
	container, err := createHealthContainer(client)
	assert.NoError(t, err, "creating container failed")

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
	containers := []*api.Container{
		{
			Name:            "container-health",
			HealthCheckType: "docker",
		},
	}
	testTask := api.Task{
		Arn:                 "container-health-task",
		DesiredStatusUnsafe: api.TaskRunning,
		KnownStatusUnsafe:   api.TaskRunning,
		Family:              "test",
		Version:             "1",
		Containers:          containers,
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(&testTask)
	dockerTaskEngine.State().AddContainer(
		&api.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  containers[0],
		},
		&testTask)

	// Create a new docker stats engine
	statsEngine := NewDockerStatsEngine(&cfg, dockerClient, containerChangeEventStream)
	err = statsEngine.MustInit(taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer statsEngine.removeAll()
	defer statsEngine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	require.NoError(t, err, "starting container failed")

	err = client.StartContainer(unmappedContainer.ID, nil)
	defer client.StopContainer(unmappedContainer.ID, defaultDockerTimeoutSeconds)
	require.NoError(t, err, "starting container failed")

	err = containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	err = containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: unmappedContainer.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := statsEngine.GetInstanceMetrics()
	assert.NoError(t, err, "gettting instance metrics failed")
	assert.Len(t, taskMetrics, 1, "incorrect number of tasks")
	assert.NoError(t, validateMetricsMetadata(metadata), "validating metadata failed")
	assert.NoError(t, validateContainerMetrics(taskMetrics[0].ContainerMetrics, 1), "validating container metrics failed")

	// Verify the health metrics of container
	healthMetadata, healthMetrics, err := statsEngine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health metrics failed")
	require.Len(t, healthMetrics, 1)
	assert.NoError(t, validateHealthMetricsMetadata(healthMetadata))
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskArn), testTask.Arn, "task arn not expected")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskDefinitionFamily), "test", "task definition family not expected")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskDefinitionVersion), taskDefinitionVersion, "task definition version not expected")
	assert.NoError(t, validateTaskHealth(healthMetrics[0].Containers, 1))

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	assert.NoError(t, err, "stopping container failed")

	err = containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerID: container.ID,
		},
	})
	assert.NoError(t, err, "failed to write to container change event stream")

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(statsEngine)
	assert.NoError(t, err, "validating idle metrics failed")

	_, healthMetrics, err = statsEngine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health failed")
	assert.Len(t, healthMetrics, 0, "no health metrics was expected")
}

func TestStatsEngineWithDockerTaskEngineMissingRemoveEvent(t *testing.T) {
	containerChangeEventStream := eventStream("TestStatsEngineWithDockerTaskEngineMissingRemoveEvent")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream, nil, dockerstate.NewTaskEngineState(), nil)

	container, err := createHealthContainer(client)
	require.NoError(t, err, "creating container failed")
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})
	containers := []*api.Container{
		{
			Name:              "container-health",
			HealthCheckType:   "docker",
			KnownStatusUnsafe: api.ContainerStopped,
		},
	}
	testTask := api.Task{
		Arn:                 "container-health-task",
		DesiredStatusUnsafe: api.TaskRunning,
		KnownStatusUnsafe:   api.TaskRunning,
		Family:              "test",
		Version:             "1",
		Containers:          containers,
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(&testTask)
	dockerTaskEngine.State().AddContainer(
		&api.DockerContainer{
			DockerID:   container.ID,
			DockerName: "container-health",
			Container:  containers[0],
		},
		&testTask)

	// Create a new docker stats engine
	statsEngine := NewDockerStatsEngine(&cfg, dockerClient, containerChangeEventStream)
	err = statsEngine.MustInit(taskEngine, defaultCluster, defaultContainerInstance)
	require.NoError(t, err, "initializing stats engine failed")
	defer statsEngine.removeAll()
	defer statsEngine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	require.NoError(t, err, "starting container failed")

	err = containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
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
	assert.NoError(t, err, "removing container failed")

	time.Sleep(checkPointSleep)

	// Simulate tcs client invoking GetInstanceMetrics.
	_, _, err = statsEngine.GetInstanceMetrics()
	assert.Error(t, err, "expect error 'no task metrics tp report' when getting instance metrics")

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(statsEngine)
	assert.NoError(t, err, "validating idle metrics failed")

	_, healthMetrics, err := statsEngine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health failed")
	assert.Len(t, healthMetrics, 0, "no health metrics was expected")
}
