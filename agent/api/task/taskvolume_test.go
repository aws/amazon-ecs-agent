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

package task

import (
	"encoding/json"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalUnmarshalOldTaskVolumes(t *testing.T) {
	taskData := `{"volumes":[{"host":{"sourcePath":"/path"},"name":"1"},{"host":{"hostpath":""}, "name":"2"}]}`

	var out Task
	err := json.Unmarshal([]byte(taskData), &out)
	require.NoError(t, err, "Could not unmarshal task")
	require.Len(t, out.Volumes, 2, "Incorrect number of volumes")

	var v1, v2 TaskVolume

	for _, v := range out.Volumes {
		switch v.Name {
		case "1":
			v1 = v
		case "2":
			v2 = v
		}
	}

	_, ok := v1.Volume.(*taskresourcevolume.FSHostVolume)
	assert.True(t, ok, "Expected v1 to be host volume")
	assert.Equal(t, "/path", v1.Volume.(*taskresourcevolume.FSHostVolume).FSSourcePath, "Unmarshaled v2 didn't match marshalled v2")
	_, ok = v2.Volume.(*taskresourcevolume.LocalDockerVolume)
	assert.True(t, ok, "Expected v2 to be local empty volume")
	assert.Equal(t, "", v2.Volume.Source(), "Expected v2 to have 'sourcepath' work correctly")
}

func TestMarshalUnmarshalTaskVolumes(t *testing.T) {
	task := &Task{
		Arn: "test",
		Volumes: []TaskVolume{
			TaskVolume{Name: "1", Type: HostVolumeType, Volume: &taskresourcevolume.LocalDockerVolume{}},
			TaskVolume{Name: "2", Type: HostVolumeType, Volume: &taskresourcevolume.FSHostVolume{FSSourcePath: "/path"}},
			TaskVolume{Name: "3", Type: DockerVolumeType, Volume: &taskresourcevolume.DockerVolumeConfig{Scope: "task", Driver: "local"}},
		},
	}

	marshal, err := json.Marshal(task)
	require.NoError(t, err, "Could not marshal task")

	var out Task
	err = json.Unmarshal(marshal, &out)
	require.NoError(t, err, "Could not unmarshal task")
	require.Len(t, out.Volumes, 3, "Incorrect number of volumes")

	var v1, v2, v3 TaskVolume

	for _, v := range out.Volumes {
		switch v.Name {
		case "1":
			v1 = v
		case "2":
			v2 = v
		case "3":
			v3 = v
		}
	}

	_, ok := v1.Volume.(*taskresourcevolume.LocalDockerVolume)
	assert.True(t, ok, "Expected v1 to be local empty volume")
	assert.Equal(t, "", v1.Volume.Source(), "Expected v2 to have 'sourcepath' work correctly")
	_, ok = v2.Volume.(*taskresourcevolume.FSHostVolume)
	assert.True(t, ok, "Expected v2 to be host volume")
	assert.Equal(t, "/path", v2.Volume.(*taskresourcevolume.FSHostVolume).FSSourcePath, "Unmarshaled v2 didn't match marshalled v2")

	dockerVolume, ok := v3.Volume.(*taskresourcevolume.DockerVolumeConfig)
	assert.True(t, ok, "incorrect DockerVolumeConfig type")
	assert.Equal(t, "task", dockerVolume.Scope)
	assert.Equal(t, "local", dockerVolume.Driver)
}

func TestInitializeLocalDockerVolume(t *testing.T) {
	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "empty-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name:   "empty-volume-test",
				Type:   "docker",
				Volume: &taskresourcevolume.LocalDockerVolume{},
			},
		},
	}

	testTask.initializeDockerLocalVolumes(nil, nil)

	assert.Len(t, testTask.ResourcesMapUnsafe, 1, "expect the resource map has an empty volume resource")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 1, "expect a volume resource as the container dependency")
}

func TestInitializeSharedProvisionedVolume(t *testing.T) {
	sharedVolumeMatchFullConfig := true
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "shared-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "shared-volume-test",
				Type: "docker",
				Volume: &taskresourcevolume.DockerVolumeConfig{
					Scope:         "shared",
					Autoprovision: false,
				},
			},
		},
	}

	// Expect the volume already exists on the instance
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.VolumeResponse{})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig, dockerClient, nil)

	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 0, "no volume resource should be provisioned by agent")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 0, "resource already exists")
}

func TestInitializeSharedProvisionedVolumeError(t *testing.T) {
	sharedVolumeMatchFullConfig := true
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "shared-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "shared-volume-test",
				Type: "docker",
				Volume: &taskresourcevolume.DockerVolumeConfig{
					Scope:         "shared",
					Autoprovision: false,
				},
			},
		},
	}

	// Expect the volume does not exists on the instance
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.VolumeResponse{Error: errors.New("volume not exist")})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig, dockerClient, nil)
	assert.Error(t, err, "volume not found for auto-provisioned resource should cause task to fail")
}

func TestInitializeSharedNonProvisionedVolume(t *testing.T) {
	sharedVolumeMatchFullConfig := true
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "shared-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "shared-volume-test",
				Type: "docker",
				Volume: &taskresourcevolume.DockerVolumeConfig{
					Scope:         "shared",
					Autoprovision: false,
					Labels:        map[string]string{"test": "test"},
				},
			},
		},
	}

	// Expect the volume already exists on the instance
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.VolumeResponse{
		DockerVolume: &docker.Volume{
			Labels: map[string]string{"test": "test"},
		},
	})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig, dockerClient, nil)

	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 0, "no volume resource should be provisioned by agent")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 0, "resource already exists")
}

func TestInitializeSharedNonProvisionedVolumeValidateNameOnly(t *testing.T) {
	sharedVolumeMatchFullConfig := false

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "shared-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "shared-volume-test",
				Type: "docker",
				Volume: &taskresourcevolume.DockerVolumeConfig{
					Scope:         "shared",
					Autoprovision: true,
					DriverOpts:    map[string]string{"type": "tmpfs"},
					Labels:        map[string]string{"domain": "test"},
				},
			},
		},
	}

	// Expect the volume already exists on the instance
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.VolumeResponse{
		DockerVolume: &docker.Volume{
			Options: map[string]string{},
			Labels:  nil,
		},
	})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig, dockerClient, nil)

	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 0, "no volume resource should be provisioned by agent")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 0, "resource already exists")
}

func TestInitializeSharedAutoprovisionVolumeNotFoundError(t *testing.T) {
	sharedVolumeMatchFullConfig := true
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "shared-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "shared-volume-test",
				Type: "docker",
				Volume: &taskresourcevolume.DockerVolumeConfig{
					Scope:         "shared",
					Autoprovision: true,
				},
			},
		},
	}

	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.VolumeResponse{Error: errors.New("not found")})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig, dockerClient, nil)
	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 1, "volume resource should be provisioned by agent")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 1, "volume resource should be in the container dependency map")
}

func TestInitializeSharedAutoprovisionVolumeNotMatchError(t *testing.T) {
	sharedVolumeMatchFullConfig := true
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "shared-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "shared-volume-test",
				Type: "docker",
				Volume: &taskresourcevolume.DockerVolumeConfig{
					Scope:         "shared",
					Autoprovision: true,
				},
			},
		},
	}

	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.VolumeResponse{
		DockerVolume: &docker.Volume{
			Labels: map[string]string{"test": "test"},
		},
	})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig, dockerClient, nil)
	assert.Error(t, err, "volume resource details not match should cause task fail")
}

func TestInitializeSharedAutoprovisionVolumeTimeout(t *testing.T) {
	sharedVolumeMatchFullConfig := true
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "shared-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "shared-volume-test",
				Type: "docker",
				Volume: &taskresourcevolume.DockerVolumeConfig{
					Scope:         "shared",
					Autoprovision: true,
				},
			},
		},
	}

	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.VolumeResponse{
		Error: &dockerapi.DockerTimeoutError{},
	})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig, dockerClient, nil)
	assert.Error(t, err, "volume resource details not match should cause task fail")
}

func TestInitializeTaskVolume(t *testing.T) {
	sharedVolumeMatchFullConfig := true
	testTask := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "task-volume-test",
						ContainerPath: "/ecs",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "task-volume-test",
				Type: "docker",
				Volume: &taskresourcevolume.DockerVolumeConfig{
					Scope: "task",
				},
			},
		},
	}

	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig, nil, nil)
	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 1, "expect the resource map has an empty volume resource")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 1, "expect a volume resource as the container dependency")
}
