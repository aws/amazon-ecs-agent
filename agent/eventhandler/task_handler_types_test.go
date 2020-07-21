// +build unit

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

package eventhandler

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/data"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testConainerName  = "test-name"
	testTaskARN       = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc"
	testAttachmentARN = "arn:aws:ecs:us-west-2:1234567890:attachment/abc"
)

func newSendableContainerEvent(event api.ContainerStateChange) *sendableEvent {
	return &sendableEvent{
		isContainerEvent: true,
		containerSent:    false,
		containerChange:  event,
	}
}

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
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	testContainer := &apicontainer.Container{
		Name:          testConainerName,
		TaskARNUnsafe: testTaskARN,
	}
	testTask := &apitask.Task{
		Arn:        testTaskARN,
		Containers: []*apicontainer.Container{testContainer},
	}

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

	setTaskChangeSent(taskStoppedStateChange, dataClient)
	assert.Equal(t, testTask.GetSentStatus(), apitaskstatus.TaskStopped)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)
	setTaskChangeSent(taskRunningStateChange, dataClient)
	assert.Equal(t, testTask.GetSentStatus(), apitaskstatus.TaskStopped)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)

	tasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, apitaskstatus.TaskStopped, tasks[0].GetSentStatus())

	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)
	assert.Equal(t, apicontainerstatus.ContainerStopped, containers[0].Container.GetSentStatus())
}

func TestSetContainerSentStatus(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	testContainer := &apicontainer.Container{
		Name:          testConainerName,
		TaskARNUnsafe: testTaskARN,
	}

	containerRunningStateChange := newSendableContainerEvent(api.ContainerStateChange{
		Status:    apicontainerstatus.ContainerRunning,
		Container: testContainer,
	})
	containerStoppedStateChange := newSendableContainerEvent(api.ContainerStateChange{
		Status:    apicontainerstatus.ContainerStopped,
		Container: testContainer,
	})

	setContainerChangeSent(containerStoppedStateChange, dataClient)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)
	setContainerChangeSent(containerRunningStateChange, dataClient)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)

	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)
	assert.Equal(t, apicontainerstatus.ContainerStopped, containers[0].Container.GetSentStatus())
}

func TestSetAttachmentSentStatus(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	testAttachment := &apieni.ENIAttachment{
		AttachStatusSent: true,
		ExpiresAt:        time.Unix(time.Now().Unix()+100, 0),
		AttachmentARN:    testAttachmentARN,
	}
	require.NoError(t, testAttachment.StartTimer(func() {}))
	event := newSendableTaskEvent(api.TaskStateChange{
		Status:     apitaskstatus.TaskStatusNone,
		Attachment: testAttachment,
	})
	setTaskAttachmentSent(event, dataClient)
	atts, err := dataClient.GetENIAttachments()
	require.NoError(t, err)
	assert.Len(t, atts, 1)
	assert.True(t, atts[0].IsSent())
}

func newTestDataClient(t *testing.T) (data.Client, func()) {
	testDir, err := ioutil.TempDir("", "agent_eventhandler_unit_test")
	require.NoError(t, err)

	testClient, err := data.NewWithSetup(testDir)

	cleanup := func() {
		require.NoError(t, testClient.Close())
		require.NoError(t, os.RemoveAll(testDir))
	}
	return testClient, cleanup
}
