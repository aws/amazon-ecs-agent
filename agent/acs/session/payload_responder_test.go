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

package session

import (
	"context"
	"reflect"
	"sync"
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	acssession "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	apiresource "github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	attachmentARN = "arn"
	ec2ID         = "ec2id"
)

var testPayloadMessage = &ecsacs.PayloadMessage{
	Tasks:                []*ecsacs.Task{{}},
	MessageId:            aws.String(testconst.MessageID),
	ClusterArn:           aws.String(testconst.ClusterARN),
	ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
}

var expectedPayloadAck = &ecsacs.AckRequest{
	MessageId:         aws.String(testconst.MessageID),
	Cluster:           aws.String(testconst.ClusterARN),
	ContainerInstance: aws.String(testconst.ContainerInstanceARN),
}

// testHelper wraps all the object required for the test.
type testHelper struct {
	ctrl                  *gomock.Controller
	payloadResponder      wsclient.RequestResponder
	mockTaskEngine        *mock_engine.MockTaskEngine
	payloadMessageHandler *payloadMessageHandler
	ctx                   context.Context
}

func setup(t *testing.T, acsResponseSender wsclient.RespondFunc) *testHelper {
	ctrl := gomock.NewController(t)
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	dataClient := data.NewNoopClient()
	credentialsManager := credentials.NewManager()
	ctx := context.Background()
	taskHandler := eventhandler.NewTaskHandler(ctx, data.NewNoopClient(), nil, nil)
	latestSeqNumberTaskManifest := int64(10)
	payloadMsgHandler := NewPayloadMessageHandler(taskEngine, ecsClient, dataClient, taskHandler, credentialsManager,
		&latestSeqNumberTaskManifest)
	payloadResponder := acssession.NewPayloadResponder(payloadMsgHandler, acsResponseSender)

	return &testHelper{
		ctrl:                  ctrl,
		payloadResponder:      payloadResponder,
		mockTaskEngine:        taskEngine,
		payloadMessageHandler: payloadMsgHandler,
		ctx:                   ctx,
	}
}

// TestHandlePayloadMessageSaveData tests that agent ACKs payload messages
// when state saver behaves in an expected way.
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
			ackSent := make(chan *ecsacs.AckRequest)
			testResponseSender := func(response interface{}) error {
				resp := response.(*ecsacs.AckRequest)
				ackSent <- resp
				return nil
			}

			tester := setup(t, testResponseSender)
			dataClient := newTestDataClient(t)
			tester.payloadMessageHandler.dataClient = dataClient
			defer tester.ctrl.Finish()

			tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Times(1)

			handlePayloadMessage :=
				tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
			testPayloadMessage.Tasks = []*ecsacs.Task{
				{
					Arn:           aws.String(testconst.TaskARN),
					DesiredStatus: aws.String(tc.taskDesiredStatus),
				},
			}
			handlePayloadMessage(testPayloadMessage)

			// Verify that payload message ACK is sent and is as expected.
			payloadAckSent := <-ackSent
			assert.Equal(t, expectedPayloadAck, payloadAckSent)

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

// TestHandlePayloadMessageSaveDataError tests that agent does not ACK payload messages
// when state saver fails to save task into db.
func TestHandlePayloadMessageSaveDataError(t *testing.T) {
	ackSent := false
	testResponseSender := func(response interface{}) error {
		ackSent = true
		return nil
	}

	tester := setup(t, testResponseSender)
	dataClient := newTestDataClient(t)
	tester.payloadMessageHandler.dataClient = dataClient
	defer tester.ctrl.Finish()

	// Save added task in the addedTask variable.
	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	}).Times(1)

	// Check if handleSingleMessage returns an error when we get error saving task data.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn:           aws.String("t1"), // Use an invalid task arn to trigger error on saving task.
			DesiredStatus: aws.String("RUNNING"),
		},
	}
	handlePayloadMessage(testPayloadMessage)
	assert.False(t, ackSent,
		"Expected no ACK of payload message when adding a task from statemanager results in error")

	// We expect task to be added to the engine even though it couldn't be saved.
	expectedTask := &apitask.Task{
		Arn:                 "t1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
		NetworkMode:         apitask.BridgeNetworkMode,
	}

	assert.Equal(t, expectedTask, addedTask, "added task is not expected")
}

// TestHandlePayloadMessageAckedWhenTaskAdded tests if the payload responder generates an ACK
// after processing a payload message.
func TestHandlePayloadMessageAckedWhenTaskAdded(t *testing.T) {
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	}).Times(1)

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn: aws.String("t1"),
		},
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	// Verify if task added == expected task.
	expectedTask := &apitask.Task{
		Arn:                "t1",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		NetworkMode:        apitask.BridgeNetworkMode,
	}
	assert.Equal(t, expectedTask, addedTask, "received task is not expected")
}

// TestHandlePayloadMessageCredentialsAckedWhenTaskAdded tests if the payload responder generates
// an ACK after processing a payload message when the payload message contains a task
// with an IAM Role. It also tests if the credentials ACK is generated.
func TestHandlePayloadMessageCredentialsAckedWhenTaskAdded(t *testing.T) {
	payloadAckSent := make(chan *ecsacs.AckRequest)
	credentialsAckSent := make(chan *ecsacs.IAMRoleCredentialsAckRequest)
	testResponseSender := func(response interface{}) error {
		var credentialsResp *ecsacs.IAMRoleCredentialsAckRequest
		var payloadMessageResp *ecsacs.AckRequest
		credentialsResp, ok := response.(*ecsacs.IAMRoleCredentialsAckRequest)
		if ok {
			credentialsAckSent <- credentialsResp
		} else {
			payloadMessageResp, ok = response.(*ecsacs.AckRequest)
			if ok {
				payloadAckSent <- payloadMessageResp
			} else {
				t.Fatal("response does not hold type ecsacs.IAMRoleCredentialsAckRequest or ecsacs.AckRequest")
			}
		}
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	}).Times(1)

	// The payload message in the test consists of a task, with credentials set.
	taskArn := "t1"
	credentialsExpiration := "expiration"
	credentialsRoleArn := "r1"
	credentialsAccessKey := "akid"
	credentialsSecretKey := "skid"
	credentialsSessionToken := "token"
	credentialsId := "credsid"

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
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
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	actualPayloadAck := <-payloadAckSent
	assert.Equal(t, expectedPayloadAck, actualPayloadAck)

	// Verify the correctness of the task added to the engine and the
	// credentials ACK generated for it.
	expectedCredentialsAck := &ecsacs.IAMRoleCredentialsAckRequest{
		MessageId:     aws.String(testconst.MessageID),
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
	actualCredentialsAck := <-credentialsAckSent
	err := validateTaskAndCredentials(actualCredentialsAck, expectedCredentialsAck, addedTask, taskArn,
		expectedCredentials)
	assert.NoError(t, err, "error validating added task or credentials ACK for the same")
}

// TestHandlePayloadMessageAddsNonStoppedTasksAfterStoppedTasks tests if tasks with desired status
// 'RUNNING' are added after tasks with desired status 'STOPPED'
func TestHandlePayloadMessageAddsNonStoppedTasksAfterStoppedTasks(t *testing.T) {
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var tasksAddedToEngine []*apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		tasksAddedToEngine = append(tasksAddedToEngine, task)
	}).Times(2)

	stoppedTaskArn := "stoppedTask"
	runningTaskArn := "runningTask"

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn:           aws.String(runningTaskArn),
			DesiredStatus: aws.String("RUNNING"),
		},
		{
			Arn:           aws.String(stoppedTaskArn),
			DesiredStatus: aws.String("STOPPED"),
		},
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	// Verify that two tasks were added to the engine.
	assert.Len(t, tasksAddedToEngine, 2)

	// Verify if stopped task is added before running task.
	firstTaskAdded := tasksAddedToEngine[0]
	assert.Equal(t, firstTaskAdded.Arn, stoppedTaskArn)
	assert.Equal(t, firstTaskAdded.GetDesiredStatus(), apitaskstatus.TaskStopped)
	secondTaskAdded := tasksAddedToEngine[1]
	assert.Equal(t, secondTaskAdded.Arn, runningTaskArn)
	assert.Equal(t, secondTaskAdded.GetDesiredStatus(), apitaskstatus.TaskRunning)
}

// TestHandlePayloadMessageCredentialsAckedWhenTaskAdded tests if the payload responder generates
// an ACK after processing a payload message when the payload message contains multiple tasks
// with an IAM Role. It also tests that the credentials ACKs are generated.
func TestHandlePayloadMessageCredentialsAckedWhenTaskAddedMultipleTasks(t *testing.T) {
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

	payloadAckSent := make(chan *ecsacs.AckRequest)
	firstCredentialsAckSent := make(chan *ecsacs.IAMRoleCredentialsAckRequest)
	secondCredentialsAckSent := make(chan *ecsacs.IAMRoleCredentialsAckRequest)
	testResponseSender := func(response interface{}) error {
		var credentialsResp *ecsacs.IAMRoleCredentialsAckRequest
		var payloadMessageResp *ecsacs.AckRequest
		credentialsResp, ok := response.(*ecsacs.IAMRoleCredentialsAckRequest)
		if ok {
			switch aws.StringValue(credentialsResp.CredentialsId) {
			case firstTaskCredentialsId:
				firstCredentialsAckSent <- credentialsResp
			case secondTaskCredentialsId:
				secondCredentialsAckSent <- credentialsResp
			default:
				t.Fatalf("credentials response credentials ID is not %s or %s",
					firstTaskCredentialsId, secondTaskCredentialsId)
			}
		} else {
			payloadMessageResp, ok = response.(*ecsacs.AckRequest)
			if ok {
				payloadAckSent <- payloadMessageResp
			} else {
				t.Fatal("response does not hold type ecsacs.IAMRoleCredentialsAckRequest or ecsacs.AckRequest")
			}
		}
		return nil
	}

	tester := setup(t, testResponseSender)
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

	// The payload message in the test consists of two tasks, with credentials set for both.
	testPayloadMessage.Tasks = []*ecsacs.Task{
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
	}

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	actualPayloadAck := <-payloadAckSent
	assert.Equal(t, expectedPayloadAck, actualPayloadAck)

	// Verify the correctness of the first task added to the engine and the
	// credentials ACK generated for it.
	expectedCredentialsAckForFirstTask := &ecsacs.IAMRoleCredentialsAckRequest{
		MessageId:     aws.String(testconst.MessageID),
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
	actualFirstCredentialsAck := <-firstCredentialsAckSent
	err := validateTaskAndCredentials(actualFirstCredentialsAck, expectedCredentialsAckForFirstTask, firstAddedTask,
		firstTaskArn, expectedCredentialsForFirstTask)
	assert.NoError(t, err, "error validating added task or credentials ACK for the same")

	// Verify the correctness of the second task added to the engine and the
	// credentials ACK generated for it.
	expectedCredentialsAckForSecondTask := &ecsacs.IAMRoleCredentialsAckRequest{
		MessageId:     aws.String(testconst.MessageID),
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
	actualSecondCredentialsAck := <-secondCredentialsAckSent
	err = validateTaskAndCredentials(actualSecondCredentialsAck, expectedCredentialsAckForSecondTask, secondAddedTask,
		secondTaskArn, expectedCredentialsForSecondTask)
	assert.NoError(t, err, "error validating added task or credentials ACK for the same")
}

// TestHandlePayloadMessageTaskAddsExecutionRoles tests the payload responder will add
// execution role credentials to the credentials manager and the field in
// task and container are appropriately set.
func TestHandlePayloadMessageTaskAddsExecutionRoles(t *testing.T) {
	payloadAckSent := make(chan *ecsacs.AckRequest)
	credentialsAckSent := make(chan *ecsacs.IAMRoleCredentialsAckRequest)
	testResponseSender := func(response interface{}) error {
		var credentialsResp *ecsacs.IAMRoleCredentialsAckRequest
		var payloadMessageResp *ecsacs.AckRequest
		credentialsResp, ok := response.(*ecsacs.IAMRoleCredentialsAckRequest)
		if ok {
			credentialsAckSent <- credentialsResp
		} else {
			payloadMessageResp, ok = response.(*ecsacs.AckRequest)
			if ok {
				payloadAckSent <- payloadMessageResp
			} else {
				t.Fatal("response does not hold type ecsacs.IAMRoleCredentialsAckRequest or ecsacs.AckRequest")
			}
		}
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task *apitask.Task) {
		addedTask = task
	})

	taskArn := "t1"
	credentialsExpiration := "expiration"
	credentialsRoleArn := "r1"
	credentialsAccessKey := "akid"
	credentialsSecretKey := "skid"
	credentialsSessionToken := "token"
	credentialsID := "credsid"

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
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
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	actualPayloadAck := <-payloadAckSent
	assert.Equal(t, expectedPayloadAck, actualPayloadAck)

	// Verify that added tasks' execution role credentials ID is as expected.
	assert.Equal(t, credentialsID, addedTask.GetExecutionCredentialsID(),
		"task execution role credentials id mismatch")

	// Verify the credentials in the payload message was stored in the credentials manager.
	actualCredentialsAck := <-credentialsAckSent
	iamRoleCredentials, ok := tester.payloadMessageHandler.credentialsManager.GetTaskCredentials(credentialsID)
	assert.True(t, ok, "execution role credentials not found in credentials manager")
	assert.Equal(t, credentialsRoleArn, iamRoleCredentials.IAMRoleCredentials.RoleArn)
	assert.Equal(t, taskArn, iamRoleCredentials.ARN)
	assert.Equal(t, credentialsExpiration, iamRoleCredentials.IAMRoleCredentials.Expiration)
	assert.Equal(t, credentialsAccessKey, iamRoleCredentials.IAMRoleCredentials.AccessKeyID)
	assert.Equal(t, credentialsSecretKey, iamRoleCredentials.IAMRoleCredentials.SecretAccessKey)
	assert.Equal(t, credentialsSessionToken, iamRoleCredentials.IAMRoleCredentials.SessionToken)
	assert.Equal(t, credentialsID, iamRoleCredentials.IAMRoleCredentials.CredentialsID)
	assert.Equal(t, credentialsID, *actualCredentialsAck.CredentialsId)
	assert.Equal(t, testconst.MessageID, *actualCredentialsAck.MessageId)
}

func TestHandlePayloadMessageAddedENIToTask(t *testing.T) {
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn: aws.String(testconst.TaskARN),
			ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
				{
					AttachmentArn: aws.String(attachmentARN),
					Ec2Id:         aws.String(ec2ID),
					Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
						{
							Primary:        aws.Bool(true),
							PrivateAddress: aws.String(testconst.IPv4Address),
						},
					},
					Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
						{
							Address: aws.String("ipv6"),
						},
					},
					SubnetGatewayIpv4Address: aws.String(testconst.GatewayIPv4),
					MacAddress:               aws.String(testconst.RandomMAC),
				},
			},
		},
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	// Validate the added task has the ENI information as expected.
	expectedENI := testPayloadMessage.Tasks[0].ElasticNetworkInterfaces[0]
	taskeni := addedTask.GetPrimaryENI()
	assert.Equal(t, aws.StringValue(expectedENI.Ec2Id), taskeni.ID)
	assert.Equal(t, aws.StringValue(expectedENI.MacAddress), taskeni.MacAddress)
	assert.Equal(t, 1, len(taskeni.IPV4Addresses))
	assert.Equal(t, 1, len(taskeni.IPV6Addresses))
	assert.Equal(t, aws.StringValue(expectedENI.Ipv4Addresses[0].PrivateAddress), taskeni.IPV4Addresses[0].Address)
	assert.Equal(t, aws.StringValue(expectedENI.Ipv6Addresses[0].Address), taskeni.IPV6Addresses[0].Address)
}

func TestHandlePayloadMessageAddedEBSToTask(t *testing.T) {
	testEBSReadOnly := false
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	handlePayloadMessage := tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn:           aws.String(testconst.TaskARN),
			DesiredStatus: aws.String("RUNNING"),
			Family:        aws.String("myFamily"),
			Version:       aws.String("1"),
			Containers: []*ecsacs.Container{
				{
					Name: aws.String("myName1"),
					MountPoints: []*ecsacs.MountPoint{
						{
							ContainerPath: aws.String("/foo"),
							SourceVolume:  aws.String("test-volume"),
							ReadOnly:      &testEBSReadOnly,
						},
					},
				},
			},
			Attachments: []*ecsacs.Attachment{
				{
					AttachmentArn: aws.String("attachmentArn"),
					AttachmentProperties: []*ecsacs.AttachmentProperty{
						{
							Name:  aws.String(apiresource.VolumeIdKey),
							Value: aws.String(taskresourcevolume.TestVolumeId),
						},
						{
							Name:  aws.String(apiresource.VolumeSizeGibKey),
							Value: aws.String(taskresourcevolume.TestVolumeSizeGib),
						},
						{
							Name:  aws.String(apiresource.DeviceNameKey),
							Value: aws.String(taskresourcevolume.TestDeviceName),
						},
						{
							Name:  aws.String(apiresource.SourceVolumeHostPathKey),
							Value: aws.String(taskresourcevolume.TestSourceVolumeHostPath),
						},
						{
							Name:  aws.String(apiresource.VolumeNameKey),
							Value: aws.String(taskresourcevolume.TestVolumeName),
						},
						{
							Name:  aws.String(apiresource.FileSystemKey),
							Value: aws.String(taskresourcevolume.TestFileSystem),
						},
					},
					AttachmentType: aws.String(apiresource.EBSTaskAttach),
				},
			},
			Volumes: []*ecsacs.Volume{
				{
					Name: aws.String(taskresourcevolume.TestVolumeName),
					Type: aws.String(apitask.AttachmentType),
				},
			},
		},
	}

	testExpectedEBSCfg := &taskresourcevolume.EBSTaskVolumeConfig{
		VolumeId:             "vol-12345",
		VolumeName:           "test-volume",
		VolumeSizeGib:        "10",
		SourceVolumeHostPath: "taskarn_vol-12345",
		DeviceName:           "/dev/nvme1n1",
		FileSystem:           "ext4",
	}

	handlePayloadMessage(testPayloadMessage)

	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	assert.Len(t, addedTask.Volumes, 1)
	ebsConfig, ok := addedTask.Volumes[0].Volume.(*taskresourcevolume.EBSTaskVolumeConfig)
	require.True(t, ok)
	assert.Equal(t, testExpectedEBSCfg, ebsConfig)
}

func TestHandlePayloadMessageAddedAppMeshToTask(t *testing.T) {
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

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

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn: aws.String(testconst.TaskARN),
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
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	// Validate the added task has the App Mesh information as expected.
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

func TestHandlePayloadMessageAddedENITrunkToTask(t *testing.T) {
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	vlanID := "12345"

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn: aws.String(testconst.TaskARN),
			ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
				{
					InterfaceAssociationProtocol: aws.String(ni.VLANInterfaceAssociationProtocol),
					AttachmentArn:                aws.String(attachmentARN),
					Ec2Id:                        aws.String(ec2ID),
					Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
						{
							Primary:        aws.Bool(true),
							PrivateAddress: aws.String(testconst.IPv4Address),
						},
					},
					Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
						{
							Address: aws.String("ipv6"),
						},
					},
					SubnetGatewayIpv4Address: aws.String(testconst.GatewayIPv4),
					MacAddress:               aws.String(testconst.RandomMAC),
					InterfaceVlanProperties: &ecsacs.NetworkInterfaceVlanProperties{
						VlanId:                   aws.String(vlanID),
						TrunkInterfaceMacAddress: aws.String(testconst.RandomMAC),
					},
				},
			},
		},
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	// Validate the added task has the ENI trunk information as expected.
	taskeni := addedTask.GetPrimaryENI()
	assert.Equal(t, ni.VLANInterfaceAssociationProtocol, taskeni.InterfaceAssociationProtocol)
	assert.Equal(t, testconst.RandomMAC, taskeni.InterfaceVlanProperties.TrunkInterfaceMacAddress)
	assert.Equal(t, vlanID, taskeni.InterfaceVlanProperties.VlanID)
}

func TestHandlePayloadMessageAddedECRAuthData(t *testing.T) {
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn: aws.String(testconst.TaskARN),
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
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	// Validate the added task has the ECRAuthData information as expected.
	expected := testPayloadMessage.Tasks[0].Containers[0].RegistryAuthentication
	actual := addedTask.Containers[0].RegistryAuthentication
	assert.NotNil(t, actual.ECRAuthData)
	assert.Nil(t, actual.ASMAuthData)
	assert.Equal(t, aws.StringValue(expected.Type), actual.Type)
	assert.Equal(t, aws.StringValue(expected.EcrAuthData.Region), actual.ECRAuthData.Region)
	assert.Equal(t, aws.StringValue(expected.EcrAuthData.RegistryId), actual.ECRAuthData.RegistryID)
}

func TestHandlePayloadMessageAddedASMAuthData(t *testing.T) {
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn: aws.String(testconst.TaskARN),
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
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	// Validate the added task has the ASMAuthData information as expected.
	expected := testPayloadMessage.Tasks[0].Containers[0].RegistryAuthentication
	actual := addedTask.Containers[0].RegistryAuthentication
	assert.NotNil(t, actual.ASMAuthData)
	assert.Nil(t, actual.ECRAuthData)
	assert.Equal(t, aws.StringValue(expected.Type), actual.Type)
	assert.Equal(t, aws.StringValue(expected.AsmAuthData.Region), actual.ASMAuthData.Region)
	assert.Equal(t, aws.StringValue(expected.AsmAuthData.CredentialsParameter), actual.ASMAuthData.CredentialsParameter)
}

func TestHandlePayloadMessageAddedFirelensData(t *testing.T) {
	ackSent := make(chan *ecsacs.AckRequest)
	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}

	tester := setup(t, testResponseSender)
	defer tester.ctrl.Finish()

	var addedTask *apitask.Task
	tester.mockTaskEngine.EXPECT().AddTask(gomock.Any()).Do(
		func(task *apitask.Task) {
			addedTask = task
		})

	// Send a payload message.
	handlePayloadMessage :=
		tester.payloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	testPayloadMessage.Tasks = []*ecsacs.Task{
		{
			Arn: aws.String(testconst.TaskARN),
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
	}
	handlePayloadMessage(testPayloadMessage)

	// Verify that payload message ACK is sent and is as expected.
	payloadAckSent := <-ackSent
	assert.Equal(t, expectedPayloadAck, payloadAckSent)

	// Validate the pieces of the Firelens container.
	expected := testPayloadMessage.Tasks[0].Containers[0].FirelensConfiguration
	actual := addedTask.Containers[0].FirelensConfig
	assert.Equal(t, aws.StringValue(expected.Type), actual.Type)
	assert.NotNil(t, actual.Options)
	assert.Equal(t, aws.StringValue(expected.Options["enable-ecs-log-metadata"]),
		actual.Options["enable-ecs-log-metadata"])
}

func TestHandleInvalidTask(t *testing.T) {
	tester := setup(t, nil)
	mockECSACSClient := mock_ecs.NewMockECSClient(tester.ctrl)
	taskHandler := eventhandler.NewTaskHandler(tester.ctx, data.NewNoopClient(), dockerstate.NewTaskEngineState(),
		mockECSACSClient)
	tester.payloadMessageHandler.ecsClient = mockECSACSClient
	tester.payloadMessageHandler.taskHandler = taskHandler
	defer tester.ctrl.Finish()

	ecsacsTask := &ecsacs.Task{Arn: aws.String(testconst.TaskARN)}
	testPayloadMessage.Tasks = []*ecsacs.Task{ecsacsTask}

	wait := &sync.WaitGroup{}
	wait.Add(1)

	mockECSACSClient.EXPECT().SubmitTaskStateChange(gomock.Any()).Do(func(change ecs.TaskStateChange) {
		assert.False(t, change.MetadataGetter.GetTaskIsNil())
		wait.Done()
	})

	tester.payloadMessageHandler.handleInvalidTask(ecsacsTask, errors.New("test error"), testPayloadMessage)
	wait.Wait()
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

// validateTaskAndCredentials compares a task and a credentials ACK object
// against expected values. It returns an error if either of the
// comparisons fail.
func validateTaskAndCredentials(taskCredentialsAck, expectedCredentialsAckForTask *ecsacs.IAMRoleCredentialsAckRequest,
	addedTask *apitask.Task,
	expectedTaskArn string,
	expectedTaskCredentials credentials.IAMRoleCredentials) error {
	if !reflect.DeepEqual(taskCredentialsAck, expectedCredentialsAckForTask) {
		return errors.Errorf("mismatch between expected and received credentials ACK requests, expected: %s, got: %s",
			expectedCredentialsAckForTask.String(), taskCredentialsAck.String())
	}

	expectedTask := &apitask.Task{
		Arn:                expectedTaskArn,
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		NetworkMode:        apitask.BridgeNetworkMode,
	}
	expectedTask.SetCredentialsID(expectedTaskCredentials.CredentialsID)

	if !reflect.DeepEqual(addedTask, expectedTask) {
		return errors.Errorf("mismatch between expected and added tasks, expected: %v, added: %v",
			expectedTask, addedTask)
	}
	return nil
}
