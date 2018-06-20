// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	docker "github.com/fsouza/go-dockerclient"
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

	mockClient.EXPECT().CreateVolume(gomock.Any(), name, driver, driverOptions, nil, dockerapi.CreateVolumeTimeout).Return(
		dockerapi.VolumeResponse{
			DockerVolume: &docker.Volume{Name: name, Driver: driver, Mountpoint: mountPoint, Labels: nil},
			Error:        nil,
		})

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume := NewVolumeResource(name, scope, autoprovision, driver, driverOptions, nil, mockClient, ctx)
	err := volume.Create()
	assert.NoError(t, err)
	assert.Equal(t, mountPoint, volume.VolumeConfig.Mountpoint)
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

	mockClient.EXPECT().CreateVolume(gomock.Any(), name, driver, nil, labels, dockerapi.CreateVolumeTimeout).Return(
		dockerapi.VolumeResponse{
			DockerVolume: nil,
			Error:        errors.New("some error"),
		})

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume := NewVolumeResource(name, scope, autoprovision, driver, nil, labels, mockClient, ctx)
	err := volume.Create()
	assert.NotNil(t, err)
}

func TestCleanupSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	scope := "task"
	autoprovision := false
	driver := "driver"

	mockClient.EXPECT().RemoveVolume(gomock.Any(), name, dockerapi.RemoveVolumeTimeout).Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume := NewVolumeResource(name, scope, autoprovision, driver, nil, nil, mockClient, ctx)
	err := volume.Cleanup()
	assert.NoError(t, err)
}

func TestCleanupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	scope := "shared"
	autoprovision := false
	driver := "driver"

	mockClient.EXPECT().RemoveVolume(gomock.Any(), name, dockerapi.RemoveVolumeTimeout).Return(errors.New("some error"))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume := NewVolumeResource(name, scope, autoprovision, driver, nil, nil, mockClient, ctx)
	err := volume.Cleanup()
	assert.NotNil(t, err)
}

func TestMarshall(t *testing.T) {
	volumeStr := "{\"name\":\"volumeName\",\"dockerVolumeConfiguration\":{\"scope\":\"shared\",\"autoprovision\":true,\"mountPoint\":\"\",\"driver\":\"driver\",\"driverOpts\":{},\"labels\":{}}," +
		"\"createdAt\":\"0001-01-01T00:00:00Z\",\"desiredStatus\":\"CREATED\",\"knownStatus\":\"NONE\"}"
	name := "volumeName"
	scope := "shared"
	autoprovision := true
	driver := "driver"
	driverOpts := make(map[string]string)
	labels := make(map[string]string)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volume := NewVolumeResource(name, scope, autoprovision, driver, driverOpts, labels, nil, ctx)
	volume.SetDesiredStatus(VolumeCreated)
	volume.SetKnownStatus(VolumeStatusNone)

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
	bytes := []byte("{\"name\":\"volumeName\",\"dockerVolumeConfiguration\":{\"scope\":\"task\",\"autoprovision\":false,\"mountPoint\":\"mountPoint\",\"driver\":\"drive\",\"labels\":{\"lab1\":\"label\"}}," +
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
	assert.Equal(t, time.Time{}, unmarshalledVolume.GetCreatedAt())
	assert.Equal(t, VolumeCreated, unmarshalledVolume.GetDesiredStatus())
	assert.Equal(t, VolumeStatusNone, unmarshalledVolume.GetKnownStatus())
}
