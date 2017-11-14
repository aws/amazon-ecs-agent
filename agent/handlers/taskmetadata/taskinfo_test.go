// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/handlers/types/v2"
	mock_audit "github.com/aws/amazon-ecs-agent/agent/logger/audit/mocks"
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

func TestTaskMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	auditLog := mock_audit.NewMockAuditLogger(ctrl)

	task := &api.Task{
		Arn:                 taskARN,
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: api.TaskRunning,
		KnownStatusUnsafe:   api.TaskRunning,
		ENI: &api.ENI{
			IPV4Addresses: []*api.ENIIPV4Address{
				{
					Address: eniIPv4Address,
				},
			},
		},
	}
	containerNameToDockerContainer := map[string]*api.DockerContainer{
		taskARN: &api.DockerContainer{
			DockerID:   containerID,
			DockerName: containerName,
			Container: &api.Container{
				Name:                containerName,
				Image:               imageName,
				ImageID:             imageID,
				DesiredStatusUnsafe: api.ContainerRunning,
				KnownStatusUnsafe:   api.ContainerRunning,
				CPU:                 cpu,
				Memory:              memory,
				Type:                api.ContainerNormal,
				Ports: []api.PortBinding{
					{
						ContainerPort: containerPort,
						Protocol:      api.TransportProtocolTCP,
					},
				},
			},
		},
	}
	labels := map[string]string{
		"foo": "bar",
	}
	gomock.InOrder(
		state.EXPECT().GetTaskByIPAddress(remoteIP).Return(taskARN, true),
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
		state.EXPECT().GetLabels(containerID).Return(labels, true),
	)
	server := setupServer(credentials.NewManager(), auditLog, state, clusterName)
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
	expectedTaskResponse := v2.TaskResponse{
		Cluster:       clusterName,
		TaskARN:       taskARN,
		Family:        family,
		Version:       version,
		DesiredStatus: statusRunning,
		KnownStatus:   statusRunning,
		Containers: []v2.ContainerResponse{
			{
				ID:            containerID,
				Name:          containerName,
				DockerName:    containerName,
				Image:         imageName,
				ImageID:       imageID,
				DesiredStatus: statusRunning,
				KnownStatus:   statusRunning,
				Limits: v2.LimitsResponse{
					CPU:    cpu,
					Memory: memory,
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
			},
		},
	}
	assert.Equal(t, expectedTaskResponse, taskResponse)
}
