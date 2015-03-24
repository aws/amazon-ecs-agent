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

package handlers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

const TestContainerInstanceArn = "test_container_instance_arn"
const TestClusterArn = "test_cluster_arn"

func TestMetadataHandler(t *testing.T) {
	metadataHandler := MetadataV1RequestHandlerMaker(utils.Strptr(TestContainerInstanceArn), &config.Config{Cluster: TestClusterArn})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "http://localhost:"+strconv.Itoa(config.AGENT_INTROSPECTION_PORT), nil)
	metadataHandler(w, req)

	var resp MetadataResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Cluster != TestClusterArn {
		t.Error("Metadata returned the wrong cluster arn")
	}
	if *resp.ContainerInstanceArn != TestContainerInstanceArn {
		t.Error("Metadata returned the wrong cluster arn")
	}
}

func getResponseBodyFromLocalHost(url string, t *testing.T) []byte {
	resp, err := http.Get("http://localhost:" + strconv.Itoa(config.AGENT_INTROSPECTION_PORT) + url)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestServeHttp(t *testing.T) {
	taskEngine := engine.NewTaskEngine(&config.Config{})
	containers := []*api.Container{
		&api.Container{
			Name: "c1",
		},
	}
	testTask1 := api.Task{
		Arn:           "task1",
		DesiredStatus: api.TaskRunning,
		KnownStatus:   api.TaskRunning,
		Family:        "test",
		Version:       "1",
		Containers:    containers,
	}
	testTask2 := api.Task{
                Arn:           "task2",
                DesiredStatus: api.TaskRunning,
                KnownStatus:   api.TaskStopped,
                Family:        "test",
                Version:       "1",
                Containers:    containers,
        }
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*engine.DockerTaskEngine)
	dockerTaskEngine.State().AddOrUpdateTask(&testTask1)
	dockerTaskEngine.State().AddContainer(&api.DockerContainer{DockerId: "docker1", DockerName: "someName", Container: containers[0]}, &testTask1)
	dockerTaskEngine.State().AddOrUpdateTask(&testTask2)
	go ServeHttp(utils.Strptr(TestContainerInstanceArn), taskEngine, &config.Config{Cluster: TestClusterArn})

	body := getResponseBodyFromLocalHost("/v1/metadata", t)
	var metadata MetadataResponse
	json.Unmarshal(body, &metadata)

	if metadata.Cluster != TestClusterArn {
		t.Error("Metadata returned the wrong cluster arn")
	}
	if *metadata.ContainerInstanceArn != TestContainerInstanceArn {
		t.Error("Metadata returned the wrong cluster arn")
	}
	var tasksResponse TasksResponse
	body = getResponseBodyFromLocalHost("/v1/tasks", t)
	json.Unmarshal(body, &tasksResponse)
	tasks := tasksResponse.Tasks

	if len(tasks) != 2 {
		t.Error("Incorrect number of tasks in response: ", len(tasks))
	}
	if tasks[0].Arn != "task1" {
		t.Error("Incorrect task arn in response: ", tasks[0].Arn)
	}
	if tasks[1].Arn != "task2" {
                t.Error("Incorrect task arn in response: ", tasks[1].Arn)
        }
	if tasks[0].KnownStatus != "RUNNING" {
                t.Error("Incorrect known status in response: ", tasks[0].KnownStatus)
        }
	if tasks[0].DesiredStatus != "RUNNING" {
                t.Error("Incorrect known status in response: ", tasks[0].KnownStatus)
        }
	if tasks[1].KnownStatus != "STOPPED" {
		t.Error("Incorrect known status in response: ", tasks[1].KnownStatus)
	}
	// Since the KnownStatus (STOPPED) > DesiredStatus (RUNNING), DesiredStatus should be empty
	if len(tasks[1].DesiredStatus) != 0 {
		t.Error("Incorrect desired status in response: ", tasks[1].DesiredStatus)
	}
	containersResponse := tasks[0].Containers
	if len(containersResponse) != 1 {
		t.Error("Incorrect number of containers in response: ", len(containersResponse))
	}
	if containersResponse[0].Name != "c1" {
		t.Error("Incorrect container name in response: ", containersResponse[0].Name)
	}
	var taskResponse TaskResponse
	body = getResponseBodyFromLocalHost("/v1/tasks?dockerid=docker1", t)
	json.Unmarshal(body, &taskResponse)
	if taskResponse.Arn != "task1" {
		t.Error("Incorrect task arn in response")
	}
	if taskResponse.Containers[0].Name != "c1" {
		t.Error("Incorrect task arn in response")
	}

	resp, err := http.Get("http://localhost:" + strconv.Itoa(config.AGENT_INTROSPECTION_PORT) + "/v1/tasks?dockerid=docker2")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Error("API did not return bad request status for invalid docker id")
	}

	body = getResponseBodyFromLocalHost("/v1/tasks?taskarn=task1", t)
	json.Unmarshal(body, &taskResponse)
	if taskResponse.Arn != "task1" {
		t.Error("Incorrect task arn in response")
	}

	resp, err = http.Get("http://localhost:" + strconv.Itoa(config.AGENT_INTROSPECTION_PORT) + "/v1/tasks?taskarn=invalidtaskarn")
	if resp.StatusCode != 400 {
		t.Error("API did not return bad request status for invalid task id")
	}

	resp, err = http.Get("http://localhost:" + strconv.Itoa(config.AGENT_INTROSPECTION_PORT) + "/v1/tasks?taskarn=")
	if resp.StatusCode != 400 {
		t.Error("API did not return bad request status for invalid task id")
	}

	resp, err = http.Get("http://localhost:" + strconv.Itoa(config.AGENT_INTROSPECTION_PORT) + "/v1/tasks?taskarn=task1&dockerid=docker1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Error("API did not return bad request status when both dockerid and taskarn are specified.")
	}
}
