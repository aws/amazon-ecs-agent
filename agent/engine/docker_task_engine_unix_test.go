// +build !windows,!integration

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
package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup/mock_control"
	"github.com/aws/amazon-ecs-agent/agent/resources/mock_resources"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/stretchr/testify/assert"
)

const (
	// dockerVersionCheckDuringInit specifies if Docker client's Version()
	// API needs to be mocked in engine tests
	//
	// isParallelPullCompatible is invoked during engine intialization
	// on linux. Docker client's Version() call needs to be mocked
	dockerVersionCheckDuringInit = true
	cgroupMountPath              = "/sys/fs/cgroup"
)

func TestPullEmptyVolumeImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, _, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	saver := mock_statemanager.NewMockStateManager(ctrl)
	taskEngine.SetSaver(saver)

	imageName := "image"
	container := &api.Container{
		Type:  api.ContainerEmptyHostVolume,
		Image: imageName,
	}
	task := &api.Task{
		Containers: []*api.Container{container},
	}

	assert.True(t, emptyvolume.LocalImage, "empty volume image is local")
	client.EXPECT().ImportLocalEmptyVolumeImage()

	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, dockerapi.DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	task := &api.Task{
		ENI: &api.ENI{
			MacAddress: mac,
		},
	}

	cfg := defaultConfig
	cfg.TaskCPUMemLimit = config.ExplicitlyEnabled
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockSaver := mock_statemanager.NewMockStateManager(ctrl)
	mockResource := mock_resources.NewMockResource(ctrl)
	taskEngine := &DockerTaskEngine{
		state:    mockState,
		saver:    mockSaver,
		cfg:      &cfg,
		resource: mockResource,
	}

	gomock.InOrder(
		mockResource.EXPECT().Cleanup(task).Return(errors.New("error")),
		mockState.EXPECT().RemoveTask(task),
		mockState.EXPECT().RemoveENIAttachment(mac),
		mockSaver.EXPECT().Save(),
	)

	taskEngine.deleteTask(task)
}

func TestEngineDisableConcurrentPull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	if dockerVersionCheckDuringInit {
		client.EXPECT().Version().Return("1.11.0", nil)
	}
	client.EXPECT().ContainerEvents(gomock.Any())

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	dockerTaskEngine, _ := taskEngine.(*DockerTaskEngine)
	assert.False(t, dockerTaskEngine.enableConcurrentPull,
		"Task engine should not be able to perform concurrent pulling for version < 1.11.1")
}

// TestResourceContainerProgression tests the container progression based on a
// resource dependency
func TestResourceContainerProgression(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer := sleepTask.Containers[0]
	sleepContainer.TransitionDependenciesMap = make(map[api.ContainerStatus]api.TransitionDependencySet)
	sleepContainer.BuildResourceDependency("cgroup", taskresource.ResourceCreated, api.ContainerPulled)

	mockControl := mock_cgroup.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	taskID, err := sleepTask.GetID()
	assert.NoError(t, err)
	cgroupMemoryPath := fmt.Sprintf("/sys/fs/cgroup/memory/ecs/%s/memory.use_hierarchy", taskID)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)
	cgroupResource := cgroup.NewCgroupResource(sleepTask.Arn, mockControl, cgroupRoot, cgroupMountPath, specs.LinuxResources{})
	cgroupResource.SetIOUtil(mockIO)

	sleepTask.Resources = []taskresource.TaskResource{cgroupResource}
	sleepTask.Resources[0].SetDesiredStatus(taskresource.ResourceCreated)

	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	// containerEventsWG is used to force the test to wait until the container created and started
	// events are processed
	containerEventsWG := sync.WaitGroup{}
	if dockerVersionCheckDuringInit {
		client.EXPECT().Version()
	}
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	gomock.InOrder(
		// Ensure that the resource is created first
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(nil, nil),
		mockIO.EXPECT().WriteFile(cgroupMemoryPath, gomock.Any(), gomock.Any()).Return(nil),
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes(),
		client.EXPECT().PullImage(sleepContainer.Image, nil).Return(dockerapi.DockerContainerMetadata{}),
		imageManager.EXPECT().RecordContainerReference(sleepContainer).Return(nil),
		imageManager.EXPECT().GetImageStateFromImageName(sleepContainer.Image).Return(nil),
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx interface{}, config *docker.Config, hostConfig *docker.HostConfig, containerName string, z time.Duration) {
				assert.True(t, strings.Contains(containerName, sleepContainer.Name))
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerCreated)
					containerEventsWG.Done()
				}()
			}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID + ":" + sleepContainer.Name}),
		// Next, the sleep container is started
		client.EXPECT().StartContainer(gomock.Any(), containerID+":"+sleepContainer.Name, defaultConfig.ContainerStartTimeout).Do(
			func(ctx interface{}, id string, timeout time.Duration) {
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerRunning)
					containerEventsWG.Done()
				}()
			}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID + ":" + sleepContainer.Name}),
	)

	addTaskToEngine(t, ctx, taskEngine, sleepTask, mockTime, containerEventsWG)

	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()

	// Simulate a container stop event from docker
	eventStream <- dockerapi.DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: containerID + ":" + sleepContainer.Name,
			ExitCode: aws.Int(exitCode),
		},
	}
	waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
}

// TestResourceContainerProgressionFailure ensures that task moves to STOPPED when
// resource creation fails
func TestResourceContainerProgressionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()
	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer := sleepTask.Containers[0]
	sleepContainer.TransitionDependenciesMap = make(map[api.ContainerStatus]api.TransitionDependencySet)
	sleepContainer.BuildResourceDependency("cgroup", taskresource.ResourceCreated, api.ContainerPulled)

	mockControl := mock_cgroup.NewMockControl(ctrl)
	taskID, err := sleepTask.GetID()
	assert.NoError(t, err)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)
	cgroupResource := cgroup.NewCgroupResource(sleepTask.Arn, mockControl, cgroupRoot, cgroupMountPath, specs.LinuxResources{})

	sleepTask.Resources = []taskresource.TaskResource{cgroupResource}
	sleepTask.Resources[0].SetDesiredStatus(taskresource.ResourceCreated)

	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	if dockerVersionCheckDuringInit {
		client.EXPECT().Version()
	}
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	gomock.InOrder(
		// resource creation failure
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(nil, errors.New("cgroup create error")),
	)
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()

	err = taskEngine.Init(ctx)
	assert.NoError(t, err)

	taskEngine.AddTask(sleepTask)
	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()
	waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
}
