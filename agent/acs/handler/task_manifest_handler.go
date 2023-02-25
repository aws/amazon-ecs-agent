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
	"strconv"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
)

// taskManifestHandler handles task manifest message for the ACS client
type taskManifestHandler struct {
	messageBufferTaskManifest                chan *ecsacs.TaskManifestMessage
	messageBufferTaskManifestAck             chan string
	messageBufferTaskStopVerificationMessage chan *ecsacs.TaskStopVerificationMessage
	messageBufferTaskStopVerificationAck     chan *ecsacs.TaskStopVerificationAck
	ctx                                      context.Context
	taskEngine                               engine.TaskEngine
	cancel                                   context.CancelFunc
	dataClient                               data.Client
	cluster                                  string
	containerInstanceArn                     string
	acsClient                                wsclient.ClientServer
	latestSeqNumberTaskManifest              *int64
	messageId                                string
	lock                                     sync.RWMutex
}

// newTaskManifestHandler returns an instance of the taskManifestHandler struct
func newTaskManifestHandler(ctx context.Context,
	cluster string, containerInstanceArn string, acsClient wsclient.ClientServer,
	dataClient data.Client, taskEngine engine.TaskEngine, latestSeqNumberTaskManifest *int64) taskManifestHandler {

	// Create a cancelable context from the parent context
	derivedContext, cancel := context.WithCancel(ctx)
	return taskManifestHandler{
		messageBufferTaskManifest:                make(chan *ecsacs.TaskManifestMessage),
		messageBufferTaskManifestAck:             make(chan string),
		messageBufferTaskStopVerificationMessage: make(chan *ecsacs.TaskStopVerificationMessage),
		messageBufferTaskStopVerificationAck:     make(chan *ecsacs.TaskStopVerificationAck),
		ctx:                                      derivedContext,
		cancel:                                   cancel,
		cluster:                                  cluster,
		containerInstanceArn:                     containerInstanceArn,
		acsClient:                                acsClient,
		taskEngine:                               taskEngine,
		dataClient:                               dataClient,
		latestSeqNumberTaskManifest:              latestSeqNumberTaskManifest,
	}
}

func (taskManifestHandler *taskManifestHandler) handlerFuncTaskManifestMessage() func(
	message *ecsacs.TaskManifestMessage) {
	return func(message *ecsacs.TaskManifestMessage) {
		taskManifestHandler.messageBufferTaskManifest <- message
	}
}

func (taskManifestHandler *taskManifestHandler) handlerFuncTaskStopVerificationMessage() func(
	message *ecsacs.TaskStopVerificationAck) {
	return func(message *ecsacs.TaskStopVerificationAck) {
		taskManifestHandler.messageBufferTaskStopVerificationAck <- message
	}
}

func (taskManifestHandler *taskManifestHandler) start() {
	// Task manifest and it's ack
	go taskManifestHandler.handleTaskManifestMessage()
	go taskManifestHandler.sendTaskManifestMessageAck()

	// Task stop verification message and it's ack
	go taskManifestHandler.sendTaskStopVerificationMessage()
	go taskManifestHandler.handleTaskStopVerificationAck()

}

func (taskManifestHandler *taskManifestHandler) getMessageId() string {
	taskManifestHandler.lock.RLock()
	defer taskManifestHandler.lock.RUnlock()
	return taskManifestHandler.messageId
}

func (taskManifestHandler *taskManifestHandler) setMessageId(messageId string) {
	taskManifestHandler.lock.Lock()
	defer taskManifestHandler.lock.Unlock()
	taskManifestHandler.messageId = messageId
}

func (taskManifestHandler *taskManifestHandler) sendTaskManifestMessageAck() {
	for {
		select {
		case messageBufferTaskManifestAck := <-taskManifestHandler.messageBufferTaskManifestAck:
			taskManifestHandler.ackTaskManifestMessage(messageBufferTaskManifestAck)
		case <-taskManifestHandler.ctx.Done():
			return
		}
	}
}

// sendPendingTaskManifestMessageAck sends all pending task manifest acks to ACS before closing the connection
func (taskManifestHandler *taskManifestHandler) sendPendingTaskManifestMessageAck() {
	for {
		select {
		case messageBufferTaskManifestAck := <-taskManifestHandler.messageBufferTaskManifestAck:
			taskManifestHandler.ackTaskManifestMessage(messageBufferTaskManifestAck)
		default:
			return
		}
	}
}

func (taskManifestHandler *taskManifestHandler) handleTaskStopVerificationAck() {
	for {
		select {
		case messageBufferTaskStopVerificationAck := <-taskManifestHandler.messageBufferTaskStopVerificationAck:
			if err := taskManifestHandler.handleSingleMessageVerificationAck(messageBufferTaskStopVerificationAck); err != nil {
				seelog.Warnf("Error handling Verification ack with messageID: %s, error: %v",
					messageBufferTaskStopVerificationAck.MessageId, err)
			}
		case <-taskManifestHandler.ctx.Done():
			return
		}
	}
}

// handlePendingTaskStopVerificationAck sends pending task stop verification acks to ACS before closing the connection
func (taskManifestHandler *taskManifestHandler) handlePendingTaskStopVerificationAck() {
	for {
		select {
		case messageBufferTaskStopVerificationAck := <-taskManifestHandler.messageBufferTaskStopVerificationAck:
			if err := taskManifestHandler.handleSingleMessageVerificationAck(messageBufferTaskStopVerificationAck); err != nil {
				seelog.Warnf("Error handling Verification ack with messageID: %s, error: %v",
					messageBufferTaskStopVerificationAck.MessageId, err)
			}
		default:
			return
		}
	}
}

func (taskManifestHandler *taskManifestHandler) clearAcks() {
	for {
		select {
		case <-taskManifestHandler.messageBufferTaskManifestAck:
		case <-taskManifestHandler.messageBufferTaskStopVerificationAck:
		default:
			return
		}
	}
}

func (taskManifestHandler *taskManifestHandler) ackTaskManifestMessage(messageID string) {
	seelog.Debugf("Acking task manifest message id: %s", messageID)
	err := taskManifestHandler.acsClient.MakeRequest(&ecsacs.AckRequest{
		Cluster:           aws.String(taskManifestHandler.cluster),
		ContainerInstance: aws.String(taskManifestHandler.containerInstanceArn),
		MessageId:         aws.String(messageID),
	})
	if err != nil {
		seelog.Warnf("Error 'ack'ing TaskManifestMessage with messageID: %s, error: %v", messageID, err)
	}
}

// stop is used to invoke a cancellation function
func (taskManifestHandler *taskManifestHandler) stop() {
	taskManifestHandler.cancel()
}

func (taskManifestHandler *taskManifestHandler) handleTaskManifestMessage() {
	for {
		select {
		case <-taskManifestHandler.ctx.Done():
			return
		case message := <-taskManifestHandler.messageBufferTaskManifest:
			if err := taskManifestHandler.handleTaskManifestSingleMessage(message); err != nil {
				seelog.Warnf("Unable to handle taskManifest message [%s]: %v", message.String(), err)
			}
		}
	}
}

func (taskManifestHandler *taskManifestHandler) sendTaskStopVerificationMessage() {
	for {
		select {
		case message := <-taskManifestHandler.messageBufferTaskStopVerificationMessage:
			if err := taskManifestHandler.acsClient.MakeRequest(message); err != nil {
				seelog.Warnf("Unable to send taskStopVerification message [%s]: %v", message.String(), err)
			}
		case <-taskManifestHandler.ctx.Done():
			return
		}
	}
}

// compares the list of tasks received in the task manifest message and tasks running on the the instance
// It returns all the task that are running on the instance but not present in task manifest message task list
func compareTasks(receivedTaskList []*ecsacs.TaskIdentifier, runningTaskList []*apitask.Task, clusterARN string) []*ecsacs.TaskIdentifier {
	tasksToBeKilled := make([]*ecsacs.TaskIdentifier, 0)
	for _, runningTask := range runningTaskList {
		// For every task running on the instance check if the task is present in receivedTaskList with the DesiredState
		// of running, if not add them to the list of task that needs to be stopped
		if runningTask.GetDesiredStatus() == apitaskstatus.TaskRunning {
			taskPresent := false
			for _, receivedTask := range receivedTaskList {
				if *receivedTask.TaskArn == runningTask.Arn && *receivedTask.
					DesiredStatus == apitaskstatus.TaskRunningString {
					// Task present, does not need to be stopped
					taskPresent = true
					break
				}
			}
			if !taskPresent {
				tasksToBeKilled = append(tasksToBeKilled, &ecsacs.TaskIdentifier{
					DesiredStatus:  aws.String(apitaskstatus.TaskStoppedString),
					TaskArn:        aws.String(runningTask.Arn),
					TaskClusterArn: aws.String(clusterARN),
				})
			}
		}
	}
	return tasksToBeKilled
}

func (taskManifestHandler *taskManifestHandler) handleSingleMessageVerificationAck(
	message *ecsacs.TaskStopVerificationAck) error {
	// Ensure that we have received a corresponding task manifest message before
	taskManifestMessageId := taskManifestHandler.getMessageId()
	if taskManifestMessageId != "" && *message.MessageId == taskManifestMessageId {
		// Reset the message id so that the message with same message id is not processed twice
		taskManifestHandler.setMessageId("")
		for _, taskToKill := range message.StopTasks {
			if *taskToKill.DesiredStatus == apitaskstatus.TaskStoppedString {
				task, isPresent := taskManifestHandler.taskEngine.GetTaskByArn(*taskToKill.TaskArn)
				if isPresent {
					seelog.Infof("Stopping task from task manifest handler: %s", task.Arn)
					task.SetDesiredStatus(apitaskstatus.TaskStopped)
					taskManifestHandler.taskEngine.AddTask(task)
				} else {
					seelog.Debugf("Task not found on the instance: %s", *taskToKill.TaskArn)
				}
			}
		}
	}
	return nil
}

func (taskManifestHandler *taskManifestHandler) handleTaskManifestSingleMessage(
	message *ecsacs.TaskManifestMessage) error {
	taskListManifestHandler := message.Tasks
	seqNumberFromMessage := *message.Timeline
	clusterARN := *message.ClusterArn
	agentLatestSequenceNumber := *taskManifestHandler.latestSeqNumberTaskManifest

	// Check if the sequence number of message received is more than the one stored in Agent
	if agentLatestSequenceNumber < seqNumberFromMessage {
		runningTasksOnInstance, err := taskManifestHandler.taskEngine.ListTasks()
		if err != nil {
			return err
		}
		*taskManifestHandler.latestSeqNumberTaskManifest = *message.Timeline
		// Save the new sequence number to disk.
		err = taskManifestHandler.dataClient.SaveMetadata(data.TaskManifestSeqNumKey, strconv.FormatInt(*message.Timeline, 10))
		if err != nil {
			return err
		}

		tasksToKill := compareTasks(taskListManifestHandler, runningTasksOnInstance, clusterARN)

		// Update messageId so that it can be compared to the messageId in TaskStopVerificationAck message
		taskManifestHandler.setMessageId(*message.MessageId)

		// Throw the task manifest ack and task verification message in async so that it does not block the current
		// thread.
		go func() {
			taskManifestHandler.messageBufferTaskManifestAck <- *message.MessageId
			if len(tasksToKill) > 0 {
				taskStopVerificationMessage := ecsacs.TaskStopVerificationMessage{
					MessageId:      message.MessageId,
					StopCandidates: tasksToKill,
				}

				taskManifestHandler.messageBufferTaskStopVerificationMessage <- &taskStopVerificationMessage
			}
		}()
	} else {
		seelog.Debugf("Skipping the task manifest message. sequence number from task manifest: %d. sequence number "+
			" from Agent: %d", seqNumberFromMessage, agentLatestSequenceNumber)
	}

	return nil
}
