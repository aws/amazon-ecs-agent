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
	"strconv"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	acssession "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	initialSeqNum = 11
	nextSeqNum    = 12
	taskARN1      = "arn1"
	taskARN2      = "arn2"
	taskARN3      = "arn3"
)

var expectedTaskManifestAck = &ecsacs.AckRequest{
	Cluster:           aws.String(testconst.ClusterARN),
	ContainerInstance: aws.String(testconst.ContainerInstanceARN),
	MessageId:         aws.String(testconst.MessageID),
}

// taskManifestTestHelper wraps the common dependencies required for task manifest tests.
type taskManifestTestHelper struct {
	ctrl                  *gomock.Controller
	taskManifestResponder wsclient.RequestResponder
	taskEngine            *mock_engine.MockTaskEngine
	dataClient            data.Client
}

func setupTaskManifestTest(t *testing.T, acsResponseSender wsclient.RespondFunc) *taskManifestTestHelper {
	ctrl := gomock.NewController(t)
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	dataClient := newTestDataClient(t)
	taskManifestResponder := acssession.NewTaskManifestResponder(
		NewTaskComparer(taskEngine),
		NewSequenceNumberAccessor(aws.Int64(initialSeqNum), dataClient),
		NewManifestMessageIDAccessor(),
		metrics.NewNopEntryFactory(),
		acsResponseSender)

	return &taskManifestTestHelper{
		ctrl:                  ctrl,
		taskManifestResponder: taskManifestResponder,
		taskEngine:            taskEngine,
		dataClient:            dataClient,
	}
}

// defaultTaskManifestMessage returns a baseline task manifest message to be used in testing.
func defaultTaskManifestMessage() *ecsacs.TaskManifestMessage {
	return &ecsacs.TaskManifestMessage{
		MessageId:            aws.String(testconst.MessageID),
		ClusterArn:           aws.String(testconst.ClusterARN),
		ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
		Tasks:                []*ecsacs.TaskIdentifier{},
		Timeline:             aws.Int64(nextSeqNum),
	}
}

// defaultTaskStopVerificationMessage returns a baseline task stop verification message to be used in testing.
func defaultTaskStopVerificationMessage() *ecsacs.TaskStopVerificationMessage {
	return &ecsacs.TaskStopVerificationMessage{
		MessageId:      aws.String(testconst.MessageID),
		StopCandidates: []*ecsacs.TaskIdentifier{},
	}
}

// TestTaskManifestResponder tests various cases of some amount of tasks on the instance needing to be stopped
// upon receiving a given task manifest message.
func TestTaskManifestResponder(t *testing.T) {
	testCases := []struct {
		name                     string
		tasksInEngine            []*task.Task
		taskManifestMsgMutation  func(message *ecsacs.TaskManifestMessage)
		taskStopVerifMsgMutation func(message *ecsacs.TaskStopVerificationMessage)
	}{
		{
			name: "All tasks to be stopped",
			tasksInEngine: []*task.Task{
				{Arn: taskARN1, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
				{Arn: taskARN2, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
			},
			taskManifestMsgMutation: func(message *ecsacs.TaskManifestMessage) {
				message.Tasks = []*ecsacs.TaskIdentifier{
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:        aws.String(testconst.TaskARN),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
				}
			},
			taskStopVerifMsgMutation: func(message *ecsacs.TaskStopVerificationMessage) {
				message.StopCandidates = []*ecsacs.TaskIdentifier{
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:        aws.String(taskARN1),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:        aws.String(taskARN2),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
				}
			},
		},
		{
			name: "Multiple tasks to be stopped",
			tasksInEngine: []*task.Task{
				{Arn: taskARN1, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
				{Arn: taskARN2, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
				{Arn: taskARN3, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
			},
			taskManifestMsgMutation: func(message *ecsacs.TaskManifestMessage) {
				message.Tasks = []*ecsacs.TaskIdentifier{
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskRunningString),
						TaskArn:        aws.String(taskARN1),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:        aws.String(taskARN2),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
				}
			},
			taskStopVerifMsgMutation: func(message *ecsacs.TaskStopVerificationMessage) {
				message.StopCandidates = []*ecsacs.TaskIdentifier{
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:        aws.String(taskARN2),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:        aws.String(taskARN3),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
				}
			},
		},
		{
			name: "One task to be stopped",
			tasksInEngine: []*task.Task{
				{Arn: taskARN1, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
				{Arn: taskARN2, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
			},
			taskManifestMsgMutation: func(message *ecsacs.TaskManifestMessage) {
				message.Tasks = []*ecsacs.TaskIdentifier{
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskRunningString),
						TaskArn:        aws.String(taskARN1),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
				}
			},
			taskStopVerifMsgMutation: func(message *ecsacs.TaskStopVerificationMessage) {
				message.StopCandidates = []*ecsacs.TaskIdentifier{
					{
						DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:        aws.String(taskARN2),
						TaskClusterArn: aws.String(testconst.ClusterARN),
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var taskManifestAckSent *ecsacs.AckRequest
			var taskStopVerificationSent *ecsacs.TaskStopVerificationMessage
			testResponseSender := func(response interface{}) error {
				messageAck, isTaskManifestAck := response.(*ecsacs.AckRequest)
				if isTaskManifestAck {
					taskManifestAckSent = messageAck
				} else {
					stopVerif, isTaskStopVerification := response.(*ecsacs.TaskStopVerificationMessage)
					if isTaskStopVerification {
						taskStopVerificationSent = stopVerif
					} else {
						t.Fatal("response does not hold type ecsacs.AckRequest or ecsacs.TaskStopVerificationMessage")
					}
				}
				return nil
			}

			tester := setupTaskManifestTest(t, testResponseSender)
			defer tester.ctrl.Finish()

			// Task manifest message contains task manifest and is received by Agent.
			message := defaultTaskManifestMessage()
			tc.taskManifestMsgMutation(message)

			// Task engine contains list of tasks on the instance.
			tester.taskEngine.EXPECT().ListTasks().Return(tc.tasksInEngine, nil)

			// Tasks stop verification message contains list of tasks that need to be stopped and is sent by Agent.
			expectedTaskStopVerification := defaultTaskStopVerificationMessage()
			tc.taskStopVerifMsgMutation(expectedTaskStopVerification)

			handleTaskManifestMessage :=
				tester.taskManifestResponder.HandlerFunc().(func(message *ecsacs.TaskManifestMessage))
			handleTaskManifestMessage(message)

			// Verify that task manifest message ACK is as expected.
			assert.Equal(t, expectedTaskManifestAck, taskManifestAckSent)

			// Verify that task stop verification message is as expected.
			assert.Equal(t, expectedTaskStopVerification, taskStopVerificationSent)

			// Verify that new seq num has been correctly saved in database.
			seqnum, err := tester.dataClient.GetMetadata(data.TaskManifestSeqNumKey)
			assert.NoError(t, err)
			assert.Equal(t, strconv.FormatInt(int64(nextSeqNum), 10), seqnum)
		})
	}
}

// TestTaskManifestResponderNoTasksToBeStopped tests the case where no tasks on the instance need to be stopped
// upon receiving a task manifest message.
func TestTaskManifestResponderNoTasksToBeStopped(t *testing.T) {
	var taskManifestAckSent *ecsacs.AckRequest
	var taskStopVerificationWasSent bool
	testResponseSender := func(response interface{}) error {
		messageAck, isTaskManifestAck := response.(*ecsacs.AckRequest)
		if isTaskManifestAck {
			taskManifestAckSent = messageAck
		} else {
			_, isTaskStopVerification := response.(*ecsacs.TaskStopVerificationMessage)
			if isTaskStopVerification {
				taskStopVerificationWasSent = true
			} else {
				t.Fatal("response does not hold type ecsacs.AckRequest or ecsacs.TaskStopVerificationMessage")
			}
		}
		return nil
	}

	tester := setupTaskManifestTest(t, testResponseSender)
	defer tester.ctrl.Finish()

	// Task manifest message contains task manifest and is received by Agent.
	message := defaultTaskManifestMessage()
	message.Tasks = []*ecsacs.TaskIdentifier{
		{
			DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
			TaskArn:       aws.String(taskARN1),
		},
		{
			DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
			TaskArn:       aws.String(taskARN2),
		},
		{
			DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
			TaskArn:       aws.String(taskARN3),
		},
	}

	// Task engine contains list of tasks on the instance.
	task1 := &task.Task{Arn: taskARN1, DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	task2 := &task.Task{Arn: taskARN2, DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	task3 := &task.Task{Arn: taskARN3, DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	tester.taskEngine.EXPECT().ListTasks().Return([]*task.Task{task1, task2, task3}, nil)

	// NOTE: There are no tasks that need to be stopped for this test case.

	handleTaskManifestMessage :=
		tester.taskManifestResponder.HandlerFunc().(func(message *ecsacs.TaskManifestMessage))
	handleTaskManifestMessage(message)

	// Verify that task manifest message ACK is sent and is as expected.
	assert.Equal(t, expectedTaskManifestAck, taskManifestAckSent)

	// Verify that task stop verification message is not sent.
	assert.False(t, taskStopVerificationWasSent)

	// Verify that new seq num has been correctly saved in database.
	seqnum, err := tester.dataClient.GetMetadata(data.TaskManifestSeqNumKey)
	assert.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(int64(nextSeqNum), 10), seqnum)
}

func TestTaskManifestResponderStaleMessage(t *testing.T) {
	testcases := []struct {
		name                  string
		initialSequenceNumber int64
	}{
		{
			name:                  "Tests the case when sequence number received is older than the one stored in agent",
			initialSequenceNumber: 13,
		},
		{
			name:                  "Tests the case when sequence number received is equal to one stored in agent",
			initialSequenceNumber: 12,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taskEngine := mock_engine.NewMockTaskEngine(ctrl)
			dataClient := newTestDataClient(t)

			var taskManifestAckWasSent, taskStopVerificationWasSent bool
			testResponseSender := func(response interface{}) error {
				_, isTaskManifestAck := response.(*ecsacs.AckRequest)
				if isTaskManifestAck {
					taskManifestAckWasSent = true
				} else {
					_, isTaskStopVerification := response.(*ecsacs.TaskStopVerificationMessage)
					if isTaskStopVerification {
						taskStopVerificationWasSent = true
					} else {
						t.Fatal("response does not hold type ecsacs.AckRequest or ecsacs.TaskStopVerificationMessage")
					}
				}
				return nil
			}

			// Save the initial sequence number.
			assert.NoError(t, dataClient.SaveMetadata(data.TaskManifestSeqNumKey,
				strconv.FormatInt(tc.initialSequenceNumber, 10)))

			taskManifestResponder := acssession.NewTaskManifestResponder(
				NewTaskComparer(taskEngine),
				NewSequenceNumberAccessor(aws.Int64(tc.initialSequenceNumber), dataClient),
				NewManifestMessageIDAccessor(),
				metrics.NewNopEntryFactory(),
				testResponseSender)

			message := &ecsacs.TaskManifestMessage{
				MessageId:            aws.String(testconst.MessageID),
				ClusterArn:           aws.String(testconst.ClusterARN),
				ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
				Tasks: []*ecsacs.TaskIdentifier{
					{
						DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:       aws.String(taskARN1),
					},
					{
						DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:       aws.String(taskARN2),
					},
				},
				Timeline: aws.Int64(nextSeqNum),
			}

			taskList := []*task.Task{
				{Arn: taskARN1, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
				{Arn: taskARN2, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
			}
			taskEngine.EXPECT().ListTasks().Return(taskList, nil).Times(0)

			handleTaskManifestMessage :=
				taskManifestResponder.HandlerFunc().(func(message *ecsacs.TaskManifestMessage))
			handleTaskManifestMessage(message)

			// Verify that task manifest message ACK was not sent (task manifest message was ignored/discarded).
			assert.False(t, taskManifestAckWasSent)

			// Verify that task stop verification message was not sent (task manifest message was ignored/discarded).
			assert.False(t, taskStopVerificationWasSent)

			// Verify that the sequence number in db remains unchanged.
			s, err := dataClient.GetMetadata(data.TaskManifestSeqNumKey)
			assert.NoError(t, err)
			seqNum, err := strconv.ParseInt(s, 10, 64)
			assert.NoError(t, err)
			assert.Equal(t, tc.initialSequenceNumber, seqNum)
		})
	}
}

func TestCompareTasksAllDifferentTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngine := mock_engine.NewMockTaskEngine(ctrl)

	message := &ecsacs.TaskManifestMessage{
		MessageId:            aws.String(testconst.MessageID),
		ClusterArn:           aws.String(testconst.ClusterARN),
		ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
		Tasks: []*ecsacs.TaskIdentifier{
			{
				DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
				TaskArn:       aws.String(taskARN1),
			},
			{
				DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
				TaskArn:       aws.String(taskARN2),
			},
		},
		Timeline: aws.Int64(nextSeqNum),
	}

	tasksInEngine := []*task.Task{
		{Arn: taskARN3, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
		{Arn: testconst.TaskARN, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
	}
	taskEngine.EXPECT().ListTasks().Return(tasksInEngine, nil)

	taskComparer := NewTaskComparer(taskEngine)
	tasksToStop, err := taskComparer.CompareRunningTasksOnInstanceWithManifest(message)

	assert.NoError(t, err)
	assert.Equal(t, len(tasksInEngine), len(tasksToStop))
}

func TestCompareTasksAllSameTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngine := mock_engine.NewMockTaskEngine(ctrl)

	message := &ecsacs.TaskManifestMessage{
		MessageId:            aws.String(testconst.MessageID),
		ClusterArn:           aws.String(testconst.ClusterARN),
		ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
		Tasks: []*ecsacs.TaskIdentifier{
			{
				DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
				TaskArn:       aws.String(taskARN1),
			},
			{
				DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
				TaskArn:       aws.String(taskARN2),
			},
		},
		Timeline: aws.Int64(nextSeqNum),
	}

	tasksInEngine := []*task.Task{
		{Arn: taskARN1, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
		{Arn: taskARN2, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
	}
	taskEngine.EXPECT().ListTasks().Return(tasksInEngine, nil)

	taskComparer := NewTaskComparer(taskEngine)
	tasksToStop, err := taskComparer.CompareRunningTasksOnInstanceWithManifest(message)

	assert.NoError(t, err)
	assert.Equal(t, 0, len(tasksToStop))
}
