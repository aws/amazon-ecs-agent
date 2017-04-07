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
	// ContainerStopped represents a container that has stopped
	ContainerStopped
	// ContainerZombie is an "impossible" state that is used as the maximum
	ContainerZombie
)

// ContainerStatus is an enumeration of valid states in the container lifecycle
type ContainerStatus int32

var containerStatusMap = map[string]ContainerStatus{
	"NONE":    ContainerStatusNone,
	"PULLED":  ContainerPulled,
	"CREATED": ContainerCreated,
	"RUNNING": ContainerRunning,
	"STOPPED": ContainerStopped,
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

// TaskStatus maps the container status to the corresponding task status
func (cs *ContainerStatus) TaskStatus() TaskStatus {
	switch *cs {
	case ContainerStatusNone:
		return TaskStatusNone
	case ContainerCreated:
		return TaskCreated
	case ContainerRunning:
		return TaskRunning
	case ContainerStopped:
		return TaskStopped
	}
	return TaskStatusNone
}

// BackendRecognized returns true if the container status is recognized as a valid state
// by ECS. Note that not all container statuses are recognized by ECS or map to ECS
// states
func (cs *ContainerStatus) BackendRecognized() bool {
	return *cs == ContainerRunning || *cs == ContainerStopped
}

// Terminal returns true if the container status is STOPPED
func (cs ContainerStatus) Terminal() bool {
	return cs == ContainerStopped
}
