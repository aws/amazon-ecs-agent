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
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

const orderingTimeout = 90 * time.Second

// TestDependencyHealthCheck is a happy-case integration test that considers a workflow with a HEALTHY dependency
// condition. We ensure that the task can be both started and stopped.
func TestDependencyHealthCheck(t *testing.T) {
	// Skip these tests on WS 2016 until the failures are root-caused.
	isWindows2016, err := config.IsWindows2016()
	if err == nil && isWindows2016 == true {
		t.Skip()
	}

	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testDependencyHealth"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	dependency := createTestContainerWithImageAndName(baseImageForOS, "dependency")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"exit 0"}
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "dependency",
			Condition:     "HEALTHY",
		},
	}
	parent.Essential = true

	dependency.EntryPoint = &entryPointForOS
	dependency.Command = []string{"sleep 90"}
	dependency.HealthCheckType = apicontainer.DockerHealthCheckType
	dependency.DockerConfig.Config = aws.String(alwaysHealthyHealthCheckConfig)

	testTask.Containers = []*apicontainer.Container{
		parent,
		dependency,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// Both containers should start
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerRunningStateChange(t, taskEngine)
		verifyTaskIsRunning(stateChangeEvents, testTask)

		// Task should stop all at once
		verifyContainerStoppedStateChange(t, taskEngine)
		verifyContainerStoppedStateChange(t, taskEngine)
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}

// TestDependencyComplete validates that the COMPLETE dependency condition will resolve when the child container exits
// with exit code 1. It ensures that the child is started and stopped before the parent starts.
func TestDependencyComplete(t *testing.T) {
	// Skip these tests on WS 2016 until the failures are root-caused.
	isWindows2016, err := config.IsWindows2016()
	if err == nil && isWindows2016 == true {
		t.Skip()
	}

	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testDependencyComplete"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	dependency := createTestContainerWithImageAndName(baseImageForOS, "dependency")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"sleep 5"}
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "dependency",
			Condition:     "COMPLETE",
		},
	}

	dependency.EntryPoint = &entryPointForOS
	dependency.Command = []string{"sleep 10 && exit 1"}
	dependency.Essential = false

	testTask.Containers = []*apicontainer.Container{
		parent,
		dependency,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// First container should run to completion and then exit
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerStoppedStateChange(t, taskEngine)

		// Second container starts after the first stops, task becomes running
		verifyContainerRunningStateChange(t, taskEngine)
		verifyTaskIsRunning(stateChangeEvents, testTask)

		// Last container stops and then the task stops
		verifyContainerStoppedStateChange(t, taskEngine)
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}

// TestDependencyStart tests a task workflow with a START container ordering dependency between 2 containers.
// Container 'parent' depends on  container 'dependency' to START. We ensure that the 'parent' container starts only
// after the 'dependency' container has started.
func TestDependencyStart(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testDependencyStart"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	dependency := createTestContainerWithImageAndName(baseImageForOS, "dependency")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"sleep 5 && exit 0"}
	parent.Essential = true
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "dependency",
			Condition:     "START",
		},
	}

	dependency.EntryPoint = &entryPointForOS
	dependency.Command = []string{"sleep 90"}
	dependency.Essential = false

	testTask.Containers = []*apicontainer.Container{
		parent,
		dependency,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// 'dependency' container should run first, followed by the 'parent' container
		verifySpecificContainerStateChange(t, taskEngine, "dependency", status.ContainerRunning)
		verifySpecificContainerStateChange(t, taskEngine, "parent", status.ContainerRunning)

		// task becomes running after 'parent' is running
		err := verifyTaskIsRunning(stateChangeEvents, testTask)
		assert.NoError(t, err)

		// 'parent' container stops and then the task stops
		verifySpecificContainerStateChange(t, taskEngine, "parent", status.ContainerStopped)
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}

// TestDependencySuccess validates that the SUCCESS dependency condition will resolve when the child container exits
// with exit code 0. It ensures that the child is started and stopped before the parent starts.
func TestDependencySuccess(t *testing.T) {
	// Skip these tests on WS 2016 until the failures are root-caused.
	isWindows2016, err := config.IsWindows2016()
	if err == nil && isWindows2016 == true {
		t.Skip()
	}

	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testDependencySuccess"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	dependency := createTestContainerWithImageAndName(baseImageForOS, "dependency")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"exit 0"}
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "dependency",
			Condition:     "SUCCESS",
		},
	}

	dependency.EntryPoint = &entryPointForOS
	dependency.Command = []string{"sleep 10"}
	dependency.Essential = false

	testTask.Containers = []*apicontainer.Container{
		parent,
		dependency,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// First container should run to completion
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerStoppedStateChange(t, taskEngine)

		// Second container starts after the first stops, task becomes running
		verifyContainerRunningStateChange(t, taskEngine)
		verifyTaskIsRunning(stateChangeEvents, testTask)

		// Last container stops and then the task stops
		verifyContainerStoppedStateChange(t, taskEngine)
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}

// TestDependencySuccess validates that the SUCCESS dependency condition will fail when the child exits 1. This is a
// contrast to how COMPLETE behaves. Instead of starting the parent, the task should simply exit.
func TestDependencySuccessErrored(t *testing.T) {
	// Skip these tests on WS 2016 until the failures are root-caused.
	isWindows2016, err := config.IsWindows2016()
	if err == nil && isWindows2016 == true {
		t.Skip()
	}

	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testDependencySuccessErrored"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	dependency := createTestContainerWithImageAndName(baseImageForOS, "dependency")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"exit 0"}
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "dependency",
			Condition:     "SUCCESS",
		},
	}

	dependency.EntryPoint = &entryPointForOS
	dependency.Command = []string{"sleep 10 && exit 1"}
	dependency.Essential = false

	testTask.Containers = []*apicontainer.Container{
		parent,
		dependency,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// First container should run to completion
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerStoppedStateChange(t, taskEngine)

		// task should transition to stopped
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}

// TestDependencySuccessTimeout
func TestDependencySuccessTimeout(t *testing.T) {
	// Skip these tests on WS 2016 until the failures are root-caused.
	isWindows2016, err := config.IsWindows2016()
	if err == nil && isWindows2016 == true {
		t.Skip()
	}

	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testDependencySuccessTimeout"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	dependency := createTestContainerWithImageAndName(baseImageForOS, "dependency")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"exit 0"}
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "dependency",
			Condition:     "SUCCESS",
		},
	}

	dependency.EntryPoint = &entryPointForOS
	dependency.Command = []string{"sleep 15"}
	dependency.Essential = false

	// set the timeout to be shorter than the amount of time it takes to stop
	dependency.StartTimeout = 8

	testTask.Containers = []*apicontainer.Container{
		parent,
		dependency,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// First container should run to completion
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerStoppedStateChange(t, taskEngine)

		// task should transition to stopped
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}

// TestDependencyHealthyTimeout
func TestDependencyHealthyTimeout(t *testing.T) {
	// Skip these tests on WS 2016 until the failures are root-caused.
	isWindows2016, err := config.IsWindows2016()
	if err == nil && isWindows2016 == true {
		t.Skip()
	}

	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testDependencyHealthyTimeout"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	dependency := createTestContainerWithImageAndName(baseImageForOS, "dependency")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"exit 0"}
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "dependency",
			Condition:     "HEALTHY",
		},
	}

	dependency.EntryPoint = &entryPointForOS
	dependency.Command = []string{"sleep 30"}
	dependency.HealthCheckType = apicontainer.DockerHealthCheckType

	// enter a healthcheck that will fail
	dependency.DockerConfig.Config = aws.String(`{
			"HealthCheck":{
				"Test":["CMD-SHELL", "exit 1"]
			}
		}`)

	// set the timeout. Duration doesn't matter since healthcheck will always be unhealthy.
	dependency.StartTimeout = 8

	testTask.Containers = []*apicontainer.Container{
		parent,
		dependency,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// First container should run to completion
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerStoppedStateChange(t, taskEngine)

		// task should transition to stopped
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}

// TestShutdownOrder
func TestShutdownOrder(t *testing.T) {
	// Skip these tests on WS 2016 until the failures are root-caused.
	isWindows2016, err := config.IsWindows2016()
	if err == nil && isWindows2016 == true {
		t.Skip()
	}

	shutdownOrderingTimeout := 120 * time.Second
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testShutdownOrder"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	A := createTestContainerWithImageAndName(baseImageForOS, "A")
	B := createTestContainerWithImageAndName(baseImageForOS, "B")
	C := createTestContainerWithImageAndName(baseImageForOS, "C")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"echo hi"}
	parent.Essential = true
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "A",
			Condition:     "START",
		},
	}

	A.EntryPoint = &entryPointForOS
	A.Command = []string{"sleep 100"}
	A.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "B",
			Condition:     "START",
		},
	}

	B.EntryPoint = &entryPointForOS
	B.Command = []string{"sleep 100"}
	B.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "C",
			Condition:     "START",
		},
	}

	C.EntryPoint = &entryPointForOS
	C.Command = []string{"sleep 100"}

	testTask.Containers = []*apicontainer.Container{
		parent,
		A,
		B,
		C,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// Everything should first progress to running
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerRunningStateChange(t, taskEngine)
		verifyTaskIsRunning(stateChangeEvents, testTask)

		// The shutdown order will now proceed. Parent will exit first since it has an explicit exit command.
		event := <-stateChangeEvents
		assert.Equal(t, event.(api.ContainerStateChange).Status, status.ContainerStopped)
		assert.Equal(t, event.(api.ContainerStateChange).ContainerName, "parent")

		// The dependency chain is A -> B -> C. We expect the inverse order to be followed for shutdown:
		// A shuts down, then B, then C
		expectedC := <-stateChangeEvents
		assert.Equal(t, expectedC.(api.ContainerStateChange).Status, status.ContainerStopped)
		assert.Equal(t, expectedC.(api.ContainerStateChange).ContainerName, "A")

		expectedB := <-stateChangeEvents
		assert.Equal(t, expectedB.(api.ContainerStateChange).Status, status.ContainerStopped)
		assert.Equal(t, expectedB.(api.ContainerStateChange).ContainerName, "B")

		expectedA := <-stateChangeEvents
		assert.Equal(t, expectedA.(api.ContainerStateChange).Status, status.ContainerStopped)
		assert.Equal(t, expectedA.(api.ContainerStateChange).ContainerName, "C")

		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, shutdownOrderingTimeout)
}

func TestMultipleContainerDependency(t *testing.T) {
	// Skip these tests on WS 2016 until the failures are root-caused.
	isWindows2016, err := config.IsWindows2016()
	if err == nil && isWindows2016 == true {
		t.Skip()
	}

	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testMultipleContainerDependency"
	testTask := createTestTask(taskArn)

	exit := createTestContainerWithImageAndName(baseImageForOS, "exit")
	A := createTestContainerWithImageAndName(baseImageForOS, "A")
	B := createTestContainerWithImageAndName(baseImageForOS, "B")

	exit.EntryPoint = &entryPointForOS
	exit.Command = []string{"exit 1"}
	exit.Essential = false

	A.EntryPoint = &entryPointForOS
	A.Command = []string{"sleep 10"}
	A.Essential = true
	A.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "exit",
			Condition:     "SUCCESS",
		},
	}

	B.EntryPoint = &entryPointForOS
	B.Command = []string{"sleep 10"}
	B.Essential = true
	B.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "A",
			Condition:     "START",
		},
		{
			ContainerName: "exit",
			Condition:     "SUCCESS",
		},
	}

	testTask.Containers = []*apicontainer.Container{
		exit,
		A,
		B,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {
		// Only exit should first progress to running
		verifyContainerRunningStateChange(t, taskEngine)

		// Exit container should stop with exit code 1
		event := <-stateChangeEvents
		assert.Equal(t, event.(api.ContainerStateChange).Status, status.ContainerStopped)
		assert.Equal(t, event.(api.ContainerStateChange).ContainerName, "exit")

		// The task should be now stopped as dependencies of A and B are not resolved
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}

func waitFinished(t *testing.T, finished <-chan interface{}, duration time.Duration) {
	select {
	case <-finished:
		t.Log("Finished successfully.")
		return
	case <-time.After(duration):
		t.Error("timed out after: ", duration)
		t.FailNow()
	}
}
