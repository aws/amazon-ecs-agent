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
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"

	"github.com/stretchr/testify/assert"
)

// TestTaskStateChangeMetadataGetter validates the data stored in
// TaskMetadataGetter to check its implementation. The Getter should
// fetch this data from the task reference, not the state change itself.
func TestTaskStateChangeMetadataGetter(t *testing.T) {
	taskArn := "arn:123"
	t1 := time.Now()
	t2 := t1.Add(1)
	t3 := t2.Add(1)
	task := &apitask.Task{
		Arn:                      taskArn,
		SentStatusUnsafe:         apitaskstatus.TaskRunning,
		PullStartedAtUnsafe:      t1,
		PullStoppedAtUnsafe:      t2,
		ExecutionStoppedAtUnsafe: t3,
	}

	metadataGetter := newTaskMetadataGetter(task)

	change := &ecs.TaskStateChange{
		MetadataGetter: metadataGetter,
	}

	assert.NotNil(t, change.MetadataGetter)
	assert.Equal(t, false, change.MetadataGetter.GetTaskIsNil())
	assert.Equal(t, apitaskstatus.TaskRunningString, change.MetadataGetter.GetTaskSentStatusString())
	assert.Equal(t, t1, change.MetadataGetter.GetTaskPullStartedAt())
	assert.Equal(t, t2, change.MetadataGetter.GetTaskPullStoppedAt())
	assert.Equal(t, t3, change.MetadataGetter.GetTaskExecutionStoppedAt())
}

// TestContainerStateChangeMetadataGetter validates the data stored in
// ContainerMetadataGetter to check its implementation. The Getter should
// fetch this data from the container reference, not the state change itself.
func TestContainerStateChangeMetadataGetter(t *testing.T) {
	dockerID := "dockerID"
	container := &apicontainer.Container{
		RuntimeID:        dockerID,
		Essential:        true,
		SentStatusUnsafe: apicontainerstatus.ContainerRunning,
	}

	metadataGetter := newContainerMetadataGetter(container)

	change := &ecs.ContainerStateChange{
		MetadataGetter: metadataGetter,
	}

	assert.NotNil(t, change.MetadataGetter)
	assert.Equal(t, false, change.MetadataGetter.GetContainerIsNil())
	assert.Equal(t, apicontainerstatus.ContainerRunning.String(), change.MetadataGetter.GetContainerSentStatusString())
	assert.Equal(t, dockerID, change.MetadataGetter.GetContainerRuntimeID())
	assert.Equal(t, true, change.MetadataGetter.GetContainerIsEssential())
}
