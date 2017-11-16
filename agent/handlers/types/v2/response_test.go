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

package v2

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	taskARN        = "t1"
	cluster        = "default"
	family         = "sleep"
	version        = "1"
	containerID    = "cid"
	containerName  = "sleepy"
	imageName      = "busybox"
	imageID        = "bUsYbOx"
	cpu            = 1024
	memory         = 512
	eniIPv4Address = "10.0.0.2"
)

func TestTaskResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
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
	container := &api.Container{
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
				ContainerPort: 80,
				Protocol:      api.TransportProtocolTCP,
			},
		},
	}
	created := time.Now()
	container.SetCreatedAt(created)
	labels := map[string]string{
		"foo": "bar",
	}
	container.SetLabels(labels)
	containerNameToDockerContainer := map[string]*api.DockerContainer{
		taskARN: &api.DockerContainer{
			DockerID:   containerID,
			DockerName: containerName,
			Container:  container,
		},
	}
	gomock.InOrder(
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
	)

	taskResponse, err := NewTaskResponse(taskARN, state, cluster)
	assert.NoError(t, err)
	_, err = json.Marshal(taskResponse)
	assert.NoError(t, err)
	assert.Equal(t, created.UTC().String(), taskResponse.Containers[0].CreatedAt.String())
}
