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
	"encoding/json"
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
)

// These bits of information should be enough to reconstruct the entire
// DockerTaskEngine state
type savedState struct {
	Tasks         []*api.Task
	IdToContainer map[string]*api.DockerContainer `json:"IdToContainer"` // DockerId -> api.DockerContainer
	IdToTask      map[string]string               `json:"IdToTask"`      // DockerId -> taskarn
	ImageStates   []*image.ImageState
}

func (state *DockerTaskEngineState) MarshalJSON() ([]byte, error) {
	var toSave savedState
	state.lock.RLock()
	defer state.lock.RUnlock()
	toSave = savedState{
		Tasks:         state.allTasks(),
		IdToContainer: state.idToContainer,
		IdToTask:      state.idToTask,
		ImageStates:   state.allImageStates(),
	}
	return json.Marshal(toSave)
}

func (state *DockerTaskEngineState) UnmarshalJSON(data []byte) error {
	var saved savedState

	err := json.Unmarshal(data, &saved)
	if err != nil {
		return err
	}
	// reset it by just creating a new one and swapping shortly.
	// This also means we don't have to lock for the remainder of this function
	// because we are the only ones with a reference to clean
	clean := newDockerTaskEngineState()

	for _, task := range saved.Tasks {
		clean.AddTask(task)
	}
	// add image states
	for _, imageState := range saved.ImageStates {
		clean.AddImageState(imageState)
	}
	for id, container := range saved.IdToContainer {
		taskArn, ok := saved.IdToTask[id]
		if !ok {
			return fmt.Errorf("Could not unmarshal state; incomplete save. There was no task for docker id %s", id)
		}
		task, ok := clean.TaskByArn(taskArn)
		if !ok {
			return fmt.Errorf("Could not unmarshal state; incomplete save. There was no task for arn %s", taskArn)
		}

		// The container.Container pointers *must* match the task's container
		// pointers for things to operate correctly; update them here
		taskContainer, err := task.ContainerByName(container.Container.Name)
		if err != nil {
			return fmt.Errorf("Could not resolve a container into a task based on name: " + task.String() + " -- " + container.String())
		}
		container.Container = taskContainer
		//pointer matching now; everyone happy
		clean.AddContainer(container, task)

	}

	*state = *clean
	return nil
}
