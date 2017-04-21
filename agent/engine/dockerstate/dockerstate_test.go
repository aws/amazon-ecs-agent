// +build !integration
// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
)

func TestCreateDockerTaskEngineState(t *testing.T) {
	state := NewTaskEngineState()

	if _, ok := state.ContainerByID("test"); ok {
		t.Error("Empty state should not have a test container")
	}

	if _, ok := state.ContainerMapByArn("test"); ok {
		t.Error("Empty state should not have a test task")
	}

	if _, ok := state.TaskByShortID("test"); ok {
		t.Error("Empty state should not have a test taskid")
	}

	if _, ok := state.TaskByID("test"); ok {
		t.Error("Empty state should not have a test taskid")
	}

	if len(state.AllTasks()) != 0 {
		t.Error("Empty state should have no tasks")
	}

	if len(state.AllImageStates()) != 0 {
		t.Error("Empty state should have no image states")
	}
}

func TestAddTask(t *testing.T) {
	state := NewTaskEngineState()

	testTask := &api.Task{Arn: "test"}
	state.AddTask(testTask)

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
	state := NewTaskEngineState()
	testTask := &api.Task{Arn: "test", Containers: []*api.Container{&api.Container{
		Name: "testContainer",
	}}}
	state.AddTask(testTask)

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
	if container.DockerID != "" {
		t.Fatal("DockerID Should be blank")
	}

	state.AddContainer(&api.DockerContainer{DockerName: "dockerName", Container: testTask.Containers[0], DockerID: "did"}, testTask)

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
	if container.DockerID != "did" {
		t.Fatal("DockerID should have been updated")
	}

	container, ok = state.ContainerByID("did")
	if !ok {
		t.Fatal("Could not get container by id")
	}
	if container.DockerName != "dockerName" || container.DockerID != "did" {
		t.Fatal("Incorrect container fetched")
	}
}

func TestRemoveTask(t *testing.T) {
	state := NewTaskEngineState()
	testContainer := &api.Container{
		Name: "c1",
	}
	testDockerContainer := &api.DockerContainer{
		DockerID:  "did",
		Container: testContainer,
	}
	testTask := &api.Task{
		Arn:        "t1",
		Containers: []*api.Container{testContainer},
	}

	state.AddTask(testTask)
	state.AddContainer(testDockerContainer, testTask)

	tasks := state.AllTasks()
	if len(tasks) != 1 {
		t.Error("Expected only 1 task")
	}

	state.RemoveTask(testTask)

	tasks = state.AllTasks()
	if len(tasks) != 0 {
		t.Error("Expected task to be removed")
	}
}

func TestAddImageState(t *testing.T) {
	state := NewTaskEngineState()

	testImage := &image.Image{ImageID: "sha256:imagedigest"}
	testImageState := &image.ImageState{Image: testImage}
	state.AddImageState(testImageState)

	if len(state.AllImageStates()) != 1 {
		t.Error("Error adding image state")
	}

	for _, imageState := range state.AllImageStates() {
		if imageState.Image.ImageID != testImage.ImageID {
			t.Error("Error in retrieving image state added")
		}
	}
}

func TestAddEmptyImageState(t *testing.T) {
	state := NewTaskEngineState()
	state.AddImageState(nil)

	if len(state.AllImageStates()) != 0 {
		t.Error("Error adding empty image state")
	}
}

func TestAddEmptyIdImageState(t *testing.T) {
	state := NewTaskEngineState()

	testImage := &image.Image{ImageID: ""}
	testImageState := &image.ImageState{Image: testImage}
	state.AddImageState(testImageState)

	if len(state.AllImageStates()) != 0 {
		t.Error("Error adding image state with empty Image Id")
	}
}

func TestRemoveImageState(t *testing.T) {
	state := NewTaskEngineState()

	testImage := &image.Image{ImageID: "sha256:imagedigest"}
	testImageState := &image.ImageState{Image: testImage}
	state.AddImageState(testImageState)

	if len(state.AllImageStates()) != 1 {
		t.Error("Error adding image state")
	}
	state.RemoveImageState(testImageState)
	if len(state.AllImageStates()) != 0 {
		t.Error("Error removing image state")
	}
}

func TestRemoveEmptyImageState(t *testing.T) {
	state := NewTaskEngineState()

	testImage := &image.Image{ImageID: "sha256:imagedigest"}
	testImageState := &image.ImageState{Image: testImage}
	state.AddImageState(testImageState)

	if len(state.AllImageStates()) != 1 {
		t.Error("Error adding image state")
	}
	state.RemoveImageState(nil)
	if len(state.AllImageStates()) == 0 {
		t.Error("Error removing empty image state")
	}
}

func TestRemoveNonExistingImageState(t *testing.T) {
	state := NewTaskEngineState()

	testImage := &image.Image{ImageID: "sha256:imagedigest"}
	testImageState := &image.ImageState{Image: testImage}
	state.AddImageState(testImageState)

	if len(state.AllImageStates()) != 1 {
		t.Error("Error adding image state")
	}
	testImage1 := &image.Image{ImageID: "sha256:imagedigest1"}
	testImageState1 := &image.ImageState{Image: testImage1}
	state.RemoveImageState(testImageState1)
	if len(state.AllImageStates()) == 0 {
		t.Error("Error removing incorrect image state")
	}
}
