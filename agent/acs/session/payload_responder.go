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

	"github.com/aws/amazon-ecs-agent/agent/api"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	apiresource "github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	nlappmesh "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

// skipAddTaskComparatorFunc defines the function pointer that accepts task status
// and returns the boolean comparison result.
type skipAddTaskComparatorFunc func(apitaskstatus.TaskStatus) bool

// payloadMessageHandler implements PayloadMessageHandler interface defined in ecs-agent module.
type payloadMessageHandler struct {
	taskEngine                  engine.TaskEngine
	ecsClient                   ecs.ECSClient
	dataClient                  data.Client
	taskHandler                 *eventhandler.TaskHandler
	credentialsManager          credentials.Manager
	latestSeqNumberTaskManifest *int64
}

// NewPayloadMessageHandler creates a new payloadMessageHandler.
func NewPayloadMessageHandler(taskEngine engine.TaskEngine,
	ecsClient ecs.ECSClient,
	dataClient data.Client,
	taskHandler *eventhandler.TaskHandler,
	credentialsManager credentials.Manager,
	latestSeqNumberTaskManifest *int64) *payloadMessageHandler {
	return &payloadMessageHandler{
		taskEngine:                  taskEngine,
		ecsClient:                   ecsClient,
		dataClient:                  dataClient,
		taskHandler:                 taskHandler,
		credentialsManager:          credentialsManager,
		latestSeqNumberTaskManifest: latestSeqNumberTaskManifest,
	}
}

func (pmHandler *payloadMessageHandler) ProcessMessage(message *ecsacs.PayloadMessage,
	ackFunc func(*ecsacs.AckRequest, []*ecsacs.IAMRoleCredentialsAckRequest)) error {

	credentialsAcks, allTasksHandled := pmHandler.addPayloadTasks(message)

	// Update latestSeqNumberTaskManifest for it to get updated in state file.
	if pmHandler.latestSeqNumberTaskManifest != nil && message.SeqNum != nil &&
		*pmHandler.latestSeqNumberTaskManifest < *message.SeqNum {
		*pmHandler.latestSeqNumberTaskManifest = *message.SeqNum
	}

	if !allTasksHandled {
		return errors.Errorf("did not handle all tasks")
	}

	// Send ACKs - do it in async such that it does not block handling more tasks.
	go ackFunc(&ecsacs.AckRequest{
		Cluster:           message.ClusterArn,
		ContainerInstance: message.ContainerInstanceArn,
		MessageId:         message.MessageId,
	}, credentialsAcks)

	return nil
}

// addPayloadTasks does validation on each task and, for all valid ones, adds
// it to the task engine. It returns a bool indicating if it could add every
// task to the taskEngine and a slice of credential ack requests.
func (pmHandler *payloadMessageHandler) addPayloadTasks(payload *ecsacs.PayloadMessage) (
	[]*ecsacs.IAMRoleCredentialsAckRequest, bool) {
	// Verify that we were able to work with all tasks in this payload.
	// This is so we know whether to ACK the whole thing or not.
	allTasksOK := true

	validTasks := make([]*apitask.Task, 0, len(payload.Tasks))
	for _, task := range payload.Tasks {
		if task == nil {
			logger.Critical("Received nil task for message", logger.Fields{
				loggerfield.MessageID: aws.StringValue(payload.MessageId),
			})
			allTasksOK = false
			continue
		}

		// Note: If we receive an EBS-backed task, we'll also receive an incomplete volume configuration in the list of Volumes
		// To accommodate this, we'll first check if the task IS EBS-backed then we'll mark the corresponding Volume object to be
		// of type "attachment". This volume object will be replaced by the newly created EBS volume configuration when we parse
		// through the task attachments.
		volName, ok := hasEBSAttachment(task)
		if ok {
			initializeAttachmentTypeVolume(task, volName)
		}

		apiTask, err := apitask.TaskFromACS(task, payload)
		if err != nil {
			pmHandler.handleInvalidTask(task, err, payload)
			allTasksOK = false
			continue
		}

		logger.Info("Received task payload from ACS", logger.Fields{
			loggerfield.TaskARN:       apiTask.Arn,
			loggerfield.TaskVersion:   apiTask.Version,
			loggerfield.DesiredStatus: apiTask.GetDesiredStatus(),
		})

		if task.RoleCredentials != nil {
			// The payload from ACS for the task has credentials for the
			// task. Add those to the credentials manager and set the
			// credentials id for the task as well.
			taskIAMRoleCredentials := credentials.IAMRoleCredentialsFromACS(task.RoleCredentials,
				credentials.ApplicationRoleType)
			err = pmHandler.credentialsManager.SetTaskCredentials(
				&(credentials.TaskIAMRoleCredentials{
					ARN:                aws.StringValue(task.Arn),
					IAMRoleCredentials: taskIAMRoleCredentials,
				}))
			if err != nil {
				pmHandler.handleInvalidTask(task, err, payload)
				allTasksOK = false
				continue
			}
			apiTask.SetCredentialsID(taskIAMRoleCredentials.CredentialsID)
		}

		// Add ENI information to the task struct.
		for _, acsENI := range task.ElasticNetworkInterfaces {
			eni, err := ni.InterfaceFromACS(acsENI)
			if err != nil {
				pmHandler.handleInvalidTask(task, err, payload)
				allTasksOK = false
				continue
			}
			apiTask.AddTaskENI(eni)
		}

		// Add the app mesh information to task struct.
		if task.ProxyConfiguration != nil {
			appmesh, err := nlappmesh.AppMeshFromACS(task.ProxyConfiguration)
			if err != nil {
				pmHandler.handleInvalidTask(task, err, payload)
				allTasksOK = false
				continue
			}
			apiTask.SetAppMesh(appmesh)
		}

		if task.ExecutionRoleCredentials != nil {
			// The payload message contains execution credentials for the task.
			// Add the credentials to the credentials manager and set the
			// task executionCredentials id.
			taskExecutionIAMRoleCredentials := credentials.IAMRoleCredentialsFromACS(task.ExecutionRoleCredentials,
				credentials.ExecutionRoleType)
			err = pmHandler.credentialsManager.SetTaskCredentials(
				&(credentials.TaskIAMRoleCredentials{
					ARN:                aws.StringValue(task.Arn),
					IAMRoleCredentials: taskExecutionIAMRoleCredentials,
				}))
			if err != nil {
				pmHandler.handleInvalidTask(task, err, payload)
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
	stoppedTasksCredentialsAcks, stoppedTasksAddedOK := pmHandler.addTasks(payload, validTasks, isTaskStatusNotStopped)
	newTasksCredentialsAcks, newTasksAddedOK := pmHandler.addTasks(payload, validTasks, isTaskStatusStopped)
	if !stoppedTasksAddedOK || !newTasksAddedOK {
		allTasksOK = false
	}

	// Construct a slice with credentials acks from all tasks.
	credentialsAcks := append(stoppedTasksCredentialsAcks, newTasksCredentialsAcks...)
	return credentialsAcks, allTasksOK
}

// handleInvalidTask handles invalid tasks by sending 'stopped' with
// a suitable reason to the backend.
func (pmHandler *payloadMessageHandler) handleInvalidTask(task *ecsacs.Task, err error,
	payload *ecsacs.PayloadMessage) {
	logger.Warn("Received unexpected ACS message", logger.Fields{
		loggerfield.MessageID: aws.StringValue(payload.MessageId),
		loggerfield.TaskARN:   aws.StringValue(task.Arn),
		loggerfield.Error:     err,
	})

	if aws.StringValue(task.Arn) == "" {
		logger.Critical("Received task with no ARN for payload message", logger.Fields{
			loggerfield.MessageID: aws.StringValue(payload.MessageId),
		})
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

	pmHandler.taskHandler.AddStateChangeEvent(taskEvent, pmHandler.ecsClient)
}

// addTasks adds the tasks to the task engine based on the skipAddTask condition.
// This is used to add non-stopped tasks before adding stopped tasks.
func (pmHandler *payloadMessageHandler) addTasks(payload *ecsacs.PayloadMessage, tasks []*apitask.Task,
	skipAddTask skipAddTaskComparatorFunc) ([]*ecsacs.IAMRoleCredentialsAckRequest, bool) {
	allTasksOK := true
	var credentialsAcks []*ecsacs.IAMRoleCredentialsAckRequest
	for _, task := range tasks {
		if skipAddTask(task.GetDesiredStatus()) {
			continue
		}
		pmHandler.taskEngine.AddTask(task)
		// Only need to save task to DB when its desired status is RUNNING (i.e. this is a new task that we are going
		// to manage). When its desired status is STOPPED, the task is already in the DB and the desired status change
		// will be saved by task manager.
		if task.GetDesiredStatus() == apitaskstatus.TaskRunning {
			err := pmHandler.dataClient.SaveTask(task)
			if err != nil {
				logger.Error("Failed to save data for task", logger.Fields{
					loggerfield.TaskARN: task.Arn,
					loggerfield.Error:   err,
				})
				allTasksOK = false
			}
		}

		ackCredentials := func(id string, description string) {
			ack, err := pmHandler.ackCredentials(payload.MessageId, id)
			if err != nil {
				allTasksOK = false
				logger.Error(fmt.Sprintf("Failed to acknowledge %s credentials for task",
					description), logger.Fields{
					"task":            task.String(),
					loggerfield.Error: err,
				})
				return
			}
			credentialsAcks = append(credentialsAcks, ack)
		}

		// Generate an ack request for the credentials in the task, if the
		// task is associated with an IAM role or the execution role.
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

func (pmHandler *payloadMessageHandler) ackCredentials(messageID *string, credentialsID string) (
	*ecsacs.IAMRoleCredentialsAckRequest, error) {
	creds, ok := pmHandler.credentialsManager.GetTaskCredentials(credentialsID)
	if !ok {
		return nil, errors.Errorf("credentials could not be retrieved")

	} else {
		return &ecsacs.IAMRoleCredentialsAckRequest{
			MessageId:     messageID,
			Expiration:    aws.String(creds.IAMRoleCredentials.Expiration),
			CredentialsId: aws.String(creds.IAMRoleCredentials.CredentialsID),
		}, nil
	}
}

// isTaskStatusStopped returns true if the task status == STOPPED.
func isTaskStatusStopped(status apitaskstatus.TaskStatus) bool {
	return status == apitaskstatus.TaskStopped
}

// isTaskStatusNotStopped returns true if the task status != STOPPED.
func isTaskStatusNotStopped(status apitaskstatus.TaskStatus) bool {
	return status != apitaskstatus.TaskStopped
}

func hasEBSAttachment(acsTask *ecsacs.Task) (string, bool) {
	// TODO: This will only work if there's one EBS volume per task. If we there is a case where we have multi-attach for a task, this needs to be modified
	for _, attachment := range acsTask.Attachments {
		if *attachment.AttachmentType == apiresource.EBSTaskAttach {
			for _, property := range attachment.AttachmentProperties {
				if *property.Name == apiresource.VolumeNameKey {
					return *property.Value, true
				}
			}
		}
	}
	return "", false
}

func initializeAttachmentTypeVolume(acsTask *ecsacs.Task, volName string) {
	for _, volume := range acsTask.Volumes {
		if *volume.Name == volName && volume.Type == nil {
			newType := "attachment"
			volume.Type = &newType
		}
	}
}
