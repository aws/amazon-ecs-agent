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

	// run precheck to shake out all the errors before resetting state.
	precheckState := newDockerTaskEngineState()
	for _, task := range saved.Tasks {
		precheckState.AddTask(task)
	}
	for id, container := range saved.IdToContainer {
		taskArn, ok := saved.IdToTask[id]
		if !ok {
			return errors.New("Could not unmarshal state; incomplete save. There was no task for docker id " + id)
		}
		task, ok := precheckState.TaskByArn(taskArn)
		if !ok {
			return errors.New("Could not unmarshal state; incomplete save. There was no task for arn " + taskArn)
		}
		_, ok = task.ContainerByName(container.Container.Name)
		if !ok {
			return errors.New("Could not resolve a container into a task based on name: " + task.String() + " -- " + container.String())
		}
	}

	// reset state and safely populate with saved
	state.Reset()
	for _, task := range saved.Tasks {
		state.AddTask(task)
	}
	for _, imageState := range saved.ImageStates {
		state.AddImageState(imageState)
	}
	for id, container := range saved.IdToContainer {
		taskArn, _ := saved.IdToTask[id]
		task, _ := state.TaskByArn(taskArn)
		taskContainer, _ := task.ContainerByName(container.Container.Name)
		container.Container = taskContainer
		state.AddContainer(container, task)
	}
	for _, eniAttachment := range saved.ENIAttachments {
		state.AddENIAttachment(eniAttachment)
	}
	for ipAddr, taskARN := range saved.IPToTask {
		state.AddTaskIPAddress(ipAddr, taskARN)
	}

	return nil
}
