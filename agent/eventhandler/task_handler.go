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
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

const (
	// concurrentEventCalls is the maximum number of tasks that may be handled at
	// once by the TaskHandler
	concurrentEventCalls = 3

	// drainEventsFrequency is the frequency at the which unsent events batched
	// by the task handler are sent to the backend
	drainEventsFrequency = time.Minute

	submitStateBackoffMin            = time.Second
	submitStateBackoffMax            = 30 * time.Second
	submitStateBackoffJitterMultiple = 0.20
	submitStateBackoffMultiple       = 1.3
)

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
	lock sync.RWMutex

	// stateSaver is a statemanager which may be used to save any
	// changes to a task or container's SentStatus
	stateSaver statemanager.Saver

	drainEventsFrequency time.Duration
	state                dockerstate.TaskEngineState
	client               api.ECSClient
	ctx                  context.Context
}

// eventList is used to group all events for a task
type eventList struct {
	// events is a list of *sendableEvents
	events *list.List
	// sending will check whether the list is already being handlerd
	sending bool
	//eventsListLock locks both the list and sending bool
	lock sync.Mutex
	// createdAt is a timestamp for when the event list was created
	createdAt time.Time
	// taskARN is the task arn that the event list is associated
	taskARN string
}

func (events *eventList) toStringUnsafe() string {
	return fmt.Sprintf("Task event list [taskARN: %s, sending: %t, createdAt: %s]",
		events.taskARN, events.sending, events.createdAt.String())
}

// NewTaskHandler returns a pointer to TaskHandler
func NewTaskHandler(ctx context.Context,
	stateManager statemanager.Saver,
	state dockerstate.TaskEngineState,
	client api.ECSClient) *TaskHandler {
	// Create a handler and start the periodic event drain loop
	taskHandler := &TaskHandler{
		ctx:                    ctx,
		tasksToEvents:          make(map[string]*eventList),
		submitSemaphore:        utils.NewSemaphore(concurrentEventCalls),
		tasksToContainerStates: make(map[string][]api.ContainerStateChange),
		stateSaver:             stateManager,
		state:                  state,
		client:                 client,
		drainEventsFrequency:   drainEventsFrequency,
	}
	go taskHandler.startDrainEventsTicker()

	return taskHandler
}

// startDrainEventsTicker starts a ticker that periodically drains the events queue
// by submitting state change events to the ECS backend
func (handler *TaskHandler) startDrainEventsTicker() {
	ticker := time.NewTicker(handler.drainEventsFrequency)
	for {
		select {
		case <-handler.ctx.Done():
			seelog.Infof("TaskHandler: Stopping periodic container state change submission ticker")
			ticker.Stop()
			return
		case <-ticker.C:
			for _, event := range handler.getBatchedContainerEvents() {
				seelog.Infof(
					"TaskHandler: Adding a state change event to send batched container events: %s",
					event.String())
				handler.AddStateChangeEvent(event, handler.client)
			}
		}
	}
}

// getBatchedContainerEvents gets a list task state changes for container events that
// have been batched and not sent beyond the drainEventsFrequency threshold
func (handler *TaskHandler) getBatchedContainerEvents() []api.TaskStateChange {
	handler.lock.RLock()
	defer handler.lock.RUnlock()

	var events []api.TaskStateChange
	for taskARN, _ := range handler.tasksToContainerStates {
		if task, ok := handler.state.TaskByArn(taskARN); ok {
			events = append(events, api.TaskStateChange{
				TaskARN: taskARN,
				Status:  task.GetKnownStatus(),
				Task:    task,
			})
		}
	}
	return events
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
	handler.lock.Lock()
	defer handler.lock.Unlock()

	seelog.Infof("TaskHandler: batching container event: %s", event.String())
	handler.tasksToContainerStates[event.TaskArn] = append(handler.tasksToContainerStates[event.TaskArn], event)
}

// flushBatch attaches the task arn's container events to TaskStateChange event that
// is being submittied to the backend
func (handler *TaskHandler) flushBatch(event *api.TaskStateChange) {
	handler.lock.Lock()
	defer handler.lock.Unlock()

	event.Containers = append(event.Containers, handler.tasksToContainerStates[event.TaskARN]...)
	delete(handler.tasksToContainerStates, event.TaskARN)
}

// addEvent prepares a given event to be sent by adding it to the handler's appropriate
// eventList and remove the entry in tasksToEvents map
func (handler *TaskHandler) addEvent(change *sendableEvent, client api.ECSClient) {
	taskEvents := handler.getTaskEventList(change)

	taskEvents.lock.Lock()
	defer taskEvents.lock.Unlock()

	// Update taskEvent
	seelog.Infof("TaskHandler: Adding event: %s", change.String())
	taskEvents.events.PushBack(change)

	if !taskEvents.sending {
		taskEvents.sending = true
		go handler.submitTaskEvents(taskEvents, client, change.taskArn())
	}
}

// getTaskEventList gets the eventList from taskToEvent map
func (handler *TaskHandler) getTaskEventList(change *sendableEvent) (taskEvents *eventList) {
	handler.lock.Lock()
	defer handler.lock.Unlock()

	taskARN := change.taskArn()
	taskEvents, ok := handler.tasksToEvents[taskARN]
	if !ok {
		taskEvents = &eventList{events: list.New(),
			sending:   false,
			createdAt: time.Now(),
			taskARN:   taskARN,
		}
		handler.tasksToEvents[taskARN] = taskEvents
		seelog.Debugf("TaskHandler: collecting events for new task; event: %s; events: %s ",
			change.String(), taskEvents.toStringUnsafe())
	}

	return taskEvents
}

// Continuously retries sending an event until it succeeds, sleeping between each
// attempt
func (handler *TaskHandler) submitTaskEvents(taskEvents *eventList, client api.ECSClient, taskARN string) {
	defer handler.removeTaskEvents(taskARN)

	backoff := utils.NewSimpleBackoff(submitStateBackoffMin, submitStateBackoffMax,
		submitStateBackoffJitterMultiple, submitStateBackoffMultiple)

	// Mirror events.sending, but without the need to lock since this is local
	// to our goroutine
	done := false
	sender := &taskEventsSender{
		handler:    handler,
		taskEvents: taskEvents,
		backoff:    backoff,
		client:     client,
	}

	// TODO: wire in the context here. Else, we have go routine leaks in tests
	for !done {
		// If we looped back up here, we successfully submitted an event, but
		// we haven't emptied the list so we should keep submitting
		backoff.Reset()
		utils.RetryWithBackoff(backoff, func() error {
			var err error
			done, err = sender.send()
			return err
		})
	}
}

func (handler *TaskHandler) removeTaskEvents(taskARN string) {
	handler.lock.Lock()
	defer handler.lock.Unlock()

	delete(handler.tasksToEvents, taskARN)
}

func (handler *TaskHandler) getTasksToEventsLen() int {
	handler.lock.RLock()
	defer handler.lock.RUnlock()

	return len(handler.tasksToEvents)
}

type taskEventsSender struct {
	handler    *TaskHandler
	taskEvents *eventList
	backoff    utils.Backoff
	client     api.ECSClient
}

func (sender *taskEventsSender) send() (bool, error) {
	// Lock and unlock within this function, allowing the list to be added
	// to while we're not actively sending an event
	seelog.Debug("TaskHandler: Waiting on semaphore to send events...")

	handler := sender.handler
	handler.submitSemaphore.Wait()
	defer handler.submitSemaphore.Post()

	seelog.Debug("TaskHandler: Aquiring lock for sending event...")
	taskEvents := sender.taskEvents
	taskEvents.lock.Lock()
	defer taskEvents.lock.Unlock()
	seelog.Debugf("TaskHandler: Aquired lock, processing event list: : %s", taskEvents.toStringUnsafe())

	if taskEvents.events.Len() == 0 {
		seelog.Debug("TaskHandler: No events left; not retrying more")
		taskEvents.sending = false
		return true, nil
	}

	eventToSubmit := taskEvents.events.Front()
	event := eventToSubmit.Value.(*sendableEvent)

	if event.containerShouldBeSent() {
		if err := sendEvent(sendContainerStatusToECS, setContainerChangeSent, "container",
			sender.client, eventToSubmit, handler.stateSaver, sender.backoff, taskEvents); err != nil {
			return false, err
		}
	} else if event.taskShouldBeSent() {
		if err := sendEvent(sendTaskStatusToECS, setTaskChangeSent, "task", sender.client,
			eventToSubmit, handler.stateSaver, sender.backoff, taskEvents); err != nil {
			return false, err
		}
	} else if event.taskAttachmentShouldBeSent() {
		if err := sendEvent(sendTaskStatusToECS, setTaskAttachmentSent, "task attachment",
			sender.client, eventToSubmit, handler.stateSaver, sender.backoff, taskEvents); err != nil {
			return false, err
		}
	} else {
		// Shouldn't be sent as either a task or container change event; must have been already sent
		seelog.Infof("TaskHandler: Not submitting redundant event; just removing: %s", event.String())
		taskEvents.events.Remove(eventToSubmit)
	}

	if taskEvents.events.Len() == 0 {
		seelog.Debug("TaskHandler: Removed the last element, no longer sending")
		taskEvents.sending = false
		return true, nil
	}

	return false, nil
}

// sendEvent tries to send an event, specified by 'eventToSubmit', of type
// 'eventType' to ECS
func sendEvent(sendStatusToECS sendStatusChangeToECS,
	setChangeSent setStatusSent,
	eventType string,
	client api.ECSClient,
	eventToSubmit *list.Element,
	stateSaver statemanager.Saver,
	backoff utils.Backoff,
	taskEvents *eventList) error {
	// Extract the wrapped event from the list element
	event := eventToSubmit.Value.(*sendableEvent)
	seelog.Infof("TaskHandler: Sending %s change: %s", eventType, event.String())
	// Try submitting the change to ECS
	if err := sendStatusToECS(client, event); err != nil {
		seelog.Errorf("TaskHandler: Unretriable error submitting %s state change [%s]: %v",
			eventType, event.String(), err)
		return err
	}
	// submitted; ensure we don't retry it
	event.setSent()
	// Mark event as sent
	setChangeSent(event)
	// Update the state file
	stateSaver.Save()
	seelog.Debugf("TaskHandler: Submitted container state change: %s", event.String())
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
	for _, containerStateChange := range event.taskChange.Containers {
		container := containerStateChange.Container
		container.SetSentStatus(containerStateChange.Status)
	}
}

// setTaskAttachmentSent sets the event's task attachment object as sent
func setTaskAttachmentSent(event *sendableEvent) {
	if event.taskChange.Attachment != nil {
		event.taskChange.Attachment.SetSentStatus()
		event.taskChange.Attachment.StopAckTimer()
	}
}
