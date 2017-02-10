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
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func contEvent(arn string) api.ContainerStateChange {
	return api.ContainerStateChange{TaskArn: arn, ContainerName: "containerName", Status: api.ContainerRunning, Container: &api.Container{}}
}
func taskEvent(arn string) api.TaskStateChange {
	return api.TaskStateChange{TaskArn: arn, Status: api.TaskRunning, Task: &api.Task{}}
}

func TestSendsEventsOneContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	// Trivial: one container, no errors
	contCalled := make(chan struct{})
	taskCalled := make(chan struct{})
	contEvent1 := contEvent("1")
	contEvent2 := contEvent("2")
	taskEvent2 := taskEvent("2")

	client.EXPECT().SubmitContainerStateChange(contEvent1).Do(func(interface{}) { contCalled <- struct{}{} })
	client.EXPECT().SubmitContainerStateChange(contEvent2).Do(func(interface{}) { contCalled <- struct{}{} })
	client.EXPECT().SubmitTaskStateChange(taskEvent2).Do(func(interface{}) { taskCalled <- struct{}{} })

	AddContainerEvent(contEvent1, client)
	AddContainerEvent(contEvent2, client)
	AddTaskEvent(taskEvent2, client)

	<-contCalled
	<-contCalled
	<-taskCalled
}

func TestSendsEventsOneEventRetries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	retriable := utils.NewRetriableError(utils.NewRetriable(true), errors.New("test"))
	contCalled := make(chan struct{})
	contEvent1 := contEvent("1")

	gomock.InOrder(
		client.EXPECT().SubmitContainerStateChange(contEvent1).Return(retriable).Do(func(interface{}) { contCalled <- struct{}{} }),
		client.EXPECT().SubmitContainerStateChange(contEvent1).Return(nil).Do(func(interface{}) { contCalled <- struct{}{} }),
	)

	AddContainerEvent(contEvent1, client)

	<-contCalled
	<-contCalled
}

func TestSendsEventsConcurrentLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

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
		AddContainerEvent(contEvent("concurrent_"+strconv.Itoa(i)), client)
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

	// Test container event replacement doesn't happen
	notReplaced := contEvent("notreplaced1")
	sortaRedundant := contEvent("notreplaced1")
	sortaRedundant.Status = api.ContainerStopped
	contCalled := make(chan struct{})
	client.EXPECT().SubmitContainerStateChange(notReplaced).Do(func(interface{}) { contCalled <- struct{}{} })
	client.EXPECT().SubmitContainerStateChange(sortaRedundant).Do(func(interface{}) { contCalled <- struct{}{} })

	AddContainerEvent(notReplaced, client)
	AddContainerEvent(sortaRedundant, client)
	<-contCalled
	<-contCalled
}

func TestSendsEventsTaskDifferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	// Test task event replacement doesn't happen
	notReplacedCont := contEvent("notreplaced2")
	sortaRedundantCont := contEvent("notreplaced2")
	sortaRedundantCont.Status = api.ContainerStopped
	notReplacedTask := taskEvent("notreplaced")
	sortaRedundantTask := taskEvent("notreplaced2")
	sortaRedundantTask.Status = api.TaskStopped

	wait := &sync.WaitGroup{}
	wait.Add(4)
	client.EXPECT().SubmitContainerStateChange(notReplacedCont).Do(func(interface{}) { wait.Done() })
	client.EXPECT().SubmitContainerStateChange(sortaRedundantCont).Do(func(interface{}) { wait.Done() })
	client.EXPECT().SubmitTaskStateChange(notReplacedTask).Do(func(interface{}) { wait.Done() })
	client.EXPECT().SubmitTaskStateChange(sortaRedundantTask).Do(func(interface{}) { wait.Done() })

	AddContainerEvent(notReplacedCont, client)
	AddTaskEvent(notReplacedTask, client)
	AddContainerEvent(sortaRedundantCont, client)
	AddTaskEvent(sortaRedundantTask, client)

	wait.Wait()
}

func TestSendsEventsDedupe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	// Verify that a task doesn't get sent if we already have 'sent' it
	task1 := taskEvent("alreadySent")
	task1.Task.SetSentStatus(api.TaskRunning)
	cont1 := contEvent("alreadySent")
	cont1.Container.SetSentStatus(api.ContainerRunning)

	AddContainerEvent(cont1, client)
	AddTaskEvent(task1, client)

	task2 := taskEvent("containerSent")
	task2.Task.SetSentStatus(api.TaskStatusNone)
	cont2 := contEvent("containerSent")
	cont2.Container.SetSentStatus(api.ContainerRunning)

	// Expect to send a task status but not a container status
	called := make(chan struct{})
	client.EXPECT().SubmitTaskStateChange(task2).Do(func(interface{}) { called <- struct{}{} })

	AddContainerEvent(cont2, client)
	AddTaskEvent(task2, client)

	<-called
	time.Sleep(5 * time.Millisecond)
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
