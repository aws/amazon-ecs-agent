// +build functional,windows

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

// Package simpletest is an auto-generated set of tests defined by the json
// descriptions in testdata/simpletests.
//
// This file should not be edited; rather you should edit the generator instead
package simpletest

import (
	"os"
	"testing"
	"time"
	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"
)

// TestContainerOrderingTimedout Check that container ordering has timed out
func TestContainerOrderingTimedout(t *testing.T) {

	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" {
		t.Parallel()
	}
	var options *AgentOptions
	if "" == "true" {
		options = &AgentOptions{EnableTaskENI: true}
	}
	agent := RunAgent(t, options)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.25.0")

	td, err := GetTaskDefinition("container-ordering-timedout-windows")

	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {

			tmpTask, err := agent.StartAWSVPCTask("container-ordering-timedout-windows", nil)

			if err != nil {
				t.Fatalf("Could not start task in awsvpc mode: %v", err)
			}
			testTasks = append(testTasks, tmpTask)
		}
	} else {
		testTasks, err = agent.StartMultipleTasks(t, td, 1)
		if err != nil {
			t.Fatalf("Could not start task: %v", err)
		}
	}

	timeout, err := time.ParseDuration("2m")
	if err != nil {
		t.Fatalf("Could not parse timeout: %#v", err)
	}

	for _, testTask := range testTasks {
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
		}

		defer agent.SweepTask(testTask)
	}

}

// TestContainerOrdering Check that container ordering works fine
func TestContainerOrdering(t *testing.T) {

	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" {
		t.Parallel()
	}
	var options *AgentOptions
	if "" == "true" {
		options = &AgentOptions{EnableTaskENI: true}
	}
	agent := RunAgent(t, options)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.25.0")

	td, err := GetTaskDefinition("container-ordering-windows")

	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("container-ordering-windows", nil)

			if err != nil {
				t.Fatalf("Could not start task in awsvpc mode: %v", err)
			}
			testTasks = append(testTasks, tmpTask)
		}
	} else {
		testTasks, err = agent.StartMultipleTasks(t, td, 1)
		if err != nil {
			t.Fatalf("Could not start task: %v", err)
		}
	}

	timeout, err := time.ParseDuration("3m")
	if err != nil {
		t.Fatalf("Could not parse timeout: %#v", err)
	}

	for _, testTask := range testTasks {
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
		}

		defer agent.SweepTask(testTask)
	}

}

// TestDataVolume Check that basic data volumes work
func TestDataVolume(t *testing.T) {

	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" {
		t.Parallel()
	}
	var options *AgentOptions
	if "" == "true" {
		options = &AgentOptions{EnableTaskENI: true}
	}
	agent := RunAgent(t, options)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.0.0")

	td, err := GetTaskDefinition("datavolume-windows")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("datavolume-windows", nil)
			if err != nil {
				t.Fatalf("Could not start task in awsvpc mode: %v", err)
			}
			testTasks = append(testTasks, tmpTask)
		}
	} else {
		testTasks, err = agent.StartMultipleTasks(t, td, 1)
		if err != nil {
			t.Fatalf("Could not start task: %v", err)
		}
	}

	timeout, err := time.ParseDuration("5m")
	if err != nil {
		t.Fatalf("Could not parse timeout: %#v", err)
	}

	for _, testTask := range testTasks {
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
		}

		if exit, ok := testTask.ContainerExitcode("exit"); !ok || exit != 42 {
			t.Errorf("Expected exit to exit with 42; actually exited (%v) with %v", ok, exit)
		}

		defer agent.SweepTask(testTask)
	}

}

// TestHostname Check that hostname works
func TestHostname(t *testing.T) {

	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" {
		t.Parallel()
	}
	var options *AgentOptions
	if "" == "true" {
		options = &AgentOptions{EnableTaskENI: true}
	}
	agent := RunAgent(t, options)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.5.0")

	td, err := GetTaskDefinition("hostname-windows")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("hostname-windows", nil)
			if err != nil {
				t.Fatalf("Could not start task in awsvpc mode: %v", err)
			}
			testTasks = append(testTasks, tmpTask)
		}
	} else {
		testTasks, err = agent.StartMultipleTasks(t, td, 1)
		if err != nil {
			t.Fatalf("Could not start task: %v", err)
		}
	}

	timeout, err := time.ParseDuration("2m")
	if err != nil {
		t.Fatalf("Could not parse timeout: %#v", err)
	}

	for _, testTask := range testTasks {
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
		}

		if exit, ok := testTask.ContainerExitcode("exit"); !ok || exit != 42 {
			t.Errorf("Expected exit to exit with 42; actually exited (%v) with %v", ok, exit)
		}

		defer agent.SweepTask(testTask)
	}

}

// TestSimpleExit Tests that the basic premis of this testing fromwork works (e.g. exit codes go through, etc)
func TestSimpleExit(t *testing.T) {

	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" {
		t.Parallel()
	}
	var options *AgentOptions
	if "" == "true" {
		options = &AgentOptions{EnableTaskENI: true}
	}
	agent := RunAgent(t, options)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.0.0")

	td, err := GetTaskDefinition("simple-exit-windows")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("simple-exit-windows", nil)
			if err != nil {
				t.Fatalf("Could not start task in awsvpc mode: %v", err)
			}
			testTasks = append(testTasks, tmpTask)
		}
	} else {
		testTasks, err = agent.StartMultipleTasks(t, td, 1)
		if err != nil {
			t.Fatalf("Could not start task: %v", err)
		}
	}

	timeout, err := time.ParseDuration("2m")
	if err != nil {
		t.Fatalf("Could not parse timeout: %#v", err)
	}

	for _, testTask := range testTasks {
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
		}

		if exit, ok := testTask.ContainerExitcode("exit"); !ok || exit != 42 {
			t.Errorf("Expected exit to exit with 42; actually exited (%v) with %v", ok, exit)
		}

		defer agent.SweepTask(testTask)
	}

}

// TestTaskLocalVolume Verify that task specific Docker volume works as expected
func TestTaskLocalVolume(t *testing.T) {

	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" {
		t.Parallel()
	}
	var options *AgentOptions
	if "" == "true" {
		options = &AgentOptions{EnableTaskENI: true}
	}
	agent := RunAgent(t, options)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.20.0")

	td, err := GetTaskDefinition("task-local-vol-windows")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("task-local-vol-windows", nil)
			if err != nil {
				t.Fatalf("Could not start task in awsvpc mode: %v", err)
			}
			testTasks = append(testTasks, tmpTask)
		}
	} else {
		testTasks, err = agent.StartMultipleTasks(t, td, 1)
		if err != nil {
			t.Fatalf("Could not start task: %v", err)
		}
	}

	timeout, err := time.ParseDuration("2m")
	if err != nil {
		t.Fatalf("Could not parse timeout: %#v", err)
	}

	for _, testTask := range testTasks {
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
		}

		if exit, ok := testTask.ContainerExitcode("exit"); !ok || exit != 42 {
			t.Errorf("Expected exit to exit with 42; actually exited (%v) with %v", ok, exit)
		}

		defer agent.SweepTask(testTask)
	}

}

// TestWorkingDir Check that working dir works
func TestWorkingDir(t *testing.T) {

	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" {
		t.Parallel()
	}
	var options *AgentOptions
	if "" == "true" {
		options = &AgentOptions{EnableTaskENI: true}
	}
	agent := RunAgent(t, options)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.5.0")

	td, err := GetTaskDefinition("working-dir-windows")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("working-dir-windows", nil)
			if err != nil {
				t.Fatalf("Could not start task in awsvpc mode: %v", err)
			}
			testTasks = append(testTasks, tmpTask)
		}
	} else {
		testTasks, err = agent.StartMultipleTasks(t, td, 1)
		if err != nil {
			t.Fatalf("Could not start task: %v", err)
		}
	}

	timeout, err := time.ParseDuration("2m")
	if err != nil {
		t.Fatalf("Could not parse timeout: %#v", err)
	}

	for _, testTask := range testTasks {
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
		}

		if exit, ok := testTask.ContainerExitcode("exit"); !ok || exit != 42 {
			t.Errorf("Expected exit to exit with 42; actually exited (%v) with %v", ok, exit)
		}

		defer agent.SweepTask(testTask)
	}

}
