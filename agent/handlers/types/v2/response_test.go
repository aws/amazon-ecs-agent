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

package v2

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/aws-sdk-go/aws"
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
	now := time.Now()
	task := &apitask.Task{
		Arn:                 taskARN,
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitask.TaskRunning,
		KnownStatusUnsafe:   apitask.TaskRunning,
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
	container := &apicontainer.Container{
		Name:                containerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainer.ContainerRunning,
		KnownStatusUnsafe:   apicontainer.ContainerRunning,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		Ports: []apicontainer.PortBinding{
			{
				ContainerPort: 80,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}
	created := time.Now()
	container.SetCreatedAt(created)
	labels := map[string]string{
		"foo": "bar",
	}
	container.SetLabels(labels)
	containerNameToDockerContainer := map[string]*apicontainer.DockerContainer{
		taskARN: &apicontainer.DockerContainer{
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

func TestContainerResponse(t *testing.T) {
	testCases := []struct {
		healthCheckType string
		result          bool
	}{
		{
			healthCheckType: "docker",
			result:          false,
		},
		{
			healthCheckType: "",
			result:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("docker health check type: %v", tc.healthCheckType), func(t *testing.T) {

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			state := mock_dockerstate.NewMockTaskEngineState(ctrl)
			container := &apicontainer.Container{
				Name:                containerName,
				Image:               imageName,
				ImageID:             imageID,
				DesiredStatusUnsafe: apicontainer.ContainerRunning,
				KnownStatusUnsafe:   apicontainer.ContainerRunning,
				CPU:                 cpu,
				Memory:              memory,
				Type:                apicontainer.ContainerNormal,
				HealthCheckType:     tc.healthCheckType,
				Health: apicontainer.HealthStatus{
					Status: apicontainer.ContainerHealthy,
					Since:  aws.Time(time.Now()),
				},
				Ports: []apicontainer.PortBinding{
					{
						ContainerPort: 80,
						Protocol:      apicontainer.TransportProtocolTCP,
					},
				},
			}
			created := time.Now()
			container.SetCreatedAt(created)
			labels := map[string]string{
				"foo": "bar",
			}
			container.SetLabels(labels)
			dockerContainer := &apicontainer.DockerContainer{
				DockerID:   containerID,
				DockerName: containerName,
				Container:  container,
			}
			task := &apitask.Task{
				ENI: &apieni.ENI{
					IPV4Addresses: []*apieni.ENIIPV4Address{
						{
							Address: eniIPv4Address,
						},
					},
				},
			}
			gomock.InOrder(
				state.EXPECT().ContainerByID(containerID).Return(dockerContainer, true),
				state.EXPECT().TaskByID(containerID).Return(task, true),
			)

			containerResponse, err := NewContainerResponse(containerID, state)
			assert.NoError(t, err)
			assert.Equal(t, containerResponse.Health == nil, tc.result)
			_, err = json.Marshal(containerResponse)
			assert.NoError(t, err)
		})
	}
}
