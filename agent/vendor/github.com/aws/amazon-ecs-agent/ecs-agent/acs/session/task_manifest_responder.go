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
	"sync"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

const (
	TaskManifestMessageName = "TaskManifestMessage"
)

// TaskComparer gets and compares running tasks on an instance to those in the ACS manifest.
// It should be implemented by an underlying struct that has access to such data.
type TaskComparer interface {
	CompareRunningTasksOnInstanceWithManifest(*ecsacs.TaskManifestMessage) ([]*ecsacs.TaskIdentifier, error)
}

// SequenceNumberAccessor is used to get and set state for the current container
// instance. It sets and gets the latest sequence number that corresponds to the most
// recent task manifest that the task manifest responder has processed it alo sets the
// latest message id
type SequenceNumberAccessor interface {
	GetLatestSequenceNumber() int64
	SetLatestSequenceNumber(seqNum int64) error
}

// taskManifestResponder responds to task manifest messages from ACS. It processes the
// message and, if the message isn't stale, creates a list of stop candidates, which are
// tasks that are running on the instance but are tasks that ACS says should not be running.
// The taskManifestResponder will ack the task manifest message and send ACS a
// TaskStopVerificationMessage if there are stop candidates.
type taskManifestResponder struct {
	taskComparer      TaskComparer
	snAccessor        SequenceNumberAccessor
	messageIDAccessor ManifestMessageIDAccessor
	respond           wsclient.RespondFunc
	metricsFactory    metrics.EntryFactory
}

// NewTaskManifestResponder returns an instance of the taskManifestResponder struct.
func NewTaskManifestResponder(taskComparer TaskComparer, snAccessor SequenceNumberAccessor, messageIDAccessor ManifestMessageIDAccessor,
	metricsFactory metrics.EntryFactory, responseSender wsclient.RespondFunc) wsclient.RequestResponder {
	r := &taskManifestResponder{
		taskComparer:      taskComparer,
		snAccessor:        snAccessor,
		messageIDAccessor: messageIDAccessor,
		metricsFactory:    metricsFactory,
	}
	r.respond = responseToACSSender(r.Name(), responseSender)
	return r
}

func (*taskManifestResponder) Name() string { return "task manifest responder" }

func (tmr *taskManifestResponder) HandlerFunc() wsclient.RequestHandler {
	return tmr.handleTaskManifestMessage
}

// handleTaskManifestMessage is the high level caller to handle a task manifest message from ACS.
// It will kick off the call stack of processTaskManifestMessage calling ack and send TaskManifestMessage
func (tmr *taskManifestResponder) handleTaskManifestMessage(message *ecsacs.TaskManifestMessage) {
	messageID := aws.StringValue(message.MessageId)
	logger.Debug(fmt.Sprintf("Processing %s", TaskManifestMessageName), logger.Fields{
		field.MessageID: messageID,
	})
	m := tmr.metricsFactory.New(metrics.TaskManifestHandlingDuration)
	err := tmr.processTaskManifestMessage(message)
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to handle %s", TaskManifestMessageName), logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
	}
	m.Done(err)
}

// processTaskManifestMessage processes a task manifest message from ACS. It verifies
// whether a task manifest message is stale or not. If not, it proceeds to create a
// list of stop candidates.
func (tmr *taskManifestResponder) processTaskManifestMessage(
	message *ecsacs.TaskManifestMessage) error {
	messageID := aws.StringValue(message.MessageId)
	manifestSeqNum := aws.Int64Value(message.Timeline)
	agentLatestSeqNum := tmr.snAccessor.GetLatestSequenceNumber()

	// Verify that task manifest isn't stale.
	// The agent will keep track of the highest sequence number received in both PayloadMessage and TaskManifestMessage.
	// If the sequence number on TaskManifestMessage is lower or equal to to the latest known one, the TaskManifestMessage must be discarded.
	// The manifest should also not be a duplicate of the last one received, so the number cannot be less than or equal to the latest.
	// ACS will guarantee that the sequence number is indeed increasing and valid for this use case.
	if manifestSeqNum <= agentLatestSeqNum {
		logger.Warn(fmt.Sprintf("Discarding task manifest message"+
			"Sequence number from manifest is less than or equal to Agent latest sequence number."), logger.Fields{
			field.MessageID:     messageID,
			"ManifestSeqNum":    manifestSeqNum,
			"AgentLatestSeqNum": agentLatestSeqNum,
		})
		return nil
	}

	// Save the task manifest's messageID and sequence number so that it can be compared
	// by the taskStopVerificationAckResponder.
	err := tmr.messageIDAccessor.SetMessageID(messageID)
	if err != nil {
		return errors.Wrap(err, "failed to update latest manifest messageID")
	}
	err = tmr.snAccessor.SetLatestSequenceNumber(*message.Timeline)
	if err != nil {
		return errors.Wrap(err, "failed to update latest manifest sequence number")
	}

	// Compare running tasks on instance with those in the manifest.
	tasksToStop, err := tmr.taskComparer.CompareRunningTasksOnInstanceWithManifest(message)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	// Acknowledge the task manifest message asynchronously.
	go func() {
		defer wg.Done()
		tmr.ackTaskManifestMessage(message)
	}()

	wg.Add(1)
	// Send the task stop verification message to ACS asynchronously.
	go func() {
		defer wg.Done()
		tmr.sendTaskStopVerification(message, tasksToStop)
	}()
	wg.Wait()
	return nil
}

func (tmr *taskManifestResponder) ackTaskManifestMessage(message *ecsacs.TaskManifestMessage) {
	messageID := aws.StringValue(message.MessageId)
	logger.Debug(fmt.Sprintf("acknowledging %s", TaskManifestMessageName), logger.Fields{field.MessageID: messageID})
	err := tmr.respond(&ecsacs.AckRequest{
		Cluster:           message.ClusterArn,
		ContainerInstance: message.ContainerInstanceArn,
		MessageId:         message.MessageId,
	})
	if err != nil {
		logger.Warn(fmt.Sprintf("Error acknowledging %s", TaskManifestMessageName), logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
	}
}

// If there are stop candidates, send a TaskStopVerificationMessage to ACS and log each stop candidate.
func (tmr *taskManifestResponder) sendTaskStopVerification(message *ecsacs.TaskManifestMessage, tasksToStop []*ecsacs.TaskIdentifier) {
	messageID := aws.StringValue(message.MessageId)
	if len(tasksToStop) == 0 {
		return
	}
	// Create a list of stop candidates to send one debug message.
	var taskARNList []string
	for _, task := range tasksToStop {
		taskARNList = append(taskARNList, aws.StringValue(task.TaskArn))
	}
	logger.Debug(fmt.Sprintf("Sending task stop verification message for %d tasks", len(taskARNList)), logger.Fields{
		field.MessageID: messageID,
		field.TaskARN:   taskARNList,
	})
	err := tmr.respond(&ecsacs.TaskStopVerificationMessage{
		MessageId:      message.MessageId,
		StopCandidates: tasksToStop,
	})
	if err != nil {
		logger.Warn("Error sending task stop verification message", logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
	}
}
