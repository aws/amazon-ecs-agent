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
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/wsclient/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

const (
	clusterName          = "default"
	containerInstanceArn = "instance"
	payloadMessageId     = "123"
)

// TestHandlePayloadMessageWithNoMessageId tests that agent doesn't ack payload messages
// that do not contain message ids
func TestHandlePayloadMessageWithNoMessageId(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	credentialsManager := credentials.NewManager()
	taskHandler := eventhandler.NewTaskHandler()

	ctx := context.Background()
	buffer := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		nil,
		stateManager,
		refreshCredentialsHandler{},
		credentialsManager,
		taskHandler)

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
	credentialsManager := credentials.NewManager()
	taskHandler := eventhandler.NewTaskHandler()

	// Return error from AddTask
	taskEngine.EXPECT().AddTask(gomock.Any()).Return(fmt.Errorf("oops")).Times(2)

	ctx := context.Background()
	buffer := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		nil,
		stateManager,
		refreshCredentialsHandler{},
		credentialsManager,
		taskHandler)

	// Test AddTask error with RUNNING task
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn:           aws.String("t1"),
				DesiredStatus: aws.String("RUNNING"),
			},
		},
		MessageId: aws.String(payloadMessageId),
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
		MessageId: aws.String(payloadMessageId),
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
	credentialsManager := credentials.NewManager()
	taskHandler := eventhandler.NewTaskHandler()

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
	buffer := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		nil,
		stateManager,
		refreshCredentialsHandler{},
		credentialsManager,
		taskHandler)

	// Check if handleSingleMessage returns an error when state manager returns error on Save()
	err := buffer.handleSingleMessage(&ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
		MessageId: aws.String(payloadMessageId),
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
	credentialsManager := credentials.NewManager()
	taskHandler := eventhandler.NewTaskHandler()
	ctx, cancel := context.WithCancel(context.Background())

	taskEngine := engine.NewMockTaskEngine(ctrl)
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
	buffer := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		mockWsClient,
		stateManager,
		refreshCredentialsHandler{},
		credentialsManager,
		taskHandler)

	go buffer.start()

	// Send a payload message
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String("t1"),
			},
		},
		MessageId: aws.String(payloadMessageId),
	}
	err := buffer.handleSingleMessage(payloadMessage)
	if err != nil {
		t.Errorf("Error handling payload message: %v", err)
	}

	// Wait till we get an ack from the ackBuffer
	select {
	case <-ctx.Done():
	}
	// Verify the message id acked
	if aws.StringValue(ackRequested.MessageId) != payloadMessageId {
		t.Errorf("Message Id mismatch. Expected: %s, got: %s", payloadMessageId, aws.StringValue(ackRequested.MessageId))
	}

	// Verify if task added == expected task
	expectedTask := &api.Task{
		Arn: "t1",
	}
	if !reflect.DeepEqual(addedTask, expectedTask) {
		t.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
}

// TestHandlePayloadMessageCredentialsAckedWhenTaskAdded tests if the handler generates
// an ack after processing a payload message when the payload message contains a task
// with an IAM Role. It also tests if the credentials ack is generated
func TestHandlePayloadMessageCredentialsAckedWhenTaskAdded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	ctx, cancel := context.WithCancel(context.Background())
	credentialsManager := credentials.NewManager()
	taskHandler := eventhandler.NewTaskHandler()

	taskEngine := engine.NewMockTaskEngine(ctrl)
	var addedTask *api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		addedTask = task
	}).Times(1)

	var payloadAckRequested *ecsacs.AckRequest
	var taskCredentialsAckRequested *ecsacs.IAMRoleCredentialsAckRequest
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	// The payload message in the test consists of a task, with credentials set
	// Record the credentials ack and the payload message ack
	gomock.InOrder(
		mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
			taskCredentialsAckRequested = ackRequest
		}),
		mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
			payloadAckRequested = ackRequest
			// Cancel the context when the ack for the payload message is received
			// This signals a successful workflow in the test
			cancel()
		}),
	)

	refreshCredsHandler := newRefreshCredentialsHandler(ctx, clusterName, containerInstanceArn, mockWsClient, credentialsManager, taskEngine)
	defer refreshCredsHandler.clearAcks()
	refreshCredsHandler.start()

	payloadHandler := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		mockWsClient,
		stateManager,
		refreshCredsHandler,
		credentialsManager,
		taskHandler)

	go payloadHandler.start()

	taskArn := "t1"
	credentialsExpiration := "expiration"
	credentialsRoleArn := "r1"
	credentialsAccessKey := "akid"
	credentialsSecretKey := "skid"
	credentialsSessionToken := "token"
	credentialsId := "credsid"

	// Send a payload message
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String(taskArn),
				RoleCredentials: &ecsacs.IAMRoleCredentials{
					AccessKeyId:     aws.String(credentialsAccessKey),
					Expiration:      aws.String(credentialsExpiration),
					RoleArn:         aws.String(credentialsRoleArn),
					SecretAccessKey: aws.String(credentialsSecretKey),
					SessionToken:    aws.String(credentialsSessionToken),
					CredentialsId:   aws.String(credentialsId),
				},
			},
		},
		MessageId:            aws.String(payloadMessageId),
		ClusterArn:           aws.String(cluster),
		ContainerInstanceArn: aws.String(containerInstance),
	}
	err := payloadHandler.handleSingleMessage(payloadMessage)
	if err != nil {
		t.Errorf("Error handling payload message: %v", err)
	}

	// Wait till we get an ack from the ackBuffer
	select {
	case <-ctx.Done():
	}

	// Verify the message id acked
	if aws.StringValue(payloadAckRequested.MessageId) != payloadMessageId {
		t.Errorf("Message Id mismatch. Expected: %s, got: %s", payloadMessageId, aws.StringValue(payloadAckRequested.MessageId))
	}

	// Verify the correctness of the task added to the engine and the
	// credentials ack generated for it
	expectedCredentialsAck := &ecsacs.IAMRoleCredentialsAckRequest{
		MessageId:     aws.String(payloadMessageId),
		Expiration:    aws.String(credentialsExpiration),
		CredentialsId: aws.String(credentialsId),
	}
	expectedCredentials := credentials.IAMRoleCredentials{
		AccessKeyID:     credentialsAccessKey,
		Expiration:      credentialsExpiration,
		RoleArn:         credentialsRoleArn,
		SecretAccessKey: credentialsSecretKey,
		SessionToken:    credentialsSessionToken,
		CredentialsID:   credentialsId,
	}
	err = validateTaskAndCredentials(taskCredentialsAckRequested, expectedCredentialsAck, addedTask, taskArn, expectedCredentials)
	if err != nil {
		t.Errorf("Error validating added task or credentials ack for the same: %v", err)
	}
}

// TestAddPayloadTaskAddsNonStoppedTasksAfterStoppedTasks tests if tasks with desired status
// 'RUNNING' are added after tasks with desired status 'STOPPED'
func TestAddPayloadTaskAddsNonStoppedTasksAfterStoppedTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)
	taskEngine := engine.NewMockTaskEngine(ctrl)
	credentialsManager := credentials.NewManager()
	taskHandler := eventhandler.NewTaskHandler()

	var tasksAddedToEngine []*api.Task
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
		tasksAddedToEngine = append(tasksAddedToEngine, task)
	}).Times(2)

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
		MessageId: aws.String(payloadMessageId),
	}

	ctx := context.Background()
	stateManager := statemanager.NewNoopStateManager()
	buffer := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		nil,
		stateManager,
		refreshCredentialsHandler{},
		credentialsManager,
		taskHandler)

	_, ok := buffer.addPayloadTasks(payloadMessage)
	assert.True(t, ok)
	assert.Len(t, tasksAddedToEngine, 2)

	// Verify if stopped task is added before running task
	firstTaskAdded := tasksAddedToEngine[0]
	assert.Equal(t, firstTaskAdded.Arn, stoppedTaskArn)
	assert.Equal(t, firstTaskAdded.GetDesiredStatus(), api.TaskStopped)

	secondTaskAdded := tasksAddedToEngine[1]
	assert.Equal(t, secondTaskAdded.Arn, runningTaskArn)
	assert.Equal(t, secondTaskAdded.GetDesiredStatus(), api.TaskRunning)
}

// TestPayloadBufferHandler tests if the async payloadBufferHandler routine
// acks messages after adding tasks
func TestPayloadBufferHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	credentialsManager := credentials.NewManager()
	taskHandler := eventhandler.NewTaskHandler()
	ctx, cancel := context.WithCancel(context.Background())

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

	buffer := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		mockWsClient,
		stateManager,
		refreshCredentialsHandler{},
		credentialsManager,
		taskHandler)

	go buffer.start()
	// Send a payload message to the payloadBufferChannel
	taskArn := "t1"
	buffer.messageBuffer <- &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String(taskArn),
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	// Wait till we get an ack
	select {
	case <-ctx.Done():
	}
	// Verify if payloadMessageId read from the ack buffer is correct
	if aws.StringValue(ackRequested.MessageId) != payloadMessageId {
		t.Errorf("Message Id mismatch. Expected: %s, got: %s", payloadMessageId, aws.StringValue(ackRequested.MessageId))
	}

	// Verify if the task added to the engine is correct
	expectedTask := &api.Task{
		Arn: taskArn,
	}
	if !reflect.DeepEqual(addedTask, expectedTask) {
		t.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
}

// TestPayloadBufferHandlerWithCredentials tests if the async payloadBufferHandler routine
// acks the payload message and credentials after adding tasks
func TestPayloadBufferHandlerWithCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_api.NewMockECSClient(ctrl)
	stateManager := statemanager.NewNoopStateManager()
	ctx, cancel := context.WithCancel(context.Background())
	credentialsManager := credentials.NewManager()
	taskHandler := eventhandler.NewTaskHandler()

	// The payload message in the test consists of two tasks, record both of them in
	// the order in which they were added
	taskEngine := engine.NewMockTaskEngine(ctrl)
	var firstAddedTask *api.Task
	var secondAddedTask *api.Task
	gomock.InOrder(
		taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
			firstAddedTask = task
		}),
		taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *api.Task) {
			secondAddedTask = task
		}),
	)

	// The payload message in the test consists of two tasks, with credentials set
	// for both. Record the credentials' ack and the payload message ack
	var payloadAckRequested *ecsacs.AckRequest
	var firstTaskCredentialsAckRequested *ecsacs.IAMRoleCredentialsAckRequest
	var secondTaskCredentialsAckRequested *ecsacs.IAMRoleCredentialsAckRequest
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	gomock.InOrder(
		mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
			firstTaskCredentialsAckRequested = ackRequest
		}),
		mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
			secondTaskCredentialsAckRequested = ackRequest
		}),
		mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
			payloadAckRequested = ackRequest
			// Cancel the context when the ack for the payload message is received
			// This signals a successful workflow in the test
			cancel()
		}),
	)

	refreshCredsHandler := newRefreshCredentialsHandler(ctx, clusterName, containerInstanceArn, mockWsClient, credentialsManager, taskEngine)
	defer refreshCredsHandler.clearAcks()
	refreshCredsHandler.start()
	payloadHandler := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		mockWsClient,
		stateManager,
		refreshCredsHandler,
		credentialsManager,
		taskHandler)

	go payloadHandler.start()

	firstTaskArn := "t1"
	firstTaskCredentialsExpiration := "expiration"
	firstTaskCredentialsRoleArn := "r1"
	firstTaskCredentialsAccessKey := "akid"
	firstTaskCredentialsSecretKey := "skid"
	firstTaskCredentialsSessionToken := "token"
	firstTaskCredentialsId := "credsid1"

	secondTaskArn := "t2"
	secondTaskCredentialsExpiration := "expirationSecond"
	secondTaskCredentialsRoleArn := "r2"
	secondTaskCredentialsAccessKey := "akid2"
	secondTaskCredentialsSecretKey := "skid2"
	secondTaskCredentialsSessionToken := "token2"
	secondTaskCredentialsId := "credsid2"

	// Send a payload message to the payloadBufferChannel
	payloadHandler.messageBuffer <- &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			&ecsacs.Task{
				Arn: aws.String(firstTaskArn),
				RoleCredentials: &ecsacs.IAMRoleCredentials{
					AccessKeyId:     aws.String(firstTaskCredentialsAccessKey),
					Expiration:      aws.String(firstTaskCredentialsExpiration),
					RoleArn:         aws.String(firstTaskCredentialsRoleArn),
					SecretAccessKey: aws.String(firstTaskCredentialsSecretKey),
					SessionToken:    aws.String(firstTaskCredentialsSessionToken),
					CredentialsId:   aws.String(firstTaskCredentialsId),
				},
			},
			&ecsacs.Task{
				Arn: aws.String(secondTaskArn),
				RoleCredentials: &ecsacs.IAMRoleCredentials{
					AccessKeyId:     aws.String(secondTaskCredentialsAccessKey),
					Expiration:      aws.String(secondTaskCredentialsExpiration),
					RoleArn:         aws.String(secondTaskCredentialsRoleArn),
					SecretAccessKey: aws.String(secondTaskCredentialsSecretKey),
					SessionToken:    aws.String(secondTaskCredentialsSessionToken),
					CredentialsId:   aws.String(secondTaskCredentialsId),
				},
			},
		},
		MessageId:            aws.String(payloadMessageId),
		ClusterArn:           aws.String(cluster),
		ContainerInstanceArn: aws.String(containerInstance),
	}

	// Wait till we get an ack
	select {
	case <-ctx.Done():
	}

	// Verify if payloadMessageId read from the ack buffer is correct
	if aws.StringValue(payloadAckRequested.MessageId) != payloadMessageId {
		t.Errorf("Message Id mismatch. Expected: %s, got: %s", payloadMessageId, aws.StringValue(payloadAckRequested.MessageId))
	}

	// Verify the correctness of the first task added to the engine and the
	// credentials ack generated for it
	expectedCredentialsAckForFirstTask := &ecsacs.IAMRoleCredentialsAckRequest{
		MessageId:     aws.String(payloadMessageId),
		Expiration:    aws.String(firstTaskCredentialsExpiration),
		CredentialsId: aws.String(firstTaskCredentialsId),
	}
	expectedCredentialsForFirstTask := credentials.IAMRoleCredentials{
		AccessKeyID:     firstTaskCredentialsAccessKey,
		Expiration:      firstTaskCredentialsExpiration,
		RoleArn:         firstTaskCredentialsRoleArn,
		SecretAccessKey: firstTaskCredentialsSecretKey,
		SessionToken:    firstTaskCredentialsSessionToken,
		CredentialsID:   firstTaskCredentialsId,
	}
	err := validateTaskAndCredentials(firstTaskCredentialsAckRequested, expectedCredentialsAckForFirstTask, firstAddedTask, firstTaskArn, expectedCredentialsForFirstTask)
	if err != nil {
		t.Errorf("Error validating added task or credentials ack for the same: %v", err)
	}

	// Verify the correctness of the second task added to the engine and the
	// credentials ack generated for it
	expectedCredentialsAckForSecondTask := &ecsacs.IAMRoleCredentialsAckRequest{
		MessageId:     aws.String(payloadMessageId),
		Expiration:    aws.String(secondTaskCredentialsExpiration),
		CredentialsId: aws.String(secondTaskCredentialsId),
	}
	expectedCredentialsForSecondTask := credentials.IAMRoleCredentials{
		AccessKeyID:     secondTaskCredentialsAccessKey,
		Expiration:      secondTaskCredentialsExpiration,
		RoleArn:         secondTaskCredentialsRoleArn,
		SecretAccessKey: secondTaskCredentialsSecretKey,
		SessionToken:    secondTaskCredentialsSessionToken,
		CredentialsID:   secondTaskCredentialsId,
	}
	err = validateTaskAndCredentials(secondTaskCredentialsAckRequested, expectedCredentialsAckForSecondTask, secondAddedTask, secondTaskArn, expectedCredentialsForSecondTask)
	if err != nil {
		t.Errorf("Error validating added task or credentials ack for the same: %v", err)
	}
}

// validateTaskAndCredentials compares a task and a credentials ack object
// against expected values. It returns an error if either of the the
// comparisons fail
func validateTaskAndCredentials(taskCredentialsAck, expectedCredentialsAckForTask *ecsacs.IAMRoleCredentialsAckRequest, addedTask *api.Task, expectedTaskArn string, expectedTaskCredentials credentials.IAMRoleCredentials) error {
	if !reflect.DeepEqual(taskCredentialsAck, expectedCredentialsAckForTask) {
		return fmt.Errorf("Mismatch between expected and received credentials ack requests, expected: %s, got: %s", expectedCredentialsAckForTask.String(), taskCredentialsAck.String())
	}

	expectedTask := &api.Task{
		Arn: expectedTaskArn,
	}
	expectedTask.SetCredentialsID(expectedTaskCredentials.CredentialsID)

	if !reflect.DeepEqual(addedTask, expectedTask) {
		return fmt.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
	return nil
}
