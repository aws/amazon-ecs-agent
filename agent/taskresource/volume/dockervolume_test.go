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

package volume

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"

	"github.com/docker/docker/api/types/volume"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestCreateSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	scope := "shared"
	autoprovision := true
	mountPoint := "some/mount/point"
	driver := "driver"
	driverOptions := map[string]string{
		"opt1": "val1",
		"opt2": "val2",
	}

	mockClient.EXPECT().CreateVolume(gomock.Any(), name, driver, driverOptions, nil, dockerclient.CreateVolumeTimeout).Return(
		dockerapi.SDKVolumeResponse{
			DockerVolume: &volume.Volume{Name: name, Driver: driver, Mountpoint: mountPoint, Labels: nil},
			Error:        nil,
		})

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume, _ := NewVolumeResource(ctx, name, "docker", name, scope, autoprovision, driver, driverOptions, nil, mockClient)
	err := volume.Create()
	assert.NoError(t, err)
	assert.Equal(t, name, volume.VolumeConfig.Mountpoint)
}

func TestCreateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	scope := "shared"
	autoprovision := true
	driver := "driver"
	labels := map[string]string{
		"label1": "val1",
		"label2": "val2",
	}

	mockClient.EXPECT().CreateVolume(gomock.Any(), name, driver, nil, labels, dockerclient.CreateVolumeTimeout).Return(
		dockerapi.SDKVolumeResponse{
			DockerVolume: nil,
			Error:        errors.New("Test this error is propogated"),
		})

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume, _ := NewVolumeResource(ctx, name, "docker", name, scope, autoprovision, driver, nil, labels, mockClient)
	err := volume.Create()
	assert.NotNil(t, err)
	assert.Equal(t, "Test this error is propogated", err.Error())
	assert.Equal(t, "Test this error is propogated", volume.GetTerminalReason())
}

func TestCleanupSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	scope := "task"
	autoprovision := false
	driver := "driver"

	mockClient.EXPECT().RemoveVolume(gomock.Any(), name, dockerclient.RemoveVolumeTimeout).Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume, _ := NewVolumeResource(ctx, name, "docker", name, scope, autoprovision, driver, nil, nil, mockClient)
	err := volume.Cleanup()
	assert.NoError(t, err)
}

func TestCleanupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	mockClient.EXPECT().RemoveVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("Test this is propogated"))

	name := "volumeName"
	scope := "task"
	autoprovision := false
	driver := "driver"

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume, _ := NewVolumeResource(ctx, name, "docker", name, scope, autoprovision, driver, nil, nil, mockClient)
	err := volume.Cleanup()
	assert.Error(t, err)
	assert.Equal(t, "Test this is propogated", err.Error())
	assert.Equal(t, "Test this is propogated", volume.GetTerminalReason())
}

func TestApplyTransitionForTaskScopeVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	scope := "task"
	autoprovision := false
	driver := "driver"
	driverOptions := map[string]string{}
	labels := map[string]string{}
	mountPoint := "some/mount/point"

	gomock.InOrder(
		mockClient.EXPECT().CreateVolume(gomock.Any(), name, driver, driverOptions, labels, dockerclient.CreateVolumeTimeout).Times(1).Return(
			dockerapi.SDKVolumeResponse{
				DockerVolume: &volume.Volume{Name: name, Driver: driver, Mountpoint: mountPoint, Labels: nil},
				Error:        nil,
			}),
	)

	volume, _ := NewVolumeResource(nil, name, "docker", name, scope, autoprovision, driver, driverOptions, labels, mockClient)
	volume.ApplyTransition(resourcestatus.ResourceStatus(VolumeCreated))
}

func TestApplyTransitionForSharedScopeVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	scope := "shared"
	autoprovision := false
	driver := "driver"
	mountPoint := "some/mount/point"

	gomock.InOrder(
		mockClient.EXPECT().CreateVolume(gomock.Any(), name, driver, nil, nil, dockerclient.CreateVolumeTimeout).Times(1).Return(
			dockerapi.SDKVolumeResponse{
				DockerVolume: &volume.Volume{Name: name, Driver: driver, Mountpoint: mountPoint, Labels: nil},
				Error:        nil,
			}),
		mockClient.EXPECT().RemoveVolume(gomock.Any(), name, dockerclient.RemoveVolumeTimeout).Times(0),
	)

	volume, _ := NewVolumeResource(nil, name, "docker", name, scope, autoprovision, driver, nil, nil, mockClient)
	volume.ApplyTransition(resourcestatus.ResourceStatus(VolumeCreated))
	volume.ApplyTransition(resourcestatus.ResourceStatus(VolumeRemoved))
}

func TestMarshall(t *testing.T) {
	volumeStr := "{\"name\":\"volumeName\"," +
		"\"dockerVolumeConfiguration\":{\"scope\":\"shared\",\"autoprovision\":true,\"mountPoint\":\"\",\"driver\":\"driver\",\"driverOpts\":{},\"labels\":{}," +
		"\"dockerVolumeName\":\"volumeName\"}," +
		"\"pauseContainerPID\":\"123\"," +
		"\"createdAt\":\"0001-01-01T00:00:00Z\",\"desiredStatus\":\"CREATED\",\"knownStatus\":\"NONE\"}"
	name := "volumeName"
	scope := "shared"
	autoprovision := true
	driver := "driver"
	driverOpts := make(map[string]string)
	labels := make(map[string]string)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume, _ := NewVolumeResource(ctx, name, "docker", name, scope, autoprovision, driver, driverOpts, labels, nil)
	volume.SetPauseContainerPID("123")
	volume.SetDesiredStatus(resourcestatus.ResourceStatus(VolumeCreated))
	volume.SetKnownStatus(resourcestatus.ResourceStatus(VolumeStatusNone))

	bytes, err := volume.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, volumeStr, string(bytes[:]))
}

func TestUnmarshall(t *testing.T) {
	name := "volumeName"
	scope := "task"
	autoprovision := false
	mountPoint := "mountPoint"
	driver := "drive"

	labels := map[string]string{
		"lab1": "label",
	}
	bytes := []byte("{\"name\":\"volumeName\",\"dockerVolumeName\":\"volumeName\"," +
		"\"dockerVolumeConfiguration\":{\"scope\":\"task\",\"autoprovision\":false,\"mountPoint\":\"mountPoint\",\"driver\":\"drive\",\"labels\":{\"lab1\":\"label\"}}," +
		"\"pauseContainerPID\":\"123\"," +
		"\"createdAt\":\"0001-01-01T00:00:00Z\",\"desiredStatus\":\"CREATED\",\"knownStatus\":\"NONE\"}")
	unmarshalledVolume := &VolumeResource{}

	err := unmarshalledVolume.UnmarshalJSON(bytes)
	assert.NoError(t, err)

	assert.Equal(t, name, unmarshalledVolume.Name)
	assert.Equal(t, scope, unmarshalledVolume.VolumeConfig.Scope)
	assert.Equal(t, autoprovision, unmarshalledVolume.VolumeConfig.Autoprovision)
	assert.Equal(t, mountPoint, unmarshalledVolume.VolumeConfig.Mountpoint)
	assert.Equal(t, driver, unmarshalledVolume.VolumeConfig.Driver)
	assert.Equal(t, labels, unmarshalledVolume.VolumeConfig.Labels)
	assert.Equal(t, "123", unmarshalledVolume.GetPauseContainerPID())
	assert.Equal(t, time.Time{}, unmarshalledVolume.GetCreatedAt())
	assert.Equal(t, resourcestatus.ResourceStatus(VolumeCreated), unmarshalledVolume.GetDesiredStatus())
	assert.Equal(t, resourcestatus.ResourceStatus(VolumeStatusNone), unmarshalledVolume.GetKnownStatus())
}

func TestNewVolumeResource(t *testing.T) {
	testCases := []struct {
		description   string
		scope         string
		autoprovision bool
		fail          bool
	}{
		{
			"task scoped volume can not be auto provisioned",
			"task",
			true,
			true,
		},
		{
			"task scoped volume should not be auto provisioned",
			"task",
			false,
			false,
		},
		{
			"shared scoped volume can be auto provisioned",
			"shared",
			false,
			false,
		},
		{
			"shared scoped volume can be non-auto provisioned",
			"shared",
			true,
			false,
		},
	}

	for _, testcase := range testCases {
		t.Run(fmt.Sprintf("%s,scope %s, autoprovision: %v", testcase.description,
			testcase.scope, testcase.autoprovision), func(t *testing.T) {
			vol, err := NewVolumeResource(nil, "volume", "efs", "dockerVolume",
				testcase.scope, testcase.autoprovision, "", nil, nil, nil)
			if testcase.fail {
				assert.Error(t, err)
				assert.Nil(t, vol)
				assert.Contains(t, err.Error(), "task scoped volume could not be autoprovisioned")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEFSVolumeBuildContainerDependency(t *testing.T) {
	testRes, _ := NewVolumeResource(nil, "volume", "efs", "dockerVolume",
		"", false, "", nil, nil, nil)

	testRes.BuildContainerDependency("PauseContainer", apicontainerstatus.ContainerCreated, resourcestatus.ResourceStatus(VolumeCreated))
	contDep := testRes.GetContainerDependencies(resourcestatus.ResourceStatus(VolumeCreated))
	assert.NotNil(t, contDep)

	assert.Equal(t, "PauseContainer", contDep[0].ContainerName)
	assert.Equal(t, apicontainerstatus.ContainerCreated, contDep[0].SatisfiedStatus)

}

func TestGetDriverOpts(t *testing.T) {
	volCfg := DockerVolumeConfig{
		Driver: ECSVolumePlugin,
		DriverOpts: map[string]string{
			"o": "iam",
		},
	}
	testCases := []struct {
		name         string
		volRes       *VolumeResource
		expectedOpts map[string]string
	}{
		{
			name: "netns option is appended when pause container pid is set and driver is the volume plugin",
			volRes: &VolumeResource{
				VolumeConfig:            volCfg,
				pauseContainerPIDUnsafe: "123",
			},
			expectedOpts: map[string]string{
				"o": "iam,netns=/proc/123/ns/net",
			},
		},
		{
			name: "netns option is not appended when pause container pid is not set",
			volRes: &VolumeResource{
				VolumeConfig: volCfg,
			},
			expectedOpts: map[string]string{
				"o": "iam,netns=/proc/123/ns/net",
			},
		},
		{
			name: "netns option is not appended when driver is not the volume plugin",
			volRes: &VolumeResource{
				VolumeConfig: DockerVolumeConfig{
					Driver: "another-driver",
					DriverOpts: map[string]string{
						"o": "iam",
					},
				},
				pauseContainerPIDUnsafe: "123",
			},
			expectedOpts: map[string]string{
				"o": "iam",
			},
		},
		{
			name: "netns option is added correctly when it's the only option",
			volRes: &VolumeResource{
				VolumeConfig: DockerVolumeConfig{
					Driver: ECSVolumePlugin,
					DriverOpts: map[string]string{
						"o": "",
					},
				},
				pauseContainerPIDUnsafe: "123",
			},
			expectedOpts: map[string]string{
				"o": "netns=/proc/123/ns/net",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOpts["o"], tc.volRes.getDriverOpts()["o"])
		})
	}
}
