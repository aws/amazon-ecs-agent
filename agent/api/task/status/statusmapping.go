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

package status

import (
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
)

// MapContainerToTaskStatus maps the container status to the corresponding task status. The
// transition map is illustrated below.
//
// Container: None -> Pulled -> Created -> Running -> Provisioned -> Stopped -> Zombie
//
// Task     : None ->     Created       ->         Running        -> Stopped
func MapContainerToTaskStatus(knownState apicontainerstatus.ContainerStatus, steadyState apicontainerstatus.ContainerStatus) TaskStatus {
	switch knownState {
	case apicontainerstatus.ContainerStatusNone:
		return TaskStatusNone
	case steadyState:
		return TaskRunning
	case apicontainerstatus.ContainerCreated:
		return TaskCreated
	case apicontainerstatus.ContainerStopped:
		return TaskStopped
	}

	if knownState == apicontainerstatus.ContainerRunning && steadyState == apicontainerstatus.ContainerResourcesProvisioned {
		return TaskCreated
	}

	return TaskStatusNone
}

// MapTaskToContainerStatus maps the task status to the corresponding container status
func MapTaskToContainerStatus(desiredState TaskStatus, steadyState apicontainerstatus.ContainerStatus) apicontainerstatus.ContainerStatus {
	switch desiredState {
	case TaskStatusNone:
		return apicontainerstatus.ContainerStatusNone
	case TaskCreated:
		return apicontainerstatus.ContainerCreated
	case TaskRunning:
		return steadyState
	case TaskStopped:
		return apicontainerstatus.ContainerStopped
	}
	return apicontainerstatus.ContainerStatusNone
}
