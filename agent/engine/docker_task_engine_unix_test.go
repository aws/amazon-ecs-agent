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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup/mock_control"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
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
	container := &apicontainer.Container{
		Type:  apicontainer.ContainerEmptyHostVolume,
		Image: imageName,
	}
	task := &api.Task{
		Containers: []*apicontainer.Container{container},
	}

	assert.True(t, emptyvolume.LocalImage, "empty volume image is local")
	client.EXPECT().ImportLocalEmptyVolumeImage()

	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, dockerapi.DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	cgroupResource := cgroup.NewCgroupResource("", mockControl, nil, "cgroupRoot", "", specs.LinuxResources{})
	task := &api.Task{
		ENI: &api.ENI{
			MacAddress: mac,
		},
		Resources: []taskresource.TaskResource{cgroupResource},
	}

	cfg := defaultConfig
	cfg.TaskCPUMemLimit = config.ExplicitlyEnabled
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockSaver := mock_statemanager.NewMockStateManager(ctrl)

	taskEngine := &DockerTaskEngine{
		state: mockState,
		saver: mockSaver,
		cfg:   &cfg,
	}

	gomock.InOrder(
		mockControl.EXPECT().Remove("cgroupRoot").Return(nil),
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

func TestTaskCPULimitHappyPath(t *testing.T) {
	testcases := []struct {
		name                string
		metadataCreateError error
		metadataUpdateError error
		metadataCleanError  error
		taskCPULimit        config.Conditional
	}{
		{
			name:                "Task CPU Limit Succeeds",
			metadataCreateError: nil,
			metadataUpdateError: nil,
			metadataCleanError:  nil,
			taskCPULimit:        config.ExplicitlyEnabled,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			metadataConfig := defaultConfig
			metadataConfig.TaskCPUMemLimit = tc.taskCPULimit
			metadataConfig.ContainerMetadataEnabled = true
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, metadataManager := mocks(
				t, ctx, &metadataConfig)
			defer ctrl.Finish()

			roleCredentials := credentials.TaskIAMRoleCredentials{
				IAMRoleCredentials: credentials.IAMRoleCredentials{CredentialsID: "credsid"},
			}
			credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(roleCredentials, true).AnyTimes()
			credentialsManager.EXPECT().RemoveCredentials(credentialsID)

			sleepTask := testdata.LoadTask("sleep5")
			sleepContainer := sleepTask.Containers[0]
			sleepContainer.TransitionDependenciesMap = make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet)
			sleepTask.SetCredentialsID(credentialsID)
			eventStream := make(chan dockerapi.DockerContainerChangeEvent)
			// containerEventsWG is used to force the test to wait until the container created and started
			// events are processed
			containerEventsWG := sync.WaitGroup{}

			if dockerVersionCheckDuringInit {
				client.EXPECT().Version().Return("1.12.6", nil)
			}
			client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
			containerName := make(chan string)
			go func() {
				name := <-containerName
				setCreatedContainerName(name)
			}()
			mockControl := mock_cgroup.NewMockControl(ctrl)
			mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
			taskID, err := sleepTask.GetID()
			assert.NoError(t, err)
			cgroupMemoryPath := fmt.Sprintf("/sys/fs/cgroup/memory/ecs/%s/memory.use_hierarchy", taskID)
			if tc.taskCPULimit.Enabled() {
				// TODO Currently, the resource Setup() method gets invoked multiple
				// times for a task. This is really a bug and a fortunate occurrence
				// that cgroup creation APIs behave idempotently.
				//
				// This should be modified so that 'Setup' is invoked exactly once
				// by moving the cgroup creation to a "resource setup" step in the
				// task life-cycle and performing the setup only in this stage
				taskEngine.(*DockerTaskEngine).resourceFields = &taskresource.ResourceFields{
					Control: mockControl,
					IOUtil:  mockIO,
				}
				mockControl.EXPECT().Exists(gomock.Any()).Return(false)
				mockControl.EXPECT().Create(gomock.Any()).Return(nil, nil)
				mockIO.EXPECT().WriteFile(cgroupMemoryPath, gomock.Any(), gomock.Any()).Return(nil)
			}

			for _, container := range sleepTask.Containers {
				validateContainerRunWorkflow(t, container, sleepTask, imageManager,
					client, &roleCredentials, containerEventsWG,
					eventStream, containerName, func() {
						metadataManager.EXPECT().Create(gomock.Any(), gomock.Any(),
							gomock.Any(), gomock.Any()).Return(tc.metadataCreateError)
						metadataManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(),
							gomock.Any()).Return(tc.metadataUpdateError)
					})
			}

			addTaskToEngine(t, ctx, taskEngine, sleepTask, mockTime, containerEventsWG)
			cleanup := make(chan time.Time, 1)
			defer close(cleanup)
			mockTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
			client.EXPECT().DescribeContainer(gomock.Any(), gomock.Any()).AnyTimes()
			// Simulate a container stop event from docker
			eventStream <- dockerapi.DockerContainerChangeEvent{
				Status: apicontainer.ContainerStopped,
				DockerContainerMetadata: dockerapi.DockerContainerMetadata{
					DockerID: containerID,
					ExitCode: aws.Int(exitCode),
				},
			}

			// StopContainer might be invoked if the test execution is slow, during
			// the cleanup phase. Account for that.
			client.EXPECT().StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
				dockerapi.DockerContainerMetadata{DockerID: containerID}).AnyTimes()
			waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
			// This ensures that managedTask.waitForStopReported makes progress
			sleepTask.SetSentStatus(api.TaskStopped)
			// Extra events should not block forever; duplicate acs and docker events are possible
			go func() { eventStream <- createDockerEvent(apicontainer.ContainerStopped) }()
			go func() { eventStream <- createDockerEvent(apicontainer.ContainerStopped) }()

			sleepTaskStop := testdata.LoadTask("sleep5")
			sleepContainer = sleepTaskStop.Containers[0]
			sleepContainer.TransitionDependenciesMap = make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet)
			sleepTaskStop.SetCredentialsID(credentialsID)
			sleepTaskStop.SetDesiredStatus(api.TaskStopped)
			taskEngine.AddTask(sleepTaskStop)
			// As above, duplicate events should not be a problem
			taskEngine.AddTask(sleepTaskStop)
			taskEngine.AddTask(sleepTaskStop)
			cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)
			if tc.taskCPULimit.Enabled() {
				mockControl.EXPECT().Remove(cgroupRoot).Return(nil)
			}
			// Expect a bunch of steady state 'poll' describes when we trigger cleanup
			client.EXPECT().RemoveContainer(gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(ctx interface{}, removedContainerName string, timeout time.Duration) {
					assert.Equal(t, getCreatedContainerName(), removedContainerName,
						"Container name mismatch")
				}).Return(nil)

			imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any())
			metadataManager.EXPECT().Clean(gomock.Any()).Return(tc.metadataCleanError)
			// trigger cleanup
			cleanup <- time.Now()
			go func() { eventStream <- createDockerEvent(apicontainer.ContainerStopped) }()
			// Wait for the task to actually be dead; if we just fallthrough immediately,
			// the remove might not have happened (expectation failure)
			for {
				tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
				if len(tasks) == 0 {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}
