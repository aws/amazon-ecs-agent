// Copyright 2014 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/auth"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

type changeFn func(change api.ContainerStateChange) utils.RetriableError

type MockECSClient struct {
	submitTaskStateChange      changeFn
	submitContainerStateChange changeFn
}

func (m *MockECSClient) CredentialProvider() credentials.AWSCredentialProvider {
	return auth.TestCredentialProvider{}
}
func (m *MockECSClient) RegisterContainerInstance() (string, error) {
	return "", nil
}
func (m *MockECSClient) DiscoverPollEndpoint(string) (string, error) {
	return "", nil
}
func (m *MockECSClient) DeregisterContainerInstance(string) error {
	return nil
}
func (m *MockECSClient) SubmitTaskStateChange(change api.ContainerStateChange) utils.RetriableError {
	return m.submitTaskStateChange(change)
}
func (m *MockECSClient) SubmitContainerStateChange(change api.ContainerStateChange) utils.RetriableError {
	return m.submitContainerStateChange(change)
}

func mockClient(task, cont changeFn) api.ECSClient {
	return &MockECSClient{
		task, cont,
	}
}

func contEvent(arn string) api.ContainerStateChange {
	return api.ContainerStateChange{TaskArn: arn, Status: api.ContainerRunning}
}
func taskEvent(arn string) api.ContainerStateChange {
	return api.ContainerStateChange{TaskArn: arn, Status: api.ContainerRunning, TaskStatus: api.TaskRunning}
}

func TestSendsEvents(t *testing.T) {

	// These channels will submit "successful" state changes back to the test
	taskStatus := make(chan api.ContainerStateChange)
	contStatus := make(chan api.ContainerStateChange)

	// These counters let us know how many errors have happened of each type
	var taskRetriableErrors, contRetriableErrors, taskUnretriableErrors, contUnretriableErrors, taskErrors, contErrors int32
	resetCounters := func() {
		taskErrors = 0
		contErrors = 0
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
		func(change api.ContainerStateChange) utils.RetriableError {
			atomic.AddInt32(&taskErrors, 1)
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
		func(change api.ContainerStateChange) utils.RetriableError {
			atomic.AddInt32(&contErrors, 1)
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

	// Trivial: one task/container, no errors

	AddTaskEvent(contEvent("1"), client)
	go func() {
		contError <- nil
	}()

	sent := <-contStatus
	if sent.TaskArn != "1" {
		t.Error("Sent event did not match added event")
	}

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

	sent = <-taskStatus
	if sent.TaskArn != "2" {
		t.Error("Wrong task submitted")
	}

	// Now a little more complicated; 1 event with retries
	resetCounters()
	AddTaskEvent(contEvent("3"), client)
	go func() {
		contError <- retriable
		contError <- retriable
		contError <- nil
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-contStatus:
			t.Error("Should not have sent a container status if there was a retriable error")
		default:
		}
	}
	sent = <-contStatus
	if sent.TaskArn != "3" {
		t.Error("Wrong task submitted")
	}
	if contRetriableErrors != 2 && contErrors != 3 {
		t.Error("Didn't get the expected number of errors")
	}

	resetCounters()
	// Test concurrency; ensure it doesn't attempt to send more than
	// CONCURRENT_EVENT_CALLS at once
	// Put on N+1 events
	for i := 0; i < CONCURRENT_EVENT_CALLS+1; i++ {
		AddTaskEvent(contEvent("concurrent_"+strconv.Itoa(i)), client)
	}
	// N events should be waiting for potential errors; verify this is so
	time.Sleep(20 * time.Millisecond)
	if contErrors != CONCURRENT_EVENT_CALLS {
		t.Error("Too many event calls got through concurrently")
	}
	// Let one through
	go func() {
		contError <- nil
	}()
	<-contStatus

	time.Sleep(20 * time.Millisecond)
	if contErrors != CONCURRENT_EVENT_CALLS+1 {
		t.Error("Another concurrent call didn't start when expected")
	}
	// let through the rest
	for i := 0; i < CONCURRENT_EVENT_CALLS; i++ {
		go func() {
			contError <- nil
		}()
		<-contStatus
	}
	time.Sleep(20 * time.Millisecond)
	if contErrors != CONCURRENT_EVENT_CALLS+1 {
		t.Error("Somehow extra concurrenct calls appeared from nowhere")
	}

	// Test container event replacement

	AddTaskEvent(contEvent("replaceme"), client)
	replacement := contEvent("replaceme")
	replacement.Status = api.ContainerStopped
	AddTaskEvent(replacement, client)
	// Expect to only get one event after a retriable error and then nil
	// error
	go func() {
		contError <- retriable
		contError <- nil
	}()

	time.Sleep(20 * time.Millisecond)
	sent = <-contStatus
	if sent.TaskArn != "replaceme" {
		t.Error("Wrong arn")
	}
	if sent.Status != api.ContainerStopped {
		t.Error("Wrong status, got " + sent.Status.String() + " instead of STOPPED")
	}

	select {
	case <-contStatus:
		t.Error("event should have been replaced")
	default:
	}

	// Test task event replacement
	AddTaskEvent(taskEvent("replaceme"), client)
	replacement = taskEvent("replaceme")
	replacement.Status = api.ContainerStopped
	replacement.TaskStatus = api.TaskStopped
	AddTaskEvent(replacement, client)

	// Expect, after a couple retries, for there to only be one event
	go func() {
		taskError <- retriable
		taskError <- nil
	}()
	go func() {
		contError <- retriable
		contError <- nil
	}()

	time.Sleep(20 * time.Millisecond)
	sent = <-contStatus
	if sent.TaskArn != "replaceme" {
		t.Error("Wrong arn")
	}
	if sent.Status != api.ContainerStopped {
		t.Error("Wrong status")
	}

	sent = <-taskStatus
	if sent.TaskArn != "replaceme" {
		t.Error("Wrong arn")
	}
	if sent.TaskStatus != api.TaskStopped {
		t.Error("Wrong status")
	}

	select {
	case <-contStatus:
		t.Error("event should have been replaced")
	case <-taskStatus:
		t.Error("event should have been replaced")
	default:
	}
}
