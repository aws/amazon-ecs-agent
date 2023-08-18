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
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

type TaskStopper interface {
	StopTask(taskARN string)
}

const (
	TaskStopVerificationACKMessageName = "TaskStopVerificationACKMessage"
)

// taskStopVerificationACKResponder processes task stop verification ACK messages from ACS.
// It processes the message and sets the desired status of all tasks from the message to STOPPED.
type taskStopVerificationACKResponder struct {
	taskStopper       TaskStopper
	messageIDAccessor ManifestMessageIDAccessor
	metricsFactory    metrics.EntryFactory
}

// NewTaskStopVerificationACKResponder returns an instance of the taskStopVerificationACKResponder struct.
func NewTaskStopVerificationACKResponder(
	taskStopper TaskStopper,
	messageIDAccessor ManifestMessageIDAccessor,
	metricsFactory metrics.EntryFactory) wsclient.RequestResponder {
	r := &taskStopVerificationACKResponder{
		taskStopper:       taskStopper,
		messageIDAccessor: messageIDAccessor,
		metricsFactory:    metricsFactory,
	}
	return r
}

func (*taskStopVerificationACKResponder) Name() string { return "task stop verification ACK responder" }

func (r *taskStopVerificationACKResponder) HandlerFunc() wsclient.RequestHandler {
	return r.handleTaskStopVerificationACK
}

// handleTaskStopVerificationACK goes through the list of verified tasks to be stopped
// and stops each one by setting the desired status of each task to STOPPED.
func (r *taskStopVerificationACKResponder) handleTaskStopVerificationACK(message *ecsacs.TaskStopVerificationAck) {
	logger.Debug(fmt.Sprintf("Handling %s", TaskStopVerificationACKMessageName))

	// Ensure that message is valid and that a corresponding task manifest message has been processed before.
	ackMessageID := aws.StringValue(message.MessageId)
	manifestMessageID := r.messageIDAccessor.GetMessageID()
	if ackMessageID == "" || ackMessageID != manifestMessageID {
		logger.Error(fmt.Sprintf("Error validating %s received from ECS", TaskStopVerificationACKMessageName),
			logger.Fields{
				field.Error: errors.New("Invalid ACK message ID received: " + ackMessageID +
					", mismatch with manifest message ID: " + manifestMessageID),
			})
		return
	}

	// Reset the message id so that the message with same message id is not processed twice.
	r.messageIDAccessor.SetMessageID("")

	// Loop through all tasks in the verified stop list and set the desired status of each one to STOPPED.
	tasksToStop := message.StopTasks
	for _, task := range tasksToStop {
		taskARN := aws.StringValue(task.TaskArn)
		metricFields := logger.Fields{
			field.MessageID: aws.StringValue(message.MessageId),
			field.TaskARN:   taskARN,
		}
		r.metricsFactory.New(metrics.TaskStoppedMetricName).WithFields(metricFields).Done(nil)

		// Send request to the task stopper to stop the task.
		logger.Info("Sending message to task stopper to stop task", logger.Fields{
			field.TaskARN: taskARN,
		})
		r.taskStopper.StopTask(taskARN)
	}
}
