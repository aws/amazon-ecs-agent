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
	"UNKNOWN": TaskStatusUnknown,
	"CREATED": TaskCreated,
	"RUNNING": TaskRunning,
	"STOPPED": TaskStopped,
	"DEAD":    TaskDead,
}

func (ts *TaskStatus) String() string {
	for k, v := range taskStatusMap {
		if v == *ts {
			return k
		}
	}
	return "UNKNOWN"
}

var containerStatusMap = map[string]ContainerStatus{
	"NONE":    ContainerStatusNone,
	"UNKNOWN": ContainerStatusUnknown,
	"PULLED":  ContainerPulled,
	"CREATED": ContainerCreated,
	"RUNNING": ContainerRunning,
	"STOPPED": ContainerStopped,
	"DEAD":    ContainerDead,
}

func (cs *ContainerStatus) String() string {
	for k, v := range containerStatusMap {
		if v == *cs {
			return k
		}
	}
	return "UNKNOWN"
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
	case ContainerDead:
		return TaskDead
	}
	return TaskStatusUnknown
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
	case TaskDead:
		return ContainerDead
	}
	return ContainerStatusUnknown
}

func (cs *ContainerStatus) Terminal() bool {
	if cs == nil {
		return false
	}
	return *cs == ContainerStopped || *cs == ContainerDead
}

func (ts *TaskStatus) Terminal() bool {
	if ts == nil {
		return false
	}
	return *ts == TaskStopped || *ts == TaskDead
}
