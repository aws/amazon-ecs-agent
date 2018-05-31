// +build unit

// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package task

import (
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/stretchr/testify/assert"
)

func TestTaskStatus(t *testing.T) {
	// Effectively set containerStatus := ContainerStatusNone, we expect the task state
	// to be TaskStatusNone
	var containerStatus apicontainer.ContainerStatus
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerRunning), TaskStatusNone)
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerResourcesProvisioned), TaskStatusNone)

	// When container state is PULLED, Task state is still NONE
	containerStatus = apicontainer.ContainerPulled
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerRunning), TaskStatusNone)
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerResourcesProvisioned), TaskStatusNone)

	// When container state is CREATED, Task state is CREATED as well
	containerStatus = apicontainer.ContainerCreated
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerRunning), TaskCreated)
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerResourcesProvisioned), TaskCreated)

	containerStatus = apicontainer.ContainerRunning
	// When container state is RUNNING and steadyState is RUNNING, Task state is RUNNING as well
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerRunning), TaskRunning)
	// When container state is RUNNING and steadyState is RESOURCES_PROVISIONED, Task state
	// still CREATED
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerResourcesProvisioned), TaskCreated)

	containerStatus = apicontainer.ContainerResourcesProvisioned
	// When container state is RESOURCES_PROVISIONED and steadyState is RESOURCES_PROVISIONED,
	// Task state is RUNNING
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerResourcesProvisioned), TaskRunning)

	// When container state is STOPPED, Task state is STOPPED as well
	containerStatus = apicontainer.ContainerStopped
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerRunning), TaskStopped)
	assert.Equal(t, MapContainerToTaskStatus(containerStatus, apicontainer.ContainerResourcesProvisioned), TaskStopped)
}
