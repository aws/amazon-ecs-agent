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
package handler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	mock_wsclient "github.com/aws/amazon-ecs-agent/agent/wsclient/mock"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	clusterName          = "default"
	containerInstanceArn = "instance"
	payloadMessageId     = "123"
	testTaskARN          = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc"
)

// testHelper wraps all the object required for the test
type testHelper struct {
	ctrl               *gomock.Controller
	payloadHandler     payloadRequestHandler
	mockTaskEngine     *mock_engine.MockTaskEngine
	ecsClient          api.ECSClient
	mockWsClient       *mock_wsclient.MockClientServer
	credentialsManager credentials.Manager
	eventHandler       *eventhandler.TaskHandler
	ctx                context.Context
	cancel             context.CancelFunc
}

func setup(t *testing.T) *testHelper {
	ctrl := gomock.NewController(t)
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	credentialsManager := credentials.NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	taskHandler := eventhandler.NewTaskHandler(ctx, data.NewNoopClient(), nil, nil)
	latestSeqNumberTaskManifest := int64(10)

	handler := newPayloadRequestHandler(
		ctx,
		taskEngine,
		ecsClient,
		clusterName,
		containerInstanceArn,
		mockWsClient,
		data.NewNoopClient(),
		refreshCredentialsHandler{},
		credentialsManager,
		taskHandler, &latestSeqNumberTaskManifest)

	return &testHelper{
		ctrl:               ctrl,
		payloadHandler:     handler,
		mockTaskEngine:     taskEngine,
		ecsClient:          ecsClient,
		mockWsClient:       mockWsClient,
		credentialsManager: credentialsManager,
		eventHandler:       taskHandler,
		ctx:                ctx,
		cancel:             cancel,
	}
}

// TestHandlePayloadMessageWithNoMessageId tests that agent doesn't ack payload messages
// that do not contain message ids
func TestHandlePayloadMessageWithNoMessageId(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()
	// test adding a payload message without the MessageId field
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String("t1"),
			},
		},
	}
	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	assert.Error(t, err, "Expected error while adding a task with no message id")

	// test adding a payload message with blank MessageId
	payloadMessage.MessageId = aws.String("")
	err = tester.payloadHandler.handleSingleMessage(payloadMessage)
	assert.Error(t, err, "Expected error while adding a task with no message id")
}

func TestHandlePayloadMessageSaveData(t *testing.T) {
	testCases := []struct {
		name              string
		taskDesiredStatus string
		shouldSave        bool
	}{
		{
			name:              "task with desired status RUNNING is saved",
			taskDesiredStatus: "RUNNING",
			shouldSave:        true,
		},
		{
			name:              "task with desired status STOPPED is not saved",
			taskDesiredStatus: "STOPPED",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tester := setup(t)
			defer tester.ctrl.Finish()
			var ackRequested *ecsacs.AckRequest
			tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
				ackRequested = ackRequest
				tester.cancel()
			}).Times(1)
			tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Times(1)

			dataClient := newTestDataClient(t)
			tester.payloadHandler.dataClient = dataClient

			go tester.payloadHandler.start()
			err := tester.payloadHandler.handleSingleMessage(&ecsacs.PayloadMessage{
				Tasks: []*ecsacs.Task{
					{
						Arn:           aws.String(testTaskARN),
						DesiredStatus: aws.String(tc.taskDesiredStatus),
					},
				},
				MessageId: aws.String(payloadMessageId),
			})
			assert.NoError(t, err)

			// Wait till we get an ack from the ackBuffer.
			select {
			case <-tester.ctx.Done():
			}
			// Verify the message id acked
			assert.Equal(t, aws.StringValue(ackRequested.MessageId), payloadMessageId, "received message is not expected")

			tasks, err := dataClient.GetTasks()
			require.NoError(t, err)
			if tc.shouldSave {
				assert.Len(t, tasks, 1)
			} else {
				assert.Len(t, tasks, 0)
			}
		})
	}
}

// TestHandlePayloadMessageSaveDataError tests that agent does not ack payload messages
// when state saver fails to save task into db.
func TestHandlePayloadMessageSaveDataError(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	dataClient := newTestDataClient(t)

	// Save added task in the addedTask variable
	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	}).Times(1)

	tester.payloadHandler.dataClient = dataClient

	// Check if handleSingleMessage returns an error when we get error saving task data.
	err := tester.payloadHandler.handleSingleMessage(&ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn:           aws.String("t1"), // Use an invalid task arn to trigger error on saving task.
				DesiredStatus: aws.String("RUNNING"),
			},
		},
		MessageId: aws.String(payloadMessageId),
	})
	assert.Error(t, err, "Expected error while adding a task from statemanager")

	// We expect task to be added to the engine even though it couldn't be saved.
	expectedTask := &apitask.Task{
		Arn:                 "t1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
		NetworkMode:         apitask.BridgeNetworkMode,
	}
	expectedTask.GetID() // to set the task setIdOnce (sync.Once) property

	assert.Equal(t, expectedTask, addedTask, "added task is not expected")
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

// TestHandlePayloadMessageAckedWhenTaskAdded tests if the handler generates an ack
// after processing a payload message.
func TestHandlePayloadMessageAckedWhenTaskAdded(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	}).Times(1)

	var ackRequested *ecsacs.AckRequest
	tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		ackRequested = ackRequest
		tester.cancel()
	}).Times(1)

	go tester.payloadHandler.start()

	// Send a payload message
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String("t1"),
			},
		},
		MessageId: aws.String(payloadMessageId),
	}
	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	assert.NoError(t, err, "Error handling payload message")

	// Wait till we get an ack from the ackBuffer
	select {
	case <-tester.ctx.Done():
	}
	// Verify the message id acked
	assert.Equal(t, aws.StringValue(ackRequested.MessageId), payloadMessageId, "received message is not expected")

	// Verify if task added == expected task
	expectedTask := &apitask.Task{
		Arn:                "t1",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		NetworkMode:        apitask.BridgeNetworkMode,
	}
	expectedTask.GetID() // to set the task setIdOnce (sync.Once) property
	assert.Equal(t, expectedTask, addedTask, "received task is not expected")
}

// TestHandlePayloadMessageCredentialsAckedWhenTaskAdded tests if the handler generates
// an ack after processing a payload message when the payload message contains a task
// with an IAM Role. It also tests if the credentials ack is generated
func TestHandlePayloadMessageCredentialsAckedWhenTaskAdded(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	}).Times(1)

	var payloadAckRequested *ecsacs.AckRequest
	var taskCredentialsAckRequested *ecsacs.IAMRoleCredentialsAckRequest
	// The payload message in the test consists of a task, with credentials set
	// Record the credentials ack and the payload message ack
	gomock.InOrder(
		tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
			taskCredentialsAckRequested = ackRequest
		}),
		tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
			payloadAckRequested = ackRequest
			// Cancel the context when the ack for the payload message is received
			// This signals a successful workflow in the test
			tester.cancel()
		}),
	)

	refreshCredsHandler := newRefreshCredentialsHandler(tester.ctx, clusterName, containerInstanceArn, tester.mockWsClient, tester.credentialsManager, tester.mockTaskEngine)
	defer refreshCredsHandler.clearAcks()
	refreshCredsHandler.start()
	tester.payloadHandler.refreshHandler = refreshCredsHandler

	go tester.payloadHandler.start()

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
			{
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
	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	assert.NoError(t, err, "error handling payload message")

	// Wait till we get an ack from the ackBuffer
	select {
	case <-tester.ctx.Done():
	}

	// Verify the message id acked
	assert.Equal(t, aws.StringValue(payloadAckRequested.MessageId), payloadMessageId, "received message is not expected")

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
	assert.NoError(t, err, "error validating added task or credentials ack for the same")
}

// TestAddPayloadTaskAddsNonStoppedTasksAfterStoppedTasks tests if tasks with desired status
// 'RUNNING' are added after tasks with desired status 'STOPPED'
func TestAddPayloadTaskAddsNonStoppedTasksAfterStoppedTasks(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var tasksAddedToEngine []*apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		tasksAddedToEngine = append(tasksAddedToEngine, task)
	}).Times(2)

	stoppedTaskArn := "stoppedTask"
	runningTaskArn := "runningTask"
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn:           aws.String(runningTaskArn),
				DesiredStatus: aws.String("RUNNING"),
			},
			{
				Arn:           aws.String(stoppedTaskArn),
				DesiredStatus: aws.String("STOPPED"),
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	_, ok := tester.payloadHandler.addPayloadTasks(payloadMessage)
	assert.True(t, ok)
	assert.Len(t, tasksAddedToEngine, 2)

	// Verify if stopped task is added before running task
	firstTaskAdded := tasksAddedToEngine[0]
	assert.Equal(t, firstTaskAdded.Arn, stoppedTaskArn)
	assert.Equal(t, firstTaskAdded.GetDesiredStatus(), apitaskstatus.TaskStopped)

	secondTaskAdded := tasksAddedToEngine[1]
	assert.Equal(t, secondTaskAdded.Arn, runningTaskArn)
	assert.Equal(t, secondTaskAdded.GetDesiredStatus(), apitaskstatus.TaskRunning)
}

// TestPayloadBufferHandler tests if the async payloadBufferHandler routine
// acks messages after adding tasks
func TestPayloadBufferHandler(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	}).Times(1)

	var ackRequested *ecsacs.AckRequest
	tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		ackRequested = ackRequest
		tester.cancel()
	}).Times(1)

	go tester.payloadHandler.start()
	// Send a payload message to the payloadBufferChannel
	taskArn := "t1"
	tester.payloadHandler.messageBuffer <- &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String(taskArn),
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	// Wait till we get an ack
	select {
	case <-tester.ctx.Done():
	}
	// Verify if payloadMessageId read from the ack buffer is correct
	assert.Equal(t, aws.StringValue(ackRequested.MessageId), payloadMessageId, "received task is not expected")

	// Verify if the task added to the engine is correct
	expectedTask := &apitask.Task{
		Arn:                taskArn,
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		NetworkMode:        apitask.BridgeNetworkMode,
	}
	expectedTask.GetID() // to set the task setIdOnce (sync.Once) property
	assert.Equal(t, expectedTask, addedTask, "received task is not expected")
}

// TestPayloadBufferHandlerWithCredentials tests if the async payloadBufferHandler routine
// acks the payload message and credentials after adding tasks
func TestPayloadBufferHandlerWithCredentials(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	// The payload message in the test consists of two tasks, record both of them in
	// the order in which they were added
	var firstAddedTask *apitask.Task
	var secondAddedTask *apitask.Task
	gomock.InOrder(
		tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
			firstAddedTask = task
		}),
		tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
			secondAddedTask = task
		}),
	)

	// The payload message in the test consists of two tasks, with credentials set
	// for both. Record the credentials' ack and the payload message ack
	var payloadAckRequested *ecsacs.AckRequest
	var firstTaskCredentialsAckRequested *ecsacs.IAMRoleCredentialsAckRequest
	var secondTaskCredentialsAckRequested *ecsacs.IAMRoleCredentialsAckRequest
	gomock.InOrder(
		tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
			firstTaskCredentialsAckRequested = ackRequest
		}),
		tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
			secondTaskCredentialsAckRequested = ackRequest
		}),
		tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
			payloadAckRequested = ackRequest
			// Cancel the context when the ack for the payload message is received
			// This signals a successful workflow in the test
			tester.cancel()
		}),
	)

	refreshCredsHandler := newRefreshCredentialsHandler(tester.ctx, clusterName, containerInstanceArn, tester.mockWsClient, tester.credentialsManager, tester.mockTaskEngine)
	defer refreshCredsHandler.clearAcks()
	refreshCredsHandler.start()
	tester.payloadHandler.refreshHandler = refreshCredsHandler

	go tester.payloadHandler.start()

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
	tester.payloadHandler.messageBuffer <- &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
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
			{
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
	case <-tester.ctx.Done():
	}

	// Verify if payloadMessageId read from the ack buffer is correct
	assert.Equal(t, aws.StringValue(payloadAckRequested.MessageId), payloadMessageId, "received message is not expected")

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
	assert.NoError(t, err, "error validating added task or credentials ack for the same")

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
	assert.NoError(t, err, "error validating added task or credentials ack for the same")
}

// TestAddPayloadTaskAddsExecutionRoles tests the payload handler will add
// execution role credentials to the credentials manager and the field in
// task and container are appropriately set
func TestAddPayloadTaskAddsExecutionRoles(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	})

	var ackRequested *ecsacs.AckRequest
	var executionCredentialsAckRequested *ecsacs.IAMRoleCredentialsAckRequest
	gomock.InOrder(
		tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
			executionCredentialsAckRequested = ackRequest
		}),
		tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
			ackRequested = ackRequest
			tester.cancel()
		}),
	)
	refreshCredsHandler := newRefreshCredentialsHandler(tester.ctx, clusterName, containerInstanceArn, tester.mockWsClient, tester.credentialsManager, tester.mockTaskEngine)
	defer refreshCredsHandler.clearAcks()
	refreshCredsHandler.start()

	tester.payloadHandler.refreshHandler = refreshCredsHandler
	go tester.payloadHandler.start()
	taskArn := "t1"
	credentialsExpiration := "expiration"
	credentialsRoleArn := "r1"
	credentialsAccessKey := "akid"
	credentialsSecretKey := "skid"
	credentialsSessionToken := "token"
	credentialsID := "credsid"

	tester.payloadHandler.messageBuffer <- &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String(taskArn),
				ExecutionRoleCredentials: &ecsacs.IAMRoleCredentials{
					AccessKeyId:     aws.String(credentialsAccessKey),
					Expiration:      aws.String(credentialsExpiration),
					RoleArn:         aws.String(credentialsRoleArn),
					SecretAccessKey: aws.String(credentialsSecretKey),
					SessionToken:    aws.String(credentialsSessionToken),
					CredentialsId:   aws.String(credentialsID),
				},
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	// Wait till we get an ack
	select {
	case <-tester.ctx.Done():
	}
	// Verify if payloadMessageId read from the ack buffer is correct
	assert.Equal(t, aws.StringValue(ackRequested.MessageId), payloadMessageId, "received message not expected")
	assert.Equal(t, "credsid", addedTask.GetExecutionCredentialsID(), "task execution role credentials id mismatch")

	// Verify the credentials in the payload message was stored in the credentials manager
	iamRoleCredentials, ok := tester.credentialsManager.GetTaskCredentials("credsid")
	assert.True(t, ok, "execution role credentials not found in credentials manager")
	assert.Equal(t, credentialsRoleArn, iamRoleCredentials.IAMRoleCredentials.RoleArn)
	assert.Equal(t, taskArn, iamRoleCredentials.ARN)
	assert.Equal(t, credentialsExpiration, iamRoleCredentials.IAMRoleCredentials.Expiration)
	assert.Equal(t, credentialsAccessKey, iamRoleCredentials.IAMRoleCredentials.AccessKeyID)
	assert.Equal(t, credentialsSecretKey, iamRoleCredentials.IAMRoleCredentials.SecretAccessKey)
	assert.Equal(t, credentialsSessionToken, iamRoleCredentials.IAMRoleCredentials.SessionToken)
	assert.Equal(t, credentialsID, iamRoleCredentials.IAMRoleCredentials.CredentialsID)
	assert.Equal(t, credentialsID, *executionCredentialsAckRequested.CredentialsId)
	assert.Equal(t, payloadMessageId, *executionCredentialsAckRequested.MessageId)
}

// validateTaskAndCredentials compares a task and a credentials ack object
// against expected values. It returns an error if either of the the
// comparisons fail
func validateTaskAndCredentials(taskCredentialsAck, expectedCredentialsAckForTask *ecsacs.IAMRoleCredentialsAckRequest,
	addedTask *apitask.Task,
	expectedTaskArn string,
	expectedTaskCredentials credentials.IAMRoleCredentials) error {
	if !reflect.DeepEqual(taskCredentialsAck, expectedCredentialsAckForTask) {
		return fmt.Errorf("Mismatch between expected and received credentials ack requests, expected: %s, got: %s", expectedCredentialsAckForTask.String(), taskCredentialsAck.String())
	}

	expectedTask := &apitask.Task{
		Arn:                expectedTaskArn,
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		NetworkMode:        apitask.BridgeNetworkMode,
	}
	expectedTask.SetCredentialsID(expectedTaskCredentials.CredentialsID)
	expectedTask.GetID() // to set the task setIdOnce (sync.Once) property

	if !reflect.DeepEqual(addedTask, expectedTask) {
		return fmt.Errorf("Mismatch between expected and added tasks, expected: %v, added: %v", expectedTask, addedTask)
	}
	return nil
}

func TestPayloadHandlerAddedENIToTask(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String("arn"),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
					{
						AttachmentArn: aws.String("arn"),
						Ec2Id:         aws.String("ec2id"),
						Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
							{
								Primary:        aws.Bool(true),
								PrivateAddress: aws.String("ipv4"),
							},
						},
						Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
							{
								Address: aws.String("ipv6"),
							},
						},
						SubnetGatewayIpv4Address: aws.String("ipv4/20"),
						MacAddress:               aws.String("mac"),
					},
				},
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	require.NoError(t, err)

	// Validate the added task has the eni information as expected
	expectedENI := payloadMessage.Tasks[0].ElasticNetworkInterfaces[0]
	taskeni := addedTask.GetPrimaryENI()
	assert.Equal(t, aws.StringValue(expectedENI.Ec2Id), taskeni.ID)
	assert.Equal(t, aws.StringValue(expectedENI.MacAddress), taskeni.MacAddress)
	assert.Equal(t, 1, len(taskeni.IPV4Addresses))
	assert.Equal(t, 1, len(taskeni.IPV6Addresses))
	assert.Equal(t, aws.StringValue(expectedENI.Ipv4Addresses[0].PrivateAddress), taskeni.IPV4Addresses[0].Address)
	assert.Equal(t, aws.StringValue(expectedENI.Ipv6Addresses[0].Address), taskeni.IPV6Addresses[0].Address)
}

func TestPayloadHandlerAddedAppMeshToTask(t *testing.T) {
	appMeshType := "APPMESH"
	mockEgressIgnoredIP1 := "128.0.0.1"
	mockEgressIgnoredIP2 := "171.1.3.24"
	mockAppPort1 := "8000"
	mockAppPort2 := "8001"
	mockEgressIgnoredPort1 := "13000"
	mockEgressIgnoredPort2 := "13001"
	mockIgnoredUID := "1337"
	mockIgnoredGID := "2339"
	mockProxyIngressPort := "9000"
	mockProxyEgressPort := "9001"
	mockAppPorts := mockAppPort1 + "," + mockAppPort2
	mockEgressIgnoredIPs := mockEgressIgnoredIP1 + "," + mockEgressIgnoredIP2
	mockEgressIgnoredPorts := mockEgressIgnoredPort1 + "," + mockEgressIgnoredPort2
	mockContainerName := "testEnvoyContainer"
	taskMetadataEndpointIP := "169.254.170.2"
	instanceMetadataEndpointIP := "169.254.169.254"
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String("arn"),
				ProxyConfiguration: &ecsacs.ProxyConfiguration{
					Type: aws.String(appMeshType),
					Properties: map[string]*string{
						"IgnoredUID":         aws.String(mockIgnoredUID),
						"IgnoredGID":         aws.String(mockIgnoredGID),
						"ProxyIngressPort":   aws.String(mockProxyIngressPort),
						"ProxyEgressPort":    aws.String(mockProxyEgressPort),
						"AppPorts":           aws.String(mockAppPorts),
						"EgressIgnoredIPs":   aws.String(mockEgressIgnoredIPs),
						"EgressIgnoredPorts": aws.String(mockEgressIgnoredPorts),
					},
					ContainerName: aws.String(mockContainerName),
				},
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	assert.NoError(t, err)

	// Validate the added task has the eni information as expected
	appMesh := addedTask.GetAppMesh()
	assert.NotNil(t, appMesh)
	assert.Equal(t, mockContainerName, appMesh.ContainerName)
	assert.Equal(t, mockIgnoredUID, appMesh.IgnoredUID)
	assert.Equal(t, mockIgnoredGID, appMesh.IgnoredGID)
	assert.Equal(t, mockProxyIngressPort, appMesh.ProxyIngressPort)
	assert.Equal(t, mockProxyEgressPort, appMesh.ProxyEgressPort)
	assert.Equal(t, 2, len(appMesh.AppPorts))
	assert.Equal(t, mockAppPort1, appMesh.AppPorts[0])
	assert.Equal(t, mockAppPort2, appMesh.AppPorts[1])
	assert.Equal(t, 4, len(appMesh.EgressIgnoredIPs))
	assert.Equal(t, mockEgressIgnoredIP1, appMesh.EgressIgnoredIPs[0])
	assert.Equal(t, mockEgressIgnoredIP2, appMesh.EgressIgnoredIPs[1])
	assert.Equal(t, taskMetadataEndpointIP, appMesh.EgressIgnoredIPs[2])
	assert.Equal(t, instanceMetadataEndpointIP, appMesh.EgressIgnoredIPs[3])
	assert.Equal(t, 2, len(appMesh.EgressIgnoredPorts))
	assert.Equal(t, mockEgressIgnoredPort1, appMesh.EgressIgnoredPorts[0])
	assert.Equal(t, mockEgressIgnoredPort2, appMesh.EgressIgnoredPorts[1])
}

func TestPayloadHandlerAddedENITrunkToTask(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String("arn"),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
					{
						InterfaceAssociationProtocol: aws.String(eni.VLANInterfaceAssociationProtocol),
						AttachmentArn:                aws.String("arn"),
						Ec2Id:                        aws.String("ec2id"),
						Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
							{
								Primary:        aws.Bool(true),
								PrivateAddress: aws.String("ipv4"),
							},
						},
						Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
							{
								Address: aws.String("ipv6"),
							},
						},
						SubnetGatewayIpv4Address: aws.String("ipv4/20"),
						MacAddress:               aws.String("mac"),
						InterfaceVlanProperties: &ecsacs.NetworkInterfaceVlanProperties{
							VlanId:                   aws.String("12345"),
							TrunkInterfaceMacAddress: aws.String("mac"),
						},
					},
				},
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	require.NoError(t, err)

	taskeni := addedTask.GetPrimaryENI()

	assert.Equal(t, taskeni.InterfaceAssociationProtocol, eni.VLANInterfaceAssociationProtocol)
	assert.Equal(t, taskeni.InterfaceVlanProperties.TrunkInterfaceMacAddress, "mac")
	assert.Equal(t, taskeni.InterfaceVlanProperties.VlanID, "12345")
}

func TestPayloadHandlerAddedECRAuthData(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String("arn"),
				Containers: []*ecsacs.Container{
					{
						RegistryAuthentication: &ecsacs.RegistryAuthenticationData{
							Type: aws.String("ecr"),
							EcrAuthData: &ecsacs.ECRAuthData{
								Region:     aws.String("us-west-2"),
								RegistryId: aws.String("registry-id"),
							},
						},
					},
				},
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	assert.NoError(t, err)

	// Validate the added task has the ECRAuthData information as expected
	expected := payloadMessage.Tasks[0].Containers[0].RegistryAuthentication
	actual := addedTask.Containers[0].RegistryAuthentication

	assert.NotNil(t, actual.ECRAuthData)
	assert.Nil(t, actual.ASMAuthData)

	assert.Equal(t, aws.StringValue(expected.Type), actual.Type)
	assert.Equal(t, aws.StringValue(expected.EcrAuthData.Region), actual.ECRAuthData.Region)
	assert.Equal(t, aws.StringValue(expected.EcrAuthData.RegistryId), actual.ECRAuthData.RegistryID)
}

func TestPayloadHandlerAddedASMAuthData(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String("arn"),
				Containers: []*ecsacs.Container{
					{
						RegistryAuthentication: &ecsacs.RegistryAuthenticationData{
							Type: aws.String("asm"),
							AsmAuthData: &ecsacs.ASMAuthData{
								Region:               aws.String("us-west-2"),
								CredentialsParameter: aws.String("asm-arn"),
							},
						},
					},
				},
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	assert.NoError(t, err)

	// Validate the added task has the ASMAuthData information as expected
	expected := payloadMessage.Tasks[0].Containers[0].RegistryAuthentication
	actual := addedTask.Containers[0].RegistryAuthentication

	assert.NotNil(t, actual.ASMAuthData)
	assert.Nil(t, actual.ECRAuthData)

	assert.Equal(t, aws.StringValue(expected.Type), actual.Type)
	assert.Equal(t, aws.StringValue(expected.AsmAuthData.Region), actual.ASMAuthData.Region)
	assert.Equal(t, aws.StringValue(expected.AsmAuthData.CredentialsParameter), actual.ASMAuthData.CredentialsParameter)
}

func TestHandleUnrecognizedTask(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	arn := "arn"
	ecsacsTask := &ecsacs.Task{Arn: &arn}
	payloadMessage := &ecsacs.PayloadMessage{
		Tasks:     []*ecsacs.Task{ecsacsTask},
		MessageId: aws.String(payloadMessageId),
	}

	mockECSACSClient := mock_api.NewMockECSClient(tester.ctrl)
	taskHandler := eventhandler.NewTaskHandler(tester.ctx, data.NewNoopClient(), dockerstate.NewTaskEngineState(), mockECSACSClient)
	tester.payloadHandler.taskHandler = taskHandler

	wait := &sync.WaitGroup{}
	wait.Add(1)

	mockECSACSClient.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(func(change api.TaskStateChange) {
		assert.NotNil(t, change.Task)
		wait.Done()
	})

	tester.payloadHandler.handleUnrecognizedTask(ecsacsTask, errors.New("test error"), payloadMessage)
	wait.Wait()
}

func TestPayloadHandlerAddedFirelensData(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	payloadMessage := &ecsacs.PayloadMessage{
		Tasks: []*ecsacs.Task{
			{
				Arn: aws.String("arn"),
				Containers: []*ecsacs.Container{
					{
						FirelensConfiguration: &ecsacs.FirelensConfiguration{
							Type: aws.String("fluentd"),
							Options: map[string]*string{
								"enable-ecs-log-metadata": aws.String("true"),
							},
						},
					},
				},
			},
		},
		MessageId: aws.String(payloadMessageId),
	}

	err := tester.payloadHandler.handleSingleMessage(payloadMessage)
	assert.NoError(t, err)

	// Validate the pieces of the Firelens container
	expected := payloadMessage.Tasks[0].Containers[0].FirelensConfiguration
	actual := addedTask.Containers[0].FirelensConfig

	assert.Equal(t, aws.StringValue(expected.Type), actual.Type)
	assert.NotNil(t, actual.Options)
	assert.Equal(t, aws.StringValue(expected.Options["enable-ecs-log-metadata"]), actual.Options["enable-ecs-log-metadata"])
}

func TestPayloadHandlerSendPendingAcks(t *testing.T) {
	tester := setup(t)
	defer tester.ctrl.Finish()

	tester.mockWsClient.EXPECT().MakeRequest(gomock.Any()).Return(nil).Times(1)

	wg := sync.WaitGroup{}
	wg.Add(2)

	// write a dummy ack into the ackRequest
	go func() {
		tester.payloadHandler.ackRequest <- "testMessageID"
		wg.Done()
	}()

	// sleep here to ensure that the sending go routine above executes before the receiving one below. if not, then the
	// receiving go routine will finish without receiving the ack msg since sendPendingAcks() is non-blocking.
	time.Sleep(1 * time.Second)

	go func() {
		tester.payloadHandler.sendPendingAcks()
		wg.Done()
	}()

	// wait for both go routines above to finish before we verify that ack channel is empty and exit the test.
	// this also ensures that the mock MakeRequest call happened as expected.
	wg.Wait()

	// verify that the ackRequest channel is empty
	assert.Equal(t, 0, len(tester.payloadHandler.ackRequest))
}
