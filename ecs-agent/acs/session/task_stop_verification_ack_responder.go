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
	respond           wsclient.RespondFunc
}

// NewTaskStopVerificationACKResponder returns an instance of the taskStopVerificationACKResponder struct.
func NewTaskStopVerificationACKResponder(
	taskStopper TaskStopper,
	messageIDAccessor ManifestMessageIDAccessor,
	metricsFactory metrics.EntryFactory,
	responseSender wsclient.RespondFunc) *taskStopVerificationACKResponder {
	r := &taskStopVerificationACKResponder{
		taskStopper:       taskStopper,
		messageIDAccessor: messageIDAccessor,
		metricsFactory:    metricsFactory,
	}
	r.respond = ResponseToACSSender(r.Name(), responseSender)
	return r
}

func (*taskStopVerificationACKResponder) Name() string { return "task stop verification ACK responder" }

func (r *taskStopVerificationACKResponder) HandlerFunc() wsclient.RequestHandler {
	return r.handleTaskStopVerificationACK
}

// handleTaskStopVerificationACK goes through the list of verified tasks to be stopped
// and stops each one by setting the desired status of each task to STOPPED.
func (r *taskStopVerificationACKResponder) handleTaskStopVerificationACK(message *ecsacs.TaskStopVerificationAck) error {
	logger.Debug(fmt.Sprintf("Handling %s", TaskStopVerificationACKMessageName))

	// Ensure that message is valid and that a corresponding task manifest message has been processed before.
	ackMessageID := aws.StringValue(message.MessageId)
	manifestMessageID := r.messageIDAccessor.GetMessageID()
	if ackMessageID == "" || ackMessageID != manifestMessageID {
		return errors.New("Invalid messageID received: " + ackMessageID + " Manifest Message ID: " + manifestMessageID)
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
	return nil
}
