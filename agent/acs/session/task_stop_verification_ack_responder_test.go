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
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
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

// taskStopVerificationAckTestHelper wraps the common dependencies required for task stop verification ACK tests.
type taskStopVerificationAckTestHelper struct {
	ctrl                             *gomock.Controller
	taskStopVerificationAckResponder wsclient.RequestResponder
	taskEngine                       *mock_engine.MockTaskEngine
}

func setupTaskStopVerificationAckTest(t *testing.T) *taskStopVerificationAckTestHelper {
	ctrl := gomock.NewController(t)
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	manifestMessageIDAccessor := NewManifestMessageIDAccessor()
	manifestMessageIDAccessor.SetMessageID(testconst.MessageID)
	taskStopVerificationAckResponder := acssession.NewTaskStopVerificationACKResponder(
		NewTaskStopper(taskEngine),
		manifestMessageIDAccessor,
		metrics.NewNopEntryFactory())

	return &taskStopVerificationAckTestHelper{
		ctrl:                             ctrl,
		taskStopVerificationAckResponder: taskStopVerificationAckResponder,
		taskEngine:                       taskEngine,
	}
}

// defaultTasksOnInstance returns a baseline map of tasks that simulates/tracks the tasks on an instance.
func defaultTasksOnInstance() map[string]*task.Task {
	return map[string]*task.Task{
		taskARN1: {Arn: taskARN1, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
		taskARN2: {Arn: taskARN2, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
		taskARN3: {Arn: taskARN3, DesiredStatusUnsafe: apitaskstatus.TaskRunning},
	}
}

// defaultTaskStopVerificationAck returns a baseline task stop verification ACK to be used in testing.
func defaultTaskStopVerificationAck() *ecsacs.TaskStopVerificationAck {
	return &ecsacs.TaskStopVerificationAck{
		GeneratedAt: aws.Int64(testconst.DummyInt),
		MessageId:   aws.String(testconst.MessageID),
		StopTasks:   []*ecsacs.TaskIdentifier{},
	}
}

// TestTaskStopVerificationAckResponderStopsMultipleTasks tests the case where some tasks on the instance are stopped
// upon receiving a task stop verification ACK.
func TestTaskStopVerificationAckResponderStopsMultipleTasks(t *testing.T) {
	tester := setupTaskStopVerificationAckTest(t)
	defer tester.ctrl.Finish()

	// The below map is used to simulate/track the tasks on the instance for the purposes of this test.
	tasksOnInstance := defaultTasksOnInstance()

	// The below ACK contains a list of tasks which ACS confirms that Agent needs to stop.
	taskStopVerificationAck := defaultTaskStopVerificationAck()
	taskStopVerificationAck.StopTasks = []*ecsacs.TaskIdentifier{
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

	tester.taskEngine.EXPECT().GetTaskByArn(taskARN2).Return(tasksOnInstance[taskARN2], true)
	tester.taskEngine.EXPECT().GetTaskByArn(taskARN3).Return(tasksOnInstance[taskARN3], true)
	tester.taskEngine.EXPECT().AddTask(tasksOnInstance[taskARN2]).Do(func(task *task.Task) {
		task.SetDesiredStatus(apitaskstatus.TaskStopped)
	})
	tester.taskEngine.EXPECT().AddTask(tasksOnInstance[taskARN3]).Do(func(task *task.Task) {
		task.SetDesiredStatus(apitaskstatus.TaskStopped)
	})

	handleTaskStopVerificationAck :=
		tester.taskStopVerificationAckResponder.HandlerFunc().(func(message *ecsacs.TaskStopVerificationAck))
	handleTaskStopVerificationAck(taskStopVerificationAck)

	// Only task2 and task3 should be stopped.
	assert.Equal(t, apitaskstatus.TaskRunning, tasksOnInstance[taskARN1].GetDesiredStatus())
	assert.Equal(t, apitaskstatus.TaskStopped, tasksOnInstance[taskARN2].GetDesiredStatus())
	assert.Equal(t, apitaskstatus.TaskStopped, tasksOnInstance[taskARN3].GetDesiredStatus())

}

// TestTaskStopVerificationAckResponderStopsAllTasks tests the case where all tasks on the instance are stopped
// upon receiving a task stop verification ACK.
func TestTaskStopVerificationAckResponderStopsAllTasks(t *testing.T) {
	tester := setupTaskStopVerificationAckTest(t)
	defer tester.ctrl.Finish()

	// The below map is used to simulate/track the tasks on the instance for the purposes of this test.
	tasksOnInstance := defaultTasksOnInstance()

	// The below ACK contains a list of tasks which ACS confirms that Agent needs to stop.
	taskStopVerificationAck := defaultTaskStopVerificationAck()
	taskStopVerificationAck.StopTasks = []*ecsacs.TaskIdentifier{
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
		{
			DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
			TaskArn:        aws.String(taskARN3),
			TaskClusterArn: aws.String(testconst.ClusterARN),
		},
	}

	tester.taskEngine.EXPECT().GetTaskByArn(taskARN1).Return(tasksOnInstance[taskARN1], true)
	tester.taskEngine.EXPECT().GetTaskByArn(taskARN2).Return(tasksOnInstance[taskARN2], true)
	tester.taskEngine.EXPECT().GetTaskByArn(taskARN3).Return(tasksOnInstance[taskARN3], true)
	tester.taskEngine.EXPECT().AddTask(tasksOnInstance[taskARN1]).Do(func(task *task.Task) {
		task.SetDesiredStatus(apitaskstatus.TaskStopped)
	})
	tester.taskEngine.EXPECT().AddTask(tasksOnInstance[taskARN2]).Do(func(task *task.Task) {
		task.SetDesiredStatus(apitaskstatus.TaskStopped)
	})
	tester.taskEngine.EXPECT().AddTask(tasksOnInstance[taskARN3]).Do(func(task *task.Task) {
		task.SetDesiredStatus(apitaskstatus.TaskStopped)
	})

	handleTaskStopVerificationAck :=
		tester.taskStopVerificationAckResponder.HandlerFunc().(func(message *ecsacs.TaskStopVerificationAck))
	handleTaskStopVerificationAck(taskStopVerificationAck)

	// All tasks on instance should be stopped.
	for _, task := range tasksOnInstance {
		assert.Equal(t, apitaskstatus.TaskStopped, task.GetDesiredStatus())
	}
}
