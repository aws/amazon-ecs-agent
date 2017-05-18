// +build !integration
// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
)

func TestContainerNextStateFromNone(t *testing.T) {
	// NONE -> RUNNING transition is allowed and actionable, when desired is Running
	// The exptected next status is Pulled
	testContainerNextStateAssertions(t,
		api.ContainerStatusNone, api.ContainerRunning,
		api.ContainerPulled, true, true)

	// NONE -> NONE transition is not be allowed and is not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerStatusNone, api.ContainerStatusNone,
		api.ContainerStatusNone, false, false)

	// NONE -> STOPPED transition will result in STOPPED and is allowed, but not
	// actionable, when desired is STOPPED
	testContainerNextStateAssertions(t,
		api.ContainerStatusNone, api.ContainerStopped,
		api.ContainerStopped, false, true)
}

func TestContainerNextStateFromPulled(t *testing.T) {
	// PULLED -> RUNNING transition is allowed and actionable, when desired is Running
	// The exptected next status is Created
	testContainerNextStateAssertions(t,
		api.ContainerPulled, api.ContainerRunning,
		api.ContainerCreated, true, true)

	// PULLED -> PULLED transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerPulled, api.ContainerPulled,
		api.ContainerStatusNone, false, false)

	// PULLED -> NONE transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerPulled, api.ContainerStatusNone,
		api.ContainerStatusNone, false, false)

	// PULLED -> STOPPED transition will result in STOPPED and is allowed, but not
	// actionable, when desired is STOPPED
	testContainerNextStateAssertions(t,
		api.ContainerPulled, api.ContainerStopped,
		api.ContainerStopped, false, true)
}

func TestContainerNextStateFromCreated(t *testing.T) {
	// CREATED -> RUNNING transition is allowed and actionable, when desired is Running
	// The exptected next status is Running
	testContainerNextStateAssertions(t,
		api.ContainerCreated, api.ContainerRunning,
		api.ContainerRunning, true, true)

	// CREATED -> CREATED transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerCreated, api.ContainerCreated,
		api.ContainerStatusNone, false, false)

	// CREATED -> NONE transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerCreated, api.ContainerStatusNone,
		api.ContainerStatusNone, false, false)

	// CREATED -> PULLED transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerCreated, api.ContainerPulled,
		api.ContainerStatusNone, false, false)

	// CREATED -> STOPPED transition will result in STOPPED and is allowed, but not
	// actionable, when desired is STOPPED
	testContainerNextStateAssertions(t,
		api.ContainerCreated, api.ContainerStopped,
		api.ContainerStopped, false, true)
}

func TestContainerNextStateFromRunning(t *testing.T) {
	// RUNNING -> STOPPED transition is allowed and actionable, when desired is Running
	// The exptected next status is STOPPED
	testContainerNextStateAssertions(t,
		api.ContainerRunning, api.ContainerStopped,
		api.ContainerStopped, true, true)

	// RUNNING -> RUNNING transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerRunning, api.ContainerRunning,
		api.ContainerStatusNone, false, false)

	// RUNNING -> NONE transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerRunning, api.ContainerStatusNone,
		api.ContainerStatusNone, false, false)

	// RUNNING -> PULLED transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerRunning, api.ContainerPulled,
		api.ContainerStatusNone, false, false)

	// RUNNING -> CREATED transition is not allowed and not actionable,
	// when desired is Running
	testContainerNextStateAssertions(t,
		api.ContainerRunning, api.ContainerCreated,
		api.ContainerStatusNone, false, false)
}

func testContainerNextStateAssertions(t *testing.T,
	containerCurrentStatus api.ContainerStatus,
	containerDesiredStatus api.ContainerStatus,
	expectedContainerStatus api.ContainerStatus,
	expectedTransitionActionable bool,
	expectedTransitionPossible bool) {
	container := &api.Container{
		DesiredStatusUnsafe: containerDesiredStatus,
		KnownStatusUnsafe:   containerCurrentStatus,
	}
	task := &managedTask{
		Task: &api.Task{
			Containers: []*api.Container{
				container,
			},
			DesiredStatusUnsafe: api.TaskRunning,
		},
	}
	nextStatus, actionRequired, possible := task.containerNextState(container)
	assert.Equal(t, nextStatus, expectedContainerStatus,
		"Retrieved next state [%s] != Expected next state [%s]",
		nextStatus.String(), expectedContainerStatus.String())
	assert.Equal(t, actionRequired, expectedTransitionActionable)
	assert.Equal(t, possible, expectedTransitionPossible)
}

func TestCleanupTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := NewMockDockerClient(ctrl)
	mockImageManager := NewMockImageManager(ctrl)
	defer ctrl.Finish()

	taskEngine := &DockerTaskEngine{
		saver:        statemanager.NewNoopStateManager(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mTask := &managedTask{
		Task:           testdata.LoadTask("sleep5"),
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
	mTask.cleanupTask(taskStoppedDuration)
}

func TestCleanupTaskWaitsForStoppedSent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := NewMockDockerClient(ctrl)
	mockImageManager := NewMockImageManager(ctrl)
	defer ctrl.Finish()

	taskEngine := &DockerTaskEngine{
		saver:        statemanager.NewNoopStateManager(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mTask := &managedTask{
		Task:           testdata.LoadTask("sleep5"),
		_time:          mockTime,
		engine:         taskEngine,
		acsMessages:    make(chan acsTransition),
		dockerMessages: make(chan dockerContainerChange),
	}
	mTask.SetKnownStatus(api.TaskStopped)
	mTask.SetSentStatus(api.TaskRunning)
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
	timesCalled := 0
	callsExpected := 3
	mockTime.EXPECT().Sleep(gomock.Any()).AnyTimes().Do(func(_ interface{}) {
		timesCalled++
		if timesCalled == callsExpected {
			mTask.SetSentStatus(api.TaskStopped)
		} else if timesCalled > callsExpected {
			t.Errorf("Sleep called too many times, called %d but expected %d", timesCalled, callsExpected)
		}
	})
	assert.Equal(t, api.TaskRunning, mTask.GetSentStatus())

	// Expectations to verify that the task gets removed
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*api.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mTask.cleanupTask(taskStoppedDuration)
	assert.Equal(t, api.TaskStopped, mTask.GetSentStatus())
}

func TestCleanupTaskGivesUpIfWaitingTooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := NewMockDockerClient(ctrl)
	mockImageManager := NewMockImageManager(ctrl)
	defer ctrl.Finish()

	taskEngine := &DockerTaskEngine{
		saver:        statemanager.NewNoopStateManager(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mTask := &managedTask{
		Task:           testdata.LoadTask("sleep5"),
		_time:          mockTime,
		engine:         taskEngine,
		acsMessages:    make(chan acsTransition),
		dockerMessages: make(chan dockerContainerChange),
	}
	mTask.SetKnownStatus(api.TaskStopped)
	mTask.SetSentStatus(api.TaskRunning)

	// Expectations for triggering cleanup
	now := mTask.GetKnownStatusTime()
	taskStoppedDuration := 1 * time.Minute
	mockTime.EXPECT().Now().Return(now).AnyTimes()
	cleanupTimeTrigger := make(chan time.Time)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanupTimeTrigger)
	go func() {
		cleanupTimeTrigger <- now
	}()
	_maxStoppedWaitTimes = 10
	defer func() {
		// reset
		_maxStoppedWaitTimes = int(maxStoppedWaitTimes)
	}()
	mockTime.EXPECT().Sleep(gomock.Any()).Times(_maxStoppedWaitTimes)
	assert.Equal(t, api.TaskRunning, mTask.GetSentStatus())

	// No cleanup expected
	mTask.cleanupTask(taskStoppedDuration)
	assert.Equal(t, api.TaskRunning, mTask.GetSentStatus())
}
