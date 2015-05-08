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

package functional_tests

import (
	"testing"
	"time"

	. "github.com/aws/amazon-ecs-agent/test/util"
)

// TestRunManyTasks runs several tasks in short succession and expects them to
// all run.
func TestRunManyTasks(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	numToRun := 15
	tasks := []*TestTask{}
	attemptsTaken := 0
	for numRun := 0; len(tasks) < numToRun; attemptsTaken++ {
		startNum := 10
		if numToRun-len(tasks) < 10 {
			startNum = numToRun - len(tasks)
		}
		startedTasks, err := agent.StartMultipleTasks(t, "simple-exit", startNum)
		if err != nil {
			continue
		}
		tasks = append(tasks, startedTasks...)
		numRun += 10
	}

	t.Logf("Ran %v containers; took %v tries\n", numToRun, attemptsTaken)
	for _, task := range tasks {
		err := task.WaitStopped(10 * time.Minute)
		if err != nil {
			t.Error(err)
		}
		if code, ok := task.ContainerExitcode("exit"); !ok || code != 42 {
			t.Error("Wrong exit code")
		}
	}
}

// TestPullInvalidImage verifies that an invalid image returns an error
func TestPullInvalidImage(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "invalid-image")
	if err != nil {
		t.Fatal("Expected to start invalid-image task")
	}
	testTask.ExpectErrorType("error", "CannotPulledContainerError", 1*time.Minute)
}

// TestOOMContainer verifies that an OOM container returns an error
func TestOOMContainer(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "oom-container")
	if err != nil {
		t.Fatal("Expected to start invalid-image task")
	}
	testTask.ExpectErrorType("error", "OutOfMemoryError", 1*time.Minute)
}

// TestSavedState verifies that stopping the agent, stopping a container under
// its control, and starting the agent results in that container being moved to
// 'stopped'
func TestSavedState(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "nginx")
	if err != nil {
		t.Fatal(err)
	}
	testTask.WaitRunning(1 * time.Minute)

	dockerId, err := agent.ResolveTaskDockerID(testTask, "nginx")
	if err != nil {
		t.Fatal(err)
	}

	err = agent.StopAgent()
	if err != nil {
		t.Fatal(err)
	}

	err = agent.DockerClient.StopContainer(dockerId, 1)
	if err != nil {
		t.Fatal(err)
	}

	err = agent.StartAgent()
	if err != nil {
		t.Fatal(err)
	}

	testTask.WaitStopped(1 * time.Minute)
}

// TestPortResourceContention verifies that running two tasks on the same port
// in quick-succession does not result in the second one failing to run. It
// verifies the 'seqnum' serialization stuff works.
func TestPortResourceContention(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "busybox-port-5180")
	if err != nil {
		t.Fatal(err)
	}
	err = testTask.WaitRunning(1 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	err = testTask.Stop()
	if err != nil {
		t.Fatal(err)
	}

	testTask2, err := agent.StartTask(t, "busybox-port-5180")
	if err != nil {
		t.Fatal(err)
	}
	err = testTask2.WaitRunning(1 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	testTask2.Stop()

	go testTask.WaitStopped(2 * time.Minute)
	testTask2.WaitStopped(2 * time.Minute)
	// 30 seconds because this busybox ignores sigterm
}
