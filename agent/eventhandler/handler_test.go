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
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func containerEvent(arn string) statechange.Event {
	return api.ContainerStateChange{TaskArn: arn, ContainerName: "containerName", Status: api.ContainerRunning, Container: &api.Container{}}
}

func containerEventStopped(arn string) statechange.Event {
	return api.ContainerStateChange{TaskArn: arn, ContainerName: "containerName", Status: api.ContainerStopped, Container: &api.Container{}}
}

func taskEvent(arn string) statechange.Event {
	return api.TaskStateChange{TaskArn: arn, Status: api.TaskRunning, Task: &api.Task{}}
}

func taskEventStopped(arn string) statechange.Event {
	return api.TaskStateChange{TaskArn: arn, Status: api.TaskStopped, Task: &api.Task{}}
}

func TestSendsEventsOneContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	handler := NewTaskHandler()

	var wg sync.WaitGroup
	wg.Add(3)

	// Trivial: one container, no errors
	contEvent1 := containerEvent("1")
	contEvent2 := containerEvent("2")
	taskEvent2 := taskEvent("2")

	client.EXPECT().SubmitContainerStateChange(contEvent1.(api.ContainerStateChange)).Do(func(interface{}) { wg.Done() })
	client.EXPECT().SubmitContainerStateChange(contEvent2.(api.ContainerStateChange)).Do(func(interface{}) { wg.Done() })
	client.EXPECT().SubmitTaskStateChange(taskEvent2.(api.TaskStateChange)).Do(func(interface{}) { wg.Done() })

	handler.AddStateChangeEvent(contEvent1, client)
	handler.AddStateChangeEvent(contEvent2, client)
	handler.AddStateChangeEvent(taskEvent2, client)

	wg.Wait()
}

func TestSendsEventsOneEventRetries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	handler := NewTaskHandler()

	var wg sync.WaitGroup
	wg.Add(2)

	retriable := utils.NewRetriableError(utils.NewRetriable(true), errors.New("test"))
	contEvent1 := containerEvent("1")

	gomock.InOrder(
		client.EXPECT().SubmitContainerStateChange(contEvent1.(api.ContainerStateChange)).Return(retriable).Do(func(interface{}) { wg.Done() }),
		client.EXPECT().SubmitContainerStateChange(contEvent1.(api.ContainerStateChange)).Return(nil).Do(func(interface{}) { wg.Done() }),
	)

	handler.AddStateChangeEvent(contEvent1, client)

	wg.Wait()
}

func TestSendsEventsConcurrentLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	handler := NewTaskHandler()

	contCalled := make(chan struct{}, concurrentEventCalls+1)
	completeStateChange := make(chan bool, concurrentEventCalls+1)
	count := 0
	countLock := &sync.Mutex{}
	client.EXPECT().SubmitContainerStateChange(gomock.Any()).Times(concurrentEventCalls + 1).Do(func(interface{}) {
		countLock.Lock()
		count++
		countLock.Unlock()
		<-completeStateChange
		contCalled <- struct{}{}
	})
	// Test concurrency; ensure it doesn't attempt to send more than
	// concurrentEventCalls at once
	// Put on N+1 events
	for i := 0; i < concurrentEventCalls+1; i++ {
		handler.AddStateChangeEvent(containerEvent("concurrent_"+strconv.Itoa(i)), client)
	}
	time.Sleep(10 * time.Millisecond)

	// N events should be waiting for potential errors since we havent started completing state changes
	assert.Equal(t, concurrentEventCalls, count, "Too many event calls got through concurrently")
	// Let one state change finish
	completeStateChange <- true
	<-contCalled
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, concurrentEventCalls+1, count, "Another concurrent call didn't start when expected")

	// ensure the remaining requests are completed
	for i := 0; i < concurrentEventCalls; i++ {
		completeStateChange <- true
		<-contCalled
	}
	time.Sleep(5 * time.Millisecond)
	assert.Equal(t, concurrentEventCalls+1, count, "Extra concurrent calls appeared from nowhere")
}

func TestSendsEventsContainerDifferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	handler := NewTaskHandler()

	var wg sync.WaitGroup
	wg.Add(2)

	// Test container event replacement doesn't happen
	contEventNotReplaced := containerEvent("notreplaced1")
	contEventSortaRedundant := containerEventStopped("notreplaced1")

	client.EXPECT().SubmitContainerStateChange(contEventNotReplaced.(api.ContainerStateChange)).Do(func(interface{}) { wg.Done() })
	client.EXPECT().SubmitContainerStateChange(contEventSortaRedundant.(api.ContainerStateChange)).Do(func(interface{}) { wg.Done() })

	handler.AddStateChangeEvent(contEventNotReplaced, client)
	handler.AddStateChangeEvent(contEventSortaRedundant, client)

	wg.Wait()
}

func TestSendsEventsTaskDifferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	handler := NewTaskHandler()

	wait := &sync.WaitGroup{}
	wait.Add(4)

	// Test task event replacement doesn't happen
	notReplacedCont := containerEvent("notreplaced2")
	sortaRedundantCont := containerEventStopped("notreplaced2")

	notReplacedTask := taskEvent("notreplaced")
	sortaRedundantTask := taskEventStopped("notreplaced2")

	client.EXPECT().SubmitContainerStateChange(notReplacedCont.(api.ContainerStateChange)).Do(func(interface{}) { wait.Done() })
	client.EXPECT().SubmitContainerStateChange(sortaRedundantCont.(api.ContainerStateChange)).Do(func(interface{}) { wait.Done() })
	client.EXPECT().SubmitTaskStateChange(notReplacedTask.(api.TaskStateChange)).Do(func(interface{}) { wait.Done() })
	client.EXPECT().SubmitTaskStateChange(sortaRedundantTask.(api.TaskStateChange)).Do(func(interface{}) { wait.Done() })

	handler.AddStateChangeEvent(notReplacedCont, client)
	handler.AddStateChangeEvent(notReplacedTask, client)
	handler.AddStateChangeEvent(sortaRedundantCont, client)
	handler.AddStateChangeEvent(sortaRedundantTask, client)

	wait.Wait()
}

func TestSendsEventsDedupe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	handler := NewTaskHandler()

	var wg sync.WaitGroup
	wg.Add(1)

	// Verify that a task doesn't get sent if we already have 'sent' it
	task1 := taskEvent("alreadySent")
	task1.(api.TaskStateChange).Task.SetSentStatus(api.TaskRunning)
	cont1 := containerEvent("alreadySent")
	cont1.(api.ContainerStateChange).Container.SetSentStatus(api.ContainerRunning)

	handler.AddStateChangeEvent(cont1, client)
	handler.AddStateChangeEvent(task1, client)

	task2 := taskEvent("containerSent")
	task2.(api.TaskStateChange).Task.SetSentStatus(api.TaskStatusNone)
	cont2 := containerEvent("containerSent")
	cont2.(api.ContainerStateChange).Container.SetSentStatus(api.ContainerRunning)

	// Expect to send a task status but not a container status
	client.EXPECT().SubmitTaskStateChange(task2.(api.TaskStateChange)).Do(func(interface{}) { wg.Done() })

	handler.AddStateChangeEvent(cont2, client)
	handler.AddStateChangeEvent(task2, client)

	wg.Wait()
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
