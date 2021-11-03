//go:build linux && unit

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
	"os"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	taskARN = "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc"
)

func TestLinuxTaskNetworkStatsSet(t *testing.T) {
	var networkModes = []struct {
		ENIs        []*apieni.ENI
		NetworkMode string
		StatsEmpty  bool
	}{
		{[]*apieni.ENI{{ID: "ec2Id"}}, "", true},
		{nil, "host", true},
		{nil, "bridge", false},
		{nil, "none", true},
	}
	for _, tc := range networkModes {
		testNetworkModeStats(t, tc.NetworkMode, tc.ENIs, tc.StatsEmpty)
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
		ENIs:              []*apieni.ENI{{ID: "ec2Id"}},
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
	engine := NewDockerStatsEngine(&cfg, nil, eventStream("TestTaskNetworkStatsSet"))
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
	_, taskMetrics, err := engine.GetInstanceMetrics()
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

func TestGetContainerStatusMessage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	testContainer := &StatsContainer{
		containerMetadata: &ContainerMetadata{
			DockerID:        "123",
			Name:            "firelens_v2_container",
			FirelensVersion: "v2",
		},
	}

	engine := NewDockerStatsEngine(&cfg, nil, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	engine.ctx = ctx
	engine.config.DataDirOnHost = os.TempDir()
	engine.dockerIdToStatusMessageSince = map[string]time.Time{}

	testCases := []struct {
		name          string
		statusMessage string
		isNil         bool
		modifyFile    bool
		fileNotExists bool
	}{
		{
			name:          "getContainerStatusMessage returns new message",
			statusMessage: "new status message",
			isNil:         false,
			modifyFile:    true,
		},
		{
			name:          "getContainerStatusMessage returns updated message",
			statusMessage: "updated status message",
			isNil:         false,
			modifyFile:    true,
		},
		{
			name:          "getContainerStatusMessage returns empty string for no update",
			statusMessage: "",
			isNil:         true,
		},
		{
			name:          "getContainerStatusMessage returns empty string when status message file does not exist",
			statusMessage: "",
			isNil:         false,
			fileNotExists: true,
		},
	}

	tempDir := "/tmp/data/telemetry/abc/status-message/firelens_v2_container"
	os.MkdirAll(tempDir, os.ModePerm)
	tempfile, _ := os.Create(tempDir + "/status-message")
	defer os.Remove(tempfile.Name())

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.modifyFile && !tc.fileNotExists {
				tempfile.Truncate(0)
				tempfile.Seek(0, 0)
				time.Sleep(1 * time.Second)
				tempfile.Write([]byte(tc.statusMessage))
			} else if tc.fileNotExists {
				os.Remove(tempfile.Name())
			}
			message, isNil := engine.getContainerStatusMessage(testContainer, taskARN)
			assert.Equal(t, tc.statusMessage, string(message))
			assert.Equal(t, tc.isNil, isNil)
		})
	}
}
