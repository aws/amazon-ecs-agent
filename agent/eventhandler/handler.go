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
	"context"
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/cihub/seelog"
)

// HandleEngineEvents handles state change events from the state change event channel by sending it to
// responsible event handler
func HandleEngineEvents(ctx context.Context, taskEngine engine.TaskEngine, client api.ECSClient,
	taskHandler *TaskHandler, attachmentEventHandler *AttachmentEventHandler) {

	for {
		stateChangeEvents := taskEngine.StateChangeEvents()

		for stateChangeEvents != nil {
			select {
			case <-ctx.Done():
				seelog.Infof("Exiting the engine event handler.")
				return
			case event, ok := <-stateChangeEvents:
				if !ok {
					stateChangeEvents = nil
					seelog.Error("Unable to handle state change event. The events channel is closed")
					break
				}
				err := handleEngineEvent(event, client, taskHandler, attachmentEventHandler)
				if err != nil {
					seelog.Errorf("Handler unable to add state change event %v: %v", event, err)
				}
			}
		}
	}
}

func handleEngineEvent(event statechange.Event, client api.ECSClient, taskHandler *TaskHandler,
	attachmentEventHandler *AttachmentEventHandler) error {
	switch event.GetEventType() {
	case statechange.TaskEvent, statechange.ContainerEvent, statechange.ManagedAgentEvent:
		return taskHandler.AddStateChangeEvent(event, client)
	case statechange.AttachmentEvent:
		return attachmentEventHandler.AddStateChangeEvent(event)
	default:
		return fmt.Errorf("unrecognized event type: %d", event.GetEventType())
	}
}
