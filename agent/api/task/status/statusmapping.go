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

package status

import (
	"github.com/aws/amazon-ecs-agent/agent/containerresource/containerstatus"
)

// MapContainerToTaskStatus maps the container status to the corresponding task status. The
// transition map is illustrated below.
//
// Container: None -> Pulled -> Created -> Running -> Provisioned -> Stopped -> Zombie
//
// Task     : None ->     Created       ->         Running        -> Stopped
func MapContainerToTaskStatus(knownState containerstatus.ContainerStatus, steadyState containerstatus.ContainerStatus) TaskStatus {
	switch knownState {
	case containerstatus.ContainerStatusNone:
		return TaskStatusNone
	case steadyState:
		return TaskRunning
	case containerstatus.ContainerCreated:
		return TaskCreated
	case containerstatus.ContainerStopped:
		return TaskStopped
	}

	if knownState == containerstatus.ContainerRunning && steadyState == containerstatus.ContainerResourcesProvisioned {
		return TaskCreated
	}

	return TaskStatusNone
}

// MapTaskToContainerStatus maps the task status to the corresponding container status
func MapTaskToContainerStatus(desiredState TaskStatus, steadyState containerstatus.ContainerStatus) containerstatus.ContainerStatus {
	switch desiredState {
	case TaskStatusNone:
		return containerstatus.ContainerStatusNone
	case TaskCreated:
		return containerstatus.ContainerCreated
	case TaskRunning:
		return steadyState
	case TaskStopped:
		return containerstatus.ContainerStopped
	}
	return containerstatus.ContainerStatusNone
}
