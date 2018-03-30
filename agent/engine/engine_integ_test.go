// +build integration
// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package engine

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/resources"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDockerStopTimeout       = 2 * time.Second
	credentialsIDIntegTest      = "credsid"
	waitTaskStateChangeDuration = 2 * time.Minute
)

func init() {
	// Set this very low for integ tests only
	_stoppedSentWaitInterval = 1 * time.Second
}

func createTestTask(arn string) *api.Task {
	return &api.Task{
		Arn:                 arn,
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: api.TaskRunning,
		Containers:          []*api.Container{createTestContainer()},
	}
}

func defaultTestConfigIntegTest() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.TaskCPUMemLimit = config.ExplicitlyDisabled
	return cfg
}

func setupWithDefaultConfig(t *testing.T) (TaskEngine, func(), credentials.Manager) {
	return setup(defaultTestConfigIntegTest(), t)
}

func setup(cfg *config.Config, t *testing.T) (TaskEngine, func(), credentials.Manager) {
	if os.Getenv("ECS_SKIP_ENGINE_INTEG_TEST") != "" {
		t.Skip("ECS_SKIP_ENGINE_INTEG_TEST")
	}
	if !isDockerRunning() {
		t.Skip("Docker not running")
	}
	clientFactory := dockerclient.NewFactory(dockerEndpoint)
	dockerClient, err := NewDockerGoClient(clientFactory, cfg)
	if err != nil {
		t.Fatalf("Error creating Docker client: %v", err)
	}
	credentialsManager := credentials.NewManager()
	state := dockerstate.NewTaskEngineState()
	imageManager := NewImageManager(cfg, dockerClient, state)
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	metadataManager := containermetadata.NewManager(dockerClient, cfg)
	resource := resources.New()
	resource.ApplyConfigDependencies(cfg)

	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, state, metadataManager, resource)
	taskEngine.MustInit(context.TODO())
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, credentialsManager
}

func verifyContainerRunningStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerRunning,
		"Expected container to be RUNNING")
}

func verifyTaskRunningStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskRunning,
		"Expected task to be RUNNING")
}

func verifyContainerStoppedStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerStopped,
		"Expected container to be STOPPED")
}

func verifyTaskStoppedStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped,
		"Expected task to be STOPPED")
}

// TestDockerStateToContainerState tests convert the container status from
// docker inspect to the status defined in agent
func TestDockerStateToContainerState(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	testTask := createTestTask("test_task")
	container := testTask.Containers[0]

	client, err := docker.NewClientFromEnv()
	require.NoError(t, err, "Creating go docker client failed")

	containerMetadata := taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)

	containerMetadata = taskEngine.(*DockerTaskEngine).createContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)
	state, _ := client.InspectContainer(containerMetadata.DockerID)
	assert.Equal(t, api.ContainerCreated, dockerStateToState(state.State))

	containerMetadata = taskEngine.(*DockerTaskEngine).startContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)
	state, _ = client.InspectContainer(containerMetadata.DockerID)
	assert.Equal(t, api.ContainerRunning, dockerStateToState(state.State))

	containerMetadata = taskEngine.(*DockerTaskEngine).stopContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)
	state, _ = client.InspectContainer(containerMetadata.DockerID)
	assert.Equal(t, api.ContainerStopped, dockerStateToState(state.State))

	// clean up the container
	err = taskEngine.(*DockerTaskEngine).removeContainer(testTask, container)
	assert.NoError(t, err, "remove the created container failed")

	// Start the container failed
	testTask = createTestTask("test_task2")
	testTask.Containers[0].EntryPoint = &[]string{"non-existed"}
	container = testTask.Containers[0]
	containerMetadata = taskEngine.(*DockerTaskEngine).createContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)
	containerMetadata = taskEngine.(*DockerTaskEngine).startContainer(testTask, container)
	assert.Error(t, containerMetadata.Error)
	state, _ = client.InspectContainer(containerMetadata.DockerID)
	assert.Equal(t, api.ContainerStopped, dockerStateToState(state.State))

	// clean up the container
	err = taskEngine.(*DockerTaskEngine).removeContainer(testTask, container)
	assert.NoError(t, err, "remove the created container failed")
}

func TestHostVolumeMount(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	tmpPath, _ := ioutil.TempDir("", "ecs_volume_test")
	defer os.RemoveAll(tmpPath)
	ioutil.WriteFile(filepath.Join(tmpPath, "test-file"), []byte("test-data"), 0644)

	testTask := createTestHostVolumeMountTask(tmpPath)

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)

	assert.NotNil(t, testTask.Containers[0].GetKnownExitCode(), "No exit code found")
	assert.Equal(t, 42, *testTask.Containers[0].GetKnownExitCode(), "Wrong exit code")

	data, err := ioutil.ReadFile(filepath.Join(tmpPath, "hello-from-container"))
	assert.Nil(t, err, "Unexpected error")
	assert.Equal(t, "hi", strings.TrimSpace(string(data)), "Incorrect file contents")
}

func TestEmptyHostVolumeMount(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	// creates a task with two containers
	testTask := createTestEmptyHostVolumeMountTask()
	for _, container := range testTask.Containers {
		container.TransitionDependenciesMap = make(map[api.ContainerStatus]api.TransitionDependencySet)
	}

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)

	assert.NotNil(t, testTask.Containers[0].GetKnownExitCode(), "No exit code found")
	assert.Equal(t, 42, *testTask.Containers[0].GetKnownExitCode(), "Wrong exit code, file probably wasn't present")
}

func TestSweepContainer(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Minute
	cfg.ContainerMetadataEnabled = true
	taskEngine, done, _ := setup(cfg, t)
	defer done()

	taskArn := "arn:aws:ecs:us-east-1:123456789012:task/testSweepContainer"
	testTask := createTestTask(taskArn)

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)

	tasks, _ := taskEngine.ListTasks()
	assert.Equal(t, len(tasks), 1)
	assert.Equal(t, tasks[0].GetKnownStatus(), api.TaskStopped)

	// Should be stopped, let's verify it's still listed...
	task, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn(taskArn)
	assert.True(t, ok, "Expected task to be present still, but wasn't")
	task.SetSentStatus(api.TaskStopped) // cleanupTask waits for TaskStopped to be sent before cleaning
	time.Sleep(1 * time.Minute)
	for i := 0; i < 60; i++ {
		_, ok = taskEngine.(*DockerTaskEngine).State().TaskByArn(taskArn)
		if !ok {
			break
		}
		time.Sleep(1 * time.Second)
	}
	assert.False(t, ok, "Expected container to have been swept but was not")

	tasks, _ = taskEngine.ListTasks()
	assert.Equal(t, len(tasks), 0)
}

// TestStartStopWithCredentials starts and stops a task for which credentials id
// has been set
func TestStartStopWithCredentials(t *testing.T) {
	taskEngine, done, credentialsManager := setupWithDefaultConfig(t)
	defer done()

	testTask := createTestTask("testStartWithCredentials")
	taskCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{CredentialsID: credentialsIDIntegTest},
	}
	credentialsManager.SetTaskCredentials(taskCredentials)
	testTask.SetCredentialsID(credentialsIDIntegTest)

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)

	// When task is stopped, credentials should have been removed for the
	// credentials id set in the task
	_, ok := credentialsManager.GetTaskCredentials(credentialsIDIntegTest)
	assert.False(t, ok, "Credentials not removed from credentials manager for stopped task")
}

func TestContainerHealthCheck(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testTaskWithHealthCheck"
	testTask := createTestHealthCheckTask(taskArn)

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskIsRunning(stateChangeEvents, testTask)

	waitForContainerHealthStatus(t, testTask)
	healthStatus := testTask.Containers[0].GetHealthStatus()
	assert.Equal(t, "HEALTHY", healthStatus.Status.BackendStatus(), "container health status is not HEALTHY")

	taskUpdate := createTestTask(taskArn)
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskIsStopped(stateChangeEvents, testTask)
}

func waitForContainerHealthStatus(t *testing.T, testTask *api.Task) {
	ctx, _ := context.WithTimeout(context.TODO(), waitTaskStateChangeDuration)
	for {
		select {
		case <-ctx.Done():
			t.Error("Timed out waiting for container health status")
		default:
			healthStatus := testTask.Containers[0].GetHealthStatus()
			if healthStatus.Status.BackendStatus() == "UNKNOWN" {
				time.Sleep(time.Second)
				continue
			}
			return
		}
	}
}
