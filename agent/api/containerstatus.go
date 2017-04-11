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

package api

const (
	// ContainerStatusNone is the zero state of a container; this container has not completed pull
	ContainerStatusNone ContainerStatus = iota
	// ContainerPulled represents a container which has had the image pulled
	ContainerPulled
	// ContainerCreated represents a container that has been created
	ContainerCreated
	// ContainerRunning represents a container that has started
	ContainerRunning
	// ContainerResourcesProvisioned represents a container that has completed provisioning all of its
	// resources. Non-internal containers (containers present in the task definition) transition to
	// this state without doing any additional work. However, containers that are added to a task
	// by the ECS Agent would possibly need to perform additional actions before they can be
	// considered "ready" and contribute to the progress of a task. For example, the "pause" container
	// would be provisioned by invoking CNI plugins
	ContainerResourcesProvisioned
	// ContainerStopped represents a container that has stopped
	ContainerStopped
	// ContainerZombie is an "impossible" state that is used as the maximum
	ContainerZombie
)

// ContainerStatus is an enumeration of valid states in the container lifecycle
type ContainerStatus int32

var containerStatusMap = map[string]ContainerStatus{
	"NONE":                  ContainerStatusNone,
	"PULLED":                ContainerPulled,
	"CREATED":               ContainerCreated,
	"RUNNING":               ContainerRunning,
	"RESOURCES_PROVISIONED": ContainerResourcesProvisioned,
	"STOPPED":               ContainerStopped,
}

// String returns a human readable string representation of this object
func (cs ContainerStatus) String() string {
	for k, v := range containerStatusMap {
		if v == cs {
			return k
		}
	}
	return "NONE"
}

// TaskStatus maps the container status to the corresponding task status. The
// transition map is illustreated below.
//
// Container: None -> Created -> Running -> Provisioned -> Stopped -> Zombie
//
// Task     : None ->     Created        -> Running     -> Stopped
func (cs *ContainerStatus) TaskStatus() TaskStatus {
	switch *cs {
	case ContainerStatusNone:
		return TaskStatusNone
	case ContainerCreated:
		return TaskCreated
	case ContainerRunning:
		return TaskCreated
	case ContainerResourcesProvisioned:
		return TaskRunning
	case ContainerStopped:
		return TaskStopped
	}
	return TaskStatusNone
}

// ShouldReportToBackend returns true if the container status is recognized as a
// valid state by ECS. Note that not all container statuses are recognized by ECS
// or map to ECS states
func (cs *ContainerStatus) ShouldReportToBackend() bool {
	return *cs == ContainerResourcesProvisioned || *cs == ContainerStopped
}

// BackendStatus maps the internal container status in the agent to that in the
// backend
func (cs *ContainerStatus) BackendStatus() ContainerStatus {
	if *cs == ContainerResourcesProvisioned {
		return ContainerRunning
	}

	if *cs == ContainerStopped {
		return ContainerStopped
	}

	return ContainerStatusNone
}

// Terminal returns true if the container status is STOPPED
func (cs ContainerStatus) Terminal() bool {
	return cs == ContainerStopped
}

// GetContainerSteadyStateStatus returns the "steady state" for a container. This
// used to be ContainerRunning prior to the addition of the
// ContainerResourcesProvisioned state. Now, we expect all containers to reach
// the new ContainerResourcesProvisioned state
func GetContainerSteadyStateStatus() ContainerStatus {
	return ContainerResourcesProvisioned
}
