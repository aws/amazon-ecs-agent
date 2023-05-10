//go:build unit
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
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	apieni "github.com/aws/amazon-ecs-agent/ecs-agent/api/eni"

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
			// ManagedAgent state change should be sent if SentStatus is != Status
			// regardless of task status
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskStatusNone,
				},
				ManagedAgents: []api.ManagedAgentStateChange{
					{
						TaskArn: "test_task_arn",
						Name:    "test_agent",
						Container: &apicontainer.Container{
							ManagedAgentsUnsafe: []apicontainer.ManagedAgent{
								{
									Name: "test_agent",
									ManagedAgentState: apicontainer.ManagedAgentState{
										Status:     apicontainerstatus.ManagedAgentRunning,
										SentStatus: apicontainerstatus.ManagedAgentStatusNone,
									},
								},
							},
						},
						Status: apicontainerstatus.ManagedAgentStatusNone,
						Reason: "test_reason",
					},
				},
			}),
			shouldBeSent: true,
		},
		{
			// ManagedAgent state change should be sent if SentStatus is != Status
			// regardless of task status
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskStatusNone,
				},
				ManagedAgents: []api.ManagedAgentStateChange{
					{
						TaskArn: "test_task_arn",
						Name:    "test_agent",
						Container: &apicontainer.Container{
							ManagedAgentsUnsafe: []apicontainer.ManagedAgent{
								{
									Name: "test_agent",
									ManagedAgentState: apicontainer.ManagedAgentState{
										Status:     apicontainerstatus.ManagedAgentRunning,
										SentStatus: apicontainerstatus.ManagedAgentStopped,
									},
								},
							},
						},
						Status: apicontainerstatus.ManagedAgentStatusNone,
						Reason: "test_reason",
					},
				},
			}),
			shouldBeSent: true,
		},
		{
			// ManagedAgent state change should not be sent if SentStatus is == Status
			// regardless of task status
			event: newSendableTaskEvent(api.TaskStateChange{
				Status: apitaskstatus.TaskStatusNone,
				Task: &apitask.Task{
					SentStatusUnsafe: apitaskstatus.TaskStatusNone,
				},
				ManagedAgents: []api.ManagedAgentStateChange{
					{
						TaskArn: "test_task_arn",
						Name:    "test_agent",
						Container: &apicontainer.Container{
							ManagedAgentsUnsafe: []apicontainer.ManagedAgent{
								{
									Name: "test_agent",
									ManagedAgentState: apicontainer.ManagedAgentState{
										Status:     apicontainerstatus.ManagedAgentRunning,
										SentStatus: apicontainerstatus.ManagedAgentRunning,
									},
								},
							},
						},
						Status: apicontainerstatus.ManagedAgentStatusNone,
						Reason: "test_reason",
					},
				},
			}),
			shouldBeSent: false,
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
		t.Run(fmt.Sprintf("Event[%v] should be sent[%t]", tc.event.toFields(), tc.shouldBeSent), func(t *testing.T) {
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
					AttachmentInfo: attachmentinfo.AttachmentInfo{
						ExpiresAt:        time.Unix(time.Now().Unix()-1, 0),
						AttachStatusSent: false,
					},
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
					AttachmentInfo: attachmentinfo.AttachmentInfo{
						ExpiresAt:        time.Unix(time.Now().Unix()+10, 0),
						AttachStatusSent: true,
					},
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
					AttachmentInfo: attachmentinfo.AttachmentInfo{
						ExpiresAt:        time.Unix(time.Now().Unix()+10, 0),
						AttachStatusSent: false,
					},
				},
			}),
			attachmentShouldBeSent: true,
			taskShouldBeSent:       false,
		},
	} {
		t.Run(fmt.Sprintf("Event[%v] should be sent[attachment=%t;task=%t]",
			tc.event.toFields(), tc.attachmentShouldBeSent, tc.taskShouldBeSent), func(t *testing.T) {
			assert.Equal(t, tc.attachmentShouldBeSent, tc.event.taskAttachmentShouldBeSent())
			assert.Equal(t, tc.taskShouldBeSent, tc.event.taskShouldBeSent())
			assert.Equal(t, false, tc.event.containerShouldBeSent())
		})
	}
}

func TestSetTaskSentStatus(t *testing.T) {
	dataClient := newTestDataClient(t)

	testManagedAgent := apicontainer.ManagedAgent{
		ManagedAgentState: apicontainer.ManagedAgentState{},
		Name:              "dummyAgent",
	}
	testContainer := &apicontainer.Container{
		Name:                testConainerName,
		TaskARNUnsafe:       testTaskARN,
		ManagedAgentsUnsafe: []apicontainer.ManagedAgent{testManagedAgent},
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
		ManagedAgents: []api.ManagedAgentStateChange{
			{
				TaskArn:   testTaskARN,
				Name:      "dummyAgent",
				Container: testContainer,
				Status:    apicontainerstatus.ManagedAgentRunning,
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
		ManagedAgents: []api.ManagedAgentStateChange{
			{
				TaskArn:   testTaskARN,
				Name:      "dummyAgent",
				Container: testContainer,
				Status:    apicontainerstatus.ManagedAgentStopped,
			},
		},
	})

	setTaskChangeSent(taskStoppedStateChange, dataClient)
	assert.Equal(t, testTask.GetSentStatus(), apitaskstatus.TaskStopped)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)

	updatedManagedAgent, _ := testContainer.GetManagedAgentByName("dummyAgent")
	assert.Equal(t, apicontainerstatus.ManagedAgentStopped, updatedManagedAgent.SentStatus)

	setTaskChangeSent(taskRunningStateChange, dataClient)
	assert.Equal(t, testTask.GetSentStatus(), apitaskstatus.TaskStopped)
	assert.Equal(t, testContainer.GetSentStatus(), apicontainerstatus.ContainerStopped)

	updatedManagedAgent, _ = testContainer.GetManagedAgentByName("dummyAgent")
	assert.Equal(t, apicontainerstatus.ManagedAgentRunning, updatedManagedAgent.SentStatus)

	tasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, apitaskstatus.TaskStopped, tasks[0].GetSentStatus())

	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)
	assert.Equal(t, apicontainerstatus.ContainerStopped, containers[0].Container.GetSentStatus())
	updatedManagedAgent, _ = containers[0].Container.GetManagedAgentByName("dummyAgent")
	assert.Equal(t, apicontainerstatus.ManagedAgentRunning, updatedManagedAgent.SentStatus)
}

func TestSetContainerSentStatus(t *testing.T) {
	dataClient := newTestDataClient(t)

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
	dataClient := newTestDataClient(t)

	testAttachment := &apieni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			AttachStatusSent: true,
			ExpiresAt:        time.Unix(time.Now().Unix()+100, 0),
			AttachmentARN:    testAttachmentARN,
		},
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

func newTestDataClient(t *testing.T) data.Client {
	testDir := t.TempDir()

	testClient, err := data.NewWithSetup(testDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, testClient.Close())
	})
	return testClient
}
