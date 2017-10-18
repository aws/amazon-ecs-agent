// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"errors"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

// Maximum number of tasks that may be handled at once by the TaskHandler
const concurrentEventCalls = 3

type eventList struct {
	// events is a list of *sendableEvents
	events *list.List
	// sending will check whether the list is already being handlerd
	sending bool
	//eventsListLock locks both the list and sending bool
	eventListLock sync.Mutex
}

// TaskHandler encapsulates the the map of a task arn to task and container events
// associated with said task
type TaskHandler struct {
	// submitSemaphore for the number of tasks that may be handled at once
	submitSemaphore utils.Semaphore
	// taskToEvents is arn:*eventList map so events may be serialized per task
	//TODO: fix leak, currently items are never removed from this map
	tasksToEvents map[string]*eventList
	// tasksToContainerStates is used to collect container events
	// between task transitions
	tasksToContainerStates map[string][]api.ContainerStateChange

	//  taskHandlerLock is used to safely access the following maps:
	// * taskToEvents
	// * tasksToContainerStates
	taskHandlerLock sync.RWMutex

	// stateSaver is a statemanager which may be used to save any
	// changes to a task or container's SentStatus
	stateSaver statemanager.Saver
}

// NewTaskHandler returns a pointer to TaskHandler
func NewTaskHandler(stateManager statemanager.Saver) *TaskHandler {
	return &TaskHandler{
		tasksToEvents:          make(map[string]*eventList),
		submitSemaphore:        utils.NewSemaphore(concurrentEventCalls),
		tasksToContainerStates: make(map[string][]api.ContainerStateChange),
		stateSaver:             stateManager,
	}
}

// AddStateChangeEvent queues up a state change for sending using the given client.
func (handler *TaskHandler) AddStateChangeEvent(change statechange.Event, client api.ECSClient) error {
	switch change.GetEventType() {
	case statechange.TaskEvent:
		event, ok := change.(api.TaskStateChange)
		if !ok {
			return errors.New("eventhandler: unable to get task event from state change event")
		}
		handler.flushBatch(&event)
		handler.addEvent(newSendableTaskEvent(event), client)
		return nil

	case statechange.ContainerEvent:
		event, ok := change.(api.ContainerStateChange)
		if !ok {
			return errors.New("eventhandler: unable to get container event from state change event")
		}
		handler.batchContainerEvent(event)
		return nil

	default:
		return errors.New("eventhandler: unable to determine event type from state change event")
	}
}

// batchContainerEvent collects container state change events for a given task arn
func (handler *TaskHandler) batchContainerEvent(event api.ContainerStateChange) {
	handler.taskHandlerLock.Lock()
	defer handler.taskHandlerLock.Unlock()

	seelog.Infof("TaskHandler, batching container event: %s", event.String())
	handler.tasksToContainerStates[event.TaskArn] = append(handler.tasksToContainerStates[event.TaskArn], event)
}

// flushBatch attaches the task arn's container events to TaskStateChange event that
// is being submittied to the backend
func (handler *TaskHandler) flushBatch(event *api.TaskStateChange) {
	handler.taskHandlerLock.Lock()
	defer handler.taskHandlerLock.Unlock()

	event.Containers = append(event.Containers, handler.tasksToContainerStates[event.TaskARN]...)
	delete(handler.tasksToContainerStates, event.TaskARN)
}

// addEvent prepares a given event to be sent by adding it to the handler's appropriate
// eventList and remove the entry in tasksToEvents map
func (handler *TaskHandler) addEvent(change *sendableEvent, client api.ECSClient) {
	handler.taskHandlerLock.Lock()
	defer handler.taskHandlerLock.Unlock()
	seelog.Infof("TaskHandler, Adding event: %s", change.String())

	taskEvents := handler.getTaskEventList(change)

	taskEvents.eventListLock.Lock()
	defer taskEvents.eventListLock.Unlock()

	// Update taskEvent
	taskEvents.events.PushBack(change)

	if !taskEvents.sending {
		taskEvents.sending = true
		go handler.SubmitTaskEvents(taskEvents, client)
	}

	delete(handler.tasksToEvents, change.taskArn())
}

// getTaskEventList gets the eventList from taskToEvent map
func (handler *TaskHandler) getTaskEventList(change *sendableEvent) (taskEvents *eventList) {
	taskEvents, ok := handler.tasksToEvents[change.taskArn()]
	if !ok {
		seelog.Debug("TaskHandler, collecting events for new task ", change)
		taskEvents = &eventList{events: list.New(), sending: false}
		handler.tasksToEvents[change.taskArn()] = taskEvents
	}

	return taskEvents
}

// Continuously retries sending an event until it succeeds, sleeping between each
// attempt
func (handler *TaskHandler) SubmitTaskEvents(taskEvents *eventList, client api.ECSClient) {
	backoff := utils.NewSimpleBackoff(1*time.Second, 30*time.Second, 0.20, 1.3)

	// Mirror events.sending, but without the need to lock since this is local
	// to our goroutine
	done := false

	for !done {
		// If we looped back up here, we successfully submitted an event, but
		// we haven't emptied the list so we should keep submitting
		backoff.Reset()
		utils.RetryWithBackoff(backoff, func() error {
			// Lock and unlock within this function, allowing the list to be added
			// to while we're not actively sending an event
			seelog.Debug("TaskHandler, Waiting on semaphore to send...")
			handler.submitSemaphore.Wait()
			defer handler.submitSemaphore.Post()

			seelog.Debug("TaskHandler, Aquiring lock for sending event...")
			taskEvents.eventListLock.Lock()
			defer taskEvents.eventListLock.Unlock()
			seelog.Debug("TaskHandler, Aquired lock!")

			var err error

			if taskEvents.events.Len() == 0 {
				seelog.Debug("TaskHandler, No events left; not retrying more")

				taskEvents.sending = false
				done = true
				return nil
			}

			eventToSubmit := taskEvents.events.Front()
			event := eventToSubmit.Value.(*sendableEvent)

			if event.containerShouldBeSent() {
				sendEvent(sendContainerStatusToECS, setContainerChangeSent, "container", client,
					eventToSubmit, handler.stateSaver, backoff, taskEvents)
			} else if event.taskShouldBeSent() {
				sendEvent(sendTaskStatusToECS, setTaskChangeSent, "task", client,
					eventToSubmit, handler.stateSaver, backoff, taskEvents)
			} else if event.taskAttachmentShouldBeSent() {
				sendEvent(sendTaskStatusToECS, setTaskAttachmentSent, "task attachment", client,
					eventToSubmit, handler.stateSaver, backoff, taskEvents)
			} else {
				// Shouldn't be sent as either a task or container change event; must have been already sent
				seelog.Infof("TaskHandler, Not submitting redundant event; just removing: %s", event.String())
				taskEvents.events.Remove(eventToSubmit)
			}

			if taskEvents.events.Len() == 0 {
				seelog.Debug("TaskHandler, Removed the last element, no longer sending")
				taskEvents.sending = false
				done = true
				return nil
			}

			return err
		})
	}
}

// sendEvent tries to send an event, specified by 'eventToSubmit', of type
// 'eventType' to ECS
func sendEvent(sendStatusToECS sendStatusChangeToECS,
	setChangeSent setStatusSent,
	eventType string,
	client api.ECSClient,
	eventToSubmit *list.Element,
	stateSaver statemanager.Saver,
	backoff *utils.SimpleBackoff,
	taskEvents *eventList) {

	// Extract the wrapped event from the list element
	event := eventToSubmit.Value.(*sendableEvent)
	seelog.Infof("TaskHandler, Sending %s change: %s", eventType, event.String())
	// Try submitting the change to ECS
	if err := sendStatusToECS(client, event); err != nil {
		seelog.Errorf("TaskHandler, Unretriable error submitting %s state change [%s]: %v",
			eventType, event.String(), err)
		return
	}
	// submitted; ensure we don't retry it
	event.setSent()
	// Mark event as sent
	setChangeSent(event)
	// Update the state file
	stateSaver.Save()
	seelog.Debugf("TaskHandler, Submitted container state change: %s", event.String())
	backoff.Reset()
	taskEvents.events.Remove(eventToSubmit)
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
type setStatusSent func(event *sendableEvent)

// setContainerChangeSent sets the event's container change object as sent
func setContainerChangeSent(event *sendableEvent) {
	if event.containerChange.Container != nil {
		event.containerChange.Container.SetSentStatus(event.containerChange.Status)
	}
}

// setTaskChangeSent sets the event's task change object as sent
func setTaskChangeSent(event *sendableEvent) {
	if event.taskChange.Task != nil {
		event.taskChange.Task.SetSentStatus(event.taskChange.Status)
	}
	for _, container := range event.taskChange.Containers {
		container.Container.SetSentStatus(event.taskChange.Status.ContainerStatus(container.Container.GetSteadyStateStatus()))
	}
}

// setTaskAttachmentSent sets the event's task attachment object as sent
func setTaskAttachmentSent(event *sendableEvent) {
	if event.taskChange.Attachment != nil {
		event.taskChange.Attachment.SetSentStatus()
		event.taskChange.Attachment.StopAckTimer()
	}
}
