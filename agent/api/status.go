// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

var taskStatusMap = map[string]TaskStatus{
	"NONE":    TaskStatusNone,
	"CREATED": TaskCreated,
	"RUNNING": TaskRunning,
	"STOPPED": TaskStopped,
}

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

func (ts *TaskStatus) BackendRecognized() bool {
	return *ts == TaskRunning || *ts == TaskStopped
}

var containerStatusMap = map[string]ContainerStatus{
	"NONE":    ContainerStatusNone,
	"PULLED":  ContainerPulled,
	"CREATED": ContainerCreated,
	"RUNNING": ContainerRunning,
	"STOPPED": ContainerStopped,
}

func (cs ContainerStatus) String() string {
	for k, v := range containerStatusMap {
		if v == cs {
			return k
		}
	}
	return "NONE"
}

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

func (cs *ContainerStatus) BackendRecognized() bool {
	return *cs == ContainerRunning || *cs == ContainerStopped
}

func (cs ContainerStatus) Terminal() bool {
	return cs == ContainerStopped
}

func (ts TaskStatus) Terminal() bool {
	return ts == TaskStopped
}
