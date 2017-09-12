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

package eventhandler

import (
	"container/list"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestSendsEventsOneContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	handler := NewTaskHandler(stateManager)
	taskarn := "taskarn"

	var wg sync.WaitGroup
	wg.Add(1)

	// Trivial: one container, no errors
	contEvent1 := containerEvent(taskarn)
	contEvent2 := containerEvent(taskarn)
	taskEvent2 := taskEvent(taskarn)

	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(func(change api.TaskStateChange) {
		assert.Equal(t, 2, len(change.Containers))
		assert.Equal(t, taskarn, change.Containers[0].TaskArn)
		assert.Equal(t, taskarn, change.Containers[1].TaskArn)
		wg.Done()
	})

	handler.AddStateChangeEvent(contEvent1, client)
	handler.AddStateChangeEvent(contEvent2, client)
	handler.AddStateChangeEvent(taskEvent2, client)

	wg.Wait()
}

func TestSendsEventsOneEventRetries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	handler := NewTaskHandler(stateManager)
	taskarn := "taskarn"

	var wg sync.WaitGroup
	wg.Add(2)

	retriable := utils.NewRetriableError(utils.NewRetriable(true), errors.New("test"))
	taskEvent := taskEvent(taskarn)

	gomock.InOrder(
		client.EXPECT().SubmitTaskStateChange(gomock.Any()).Return(retriable).Do(func(interface{}) { wg.Done() }),
		client.EXPECT().SubmitTaskStateChange(gomock.Any()).Return(nil).Do(func(interface{}) { wg.Done() }),
	)

	handler.AddStateChangeEvent(taskEvent, client)

	wg.Wait()
}

func TestSendsEventsConcurrentLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	handler := NewTaskHandler(stateManager)

	completeStateChange := make(chan bool, concurrentEventCalls+1)
	var wg sync.WaitGroup

	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Times(concurrentEventCalls + 1).Do(func(interface{}) {
		wg.Done()
		<-completeStateChange
	})

	// Test concurrency; ensure it doesn't attempt to send more than
	// concurrentEventCalls at once
	wg.Add(concurrentEventCalls)

	// Put on N+1 events
	for i := 0; i < concurrentEventCalls+1; i++ {
		handler.AddStateChangeEvent(taskEvent("concurrent_"+strconv.Itoa(i)), client)
	}
	wg.Wait()

	// accept a single change event
	wg.Add(1)
	completeStateChange <- true
	wg.Wait()

	// ensure the remaining requests are completed
	for i := 0; i < concurrentEventCalls; i++ {
		completeStateChange <- true
	}
}

func TestSendsEventsContainerDifferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	handler := NewTaskHandler(stateManager)
	taskarn := "taskarn"

	var wg sync.WaitGroup
	wg.Add(1)

	// Test container event replacement doesn't happen
	contEvent1 := containerEvent(taskarn)
	contEvent2 := containerEventStopped(taskarn)
	taskEvent := taskEvent(taskarn)

	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(func(change api.TaskStateChange) {
		assert.Equal(t, 2, len(change.Containers))
		assert.Equal(t, taskarn, change.Containers[0].TaskArn)
		assert.Equal(t, api.ContainerRunning, change.Containers[0].Status)
		assert.Equal(t, taskarn, change.Containers[1].TaskArn)
		assert.Equal(t, api.ContainerStopped, change.Containers[1].Status)
		wg.Done()
	})

	handler.AddStateChangeEvent(contEvent1, client)
	handler.AddStateChangeEvent(contEvent2, client)
	handler.AddStateChangeEvent(taskEvent, client)

	wg.Wait()
}

func TestSendsEventsTaskDifferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	handler := NewTaskHandler(stateManager)
	taskarnA := "taskarnA"
	taskarnB := "taskarnB"

	var wg sync.WaitGroup
	wg.Add(2)

	var wgAddEvent sync.WaitGroup
	wgAddEvent.Add(1)

	// Test task event replacement doesn't happen
	taskEventA := taskEvent(taskarnA)
	contEventA1 := containerEvent(taskarnA)

	contEventB1 := containerEvent(taskarnB)
	contEventB2 := containerEventStopped(taskarnB)
	taskEventB := taskEventStopped(taskarnB)

	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(func(change api.TaskStateChange) {
		assert.Equal(t, taskarnA, change.TaskARN)
		wgAddEvent.Done()
		wg.Done()
	})

	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(func(change api.TaskStateChange) {
		assert.Equal(t, taskarnB, change.TaskARN)
		wg.Done()
	})

	handler.AddStateChangeEvent(contEventB1, client)
	handler.AddStateChangeEvent(contEventA1, client)
	handler.AddStateChangeEvent(contEventB2, client)

	handler.AddStateChangeEvent(taskEventA, client)
	wgAddEvent.Wait()

	handler.AddStateChangeEvent(taskEventB, client)

	wg.Wait()
}

func TestSendsEventsDedupe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	handler := NewTaskHandler(stateManager)
	taskarnA := "taskarnA"
	taskarnB := "taskarnB"

	var wg sync.WaitGroup
	wg.Add(1)

	// Verify that a task doesn't get sent if we already have 'sent' it
	task1 := taskEvent(taskarnA)
	task1.(api.TaskStateChange).Task.SetSentStatus(api.TaskRunning)
	cont1 := containerEvent(taskarnA)
	cont1.(api.ContainerStateChange).Container.SetSentStatus(api.ContainerRunning)

	handler.AddStateChangeEvent(cont1, client)
	handler.AddStateChangeEvent(task1, client)

	task2 := taskEvent(taskarnB)
	task2.(api.TaskStateChange).Task.SetSentStatus(api.TaskStatusNone)
	cont2 := containerEvent(taskarnB)
	cont2.(api.ContainerStateChange).Container.SetSentStatus(api.ContainerRunning)

	// Expect to send a task status but not a container status
	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(func(change api.TaskStateChange) {
		assert.Equal(t, 1, len(change.Containers))
		assert.Equal(t, taskarnB, change.Containers[0].TaskArn)
		assert.Equal(t, taskarnB, change.TaskARN)
		wg.Done()
	})

	handler.AddStateChangeEvent(cont2, client)
	handler.AddStateChangeEvent(task2, client)

	wg.Wait()
}

// TestCleanupTaskEventAfterSubmit tests the map of task event is removed after
// calling submittaskstatechange
func TestCleanupTaskEventAfterSubmit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	stateManager := statemanager.NewNoopStateManager()
	client := mock_api.NewMockECSClient(ctrl)

	handler := NewTaskHandler(stateManager)
	taskarn := "taskarn"
	taskarn2 := "taskarn2"

	var wg sync.WaitGroup
	wg.Add(3)

	taskEvent1 := taskEvent(taskarn)
	taskEvent2 := taskEvent(taskarn)
	taskEvent3 := taskEvent(taskarn2)

	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(
		func(change api.TaskStateChange) {
			wg.Done()
		}).Times(3)

	handler.AddStateChangeEvent(taskEvent1, client)
	handler.AddStateChangeEvent(taskEvent2, client)
	handler.AddStateChangeEvent(taskEvent3, client)

	wg.Wait()
	assert.Len(t, handler.tasksToEvents, 0)
}

func TestShouldBeSent(t *testing.T) {
	sendableEvent := newSendableContainerEvent(api.ContainerStateChange{
		Status: api.ContainerStopped,
	})

	if sendableEvent.taskShouldBeSent() {
		t.Error("Container event should not be sent as a task")
	}

	if !sendableEvent.containerShouldBeSent() {
		t.Error("Container should be sent if it's the first try")
	}
}

func containerEvent(arn string) statechange.Event {
	return api.ContainerStateChange{TaskArn: arn, ContainerName: "containerName", Status: api.ContainerRunning, Container: &api.Container{}}
}

func containerEventStopped(arn string) statechange.Event {
	return api.ContainerStateChange{TaskArn: arn, ContainerName: "containerName", Status: api.ContainerStopped, Container: &api.Container{}}
}

func taskEvent(arn string) statechange.Event {
	return api.TaskStateChange{TaskARN: arn, Status: api.TaskRunning, Task: &api.Task{}}
}

func taskEventStopped(arn string) statechange.Event {
	return api.TaskStateChange{TaskARN: arn, Status: api.TaskStopped, Task: &api.Task{}}
}

func TestENISentStatusChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	task := &api.Task{
		Arn: "taskarn",
	}

	eniAttachment := &api.ENIAttachment{
		TaskARN:          "taskarn",
		AttachStatusSent: false,
		ExpiresAt:        time.Now().Add(time.Second),
	}
	timeoutFunc := func() {
		eniAttachment.AttachStatusSent = true
	}
	assert.NoError(t, eniAttachment.StartTimer(timeoutFunc))

	sendableTaskEvent := newSendableTaskEvent(api.TaskStateChange{
		Attachment: eniAttachment,
		TaskARN:    "taskarn",
		Status:     api.TaskStatusNone,
		Task:       task,
	})

	client.EXPECT().SubmitTaskStateChange(gomock.Any()).Return(nil)

	events := list.New()
	events.PushBack(sendableTaskEvent)
	handler := NewTaskHandler(statemanager.NewNoopStateManager())
	handler.SubmitTaskEvents(&eventList{
		events: events,
	}, client)

	assert.True(t, eniAttachment.AttachStatusSent)
}
