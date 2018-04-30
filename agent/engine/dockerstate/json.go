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
	"errors"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
)

// These bits of information should be enough to reconstruct the entire
// DockerTaskEngine state
type savedState struct {
	Tasks          []*apitask.Task
	IdToContainer  map[string]*apicontainer.DockerContainer `json:"IdToContainer"` // DockerId -> apicontainer.DockerContainer
	IdToTask       map[string]string                        `json:"IdToTask"`      // DockerId -> taskarn
	ImageStates    []*image.ImageState
	ENIAttachments []*apieni.ENIAttachment `json:"ENIAttachments"`
	IPToTask       map[string]string       `json:"IPToTask"`
}

func (state *DockerTaskEngineState) MarshalJSON() ([]byte, error) {
	state.lock.RLock()
	defer state.lock.RUnlock()
	toSave := savedState{
		Tasks:          state.allTasksUnsafe(),
		IdToContainer:  state.idToContainer,
		IdToTask:       state.idToTask,
		ImageStates:    state.allImageStatesUnsafe(),
		ENIAttachments: state.allENIAttachmentsUnsafe(),
		IPToTask:       state.ipToTask,
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
			return errors.New("Could not unmarshal state; incomplete save. There was no task for docker id " + id)
		}
		task, ok := clean.TaskByArn(taskArn)
		if !ok {
			return errors.New("Could not unmarshal state; incomplete save. There was no task for arn " + taskArn)
		}

		// The container.Container pointers *must* match the task's container
		// pointers for things to operate correctly; update them here
		taskContainer, ok := task.ContainerByName(container.Container.Name)
		if !ok {
			return errors.New("Could not resolve a container into a task based on name: " + task.String() + " -- " + container.String())
		}
		container.Container = taskContainer
		//pointer matching now; everyone happy
		clean.AddContainer(container, task)
	}

	for _, eniAttachment := range saved.ENIAttachments {
		clean.AddENIAttachment(eniAttachment)
	}

	for ipAddr, taskARN := range saved.IPToTask {
		clean.AddTaskIPAddress(ipAddr, taskARN)
	}

	*state = *clean
	return nil
}
