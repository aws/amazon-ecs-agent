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

package eventhandler

import (
	"container/list"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

// Maximum number of tasks that may be handled at once by the taskHandler
const CONCURRENT_EVENT_CALLS = 3

// a state change that may have a container and, optionally, a task event to
// send
type sendableEvent struct {
	containerSent bool
	taskSent      bool

	api.ContainerStateChange
}

func newSendableEvent(event api.ContainerStateChange) *sendableEvent {
	return &sendableEvent{
		containerSent:        false,
		taskSent:             false,
		ContainerStateChange: event,
	}
}

func (event *sendableEvent) taskShouldBeSent() bool {
	if event.TaskStatus == api.TaskStatusNone {
		return false // container only event
	}
	if event.taskSent || event.Task.SentStatus >= event.TaskStatus {
		return false // redundant event
	}
	return true
}

func (event *sendableEvent) containerShouldBeSent() bool {
	if event.containerSent || event.Container.SentStatus >= event.Status {
		return false
	}
	return true
}

type eventList struct {
	sending    bool // whether the list is already being handled
	sync.Mutex      // Locks both the list and sending bool
	*list.List      // list of *sendableEvents
}

type taskHandler struct {
	submitSemaphore utils.Semaphore       // Semaphore on the number of tasks that may be handled at once
	taskMap         map[string]*eventList // arn:*eventList map so events may be serialized per task

	sync.RWMutex // Lock for the taskMap
}

func newTaskHandler() taskHandler {
	taskMap := make(map[string]*eventList)
	submitSemaphore := utils.NewSemaphore(CONCURRENT_EVENT_CALLS)

	return taskHandler{
		taskMap:         taskMap,
		submitSemaphore: submitSemaphore,
	}
}
