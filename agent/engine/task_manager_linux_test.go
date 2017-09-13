// +build linux

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

package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup/factory/mock"
	"github.com/aws/amazon-ecs-agent/agent/resources/cgroup/mock_control"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	validTaskArn   = "arn:aws:ecs:region:account-id:task/task-id"
	invalidTaskArn = "invalid:task::arn"
)

// TestSetupPlatformResourcesWithCgroupDisabled checks if platform resources
// can be setup without errors when task cgroups are disabled
func TestSetupPlatformResourcesWithCgroupDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	mtask := managedTask{
		Task: testdata.LoadTask("sleep5"),
		engine: &DockerTaskEngine{
			cfg: &cfg,
		},
	}

	assert.NoError(t, mtask.SetupPlatformResources())
}

// TestCleanupPlatformResourcesCgroupDisabled checks if platform resources
// can be cleaned up without errors when task cgroups are disabled
func TestCleanupPlatformResourcesCgroupDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	mtask := managedTask{
		Task: testdata.LoadTask("sleep5"),
		engine: &DockerTaskEngine{
			cfg: &cfg,
		},
	}

	assert.NoError(t, mtask.CleanupPlatformResources())
}

// TestCleanupPlatformResourcesCgroupEnabled checks if platform resources
// can be cleaned up without errors when task cgroups are disabled
func TestCleanupPlatformResourcesCgroupEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	cfg := config.Config{TaskCPUMemLimit: true}
	mtask := managedTask{
		Task: testdata.LoadTask("sleep5TaskCgroup"),
		engine: &DockerTaskEngine{
			cfg: &cfg,
		},
	}

	mockControl.EXPECT().Remove(gomock.Any()).Return(nil)

	err := mtask.CleanupPlatformResources()
	assert.NoError(t, err)
}

// TestCleanupCgroupHappyPath validates the cleanup path for task cgroups
func TestCleanupCgroupHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	mtask := managedTask{
		Task: testdata.LoadTask("sleep5TaskCgroup"),
	}

	mockControl.EXPECT().Remove(gomock.Any()).Return(nil)

	err := mtask.cleanupCgroup()
	assert.NoError(t, err)
}

// TestCleanupCgroupWithInvalidTaskArn attempts to perform task cleanup for
// tasks with invalid taskARN's
func TestCleanupCgroupWithInvalidTaskArn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	cfg := config.Config{TaskCPUMemLimit: true}
	mtask := managedTask{
		Task: &api.Task{
			Arn: invalidTaskArn,
		},
		engine: &DockerTaskEngine{
			cfg: &cfg,
		},
	}

	err := mtask.cleanupCgroup()
	assert.Error(t, err, "invalid taskARN")
}

// TestCleanupCgroupWitRemoveError validates the cgroup cleanup path when the
// call to remove the cgroup fails
func TestCleanupCgroupWitRemoveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	cfg := config.Config{TaskCPUMemLimit: true}
	mtask := managedTask{
		Task: testdata.LoadTask("sleep5TaskCgroup"),
		engine: &DockerTaskEngine{
			cfg: &cfg,
		},
	}

	mockControl.EXPECT().Remove(gomock.Any()).Return(errors.New("cgroups remove error"))

	err := mtask.cleanupCgroup()
	assert.Error(t, err, "cgroup remove error")
}

// TestSetupCgroupHappyPath validates the happy path for task cgroup setup
func TestSetupCgroupHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCgroup := mock_cgroups.NewMockCgroup(ctrl)
	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	cfg := config.Config{TaskCPUMemLimit: true}
	mtask := managedTask{
		Task: testdata.LoadTask("sleep5TaskCgroup"),
		engine: &DockerTaskEngine{
			cfg: &cfg,
		},
	}

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(mockCgroup, nil),
	)

	assert.NoError(t, mtask.setupCgroup())
}

// TestSetupCgroupInvalidTaskARN attempts to setup a task cgroup for tasks with
// invalid taskARN's
func TestSetupCgroupInvalidTaskARN(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mtask := managedTask{
		Task: &api.Task{
			Arn: invalidTaskArn,
		},
	}

	assert.Error(t, mtask.setupCgroup())
}
func TestSetupCgroupInvalidMemConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	mtask := managedTask{
		Task: &api.Task{
			Arn: validTaskArn,
			Containers: []*api.Container{
				{
					Name:   "C1",
					Memory: uint(1024),
				},
			},
			MemoryLimit: int64(512),
		},
	}

	mockControl.EXPECT().Exists(gomock.Any()).Return(false)

	assert.Error(t, mtask.setupCgroup())
}

// TestSetupCgroupExists validates the path when task cgroup already exists
func TestSetupCgroupExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	mtask := managedTask{
		Task: testdata.LoadTask("sleep5TaskCgroup"),
	}

	mockControl.EXPECT().Exists(gomock.Any()).Return(true)

	err := mtask.setupCgroup()
	assert.NoError(t, err, "cgroup already exists")
}

// TestSetupCgroupCreateError validates the path when cgroup creation fails
func TestSetupCgroupCreateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCgroup := mock_cgroups.NewMockCgroup(ctrl)
	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	mtask := managedTask{
		Task: testdata.LoadTask("sleep5TaskCgroup"),
	}

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(mockCgroup, errors.New("cgroup create error")),
	)

	err := mtask.setupCgroup()
	assert.Error(t, err, "cgroup create error")
}

// TestSetupCgroupNil validates when the cgroup object is nil without any error
func TestSetupCgroupNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)

	mtask := managedTask{
		Task: testdata.LoadTask("sleep5TaskCgroup"),
	}

	gomock.InOrder(
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(nil, nil),
	)

	err := mtask.setupCgroup()
	assert.Error(t, err)
}

// TestSetControl validates the set cgroup control method
func TestSetControl(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_cgroup.NewMockControl(ctrl)
	setControl(mockControl)
	assert.Equal(t, mockControl, control)
}

// TestCleanupTaskWithCgroupHappyPath validates the task cgroup happy path
// cleanup
func TestCleanupTaskWithCgroupHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := NewMockDockerClient(ctrl)
	mockImageManager := NewMockImageManager(ctrl)
	mockControl := mock_cgroup.NewMockControl(ctrl)
	defer ctrl.Finish()

	setControl(mockControl)

	cfg := config.Config{TaskCPUMemLimit: true}
	taskEngine := &DockerTaskEngine{
		cfg:          &cfg,
		saver:        statemanager.NewNoopStateManager(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mTask := &managedTask{
		Task:           testdata.LoadTask("sleep5TaskCgroup"),
		_time:          mockTime,
		engine:         taskEngine,
		acsMessages:    make(chan acsTransition),
		dockerMessages: make(chan dockerContainerChange),
	}

	mTask.SetKnownStatus(api.TaskStopped)
	mTask.SetSentStatus(api.TaskStopped)
	container := mTask.Containers[0]
	dockerContainer := &api.DockerContainer{
		DockerName: "dockerContainer",
	}

	// Expectations for triggering cleanup
	now := mTask.GetKnownStatusTime()
	taskStoppedDuration := 1 * time.Minute
	mockTime.EXPECT().Now().Return(now).AnyTimes()
	cleanupTimeTrigger := make(chan time.Time)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanupTimeTrigger)
	go func() {
		cleanupTimeTrigger <- now
	}()

	// Expectations to verify that the task gets removed
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*api.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mockControl.EXPECT().Remove(gomock.Any()).Return(nil)
	mTask.cleanupTask(taskStoppedDuration)
}

// TestCleanupTaskWithCgroupError validates cgroup remove error path
func TestCleanupTaskWithCgroupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := NewMockDockerClient(ctrl)
	mockImageManager := NewMockImageManager(ctrl)
	mockControl := mock_cgroup.NewMockControl(ctrl)
	defer ctrl.Finish()

	setControl(mockControl)

	cfg := config.Config{TaskCPUMemLimit: true}
	taskEngine := &DockerTaskEngine{
		cfg:          &cfg,
		saver:        statemanager.NewNoopStateManager(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mTask := &managedTask{
		Task:           testdata.LoadTask("sleep5TaskCgroup"),
		_time:          mockTime,
		engine:         taskEngine,
		acsMessages:    make(chan acsTransition),
		dockerMessages: make(chan dockerContainerChange),
	}

	mTask.SetKnownStatus(api.TaskStopped)
	mTask.SetSentStatus(api.TaskStopped)
	container := mTask.Containers[0]
	dockerContainer := &api.DockerContainer{
		DockerName: "dockerContainer",
	}

	// Expectations for triggering cleanup
	now := mTask.GetKnownStatusTime()
	taskStoppedDuration := 1 * time.Minute
	mockTime.EXPECT().Now().Return(now).AnyTimes()
	cleanupTimeTrigger := make(chan time.Time)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanupTimeTrigger)
	go func() {
		cleanupTimeTrigger <- now
	}()

	// Expectations to verify that the task gets removed
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*api.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mockControl.EXPECT().Remove(gomock.Any()).Return(errors.New("cgroup remove error"))
	mTask.cleanupTask(taskStoppedDuration)
}
