// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
package handler

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/wsclient/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"golang.org/x/net/context"
)

const (
	clusterName          = "default"
	containerInstanceArn = "instance"
)

// TestHandlePayloadMessageWithNoMessageId tests that agent doesn't ack payload messages
// that do not contain message ids
func TestHandlePayloadMessageWithNoMessageId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	ctx := context.Background()
	buffer := newPayloadRequestHandler(taskEngine, ecsClient, clusterName, containerInstanceArn, nil, stateManager, ctx)

	// test adding a payload message without the MessageId field
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
	}
	err := buffer.handleSingleMessage(payloadMessage)
	if err == nil {
		t.Error("Expected error while adding a task with no message id")
	}

	// test adding a payload message with blank MessageId
	payloadMessage.MessageId = aws.String("")
	err = buffer.handleSingleMessage(payloadMessage)

	if err == nil {
		t.Error("Expected error while adding a task with no message id")
	}

}

// TestHandlePayloadMessageAddTaskError tests that agent does not ack payload messages
// when task engine fails to add tasks
func TestHandlePayloadMessageAddTaskError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()

	// Return error from AddTask
	taskEngine.EXPECT().AddTask(gomock.Any()).Return(fmt.Errorf("oops")).Times(2)

	ctx := context.Background()
	buffer := newPayloadRequestHandler(taskEngine, ecsClient, clusterName, containerInstanceArn, nil, stateManager, ctx)

	// Test AddTask error with RUNNING task
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn:           aws.String("t1"),
				DesiredStatus: aws.String("RUNNING"),
			},
		},
		MessageId: aws.String("123"),
	}
	err := buffer.handleSingleMessage(payloadMessage)
	if err == nil {
		t.Error("Expected error while adding the task")
	}

	payloadMessage = &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn:           aws.String("t1"),
				DesiredStatus: aws.String("STOPPED"),
			},
		},
		MessageId: aws.String("123"),
	}
	// Test AddTask error with STOPPED task
	err = buffer.handleSingleMessage(payloadMessage)
	if err == nil {
		t.Error("Expected error while adding the task")
	}
}

// TestHandlePayloadMessageStateSaveError tests that agent does not ack payload messages
// when state saver fails to save state
func TestHandlePayloadMessageStateSaveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)

	taskEngine := engine.NewMockTaskEngine(ctrl)
	// Save added task in the addedTask variable
	var addedTask *api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		addedTask = task
	}).Times(1)

	// State manager returns error on save
	stateManager := mock_statemanager.NewMockStateManager(ctrl)
	stateManager.EXPECT().Save().Return(fmt.Errorf("oops"))

	ctx := context.Background()
	buffer := newPayloadRequestHandler(taskEngine, ecsClient, clusterName, containerInstanceArn, nil, stateManager, ctx)

	// Check if handleSingleMessage returns an error when state manager returns error on Save()
	err := buffer.handleSingleMessage(&ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
		MessageId: aws.String("123"),
	})
	if err == nil {
		t.Error("Expected error while adding a task from statemanager")
	}

	// We expect task to be added to the engine even though it hasn't been saved
	expectedTask := &api.Task{
		Arn: "t1",
	}
	if !reflect.DeepEqual(addedTask, expectedTask) {
		t.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
}

// TestHandlePayloadMessageAckedWhenTaskAdded tests if the handler generates an ack
// after processing a payload message.
func TestHandlePayloadMessageAckedWhenTaskAdded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	ctx, cancel := context.WithCancel(context.Background())

	taskEngine := engine.NewMockTaskEngine(ctrl)
	var addedTask *api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		addedTask = task
	}).Times(1)

	// Send a payload message
	messageId := "123"
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
		MessageId: aws.String(messageId),
	}
	var ackRequested *ecsacs.AckRequest
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		ackRequested = ackRequest
		cancel()
	}).Times(1)
	buffer := newPayloadRequestHandler(taskEngine, ecsClient, clusterName, containerInstanceArn, mockWsClient, stateManager, ctx)
	go buffer.start()
	err := buffer.handleSingleMessage(payloadMessage)
	if err != nil {
		t.Errorf("Error handling payload message: %v", err)
	}

	// Wait till we get an ack from the ackBuffer
	select {
	case <-ctx.Done():
	}
	// Verify the message id acked
	if aws.StringValue(ackRequested.MessageId) != messageId {
		t.Errorf("Message Id mismatch. Expected: %s, got: %s", messageId, aws.StringValue(ackRequested.MessageId))
	}

	// Verify if task added == expected task
	expectedTask := &api.Task{
		Arn: "t1",
	}
	if !reflect.DeepEqual(addedTask, expectedTask) {
		t.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
}

// TestAddPayloadTaskAddsNonStoppedTasksAfterStoppedTasks tests if tasks with desired status
// 'RUNNING' are added after tasks with desired status 'STOPPED'
func TestAddPayloadTaskAddsNonStoppedTasksAfterStoppedTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)
	taskEngine := engine.NewMockTaskEngine(ctrl)

	var tasksAddedToEngine []*api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		tasksAddedToEngine = append(tasksAddedToEngine, task)
	}).Times(2)

	messageId := "123"
	stoppedTaskArn := "stoppedTask"
	runningTaskArn := "runningTask"
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn:           aws.String(runningTaskArn),
				DesiredStatus: aws.String("RUNNING"),
			},
			&ecsacs.Task{
				Arn:           aws.String(stoppedTaskArn),
				DesiredStatus: aws.String("STOPPED"),
			},
		},
		MessageId: aws.String(messageId),
	}

	ctx := context.Background()
	stateManager := statemanager.NewNoopStateManager()
	buffer := newPayloadRequestHandler(taskEngine, ecsClient, clusterName, containerInstanceArn, nil, stateManager, ctx)
	ok := buffer.addPayloadTasks(payloadMessage)
	if !ok {
		t.Error("addPayloadTasks returned false")
	}
	if len(tasksAddedToEngine) != 2 {
		t.Errorf("Incorrect number of tasks added to the engine. Expected: %d, got: %d", 2, len(tasksAddedToEngine))
	}

	// Verify if stopped task is added before running task
	firstTaskAdded := tasksAddedToEngine[0]
	if firstTaskAdded.Arn != stoppedTaskArn {
		t.Errorf("Expected first task arn: %s, got: %s", stoppedTaskArn, firstTaskAdded.Arn)
	}
	if firstTaskAdded.DesiredStatus != api.TaskStopped {
		t.Errorf("Expected first task state be be: %s , got: %s", "STOPPED", firstTaskAdded.DesiredStatus.String())
	}

	secondTaskAdded := tasksAddedToEngine[1]
	if secondTaskAdded.Arn != runningTaskArn {
		t.Errorf("Expected second task arn: %s, got: %s", runningTaskArn, secondTaskAdded.Arn)
	}
	if secondTaskAdded.DesiredStatus != api.TaskRunning {
		t.Errorf("Expected second task state be be: %s , got: %s", "RUNNNING", secondTaskAdded.DesiredStatus.String())
	}
}

// TestPayloadBufferHandler tests if the async payloadBufferHandler routine
// acks messages after adding tasks
func TestPayloadBufferHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	ctx, cancel := context.WithCancel(context.Background())

	// payloadBuffer := make(chan *ecsacs.PayloadMessage, payloadMessageBufferSize)
	var addedTask *api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		addedTask = task
	}).Times(1)

	var ackRequested *ecsacs.AckRequest
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		ackRequested = ackRequest
		cancel()
	}).Times(1)

	buffer := newPayloadRequestHandler(taskEngine, ecsClient, clusterName, containerInstanceArn, mockWsClient, stateManager, ctx)
	go buffer.start()
	// Send a payload message to the payloadBufferChannel
	messageId := "123"
	taskArn := "t1"
	buffer.messageBuffer <- &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String(taskArn),
			},
		},
		MessageId: aws.String(messageId),
	}

	// Wait till we get an ack
	select {
	case <-ctx.Done():
	}
	// Verify if messageId read from the ack buffer is correct
	if aws.StringValue(ackRequested.MessageId) != messageId {
		t.Errorf("Message Id mismatch. Expected: %s, got: %s", messageId, aws.StringValue(ackRequested.MessageId))
	}

	// Verify if the task added to the engine is correct
	expectedTask := &api.Task{
		Arn: taskArn,
	}
	if !reflect.DeepEqual(addedTask, expectedTask) {
		t.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
}
