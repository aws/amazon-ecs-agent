// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
)

// Implementation of the ContainerStateChange ContainerMetadataGetter Interface.
type containerMetadataGetter struct {
	container *apicontainer.Container
}

func newContainerMetadataGetter(container *apicontainer.Container) *containerMetadataGetter {
	return &containerMetadataGetter{
		container: container,
	}
}

// GetContainerIsNil returns the existence of the container.
func (cmg *containerMetadataGetter) GetContainerIsNil() bool {
	return cmg.container == nil
}

// GetContainerSentStatusString returns the last unsafe status sent to
// the ECS SubmitContainerStateChange API.
func (cmg *containerMetadataGetter) GetContainerSentStatusString() string {
	return cmg.container.GetSentStatus().String()
}

// GetContainerRuntimeID returns the Docker ID of the container.
func (cmg *containerMetadataGetter) GetContainerRuntimeID() string {
	return cmg.container.GetRuntimeID()
}

// GetContainerIsEssential returns whether the container is essential
// or not.
func (cmg *containerMetadataGetter) GetContainerIsEssential() bool {
	return cmg.container.IsEssential()
}

// Implementation of the TaskStateChange TaskMetadataGetter Interface.
type taskMetadataGetter struct {
	task *apitask.Task
}

func newTaskMetadataGetter(task *apitask.Task) *taskMetadataGetter {
	return &taskMetadataGetter{
		task: task,
	}
}

// GetTaskIsNil returns whether the task exists or not.
func (tmg *taskMetadataGetter) GetTaskIsNil() bool {
	return tmg.task == nil
}

// GetTaskSentStatusString returns the SentStatus of the task.
func (tmg *taskMetadataGetter) GetTaskSentStatusString() string {
	return tmg.task.GetSentStatus().String()
}

// GetTaskPullStartedAt returns the pull started at time of the task.
func (tmg *taskMetadataGetter) GetTaskPullStartedAt() time.Time {
	return tmg.task.GetPullStartedAt()
}

// GetTaskPullStartedAt returns the pull stopped at time of the task.
func (tmg *taskMetadataGetter) GetTaskPullStoppedAt() time.Time {
	return tmg.task.GetPullStoppedAt()
}

// GetTaskPullStartedAt returns the execution stopped at time of the task.
func (tmg *taskMetadataGetter) GetTaskExecutionStoppedAt() time.Time {
	return tmg.task.GetExecutionStoppedAt()
}
