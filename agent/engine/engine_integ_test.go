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

package engine

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	sdkClient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDockerStopTimeout  = 5 * time.Second
	credentialsIDIntegTest = "credsid"
	serverContent          = "ecs test container"
	dialTimeout            = 200 * time.Millisecond
	localhost              = "127.0.0.1"
	waitForDockerDuration  = 50 * time.Millisecond
	removeVolumeTimeout    = 5 * time.Second

	alwaysHealthyHealthCheckConfig = `{
			"HealthCheck":{
				"Test":["CMD-SHELL", "echo hello"],
				"Interval":100000000,
				"Timeout":2000000000,
				"StartPeriod":100000000,
				"Retries":3}
		}`
)

func init() {
	// Set this very low for integ tests only
	_stoppedSentWaitInterval = 1 * time.Second
}

func setupWithDefaultConfig(t *testing.T) (TaskEngine, func(), credentials.Manager) {
	return setup(defaultTestConfigIntegTest(), nil, t)
}

func setupWithState(t *testing.T, state dockerstate.TaskEngineState) (TaskEngine, func(), credentials.Manager) {
	return setup(defaultTestConfigIntegTest(), state, t)
}

func verifyTaskRunningStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskRunning,
		"Expected task to be RUNNING")
}

func verifyTaskRunningStateChangeWithRuntimeID(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskRunning,
		"Expected task to be RUNNING")
	assert.NotEqualf(t, "", event.(api.TaskStateChange).Task.Containers[0].RuntimeID,
		"Expected task with runtimeID per container should not empty when Running")
}

func verifyTaskStoppedStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskStopped,
		"Expected task to be STOPPED")
}

func verifyTaskStoppedStateChangeWithRuntimeID(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskStopped,
		"Expected task to be STOPPED")
	assert.NotEqual(t, "", event.(api.TaskStateChange).Task.Containers[0].RuntimeID,
		"Expected task with runtimeID per container should not empty when stopped")
}

func dialWithRetries(proto string, address string, tries int, timeout time.Duration) (net.Conn, error) {
	var err error
	var conn net.Conn
	for i := 0; i < tries; i++ {
		conn, err = net.DialTimeout(proto, address, timeout)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return conn, err
}

func removeImage(t *testing.T, img string) {
	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "create docker client failed")
	client.ImageRemove(context.TODO(), img, types.ImageRemoveOptions{})
}

func cleanVolumes(testTask *apitask.Task, taskEngine TaskEngine) {
	client := taskEngine.(*DockerTaskEngine).client
	for _, aVolume := range testTask.Volumes {
		client.RemoveVolume(context.TODO(), aVolume.Name, removeVolumeTimeout)
	}
}

// TestDockerStateToContainerState tests convert the container status from
// docker inspect to the status defined in agent
func TestDockerStateToContainerState(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	testTask := createTestTask("test_task")
	container := testTask.Containers[0]

	// let the container keep running to prevent the edge case where it's already stopped when we check whether
	// it's running
	container.Command = getLongRunningCommand()

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	containerMetadata := taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)

	containerMetadata = taskEngine.(*DockerTaskEngine).createContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)
	state, _ := client.ContainerInspect(ctx, containerMetadata.DockerID)
	assert.Equal(t, apicontainerstatus.ContainerCreated, dockerapi.DockerStateToState(state.ContainerJSONBase.State))

	containerMetadata = taskEngine.(*DockerTaskEngine).startContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)
	state, _ = client.ContainerInspect(ctx, containerMetadata.DockerID)
	assert.Equal(t, apicontainerstatus.ContainerRunning, dockerapi.DockerStateToState(state.ContainerJSONBase.State))

	containerMetadata = taskEngine.(*DockerTaskEngine).stopContainer(testTask, container)
	assert.NoError(t, containerMetadata.Error)
	state, _ = client.ContainerInspect(ctx, containerMetadata.DockerID)
	assert.Equal(t, apicontainerstatus.ContainerStopped, dockerapi.DockerStateToState(state.ContainerJSONBase.State))

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
	state, _ = client.ContainerInspect(ctx, containerMetadata.DockerID)
	assert.Equal(t, apicontainerstatus.ContainerStopped, dockerapi.DockerStateToState(state.ContainerJSONBase.State))

	// clean up the container
	err = taskEngine.(*DockerTaskEngine).removeContainer(testTask, container)
	assert.NoError(t, err, "remove the created container failed")
}

func TestHostVolumeMount(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	tmpPath := t.TempDir()
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

	cleanVolumes(testTask, taskEngine)
}

func TestSweepContainer(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Minute
	cfg.ContainerMetadataEnabled = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	taskEngine, done, _ := setup(cfg, nil, t)
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
	assert.Equal(t, tasks[0].GetKnownStatus(), apitaskstatus.TaskStopped)

	// Should be stopped, let's verify it's still listed...
	task, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn(taskArn)
	assert.True(t, ok, "Expected task to be present still, but wasn't")
	task.SetSentStatus(apitaskstatus.TaskStopped) // cleanupTask waits for TaskStopped to be sent before cleaning
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
	credentialsManager.SetTaskCredentials(&taskCredentials)
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

// TestStartStopWithRuntimeID starts and stops a task for which runtimeID has been set.
func TestStartStopWithRuntimeID(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	testTask := createTestTask("testTaskWithContainerID")
	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChangeWithRuntimeID(t, taskEngine)
	verifyTaskRunningStateChangeWithRuntimeID(t, taskEngine)
	verifyContainerStoppedStateChangeWithRuntimeID(t, taskEngine)
	verifyTaskStoppedStateChangeWithRuntimeID(t, taskEngine)
}

func TestTaskStopWhenPullImageFail(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.ImagePullBehavior = config.ImagePullAlwaysBehavior
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()

	testTask := createTestTask("testTaskStopWhenPullImageFail")
	// Assign an invalid image to the task, and verify the task fails
	// when the pull image behavior is "always".
	testTask.Containers = []*apicontainer.Container{createTestContainerWithImageAndName("invalidImage", "invalidName")}

	go taskEngine.AddTask(testTask)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
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
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskIsStopped(stateChangeEvents, testTask)
}

// TestEngineSynchronize tests the agent synchronize the container status on restart
func TestEngineSynchronize(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)

	taskArn := "arn:aws:ecs:us-east-1:123456789012:task/testEngineSynchronize"
	testTask := createTestTask(taskArn)
	testTask.Containers[0].Image = testVolumeImage

	// Start a task
	go taskEngine.AddTask(testTask)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)
	// Record the container information
	state := taskEngine.(*DockerTaskEngine).State()
	containersMap, ok := state.ContainerMapByArn(taskArn)
	require.True(t, ok, "no container found in the agent state")
	require.Len(t, containersMap, 1)
	containerIDs := state.GetAllContainerIDs()
	require.Len(t, containerIDs, 1)
	imageStates := state.AllImageStates()
	assert.Len(t, imageStates, 1)
	taskEngine.(*DockerTaskEngine).stopEngine()

	containerBeforeSync, ok := containersMap[testTask.Containers[0].Name]
	assert.True(t, ok, "container not found in the containers map")
	// Task and Container restored from state file
	containerSaved := &apicontainer.Container{
		Name:                containerBeforeSync.Container.Name,
		SentStatusUnsafe:    apicontainerstatus.ContainerRunning,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
	}
	task := &apitask.Task{
		Arn: taskArn,
		Containers: []*apicontainer.Container{
			containerSaved,
		},
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		SentStatusUnsafe:    apitaskstatus.TaskRunning,
	}

	state = dockerstate.NewTaskEngineState()
	state.AddTask(task)
	state.AddContainer(&apicontainer.DockerContainer{
		DockerID:  containerBeforeSync.DockerID,
		Container: containerSaved,
	}, task)
	state.AddImageState(imageStates[0])

	// Simulate the agent restart
	taskEngine, done, _ = setupWithState(t, state)
	defer done()

	taskEngine.MustInit(context.TODO())

	// Check container status/metadata and image information are correctly synchronized
	containerIDAfterSync := state.GetAllContainerIDs()
	require.Len(t, containerIDAfterSync, 1)
	containerAfterSync, ok := state.ContainerByID(containerIDAfterSync[0])
	assert.True(t, ok, "no container found in the agent state")

	assert.Equal(t, containerAfterSync.Container.GetKnownStatus(), containerBeforeSync.Container.GetKnownStatus())
	assert.Equal(t, containerAfterSync.Container.GetLabels(), containerBeforeSync.Container.GetLabels())
	assert.Equal(t, containerAfterSync.Container.GetStartedAt(), containerBeforeSync.Container.GetStartedAt())
	assert.Equal(t, containerAfterSync.Container.GetCreatedAt(), containerBeforeSync.Container.GetCreatedAt())

	imageStateAfterSync := state.AllImageStates()
	assert.Len(t, imageStateAfterSync, 1)
	assert.Equal(t, *imageStateAfterSync[0], *imageStates[0])

	testTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(testTask)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

func TestLabels(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	testArn := "TestLabels"
	testTask := createTestTask(testArn)
	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{Config: aws.String(`{
	"Labels": {
		"com.foo.label2": "value",
		"label1":""
	}}`)}
	go taskEngine.AddTask(testTask)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	state, _ := client.ContainerInspect(ctx, cid)
	assert.EqualValues(t, "value", state.Config.Labels["com.foo.label2"])
	assert.EqualValues(t, "", state.Config.Labels["label1"])

	// Kill the existing container now
	// Create instead of copying the testTask, to avoid race condition.
	// AddTask idempotently handles update, filtering by Task ARN.
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

func TestLogDriverOptions(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint),
		sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	testArn := "TestLogdriverOptions"
	testTask := createTestTask(testArn)
	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{HostConfig: aws.String(`{
	"LogConfig": {
		"Type": "json-file",
		"Config": {
			"max-file": "50",
			"max-size": "50k"
		}
	}}`)}
	go taskEngine.AddTask(testTask)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	state, _ := client.ContainerInspect(ctx, cid)

	containerExpected := container.LogConfig{
		Type:   "json-file",
		Config: map[string]string{"max-file": "50", "max-size": "50k"},
	}

	assert.EqualValues(t, containerExpected, state.HostConfig.LogConfig)

	// Kill the existing container now
	// Create instead of copying the testTask, to avoid race condition.
	// AddTask idempotently handles update, filtering by Task ARN.
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

// TestNetworkModeNone tests the container network can be configured
// as None mode in task definition
func TestNetworkModeNone(t *testing.T) {
	testNetworkMode(t, "none")
}

func testNetworkMode(t *testing.T, networkMode string) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint),
		sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	testArn := "TestNetworkMode"
	testTask := createTestTask(testArn)
	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{
		HostConfig: aws.String(fmt.Sprintf(`{"NetworkMode":"%s"}`, networkMode))}

	go taskEngine.AddTask(testTask)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID

	state, _ := client.ContainerInspect(ctx, cid)
	assert.NotNil(t, state.NetworkSettings, "Couldn't find the container network setting info")

	var networks []string
	for key := range state.NetworkSettings.Networks {
		networks = append(networks, key)
	}
	assert.Equal(t, 1, len(networks), "found multiple networks in container config")
	assert.Equal(t, networkMode, networks[0], "did not find the expected network mode")

	// Kill the existing container now
	// Create instead of copying the testTask, to avoid race condition.
	// AddTask idempotently handles update, filtering by Task ARN.
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

func TestTaskCleanup(t *testing.T) {
	os.Setenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "40s")
	defer os.Unsetenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION")
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(
		sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	testArn := "TestTaskCleanup"
	testTask := createTestTask(testArn)

	go taskEngine.AddTask(testTask)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	_, err = client.ContainerInspect(ctx, cid)
	assert.NoError(t, err, "Inspect should work")

	// Create instead of copying the testTask, to avoid race condition.
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)

	task, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn(testArn)
	assert.True(t, ok, "Expected task to be present still, but wasn't")
	task.SetSentStatus(apitaskstatus.TaskStopped)   // cleanupTask waits for TaskStopped to be sent before cleaning
	waitForTaskCleanup(t, taskEngine, testArn, 120) // 120 seconds

	_, err = client.ContainerInspect(ctx, cid)
	assert.Error(t, err, "Inspect should not work")
}
