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
	mountPoint := "some/mount/point"
	driver := "driver"
	driverOptions := map[string]string{
		"opt1": "val1",
		"opt2": "val2",
	}

	mockClient.EXPECT().CreateVolume(name, driver, driverOptions, nil, dockerapi.CreateVolumeTimeout).Return(
		dockerapi.VolumeResponse{
			DockerVolume: &docker.Volume{Name: name, Driver: driver, Mountpoint: mountPoint, Labels: nil},
			Error:        nil,
		})

	volume := NewVolumeResource(name, driver, driverOptions, nil, mockClient)
	err := volume.Create()
	assert.NoError(t, err)
	assert.Equal(t, mountPoint, volume.mountpoint)
}

func TestCreateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	driver := "driver"
	labels := map[string]string{
		"label1": "val1",
		"label2": "val2",
	}

	mockClient.EXPECT().CreateVolume(name, driver, nil, labels, dockerapi.CreateVolumeTimeout).Return(
		dockerapi.VolumeResponse{
			DockerVolume: nil,
			Error:        errors.New("some error"),
		})

	volume := NewVolumeResource(name, driver, nil, labels, mockClient)
	err := volume.Create()
	assert.NotNil(t, err)
}

func TestCleanupSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	driver := "driver"

	mockClient.EXPECT().RemoveVolume(name, dockerapi.RemoveVolumeTimeout).Return(nil)

	volume := NewVolumeResource(name, driver, nil, nil, mockClient)
	err := volume.Cleanup()
	assert.NoError(t, err)
}

func TestCleanupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)

	name := "volumeName"
	driver := "driver"

	mockClient.EXPECT().RemoveVolume(name, dockerapi.RemoveVolumeTimeout).Return(errors.New("some error"))

	volume := NewVolumeResource(name, driver, nil, nil, mockClient)
	err := volume.Cleanup()
	assert.NotNil(t, err)
}

func TestMarshall(t *testing.T) {
	volumeStr := "{\"Name\":\"volumeName\",\"MountPoint\":\"\",\"Driver\":\"driver\",\"DriverOpts\":{},\"Labels\":{}," +
		"\"CreatedAt\":\"0001-01-01T00:00:00Z\",\"DesiredStatus\":\"CREATED\",\"KnownStatus\":\"NONE\"}"
	name := "volumeName"
	driver := "driver"
	driverOpts := make(map[string]string)
	labels := make(map[string]string)

	volume := NewVolumeResource(name, driver, driverOpts, labels, nil)
	volume.SetDesiredStatus(VolumeCreated)
	volume.SetKnownStatus(VolumeStatusNone)

	bytes, err := volume.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, volumeStr, string(bytes[:]))
}

func TestUnmarshall(t *testing.T) {
	name := "volumeName"
	mountPoint := "mountPoint"
	driver := "drive"

	labels := map[string]string{
		"lab1": "label",
	}
	bytes := []byte("{\"Name\":\"volumeName\",\"MountPoint\":\"mountPoint\",\"Driver\":\"drive\",\"Labels\":{\"lab1\":\"label\"}," +
		"\"CreatedAt\":\"0001-01-01T00:00:00Z\",\"DesiredStatus\":\"CREATED\",\"KnownStatus\":\"NONE\"}")
	unmarshalledVolume := &VolumeResource{}
	err := unmarshalledVolume.UnmarshalJSON(bytes)
	assert.NoError(t, err)

	assert.Equal(t, name, unmarshalledVolume.Name)
	assert.Equal(t, mountPoint, unmarshalledVolume.mountpoint)
	assert.Equal(t, driver, unmarshalledVolume.Driver)
	assert.Equal(t, labels, unmarshalledVolume.Labels)
	assert.Equal(t, time.Time{}, unmarshalledVolume.GetCreatedAt())
	assert.Equal(t, VolumeCreated, unmarshalledVolume.GetDesiredStatus())
	assert.Equal(t, VolumeStatusNone, unmarshalledVolume.GetKnownStatus())
}
