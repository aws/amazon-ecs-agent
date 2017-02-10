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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

var handler *taskHandler

func init() {
	handler = newTaskHandler()
}

// AddTaskEvent queues up a state change for sending using the given client.
func AddTaskEvent(change api.TaskStateChange, client api.ECSClient) {
	addEvent(newSendableTaskEvent(change), client)
}

// AddContainerEvent queues up a state change for sending using the given client.
func AddContainerEvent(change api.ContainerStateChange, client api.ECSClient) {
	addEvent(newSendableContainerEvent(change), client)
}

// Prepares a given event to be sent by adding it to the handler's appropriate
// eventList
func addEvent(change *sendableEvent, client api.ECSClient) {
	var taskList *eventList
	var preexisting bool
	log.Info("Adding event", "change", change)

	// TaskEvents lock scope
	func() {
		handler.Lock()
		defer handler.Unlock()

		taskList, preexisting = handler.taskMap[change.taskArn()]
		if !preexisting {
			log.Debug("New event", "change", change)
			taskList = &eventList{List: list.New(), sending: false}
			handler.taskMap[change.taskArn()] = taskList
		}
	}()

	taskList.Lock()
	defer taskList.Unlock()

	// Update taskEvent
	taskList.PushBack(change)

	if !taskList.sending {
		taskList.sending = true
		go SubmitTaskEvents(taskList, client)
	}
}

// Continuously retries sending an event until it succeeds, sleeping between each
// attempt
func SubmitTaskEvents(events *eventList, client api.ECSClient) {
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
			log.Debug("Waiting on semaphore to send...")
			handler.submitSemaphore.Wait()
			defer handler.submitSemaphore.Post()

			log.Debug("Aquiring lock for sending event...")
			events.Lock()
			defer events.Unlock()
			log.Debug("Aquired lock!")

			var err error

			if events.Len() == 0 {
				log.Debug("No events left; not retrying more")

				events.sending = false
				done = true
				return nil
			}

			eventToSubmit := events.Front()
			event := eventToSubmit.Value.(*sendableEvent)
			llog := log.New("event", event)

			if event.containerShouldBeSent() {
				llog.Info("Sending container change", "change", event)
				err = client.SubmitContainerStateChange(event.containerChange)
				if err == nil {
					// submitted; ensure we don't retry it
					event.containerSent = true
					if event.containerChange.Container != nil {
						event.containerChange.Container.SetSentStatus(event.containerChange.Status)
					}
					statesaver.Save()
					llog.Debug("Submitted container state change")
					backoff.Reset()
					events.Remove(eventToSubmit)
				} else {
					llog.Error("Unretriable error submitting container state change", "err", err)
				}
			} else if event.taskShouldBeSent() {
				llog.Info("Sending task change", "change", event)
				err = client.SubmitTaskStateChange(event.taskChange)
				if err == nil {
					// submitted or can't be retried; ensure we don't retry it
					event.taskSent = true
					if event.taskChange.Task != nil {
						event.taskChange.Task.SetSentStatus(event.taskChange.Status)
					}
					statesaver.Save()
					llog.Debug("Submitted task state change")
					backoff.Reset()
					events.Remove(eventToSubmit)
				} else {
					llog.Error("Unretriable error submitting container state change", "err", err)
				}
			} else {
				// Shouldn't be sent as either a task or container change event; must have been already sent
				llog.Info("Not submitting redundant event; just removing")
				events.Remove(eventToSubmit)
			}

			if events.Len() == 0 {
				llog.Debug("Removed the last element, no longer sending")
				events.sending = false
				done = true
				return nil
			}

			return err
		})
	}
}
