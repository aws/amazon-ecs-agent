// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	docker "github.com/fsouza/go-dockerclient"
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

var endpoint = utils.DefaultIfBlank(os.Getenv(engine.DOCKER_ENDPOINT_ENV_VARIABLE), engine.DOCKER_DEFAULT_ENDPOINT)

var client, _ = docker.NewClient(endpoint)

var cfg = config.DefaultConfig()

func init() {
	// Set DockerGraphPath as per changes in 1.6
	cfg.DockerGraphPath = "/var/run/docker"
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
	// This should be a functional test. Upgrading to docker 1.6 breaks our ability to
	// read state.json file for containers.
	t.Skip("Skipping integ test as this is really a functional test")
	engine := NewDockerStatsEngine(&cfg)
	err := engine.initDockerClient()
	if err != nil {
		t.Error("Error initializing stats engine: ", err)
	}

	// Create a container to get the container id.
	container, err := createGremlin(client)
	if err != nil {
		t.Fatal("Error creating container", err)
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
		t.Error("Error starting container: ", container.ID, " error: ", err)
	}
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)

	// Simulate container start prior to listener initialization.
	time.Sleep(checkPointSleep)
	err = engine.Init()
	if err != nil {
		t.Error("Error initializing stats engine: ", err)
	}

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	if err != nil {
		t.Error("Error gettting instance metrics: ", err)
	}
	err = validateMetricsMetadata(metadata)
	if err != nil {
		t.Error("Error validating metadata: ", err)
	}

	if len(taskMetrics) != 1 {
		t.Error("Incorrect number of tasks. Expected: 1, got: ", len(taskMetrics))
	}

	taskMetric := taskMetrics[0]
	if *taskMetric.TaskDefinitionFamily != taskDefinitionFamily {
		t.Error("Excpected task definition family to be: ", taskDefinitionFamily, " got: ", *taskMetric.TaskDefinitionFamily)
	}
	if *taskMetric.TaskDefinitionVersion != taskDefinitionVersion {
		t.Error("Excpected task definition family to be: ", taskDefinitionVersion, " got: ", *taskMetric.TaskDefinitionVersion)
	}
	err = validateContainerMetrics(taskMetric.ContainerMetrics, 1)
	if err != nil {
		t.Error("Error validating container metrics: ", err)
	}

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Error("Error stopping container: ", container.ID, " error: ", err)
	}

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(engine)
	if err != nil {
		t.Fatal("Error validating metadata: ", err)
	}
}

func TestStatsEngineWithNewContainers(t *testing.T) {
	// This should be a functional test. Upgrading to docker 1.6 breaks our ability to
	// read state.json file for containers.
	t.Skip("Skipping integ test as this is really a functional test")
	engine := NewDockerStatsEngine(&cfg)
	err := engine.initDockerClient()
	if err != nil {
		t.Error("Error initializing stats engine: ", err)
	}
	container, err := createGremlin(client)
	if err != nil {
		t.Fatal("Error creating container", err)
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

	err = engine.Init()
	if err != nil {
		t.Error("Error initializing stats engine: ", err)
	}

	err = client.StartContainer(container.ID, nil)
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Error("Error starting container: ", container.ID, " error: ", err)
	}

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	if err != nil {
		t.Error("Error gettting instance metrics: ", err)
	}

	err = validateMetricsMetadata(metadata)
	if err != nil {
		t.Error("Error validating metadata: ", err)
	}

	if len(taskMetrics) != 1 {
		t.Error("Incorrect number of tasks. Expected: 1, got: ", len(taskMetrics))
	}
	taskMetric := taskMetrics[0]
	if *taskMetric.TaskDefinitionFamily != taskDefinitionFamily {
		t.Error("Excpected task definition family to be: ", taskDefinitionFamily, " got: ", *taskMetric.TaskDefinitionFamily)
	}
	if *taskMetric.TaskDefinitionVersion != taskDefinitionVersion {
		t.Error("Excpected task definition family to be: ", taskDefinitionVersion, " got: ", *taskMetric.TaskDefinitionVersion)
	}

	err = validateContainerMetrics(taskMetric.ContainerMetrics, 1)
	if err != nil {
		t.Error("Error validating container metrics: ", err)
	}

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Error("Error stopping container: ", container.ID, " error: ", err)
	}

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(engine)
	if err != nil {
		t.Fatal("Error validating metadata: ", err)
	}
}

func TestStatsEngineWithDockerTaskEngine(t *testing.T) {
	// This should be a functional test. Upgrading to docker 1.6 breaks our ability to
	// read state.json file for containers.
	t.Skip("Skipping integ test as this is really a functional test")
	taskEngine := engine.NewTaskEngine(&config.Config{}, false)
	container, err := createGremlin(client)
	if err != nil {
		t.Fatal("Error creating container", err)
	}
	defer client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    container.ID,
		Force: true,
	})
	unmappedContainer, err := createGremlin(client)
	if err != nil {
		t.Fatal("Error creating container", err)
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
	dockerTaskEngine, _ := taskEngine.(*engine.DockerTaskEngine)
	dockerTaskEngine.State().AddTask(&testTask)
	dockerTaskEngine.State().AddContainer(
		&api.DockerContainer{
			DockerId:   container.ID,
			DockerName: "gremlin",
			Container:  containers[0],
		},
		&testTask)
	statsEngine := NewDockerStatsEngine(&cfg)
	statsEngine.client, err = engine.NewDockerGoClient(nil, "", config.NewSensitiveRawMessage([]byte("")), false)
	if err != nil {
		t.Fatal("Error initializing docker client: ", err)
	}
	err = statsEngine.MustInit(taskEngine, defaultCluster, defaultContainerInstance)
	if err != nil {
		t.Error("Error initializing stats engine: ", err)
	}

	err = client.StartContainer(container.ID, nil)
	defer client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Error("Error starting container: ", container.ID, " error: ", err)
	}

	err = client.StartContainer(unmappedContainer.ID, nil)
	defer client.StopContainer(unmappedContainer.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Error("Error starting container: ", unmappedContainer.ID, " error: ", err)
	}

	// Wait for the stats collection go routine to start.
	time.Sleep(checkPointSleep)

	metadata, taskMetrics, err := statsEngine.GetInstanceMetrics()
	if err != nil {
		t.Error("Error gettting instance metrics: ", err)
	}

	if len(taskMetrics) != 1 {
		t.Error("Incorrect number of tasks. Expected: 1, got: ", len(taskMetrics))
	}
	err = validateMetricsMetadata(metadata)
	if err != nil {
		t.Error("Error validating metadata: ", err)
	}

	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 1)
	if err != nil {
		t.Error("Error validating container metrics: ", err)
	}

	err = client.StopContainer(container.ID, defaultDockerTimeoutSeconds)
	if err != nil {
		t.Error("Error stopping container: ", container.ID, " error: ", err)
	}

	time.Sleep(waitForCleanupSleep)

	// Should not contain any metrics after cleanup.
	err = validateIdleContainerMetrics(statsEngine)
	if err != nil {
		t.Fatal("Error validating metadata: ", err)
	}
}
