// +build unit

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

package taskmetadata

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/handlers/types/v2"
	mock_audit "github.com/aws/amazon-ecs-agent/agent/logger/audit/mocks"
	"github.com/aws/amazon-ecs-agent/agent/stats/mock"
	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	clusterName           = "default"
	remoteIP              = "169.254.170.3"
	remotePort            = "32146"
	taskARN               = "t1"
	family                = "sleep"
	version               = "1"
	containerID           = "cid"
	containerName         = "sleepy"
	imageName             = "busybox"
	imageID               = "bUsYbOx"
	cpu                   = 1024
	memory                = 512
	statusRunning         = "RUNNING"
	containerType         = "NORMAL"
	containerPort         = 80
	containerPortProtocol = "tcp"
	eniIPv4Address        = "10.0.0.2"
)

var (
	now  = time.Now()
	task = &apitask.Task{
		Arn:                 taskARN,
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		ENI: &apieni.ENI{
			IPV4Addresses: []*apieni.ENIIPV4Address{
				{
					Address: eniIPv4Address,
				},
			},
		},
		CPU:                      cpu,
		Memory:                   memory,
		PullStartedAtUnsafe:      now,
		PullStoppedAtUnsafe:      now,
		ExecutionStoppedAtUnsafe: now,
	}
	container = &apicontainer.Container{
		Name:                containerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		Ports: []apicontainer.PortBinding{
			{
				ContainerPort: containerPort,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}
	dockerContainer = &apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: containerName,
		Container:  container,
	}
	containerNameToDockerContainer = map[string]*apicontainer.DockerContainer{
		taskARN: dockerContainer,
	}
	labels = map[string]string{
		"foo": "bar",
	}
	expectedContainerResponse = v2.ContainerResponse{
		ID:            containerID,
		Name:          containerName,
		DockerName:    containerName,
		Image:         imageName,
		ImageID:       imageID,
		DesiredStatus: statusRunning,
		KnownStatus:   statusRunning,
		Limits: v2.LimitsResponse{
			CPU:    aws.Float64(cpu),
			Memory: aws.Int64(memory),
		},
		Type:   containerType,
		Labels: labels,
		Ports: []v2.PortResponse{
			{
				ContainerPort: containerPort,
				Protocol:      containerPortProtocol,
				HostPort:      containerPort,
			},
		},
		Networks: []containermetadata.Network{
			{
				NetworkMode:   "awsvpc",
				IPv4Addresses: []string{eniIPv4Address},
			},
		},
	}
	expectedTaskResponse = v2.TaskResponse{
		Cluster:       clusterName,
		TaskARN:       taskARN,
		Family:        family,
		Revision:      version,
		DesiredStatus: statusRunning,
		KnownStatus:   statusRunning,
		Containers:    []v2.ContainerResponse{expectedContainerResponse},
		Limits: &v2.LimitsResponse{
			CPU:    aws.Float64(cpu),
			Memory: aws.Int64(memory),
		},
		PullStartedAt:      aws.Time(now.UTC()),
		PullStoppedAt:      aws.Time(now.UTC()),
		ExecutionStoppedAt: aws.Time(now.UTC()),
	}
)

func init() {
	container.SetLabels(labels)
}

func TestTaskMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)

	gomock.InOrder(
		state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
	)
	server := setupServer(credentials.NewManager(), auditLog, state, clusterName, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", metadataPath, nil)
	req.RemoteAddr = remoteIP + ":" + remotePort
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var taskResponse v2.TaskResponse
	err = json.Unmarshal(res, &taskResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedTaskResponse, taskResponse)
}

func TestContainerMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)

	gomock.InOrder(
		state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
		state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true),
		state.EXPECT().TaskByID(containerID).Return(task, true),
	)
	server := setupServer(credentials.NewManager(), auditLog, state, clusterName, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", metadataPath+"/"+containerID, nil)
	req.RemoteAddr = remoteIP + ":" + remotePort
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var containerResponse v2.ContainerResponse
	err = json.Unmarshal(res, &containerResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedContainerResponse, containerResponse)
}

func TestContainerStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)

	dockerStats := &docker.Stats{NumProcs: 2}
	gomock.InOrder(
		state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
		statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, nil),
	)
	server := setupServer(credentials.NewManager(), auditLog, state, clusterName, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", statsPath+"/"+containerID, nil)
	req.RemoteAddr = remoteIP + ":" + remotePort
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var statsFromResult *docker.Stats
	err = json.Unmarshal(res, &statsFromResult)
	assert.NoError(t, err)
	assert.Equal(t, dockerStats.NumProcs, statsFromResult.NumProcs)
}

func TestTaskStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)
	statsEngine := mock_stats.NewMockEngine(ctrl)

	dockerStats := &docker.Stats{NumProcs: 2}
	containerMap := map[string]*apicontainer.DockerContainer{
		containerName: &apicontainer.DockerContainer{
			DockerID: containerID,
		},
	}
	gomock.InOrder(
		state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerMap, true),
		statsEngine.EXPECT().ContainerDockerStats(taskARN, containerID).Return(dockerStats, nil),
	)
	server := setupServer(credentials.NewManager(), auditLog, state, clusterName, statsEngine,
		config.DefaultTaskMetadataSteadyStateRate, config.DefaultTaskMetadataBurstRate)
	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", statsPath, nil)
	req.RemoteAddr = remoteIP + ":" + remotePort
	server.Handler.ServeHTTP(recorder, req)
	res, err := ioutil.ReadAll(recorder.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var statsFromResult map[string]*docker.Stats
	err = json.Unmarshal(res, &statsFromResult)
	assert.NoError(t, err)
	containerStats, ok := statsFromResult[containerID]
	assert.True(t, ok)
	assert.Equal(t, dockerStats.NumProcs, containerStats.NumProcs)
}
