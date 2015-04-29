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
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"
)

func TestRunSimpleTests(t *testing.T) {
	simpleTests, err := filepath.Glob(filepath.Join("testdata", "simpletests", "*.json"))
	if err != nil {
		t.Fatal(err)
	}

	type simpleTestMetadata struct {
		TaskDefinition string
		Timeout        string
		ExitCodes      map[string]int
	}

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	for _, test := range simpleTests {
		testData, err := ioutil.ReadFile(test)
		if err != nil {
			t.Error(err)
		}
		var aTest simpleTestMetadata
		json.Unmarshal(testData, &aTest)
		testTask, err := agent.StartTask(t, aTest.TaskDefinition)
		if err != nil {
			t.Fatal(err)
		}
		timeout, err := time.ParseDuration(aTest.Timeout)
		if err != nil {
			t.Fatal(err)
		}
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatal(err)
		}
		for name, code := range aTest.ExitCodes {
			if exit, ok := testTask.ContainerExitcode(name); !ok || exit != code {
				t.Error(name, code, exit, ok)
			}
		}
	}
}

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

func TestPullInvalidImage(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "invalid-image")
	if err != nil {
		t.Fatal("Expected to start invalid-image task")
	}
	testTask.ExpectErrorType("error", "CannotPulledContainerError", 1*time.Minute)
}
