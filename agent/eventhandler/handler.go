// Copyright 2014 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
)

var log = logger.ForModule("eventhandler")

func HandleEngineEvents(taskEngine engine.TaskEngine, client api.ECSClient) {
	for {
		task_events, task_errc := taskEngine.TaskEvents()

		for task_events != nil {
			select {
			case event, open := <-task_events:
				if !open {
					task_events = nil
					log.Error("Task events closed; this should not happen")
					break
				}

				go AddTaskEvent(event, client)
			case err, _ := <-task_errc:
				log.Error("Error with task events", "err", err)
			}
		}
	}
}
