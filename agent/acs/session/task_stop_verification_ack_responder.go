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
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
)

// taskStopper implements the TaskStopper interface defined in ecs-agent module.
type taskStopper struct {
	taskEngine engine.TaskEngine
	dataClient data.Client
}

// NewTaskStopper creates a new taskStopper.
func NewTaskStopper(taskEngine engine.TaskEngine, dataClient data.Client) *taskStopper {
	return &taskStopper{
		taskEngine: taskEngine,
		dataClient: dataClient,
	}
}

func (ts *taskStopper) StopTask(taskARN string) {
	task, isPresent := ts.taskEngine.GetTaskByArn(taskARN)
	if isPresent {
		logger.Info("Stopping task from task stop verification ACK: %s", logger.Fields{
			loggerfield.TaskARN: task.Arn,
		})
		// Only task ARN and desired status are required to signal to task engine that the task
		// should be stopped via call to task engine `UpsertTask`.
		taskWithDesiredStatusStopped := createTaskWithARNAndDesiredStatus(task.Arn, apitaskstatus.TaskStopped)
		ts.taskEngine.UpsertTask(taskWithDesiredStatusStopped)
		if err := ts.dataClient.SaveTask(task); err != nil {
			logger.Error("Failed to save data for task", logger.Fields{
				loggerfield.TaskARN: task.Arn,
				loggerfield.Error:   err,
			})
		}
	} else {
		logger.Debug("Task from task stop verification ACK not found on the instance", logger.Fields{
			loggerfield.TaskARN: taskARN,
		})
	}
}

// createTaskWithARNAndDesiredStatus creates a task with ARN `taskARN` and desired status `desiredStatus`.
func createTaskWithARNAndDesiredStatus(taskARN string, desiredStatus apitaskstatus.TaskStatus) *apitask.Task {
	resultTask := apitask.Task{
		Arn: taskARN,
	}
	resultTask.SetDesiredStatus(desiredStatus)
	return &resultTask
}
