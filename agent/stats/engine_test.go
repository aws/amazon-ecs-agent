//go:build unit
// +build unit

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

package stats

import (
	"context"
	"fmt"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsEngineAddRemoveContainers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)
	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	t1 := &apitask.Task{Arn: "t1", Family: "f1"}
	t2 := &apitask.Task{Arn: "t2", Family: "f2"}
	t3 := &apitask.Task{Arn: "t3"}
	name := "testContainer"
	networkMode := "bridge"
	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveTask("c2").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveTask("c3").AnyTimes().Return(t2, nil)
	resolver.EXPECT().ResolveTask("c4").AnyTimes().Return(nil, fmt.Errorf("unmapped container"))
	resolver.EXPECT().ResolveTask("c5").AnyTimes().Return(t2, nil)
	resolver.EXPECT().ResolveTask("c6").AnyTimes().Return(t3, nil)
	resolver.EXPECT().ResolveContainer(gomock.Any()).AnyTimes().Return(&apicontainer.DockerContainer{
		Container: &apicontainer.Container{
			Name:              name,
			NetworkModeUnsafe: networkMode,
		},
	}, nil)
	mockStatsChannel := make(chan *types.StatsJSON)
	defer close(mockStatsChannel)
	mockDockerClient.EXPECT().Stats(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockStatsChannel, nil).AnyTimes()

	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestStatsEngineAddRemoveContainers"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.resolver = resolver
	engine.client = mockDockerClient
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance
	defer engine.removeAll()

	engine.addAndStartStatsContainer("c1")
	engine.addAndStartStatsContainer("c1")

	if len(engine.tasksToContainers) != 1 {
		t.Errorf("Adding containers failed. Expected num tasks = 1, got: %d", len(engine.tasksToContainers))
	}

	containers, _ := engine.tasksToContainers["t1"]
	if len(containers) != 1 {
		t.Error("Adding duplicate containers failed.")
	}
	_, exists := containers["c1"]
	if !exists {
		t.Error("Container c1 not found in engine")
	}

	engine.addAndStartStatsContainer("c2")
	containers, _ = engine.tasksToContainers["t1"]
	_, exists = containers["c2"]
	if !exists {
		t.Error("Container c2 not found in engine")
	}

	for _, statsContainer := range containers {
		assert.Equal(t, name, statsContainer.containerMetadata.Name)
		assert.Equal(t, networkMode, statsContainer.containerMetadata.NetworkMode)
		for _, fakeContainerStats := range createFakeContainerStats() {
			statsContainer.statsQueue.add(fakeContainerStats)
		}
	}

	// Ensure task shows up in metrics.
	containerMetrics, err := engine.taskContainerMetricsUnsafe("t1")
	if err != nil {
		t.Errorf("Error getting container metrics: %v", err)
	}
	err = validateContainerMetrics(containerMetrics, 2)
	if err != nil {
		t.Errorf("Error validating container metrics: %v", err)
	}

	metadata, taskMetrics, err := engine.GetInstanceMetrics(false)
	if err != nil {
		t.Errorf("Error gettting instance metrics: %v", err)
	}

	err = validateMetricsMetadata(metadata)
	require.NoError(t, err)
	require.Len(t, taskMetrics, 1, "Incorrect number of tasks.")
	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 2)
	require.NoError(t, err)
	require.Equal(t, "t1", *taskMetrics[0].TaskArn)

	for _, statsContainer := range containers {
		assert.Equal(t, name, statsContainer.containerMetadata.Name)
		assert.Equal(t, networkMode, statsContainer.containerMetadata.NetworkMode)
		for _, fakeContainerStats := range createFakeContainerStats() {
			statsContainer.statsQueue.add(fakeContainerStats)
		}
	}

	// Ensure task shows up in metrics.
	containerMetrics, err = engine.taskContainerMetricsUnsafe("t1")
	if err != nil {
		t.Errorf("Error getting container metrics: %v", err)
	}
	err = validateContainerMetrics(containerMetrics, 2)
	if err != nil {
		t.Errorf("Error validating container metrics: %v", err)
	}

	metadata, taskMetrics, err = engine.GetInstanceMetrics(true)
	if err != nil {
		t.Errorf("Error gettting instance metrics: %v", err)
	}

	err = validateMetricsMetadata(metadata)
	require.NoError(t, err)
	require.Len(t, taskMetrics, 1, "Incorrect number of tasks.")
	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 2)
	require.NoError(t, err)
	require.Equal(t, "t1", *taskMetrics[0].TaskArn)

	// Ensure that only valid task shows up in metrics.
	_, err = engine.taskContainerMetricsUnsafe("t2")
	if err == nil {
		t.Error("Expected non-empty error for non existent task")
	}

	engine.removeContainer("c1")
	containers, _ = engine.tasksToContainers["t1"]
	_, exists = containers["c1"]
	if exists {
		t.Error("Container c1 not removed from engine")
	}
	engine.removeContainer("c2")
	containers, _ = engine.tasksToContainers["t1"]
	_, exists = containers["c2"]
	if exists {
		t.Error("Container c2 not removed from engine")
	}
	engine.addAndStartStatsContainer("c3")
	containers, _ = engine.tasksToContainers["t2"]
	_, exists = containers["c3"]
	if !exists {
		t.Error("Container c3 not found in engine")
	}

	_, _, err = engine.GetInstanceMetrics(false)
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
	engine.removeContainer("c3")

	// Should get an error while adding this container due to unmapped
	// container to task.
	engine.addAndStartStatsContainer("c4")
	validateIdleContainerMetrics(t, engine)

	// Should get an error while adding this container due to unmapped
	// task arn to task definition family.
	engine.addAndStartStatsContainer("c6")
	validateIdleContainerMetrics(t, engine)
}

func TestStatsEngineMetadataInStatsSets(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	mockDockerClient := mock_dockerapi.NewMockDockerClient(mockCtrl)
	t1 := &apitask.Task{Arn: "t1", Family: "f1"}
	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveContainer(gomock.Any()).AnyTimes().Return(&apicontainer.DockerContainer{
		Container: &apicontainer.Container{
			Name: "test",
		},
	}, nil)
	mockDockerClient.EXPECT().Stats(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(t1, nil).AnyTimes()

	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestStatsEngineMetadataInStatsSets"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.resolver = resolver
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance
	engine.client = mockDockerClient
	engine.addAndStartStatsContainer("c1")
	ts1 := parseNanoTime("2015-02-12T21:22:05.131117533Z")
	ts2 := parseNanoTime("2015-02-12T21:22:05.232291187Z")
	containerStats := createFakeContainerStats()
	dockerStats := []*types.StatsJSON{{}, {}}
	dockerStats[0].Read = ts1
	dockerStats[1].Read = ts2
	containers, _ := engine.tasksToContainers["t1"]
	for _, statsContainer := range containers {
		for i := 0; i < 2; i++ {
			statsContainer.statsQueue.add(containerStats[i])
			statsContainer.statsQueue.setLastStat(dockerStats[i])
		}
	}
	metadata, taskMetrics, err := engine.GetInstanceMetrics(false)
	if err != nil {
		t.Errorf("Error gettting instance metrics: %v", err)
	}
	if len(taskMetrics) != 1 {
		t.Fatalf("Incorrect number of tasks. Expected: 1, got: %d", len(taskMetrics))
	}
	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 1)
	if err != nil {
		t.Errorf("Error validating container metrics: %v", err)
	}
	if *taskMetrics[0].TaskArn != "t1" {
		t.Errorf("Incorrect task arn. Expected: t1, got: %s", *taskMetrics[0].TaskArn)
	}
	err = validateMetricsMetadata(metadata)
	if err != nil {
		t.Errorf("Error validating metadata: %v", err)
	}

	dockerStat, _, err := engine.ContainerDockerStats("t1", "c1")
	assert.NoError(t, err)
	assert.Equal(t, ts2, dockerStat.Read)

	engine.removeContainer("c1")
	validateIdleContainerMetrics(t, engine)
}

func TestStatsEngineInvalidTaskEngine(t *testing.T) {
	statsEngine := NewDockerStatsEngine(&cfg, nil, eventStream("TestStatsEngineInvalidTaskEngine"))
	taskEngine := &MockTaskEngine{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := statsEngine.MustInit(ctx, taskEngine, "", "")
	if err == nil {
		t.Error("Expected error in engine initialization, got nil")
	}
}

func TestStatsEngineUninitialized(t *testing.T) {
	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestStatsEngineUninitialized"))
	defer engine.removeAll()

	engine.resolver = &DockerContainerMetadataResolver{}
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance
	engine.addAndStartStatsContainer("c1")
	validateIdleContainerMetrics(t, engine)
}

func TestStatsEngineTerminalTask(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	resolver.EXPECT().ResolveTask("c1").Return(&apitask.Task{
		Arn:               "t1",
		KnownStatusUnsafe: apitaskstatus.TaskStopped,
		Family:            "f1",
	}, nil)
	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestStatsEngineTerminalTask"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	defer engine.removeAll()

	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance
	engine.resolver = resolver

	engine.addAndStartStatsContainer("c1")
	validateIdleContainerMetrics(t, engine)
}

func TestGetTaskHealthMetrics(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	containerID := "containerID"
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	resolver.EXPECT().ResolveContainer(containerID).Return(&apicontainer.DockerContainer{
		DockerID: containerID,
		Container: &apicontainer.Container{
			KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			HealthCheckType:   "docker",
			Health: apicontainer.HealthStatus{
				Status: apicontainerstatus.ContainerHealthy,
				Since:  aws.Time(time.Now()),
			},
		},
	}, nil).Times(3)

	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestGetTaskHealthMetrics"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.containerInstanceArn = "container_instance"

	containerToStats := make(map[string]*StatsContainer)
	var err error
	containerToStats[containerID], err = newStatsContainer(containerID, nil, resolver, nil)
	assert.NoError(t, err)
	engine.tasksToHealthCheckContainers["t1"] = containerToStats
	engine.tasksToDefinitions["t1"] = &taskDefinition{
		family:  "f1",
		version: "1",
	}

	engine.resolver = resolver
	metadata, taskHealth, err := engine.GetTaskHealthMetrics()
	assert.NoError(t, err)

	assert.Equal(t, aws.StringValue(metadata.ContainerInstance), "container_instance")
	assert.Len(t, taskHealth, 1)
	assert.Len(t, taskHealth[0].Containers, 1)
	assert.Equal(t, aws.StringValue(taskHealth[0].Containers[0].HealthStatus), "HEALTHY")
}

func TestGetTaskHealthMetricsStoppedContainer(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	containerID := "containerID"
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	resolver.EXPECT().ResolveContainer(containerID).Return(&apicontainer.DockerContainer{
		DockerID: containerID,
		Container: &apicontainer.Container{
			KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
			HealthCheckType:   "docker",
			Health: apicontainer.HealthStatus{
				Status: apicontainerstatus.ContainerHealthy,
				Since:  aws.Time(time.Now()),
			},
		},
	}, nil).Times(2)

	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestGetTaskHealthMetrics"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.containerInstanceArn = "container_instance"

	containerToStats := make(map[string]*StatsContainer)
	var err error
	containerToStats[containerID], err = newStatsContainer(containerID, nil, resolver, nil)
	assert.NoError(t, err)
	engine.tasksToHealthCheckContainers["t1"] = containerToStats
	engine.tasksToDefinitions["t1"] = &taskDefinition{
		family:  "f1",
		version: "1",
	}

	engine.resolver = resolver
	_, _, err = engine.GetTaskHealthMetrics()
	assert.Error(t, err, "empty metrics should cause an error")
}

// TestMetricsDisabled tests container won't call docker api to collect stats
// but will track container health when metrics is disabled in agent.
func TestMetricsDisabled(t *testing.T) {
	disableMetricsConfig := cfg
	disableMetricsConfig.DisableMetrics = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}

	containerID := "containerID"
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	resolver.EXPECT().ResolveTask(containerID).Return(&apitask.Task{
		Arn:               "t1",
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
		Family:            "f1",
	}, nil)
	resolver.EXPECT().ResolveContainer(containerID).Return(&apicontainer.DockerContainer{
		DockerID: containerID,
		Container: &apicontainer.Container{
			HealthCheckType: "docker",
		},
	}, nil).Times(2)

	engine := NewDockerStatsEngine(&disableMetricsConfig, nil, eventStream("TestMetricsDisabled"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.resolver = resolver
	engine.addAndStartStatsContainer(containerID)

	assert.Len(t, engine.tasksToContainers, 0, "No containers should be tracked if metrics is disabled")
	assert.Len(t, engine.tasksToHealthCheckContainers, 1)
}

func TestSynchronizeOnRestart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	containerID := "containerID"
	statsChan := make(chan *types.StatsJSON)
	statsStarted := make(chan struct{})
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)

	engine := NewDockerStatsEngine(&cfg, client, eventStream("TestSynchronizeOnRestart"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.resolver = resolver

	client.EXPECT().ListContainers(gomock.Any(), false, gomock.Any()).Return(dockerapi.ListContainersResponse{
		DockerIDs: []string{containerID},
	})
	client.EXPECT().Stats(gomock.Any(), containerID, gomock.Any()).Do(func(ctx context.Context, id string, inactivityTimeout time.Duration) {
		statsStarted <- struct{}{}
	}).Return(statsChan, nil)

	testTask := &apitask.Task{
		Arn:               "t1",
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
		Family:            "f1",
	}

	resolver.EXPECT().ResolveTask(containerID).Return(testTask, nil).Times(2)
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(testTask, nil).AnyTimes()
	resolver.EXPECT().ResolveContainer(containerID).Return(&apicontainer.DockerContainer{
		DockerID: containerID,
		Container: &apicontainer.Container{
			HealthCheckType: "docker",
		},
	}, nil).Times(3)
	err := engine.synchronizeState()
	assert.NoError(t, err)

	assert.Len(t, engine.tasksToContainers, 1)
	assert.Len(t, engine.tasksToHealthCheckContainers, 1)

	statsContainer := engine.tasksToContainers["t1"][containerID]
	assert.NotNil(t, statsContainer)
	<-statsStarted
	statsContainer.StopStatsCollection()
}

func TestTaskNetworkStatsSet(t *testing.T) {
	var networkModes = []struct {
		ENIs        []*apieni.ENI
		NetworkMode string
		StatsEmpty  bool
	}{
		{nil, "default", false},
	}
	for _, tc := range networkModes {
		testNetworkModeStats(t, tc.NetworkMode, tc.ENIs, tc.StatsEmpty)
	}
}

func testNetworkModeStats(t *testing.T, netMode string, enis []*apieni.ENI, emptyStats bool) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	mockDockerClient := mock_dockerapi.NewMockDockerClient(mockCtrl)

	testContainer := &apicontainer.DockerContainer{
		Container: &apicontainer.Container{
			Name:              "test",
			NetworkModeUnsafe: netMode,
			Type:              apicontainer.ContainerCNIPause,
		},
	}

	t1 := &apitask.Task{
		Arn:               "t1",
		Family:            "f1",
		ENIs:              enis,
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			{Name: "test"},
			{Name: "test1"},
		},
	}

	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(t1, nil).AnyTimes()

	resolver.EXPECT().ResolveContainer(gomock.Any()).AnyTimes().Return(testContainer, nil)
	mockDockerClient.EXPECT().Stats(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	mockDockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    "test",
			State: &types.ContainerState{Pid: 23},
		},
	}, nil).AnyTimes()
	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestTaskNetworkStatsSet"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.resolver = resolver
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance
	engine.client = mockDockerClient
	engine.addAndStartStatsContainer("c1")
	ts1 := parseNanoTime("2015-02-12T21:22:05.131117533Z")
	containerStats := createFakeContainerStats()
	dockerStats := []*types.StatsJSON{{}, {}}
	dockerStats[0].Read = ts1
	containers, _ := engine.tasksToContainers["t1"]
	for _, statsContainer := range containers {
		for i := 0; i < 2; i++ {
			statsContainer.statsQueue.add(containerStats[i])
			statsContainer.statsQueue.setLastStat(dockerStats[i])
		}
	}
	_, taskMetrics, err := engine.GetInstanceMetrics(false)
	assert.NoError(t, err)
	assert.Len(t, taskMetrics, 1)
	for _, containerMetric := range taskMetrics[0].ContainerMetrics {
		if emptyStats {
			assert.Nil(t, containerMetric.NetworkStatsSet, "network stats should be empty")
		} else {
			assert.NotNil(t, containerMetric.NetworkStatsSet, "network stats should be non-empty")
		}
	}
}
