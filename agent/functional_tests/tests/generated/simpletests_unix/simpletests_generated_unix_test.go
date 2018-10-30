// +build functional,!windows

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

// TestAddAndDropCapabilities checks that adding and dropping Linux capabilities work
func TestAddAndDropCapabilities(t *testing.T) {

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

	td, err := GetTaskDefinition("add-drop-capabilities")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("add-drop-capabilities", nil)
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

// TestHostNameAwsvpc checks ec2 private dns was added as the container hostnmae
func TestHostNameAwsvpc(t *testing.T) {

	// Parallel is opt in because resource constraints could cause test failures
	// on smaller instances
	if os.Getenv("ECS_FUNCTIONAL_PARALLEL") != "" {
		t.Parallel()
	}
	var options *AgentOptions
	if "true" == "true" {
		options = &AgentOptions{EnableTaskENI: true}
	}
	agent := RunAgent(t, options)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.17.3")

	td, err := GetTaskDefinition("hostname-awsvpc")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "true" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("hostname-awsvpc", nil)
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

		if exit, ok := testTask.ContainerExitcode("exit"); !ok || exit != 0 {
			t.Errorf("Expected exit to exit with 0; actually exited (%v) with %v", ok, exit)
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

	td, err := GetTaskDefinition("datavolume")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("datavolume", nil)
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

// TestDataVolume2 Verify that more complex datavolumes (including empty and volumes-from) work as expected; see Related
func TestDataVolume2(t *testing.T) {

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
	agent.RequireVersion(">1.0.0")

	td, err := GetTaskDefinition("datavolume2")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("datavolume2", nil)
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

// TestDevices checks that adding devices works
func TestDevices(t *testing.T) {

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

	td, err := GetTaskDefinition("devices")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("devices", nil)
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

// TestDisableNetworking Check that disable networking works
func TestDisableNetworking(t *testing.T) {

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

	td, err := GetTaskDefinition("network-disabled")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("network-disabled", nil)
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

// TestDnsSearchDomains Check that dns search domains works
func TestDnsSearchDomains(t *testing.T) {

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

	td, err := GetTaskDefinition("dns-search-domains")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("dns-search-domains", nil)
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

// TestDnsServers Check that dns servers works
func TestDnsServers(t *testing.T) {

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

	td, err := GetTaskDefinition("dns-servers")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("dns-servers", nil)
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

// TestExtraHosts Check that extra hosts works
func TestExtraHosts(t *testing.T) {

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

	td, err := GetTaskDefinition("extra-hosts")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("extra-hosts", nil)
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

	td, err := GetTaskDefinition("hostname")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("hostname", nil)
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

// TestInitProcessEnabled checks that enabling init process works
func TestInitProcessEnabled(t *testing.T) {

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
	agent.RequireVersion(">=1.15.0")

	td, err := GetTaskDefinition("init-process")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("init-process", nil)
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

// TestInteractiveTty checks that interactive tty works
func TestInteractiveTty(t *testing.T) {

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

	td, err := GetTaskDefinition("interactive-tty")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("interactive-tty", nil)
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

// TestLinkVolumeDependencies Tests that the dependency graph of task definitions is resolved correctly
func TestLinkVolumeDependencies(t *testing.T) {

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

	td, err := GetTaskDefinition("network-link-2")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("network-link-2", nil)
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

// TestNetworkLink Tests that basic network linking works
func TestNetworkLink(t *testing.T) {

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

	td, err := GetTaskDefinition("network-link")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("network-link", nil)
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

// TestParallelPull check docker pull in parallel works for docker >= 1.11.1
func TestParallelPull(t *testing.T) {

	// Test only available for docker version >=1.11.1
	RequireDockerVersion(t, ">=1.11.1")

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

	td, err := GetTaskDefinition("parallel-pull")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 4; i++ {
			tmpTask, err := agent.StartAWSVPCTask("parallel-pull", nil)
			if err != nil {
				t.Fatalf("Could not start task in awsvpc mode: %v", err)
			}
			testTasks = append(testTasks, tmpTask)
		}
	} else {
		testTasks, err = agent.StartMultipleTasks(t, td, 4)
		if err != nil {
			t.Fatalf("Could not start task: %v", err)
		}
	}

	timeout, err := time.ParseDuration("1m")
	if err != nil {
		t.Fatalf("Could not parse timeout: %#v", err)
	}

	// Make sure the task is running
	for _, testTask := range testTasks {
		err = testTask.WaitRunning(timeout)
		if err != nil {
			t.Errorf("Timed out waiting for task to reach running. Error %v, task %v", err, testTask)
		}
	}

	// Cleanup, stop all the tasks and wait for the containers to be stopped
	for _, testTask := range testTasks {
		err = testTask.Stop()
		if err != nil {
			t.Errorf("Failed to stop task, Error %v, task %v", err, testTask)
		}
	}

	for _, testTask := range testTasks {
		err = testTask.WaitStopped(timeout)
		if err != nil {
			t.Fatalf("Timed out waiting for task to reach stopped. Error %#v, task %#v", err, testTask)
		}

		defer agent.SweepTask(testTask)
	}

}

// TestPrivileged Check that privileged works
func TestPrivileged(t *testing.T) {

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

	td, err := GetTaskDefinition("privileged")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("privileged", nil)
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

// TestReadonlyRootfs Check that readonly rootfs works
func TestReadonlyRootfs(t *testing.T) {

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

	td, err := GetTaskDefinition("readonly-rootfs")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("readonly-rootfs", nil)
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

// TestSecurityOptNoNewPrivileges Check that security-opt=no-new-privileges works
func TestSecurityOptNoNewPrivileges(t *testing.T) {

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
	agent.RequireVersion(">=1.12.1")

	td, err := GetTaskDefinition("security-opt-nonewprivileges")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("security-opt-nonewprivileges", nil)
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

// TestShmSize checks that setting size of shared memory volume works
func TestShmSize(t *testing.T) {

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
	agent.RequireVersion(">=1.11.0")

	td, err := GetTaskDefinition("shmsize")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("shmsize", nil)
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

	td, err := GetTaskDefinition("simple-exit")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("simple-exit", nil)
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

// TestSysctl checks that sysctl works
func TestSysctl(t *testing.T) {

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
	agent.RequireVersion(">=1.14.4")

	td, err := GetTaskDefinition("sysctl")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("sysctl", nil)
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

	td, err := GetTaskDefinition("task-local-vol")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("task-local-vol", nil)
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

// TestTmpfs checks that adding tmpfs volume works
func TestTmpfs(t *testing.T) {

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
	agent.RequireVersion(">=1.11.0")

	td, err := GetTaskDefinition("tmpfs")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("tmpfs", nil)
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

// TestNofilesULimit Check that nofiles ulimit works
func TestNofilesULimit(t *testing.T) {

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

	td, err := GetTaskDefinition("nofiles-ulimit")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("nofiles-ulimit", nil)
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

// TestUserNobody Check that user works
func TestUserNobody(t *testing.T) {

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

	td, err := GetTaskDefinition("user-nobody")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("user-nobody", nil)
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

	td, err := GetTaskDefinition("working-dir")
	if err != nil {
		t.Fatalf("Could not register task definition: %v", err)
	}
	var testTasks []*TestTask
	if "" == "true" {
		for i := 0; i < 1; i++ {
			tmpTask, err := agent.StartAWSVPCTask("working-dir", nil)
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
