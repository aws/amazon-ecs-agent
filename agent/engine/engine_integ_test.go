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
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
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

	// busybox image avaialble in a local registry set up by `make test-registry`
	localRegistryBusyboxImage       = "127.0.0.1:51670/busybox"
	localRegistryBusyboxImageDigest = "sha256:51de9138b0cc394c813df84f334d638499333cac22edd05d0300b2c9a2dc80dd"
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

// Tests that containers with ordering dependencies are able to reach MANIFEST_PULLED state
// regardless of the dependencies.
func TestManifestPulledDoesNotDependOnContainerOrdering(t *testing.T) {
	imagePullBehaviors := []config.ImagePullBehaviorType{
		config.ImagePullDefaultBehavior, config.ImagePullAlwaysBehavior,
		config.ImagePullPreferCachedBehavior, config.ImagePullOnceBehavior,
	}

	for _, behavior := range imagePullBehaviors {
		t.Run(fmt.Sprintf("%v", behavior), func(t *testing.T) {
			cfg := defaultTestConfigIntegTest()
			cfg.ImagePullBehavior = behavior
			cfg.DockerStopTimeout = 100 * time.Millisecond
			taskEngine, done, _ := setup(cfg, nil, t)
			defer done()

			image := "public.ecr.aws/docker/library/busybox:1.36.1"
			first := createTestContainerWithImageAndName(image, "first")
			first.Command = []string{"sh", "-c", "sleep 60"}

			second := createTestContainerWithImageAndName(image, "second")
			second.SetDependsOn([]apicontainer.DependsOn{
				{ContainerName: first.Name, Condition: "COMPLETE"},
			})

			task := &apitask.Task{
				Arn:                 "test-arn",
				Family:              "family",
				Version:             "1",
				DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				Containers:          []*apicontainer.Container{first, second},
			}

			// Start the task and wait for first container to start running
			go taskEngine.AddTask(task)
			verifyContainerRunningStateChange(t, taskEngine)

			// The first container should be in RUNNING state
			assert.Equal(t, apicontainerstatus.ContainerRunning, first.GetKnownStatus())
			// The second container should be waiting in MANIFEST_PULLED state
			assert.Equal(t, apicontainerstatus.ContainerManifestPulled, second.GetKnownStatus())

			// Assert that both containers have the right image digest populated
			expectedDigest := "sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966"
			assert.Equal(t, expectedDigest, first.GetImageDigest())
			assert.Equal(t, expectedDigest, second.GetImageDigest())

			// Cleanup
			first.SetDesiredStatus(apicontainerstatus.ContainerStopped)
			second.SetDesiredStatus(apicontainerstatus.ContainerStopped)
			verifyContainerStoppedStateChange(t, taskEngine)
			verifyContainerStoppedStateChange(t, taskEngine)
			verifyTaskStoppedStateChange(t, taskEngine)
			taskEngine.(*DockerTaskEngine).removeContainer(task, first)
			taskEngine.(*DockerTaskEngine).removeContainer(task, second)
			removeImage(t, image)
		})
	}
}

// Integration test for pullContainerManifest.
// The test depends on 127.0.0.1:51670/busybox image that is prepared by `make test-registry`
// command.
func TestPullContainerManifestInteg(t *testing.T) {
	allPullBehaviors := []config.ImagePullBehaviorType{
		config.ImagePullDefaultBehavior, config.ImagePullAlwaysBehavior,
		config.ImagePullOnceBehavior, config.ImagePullPreferCachedBehavior,
	}
	tcs := []struct {
		name               string
		image              string
		setConfig          func(c *config.Config)
		imagePullBehaviors []config.ImagePullBehaviorType
		assertError        func(t *testing.T, err error)
	}{
		{
			name:               "digest available in image reference",
			image:              "ubuntu@sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			imagePullBehaviors: allPullBehaviors,
		},
		{
			name:               "digest can be resolved from explicit tag",
			image:              localRegistryBusyboxImage,
			imagePullBehaviors: allPullBehaviors,
		},
		{
			name:               "digest can be resolved without an explicit tag",
			image:              localRegistryBusyboxImage,
			imagePullBehaviors: allPullBehaviors,
		},
		{
			name:               "manifest pull can timeout",
			image:              localRegistryBusyboxImage,
			setConfig:          func(c *config.Config) { c.ManifestPullTimeout = 0 },
			imagePullBehaviors: []config.ImagePullBehaviorType{config.ImagePullAlwaysBehavior},

			assertError: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "Could not transition to MANIFEST_PULLED; timed out")
			},
		},
		{
			name:               "manifest pull can timeout - non-zero timeout",
			image:              localRegistryBusyboxImage,
			setConfig:          func(c *config.Config) { c.ManifestPullTimeout = 100 * time.Microsecond },
			imagePullBehaviors: []config.ImagePullBehaviorType{config.ImagePullAlwaysBehavior},
			assertError: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "Could not transition to MANIFEST_PULLED; timed out")
			},
		},
	}
	for _, tc := range tcs {
		for _, imagePullBehavior := range tc.imagePullBehaviors {
			t.Run(fmt.Sprintf("%s - %v", tc.name, imagePullBehavior), func(t *testing.T) {
				cfg := defaultTestConfigIntegTest()
				cfg.ImagePullBehavior = imagePullBehavior

				if tc.setConfig != nil {
					tc.setConfig(cfg)
				}

				taskEngine, done, _ := setup(cfg, nil, t)
				defer done()

				container := &apicontainer.Container{Image: tc.image}
				task := &apitask.Task{Containers: []*apicontainer.Container{container}}

				res := taskEngine.(*DockerTaskEngine).pullContainerManifest(task, container)
				if tc.assertError == nil {
					require.NoError(t, res.Error)
					assert.NotEmpty(t, container.GetImageDigest())
				} else {
					tc.assertError(t, res.Error)
				}
			})
		}
	}
}

// Tests pullContainer method pulls container image as expected with and without an image
// digest populated on the container. If an image digest is populated then pullContainer
// uses the digest to prepare a canonical reference for the image to pull the image version
// referenced by the digest.
func TestPullContainerWithAndWithoutDigestInteg(t *testing.T) {
	tcs := []struct {
		name        string
		image       string
		imageDigest string
	}{
		{
			name:        "no tag no digest",
			image:       "public.ecr.aws/docker/library/alpine",
			imageDigest: "",
		},
		{
			name:        "tag but no digest",
			image:       "public.ecr.aws/docker/library/alpine:latest",
			imageDigest: "",
		},
		{
			name:        "no tag with digest",
			image:       "public.ecr.aws/docker/library/alpine",
			imageDigest: "sha256:c5b1261d6d3e43071626931fc004f70149baeba2c8ec672bd4f27761f8e1ad6b",
		},
		{
			name:        "tag with digest",
			image:       "public.ecr.aws/docker/library/alpine:3.19",
			imageDigest: "sha256:c5b1261d6d3e43071626931fc004f70149baeba2c8ec672bd4f27761f8e1ad6b",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare task
			task := &apitask.Task{Containers: []*apicontainer.Container{{Image: tc.image}}}
			container := task.Containers[0]
			container.SetImageDigest(tc.imageDigest)

			// Prepare task engine
			cfg := defaultTestConfigIntegTest()
			cfg.ImagePullBehavior = config.ImagePullAlwaysBehavior
			taskEngine, done, _ := setup(cfg, nil, t)
			defer done()
			dockerClient := taskEngine.(*DockerTaskEngine).client

			// Remove image from the host if it exists to start from a clean slate
			removeImage(t, container.Image)

			// Pull the image
			pullRes := taskEngine.(*DockerTaskEngine).pullContainer(task, container)
			require.NoError(t, pullRes.Error)

			// Check that the image was pulled
			_, err := dockerClient.InspectImage(container.Image)
			require.NoError(t, err)

			// Cleanup
			removeImage(t, container.Image)
		})
	}
}

// Tests that pullContainer pulls the same image when a digest is used versus when a digest
// is not used.
func TestPullContainerWithAndWithoutDigestConsistency(t *testing.T) {
	image := "public.ecr.aws/docker/library/alpine:3.19"
	imageDigest := "sha256:c5b1261d6d3e43071626931fc004f70149baeba2c8ec672bd4f27761f8e1ad6b"

	// Prepare task
	task := &apitask.Task{Containers: []*apicontainer.Container{{Image: image}}}
	container := task.Containers[0]

	// Prepare task engine
	cfg := defaultTestConfigIntegTest()
	cfg.ImagePullBehavior = config.ImagePullAlwaysBehavior
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()
	dockerClient := taskEngine.(*DockerTaskEngine).client

	// Remove image from the host if it exists to start from a clean slate
	removeImage(t, container.Image)

	// Pull the image without digest
	pullRes := taskEngine.(*DockerTaskEngine).pullContainer(task, container)
	require.NoError(t, pullRes.Error)
	inspectWithoutDigest, err := dockerClient.InspectImage(container.Image)
	require.NoError(t, err)
	removeImage(t, container.Image)

	// Pull the image with digest
	container.SetImageDigest(imageDigest)
	pullRes = taskEngine.(*DockerTaskEngine).pullContainer(task, container)
	require.NoError(t, pullRes.Error)
	inspectWithDigest, err := dockerClient.InspectImage(container.Image)
	require.NoError(t, err)
	removeImage(t, container.Image)

	// Image should be the same
	assert.Equal(t, inspectWithDigest.ID, inspectWithoutDigest.ID)
}

// Tests that a task with invalid image fails as expected.
func TestInvalidImageInteg(t *testing.T) {
	// Prepare task engine
	cfg := defaultTestConfigIntegTest()
	cfg.ImagePullBehavior = config.ImagePullAlwaysBehavior
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()

	// Prepare a task
	container := createTestContainerWithImageAndName("127.0.0.1:51670/invalid-image", "container")
	task := &apitask.Task{
		Arn:                 "test-arn",
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{container},
	}

	// Start the task
	go taskEngine.AddTask(task)

	// The container and the task both should stop
	verifyContainerStoppedStateChangeWithReason(t, taskEngine,
		"CannotPullImageManifestError: Error response from daemon: manifest unknown: manifest unknown")
	verifyTaskStoppedStateChange(t, taskEngine)

	err := taskEngine.(*DockerTaskEngine).removeContainer(task, container)
	require.NoError(t, err, "failed to remove container during cleanup")
}

// Tests that a task with an image that has a digest specified works normally.
func TestImageWithDigestInteg(t *testing.T) {
	// Prepare task engine
	cfg := defaultTestConfigIntegTest()
	cfg.ImagePullBehavior = config.ImagePullAlwaysBehavior
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()

	// Prepare a task with image digest
	container := createTestContainerWithImageAndName(
		localRegistryBusyboxImage+"@"+localRegistryBusyboxImageDigest, "container")
	container.Command = []string{"sh", "-c", "sleep 1"}
	task := &apitask.Task{
		Arn:                 "test-arn",
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{container},
	}

	// Start the task
	go taskEngine.AddTask(task)

	// The task should run
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)
	assert.Equal(t, localRegistryBusyboxImageDigest, container.GetImageDigest())

	// Cleanup
	container.SetDesiredStatus(apicontainerstatus.ContainerStopped)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
	err := taskEngine.(*DockerTaskEngine).removeContainer(task, container)
	require.NoError(t, err, "failed to remove container during cleanup")
}
