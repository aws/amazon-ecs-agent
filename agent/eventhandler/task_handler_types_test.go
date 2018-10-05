// +build unit

// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/stretchr/testify/assert"
)

func TestShouldContainerEventBeSent(t *testing.T) {
	event := newSendableContainerEvent(api.ContainerStateChange{
		Status: apicontainerstatus.ContainerStopped,
	})
	assert.Equal(t, true, event.containerShouldBeSent())
	assert.Equal(t, false, event.taskShouldBeSent())
}

func TestShouldTaskEventBeSent(t *testing.T) {
	for _, tc := range []struct {
		event        *sendableEvent
		shouldBeSent bool
	}{
		{
			// We don't send a task event to backend if task status == NONE
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskStatusNone,
				},
			}),
			shouldBeSent: false,
		},
		{
			// task status == RUNNING should be sent to backend
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskRunning,
				Task:   &apitask.Task{},
			}),
			shouldBeSent: true,
		},
		{
			// task event will not be sent if sent status >= task status
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskRunning,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskRunning,
				},
			}),
			shouldBeSent: false,
		},
		{
			// this is a valid event as task status >= sent status
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStopped,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskRunning,
				},
			}),
			shouldBeSent: true,
		},
		{
			// Even though the task has been sent, there's a container
			// state change that needs to be sent
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskRunning,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskRunning,
				},
				Containers: []api.ContainerStateChange{
					{
						Container: &apicontainer.Container{
							SentStatusUnsafe:  apicontainerstatus.ContainerRunning,
							KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
						},
					},
					{
						Container: &apicontainer.Container{
							SentStatusUnsafe:  apicontainerstatus.ContainerRunning,
							KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
						},
					},
				},
			}),
			shouldBeSent: true,
		},
		{
			// Container state change should be sent regardless of task
			// status.
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskStatusNone,
				},
				Containers: []api.ContainerStateChange{
					{
						Container: &apicontainer.Container{
							SentStatusUnsafe:  apicontainerstatus.ContainerStatusNone,
							KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
						},
					},
				},
			}),
			shouldBeSent: true,
		},
		{
			// All states sent, nothing to send
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskRunning,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskRunning,
				},
				Containers: []api.ContainerStateChange{
					{
						Container: &apicontainer.Container{
							SentStatusUnsafe:  apicontainerstatus.ContainerRunning,
							KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
						},
					},
					{
						Container: &apicontainer.Container{
							SentStatusUnsafe:  apicontainerstatus.ContainerStopped,
							KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
						},
					},
				},
			}),
			shouldBeSent: false,
		},
	} {
		t.Run(fmt.Sprintf("Event[%s] should be sent[%t]", tc.event.toString(), tc.shouldBeSent), func(t *testing.T) {
			assert.Equal(t, tc.shouldBeSent, tc.event.taskShouldBeSent())
			assert.Equal(t, false, tc.event.containerShouldBeSent())
			assert.Equal(t, false, tc.event.taskAttachmentShouldBeSent())
		})
	}
}

func TestShouldTaskAttachmentEventBeSent(t *testing.T) {
	for _, tc := range []struct {
		event                  *sendableEvent
		attachmentShouldBeSent bool
		taskShouldBeSent       bool
	}{
		{
			// ENI Attachment is only sent if task status == NONE
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStopped,
				Task:   &apitask.Task{},
			}),
			attachmentShouldBeSent: false,
			taskShouldBeSent:       true,
		},
		{
			// ENI Attachment is only sent if task status == NONE and if
			// the event has a non nil attachment object
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
			}),
			attachmentShouldBeSent: false,
			taskShouldBeSent:       false,
		},
		{
			// ENI Attachment is only sent if task status == NONE and if
			// the event has a non nil attachment object and if expiration
			// ack timeout is set for future
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
				Attachment: &apieni.ENIAttachment{
					ExpiresAt:        time.Unix(time.Now().Unix()-1, 0),
					AttachStatusSent: false,
				},
			}),
			attachmentShouldBeSent: false,
			taskShouldBeSent:       false,
		},
		{
			// ENI Attachment is only sent if task status == NONE and if
			// the event has a non nil attachment object and if expiration
			// ack timeout is set for future and if attachment status hasn't
			// already been sent
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
				Attachment: &apieni.ENIAttachment{
					ExpiresAt:        time.Unix(time.Now().Unix()+10, 0),
					AttachStatusSent: true,
				},
			}),
			attachmentShouldBeSent: false,
			taskShouldBeSent:       false,
		},
		{
			// Valid attachment event, ensure that its sent
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
				Attachment: &apieni.ENIAttachment{
					ExpiresAt:        time.Unix(time.Now().Unix()+10, 0),
					AttachStatusSent: false,
				},
			}),
			attachmentShouldBeSent: true,
			taskShouldBeSent:       false,
		},
	} {
		t.Run(fmt.Sprintf("Event[%s] should be sent[attachment=%t;task=%t]",
			tc.event.toString(), tc.attachmentShouldBeSent, tc.taskShouldBeSent), func(t *testing.T) {
			assert.Equal(t, tc.attachmentShouldBeSent, tc.event.taskAttachmentShouldBeSent())
			assert.Equal(t, tc.taskShouldBeSent, tc.event.taskShouldBeSent())
			assert.Equal(t, false, tc.event.containerShouldBeSent())
		})
	}
}

func TestSetTaskSentStatus(t *testing.T) {
	testContainer := &apicontainer.Container{}
	testTask := &apitask.Task{}

	taskRunningStateChange := newSendableTaskEvent(api.TaskStateChange{
		Status: apitaskstatus.TaskRunning,
		Task:   testTask,
		Containers: []api.ContainerStateChange{
			{
				Status:    apicontainerstatus.ContainerRunning,
				Container: testContainer,
			},
		},
	})
	taskStoppedStateChange := newSendableTaskEvent(api.TaskStateChange{
		Status: apitaskstatus.TaskStopped,
		Task:   testTask,
		Containers: []api.ContainerStateChange{
			{
				Status:    apicontainerstatus.ContainerStopped,
				Container: testContainer,
			},
		},
	})

	setTaskChangeSent(taskStoppedStateChange)
	assert.Equal(t, testTask.GetSentStatus(), apitaskstatus.TaskStopped)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)
	setTaskChangeSent(taskRunningStateChange)
	assert.Equal(t, testTask.GetSentStatus(), apitaskstatus.TaskStopped)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)
}

func TestSetContainerSentStatus(t *testing.T) {
	testContainer := &apicontainer.Container{}

	containerRunningStateChange := newSendableContainerEvent(api.ContainerStateChange{
		Status:    apicontainerstatus.ContainerRunning,
		Container: testContainer,
	})
	containerStoppedStateChange := newSendableContainerEvent(api.ContainerStateChange{
		Status:    apicontainerstatus.ContainerStopped,
		Container: testContainer,
	})

	setContainerChangeSent(containerStoppedStateChange)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)
	setContainerChangeSent(containerRunningStateChange)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)
}
