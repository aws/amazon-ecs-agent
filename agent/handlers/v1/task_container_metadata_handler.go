// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package v1

import (
	"encoding/json"
	"net/http"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	"github.com/cihub/seelog"
)

const (
	// TaskContainerMetadataPath is the task/container metadata path for v1 handler.
	TaskContainerMetadataPath = "/v1/tasks"
	dockerIDQueryField        = "dockerid"
	taskARNQueryField         = "taskarn"
	dockerShortIDLen          = 12
)

// createTaskResponse creates JSON response and sets the http status code for the task queried.
func createTaskResponse(task *apitask.Task, found bool, resourceID string, state dockerstate.TaskEngineState) ([]byte, int) {
	var responseJSON []byte
	status := http.StatusOK
	if found {
		containerMap, _ := state.ContainerMapByArn(task.Arn)
		responseJSON, _ = json.Marshal(NewTaskResponse(task, containerMap))
	} else {
		seelog.Warn("Could not find requested resource: " + resourceID)
		responseJSON, _ = json.Marshal(&TaskResponse{})
		status = http.StatusNotFound
	}
	return responseJSON, status
}

// TaskContainerMetadataHandler creates response for the 'v1/tasks' API. Lists all tasks if the request
// doesn't contain any fields. Returns a Task if either of 'dockerid' or
// 'taskarn' are specified in the request.
func TaskContainerMetadataHandler(taskEngine utils.DockerStateResolver) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var responseJSON []byte
		dockerTaskEngineState := taskEngine.State()
		dockerID, dockerIDExists := utils.ValueFromRequest(r, dockerIDQueryField)
		taskArn, taskARNExists := utils.ValueFromRequest(r, taskARNQueryField)
		var status int
		if dockerIDExists && taskARNExists {
			seelog.Info("Request contains both ", dockerIDQueryField, " and ", taskARNQueryField, ". Expect at most one of these.")
			w.WriteHeader(http.StatusBadRequest)
			w.Write(responseJSON)
			return
		}
		if dockerIDExists {
			// Create TaskResponse for the docker id in the query.
			var task *apitask.Task
			var found bool
			if len(dockerID) > dockerShortIDLen {
				task, found = dockerTaskEngineState.TaskByID(dockerID)
			} else {
				tasks, _ := dockerTaskEngineState.TaskByShortID(dockerID)
				if len(tasks) == 0 {
					task = nil
					found = false
				} else if len(tasks) == 1 {
					task = tasks[0]
					found = true
				} else {
					seelog.Info("Multiple tasks found for requested dockerId: " + dockerID)
					w.WriteHeader(http.StatusBadRequest)
					w.Write(responseJSON)
					return
				}
			}
			responseJSON, status = createTaskResponse(task, found, dockerID, dockerTaskEngineState)
			w.WriteHeader(status)
		} else if taskARNExists {
			// Create TaskResponse for the task arn in the query.
			task, found := dockerTaskEngineState.TaskByArn(taskArn)
			responseJSON, status = createTaskResponse(task, found, taskArn, dockerTaskEngineState)
			w.WriteHeader(status)
		} else {
			// List all tasks.
			responseJSON, _ = json.Marshal(NewTasksResponse(dockerTaskEngineState))
		}
		w.Write(responseJSON)
	}
}
