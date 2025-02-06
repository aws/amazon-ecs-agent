//go:build integration
// +build integration

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

package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_utils "github.com/aws/amazon-ecs-agent/agent/handlers/mocks"
	v1 "github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection"
	introspection_v1 "github.com/aws/amazon-ecs-agent/ecs-agent/introspection/v1"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testContainerInstanceArn = "test_container_instance_arn"
	testClusterArn           = "test_cluster_arn"
	ipv4Address              = "10.0.0.2"
)

var runtimeStatsConfigForTest = config.BooleanDefaultFalse{}

func basicIntegConfig(t *testing.T, ctrl *gomock.Controller) inspecIntegConfig {
	mockStateResolver := mock_utils.NewMockDockerStateResolver(ctrl)

	state := dockerstate.NewTaskEngineState()
	stateSetupHelper(state, testTasks())

	mockStateResolver.EXPECT().State().Return(state)

	return inspecIntegConfig{
		state: &v1.AgentStateImpl{
			ContainerInstanceArn: aws.String(testContainerInstanceArn),
			ClusterName:          testClusterArn,
			TaskEngine:           mockStateResolver,
		},
		metricsFactory: metrics.NewNopEntryFactory(),
	}
}

func TestMetadataHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateResolver := mock_utils.NewMockDockerStateResolver(ctrl)
	testConfig := inspecIntegConfig{
		state: &v1.AgentStateImpl{
			ContainerInstanceArn: aws.String(testContainerInstanceArn),
			ClusterName:          testClusterArn,
			TaskEngine:           mockStateResolver,
		},
		metricsFactory: metrics.NewNopEntryFactory(),
	}
	response := testConfig.performRequest(t, "/v1/metadata")

	var resp introspection_v1.AgentMetadataResponse
	unmarshalResponse(t, response, &resp)

	if resp.Cluster != testClusterArn {
		t.Error("Metadata returned the wrong cluster arn")
	}
	if *resp.ContainerInstanceArn != testContainerInstanceArn {
		t.Error("Metadata returned the wrong cluster arn")
	}
}

func TestListMultipleTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks")

	var tasksResponse introspection_v1.TasksResponse
	unmarshalResponse(t, response, &tasksResponse)

	taskDiffHelper(t, testTasks(), tasksResponse)
}

func TestGetTaskByDockerID(t *testing.T) {
	// stateSetupHelper uses the convention of dockerid-$arn-$containerName; the
	// second task has a container named foo
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?dockerid=dockerid-task2-foo")

	var taskResponse introspection_v1.TaskResponse
	unmarshalResponse(t, response, &taskResponse)

	taskDiffHelper(t, []*apitask.Task{testTasks()[1]}, introspection_v1.TasksResponse{Tasks: []*introspection_v1.TaskResponse{&taskResponse}})
}

func TestTaskNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?dockerid=dockerid-task2-foo-bar")

	var taskResponse introspection_v1.TaskResponse
	unmarshalResponse(t, response, &taskResponse)

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
	assert.Equal(t, introspection_v1.TaskResponse{}, taskResponse)
}

func TestGetTaskByShortDockerIDMultiple(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?dockerid=dockerid-tas")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode, "Expected http 400 for dockerid with multiple matches")
}

func TestGetTaskShortByDockerID404(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?dockerid=notfound")

	assert.Equal(t, http.StatusNotFound, response.StatusCode, "API did not return 404 for bad dockerid")
}

func TestGetTaskByShortDockerID(t *testing.T) {
	// stateSetupHelper uses the convention of dockerid-$arn-$containerName; the
	// first task has a container name prefix of dockerid-tas
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?dockerid=dockerid-by")

	var taskResponse introspection_v1.TaskResponse
	unmarshalResponse(t, response, &taskResponse)

	taskDiffHelper(t, []*apitask.Task{testTasks()[2]}, introspection_v1.TasksResponse{Tasks: []*introspection_v1.TaskResponse{&taskResponse}})
}

func TestGetTaskByDockerID404(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?dockerid=does-not-exist")

	if response.StatusCode != 404 {
		t.Error("API did not return 404 for bad dockerid")
	}
}

func TestGetTaskByTaskArn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?taskarn=task1")

	var taskResponse introspection_v1.TaskResponse
	unmarshalResponse(t, response, &taskResponse)

	taskDiffHelper(t, []*apitask.Task{testTasks()[0]}, introspection_v1.TasksResponse{Tasks: []*introspection_v1.TaskResponse{&taskResponse}})
}

func TestGetAWSVPCTaskByTaskArn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?taskarn=awsvpcTask")

	var taskResponse introspection_v1.TaskResponse
	unmarshalResponse(t, response, &taskResponse)

	resp := introspection_v1.TasksResponse{Tasks: []*introspection_v1.TaskResponse{&taskResponse}}

	assert.Equal(t, ipv4Address, resp.Tasks[0].Containers[0].Networks[0].IPv4Addresses[0])
	taskDiffHelper(t, []*apitask.Task{testTasks()[3]}, resp)
}

func TestGetHostNeworkingTaskByTaskArn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?taskarn=hostModeNetworkingTask")

	var taskResponse introspection_v1.TaskResponse
	unmarshalResponse(t, response, &taskResponse)

	resp := introspection_v1.TasksResponse{Tasks: []*introspection_v1.TaskResponse{&taskResponse}}

	assert.Equal(t, uint16(80), resp.Tasks[0].Containers[0].Ports[0].ContainerPort)
	assert.Equal(t, "tcp", resp.Tasks[0].Containers[0].Ports[0].Protocol)

	taskDiffHelper(t, []*apitask.Task{testTasks()[4]}, resp)
}

func TestGetBridgeNeworkingTaskByTaskArn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?taskarn=bridgeModeNetworkingTask")

	var taskResponse introspection_v1.TaskResponse
	unmarshalResponse(t, response, &taskResponse)

	resp := introspection_v1.TasksResponse{Tasks: []*introspection_v1.TaskResponse{&taskResponse}}

	assert.Equal(t, uint16(80), resp.Tasks[0].Containers[0].Ports[0].ContainerPort)
	assert.Equal(t, "tcp", resp.Tasks[0].Containers[0].Ports[0].Protocol)

	taskDiffHelper(t, []*apitask.Task{testTasks()[5]}, resp)
}

func TestGetTaskByTaskArnNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	response := basicIntegConfig(t, ctrl).performRequest(t, "/v1/tasks?taskarn=doesnotexist")

	if response.StatusCode != http.StatusNotFound {
		t.Errorf("Expected %d for bad taskarn, but was %d", http.StatusNotFound, response.StatusCode)
	}
}

func TestBackendMismatchMapping(t *testing.T) {
	// Test that a KnownStatus past a DesiredStatus suppresses the DesiredStatus output
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateResolver := mock_utils.NewMockDockerStateResolver(ctrl)

	containers := []*apicontainer.Container{
		{
			Name: "c1",
		},
	}
	testTask := &apitask.Task{
		Arn:                 "task1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskStopped,
		Family:              "test",
		Version:             "1",
		Containers:          containers,
	}

	state := dockerstate.NewTaskEngineState()
	stateSetupHelper(state, []*apitask.Task{testTask})

	mockStateResolver.EXPECT().State().Return(state)

	testConfig := inspecIntegConfig{
		state: &v1.AgentStateImpl{
			ContainerInstanceArn: aws.String(testContainerInstanceArn),
			ClusterName:          testClusterArn,
			TaskEngine:           mockStateResolver,
		},
		metricsFactory: metrics.NewNopEntryFactory(),
	}

	response := testConfig.performRequest(t, "/v1/tasks")

	var tasksResponse introspection_v1.TasksResponse
	unmarshalResponse(t, response, &tasksResponse)

	if tasksResponse.Tasks[0].DesiredStatus != "" {
		t.Error("Expected '', was ", tasksResponse.Tasks[0].DesiredStatus)
	}
	if tasksResponse.Tasks[0].KnownStatus != "STOPPED" {
		t.Error("Expected STOPPED, was ", tasksResponse.Tasks[0].KnownStatus)
	}
}

func TestPProfFlag(t *testing.T) {
	t.Run("profiling enabled", func(t *testing.T) {
		testConfig := inspecIntegConfig{
			state: &v1.AgentStateImpl{
				ContainerInstanceArn: aws.String(testContainerInstanceArn),
				ClusterName:          testClusterArn,
				TaskEngine:           nil,
			},
			metricsFactory: metrics.NewNopEntryFactory(),
		}

		response := testConfig.performRequest(t, "/", introspection.WithRuntimeStats(true))

		assert.Equal(t, http.StatusOK, response.StatusCode)

		bodyBytes, err := io.ReadAll(response.Body)
		assert.NoError(t, err)
		// List of available commands should include pprof endpoints.
		assert.Contains(t, string(bodyBytes), "pprof")
	})

	t.Run("profiling disabled", func(t *testing.T) {
		testConfig := inspecIntegConfig{
			state: &v1.AgentStateImpl{
				ContainerInstanceArn: aws.String(testContainerInstanceArn),
				ClusterName:          testClusterArn,
				TaskEngine:           nil,
			},
			metricsFactory: metrics.NewNopEntryFactory(),
		}

		response := testConfig.performRequest(t, "/", introspection.WithRuntimeStats(false))

		assert.Equal(t, http.StatusOK, response.StatusCode)

		bodyBytes, err := io.ReadAll(response.Body)
		assert.NoError(t, err)
		// List of available commands should NOT include pprof endpoints.
		assert.NotContains(t, string(bodyBytes), "pprof")
	})

	t.Run("default: profiling disabled", func(t *testing.T) {
		testConfig := inspecIntegConfig{
			state: &v1.AgentStateImpl{
				ContainerInstanceArn: aws.String(testContainerInstanceArn),
				ClusterName:          testClusterArn,
				TaskEngine:           nil,
			},
			metricsFactory: metrics.NewNopEntryFactory(),
		}

		response := testConfig.performRequest(t, "/")

		assert.Equal(t, http.StatusOK, response.StatusCode)

		bodyBytes, err := io.ReadAll(response.Body)
		assert.NoError(t, err)
		// List of available commands should NOT include pprof endpoints.
		assert.NotContains(t, string(bodyBytes), "pprof")
	})
}

func taskDiffHelper(t *testing.T, expected []*apitask.Task, actual introspection_v1.TasksResponse) {
	if len(expected) != len(actual.Tasks) {
		t.Errorf("Expected %v tasks, had %v tasks", len(expected), len(actual.Tasks))
	}

	for _, task := range expected {
		// Find related actual task
		var respTask *introspection_v1.TaskResponse
		for _, actualTask := range actual.Tasks {
			if actualTask.Arn == task.Arn {
				respTask = actualTask
			}
		}

		if respTask == nil {
			t.Errorf("Could not find matching task for arn: %v", task.Arn)
			continue
		}

		if respTask.DesiredStatus != task.GetDesiredStatus().String() {
			t.Errorf("DesiredStatus mismatch: %v != %v", respTask.DesiredStatus, task.GetDesiredStatus())
		}
		taskKnownStatus := task.GetKnownStatus().String()
		if respTask.KnownStatus != taskKnownStatus {
			t.Errorf("KnownStatus mismatch: %v != %v", respTask.KnownStatus, taskKnownStatus)
		}

		if respTask.Family != task.Family || respTask.Version != task.Version {
			t.Errorf("Family mismatch: %v:%v != %v:%v", respTask.Family, respTask.Version, task.Family, task.Version)
		}

		if len(respTask.Containers) != len(task.Containers) {
			t.Errorf("Expected %v containers in %v, was %v", len(task.Containers), task.Arn, len(respTask.Containers))
			continue
		}
		for _, respCont := range respTask.Containers {
			_, ok := task.ContainerByName(respCont.Name)
			if !ok {
				t.Errorf("Could not find container %v", respCont.Name)
			}
			if respCont.DockerID == "" {
				t.Error("blank dockerid")
			}
		}
	}
}

func testTasks() []*apitask.Task {
	return []*apitask.Task{
		{
			Arn:                 "task1",
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
			KnownStatusUnsafe:   apitaskstatus.TaskRunning,
			Family:              "test",
			Version:             "1",
			Containers: []*apicontainer.Container{
				{
					Name: "one",
				},
				{
					Name: "two",
				},
			},
		},
		{
			Arn:                 "task2",
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
			KnownStatusUnsafe:   apitaskstatus.TaskRunning,
			Family:              "test",
			Version:             "2",
			Containers: []*apicontainer.Container{
				{
					Name: "foo",
				},
			},
		},
		{
			Arn:                 "byShortId",
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
			KnownStatusUnsafe:   apitaskstatus.TaskRunning,
			Family:              "test",
			Version:             "2",
			Containers: []*apicontainer.Container{
				{
					Name: "shortId",
				},
			},
		},
		{
			Arn:                 "awsvpcTask",
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
			KnownStatusUnsafe:   apitaskstatus.TaskRunning,
			Family:              "test",
			Version:             "1",
			Containers: []*apicontainer.Container{
				{
					Name: "awsvpc",
				},
			},
			ENIs: []*ni.NetworkInterface{
				{
					IPV4Addresses: []*ni.IPV4Address{
						{
							Address: ipv4Address,
						},
					},
				},
			},
		},
		{
			Arn:                 "hostModeNetworkingTask",
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
			KnownStatusUnsafe:   apitaskstatus.TaskRunning,
			Family:              "test",
			Version:             "1",
			Containers: []*apicontainer.Container{
				{
					Name: "awsvpc",
					Ports: []apicontainer.PortBinding{
						{
							ContainerPort: 80,
							HostPort:      80,
							Protocol:      apicontainer.TransportProtocolTCP,
						},
					},
				},
			},
		},
		{
			Arn:                 "bridgeModeNetworkingTask",
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
			KnownStatusUnsafe:   apitaskstatus.TaskRunning,
			Family:              "test",
			Version:             "1",
			Containers: []*apicontainer.Container{
				{
					Name: "awsvpc",
					KnownPortBindingsUnsafe: []apicontainer.PortBinding{
						{
							ContainerPort: 80,
							HostPort:      80,
							Protocol:      apicontainer.TransportProtocolTCP,
						},
					},
				},
			},
		},
	}
}

func stateSetupHelper(state dockerstate.TaskEngineState, tasks []*apitask.Task) {
	for _, task := range tasks {
		state.AddTask(task)
		for _, container := range task.Containers {
			state.AddContainer(&apicontainer.DockerContainer{
				Container:  container,
				DockerID:   "dockerid-" + task.Arn + "-" + container.Name,
				DockerName: "dockername-" + task.Arn + "-" + container.Name,
			}, task)
		}
	}
}

type inspecIntegConfig struct {
	state          *v1.AgentStateImpl
	metricsFactory metrics.EntryFactory
}

func (test inspecIntegConfig) performRequest(t *testing.T, path string, options ...introspection.ConfigOpt) *http.Response {
	var waitForServer = func(client *http.Client, serverAddress string) error {
		var err error
		// wait for the server to come up
		for i := 0; i < 10; i++ {
			time.Sleep(100 * time.Millisecond)
			_, err = client.Get(serverAddress)
			if err == nil {
				return nil // server is up now
			}
		}
		return fmt.Errorf("timed out waiting for server %s to come up: %w", serverAddress, err)
	}
	var startServer = func(t *testing.T, server *http.Server) int {
		server.Addr = ":0"
		listener, err := net.Listen("tcp", server.Addr)
		assert.NoError(t, err)

		port := listener.Addr().(*net.TCPAddr).Port
		t.Logf("Server started on port: %d", port)

		go func() {
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				t.Logf("ListenAndServe(): %s\n", err)
			}
		}()

		return port
	}

	server, err := introspection.NewServer(test.state, test.metricsFactory, options...)

	port := startServer(t, server)
	defer server.Close()

	serverAddress := fmt.Sprintf("http://localhost:%d", port)

	client := http.DefaultClient
	err = waitForServer(client, serverAddress)
	assert.NoError(t, err)

	response, err := client.Get(fmt.Sprintf("%s%s", serverAddress, path))
	assert.NoError(t, err)

	return response
}

func unmarshalResponse(t *testing.T, response *http.Response, v any) {
	bodyBytes, err := io.ReadAll(response.Body)
	assert.NoError(t, err)

	err = json.Unmarshal(bodyBytes, v)
	if err != nil {
		t.Fatal(err)
	}
}
