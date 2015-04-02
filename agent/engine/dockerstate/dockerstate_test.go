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

package dockerstate

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
)

func TestCreateDockerTaskEngineState(t *testing.T) {
	state := NewDockerTaskEngineState()

	if _, ok := state.ContainerById("test"); ok {
		t.Error("Empty state should not have a test container")
	}

	if _, ok := state.ContainerMapByArn("test"); ok {
		t.Error("Empty state should not have a test task")
	}

	if _, ok := state.TaskById("test"); ok {
		t.Error("Empty state should not have a test taskid")
	}

	if len(state.AllTasks()) != 0 {
		t.Error("Empty state should have no tasks")
	}
}

func TestAddTask(t *testing.T) {
	state := NewDockerTaskEngineState()

	testTask := &api.Task{Arn: "test"}
	state.AddOrUpdateTask(testTask)

	if len(state.AllTasks()) != 1 {
		t.Error("Should have 1 task")
	}

	task, ok := state.TaskByArn("test")
	if !ok {
		t.Error("Couldn't find the test task")
	}
	if task.Arn != "test" {
		t.Error("Wrong task retrieved")
	}
}

func TestTwophaseAddContainer(t *testing.T) {
	state := NewDockerTaskEngineState()
	testTask := &api.Task{Arn: "test", Containers: []*api.Container{&api.Container{
		Name: "testContainer",
	}}}
	state.AddOrUpdateTask(testTask)

	state.AddContainer(&api.DockerContainer{DockerName: "dockerName", Container: testTask.Containers[0]}, testTask)

	if len(state.AllTasks()) != 1 {
		t.Fatal("Should have 1 task")
	}

	task, ok := state.TaskByArn("test")
	if !ok {
		t.Error("Couldn't find the test task")
	}
	if task.Arn != "test" {
		t.Error("Wrong task retrieved")
	}

	containerMap, ok := state.ContainerMapByArn("test")
	if !ok {
		t.Fatal("Could not get container map")
	}

	container, ok := containerMap["testContainer"]
	if !ok {
		t.Fatal("Could not get container")
	}
	if container.DockerName != "dockerName" {
		t.Fatal("Incorrect docker name")
	}
	if container.DockerId != "" {
		t.Fatal("DockerID Should be blank")
	}

	state.AddContainer(&api.DockerContainer{DockerName: "dockerName", Container: testTask.Containers[0], DockerId: "did"}, testTask)

	containerMap, ok = state.ContainerMapByArn("test")
	if !ok {
		t.Fatal("Could not get container map")
	}

	container, ok = containerMap["testContainer"]
	if !ok {
		t.Fatal("Could not get container")
	}
	if container.DockerName != "dockerName" {
		t.Fatal("Incorrect docker name")
	}
	if container.DockerId != "did" {
		t.Fatal("DockerID should have been updated")
	}

	container, ok = state.ContainerById("did")
	if !ok {
		t.Fatal("Could not get container by id")
	}
	if container.DockerName != "dockerName" || container.DockerId != "did" {
		t.Fatal("Incorrect container fetched")
	}
}
