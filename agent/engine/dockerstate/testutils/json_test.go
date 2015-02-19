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

package testutils

import (
	"encoding/json"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
)

func createTestContainer(num int) *api.Container {
	return &api.Container{
		Name:          "busybox-" + strconv.Itoa(num),
		Image:         "busybox:latest",
		Essential:     true,
		DesiredStatus: api.ContainerRunning,
	}
}

func createTestTask(arn string, numContainers int) *api.Task {
	task := &api.Task{
		Arn:           arn,
		Family:        arn,
		Version:       "1",
		DesiredStatus: api.TaskRunning,
		Containers:    []*api.Container{},
	}

	for i := 0; i < numContainers; i++ {
		task.Containers = append(task.Containers, createTestContainer(i+1))
	}
	return task
}

func decodeEqual(t *testing.T, state *dockerstate.DockerTaskEngineState) *dockerstate.DockerTaskEngineState {
	data, err := json.Marshal(&state)
	if err != nil {
		t.Error(err)
	}
	otherState := dockerstate.NewDockerTaskEngineState()
	err = json.Unmarshal(data, &otherState)
	if err != nil {
		t.Error(err)
	}
	if !DockerStatesEqual(state, otherState) {
		debug.PrintStack()
		t.Error("States were not equal")
	}
	return otherState
}

func TestJsonEncoding(t *testing.T) {
	state := dockerstate.NewDockerTaskEngineState()
	decodeEqual(t, state)

	testState := dockerstate.NewDockerTaskEngineState()
	testTask := createTestTask("test1", 1)
	testState.AddOrUpdateTask(testTask)
	for i, cont := range testTask.Containers {
		testState.AddContainer(&api.DockerContainer{DockerId: "docker" + strconv.Itoa(i), DockerName: "someName", Container: cont}, testTask)
	}
	other := decodeEqual(t, testState)
	_, ok := other.ContainerMapByArn("test1")
	if !ok {
		t.Error("Could not retrieve expected task")
	}

}
