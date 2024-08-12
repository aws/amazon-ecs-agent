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
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/container/restart"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

type StatTestData struct {
	timestamp time.Time
	cpuTime   uint64
	memBytes  uint64
}

var statsData = []*StatTestData{
	{parseNanoTime("2015-02-12T21:22:05.131117533Z"), 22400432, 1839104},
	{parseNanoTime("2015-02-12T21:22:05.232291187Z"), 116499979, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.333776335Z"), 248503503, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.434753595Z"), 372167097, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.535746779Z"), 502862518, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.638709495Z"), 638485801, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.739985398Z"), 780707806, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.840941705Z"), 911624529, 3649536},
}

func getTestTask() *apitask.Task {
	return &apitask.Task{
		Arn:               "t1",
		Family:            "f1",
		ENIs:              []*ni.NetworkInterface{{ID: "ec2Id"}},
		NetworkMode:       apitask.AWSVPCNetworkMode,
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			{
				Name:      "test1",
				RuntimeID: "container1",
			},
			{
				Name:      "test2",
				RuntimeID: "container2",
			},
		},
	}
}

func TestContainerStatsCollection(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	dockerID := "container1"
	ctx, cancel := context.WithCancel(context.TODO())
	statChan := make(chan *types.StatsJSON)
	errC := make(chan error)
	numStats := 8
	mockDockerClient.EXPECT().Stats(ctx, dockerID, dockerclient.StatsInactivityTimeout).Return(statChan, errC)
	go metricSenderFunc(statChan, numStats, nil)()

	container := &StatsContainer{
		containerMetadata: &ContainerMetadata{
			DockerID: dockerID,
		},
		ctx:    ctx,
		cancel: cancel,
		client: mockDockerClient,
	}
	container.StartStatsCollection()
	time.Sleep(checkPointSleep)
	container.StopStatsCollection()
	cpuStatsSet, err := container.statsQueue.GetCPUStatsSet()
	if err != nil {
		t.Fatal("Error gettting cpu stats set:", err)
	}
	if *cpuStatsSet.Min == math.MaxFloat64 || math.IsNaN(*cpuStatsSet.Min) {
		t.Error("Min value incorrectly set: ", *cpuStatsSet.Min)
	}
	if *cpuStatsSet.Max == -math.MaxFloat64 || math.IsNaN(*cpuStatsSet.Max) {
		t.Error("Max value incorrectly set: ", *cpuStatsSet.Max)
	}
	if *cpuStatsSet.SampleCount == 0 {
		t.Error("Samplecount is 0")
	}
	if *cpuStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *cpuStatsSet.Sum)
	}

	memStatsSet, err := container.statsQueue.GetMemoryStatsSet()
	if err != nil {
		t.Error("Error gettting cpu stats set:", err)
	}
	if *memStatsSet.Min == math.MaxFloat64 {
		t.Error("Min value incorrectly set: ", *memStatsSet.Min)
	}
	if *memStatsSet.Max == 0 {
		t.Error("Max value incorrectly set: ", *memStatsSet.Max)
	}
	if *memStatsSet.SampleCount == 0 {
		t.Error("Samplecount is 0")
	}
	if *memStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *memStatsSet.Sum)
	}

	restartStatSet, err := container.statsQueue.GetRestartStatsSet()
	require.Error(t, err, "Expect no restart stats set for container without a restart policy")
	require.Nil(t, restartStatSet, "Expect nil restart stats set for container without a restart policy")
}

func TestGetNonDockerContainerStats(t *testing.T) {
	restartPolicy := restart.RestartPolicy{Enabled: true}
	restartTracker := restart.NewRestartTracker(restartPolicy)
	container := &apicontainer.Container{
		RestartPolicy:  &restartPolicy,
		RestartTracker: restartTracker,
	}
	nonDockerStats := getNonDockerContainerStats(container)
	require.NotNil(t, nonDockerStats.restartCount)
	require.Equal(t, int64(0), *nonDockerStats.restartCount)

	restartTracker.RecordRestart()
	nonDockerStats = getNonDockerContainerStats(container)
	require.NotNil(t, nonDockerStats.restartCount)
	require.Equal(t, int64(1), *nonDockerStats.restartCount)

	for i := 0; i < 10; i++ {
		restartTracker.RecordRestart()
	}
	nonDockerStats = getNonDockerContainerStats(container)
	require.NotNil(t, nonDockerStats.restartCount)
	require.Equal(t, int64(11), *nonDockerStats.restartCount)
}

func TestGetNonDockerContainerStats_ExpectNil(t *testing.T) {
	// disabled policy returns nil restart count
	restartPolicy := restart.RestartPolicy{Enabled: false}
	restartTracker := restart.NewRestartTracker(restartPolicy)
	container1 := &apicontainer.Container{
		RestartPolicy:  &restartPolicy,
		RestartTracker: restartTracker,
	}
	nonDockerStats := getNonDockerContainerStats(container1)
	require.Nil(t, nonDockerStats.restartCount)

	// no policy returns nil restart count
	container2 := &apicontainer.Container{}
	nonDockerStats = getNonDockerContainerStats(container2)
	require.Nil(t, nonDockerStats.restartCount)
}

func TestContainerStatsCollection_WithRestartPolicy(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)

	dockerID := "container1"
	ctx, cancel := context.WithCancel(context.TODO())
	statChan := make(chan *types.StatsJSON)
	errC := make(chan error)
	numStatsPreRestart := 8
	numStatsPostRestart := 5
	totalNumStats := numStatsPreRestart + numStatsPostRestart

	container := &StatsContainer{
		containerMetadata: &ContainerMetadata{
			DockerID: dockerID,
		},
		ctx:      ctx,
		cancel:   cancel,
		client:   mockDockerClient,
		resolver: resolver,
	}
	t1 := getTestTask()
	restartPolicy := restart.RestartPolicy{Enabled: true}
	restartTracker := restart.NewRestartTracker(restartPolicy)

	mockContainer := &apicontainer.DockerContainer{
		DockerID: dockerID,
		Container: &apicontainer.Container{
			KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			RestartPolicy:     &restart.RestartPolicy{Enabled: true},
			RestartTracker:    restartTracker,
		},
	}
	mockDockerClient.EXPECT().Stats(ctx, dockerID, dockerclient.StatsInactivityTimeout).Return(statChan, errC).AnyTimes()
	mockDockerClient.EXPECT().DescribeContainer(ctx, dockerID).Times(totalNumStats)
	resolver.EXPECT().ResolveTask(dockerID).Return(t1, nil).AnyTimes()
	resolver.EXPECT().ResolveContainer(dockerID).Return(mockContainer, nil).AnyTimes()
	container.StartStatsCollection()
	go metricSenderFunc(statChan, numStatsPreRestart, restartTracker)()
	time.Sleep(checkPointSleep)

	restartStatSet, err := container.statsQueue.GetRestartStatsSet()
	require.NoError(t, err)
	require.Equal(t, int64(numStatsPreRestart), *restartStatSet.RestartCount)
	// Reset sets all of the existing stats to "sent" status in the stats queue
	container.statsQueue.Reset()

	go metricSenderFunc(statChan, numStatsPostRestart, restartTracker)()
	time.Sleep(checkPointSleep)
	restartStatSet, err = container.statsQueue.GetRestartStatsSet()
	require.NoError(t, err)
	// at this point the raw restart count will be totalNumStats, and the GetRestartStatsSet should
	// subtract the numStatsPreRestart earlier restarts that were already counted, so that we send numStatsPostRestart
	// restarts to TCS.
	require.Equal(t, totalNumStats, restartTracker.GetRestartCount(), fmt.Sprintf(
		"Raw restart count should be %d + %d = %d", numStatsPreRestart, numStatsPostRestart, totalNumStats))
	require.Equal(t, int64(numStatsPostRestart), *restartStatSet.RestartCount, fmt.Sprintf(
		"Metric sent to TCS should be %d - %d = %d", totalNumStats, numStatsPreRestart, numStatsPostRestart))
	container.StopStatsCollection()
}

func metricSenderFunc(statChan chan *types.StatsJSON, n int, restartTracker *restart.RestartTracker) func() {
	return func() {
		for i := 0; i < n; i++ {
			stat := statsData[i]
			if restartTracker != nil && i == n-1 {
				// "record restarts" at the end because restarts recorded in the very
				// first metric can get missed.
				for i := 0; i < n; i++ {
					restartTracker.RecordRestart()
				}
			}
			jsonStat := fmt.Sprintf(`
				{
					"memory_stats": {"usage":%d, "privateworkingset":%d},
					"cpu_stats":{
						"cpu_usage":{
							"percpu_usage":[%d],
							"total_usage":%d
						}
					}
				}`, stat.memBytes, stat.memBytes, stat.cpuTime, stat.cpuTime)
			dockerStat := &types.StatsJSON{}
			json.Unmarshal([]byte(jsonStat), dockerStat)
			dockerStat.Read = stat.timestamp
			statChan <- dockerStat
		}
	}
}

func TestContainerStatsCollectionReconnection(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)

	dockerID := "container1"
	ctx, cancel := context.WithCancel(context.TODO())

	statChan := make(chan *types.StatsJSON)
	errChan := make(chan error)
	go func() { errChan <- fmt.Errorf("test error") }()
	closedChan := make(chan *types.StatsJSON)
	close(closedChan)

	mockContainer := &apicontainer.DockerContainer{
		DockerID: dockerID,
		Container: &apicontainer.Container{
			KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
		},
	}
	gomock.InOrder(
		mockDockerClient.EXPECT().Stats(ctx, dockerID, dockerclient.StatsInactivityTimeout).Return(closedChan, errChan),
		resolver.EXPECT().ResolveContainer(dockerID).Return(mockContainer, nil).AnyTimes(),
		mockDockerClient.EXPECT().Stats(ctx, dockerID, dockerclient.StatsInactivityTimeout).Return(closedChan, nil),
		resolver.EXPECT().ResolveContainer(dockerID).Return(mockContainer, nil).AnyTimes(),
		mockDockerClient.EXPECT().Stats(ctx, dockerID, dockerclient.StatsInactivityTimeout).Return(statChan, nil),
		resolver.EXPECT().ResolveContainer(dockerID).Return(mockContainer, nil).AnyTimes(),
	)

	container := &StatsContainer{
		containerMetadata: &ContainerMetadata{
			DockerID: dockerID,
		},
		ctx:      ctx,
		cancel:   cancel,
		client:   mockDockerClient,
		resolver: resolver,
	}
	container.StartStatsCollection()
	time.Sleep(checkPointSleep)
	container.StopStatsCollection()
}

func TestContainerStatsCollectionStopsIfContainerIsTerminal(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)

	dockerID := "container1"
	ctx, cancel := context.WithCancel(context.TODO())

	closedChan := make(chan *types.StatsJSON)
	close(closedChan)
	errC := make(chan error)

	statsErr := fmt.Errorf("test error")
	mockContainer := &apicontainer.DockerContainer{
		DockerID: dockerID,
		Container: &apicontainer.Container{
			KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
		},
	}
	gomock.InOrder(
		mockDockerClient.EXPECT().Stats(ctx, dockerID, dockerclient.StatsInactivityTimeout).Return(closedChan, errC),
		resolver.EXPECT().ResolveContainer(dockerID).Return(mockContainer, statsErr).AnyTimes(),
	)

	container := &StatsContainer{
		containerMetadata: &ContainerMetadata{
			DockerID: dockerID,
		},
		ctx:      ctx,
		cancel:   cancel,
		client:   mockDockerClient,
		resolver: resolver,
	}
	container.StartStatsCollection()
	select {
	case <-ctx.Done():
	}
}

func TestSyncContainerRestartAggregationData(t *testing.T) {
	t.Parallel()

	testTime := time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC)
	require.Greater(t, len(statsData), 0)
	jsonStat := fmt.Sprintf(`
				{
					"memory_stats": {"usage":%d, "privateworkingset":%d},
					"cpu_stats":{
						"cpu_usage":{
							"percpu_usage":[%d],
							"total_usage":%d
						}
					}
				}`, statsData[0].memBytes, statsData[0].memBytes, statsData[0].cpuTime, statsData[0].cpuTime)
	dockerStat := &types.StatsJSON{}
	dockerStat.Read = statsData[0].timestamp
	json.Unmarshal([]byte(jsonStat), dockerStat)

	testCases := []struct {
		name                       string
		lastRestartDetectedAt      time.Time
		describeContainerStartedAt time.Time
		expectedDetected           bool
	}{
		{
			name:                       "restart not detected",
			lastRestartDetectedAt:      testTime,
			describeContainerStartedAt: testTime,
			expectedDetected:           false,
		},
		{
			name:                       "restart detected and container has not restarted before",
			lastRestartDetectedAt:      time.Time{},
			describeContainerStartedAt: testTime.Add(1 * time.Minute),
			expectedDetected:           true,
		},
		{
			name:                       "restart detected and container has restarted before",
			lastRestartDetectedAt:      testTime,
			describeContainerStartedAt: testTime.Add(1 * time.Minute),
			expectedDetected:           true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

			dockerID := "containerDockerID"
			ctx, cancel := context.WithCancel(context.TODO())

			statsContainer := &StatsContainer{
				cancel: cancel,
				client: mockDockerClient,
				containerMetadata: &ContainerMetadata{
					DockerID:  dockerID,
					StartedAt: testTime.Add(-1 * time.Minute),
				},
				ctx: ctx,
				restartAggregationData: apicontainer.ContainerRestartAggregationDataForStats{
					LastRestartDetectedAt:     tc.lastRestartDetectedAt,
					LastStatBeforeLastRestart: types.StatsJSON{},
				},
				statsQueue: &Queue{
					lastStat: dockerStat,
				},
			}

			mockDockerClient.EXPECT().DescribeContainer(ctx, dockerID).Return(apicontainerstatus.ContainerRunning,
				dockerapi.DockerContainerMetadata{StartedAt: tc.describeContainerStartedAt})

			restartDetected := statsContainer.syncContainerRestartAggregationData()
			require.Equal(t, tc.expectedDetected, restartDetected)
			if restartDetected {
				require.Greater(t, statsContainer.restartAggregationData.LastRestartDetectedAt,
					tc.lastRestartDetectedAt)
				require.Equal(t, *dockerStat, statsContainer.restartAggregationData.LastStatBeforeLastRestart)
			}
		})
	}
}
