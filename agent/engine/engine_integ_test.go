// +build integration
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

package engine

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

const (
	testDockerStopTimeout  = 2 * time.Second
	credentialsIDIntegTest = "credsid"
)

func createTestTask(arn string) *api.Task {
	return &api.Task{
		Arn:           arn,
		Family:        arn,
		Version:       "1",
		DesiredStatus: api.TaskRunning,
		Containers:    []*api.Container{createTestContainer()},
	}
}

func defaultTestConfigIntegTest() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
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
	dockerClient, err := NewDockerGoClient(clientFactory, false, cfg)
	if err != nil {
		t.Fatalf("Error creating Docker client: %v", err)
	}
	credentialsManager := credentials.NewManager()
	state := dockerstate.NewDockerTaskEngineState()
	imageManager := NewImageManager(cfg, dockerClient, state)
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, state)
	taskEngine.Init()
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, credentialsManager
}

func discardEvents(from interface{}) func() {
	done := make(chan bool)

	go func() {
		for {
			ndx, _, _ := reflect.Select([]reflect.SelectCase{
				reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(from),
				},
				reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(done),
				},
			})
			if ndx == 1 {
				break
			}
		}
	}()
	return func() {
		done <- true
	}
}

func TestHostVolumeMount(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	tmpPath, _ := ioutil.TempDir("", "ecs_volume_test")
	defer os.RemoveAll(tmpPath)
	ioutil.WriteFile(filepath.Join(tmpPath, "test-file"), []byte("test-data"), 0644)

	testTask := createTestHostVolumeMountTask(tmpPath)

	go taskEngine.AddTask(testTask)

	verifyTaskIsStopped(taskEvents, testTask)

	assert.NotNil(t, testTask.Containers[0].KnownExitCode, "No exit code found")
	assert.Equal(t, 42, *testTask.Containers[0].KnownExitCode, "Wrong exit code")

	data, err := ioutil.ReadFile(filepath.Join(tmpPath, "hello-from-container"))
	assert.Nil(t, err, "Unexpected error")
	assert.Equal(t, "hi", strings.TrimSpace(string(data)), "Incorrect file contents")
}

func TestEmptyHostVolumeMount(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testTask := createTestEmptyHostVolumeMountTask()
	go taskEngine.AddTask(testTask)

	verifyTaskIsStopped(taskEvents, testTask)

	assert.NotNil(t, testTask.Containers[0].KnownExitCode, "No exit code found")
	assert.Equal(t, 42, *testTask.Containers[0].KnownExitCode, "Wrong exit code, file probably wasn't present")
}

func TestSweepContainer(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Minute
	taskEngine, done, _ := setup(cfg, t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testTask := createTestTask("testSweepContainer")

	go taskEngine.AddTask(testTask)

	expectedEvents := []api.TaskStatus{api.TaskRunning, api.TaskStopped}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expectedEvent := expectedEvents[0]
		expectedEvents = expectedEvents[1:]
		assert.Equal(t, expectedEvent, taskEvent.Status, "Got incorrect event")
		if len(expectedEvents) == 0 {
			break
		}
	}

	defer discardEvents(taskEvents)()

	// Should be stopped, let's verify it's still listed...
	_, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn("testSweepContainer")
	assert.True(t, ok, "Expected task to be present still, but wasn't")
	time.Sleep(1 * time.Minute)
	for i := 0; i < 60; i++ {
		_, ok = taskEngine.(*DockerTaskEngine).State().TaskByArn("testSweepContainer")
		if !ok {
			break
		}
		time.Sleep(1 * time.Second)
	}
	assert.False(t, ok, "Expected container to have been swept but was not")
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
	testTask.SetCredentialsId(credentialsIDIntegTest)

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	verifyTaskIsStopped(taskEvents, testTask)

	// When task is stopped, credentials should have been removed for the
	// credentials id set in the task
	_, ok := credentialsManager.GetTaskCredentials(credentialsIDIntegTest)
	assert.False(t, ok, "Credentials not removed from credentials manager for stopped task")
}

func verifyTaskIsRunning(taskEvents <-chan api.TaskStateChange, testTasks ...*api.Task) error {
	for {
		select {
		case taskEvent := <-taskEvents:
			for i, task := range testTasks {
				if taskEvent.TaskArn != task.Arn {
					continue
				}
				if taskEvent.Status == api.TaskRunning {
					if len(testTasks) == 1 {
						return nil
					}
					testTasks = append(testTasks[:i], testTasks[i+1:]...)
				} else if taskEvent.Status > api.TaskRunning {
					return fmt.Errorf("Task went straight to %s without running, task: %s", taskEvent.Status.String(), task.Arn)
				}
			}
		}
	}
}

func verifyTaskIsStopped(taskEvents <-chan api.TaskStateChange, testTasks ...*api.Task) {
	for {
		select {
		case taskEvent := <-taskEvents:
			for i, task := range testTasks {
				if taskEvent.TaskArn == task.Arn && taskEvent.Status >= api.TaskStopped {
					if len(testTasks) == 1 {
						return
					}
					testTasks = append(testTasks[:i], testTasks[i+1:]...)
				}
			}
		}
	}
}
