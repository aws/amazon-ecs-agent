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
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	mock_statemanager "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	mock_wsclient "github.com/aws/amazon-ecs-agent/agent/wsclient/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// Tests the case when all the tasks running on the instance needs to be killed
func TestManifestHandlerKillAllTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	manager := mock_statemanager.NewMockStateManager(ctrl)
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	cluster := "mock-cluster"
	containerInstanceArn := "mock-container-instance"
	messageId := "mock-message-id"

	ctx := context.TODO()
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)

	newTaskManifest := newTaskManifestHandler(ctx, cluster, containerInstanceArn, mockWSClient, manager, taskEngine,
		aws.Int64(11))

	ackRequested := &ecsacs.AckRequest{
		Cluster:           aws.String(cluster),
		ContainerInstance: aws.String(containerInstanceArn),
		MessageId:         aws.String(messageId),
	}

	task2 := &task.Task{Arn: "arn2", DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	task1 := &task.Task{Arn: "arn1", DesiredStatusUnsafe: apitaskstatus.TaskRunning}

	taskList := []*task.Task{task1, task2}

	//Task that needs to be stopped, sent back by agent
	taskIdentifierFinal := []*ecsacs.TaskIdentifier{
		{DesiredStatus: aws.String(apitaskstatus.TaskStoppedString), TaskArn: aws.String("arn1")},
		{DesiredStatus: aws.String(apitaskstatus.TaskStoppedString), TaskArn: aws.String("arn2")},
	}

	taskStopVerificationMessage := &ecsacs.TaskStopVerificationMessage{
		MessageId:      aws.String(messageId),
		StopCandidates: taskIdentifierFinal,
	}

	messageTaskStopVerificationAck := &ecsacs.TaskStopVerificationAck{
		GeneratedAt: aws.Int64(123),
		MessageId:   aws.String(messageId),
		StopTasks:   taskIdentifierFinal,
	}

	gomock.InOrder(
		taskEngine.EXPECT().ListTasks().Return(taskList, nil).Times(1),
		manager.EXPECT().Save().Return(nil).Times(1),
		// AddTask function needs to be called twice for both the tasks getting stopped
		taskEngine.EXPECT().AddTask(gomock.Any()),
		taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task1 *task.Task) {
			newTaskManifest.stop()
		}),
	)

	mockWSClient.EXPECT().MakeRequest(ackRequested).Times(1)

	mockWSClient.EXPECT().MakeRequest(taskStopVerificationMessage).Times(1).Do(func(message *ecsacs.TaskStopVerificationMessage) {
		// Agent receives the ack message when taskStopVerificationMessage is processed by ACS
		newTaskManifest.messageBufferTaskStopVerificationAck <- messageTaskStopVerificationAck
	})

	taskEngine.EXPECT().GetTaskByArn("arn1").Return(task1, true)
	taskEngine.EXPECT().GetTaskByArn("arn2").Return(task2, true)

	message := &ecsacs.TaskManifestMessage{
		MessageId:            aws.String(messageId),
		ClusterArn:           aws.String(cluster),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		Tasks: []*ecsacs.TaskIdentifier{
			{DesiredStatus: aws.String("STOPPED"), TaskArn: aws.String("arn-long")},
		},
		Timeline: aws.Int64(12),
	}

	go newTaskManifest.start()

	newTaskManifest.messageBufferTaskManifest <- message

	// mockWSClient.EXPECT().MakeRequest(ackRequested).Times(1) in this test is called by an asynchronous routine.
	// Sometimes functions execution finishes before a call to this asynchronous routine, this sleep will ensure that
	// asynchronous routine is called before function ends
	time.Sleep(2 * time.Second)

	select {
	case <-newTaskManifest.ctx.Done():
	}
}

// Tests the case when two of three tasks running on the instance needs to be killed
func TestManifestHandlerKillFewTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	manager := mock_statemanager.NewMockStateManager(ctrl)
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	cluster := "mock-cluster"
	containerInstanceArn := "mock-container-instance"
	messageId := "mock-message-id"

	ctx := context.TODO()
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)

	newTaskManifest := newTaskManifestHandler(ctx, cluster, containerInstanceArn, mockWSClient, manager, taskEngine,
		aws.Int64(11))

	ackRequested := &ecsacs.AckRequest{
		Cluster:           aws.String(cluster),
		ContainerInstance: aws.String(containerInstanceArn),
		MessageId:         aws.String(messageId),
	}

	task2 := &task.Task{Arn: "arn2", DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	task1 := &task.Task{Arn: "arn1", DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	task3 := &task.Task{Arn: "arn3", DesiredStatusUnsafe: apitaskstatus.TaskRunning}

	taskList := []*task.Task{task1, task2, task3}

	//Task that needs to be stopped, sent back by agent
	taskIdentifierFinal := []*ecsacs.TaskIdentifier{
		{DesiredStatus: aws.String(apitaskstatus.TaskStoppedString), TaskArn: aws.String("arn2")},
		{DesiredStatus: aws.String(apitaskstatus.TaskStoppedString), TaskArn: aws.String("arn3")},
	}

	taskStopVerificationMessage := &ecsacs.TaskStopVerificationMessage{
		MessageId:      aws.String(messageId),
		StopCandidates: taskIdentifierFinal,
	}

	messageTaskStopVerificationAck := &ecsacs.TaskStopVerificationAck{
		GeneratedAt: aws.Int64(123),
		MessageId:   aws.String(messageId),
		StopTasks:   taskIdentifierFinal,
	}

	gomock.InOrder(
		taskEngine.EXPECT().ListTasks().Return(taskList, nil).Times(1),
		manager.EXPECT().Save().Return(nil).Times(1),
		taskEngine.EXPECT().AddTask(gomock.Any()),
		taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(task1 *task.Task) {
			newTaskManifest.stop()
		}),
	)

	mockWSClient.EXPECT().MakeRequest(ackRequested).Times(1)

	mockWSClient.EXPECT().MakeRequest(taskStopVerificationMessage).Times(1).Do(func(message *ecsacs.TaskStopVerificationMessage) {
		newTaskManifest.messageBufferTaskStopVerificationAck <- messageTaskStopVerificationAck
	})

	taskEngine.EXPECT().GetTaskByArn("arn3").Return(task1, true)
	taskEngine.EXPECT().GetTaskByArn("arn2").Return(task2, true)

	message := &ecsacs.TaskManifestMessage{
		MessageId:            aws.String(messageId),
		ClusterArn:           aws.String(cluster),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		Tasks: []*ecsacs.TaskIdentifier{
			{
				DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
				TaskArn:       aws.String("arn1"),
			},
			{
				DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
				TaskArn:       aws.String("arn2"),
			},
		},
		Timeline: aws.Int64(12),
	}

	go newTaskManifest.start()

	newTaskManifest.messageBufferTaskManifest <- message

	// mockWSClient.EXPECT().MakeRequest(ackRequested).Times(1) in this test is called by an asynchronous routine.
	// Sometimes functions execution finishes before a call to this asynchronous routine, this sleep will ensure that
	// asynchronous routine is called before function ends
	time.Sleep(2 * time.Second)

	select {
	case <-newTaskManifest.ctx.Done():
	}
}

// Tests the case when their is no difference in task running on the instance and tasks received in task manifest. No
// tasks on the instance needs to be killed
func TestManifestHandlerKillNoTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	manager := mock_statemanager.NewMockStateManager(ctrl)
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	cluster := "mock-cluster"
	containerInstanceArn := "mock-container-instance"
	messageId := "mock-message-id"

	ctx := context.TODO()
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)

	newTaskManifest := newTaskManifestHandler(ctx, cluster, containerInstanceArn, mockWSClient, manager, taskEngine,
		aws.Int64(11))

	ackRequested := &ecsacs.AckRequest{
		Cluster:           aws.String(cluster),
		ContainerInstance: aws.String(containerInstanceArn),
		MessageId:         aws.String(messageId),
	}

	task2 := &task.Task{Arn: "arn2", DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	task1 := &task.Task{Arn: "arn1", DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	task3 := &task.Task{Arn: "arn3", DesiredStatusUnsafe: apitaskstatus.TaskRunning}

	taskList := []*task.Task{task1, task2, task3}

	//Task that needs to be stopped, sent back by agent
	taskIdentifierFinal := []*ecsacs.TaskIdentifier{
		{DesiredStatus: aws.String("STOPPED"), TaskArn: aws.String("arn2")},
		{DesiredStatus: aws.String("STOPPED"), TaskArn: aws.String("arn3")},
	}

	taskStopVerificationMessage := &ecsacs.TaskStopVerificationMessage{
		MessageId:      aws.String(messageId),
		StopCandidates: taskIdentifierFinal,
	}

	gomock.InOrder(
		taskEngine.EXPECT().ListTasks().Return(taskList, nil).Times(1),
		manager.EXPECT().Save().Return(nil).Times(1),
	)

	mockWSClient.EXPECT().MakeRequest(taskStopVerificationMessage).Times(0)
	mockWSClient.EXPECT().MakeRequest(ackRequested).Times(1).Do(func(message *ecsacs.AckRequest) {
		newTaskManifest.stop()
	})

	message := &ecsacs.TaskManifestMessage{
		MessageId:            aws.String(messageId),
		ClusterArn:           aws.String(cluster),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		Tasks: []*ecsacs.TaskIdentifier{
			{
				DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
				TaskArn:       aws.String("arn1"),
			},
			{
				DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
				TaskArn:       aws.String("arn2"),
			},
			{
				DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
				TaskArn:       aws.String("arn3"),
			},
		},
		Timeline: aws.Int64(12),
	}

	go newTaskManifest.start()

	newTaskManifest.messageBufferTaskManifest <- message

	// mockWSClient.EXPECT().MakeRequest(ackRequested).Times(1) in this test is called by an asynchronous routine.
	// Sometimes functions execution finishes before a call to this asynchronous routine, this sleep will ensure that
	// asynchronous routine is called before function ends
	time.Sleep(2 * time.Second)

	select {
	case <-newTaskManifest.ctx.Done():
	}
}

// Tests the case when the task list received in TaskManifest message is different than the one received in
// TaskStopVerificationMessage
func TestManifestHandlerDifferentTaskLists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	manager := mock_statemanager.NewMockStateManager(ctrl)
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	cluster := "mock-cluster"
	containerInstanceArn := "mock-container-instance"
	messageId := "mock-message-id"

	ctx := context.TODO()
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)

	newTaskManifest := newTaskManifestHandler(ctx, cluster, containerInstanceArn, mockWSClient, manager, taskEngine,
		aws.Int64(11))

	ackRequested := &ecsacs.AckRequest{
		Cluster:           aws.String(cluster),
		ContainerInstance: aws.String(containerInstanceArn),
		MessageId:         aws.String(messageId),
	}

	task2 := &task.Task{Arn: "arn2", DesiredStatusUnsafe: apitaskstatus.TaskRunning}
	task1 := &task.Task{Arn: "arn1", DesiredStatusUnsafe: apitaskstatus.TaskRunning}

	taskList := []*task.Task{task1, task2}

	// tasks that suppose to be running
	taskIdentifierInitial := ecsacs.TaskIdentifier{
		DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
		TaskArn:       aws.String("arn1"),
	}

	//Task that needs to be stopped, sent back by agent
	taskIdentifierAckFinal := []*ecsacs.TaskIdentifier{
		{DesiredStatus: aws.String(apitaskstatus.TaskRunningString), TaskArn: aws.String("arn1")},
		{DesiredStatus: aws.String(apitaskstatus.TaskStoppedString), TaskArn: aws.String("arn2")},
	}

	//Task that needs to be stopped, sent back by agent
	taskIdentifierMessage := []*ecsacs.TaskIdentifier{
		{DesiredStatus: aws.String(apitaskstatus.TaskStoppedString), TaskArn: aws.String("arn1")},
		{DesiredStatus: aws.String(apitaskstatus.TaskStoppedString), TaskArn: aws.String("arn2")},
	}

	taskStopVerificationMessage := &ecsacs.TaskStopVerificationMessage{
		MessageId:      aws.String(messageId),
		StopCandidates: taskIdentifierMessage,
	}

	messageTaskStopVerificationAck := &ecsacs.TaskStopVerificationAck{
		GeneratedAt: aws.Int64(123),
		MessageId:   aws.String(messageId),
		StopTasks:   taskIdentifierAckFinal,
	}

	gomock.InOrder(
		taskEngine.EXPECT().ListTasks().Return(taskList, nil).Times(1),
		manager.EXPECT().Save().Return(nil).Times(1),
		taskEngine.EXPECT().AddTask(gomock.Any()).Times(1).Do(func(task1 *task.Task) {
			newTaskManifest.stop()
		}),
	)

	mockWSClient.EXPECT().MakeRequest(ackRequested).Times(1)

	mockWSClient.EXPECT().MakeRequest(taskStopVerificationMessage).Times(1).Do(func(
		message *ecsacs.TaskStopVerificationMessage) {
		newTaskManifest.messageBufferTaskStopVerificationAck <- messageTaskStopVerificationAck
	})

	taskEngine.EXPECT().GetTaskByArn("arn1").Times(0)
	taskEngine.EXPECT().GetTaskByArn("arn2").Return(task2, true)

	message := &ecsacs.TaskManifestMessage{
		MessageId:            aws.String(messageId),
		ClusterArn:           aws.String(cluster),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		Tasks: []*ecsacs.TaskIdentifier{
			&taskIdentifierInitial,
		},
		Timeline: aws.Int64(12),
	}

	go newTaskManifest.start()

	newTaskManifest.messageBufferTaskManifest <- message

	// mockWSClient.EXPECT().MakeRequest(ackRequested).Times(1) in this test is called by an asynchronous routine.
	// Sometimes functions execution finishes before a call to this asynchronous routine, this sleep will ensure that
	// asynchronous routine is called before function ends
	time.Sleep(2 * time.Second)

	select {
	case <-newTaskManifest.ctx.Done():
	}
}

func TestManifestHandlerSequenceNumbers(t *testing.T) {
	testcases := []struct {
		name                string
		inputSequenceNumber int64
	}{
		{
			name:                "Tests the case when sequence number received is older than the one stored in agent",
			inputSequenceNumber: 13,
		},
		{
			name:                "Tests the case when sequence number received is equal to one stored in agent",
			inputSequenceNumber: 12,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			manager := mock_statemanager.NewMockStateManager(ctrl)
			taskEngine := mock_engine.NewMockTaskEngine(ctrl)

			ctx := context.TODO()
			mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
			newTaskManifest := newTaskManifestHandler(ctx, cluster, containerInstanceArn, mockWSClient, manager,
				taskEngine, aws.Int64(tc.inputSequenceNumber))

			taskList := []*task.Task{
				{Arn: "arn2", DesiredStatusUnsafe: apitaskstatus.TaskRunning},
				{Arn: "arn1", DesiredStatusUnsafe: apitaskstatus.TaskRunning},
			}

			gomock.InOrder(
				taskEngine.EXPECT().ListTasks().Return(taskList, nil).Times(0),
				taskEngine.EXPECT().AddTask(gomock.Any()).Times(0),
				manager.EXPECT().Save().Return(nil).Times(0),
			)

			message := &ecsacs.TaskManifestMessage{
				MessageId:            aws.String(eniMessageId),
				ClusterArn:           aws.String(clusterName),
				ContainerInstanceArn: aws.String(containerInstanceArn),
				Tasks: []*ecsacs.TaskIdentifier{
					{
						DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:       aws.String("arn-long"),
					},
					{
						DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
						TaskArn:       aws.String("arn-long-1"),
					},
				},
				Timeline: aws.Int64(12),
			}
			err := newTaskManifest.handleTaskManifestSingleMessage(message)
			assert.NoError(t, err)

		})
	}
}

func TestCompareTasksDifferentTasks(t *testing.T) {
	receivedTaskList := []*ecsacs.TaskIdentifier{
		{
			DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
			TaskArn:       aws.String("arn-long"),
		},
		{
			DesiredStatus: aws.String(apitaskstatus.TaskStoppedString),
			TaskArn:       aws.String("arn-long-1"),
		},
	}

	taskList := []*task.Task{
		{Arn: "arn2", DesiredStatusUnsafe: apitaskstatus.TaskRunning},
		{Arn: "arn1", DesiredStatusUnsafe: apitaskstatus.TaskRunning},
	}

	compareTaskList := compareTasks(receivedTaskList, taskList)

	assert.Equal(t, 2, len(compareTaskList))
}

func TestCompareTasksSameTasks(t *testing.T) {
	receivedTaskList := []*ecsacs.TaskIdentifier{
		{
			DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
			TaskArn:       aws.String("arn1"),
		},
		{
			DesiredStatus: aws.String(apitaskstatus.TaskRunningString),
			TaskArn:       aws.String("arn2"),
		},
	}

	taskList := []*task.Task{
		{Arn: "arn2", DesiredStatusUnsafe: apitaskstatus.TaskRunning},
		{Arn: "arn1", DesiredStatusUnsafe: apitaskstatus.TaskRunning},
	}

	compareTaskList := compareTasks(receivedTaskList, taskList)

	assert.Equal(t, 0, len(compareTaskList))
}
