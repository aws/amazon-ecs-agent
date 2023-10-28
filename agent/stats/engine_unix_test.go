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
	"errors"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	apiresource "github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/csiclient"
	mock_csiclient "github.com/aws/amazon-ecs-agent/ecs-agent/csiclient/mocks"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
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

	t1 := &apitask.Task{
		Arn:               "t1",
		Family:            "f1",
		ENIs:              []*ni.NetworkInterface{{ID: "ec2Id"}},
		NetworkMode:       apitask.AWSVPCNetworkMode,
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			{Name: "test"},
			{Name: "test1"},
		},
	}

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
	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestTaskNetworkStatsSet"), nil, nil)
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
	resolver.EXPECT().ResolveTask(containerID).Return(&apitask.Task{
		Arn:               "t1",
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
		Family:            "f1",
		ServiceConnectConfig: &serviceconnect.Config{
			ContainerName: "service-connect",
		},
		Containers: []*apicontainer.Container{&container},
	}, nil)
	resolver.EXPECT().ResolveContainer(containerID).Return(&apicontainer.DockerContainer{
		DockerID:  containerID,
		Container: &container,
	}, nil).Times(2)

	engine := NewDockerStatsEngine(&disableMetricsConfig, nil, eventStream("TestServiceConnectWithDisabledMetrics"), nil, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.resolver = resolver
	engine.addAndStartStatsContainer(containerID)

	assert.Len(t, engine.tasksToContainers, 0, "No containers should be tracked if metrics is disabled")
	assert.Len(t, engine.tasksToHealthCheckContainers, 1)
	assert.Len(t, engine.taskToServiceConnectStats, 1)
}

func TestFetchEBSVolumeMetrics(t *testing.T) {
	tcs := []struct {
		name                    string
		setCSIClientExpectation func(*mock_csiclient.MockCSIClient)
		expectedMetrics         []*ecstcs.VolumeMetric
		numMetrics              int
	}{
		{
			name: "Success",
			setCSIClientExpectation: func(csi *mock_csiclient.MockCSIClient) {
				csi.EXPECT().GetVolumeMetrics(gomock.Any(), "vol-12345", "/mnt/ecs/ebs/taskarn_vol-12345").Return(&csiclient.Metrics{
					Used:     15 * 1024 * 1024 * 1024,
					Capacity: 20 * 1024 * 1024 * 1024,
				}, nil).Times(1)
			},
			expectedMetrics: []*ecstcs.VolumeMetric{
				{
					VolumeId:   aws.String("vol-12345"),
					VolumeName: aws.String("test-volume"),
					Utilized: &ecstcs.UDoubleCWStatsSet{
						Max:         aws.Float64(15 * 1024 * 1024 * 1024),
						Min:         aws.Float64(15 * 1024 * 1024 * 1024),
						SampleCount: aws.Int64(1),
						Sum:         aws.Float64(15 * 1024 * 1024 * 1024),
					},
					Size: &ecstcs.UDoubleCWStatsSet{
						Max:         aws.Float64(20 * 1024 * 1024 * 1024),
						Min:         aws.Float64(20 * 1024 * 1024 * 1024),
						SampleCount: aws.Int64(1),
						Sum:         aws.Float64(20 * 1024 * 1024 * 1024),
					},
				},
			},
			numMetrics: 1,
		},
		{
			name: "Failure",
			setCSIClientExpectation: func(csi *mock_csiclient.MockCSIClient) {
				csi.EXPECT().GetVolumeMetrics(gomock.Any(), "vol-12345", "/mnt/ecs/ebs/taskarn_vol-12345").Return(nil, errors.New("err")).Times(1)
			},
			expectedMetrics: nil,
			numMetrics:      0,
		},
		{
			name: "TimeoutFailure",
			setCSIClientExpectation: func(csi *mock_csiclient.MockCSIClient) {
				csi.EXPECT().GetVolumeMetrics(gomock.Any(), "vol-12345", "/mnt/ecs/ebs/taskarn_vol-12345").Return(nil, errors.New("rpc error: code = DeadlineExceeded desc = context deadline exceeded")).Times(1)
			},
			expectedMetrics: nil,
			numMetrics:      0,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
			mockDockerClient := mock_dockerapi.NewMockDockerClient(mockCtrl)
			t1 := &apitask.Task{
				Arn: "t1",
				Volumes: []apitask.TaskVolume{
					{
						Name: "1",
						Type: apiresource.EBSTaskAttach,
						Volume: &taskresourcevolume.EBSTaskVolumeConfig{
							VolumeId:             "vol-12345",
							VolumeName:           "test-volume",
							VolumeSizeGib:        "10",
							SourceVolumeHostPath: "taskarn_vol-12345",
							DeviceName:           "/dev/nvme1n1",
							FileSystem:           "ext4",
						},
					},
				},
			}
			engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestFetchEBSVolumeMetrics"), nil, nil)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			engine.ctx = ctx
			engine.resolver = resolver
			engine.cluster = defaultCluster
			engine.containerInstanceArn = defaultContainerInstance
			engine.client = mockDockerClient

			mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)
			tc.setCSIClientExpectation(mockCsiClient)
			engine.csiClient = mockCsiClient

			actualMetrics := engine.fetchEBSVolumeMetrics(t1, "t1")

			assert.Len(t, actualMetrics, tc.numMetrics)
			assert.Equal(t, actualMetrics, tc.expectedMetrics)
		})
	}

}
