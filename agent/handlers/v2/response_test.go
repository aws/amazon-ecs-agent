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

package v2

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	taskARN              = "t1"
	cluster              = "default"
	family               = "sleep"
	version              = "1"
	containerID          = "cid"
	containerName        = "sleepy"
	imageName            = "busybox"
	imageID              = "bUsYbOx"
	cpu                  = 1024
	memory               = 512
	eniIPv4Address       = "10.0.0.2"
	volName              = "volume1"
	volSource            = "/var/lib/volume1"
	volDestination       = "/volume"
	availabilityZone     = "us-west-2b"
	containerInstanceArn = "containerInstance-test"
)

func TestTaskResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)
	now := time.Now()
	task := &apitask.Task{
		Arn:                 taskARN,
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		ENIs: []*apieni.ENI{
			{
				IPV4Addresses: []*apieni.ENIIPV4Address{
					{
						Address: eniIPv4Address,
					},
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
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		Ports: []apicontainer.PortBinding{
			{
				ContainerPort: 80,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
		VolumesUnsafe: []types.MountPoint{
			{
				Name:        volName,
				Source:      volSource,
				Destination: volDestination,
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
		taskARN: {
			DockerID:   containerID,
			DockerName: containerName,
			Container:  container,
		},
	}
	gomock.InOrder(
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
	)

	taskResponse, err := NewTaskResponse(taskARN, state, ecsClient, cluster, availabilityZone, containerInstanceArn, false)
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
				DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
				KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
				CPU:                 cpu,
				Memory:              memory,
				Type:                apicontainer.ContainerNormal,
				HealthCheckType:     tc.healthCheckType,
				Health: apicontainer.HealthStatus{
					Status: apicontainerstatus.ContainerHealthy,
					Since:  aws.Time(time.Now()),
				},
				Ports: []apicontainer.PortBinding{
					{
						ContainerPort: 80,
						Protocol:      apicontainer.TransportProtocolTCP,
					},
				},
				VolumesUnsafe: []types.MountPoint{
					{
						Name:        volName,
						Source:      volSource,
						Destination: volDestination,
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
				ENIs: []*apieni.ENI{
					{
						IPV4Addresses: []*apieni.ENIIPV4Address{
							{
								Address: eniIPv4Address,
							},
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

func TestTaskResponseMarshal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedTaskResponseMap := map[string]interface{}{
		"Cluster":          cluster,
		"TaskARN":          taskARN,
		"Family":           family,
		"Revision":         version,
		"DesiredStatus":    "RUNNING",
		"KnownStatus":      "RUNNING",
		"AvailabilityZone": availabilityZone,
		"Containers": []interface{}{
			map[string]interface{}{
				"DockerId":   containerID,
				"Name":       containerName,
				"DockerName": containerName,
				"Image":      imageName,
				"ImageID":    imageID,
				"Ports": []interface{}{
					map[string]interface{}{
						"HostPort":      float64(80),
						"ContainerPort": float64(80),
						"Protocol":      "tcp",
					},
				},
				"DesiredStatus": "NONE",
				"KnownStatus":   "NONE",
				"Limits": map[string]interface{}{
					"CPU":    float64(0),
					"Memory": float64(0),
				},
				"Type": "NORMAL",
				"Networks": []interface{}{
					map[string]interface{}{
						"IPv4Addresses": []interface{}{
							eniIPv4Address,
						},
						"NetworkMode": "awsvpc",
					},
				},
			},
		},
		"ContainerInstanceTags": map[string]interface{}{
			"ContainerInstanceTag1": "firstTag",
			"ContainerInstanceTag2": "secondTag",
		},
		"TaskTags": map[string]interface{}{
			"TaskTag1": "firstTag",
			"TaskTag2": "secondTag",
		},
	}

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	task := &apitask.Task{
		Arn:                 taskARN,
		Family:              family,
		Version:             version,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		KnownStatusUnsafe:   apitaskstatus.TaskRunning,
		ENIs: []*apieni.ENI{
			{
				IPV4Addresses: []*apieni.ENIIPV4Address{
					{
						Address: eniIPv4Address,
					},
				},
			},
		},
	}

	container := &apicontainer.Container{
		Name:         containerName,
		V3EndpointID: "",
		Image:        imageName,
		ImageID:      imageID,
		KnownPortBindingsUnsafe: []apicontainer.PortBinding{
			{
				ContainerPort: 80,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}

	containerNameToDockerContainer := map[string]*apicontainer.DockerContainer{
		taskARN: {
			DockerID:   containerID,
			DockerName: containerName,
			Container:  container,
		},
	}

	contInstTag1Key := "ContainerInstanceTag1"
	contInstTag1Val := "firstTag"
	contInstTag2Key := "ContainerInstanceTag2"
	contInstTag2Val := "secondTag"
	taskTag1Key := "TaskTag1"
	taskTag1Val := "firstTag"
	taskTag2Key := "TaskTag2"
	taskTag2Val := "secondTag"

	gomock.InOrder(
		state.EXPECT().TaskByArn(taskARN).Return(task, true),
		state.EXPECT().ContainerMapByArn(taskARN).Return(containerNameToDockerContainer, true),
		ecsClient.EXPECT().GetResourceTags(containerInstanceArn).Return([]*ecs.Tag{
			&ecs.Tag{
				Key:   &contInstTag1Key,
				Value: &contInstTag1Val,
			},
			&ecs.Tag{
				Key:   &contInstTag2Key,
				Value: &contInstTag2Val,
			},
		}, nil),
		ecsClient.EXPECT().GetResourceTags(taskARN).Return([]*ecs.Tag{
			&ecs.Tag{
				Key:   &taskTag1Key,
				Value: &taskTag1Val,
			},
			&ecs.Tag{
				Key:   &taskTag2Key,
				Value: &taskTag2Val,
			},
		}, nil),
	)

	taskResponse, err := NewTaskResponse(taskARN, state, ecsClient, cluster, availabilityZone, containerInstanceArn, true)
	assert.NoError(t, err)

	taskResponseJSON, err := json.Marshal(taskResponse)
	assert.NoError(t, err)

	taskResponseMap := make(map[string]interface{})
	json.Unmarshal(taskResponseJSON, &taskResponseMap)
	assert.Equal(t, expectedTaskResponseMap, taskResponseMap)
}

func TestContainerResponseMarshal(t *testing.T) {
	timeRFC3339, _ := time.Parse(time.RFC3339, "2014-11-12T11:45:26Z")

	expectedContainerResponseMap := map[string]interface{}{
		"DockerId":   containerID,
		"DockerName": containerName,
		"Name":       containerName,
		"Image":      imageName,
		"ImageID":    imageID,
		"Ports": []interface{}{
			map[string]interface{}{
				"ContainerPort": float64(80),
				"Protocol":      "tcp",
				"HostPort":      float64(80),
			},
		},
		"Labels": map[string]interface{}{
			"foo": "bar",
		},
		"DesiredStatus": "RUNNING",
		"KnownStatus":   "RUNNING",
		"Limits": map[string]interface{}{
			"CPU":    float64(cpu),
			"Memory": float64(memory),
		},
		"CreatedAt": timeRFC3339.Format(time.RFC3339),
		"Type":      "NORMAL",
		"Networks": []interface{}{
			map[string]interface{}{
				"NetworkMode": "awsvpc",
				"IPv4Addresses": []interface{}{
					eniIPv4Address,
				},
			},
		},
		"Health": map[string]interface{}{
			"statusSince": timeRFC3339.Format(time.RFC3339),
			"status":      "HEALTHY",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	state := mock_dockerstate.NewMockTaskEngineState(ctrl)
	container := &apicontainer.Container{
		Name:                containerName,
		Image:               imageName,
		ImageID:             imageID,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		CPU:                 cpu,
		Memory:              memory,
		Type:                apicontainer.ContainerNormal,
		HealthCheckType:     "docker",
		Health: apicontainer.HealthStatus{
			Status: apicontainerstatus.ContainerHealthy,
			Since:  aws.Time(timeRFC3339),
		},
		KnownPortBindingsUnsafe: []apicontainer.PortBinding{
			{
				ContainerPort: 80,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}

	container.SetCreatedAt(timeRFC3339)
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
		ENIs: []*apieni.ENI{
			{
				IPV4Addresses: []*apieni.ENIIPV4Address{
					{
						Address: eniIPv4Address,
					},
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

	containerResponseJSON, err := json.Marshal(containerResponse)
	assert.NoError(t, err)

	containerResponseMap := make(map[string]interface{})
	json.Unmarshal(containerResponseJSON, &containerResponseMap)
	assert.Equal(t, expectedContainerResponseMap, containerResponseMap)
}
