//go:build integration && !windows
// +build integration,!windows

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

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/stretchr/testify/assert"
)

const (
	baseImageForOS = testRegistryHost + "/" + "busybox"
)

var (
	entryPointForOS = []string{"sh", "-c"}
)

func TestGranularStopTimeout(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "TestGranularStopTimeout"
	testTask := createTestTask(taskArn)

	parent := createTestContainerWithImageAndName(baseImageForOS, "parent")
	dependency1 := createTestContainerWithImageAndName(baseImageForOS, "dependency1")
	dependency2 := createTestContainerWithImageAndName(baseImageForOS, "dependency2")

	parent.EntryPoint = &entryPointForOS
	parent.Command = []string{"sleep 30"}
	parent.Essential = true
	parent.DependsOnUnsafe = []apicontainer.DependsOn{
		{
			ContainerName: "dependency1",
			Condition:     "START",
		},
		{
			ContainerName: "dependency2",
			Condition:     "START",
		},
	}

	dependency1.EntryPoint = &entryPointForOS
	dependency1.Command = []string{"trap 'echo caught' SIGTERM; sleep 60"}
	dependency1.StopTimeout = 5

	dependency2.EntryPoint = &entryPointForOS
	dependency2.Command = []string{"trap 'echo caught' SIGTERM; sleep 60"}
	dependency2.StopTimeout = 50

	testTask.Containers = []*apicontainer.Container{
		dependency1,
		dependency2,
		parent,
	}

	go taskEngine.AddTask(testTask)

	finished := make(chan interface{})
	go func() {

		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerRunningStateChange(t, taskEngine)
		verifyContainerRunningStateChange(t, taskEngine)

		verifyTaskIsRunning(stateChangeEvents, testTask)

		verifyContainerStoppedStateChange(t, taskEngine)
		verifyContainerStoppedStateChange(t, taskEngine)
		verifyContainerStoppedStateChange(t, taskEngine)

		verifyTaskIsStopped(stateChangeEvents, testTask)

		assert.Equal(t, 137, *testTask.Containers[0].GetKnownExitCode(), "Dependency1 should exit with code 137")
		assert.Equal(t, 0, *testTask.Containers[1].GetKnownExitCode(), "Dependency2 should exit with code 0")

		close(finished)
	}()

	waitFinished(t, finished, orderingTimeout)
}
