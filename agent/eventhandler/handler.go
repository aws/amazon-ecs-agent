// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
)

var log = logger.ForModule("eventhandler")

// statesaver is a package-wise statemanager which may be used to save any
// changes to a task or container's SentStatus
var statesaver statemanager.Saver = statemanager.NewNoopStateManager()

func HandleEngineEvents(taskEngine engine.TaskEngine, client api.ECSClient, saver statemanager.Saver) {
	statesaver = saver
	for {
		task_events := taskEngine.TaskEvents()

		for task_events != nil {
			select {
			case event, open := <-task_events:
				if !open {
					task_events = nil
					log.Error("Task events closed; this should not happen")
					break
				}

				go AddTaskEvent(event, client)
			}
		}
	}
}
