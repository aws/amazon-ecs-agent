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

package task

import (
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"

	"github.com/docker/docker/api/types"
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

func TestMarshalTaskVolumesEFS(t *testing.T) {
	task := &Task{
		Arn: "test",
		Volumes: []TaskVolume{
			{
				Name: "1",
				Type: EFSVolumeType,
				Volume: &taskresourcevolume.EFSVolumeConfig{
					AuthConfig: taskresourcevolume.EFSAuthConfig{
						AccessPointId: "fsap-123",
						Iam:           "ENABLED",
					},
					FileSystemID:          "fs-12345",
					RootDirectory:         "/tmp",
					TransitEncryption:     "ENABLED",
					TransitEncryptionPort: 23456,
				},
			},
		},
	}

	marshal, err := json.Marshal(task)
	require.NoError(t, err, "Could not marshal task")
	expectedTaskDef := `{
		"Arn": "test",
		"Family": "",
		"Version": "",
		"ServiceName": "",
		"Containers": null,
		"associations": null,
		"resources": null,
		"volumes": [
		  {
			"efsVolumeConfiguration": {
				"authorizationConfig": {
					"accessPointId": "fsap-123",
					"iam": "ENABLED"
				},
				"fileSystemId": "fs-12345",
				"rootDirectory": "/tmp",
				"transitEncryption": "ENABLED",
				"transitEncryptionPort": 23456,
			 	"dockerVolumeName": ""
			},
			"name": "1",
			"type": "efs"
		  }
		],
		"DesiredStatus": "NONE",
		"KnownStatus": "NONE",
		"KnownTime": "0001-01-01T00:00:00Z",
		"PullStartedAt": "0001-01-01T00:00:00Z",
		"PullStoppedAt": "0001-01-01T00:00:00Z",
		"ExecutionStoppedAt": "0001-01-01T00:00:00Z",
		"SentStatus": "NONE",
		"StartSequenceNumber": 0,
		"StopSequenceNumber": 0,
		"executionCredentialsID": "",
		"ENI": null,
		"AppMesh": null,
		"PlatformFields": %s
	  }`
	if runtime.GOOS == "windows" {
		// windows task defs have a special 'cpu/memory unbounded' field added.
		// see https://github.com/aws/amazon-ecs-agent/pull/1227
		expectedTaskDef = fmt.Sprintf(expectedTaskDef, `{"cpuUnbounded": null, "memoryUnbounded": null}`)
	} else {
		expectedTaskDef = fmt.Sprintf(expectedTaskDef, "{}")
	}
	require.JSONEq(t, expectedTaskDef, string(marshal))
}

func TestUnmarshalTaskVolumesEFS(t *testing.T) {
	taskDef := []byte(`{
		"Arn": "test",
		"Family": "",
		"Version": "",
		"Containers": null,
		"associations": null,
		"resources": null,
		"volumes": [
		  {
			"efsVolumeConfiguration": {
			  "authorizationConfig": {
					"accessPointId": "fsap-123",
					"iam": "ENABLED"
				},
				"fileSystemId": "fs-12345",
				"rootDirectory": "/tmp",
				"transitEncryption": "ENABLED",
				"transitEncryptionPort": 23456,
				"dockerVolumeName": ""
			},
			"name": "1",
			"type": "efs"
		  }
		],
		"DesiredStatus": "NONE",
		"KnownStatus": "NONE",
		"KnownTime": "0001-01-01T00:00:00Z",
		"PullStartedAt": "0001-01-01T00:00:00Z",
		"PullStoppedAt": "0001-01-01T00:00:00Z",
		"ExecutionStoppedAt": "0001-01-01T00:00:00Z",
		"SentStatus": "NONE",
		"StartSequenceNumber": 0,
		"StopSequenceNumber": 0,
		"executionCredentialsID": "",
		"ENI": null,
		"AppMesh": null,
		"PlatformFields": {}
	  }`)
	var task Task
	err := json.Unmarshal(taskDef, &task)
	require.NoError(t, err, "Could not unmarshal task")

	require.Len(t, task.Volumes, 1)
	assert.Equal(t, "efs", task.Volumes[0].Type)
	assert.Equal(t, "1", task.Volumes[0].Name)
	efsConfig, ok := task.Volumes[0].Volume.(*taskresourcevolume.EFSVolumeConfig)
	require.True(t, ok)
	assert.Equal(t, "fsap-123", efsConfig.AuthConfig.AccessPointId)
	assert.Equal(t, "ENABLED", efsConfig.AuthConfig.Iam)
	assert.Equal(t, "fs-12345", efsConfig.FileSystemID)
	assert.Equal(t, "/tmp", efsConfig.RootDirectory)
	assert.Equal(t, "ENABLED", efsConfig.TransitEncryption)
	assert.Equal(t, int64(23456), efsConfig.TransitEncryptionPort)
}

func TestMarshalUnmarshalTaskVolumes(t *testing.T) {
	task := &Task{
		Arn: "test",
		Volumes: []TaskVolume{
			{Name: "1", Type: HostVolumeType, Volume: &taskresourcevolume.LocalDockerVolume{}},
			{Name: "2", Type: HostVolumeType, Volume: &taskresourcevolume.FSHostVolume{FSSourcePath: "/path"}},
			{Name: "3", Type: DockerVolumeType, Volume: &taskresourcevolume.DockerVolumeConfig{Scope: "task", Driver: "local"}},
			{Name: "4", Type: EFSVolumeType, Volume: &taskresourcevolume.EFSVolumeConfig{FileSystemID: "fs-12345", RootDirectory: "/tmp"}},
		},
	}

	marshal, err := json.Marshal(task)
	require.NoError(t, err, "Could not marshal task")

	var out Task
	err = json.Unmarshal(marshal, &out)
	require.NoError(t, err, "Could not unmarshal task")
	require.Len(t, out.Volumes, 4, "Incorrect number of volumes")

	var v1, v2, v3, v4 TaskVolume

	for _, v := range out.Volumes {
		switch v.Name {
		case "1":
			v1 = v
		case "2":
			v2 = v
		case "3":
			v3 = v
		case "4":
			v4 = v
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

	efsVolume, ok := v4.Volume.(*taskresourcevolume.EFSVolumeConfig)
	assert.True(t, ok, "incorrect EFSVolumeConfig type")
	assert.Equal(t, "fs-12345", efsVolume.FileSystemID)
	assert.Equal(t, "/tmp", efsVolume.RootDirectory)
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
	sharedVolumeMatchFullConfig := config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
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
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.SDKVolumeResponse{})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig.Enabled(), dockerClient, nil)

	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 0, "no volume resource should be provisioned by agent")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 0, "resource already exists")
}

func TestInitializeEFSVolume_UseLocalVolumeDriver(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := getEFSTask()

	cfg := &config.Config{
		AWSRegion: "us-west-1",
	}
	err := testTask.initializeEFSVolumes(cfg, dockerClient, nil)

	assert.NoError(t, err)
	assert.Len(t, testTask.Volumes, 1)

	dockervol, ok := testTask.Volumes[0].Volume.(*taskresourcevolume.DockerVolumeConfig)
	assert.True(t, ok)
	assert.Equal(t, "local", dockervol.Driver)
	assert.Equal(t, "nfs", dockervol.DriverOpts["type"])
	assert.Equal(t, "addr=fs-12345.efs.us-west-1.amazonaws.com,nfsvers=4.1,rsize=1048576,wsize=1048576,hard,timeo=600,retrans=2,noresvport", dockervol.DriverOpts["o"])
	assert.Equal(t, ":/my/root/dir", dockervol.DriverOpts["device"])
}

func TestInitializeEFSVolume_UseECSVolumePlugin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := getEFSTask()
	testTask.credentialsRelativeURIUnsafe = "/v2/creds-id"

	cfg := &config.Config{
		VolumePluginCapabilities: []string{"efsAuth"},
	}
	err := testTask.initializeEFSVolumes(cfg, dockerClient, nil)

	require.NoError(t, err)
	assert.Len(t, testTask.Volumes, 1)

	dockervol, ok := testTask.Volumes[0].Volume.(*taskresourcevolume.DockerVolumeConfig)
	require.True(t, ok)
	assert.Equal(t, "efs", dockervol.DriverOpts["type"])
	assert.Equal(t, "tls,tlsport=12345,iam,awscredsuri=/v2/creds-id,accesspoint=fsap-123", dockervol.DriverOpts["o"])
	assert.Equal(t, "fs-12345:/my/root/dir", dockervol.DriverOpts["device"])
}

func TestInitializeEFSVolume_WrongVolumeConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := getEFSTask()
	testTask.Volumes[0].Volume = &taskresourcevolume.DockerVolumeConfig{}

	cfg := &config.Config{
		AWSRegion: "us-west-1",
	}
	err := testTask.initializeEFSVolumes(cfg, dockerClient, nil)

	assert.Error(t, err)
}

func TestInitializeEFSVolume_WrongVolumeType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)

	testTask := getEFSTask()
	testTask.Volumes[0].Type = "docker"

	cfg := &config.Config{
		AWSRegion: "us-west-1",
	}
	err := testTask.initializeEFSVolumes(cfg, dockerClient, nil)

	assert.NoError(t, err)
	err = testTask.initializeDockerVolumes(true, dockerClient, nil)
	assert.Error(t, err)
}

func TestInitializeSharedProvisionedVolumeError(t *testing.T) {
	sharedVolumeMatchFullConfig := config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
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
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.SDKVolumeResponse{Error: errors.New("volume not exist")})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig.Enabled(), dockerClient, nil)
	assert.Error(t, err, "volume not found for auto-provisioned resource should cause task to fail")
}

func TestInitializeSharedNonProvisionedVolume(t *testing.T) {
	sharedVolumeMatchFullConfig := config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
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
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.SDKVolumeResponse{
		DockerVolume: &types.Volume{
			Labels: map[string]string{"test": "test"},
		},
	})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig.Enabled(), dockerClient, nil)

	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 0, "no volume resource should be provisioned by agent")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 0, "resource already exists")
}

func TestInitializeSharedNonProvisionedVolumeValidateNameOnly(t *testing.T) {
	sharedVolumeMatchFullConfig := config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled}

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
	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.SDKVolumeResponse{
		DockerVolume: &types.Volume{
			Options: map[string]string{},
			Labels:  nil,
		},
	})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig.Enabled(), dockerClient, nil)

	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 0, "no volume resource should be provisioned by agent")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 0, "resource already exists")
}

func TestInitializeSharedAutoprovisionVolumeNotFoundError(t *testing.T) {
	sharedVolumeMatchFullConfig := config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
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

	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.SDKVolumeResponse{Error: errors.New("not found")})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig.Enabled(), dockerClient, nil)
	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 1, "volume resource should be provisioned by agent")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 1, "volume resource should be in the container dependency map")
}

func TestInitializeSharedAutoprovisionVolumeNotMatchError(t *testing.T) {
	sharedVolumeMatchFullConfig := config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
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

	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.SDKVolumeResponse{
		DockerVolume: &types.Volume{
			Labels: map[string]string{"test": "test"},
		},
	})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig.Enabled(), dockerClient, nil)
	assert.Error(t, err, "volume resource details not match should cause task fail")
}

func TestInitializeSharedAutoprovisionVolumeTimeout(t *testing.T) {
	sharedVolumeMatchFullConfig := config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
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

	dockerClient.EXPECT().InspectVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.SDKVolumeResponse{
		Error: &dockerapi.DockerTimeoutError{},
	})
	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig.Enabled(), dockerClient, nil)
	assert.Error(t, err, "volume resource details not match should cause task fail")
}

func TestInitializeTaskVolume(t *testing.T) {
	sharedVolumeMatchFullConfig := config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
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

	err := testTask.initializeDockerVolumes(sharedVolumeMatchFullConfig.Enabled(), nil, nil)
	assert.NoError(t, err)
	assert.Len(t, testTask.ResourcesMapUnsafe, 1, "expect the resource map has an empty volume resource")
	assert.Len(t, testTask.Containers[0].TransitionDependenciesMap, 1, "expect a volume resource as the container dependency")
}

func TestGetEFSVolumeDriverName(t *testing.T) {
	cfg := &config.Config{
		VolumePluginCapabilities: []string{"efsAuth"},
	}
	assert.Equal(t, "amazon-ecs-volume-plugin", getEFSVolumeDriverName(cfg))
	assert.Equal(t, "local", getEFSVolumeDriverName(&config.Config{}))
}

func TestSetPausePIDInVolumeResources(t *testing.T) {
	task := getEFSTask()
	volRes := &taskresourcevolume.VolumeResource{}
	task.AddResource("dockerVolume", volRes)
	task.SetPausePIDInVolumeResources("pid")
	assert.Equal(t, "pid", volRes.GetPauseContainerPID())
}
