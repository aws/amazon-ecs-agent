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
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/api"
	apiappmesh "github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
)

// payloadRequestHandler represents the payload operation for the ACS client
type payloadRequestHandler struct {
	// messageBuffer is used to process PayloadMessages received from the server
	messageBuffer chan *ecsacs.PayloadMessage
	// ackRequest is used to send acks to the backend
	ackRequest  chan string
	ctx         context.Context
	taskEngine  engine.TaskEngine
	ecsClient   api.ECSClient
	dataClient  data.Client
	taskHandler *eventhandler.TaskHandler
	// cancel is used to stop go routines started by start() method
	cancel                      context.CancelFunc
	cluster                     string
	containerInstanceArn        string
	acsClient                   wsclient.ClientServer
	refreshHandler              refreshCredentialsHandler
	credentialsManager          credentials.Manager
	latestSeqNumberTaskManifest *int64
}

// newPayloadRequestHandler returns a new payloadRequestHandler object
func newPayloadRequestHandler(
	ctx context.Context,
	taskEngine engine.TaskEngine,
	ecsClient api.ECSClient,
	cluster string,
	containerInstanceArn string,
	acsClient wsclient.ClientServer,
	dataClient data.Client,
	refreshHandler refreshCredentialsHandler,
	credentialsManager credentials.Manager,
	taskHandler *eventhandler.TaskHandler, seqNumTaskManifest *int64) payloadRequestHandler {
	// Create a cancelable context from the parent context
	derivedContext, cancel := context.WithCancel(ctx)
	return payloadRequestHandler{
		messageBuffer:               make(chan *ecsacs.PayloadMessage, payloadMessageBufferSize),
		ackRequest:                  make(chan string, payloadMessageBufferSize),
		taskEngine:                  taskEngine,
		ecsClient:                   ecsClient,
		dataClient:                  dataClient,
		taskHandler:                 taskHandler,
		ctx:                         derivedContext,
		cancel:                      cancel,
		cluster:                     cluster,
		containerInstanceArn:        containerInstanceArn,
		acsClient:                   acsClient,
		refreshHandler:              refreshHandler,
		credentialsManager:          credentialsManager,
		latestSeqNumberTaskManifest: seqNumTaskManifest,
	}
}

// handlerFunc returns the request handler function for the ecsacs.PayloadMessage type
func (payloadHandler *payloadRequestHandler) handlerFunc() func(payload *ecsacs.PayloadMessage) {
	// return a function that just enqueues PayloadMessages into the message buffer
	return func(payload *ecsacs.PayloadMessage) {
		payloadHandler.messageBuffer <- payload
	}
}

// start invokes go routines to:
// 1. handle messages in the payload message buffer
// 2. handle ack requests to be sent to ACS
func (payloadHandler *payloadRequestHandler) start() {
	go payloadHandler.handleMessages()
	go payloadHandler.sendAcks()
}

// stop cancels the context being used by the payload handler. This is used
// to stop the go routines started by 'start()'
func (payloadHandler *payloadRequestHandler) stop() {
	payloadHandler.cancel()
}

// sendAcks sends ack requests to ACS
func (payloadHandler *payloadRequestHandler) sendAcks() {
	for {
		select {
		case mid := <-payloadHandler.ackRequest:
			payloadHandler.ackMessageId(mid)
		case <-payloadHandler.ctx.Done():
			return
		}
	}
}

// sendPendingAcks sends ack requests to ACS before closing the connection
func (payloadHandler *payloadRequestHandler) sendPendingAcks() {
	for {
		select {
		case mid := <-payloadHandler.ackRequest:
			payloadHandler.ackMessageId(mid)
		default:
			return
		}
	}
}

// ackMessageId sends an AckRequest for a message id
func (payloadHandler *payloadRequestHandler) ackMessageId(messageID string) {
	seelog.Debugf("Acking payload message id: %s", messageID)
	err := payloadHandler.acsClient.MakeRequest(&ecsacs.AckRequest{
		Cluster:           aws.String(payloadHandler.cluster),
		ContainerInstance: aws.String(payloadHandler.containerInstanceArn),
		MessageId:         aws.String(messageID),
	})
	if err != nil {
		logger.Warn("Error ack'ing request", logger.Fields{
			"messageID": messageID,
			field.Error: err,
		})
	}
}

// handleMessages processes payload messages in the payload message buffer in-order
func (payloadHandler *payloadRequestHandler) handleMessages() {
	for {
		select {
		case payload := <-payloadHandler.messageBuffer:
			payloadHandler.handleSingleMessage(payload)
		case <-payloadHandler.ctx.Done():
			return
		}
	}
}

// handleSingleMessage processes a single payload message. It adds tasks in the message to the task engine
// An error is returned if the message was not handled correctly. The error is being used only for testing
// today. In the future, it could be used for doing more interesting things.
func (payloadHandler *payloadRequestHandler) handleSingleMessage(payload *ecsacs.PayloadMessage) error {
	if aws.StringValue(payload.MessageId) == "" {
		seelog.Criticalf("Received a payload with no message id")
		return fmt.Errorf("received a payload with no message id")
	}
	seelog.Debugf("Received payload message, message id: %s", aws.StringValue(payload.MessageId))
	credentialsAcks, allTasksHandled := payloadHandler.addPayloadTasks(payload)

	// Update latestSeqNumberTaskManifest for it to get updated in state file
	if payloadHandler.latestSeqNumberTaskManifest != nil && payload.SeqNum != nil &&
		*payloadHandler.latestSeqNumberTaskManifest < *payload.SeqNum {

		*payloadHandler.latestSeqNumberTaskManifest = *payload.SeqNum
	}

	if !allTasksHandled {
		return fmt.Errorf("did not handle all tasks")
	}

	go func() {
		// Throw the ack in async; it doesn't really matter all that much and this is blocking handling more tasks.
		for _, credentialsAck := range credentialsAcks {
			payloadHandler.refreshHandler.ackMessage(credentialsAck)
		}
		payloadHandler.ackRequest <- *payload.MessageId
	}()

	return nil
}

// addPayloadTasks does validation on each task and, for all valid ones, adds
// it to the task engine. It returns a bool indicating if it could add every
// task to the taskEngine and a slice of credential ack requests
func (payloadHandler *payloadRequestHandler) addPayloadTasks(payload *ecsacs.PayloadMessage) ([]*ecsacs.IAMRoleCredentialsAckRequest, bool) {
	// verify that we were able to work with all tasks in this payload so we know whether to ack the whole thing or not
	allTasksOK := true

	validTasks := make([]*apitask.Task, 0, len(payload.Tasks))
	for _, task := range payload.Tasks {
		if task == nil {
			seelog.Criticalf("Received nil task for messageId: %s", aws.StringValue(payload.MessageId))
			allTasksOK = false
			continue
		}
		apiTask, err := apitask.TaskFromACS(task, payload)
		if err != nil {
			payloadHandler.handleUnrecognizedTask(task, err, payload)
			allTasksOK = false
			continue
		}

		logger.Info("Received task payload from ACS", logger.Fields{
			field.TaskARN:       apiTask.Arn,
			"version":           apiTask.Version,
			field.DesiredStatus: apiTask.GetDesiredStatus(),
		})

		if task.RoleCredentials != nil {
			// The payload from ACS for the task has credentials for the
			// task. Add those to the credentials manager and set the
			// credentials id for the task as well
			taskIAMRoleCredentials := credentials.IAMRoleCredentialsFromACS(task.RoleCredentials, credentials.ApplicationRoleType)
			err = payloadHandler.credentialsManager.SetTaskCredentials(
				&(credentials.TaskIAMRoleCredentials{
					ARN:                aws.StringValue(task.Arn),
					IAMRoleCredentials: taskIAMRoleCredentials,
				}))
			if err != nil {
				payloadHandler.handleUnrecognizedTask(task, err, payload)
				allTasksOK = false
				continue
			}
			apiTask.SetCredentialsID(taskIAMRoleCredentials.CredentialsID)
		}

		// Add ENI information to the task struct.
		for _, acsENI := range task.ElasticNetworkInterfaces {
			eni, err := apieni.ENIFromACS(acsENI)
			if err != nil {
				payloadHandler.handleUnrecognizedTask(task, err, payload)
				allTasksOK = false
				continue
			}
			apiTask.AddTaskENI(eni)
		}

		// Add the app mesh information to task struct
		if task.ProxyConfiguration != nil {
			appmesh, err := apiappmesh.AppMeshFromACS(task.ProxyConfiguration)
			if err != nil {
				payloadHandler.handleUnrecognizedTask(task, err, payload)
				allTasksOK = false
				continue
			}
			apiTask.SetAppMesh(appmesh)
		}

		if task.ExecutionRoleCredentials != nil {
			// The payload message contains execution credentials for the task.
			// Add the credentials to the credentials manager and set the
			// task executionCredentials id.
			taskExecutionIAMRoleCredentials := credentials.IAMRoleCredentialsFromACS(task.ExecutionRoleCredentials, credentials.ExecutionRoleType)
			err = payloadHandler.credentialsManager.SetTaskCredentials(
				&(credentials.TaskIAMRoleCredentials{
					ARN:                aws.StringValue(task.Arn),
					IAMRoleCredentials: taskExecutionIAMRoleCredentials,
				}))
			if err != nil {
				payloadHandler.handleUnrecognizedTask(task, err, payload)
				allTasksOK = false
				continue
			}
			apiTask.SetExecutionRoleCredentialsID(taskExecutionIAMRoleCredentials.CredentialsID)
		}

		validTasks = append(validTasks, apiTask)
	}

	// Add 'stop' transitions first to allow seqnum ordering to work out
	// Because a 'start' sequence number should only be proceeded if all 'stop's
	// of the same sequence number have completed, the 'start' events need to be
	// added after the 'stop' events are there to block them.
	stoppedTasksCredentialsAcks, stoppedTasksAddedOK := payloadHandler.addTasks(payload, validTasks, isTaskStatusNotStopped)
	newTasksCredentialsAcks, newTasksAddedOK := payloadHandler.addTasks(payload, validTasks, isTaskStatusStopped)
	if !stoppedTasksAddedOK || !newTasksAddedOK {
		allTasksOK = false
	}

	// Construct a slice with credentials acks from all tasks
	credentialsAcks := append(stoppedTasksCredentialsAcks, newTasksCredentialsAcks...)
	return credentialsAcks, allTasksOK
}

// addTasks adds the tasks to the task engine based on the skipAddTask condition
// This is used to add non-stopped tasks before adding stopped tasks
func (payloadHandler *payloadRequestHandler) addTasks(payload *ecsacs.PayloadMessage, tasks []*apitask.Task, skipAddTask skipAddTaskComparatorFunc) ([]*ecsacs.IAMRoleCredentialsAckRequest, bool) {
	allTasksOK := true
	var credentialsAcks []*ecsacs.IAMRoleCredentialsAckRequest
	for _, task := range tasks {
		if skipAddTask(task.GetDesiredStatus()) {
			continue
		}
		payloadHandler.taskEngine.AddTask(task)
		// Only need to save task to DB when its desired status is RUNNING (i.e. this is a new task that we are going
		// to manage). When its desired status is STOPPED, the task is already in the DB and the desired status change
		// will be saved by task manager.
		if task.GetDesiredStatus() == apitaskstatus.TaskRunning {
			err := payloadHandler.dataClient.SaveTask(task)
			if err != nil {
				seelog.Errorf("Failed to save data for task %s: %v", task.Arn, err)
				allTasksOK = false
			}
		}

		ackCredentials := func(id string, description string) {
			ack, err := payloadHandler.ackCredentials(payload.MessageId, id)
			if err != nil {
				allTasksOK = false
				seelog.Errorf("Failed to acknowledge %s credentials for task: %s, err: %v", description, task.String(), err)
				return
			}
			credentialsAcks = append(credentialsAcks, ack)
		}

		// Generate an ack request for the credentials in the task, if the
		// task is associated with an IAM role or the execution role
		taskCredentialsID := task.GetCredentialsID()
		if taskCredentialsID != "" {
			ackCredentials(taskCredentialsID, "task iam role")
		}

		taskExecutionCredentialsID := task.GetExecutionCredentialsID()
		if taskExecutionCredentialsID != "" {
			ackCredentials(taskExecutionCredentialsID, "task execution role")
		}
	}
	return credentialsAcks, allTasksOK
}

func (payloadHandler *payloadRequestHandler) ackCredentials(messageID *string, credentialsID string) (*ecsacs.IAMRoleCredentialsAckRequest, error) {
	creds, ok := payloadHandler.credentialsManager.GetTaskCredentials(credentialsID)
	if !ok {
		return nil, fmt.Errorf("credentials could not be retrieved")
	} else {
		return &ecsacs.IAMRoleCredentialsAckRequest{
			MessageId:     messageID,
			Expiration:    aws.String(creds.IAMRoleCredentials.Expiration),
			CredentialsId: aws.String(creds.IAMRoleCredentials.CredentialsID),
		}, nil
	}
}

// skipAddTaskComparatorFunc defines the function pointer that accepts task status
// and returns the boolean comparison result
type skipAddTaskComparatorFunc func(apitaskstatus.TaskStatus) bool

// isTaskStatusStopped returns true if the task status == STOPPED
func isTaskStatusStopped(status apitaskstatus.TaskStatus) bool {
	return status == apitaskstatus.TaskStopped
}

// isTaskStatusNotStopped returns true if the task status != STOPPED
func isTaskStatusNotStopped(status apitaskstatus.TaskStatus) bool {
	return status != apitaskstatus.TaskStopped
}

// handleUnrecognizedTask handles unrecognized tasks by sending 'stopped' with
// a suitable reason to the backend
func (payloadHandler *payloadRequestHandler) handleUnrecognizedTask(task *ecsacs.Task, err error, payload *ecsacs.PayloadMessage) {
	seelog.Warnf("Received unexpected acs message, messageID: %s, task: %v, err: %v",
		aws.StringValue(payload.MessageId), aws.StringValue(task.Arn), err)

	if aws.StringValue(task.Arn) == "" {
		seelog.Criticalf("Received task with no arn, messageId: %s", aws.StringValue(payload.MessageId))
		return
	}

	// Only need to stop the task; it brings down the containers too.
	taskEvent := api.TaskStateChange{
		TaskARN: *task.Arn,
		Status:  apitaskstatus.TaskStopped,
		Reason:  UnrecognizedTaskError{err}.Error(),
		// The real task cannot be extracted from payload message, so we send an empty task.
		// This is necessary because the task handler will not send an event whose
		// Task is nil.
		Task: &apitask.Task{},
	}

	payloadHandler.taskHandler.AddStateChangeEvent(taskEvent, payloadHandler.ecsClient)
}

// clearAcks drains the ack request channel
func (payloadHandler *payloadRequestHandler) clearAcks() {
	for {
		select {
		case <-payloadHandler.ackRequest:
		default:
			return
		}
	}
}
