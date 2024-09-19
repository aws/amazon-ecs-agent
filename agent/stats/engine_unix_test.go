//go:build linux && unit
// +build linux,unit

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
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testPublishMetricsInterval = 5 * time.Second
)

func TestLinuxTaskNetworkStatsSet(t *testing.T) {
	var networkModes = []struct {
		ENIs        []*ni.NetworkInterface
		NetworkMode string
		StatsEmpty  bool
	}{
		{[]*ni.NetworkInterface{{ID: "ec2Id"}}, "awsvpc", true},
		{nil, "host", true},
		{nil, "bridge", false},
		{nil, "none", true},
	}
	for _, tc := range networkModes {
		testNetworkModeStats(t, tc.NetworkMode, tc.ENIs, false, tc.StatsEmpty)
	}
}

func TestNetworkModeStatsAWSVPCMode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	mockDockerClient := mock_dockerapi.NewMockDockerClient(mockCtrl)

	testContainer := &apicontainer.DockerContainer{
		Container: &apicontainer.Container{
			Name: "test",
			Type: apicontainer.ContainerCNIPause,
		},
	}

	testContainer1 := &apicontainer.DockerContainer{
		Container: &apicontainer.Container{
			Name: "test1",
		},
	}

	t1 := getTestTask()

	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveTask("c2").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(t1, nil).AnyTimes()

	resolver.EXPECT().ResolveContainer("c1").AnyTimes().Return(testContainer, nil)
	resolver.EXPECT().ResolveContainer("c2").AnyTimes().Return(testContainer1, nil)
	mockDockerClient.EXPECT().Stats(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	mockDockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    "test",
			State: &types.ContainerState{Pid: 23},
		},
	}, nil).AnyTimes()
	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestTaskNetworkStatsSet"), nil, nil, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.resolver = resolver
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance
	engine.client = mockDockerClient
	engine.addAndStartStatsContainer("c1")
	engine.addAndStartStatsContainer("c2")
	ts1 := parseNanoTime("2015-02-12T21:22:05.131117533Z")
	containerStats := createFakeContainerStats()
	dockerStats := []*types.StatsJSON{{}, {}}
	dockerStats[0].Read = ts1
	containers, _ := engine.tasksToContainers["t1"]
	taskContainers, _ := engine.taskToTaskStats["t1"]
	for _, statsContainer := range containers {
		for i := 0; i < 2; i++ {
			statsContainer.statsQueue.add(containerStats[i])
			statsContainer.statsQueue.setLastStat(dockerStats[i])
			taskContainers.StatsQueue.add(containerStats[i])
		}
	}
	_, taskMetrics, err := engine.GetInstanceMetrics(false)
	assert.NoError(t, err)
	assert.Len(t, taskMetrics, 1)
	for _, containerMetric := range taskMetrics[0].ContainerMetrics {
		if *containerMetric.ContainerName == "test1" {
			assert.NotNil(t, containerMetric.NetworkStatsSet, "network stats should be non-empty")
		} else {
			assert.Nil(t, containerMetric.NetworkStatsSet, "network stats should be empty for pause")
		}
	}
}

func TestServiceConnectWithDisabledMetrics(t *testing.T) {
	disableMetricsConfig := cfg
	disableMetricsConfig.DisableMetrics = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	containerID := "containerID"
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	container := apicontainer.Container{
		Name:            "service-connect",
		HealthCheckType: "docker",
	}
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	t1 := getTestTask()
	t1.ServiceConnectConfig = &serviceconnect.Config{
		ContainerName: "service-connect",
	}
	t1.Containers = []*apicontainer.Container{&container}
	resolver.EXPECT().ResolveTask(containerID).Return(t1, nil)
	resolver.EXPECT().ResolveContainer(containerID).Return(&apicontainer.DockerContainer{
		DockerID:  containerID,
		Container: &container,
	}, nil).Times(2)

	engine := NewDockerStatsEngine(&disableMetricsConfig, nil, eventStream("TestServiceConnectWithDisabledMetrics"), nil, nil, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.resolver = resolver
	engine.addAndStartStatsContainer(containerID)

	assert.Len(t, engine.tasksToContainers, 0, "No containers should be tracked if metrics is disabled")
	assert.Len(t, engine.tasksToHealthCheckContainers, 1)
	assert.Len(t, engine.taskToServiceConnectStats, 1)
}

// This test has been moved to be run Linux only. For Windows, the publish metrics timeout has been set to be 5 seconds
// for the reason that Windows takes a bit more time to fetch the volume metrics. This test results in a race condition
// for Windows. Specifically, the discard message condition cannot be replicated in Windows because of the higher
// timeout.
func TestStartMetricsPublishForChannelFull(t *testing.T) {
	testcases := []struct {
		name                       string
		hasPublishTicker           bool
		expectedInstanceMessageNum int
		expectedHealthMessageNum   int
		expectedNonEmptyMetricsMsg bool
		serviceConnectEnabled      bool
		disableMetrics             bool
		channelSize                int
	}{
		{
			name:                       "ChannelFull",
			hasPublishTicker:           true,
			expectedInstanceMessageNum: 1, // expecting discarding messages after channel is full
			expectedHealthMessageNum:   1,
			expectedNonEmptyMetricsMsg: true,
			serviceConnectEnabled:      false,
			disableMetrics:             false,
			channelSize:                testTelemetryChannelBufferSizeForChannelFull,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			publishMetricsCfg := cfg
			if tc.disableMetrics {
				publishMetricsCfg.DisableMetrics = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
			}

			containerID := "c1"
			t1 := &apitask.Task{
				Arn:               "t1",
				Family:            "f1",
				KnownStatusUnsafe: apitaskstatus.TaskRunning,
				Containers: []*apicontainer.Container{
					{Name: containerID},
				},
			}

			mockDockerClient := mock_dockerapi.NewMockDockerClient(mockCtrl)
			resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)

			mockDockerClient.EXPECT().Stats(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			mockDockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:    containerID,
					State: &types.ContainerState{Pid: 23},
				},
			}, nil).AnyTimes()

			resolver.EXPECT().ResolveTask(containerID).AnyTimes().Return(t1, nil)
			resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(t1, nil).AnyTimes()
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
			}, nil).AnyTimes()

			telemetryMessages := make(chan ecstcs.TelemetryMessage, tc.channelSize)
			healthMessages := make(chan ecstcs.HealthMessage, tc.channelSize)

			engine := NewDockerStatsEngine(&publishMetricsCfg, nil, eventStream("TestStartMetricsPublish"), telemetryMessages, healthMessages, nil)
			ctx, cancel := context.WithCancel(context.TODO())
			engine.ctx = ctx
			engine.resolver = resolver
			engine.cluster = defaultCluster
			engine.containerInstanceArn = defaultContainerInstance
			engine.client = mockDockerClient
			ticker := time.NewTicker(testPublishMetricsInterval)
			if !tc.hasPublishTicker {
				ticker = nil
			}
			engine.publishMetricsTicker = ticker

			engine.addAndStartStatsContainer(containerID)
			ts1 := parseNanoTime("2015-02-12T21:22:05.131117533Z")

			containerStats := createFakeContainerStats()
			dockerStats := []*types.StatsJSON{{}, {}}
			dockerStats[0].Read = ts1
			containers, _ := engine.tasksToContainers["t1"]

			// Two docker stats sample can be one CW stats.
			for _, statsContainer := range containers {
				for i := 0; i < 2; i++ {
					statsContainer.statsQueue.add(containerStats[i])
					statsContainer.statsQueue.setLastStat(dockerStats[i])
				}
			}

			go engine.StartMetricsPublish()

			// wait 1s for first set of metrics sent (immediately), and then add a second set of stats
			time.Sleep(time.Second)
			for _, statsContainer := range containers {
				for i := 0; i < 2; i++ {
					statsContainer.statsQueue.add(containerStats[i])
					statsContainer.statsQueue.setLastStat(dockerStats[i])
				}
			}

			time.Sleep(testPublishMetricsInterval + time.Second)

			assert.Len(t, telemetryMessages, tc.expectedInstanceMessageNum)
			assert.Len(t, healthMessages, tc.expectedHealthMessageNum)

			if tc.expectedInstanceMessageNum > 0 {
				telemetryMessage := <-telemetryMessages
				if tc.expectedNonEmptyMetricsMsg {
					assert.NotEmpty(t, telemetryMessage.TaskMetrics)
					assert.NotZero(t, *telemetryMessage.TaskMetrics[0].ContainerMetrics[0].StorageStatsSet.ReadSizeBytes.Sum)
				} else {
					assert.Empty(t, telemetryMessage.TaskMetrics)
				}
			}
			if tc.expectedHealthMessageNum > 0 {
				healthMessage := <-healthMessages
				assert.NotEmpty(t, healthMessage.HealthMetrics)
			}

			// verify full channel behavior: the message is dropped
			if tc.channelSize == testTelemetryChannelBufferSizeForChannelFull {

				// add a third set of metrics. This time, change storageReadBytes to 0 to verify that the 2nd set of metrics
				// are dropped as expected.
				containerStats[0].storageReadBytes = uint64(0)
				containerStats[1].storageReadBytes = uint64(0)
				for _, statsContainer := range containers {
					for i := 0; i < 2; i++ {
						statsContainer.statsQueue.add(containerStats[i])
						statsContainer.statsQueue.setLastStat(dockerStats[i])
					}
				}

				telemetryMessage := <-telemetryMessages
				healthMessage := <-healthMessages
				assert.NotEmpty(t, telemetryMessage.TaskMetrics)
				assert.NotEmpty(t, healthMessage.HealthMetrics)
				assert.Zero(t, *telemetryMessage.TaskMetrics[0].ContainerMetrics[0].StorageStatsSet.ReadSizeBytes.Sum)
			}

			cancel()
			if ticker != nil {
				ticker.Stop()
			}
			close(telemetryMessages)
			close(healthMessages)
		})
	}
}
