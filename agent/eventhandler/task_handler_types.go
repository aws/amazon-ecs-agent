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
	"fmt"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
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
// both task state change and container state change events
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

	// Task event should be sent
	if tevent.Task.GetSentStatus() < tevent.Status {
		return true
	}

	// Container event should be sent
	for _, containerStateChange := range tevent.Containers {
		container := containerStateChange.Container
		if container.GetSentStatus() < container.GetKnownStatus() {
			// We found a container that needs its state
			// change to be sent to ECS.
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
	client api.ECSClient,
	eventToSubmit *list.Element,
	dataClient data.Client,
	backoff retry.Backoff,
	taskEvents *taskSendableEvents) error {

	seelog.Infof("TaskHandler: Sending %s change: %s", eventType, event.toString())
	// Try submitting the change to ECS
	if err := sendStatusToECS(client, event); err != nil {
		seelog.Errorf("TaskHandler: Unretriable error submitting %s state change [%s]: %v",
			eventType, event.toString(), err)
		return err
	}
	// submitted; ensure we don't retry it
	event.setSent()
	// Mark event as sent
	setChangeSent(event, dataClient)
	seelog.Debugf("TaskHandler: Submitted task state change: %s", event.toString())
	taskEvents.events.Remove(eventToSubmit)
	backoff.Reset()
	return nil
}

// sendStatusChangeToECS defines a function type for invoking the appropriate ECS state change API
type sendStatusChangeToECS func(client api.ECSClient, event *sendableEvent) error

// sendContainerStatusToECS invokes the SubmitContainerStateChange API to send a
// container status change to ECS
func sendContainerStatusToECS(client api.ECSClient, event *sendableEvent) error {
	return client.SubmitContainerStateChange(event.containerChange)
}

// sendTaskStatusToECS invokes the SubmitTaskStateChange API to send a task
// status change to ECS
func sendTaskStatusToECS(client api.ECSClient, event *sendableEvent) error {
	return client.SubmitTaskStateChange(event.taskChange)
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
		container := containerStateChange.Container
		containerChangeStatus := containerStateChange.Status
		if container.GetSentStatus() < containerChangeStatus {
			updateContainerSentStatus(container, containerStateChange.Status, dataClient)
		}
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

func (event *sendableEvent) toString() string {
	event.lock.RLock()
	defer event.lock.RUnlock()

	if event.isContainerEvent {
		return "ContainerChange: [" + event.containerChange.String() + fmt.Sprintf("] sent: %t", event.containerSent)
	} else {
		return "TaskChange: [" + event.taskChange.String() + fmt.Sprintf("] sent: %t", event.taskSent)
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
	container.SetSentStatus(status)
	err := dataClient.SaveContainer(container)
	if err != nil {
		seelog.Errorf("Failed to update container sent status in databse for container %s: %v", container.Name, err)
	}
}
