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

package eventhandler

import (
	"container/list"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/cihub/seelog"
)

// a state change that may have a container and, optionally, a task event to
// send
type sendableEvent struct {
	// Either is a contaienr event or a task event
	isContainerEvent bool

	containerSent   bool
	containerChange api.ContainerStateChange

	taskSent   bool
	taskChange api.TaskStateChange

	lock sync.RWMutex
}

func newSendableTaskEvent(event api.TaskStateChange) *sendableEvent {
	return &sendableEvent{
		isContainerEvent: false,
		taskSent:         false,
		taskChange:       event,
	}
}

func (event *sendableEvent) taskArn() string {
	if event.isContainerEvent {
		return event.containerChange.TaskArn
	}
	return event.taskChange.TaskARN
}

// taskShouldBeSent checks whether the event should be sent, this includes
// both task state change and container/managed agent state change events
func (event *sendableEvent) taskShouldBeSent() bool {
	event.lock.RLock()
	defer event.lock.RUnlock()

	if event.isContainerEvent {
		return false
	}
	tevent := event.taskChange
	if event.taskSent {
		return false // redundant event
	}

	// task and container change event should have task != nil
	if tevent.Task == nil {
		return false
	}

	//  internal task state change does not need to be sent
	if tevent.Task.IsInternal {
		return false
	}

	// Task event should be sent
	if tevent.Task.GetSentStatus() < tevent.Status {
		return true
	}

	// Container event should be sent
	for _, containerStateChange := range tevent.Containers {
		container := containerStateChange.Container
		if container.GetSentStatus() < container.GetKnownStatus() {
			return true
		}
	}

	// Managed agent event should be sent
	for _, managedAgentStateChange := range tevent.ManagedAgents {
		managedAgentName := managedAgentStateChange.Name
		container := managedAgentStateChange.Container
		if container.GetManagedAgentSentStatus(managedAgentName) != container.GetManagedAgentStatus(managedAgentName) {
			return true
		}
	}
	return false
}

func (event *sendableEvent) taskAttachmentShouldBeSent() bool {
	event.lock.RLock()
	defer event.lock.RUnlock()
	if event.isContainerEvent {
		return false
	}
	tevent := event.taskChange
	return tevent.Status == apitaskstatus.TaskStatusNone && // Task Status is not set for attachments as task record has yet to be streamed down
		tevent.Attachment != nil && // Task has attachment records
		!tevent.Attachment.HasExpired() && // ENI attachment ack timestamp hasn't expired
		!tevent.Attachment.IsSent() // Task status hasn't already been sent
}

func (event *sendableEvent) containerShouldBeSent() bool {
	event.lock.RLock()
	defer event.lock.RUnlock()
	if !event.isContainerEvent {
		return false
	}
	cevent := event.containerChange
	if event.containerSent || (cevent.Container != nil && cevent.Container.GetSentStatus() >= cevent.Status) {
		return false
	}
	return true
}

func (event *sendableEvent) setSent() {
	event.lock.Lock()
	defer event.lock.Unlock()
	if event.isContainerEvent {
		event.containerSent = true
	} else {
		event.taskSent = true
	}
}

// send tries to send an event, specified by 'eventToSubmit', of type
// 'eventType' to ECS
func (event *sendableEvent) send(
	sendStatusToECS sendStatusChangeToECS,
	setChangeSent setStatusSent,
	eventType string,
	client ecs.ECSClient,
	eventToSubmit *list.Element,
	dataClient data.Client,
	backoff retry.Backoff,
	taskEvents *taskSendableEvents) error {

	fields := event.toFields()
	logger.Info("Sending state change to ECS", fields)
	// Try submitting the change to ECS
	if err := sendStatusToECS(client, event); err != nil {
		fields[field.Error] = err
		logger.Error("Unretriable error sending state change to ECS", fields)
		return err
	}
	// submitted; ensure we don't retry it
	event.setSent()
	// Mark event as sent
	setChangeSent(event, dataClient)
	logger.Debug("Submitted state change to ECS", fields)
	taskEvents.events.Remove(eventToSubmit)
	backoff.Reset()
	return nil
}

// sendStatusChangeToECS defines a function type for invoking the appropriate ECS state change API
type sendStatusChangeToECS func(client ecs.ECSClient, event *sendableEvent) error

// sendContainerStatusToECS invokes the SubmitContainerStateChange API to send a
// container status change to ECS
func sendContainerStatusToECS(client ecs.ECSClient, event *sendableEvent) error {
	containerStateChange, err := event.containerChange.ToECSAgent()
	if err != nil {
		return err
	}

	// containerStateChange and err both nil in the case of an unsupported upstream container state.
	// No-op (i.e., don't submit container state change) in this case.
	if containerStateChange == nil {
		return nil
	}
	return client.SubmitContainerStateChange(*containerStateChange)
}

// sendTaskStatusToECS invokes the SubmitTaskStateChange API to send a task
// status change to ECS
func sendTaskStatusToECS(client ecs.ECSClient, event *sendableEvent) error {
	taskStateChange, err := event.taskChange.ToECSAgent()
	if err != nil {
		return err
	}
	return client.SubmitTaskStateChange(*taskStateChange)
}

// setStatusSent defines a function type to mark the event as sent
type setStatusSent func(event *sendableEvent, dataClient data.Client)

// setContainerChangeSent sets the event's container change object as sent
func setContainerChangeSent(event *sendableEvent, dataClient data.Client) {
	containerChangeStatus := event.containerChange.Status
	container := event.containerChange.Container
	if container != nil && container.GetSentStatus() < containerChangeStatus {
		updateContainerSentStatus(container, containerChangeStatus, dataClient)
	}
}

// setTaskChangeSent sets the event's task change object as sent
func setTaskChangeSent(event *sendableEvent, dataClient data.Client) {
	taskChangeStatus := event.taskChange.Status
	task := event.taskChange.Task
	if task != nil && task.GetSentStatus() < taskChangeStatus {
		updataTaskSentStatus(task, taskChangeStatus, dataClient)
	}
	for _, containerStateChange := range event.taskChange.Containers {
		updateContainerSentStatus(containerStateChange.Container, containerStateChange.Status, dataClient)
	}
	for _, managedAgentStateChange := range event.taskChange.ManagedAgents {
		updateManagedAgentSentStatus(managedAgentStateChange.Container, managedAgentStateChange.Name, managedAgentStateChange.Status, dataClient)
	}

}

// setTaskAttachmentSent sets the event's task attachment object as sent
func setTaskAttachmentSent(event *sendableEvent, dataClient data.Client) {
	if event.taskChange.Attachment != nil {
		attachment := event.taskChange.Attachment
		attachment.SetSentStatus()
		attachment.StopAckTimer()
		err := dataClient.SaveENIAttachment(attachment)
		if err != nil {
			seelog.Errorf("Failed to update attachment sent status in database for attachment %s: %v", attachment.AttachmentARN, err)
		}
	}
}

func (event *sendableEvent) toFields() logger.Fields {
	event.lock.RLock()
	defer event.lock.RUnlock()

	if event.isContainerEvent {
		return event.containerChange.ToFields()
	} else {
		return event.taskChange.ToFields()
	}
}

func updataTaskSentStatus(task *apitask.Task, status apitaskstatus.TaskStatus, dataClient data.Client) {
	task.SetSentStatus(status)
	err := dataClient.SaveTask(task)
	if err != nil {
		seelog.Errorf("Failed to update task sent status in database for task %s: %v", task.Arn, err)
	}
}

func updateContainerSentStatus(container *apicontainer.Container, status apicontainerstatus.ContainerStatus, dataClient data.Client) {
	if container.GetSentStatus() < status {
		container.SetSentStatus(status)
		if err := dataClient.SaveContainer(container); err != nil {
			seelog.Errorf("Failed to update container sent status in database for container %s: %v", container.Name, err)
		}
	}
}

func updateManagedAgentSentStatus(container *apicontainer.Container, managedAgentName string, status apicontainerstatus.ManagedAgentStatus, dataClient data.Client) {
	if container.GetManagedAgentSentStatus(managedAgentName) != status {
		container.UpdateManagedAgentSentStatus(managedAgentName, status)
		if err := dataClient.SaveContainer(container); err != nil {
			seelog.Errorf("Failed to update %s managed agent sent status in database for container %s: %v", managedAgentName, container.Name, err)
		}
	}
}
