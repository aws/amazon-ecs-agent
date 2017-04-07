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
	// TaskStatusNone is the zero state of a task; this task has been received but no further progress has completed
	TaskStatusNone TaskStatus = iota
	// TaskPulled represents a task which has had all its container images pulled, but not all have yet progressed passed pull
	TaskPulled
	// TaskCreated represents a task which has had all its containers created
	TaskCreated
	// TaskRunning represents a task which has had all its containers started
	TaskRunning
	// TaskStopped represents a task in which all containers are stopped
	TaskStopped
)

// TaskStatus is an enumeration of valid states in the task lifecycle
type TaskStatus int32

var taskStatusMap = map[string]TaskStatus{
	"NONE":    TaskStatusNone,
	"CREATED": TaskCreated,
	"RUNNING": TaskRunning,
	"STOPPED": TaskStopped,
}

// String returns a human readable string representation of this object
func (ts TaskStatus) String() string {
	for k, v := range taskStatusMap {
		if v == ts {
			return k
		}
	}
	return "NONE"
}

// Mapping task status in the agent to that in the backend
func (ts *TaskStatus) BackendStatus() string {
	switch *ts {
	case TaskRunning:
		fallthrough
	case TaskStopped:
		return ts.String()
	}
	return "PENDING"
}

// BackendRecognized returns true if the task status is recognized as a valid state
// by ECS. Note that not all task statuses are recognized by ECS or map to ECS
// states
func (ts *TaskStatus) BackendRecognized() bool {
	return *ts == TaskRunning || *ts == TaskStopped
}

// ContainerStatus maps the task status to the corresponding container status
func (ts *TaskStatus) ContainerStatus() ContainerStatus {
	switch *ts {
	case TaskStatusNone:
		return ContainerStatusNone
	case TaskCreated:
		return ContainerCreated
	case TaskRunning:
		return ContainerRunning
	case TaskStopped:
		return ContainerStopped
	}
	return ContainerStatusNone
}

// Terminal returns true if the Task status is STOPPED
func (ts TaskStatus) Terminal() bool {
	return ts == TaskStopped
}
