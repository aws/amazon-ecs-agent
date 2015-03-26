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
	testTask := api.Task{
		Arn:           "task1",
		DesiredStatus: api.TaskRunning,
		KnownStatus:   api.TaskRunning,
		Family:        "test",
		Version:       "1",
		Containers:    containers,
	}
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*engine.DockerTaskEngine)
	dockerTaskEngine.State().AddOrUpdateTask(&testTask)
	dockerTaskEngine.State().AddContainer(&api.DockerContainer{DockerId: "docker1", DockerName: "someName", Container: containers[0]}, &testTask)
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

	if len(tasks) != 1 {
		t.Error("Incorrect number of tasks in response: ", len(tasks))
	}
	if tasks[0].Arn != "task1" {
		t.Error("Incorrect task arn in response: ", tasks[0].Arn)
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

	resp, err = http.Get("http://localhost:" + strconv.Itoa(config.AGENT_INTROSPECTION_PORT) + "/v1/tasks?taskarn=task2")
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

func backendMappingTestHelper(containers []*api.Container, testTask api.Task, desiredStatus string, knownStatus string, t *testing.T) {
	taskEngine := engine.NewTaskEngine(&config.Config{})
	// Populate Tasks and Container map in the engine.
	dockerTaskEngine, _ := taskEngine.(*engine.DockerTaskEngine)
	dockerTaskEngine.State().AddOrUpdateTask(&testTask)
	dockerTaskEngine.State().AddContainer(&api.DockerContainer{DockerId: "docker1", DockerName: "someName", Container: containers[0]}, &testTask)
	taskHandler := TasksV1RequestHandlerMaker(taskEngine)
	server := httptest.NewServer(http.HandlerFunc(taskHandler))
	defer server.Close()
	resp, err := http.Get(server.URL+"/v1/tasks")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var taskResponse TasksResponse
	json.Unmarshal(body, &taskResponse)
	tasks := taskResponse.Tasks
	if tasks[0].DesiredStatus != desiredStatus {
		t.Error("Incorrect known status in response: ", tasks[0].DesiredStatus)
	}
	if tasks[0].KnownStatus != knownStatus {
		t.Error("Incorrect known status in response: ", tasks[0].KnownStatus)
	}
}

func TestBackendMapping(t *testing.T) {
	containers := []*api.Container{
		&api.Container{
			Name: "c1",
		},
	}
	testTask := api.Task{
		Arn:           "task1",
		DesiredStatus: api.TaskRunning,
		KnownStatus:   api.TaskRunning,
		Family:        "test",
		Version:       "1",
		Containers:    containers,
	}
	backendMappingTestHelper(containers, testTask, "RUNNING", "RUNNING", t)

	testTask = api.Task{
		Arn:           "task1",
		DesiredStatus: api.TaskRunning,
		KnownStatus:   api.TaskStopped,
		Family:        "test",
		Version:       "1",
		Containers:    containers,
	}
	// Since the KnownStatus (STOPPED) > DesiredStatus (RUNNING), DesiredStatus should be empty
	backendMappingTestHelper(containers, testTask, "", "STOPPED", t)
}
