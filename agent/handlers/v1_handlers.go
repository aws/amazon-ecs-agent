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

// Package handlers deals with the agent introspection api.
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/version"
)

var log = logger.ForModule("Handlers")

const statusBadRequest = 400
const statusNotImplemented = 501
const statusOK = 200
const statusInternalServerError = 500

const dockerIdQueryField = "dockerid"
const taskArnQueryField = "taskarn"

type RootResponse struct {
	AvailableCommands []string
}

func MetadataV1RequestHandlerMaker(containerInstanceArn *string, cfg *config.Config) func(http.ResponseWriter, *http.Request) {
	resp := &MetadataResponse{
		Cluster:              cfg.Cluster,
		ContainerInstanceArn: containerInstanceArn,
		Version:              version.String(),
	}
	responseJSON, _ := json.Marshal(resp)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Write(responseJSON)
	}
}

func NewTaskResponse(task *api.Task, containerMap map[string]*api.DockerContainer) *TaskResponse {
	containers := []ContainerResponse{}
	for containerName, container := range containerMap {
		if container.Container.IsInternal {
			continue
		}
		containers = append(containers, ContainerResponse{container.DockerId, container.DockerName, containerName})
	}

	knownStatus := task.KnownStatus.BackendStatus()
	desiredStatus := task.DesiredStatus.BackendStatus()

	if (knownStatus == "STOPPED" && desiredStatus != "STOPPED") || (knownStatus == "RUNNING" && desiredStatus == "PENDING") {
		desiredStatus = ""
	}

	return &TaskResponse{
		Arn:           task.Arn,
		DesiredStatus: desiredStatus,
		KnownStatus:   knownStatus,
		Family:        task.Family,
		Version:       task.Version,
		Containers:    containers,
	}
}

func NewTasksResponse(state *dockerstate.DockerTaskEngineState) *TasksResponse {
	allTasks := state.AllTasks()
	taskResponses := make([]*TaskResponse, len(allTasks))
	for ndx, task := range allTasks {
		containerMap, _ := state.ContainerMapByArn(task.Arn)
		taskResponses[ndx] = NewTaskResponse(task, containerMap)
	}

	return &TasksResponse{Tasks: taskResponses}
}

// Returns the value of a field in the http request. The boolean value is
// set to true if the field exists in the query.
func valueFromRequest(r *http.Request, field string) (string, bool) {
	values := r.URL.Query()
	_, exists := values[field]
	return values.Get(field), exists
}

// Creates JSON response and sets the http status code for the task queried.
func createTaskJSONResponse(task *api.Task, found bool, resourceId string, state *dockerstate.DockerTaskEngineState) ([]byte, int) {
	var responseJSON []byte
	status := statusOK
	if found {
		containerMap, _ := state.ContainerMapByArn(task.Arn)
		responseJSON, _ = json.Marshal(NewTaskResponse(task, containerMap))
	} else {
		log.Warn("Could not find", resourceId)
		responseJSON, _ = json.Marshal(&TaskResponse{})
		status = statusBadRequest
	}
	return responseJSON, status
}

// Creates response for the 'v1/tasks' API. Lists all tasks if the request
// doesn't contain any fields. Returns a Task if either of 'dockerid' or
// 'taskarn' are specified in the request.
func TasksV1RequestHandlerMaker(taskEngine engine.TaskEngine) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var responseJSON []byte
		dockerTaskEngine, ok := taskEngine.(*engine.DockerTaskEngine)
		if !ok {
			// Could not load docker task engine.
			w.WriteHeader(statusInternalServerError)
			w.Write(responseJSON)
			return
		}
		dockerTaskEngineState := dockerTaskEngine.State()
		dockerId, dockerIdExists := valueFromRequest(r, dockerIdQueryField)
		taskArn, taskArnExists := valueFromRequest(r, taskArnQueryField)
		var status int
		if dockerIdExists && taskArnExists {
			log.Info("Request contains both ", dockerIdQueryField, " and ", taskArnQueryField, ". Expect at most one of these.")
			w.WriteHeader(statusBadRequest)
			w.Write(responseJSON)
			return
		}
		if dockerIdExists {
			// Create TaskResponse for the docker id in the query.
			task, found := dockerTaskEngineState.TaskById(dockerId)
			responseJSON, status = createTaskJSONResponse(task, found, dockerId, dockerTaskEngineState)
			w.WriteHeader(status)
		} else if taskArnExists {
			// Create TaskResponse for the task arn in the query.
			task, found := dockerTaskEngineState.TaskByArn(taskArn)
			responseJSON, status = createTaskJSONResponse(task, found, taskArn, dockerTaskEngineState)
			w.WriteHeader(status)
		} else {
			// List all tasks.
			responseJSON, _ = json.Marshal(NewTasksResponse(dockerTaskEngineState))
		}
		w.Write(responseJSON)
	}
}

func ServeHttp(containerInstanceArn *string, taskEngine engine.TaskEngine, cfg *config.Config) {
	serverFunctions := map[string]func(w http.ResponseWriter, r *http.Request){
		"/v1/metadata": MetadataV1RequestHandlerMaker(containerInstanceArn, cfg),
		"/v1/tasks":    TasksV1RequestHandlerMaker(taskEngine),
	}

	paths := make([]string, 0, len(serverFunctions))
	for path := range serverFunctions {
		paths = append(paths, path)
	}
	availableCommands := &RootResponse{paths}
	// Autogenerated list of the above serverFunctions paths
	availableCommandResponse, _ := json.Marshal(&availableCommands)

	defaultHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Write(availableCommandResponse)
	}

	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/", defaultHandler)
	for key, fn := range serverFunctions {
		serverMux.HandleFunc(key, fn)
	}

	// Log all requests and then pass through to serverMux
	loggingServeMux := http.NewServeMux()
	loggingServeMux.Handle("/", LoggingHandler{serverMux})

	server := http.Server{
		Addr:         ":" + strconv.Itoa(config.AGENT_INTROSPECTION_PORT),
		Handler:      loggingServeMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	for {
		once := sync.Once{}
		utils.RetryWithBackoff(utils.NewSimpleBackoff(time.Second, time.Minute, 0.2, 2), func() error {
			// TODO, make this cancellable and use the passed in context; for
			// now, not critical if this gets interrupted
			err := server.ListenAndServe()
			once.Do(func() {
				log.Error("Error running http api", "err", err)
			})
			return err
		})
	}
}
