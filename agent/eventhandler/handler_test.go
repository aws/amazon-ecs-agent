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
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

type containerChangeFn func(change api.ContainerStateChange) error
type taskChangeFn func(change api.TaskStateChange) error

type MockECSClient struct {
	submitTaskStateChange      taskChangeFn
	submitContainerStateChange containerChangeFn
}

func (m *MockECSClient) CredentialProvider() *credentials.Credentials {
	return credentials.AnonymousCredentials
}
func (m *MockECSClient) RegisterContainerInstance(string, []string) (string, error) {
	return "", nil
}
func (m *MockECSClient) DiscoverPollEndpoint(string) (string, error) {
	return "", nil
}
func (m *MockECSClient) SubmitTaskStateChange(change api.TaskStateChange) error {
	return m.submitTaskStateChange(change)
}
func (m *MockECSClient) SubmitContainerStateChange(change api.ContainerStateChange) error {
	return m.submitContainerStateChange(change)
}
func (m *MockECSClient) DiscoverTelemetryEndpoint(string) (string, error) {
	return "", nil
}

func mockClient(task taskChangeFn, cont containerChangeFn) api.ECSClient {
	return &MockECSClient{
		task, cont,
	}
}

func contEvent(arn string) api.ContainerStateChange {
	return api.ContainerStateChange{TaskArn: arn, ContainerName: "containerName", Status: api.ContainerRunning}
}
func taskEvent(arn string) api.TaskStateChange {
	return api.TaskStateChange{TaskArn: arn, Status: api.TaskRunning}
}

func TestSendsEvents(t *testing.T) {

	// These channels will submit "successful" state changes back to the test
	taskStatus := make(chan api.TaskStateChange)
	contStatus := make(chan api.ContainerStateChange)

	// These counters let us know how many errors have happened of each type
	var taskRetriableErrors, contRetriableErrors, taskUnretriableErrors, contUnretriableErrors, taskCalls, contCalls int32
	resetCounters := func() {
		taskCalls = 0
		contCalls = 0
		taskRetriableErrors = 0
		contRetriableErrors = 0
		taskUnretriableErrors = 0
		contUnretriableErrors = 0
	}
	resetCounters()

	// These channels are used to tell the mock functions if they should error
	// or not
	taskError := make(chan utils.RetriableError)
	contError := make(chan utils.RetriableError)

	// premade errors for ease of testing
	retriable := utils.NewRetriableError(utils.NewRetriable(true), errors.New("test"))

	client := mockClient(
		func(change api.TaskStateChange) error {
			atomic.AddInt32(&taskCalls, 1)
			err := <-taskError
			if err == nil {
				taskStatus <- change
				return err
			}
			if err.Retry() {
				atomic.AddInt32(&taskRetriableErrors, 1)
				return err
			}

			atomic.AddInt32(&taskUnretriableErrors, 1)
			return err
		},
		func(change api.ContainerStateChange) error {
			atomic.AddInt32(&contCalls, 1)
			err := <-contError
			if err == nil {
				contStatus <- change
				return err
			}
			if err.Retry() {
				atomic.AddInt32(&contRetriableErrors, 1)
				return err
			}
			atomic.AddInt32(&contUnretriableErrors, 1)
			return err
		},
	)

	// Trivial: one container, no errors

	AddContainerEvent(contEvent("1"), client)
	go func() {
		contError <- nil
	}()

	sent := <-contStatus
	if sent.TaskArn != "1" || sent.Status != api.ContainerRunning {
		t.Error("Sent event did not match added event")
	}

	AddContainerEvent(contEvent("2"), client)
	AddTaskEvent(taskEvent("2"), client)
	go func() {
		contError <- nil
	}()
	go func() {
		taskError <- nil
	}()

	select {
	case <-taskStatus:
		t.Error("Should not submit task until after container")
	case sent := <-contStatus:
		if sent.TaskArn != "2" {
			t.Error("Wrong task submitted")
		}
	}

	tsent := <-taskStatus
	if tsent.TaskArn != "2" {
		t.Error("Wrong task submitted")
	}

	select {
	case <-contStatus:
		t.Error("event should have been replaced")
	case <-taskStatus:
		t.Error("There should be no pending taskStatus events")
	default:
	}

	// Now a little more complicated; 1 event with retries
	resetCounters()
	AddContainerEvent(contEvent("3"), client)
	go func() {
		contError <- retriable
		contError <- nil
	}()
	select {
	case <-contStatus:
		t.Error("Should not have sent a container status if there was a retriable error")
	default:
	}
	sent = <-contStatus
	if sent.TaskArn != "3" {
		t.Error("Wrong task submitted")
	}
	if contRetriableErrors != 1 && contCalls != 2 {
		t.Error("Didn't get the expected number of errors")
	}

	select {
	case <-contStatus:
		t.Error("event should have been replaced")
	case <-taskStatus:
		t.Error("There should be no pending taskStatus events")
	default:
	}

	resetCounters()
	// Test concurrency; ensure it doesn't attempt to send more than
	// concurrentEventCalls at once
	// Put on N+1 events
	for i := 0; i < concurrentEventCalls+1; i++ {
		AddContainerEvent(contEvent("concurrent_"+strconv.Itoa(i)), client)
	}
	// N events should be waiting for potential errors; verify this is so
	time.Sleep(5 * time.Millisecond)
	if contCalls != concurrentEventCalls {
		t.Error("Too many event calls got through concurrently")
	}
	// Let one through
	go func() {
		contError <- nil
	}()
	<-contStatus

	time.Sleep(5 * time.Millisecond)
	if contCalls != concurrentEventCalls+1 {
		t.Error("Another concurrent call didn't start when expected")
	}
	// let through the rest
	for i := 0; i < concurrentEventCalls; i++ {
		go func() {
			contError <- nil
		}()
		<-contStatus
	}
	time.Sleep(5 * time.Millisecond)
	if contCalls != concurrentEventCalls+1 {
		t.Error("Somehow extra concurrenct calls appeared from nowhere")
	}

	// Test container event replacement doesn't happen
	AddContainerEvent(contEvent("notreplaced1"), client)
	sortaRedundant := contEvent("notreplaced1")
	sortaRedundant.Status = api.ContainerStopped
	AddContainerEvent(sortaRedundant, client)
	go func() {
		contError <- nil
		contError <- retriable
		contError <- nil
	}()

	time.Sleep(5 * time.Millisecond)
	sent = <-contStatus
	if sent.TaskArn != "notreplaced1" {
		t.Error("Wrong arn, got " + sent.TaskArn)
	}
	if sent.Status != api.ContainerRunning {
		t.Error("Wrong status, got " + sent.Status.String() + " instead of RUNNING")
	}
	sent = <-contStatus
	if sent.TaskArn != "notreplaced1" {
		t.Error("Wrong arn")
	}
	if sent.Status != api.ContainerStopped {
		t.Error("Wrong status, got " + sent.Status.String() + " instead of STOPPED")
	}

	select {
	case <-contStatus:
		t.Error("event should have been replaced")
	case <-taskStatus:
		t.Error("There should be no pending taskStatus events")
	default:
	}

	// Test task event replacement doesn't happen
	AddContainerEvent(contEvent("notreplaced2"), client)
	AddTaskEvent(taskEvent("notreplaced2"), client)
	sortaRedundantc := contEvent("notreplaced2")
	sortaRedundantc.Status = api.ContainerStopped
	sortaRedundantt := taskEvent("notreplaced2")
	sortaRedundantt.Status = api.TaskStopped
	AddContainerEvent(sortaRedundantc, client)
	AddTaskEvent(sortaRedundantt, client)

	go func() {
		taskError <- nil
		taskError <- nil
	}()
	go func() {
		contError <- nil
		contError <- nil
	}()

	time.Sleep(5 * time.Millisecond)
	sent = <-contStatus
	if sent.TaskArn != "notreplaced2" {
		t.Error("Lost a task or task out of order")
	}
	tsent = <-taskStatus
	if tsent.TaskArn != "notreplaced2" {
		t.Error("Lost a task or task out of order")
	}
	if tsent.Status != api.TaskRunning {
		t.Error("Wrong status")
	}
	sent = <-contStatus
	if sent.TaskArn != "notreplaced2" {
		t.Error("Lost a task or task out of order")
	}
	tsent = <-taskStatus
	if tsent.TaskArn != "notreplaced2" {
		t.Error("Lost a task or task out of order")
	}
	if tsent.Status != api.TaskStopped {
		t.Error("Wrong status")
	}

	// Verify that a task doesn't get sent if we already have 'sent' it
	task := taskEvent("alreadySent")
	taskRunning := api.TaskRunning
	task.SentStatus = &taskRunning
	cont := contEvent("alreadySent")
	containerRunning := api.ContainerRunning
	cont.SentStatus = &containerRunning
	AddContainerEvent(cont, client)
	AddTaskEvent(task, client)
	time.Sleep(5 * time.Millisecond)
	select {
	case <-contStatus:
		t.Error("Did not expect container change; already sent")
	case <-taskStatus:
		t.Error("Did not expect task change; already sent")
	case taskError <- nil:
		t.Error("Did not expect to be able to write to taskError")
	case contError <- nil:
		t.Error("Did not expect to be able to write to contError")
	default:
	}

	task = taskEvent("containerSent")
	taskNone := api.TaskStatusNone
	task.SentStatus = &taskNone
	cont = contEvent("containerSent")
	cont.SentStatus = &containerRunning
	AddContainerEvent(cont, client)
	AddTaskEvent(task, client)
	// Expect to send a task status but not a container status
	go func() {
		taskError <- nil
	}()
	tsent = <-taskStatus
	time.Sleep(5 * time.Millisecond)
	if tsent.TaskArn != "containerSent" {
		t.Error("Wrong arn")
	}
	if tsent.Status != api.TaskRunning {
		t.Error("Wrong status")
	}
	if *task.SentStatus != api.TaskRunning {
		t.Error("Status not updated: ")
	}

	select {
	case <-contStatus:
		t.Error("Read all events")
	case <-taskStatus:
		t.Error("Read all events")
	case taskError <- nil:
		t.Error("Task error channel read pending")
	case contError <- nil:
		t.Error("Container error channel read pending")
	default:
	}
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
