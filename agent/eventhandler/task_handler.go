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
	tasksToEvents map[string]*taskSendableEvents
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

// taskSendableEvents is used to group all events for a task
type taskSendableEvents struct {
	// events is a list of *sendableEvents
	events *list.List
	// sending will check whether the list is already being handlerd
	sending bool
	//eventsListLock locks both the list and sending bool
	lock sync.Mutex
	// createdAt is a timestamp for when the event list was created
	createdAt time.Time
	// taskARN is the task arn that the event list is associated with
	taskARN string
}

// NewTaskHandler returns a pointer to TaskHandler
func NewTaskHandler(ctx context.Context,
	stateManager statemanager.Saver,
	state dockerstate.TaskEngineState,
	client api.ECSClient) *TaskHandler {
	// Create a handler and start the periodic event drain loop
	taskHandler := &TaskHandler{
		ctx:                    ctx,
		tasksToEvents:          make(map[string]*taskSendableEvents),
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

// AddStateChangeEvent queues up a state change for sending using the given client.
func (handler *TaskHandler) AddStateChangeEvent(change statechange.Event, client api.ECSClient) error {
	handler.lock.Lock()
	defer handler.lock.Unlock()

	switch change.GetEventType() {
	case statechange.TaskEvent:
		event, ok := change.(api.TaskStateChange)
		if !ok {
			return errors.New("eventhandler: unable to get task event from state change event")
		}
		handler.flushBatchUnsafe(&event, client)
		return nil

	case statechange.ContainerEvent:
		event, ok := change.(api.ContainerStateChange)
		if !ok {
			return errors.New("eventhandler: unable to get container event from state change event")
		}
		handler.batchContainerEventUnsafe(event)
		return nil

	default:
		return errors.New("eventhandler: unable to determine event type from state change event")
	}
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
	for taskARN := range handler.tasksToContainerStates {
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

// batchContainerEventUnsafe collects container state change events for a given task arn
func (handler *TaskHandler) batchContainerEventUnsafe(event api.ContainerStateChange) {
	seelog.Infof("TaskHandler: batching container event: %s", event.String())
	handler.tasksToContainerStates[event.TaskArn] = append(handler.tasksToContainerStates[event.TaskArn], event)
}

// flushBatchUnsafe attaches the task arn's container events to TaskStateChange event that
// is being submittied to the backend and sends it to the backend
func (handler *TaskHandler) flushBatchUnsafe(event *api.TaskStateChange, client api.ECSClient) {
	event.Containers = append(event.Containers, handler.tasksToContainerStates[event.TaskARN]...)
	delete(handler.tasksToContainerStates, event.TaskARN)

	// Prepare a given event to be sent by adding it to the handler's appropriate
	// eventList and remove the entry in tasksToEvents map
	change := newSendableTaskEvent(*event)
	taskEvents := handler.getTaskEventsUnsafe(change)

	taskEvents.sendChange(change, client, handler)
}

// getTaskEventsUnsafe gets the eventList from taskToEvent map
func (handler *TaskHandler) getTaskEventsUnsafe(change *sendableEvent) *taskSendableEvents {
	taskARN := change.taskArn()
	taskEvents, ok := handler.tasksToEvents[taskARN]
	if !ok {
		taskEvents = &taskSendableEvents{
			events:    list.New(),
			sending:   false,
			createdAt: time.Now(),
			taskARN:   taskARN,
		}
		handler.tasksToEvents[taskARN] = taskEvents
		seelog.Debugf("TaskHandler: collecting events for new task; event: %s; events: %s ",
			change.toString(), taskEvents.toStringUnsafe())
	}

	return taskEvents
}

// Continuously retries sending an event until it succeeds, sleeping between each
// attempt
func (handler *TaskHandler) submitTaskEvents(taskEvents *taskSendableEvents, client api.ECSClient, taskARN string) {
	defer handler.removeTaskEvents(taskARN)

	backoff := utils.NewSimpleBackoff(submitStateBackoffMin, submitStateBackoffMax,
		submitStateBackoffJitterMultiple, submitStateBackoffMultiple)

	// Mirror events.sending, but without the need to lock since this is local
	// to our goroutine
	done := false
	// TODO: wire in the context here. Else, we have go routine leaks in tests
	for !done {
		// If we looped back up here, we successfully submitted an event, but
		// we haven't emptied the list so we should keep submitting
		backoff.Reset()
		utils.RetryWithBackoff(backoff, func() error {
			// Lock and unlock within this function, allowing the list to be added
			// to while we're not actively sending an event
			seelog.Debug("TaskHandler: Waiting on semaphore to send events...")
			handler.submitSemaphore.Wait()
			defer handler.submitSemaphore.Post()

			var err error
			done, err = taskEvents.submitFirstEvent(handler, backoff)
			return err
		})
	}
}

func (handler *TaskHandler) removeTaskEvents(taskARN string) {
	handler.lock.Lock()
	defer handler.lock.Unlock()

	delete(handler.tasksToEvents, taskARN)
}

// sendChange adds the change to the sendable events list. If the
// event for the task is in the process of being sent, it doesn't trigger
// the async submit state api method
func (taskEvents *taskSendableEvents) sendChange(change *sendableEvent,
	client api.ECSClient,
	handler *TaskHandler) {

	taskEvents.lock.Lock()
	defer taskEvents.lock.Unlock()

	// Update taskEvent
	seelog.Infof("TaskHandler: Adding event: %s", change.toString())
	taskEvents.events.PushBack(change)

	if !taskEvents.sending {
		taskEvents.sending = true
		go handler.submitTaskEvents(taskEvents, client, change.taskArn())
	} else {
		seelog.Debugf("TaskHandler: Not submitting change as the task is already being sent: %s", change.toString())
	}
}

// submitFirstEvent submits the first event for the task from the event list. It
// returns true if the list became empty after submitting the event. Else, it returns
// false. An error is returned if there was an error with submitting the state change
// to ECS. The error is used by the backoff handler to backoff before retrying the
// state change submission for the first event
func (taskEvents *taskSendableEvents) submitFirstEvent(handler *TaskHandler, backoff utils.Backoff) (bool, error) {
	seelog.Debug("TaskHandler: Aquiring lock for sending event...")
	taskEvents.lock.Lock()
	defer taskEvents.lock.Unlock()

	seelog.Debugf("TaskHandler: Aquired lock, processing event list: : %s", taskEvents.toStringUnsafe())

	if taskEvents.events.Len() == 0 {
		seelog.Debug("TaskHandler: No events left; not retrying more")
		taskEvents.sending = false
		return true, nil
	}

	eventToSubmit := taskEvents.events.Front()
	// Extract the wrapped event from the list element
	event := eventToSubmit.Value.(*sendableEvent)

	if event.containerShouldBeSent() {
		if err := event.send(sendContainerStatusToECS, setContainerChangeSent, "container",
			handler.client, eventToSubmit, handler.stateSaver, backoff, taskEvents); err != nil {
			return false, err
		}
	} else if event.taskShouldBeSent() {
		if err := event.send(sendTaskStatusToECS, setTaskChangeSent, "task",
			handler.client, eventToSubmit, handler.stateSaver, backoff, taskEvents); err != nil {
			return false, err
		}
	} else if event.taskAttachmentShouldBeSent() {
		if err := event.send(sendTaskStatusToECS, setTaskAttachmentSent, "task attachment",
			handler.client, eventToSubmit, handler.stateSaver, backoff, taskEvents); err != nil {
			return false, err
		}
	} else {
		// Shouldn't be sent as either a task or container change event; must have been already sent
		seelog.Infof("TaskHandler: Not submitting redundant event; just removing: %s", event.toString())
		taskEvents.events.Remove(eventToSubmit)
	}

	if taskEvents.events.Len() == 0 {
		seelog.Debug("TaskHandler: Removed the last element, no longer sending")
		taskEvents.sending = false
		return true, nil
	}

	return false, nil
}

func (taskEvents *taskSendableEvents) toStringUnsafe() string {
	return fmt.Sprintf("Task event list [taskARN: %s, sending: %t, createdAt: %s]",
		taskEvents.taskARN, taskEvents.sending, taskEvents.createdAt.String())
}

// getTasksToEventsLen returns the length of the tasksToEvents map. It is
// used only in the test code to ascertain that map has been cleaned up
func (handler *TaskHandler) getTasksToEventsLen() int {
	handler.lock.RLock()
	defer handler.lock.RUnlock()

	return len(handler.tasksToEvents)
}
