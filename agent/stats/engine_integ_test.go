// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	docker "github.com/fsouza/go-dockerclient"

	"golang.org/x/net/context"
)

const (
	testImageName = "amazon/amazon-ecs-gremlin:make"

	// defaultDockerTimeoutSeconds is the timeout for dialing the docker remote API.
	defaultDockerTimeoutSeconds uint = 10

	// waitForCleanupSleep is the sleep duration in milliseconds
	// for the waiting after container cleanup before checking the state of the manager.
	waitForCleanupSleep = 10 * time.Millisecond

	taskArn               = "gremlin"
	taskDefinitionFamily  = "docker-gremlin"
	taskDefinitionVersion = "1"
	containerName         = "gremlin-container"
)

var endpoint = utils.DefaultIfBlank(os.Getenv(ecsengine.DOCKER_ENDPOINT_ENV_VARIABLE), ecsengine.DOCKER_DEFAULT_ENDPOINT)

var client, _ = docker.NewClient(endpoint)
var clientFactory = dockerclient.NewFactory(endpoint)
var cfg = config.DefaultConfig()

var dockerClient ecsengine.DockerClient

func init() {
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte{})
	dockerClient, _ = ecsengine.NewDockerGoClient(clientFactory, false, &cfg)
}

// eventStream returns the event stream used to receive container change events
func eventStream(name string) *eventstream.EventStream {
	eventStream := eventstream.NewEventStream(name, context.Background())
	eventStream.StartListening()
	return eventStream
}

// createGremlin creates the gremlin container using the docker client.
// It is used only in the test code.
func createGremlin(client *docker.Client) (*docker.Container, error) {
	container, err := client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: testImageName,
		},
	})

	return container, err
}

type IntegContainerMetadataResolver struct {
	containerIDToTask map[string]*api.Task
	containerIDToName map[string]string
}

func newIntegContainerMetadataResolver() *IntegContainerMetadataResolver {
	resolver := IntegContainerMetadataResolver{
		containerIDToTask: make(map[string]*api.Task),
		containerIDToName: make(map[string]string),
	}

	return &resolver
}

func (resolver *IntegContainerMetadataResolver) ResolveTask(containerID string) (*api.Task, error) {
	task, exists := resolver.containerIDToTask[containerID]
	if !exists {
		return nil, fmt.Errorf("unmapped container")
	}

	return task, nil
}

func (resolver *IntegContainerMetadataResolver) ResolveName(dockerID string) (string, error) {
	name, exists := resolver.containerIDToName[dockerID]
	if !exists {
		return "", fmt.Errorf("unmapped container")
	}

	return name, nil
}

func (resolver *IntegContainerMetadataResolver) addToMap(containerID string) {
	resolver.containerIDToTask[containerID] = &api.Task{Arn: taskArn, Family: taskDefinitionFamily, Version: taskDefinitionVersion}
	resolver.containerIDToName[containerID] = containerName
}

func TestStatsEngineWithExistingContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}

	// Create a new docker stats engine
	// TODO make dockerStatsEngine not a singleton object
	dockerStatsEngine = nil
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithExistingContainers"))

	// Create a container to get the container id.
	container, err := createGremlin(client)
	if err != nil {
		t.Fatalf("Error creating container: %v", err)
	}
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})
	resolver := newIntegContainerMetadataResolver()
	// Initialize mock interface so that task id is resolved only for the container
	// that was launched during the test.
	resolver.addToMap(container.ID)

	// Wait for containers from previous tests to transition states.
	time.Sleep(checkPointSleep)

	engine.resolver = resolver
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = client.StartContainer(container.ID, nil)
	if err != nil {
		t.Errorf("Error starting container: %s, err: %v", container.ID, err)
	}
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerId: container.ID,
		},
	})
	if err != nil {
		t.Errorf("Failed to write to container change event stream, err: %v", err)
	}

	// Simulate container start prior to listener initialization.
	time.Sleep(checkPointSleep)
	err = engine.Init()
	if err != nil {
		t.Errorf("Error initializing stats engine: %v", err)
	}
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	if err != nil {
		t.Errorf("Error gettting instance metrics: %v", err)
	}
	err = validateMetricsMetadata(metadata)
	if err != nil {
		t.Errorf("Error validating metadata: %v", err)
	}

	if len(taskMetrics) != 1 {
		t.Fatalf("Incorrect number of tasks. Expected: 1, got: %d", len(taskMetrics))
	}

	taskMetric := taskMetrics[0]
	if *taskMetric.TaskDefinitionFamily != taskDefinitionFamily {
		t.Errorf("Excpected task definition family to be: %s, got: %s", taskDefinitionFamily, *taskMetric.TaskDefinitionFamily)
	}
	if *taskMetric.TaskDefinitionVersion != taskDefinitionVersion {
		t.Errorf("Excpected task definition family to be: %s, got: %s", taskDefinitionVersion, *taskMetric.TaskDefinitionVersion)
	}
	err = validateContainerMetrics(taskMetric.ContainerMetrics, 1)
	if err != nil {
		t.Errorf("Error validating container metrics: %v", err)
	}

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Errorf("Error stopping container: %s, err: %v", container.ID, err)
	}

	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerId: container.ID,
		},
	})
	if err != nil {
		t.Errorf("Failed to write to container change event stream, err: %v", err)
	}

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(engine)
	if err != nil {
		t.Fatalf("Error validating idle metrics: %v", err)
	}
}

func TestStatsEngineWithNewContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}

	// Create a new docker stats engine
	// TODO make dockerStatsEngine not a singleton object
	dockerStatsEngine = nil
	engine := NewDockerStatsEngine(&cfg, dockerClient, eventStream("TestStatsEngineWithNewContainers"))
	defer engine.removeAll()

	container, err := createGremlin(client)
	if err != nil {
		t.Fatalf("Error creating container: %v", err)
	}
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})

	resolver := newIntegContainerMetadataResolver()
	// Initialize mock interface so that task id is resolved only for the container
	// that was launched during the test.
	resolver.addToMap(container.ID)

	// Wait for containers from previous tests to transition states.
	time.Sleep(checkPointSleep * 2)
	engine.resolver = resolver
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance

	err = engine.Init()
	if err != nil {
		t.Errorf("Error initializing stats engine: %v", err)
	}
	defer engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Errorf("Error starting container: %s, err: %v", container.ID, err)
	}

	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerId: container.ID,
		},
	})
	if err != nil {
		t.Errorf("Failed to write to container change event stream, err: %v", err)
	}

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	if err != nil {
		t.Errorf("Error gettting instance metrics: %v", err)
	}

	err = validateMetricsMetadata(metadata)
	if err != nil {
		t.Errorf("Error validating metadata: %v", err)
	}

	if len(taskMetrics) != 1 {
		t.Fatalf("Incorrect number of tasks. Expected: 1, got: %d", len(taskMetrics))
	}
	taskMetric := taskMetrics[0]
	if *taskMetric.TaskDefinitionFamily != taskDefinitionFamily {
		t.Errorf("Excpected task definition family to be: %s, got: %s", taskDefinitionFamily, *taskMetric.TaskDefinitionFamily)
	}
	if *taskMetric.TaskDefinitionVersion != taskDefinitionVersion {
		t.Errorf("Excpected task definition family to be: %s, got: %s", taskDefinitionVersion, *taskMetric.TaskDefinitionVersion)
	}

	err = validateContainerMetrics(taskMetric.ContainerMetrics, 1)
	if err != nil {
		t.Errorf("Error validating container metrics: %v", err)
	}

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Errorf("Error stopping container: %s, err: %v", container.ID, err)
	}
	// Write the container change event to event stream
	err = engine.containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerId: container.ID,
		},
	})
	if err != nil {
		t.Errorf("Failed to write to container change event stream, err: %v", err)
	}

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(engine)
	if err != nil {
		t.Fatalf("Error validating idle metrics: %v", err)
	}
}

func TestStatsEngineWithDockerTaskEngine(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}

	containerChangeEventStream := eventStream("TestStatsEngineWithDockerTaskEngine")
	taskEngine := ecsengine.NewTaskEngine(&config.Config{}, nil, nil, containerChangeEventStream, nil, dockerstate.NewDockerTaskEngineState())
	container, err := createGremlin(client)
	if err != nil {
		t.Fatalf("Error creating container: %v", err)
	}
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})
	unmappedContainer, err := createGremlin(client)
	if err != nil {
		t.Fatalf("Error creating container: %v", err)
	}
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    unmappedContainer.ID,
		Force: true,
	})
	containers := []*api.Container{
		&api.Container{
			Name: "gremlin",
		},
	}
	testTask := api.Task{
		Arn:           "gremlin-task",
		DesiredStatus: api.TaskRunning,
		KnownStatus:   api.TaskRunning,
		Family:        "test",
		Version:       "1",
		Containers:    containers,
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*ecsengine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(&testTask)
	dockerTaskEngine.State().AddContainer(
		&api.DockerContainer{
			DockerId:   container.ID,
			DockerName: "gremlin",
			Container:  containers[0],
		},
		&testTask)

	// Create a new docker stats engine
	// TODO make dockerStatsEngine not a singleton object
	dockerStatsEngine = nil
	statsEngine := NewDockerStatsEngine(&cfg, dockerClient, containerChangeEventStream)
	err = statsEngine.MustInit(taskEngine, defaultCluster, defaultContainerInstance)
	if err != nil {
		t.Errorf("Error initializing stats engine: %v", err)
	}
	defer statsEngine.removeAll()
	defer statsEngine.containerChangeEventStream.Unsubscribe(containerChangeHandler)

	err = client.StartContainer(container.ID, nil)
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Errorf("Error starting container: %s, err: %v", container.ID, err)
	}

	err = client.StartContainer(unmappedContainer.ID, nil)
	defer client.StopContainer(unmappedContainer.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Errorf("Error starting container: %s, err: %v", unmappedContainer.ID, err)
	}

	err = containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerId: container.ID,
		},
	})
	if err != nil {
		t.Errorf("Failed to write to container change event stream err: %v", err)
	}

	err = containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerId: unmappedContainer.ID,
		},
	})
	if err != nil {
		t.Errorf("Failed to write to container change event stream, err: %v", err)
	}

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := statsEngine.GetInstanceMetrics()
	if err != nil {
		t.Errorf("Error gettting instance metrics: %v", err)
	}

	if len(taskMetrics) != 1 {
		t.Errorf("Incorrect number of tasks. Expected: 1, got: %d", len(taskMetrics))
	}
	err = validateMetricsMetadata(metadata)
	if err != nil {
		t.Errorf("Error validating metadata: %v", err)
	}

	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 1)
	if err != nil {
		t.Errorf("Error validating container metrics: %v", err)
	}

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Errorf("Error stopping container: %s, err: %v", container.ID, err)
	}
	err = containerChangeEventStream.WriteToEventStream(ecsengine.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: ecsengine.DockerContainerMetadata{
			DockerId: container.ID,
		},
	})
	if err != nil {
		t.Errorf("Failed to write to container change event stream, err: %v", err)
	}

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(statsEngine)
	if err != nil {
		t.Fatalf("Error validating idle metrics: %v", err)
	}
}
