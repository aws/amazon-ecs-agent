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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
	"golang.org/x/net/context"
)

func TestContainerNextState(t *testing.T) {
	testCases := []struct {
		containerCurrentStatus       api.ContainerStatus
		containerDesiredStatus       api.ContainerStatus
		expectedContainerStatus      api.ContainerStatus
		expectedTransitionActionable bool
		expectedTransitionPossible   bool
	}{
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running
		// The exptected next status is Pulled
		{api.ContainerStatusNone, api.ContainerRunning, api.ContainerPulled, true, true},
		// NONE -> NONE transition is not be allowed and is not actionable,
		// when desired is Running
		{api.ContainerStatusNone, api.ContainerStatusNone, api.ContainerStatusNone, false, false},
		// NONE -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{api.ContainerStatusNone, api.ContainerStopped, api.ContainerStopped, false, true},
		// PULLED -> RUNNING transition is allowed and actionable, when desired is Running
		// The exptected next status is Created
		{api.ContainerPulled, api.ContainerRunning, api.ContainerCreated, true, true},
		// PULLED -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerPulled, api.ContainerPulled, api.ContainerStatusNone, false, false},
		// PULLED -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerPulled, api.ContainerStatusNone, api.ContainerStatusNone, false, false},
		// PULLED -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{api.ContainerPulled, api.ContainerStopped, api.ContainerStopped, false, true},
		// CREATED -> RUNNING transition is allowed and actionable, when desired is Running
		// The exptected next status is Running
		{api.ContainerCreated, api.ContainerRunning, api.ContainerRunning, true, true},
		// CREATED -> CREATED transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerCreated, api.ContainerCreated, api.ContainerStatusNone, false, false},
		// CREATED -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerCreated, api.ContainerStatusNone, api.ContainerStatusNone, false, false},
		// CREATED -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerCreated, api.ContainerPulled, api.ContainerStatusNone, false, false},
		// CREATED -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{api.ContainerCreated, api.ContainerStopped, api.ContainerStopped, false, true},
		// RUNNING -> STOPPED transition is allowed and actionable, when desired is Running
		// The exptected next status is STOPPED
		{api.ContainerRunning, api.ContainerStopped, api.ContainerStopped, true, true},
		// RUNNING -> RUNNING transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerRunning, api.ContainerRunning, api.ContainerStatusNone, false, false},
		// RUNNING -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerRunning, api.ContainerStatusNone, api.ContainerStatusNone, false, false},
		// RUNNING -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerRunning, api.ContainerPulled, api.ContainerStatusNone, false, false},
		// RUNNING -> CREATED transition is not allowed and not actionable,
		// when desired is Running
		{api.ContainerRunning, api.ContainerCreated, api.ContainerStatusNone, false, false},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s to %s Transition",
			tc.containerCurrentStatus.String(), tc.containerDesiredStatus.String()), func(t *testing.T) {
			container := &api.Container{
				DesiredStatusUnsafe: tc.containerDesiredStatus,
				KnownStatusUnsafe:   tc.containerCurrentStatus,
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
			assert.Equal(t, tc.expectedContainerStatus, nextStatus,
				"Expected next state [%s] != Retrieved next state [%s]",
				tc.expectedContainerStatus.String(), nextStatus.String())
			assert.Equal(t, tc.expectedTransitionActionable, actionRequired)
			assert.Equal(t, tc.expectedTransitionPossible, possible)
		})
	}
}

func TestStartContainerTransitionsWhenForwardTransitionPossible(t *testing.T) {
	firstContainerName := "container1"
	firstContainer := &api.Container{
		KnownStatusUnsafe:   api.ContainerCreated,
		DesiredStatusUnsafe: api.ContainerRunning,
		Name:                firstContainerName,
	}
	secondContainerName := "container2"
	secondContainer := &api.Container{
		KnownStatusUnsafe:   api.ContainerPulled,
		DesiredStatusUnsafe: api.ContainerRunning,
		Name:                secondContainerName,
	}
	task := &managedTask{
		Task: &api.Task{
			Containers: []*api.Container{
				firstContainer,
				secondContainer,
			},
			DesiredStatusUnsafe: api.TaskRunning,
		},
		engine: &DockerTaskEngine{},
	}

	waitForAssertions := sync.WaitGroup{}
	waitForAssertions.Add(2)
	canTransition, transitions := task.startContainerTransitions(
		func(cont *api.Container, nextStatus api.ContainerStatus) {
			if cont.Name == firstContainerName {
				assert.Equal(t, nextStatus, api.ContainerRunning)
			} else if cont.Name == secondContainerName {
				assert.Equal(t, nextStatus, api.ContainerCreated)
			}
			waitForAssertions.Done()
		})
	waitForAssertions.Wait()
	assert.True(t, canTransition)
	assert.NotEmpty(t, transitions)
	assert.Len(t, transitions, 2)
	firstContainerTransition, ok := transitions[firstContainerName]
	assert.True(t, ok)
	assert.Equal(t, firstContainerTransition, api.ContainerRunning)
	secondContainerTransition, ok := transitions[secondContainerName]
	assert.True(t, ok)
	assert.Equal(t, secondContainerTransition, api.ContainerCreated)
}

func TestStartContainerTransitionsWhenForwardTransitionIsNotPossible(t *testing.T) {
	firstContainerName := "container1"
	firstContainer := &api.Container{
		KnownStatusUnsafe:   api.ContainerRunning,
		DesiredStatusUnsafe: api.ContainerRunning,
		Name:                firstContainerName,
	}
	secondContainerName := "container2"
	secondContainer := &api.Container{
		KnownStatusUnsafe:   api.ContainerRunning,
		DesiredStatusUnsafe: api.ContainerRunning,
		Name:                secondContainerName,
	}
	task := &managedTask{
		Task: &api.Task{
			Containers: []*api.Container{
				firstContainer,
				secondContainer,
			},
			DesiredStatusUnsafe: api.TaskRunning,
		},
		engine: &DockerTaskEngine{},
	}

	canTransition, transitions := task.startContainerTransitions(
		func(cont *api.Container, nextStatus api.ContainerStatus) {
			t.Error("Transition function should not be called when no transitions are possible")
		})
	assert.False(t, canTransition)
	assert.Empty(t, transitions)
}

func TestStartContainerTransitionsInvokesHandleContainerChange(t *testing.T) {
	eventStreamName := "TESTTASKENGINE"

	// Create a container with the intent to do
	// CREATERD -> STOPPED transition. This triggers
	// `managedTask.handleContainerChange()` and generates the following
	// events:
	// 1. container state change event for Submit* API
	// 2. task state change event for Submit* API
	// 3. container state change event for the internal event stream
	firstContainerName := "container1"
	firstContainer := &api.Container{
		KnownStatusUnsafe:   api.ContainerCreated,
		DesiredStatusUnsafe: api.ContainerStopped,
		Name:                firstContainerName,
	}

	containerChangeEventStream := eventstream.NewEventStream(eventStreamName, context.Background())
	containerChangeEventStream.StartListening()

	containerEvents := make(chan api.ContainerStateChange)
	taskEvents := make(chan api.TaskStateChange)

	task := &managedTask{
		Task: &api.Task{
			Containers: []*api.Container{
				firstContainer,
			},
			DesiredStatusUnsafe: api.TaskRunning,
		},
		engine: &DockerTaskEngine{
			containerChangeEventStream: containerChangeEventStream,
			containerEvents:            containerEvents,
			taskEvents:                 taskEvents,
		},
	}

	eventsGenerated := sync.WaitGroup{}
	eventsGenerated.Add(3)
	containerChangeEventStream.Subscribe(eventStreamName, func(events ...interface{}) error {
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		event := events[0]
		containerChangeEvent, ok := event.(DockerContainerChangeEvent)
		assert.True(t, ok)
		assert.Equal(t, containerChangeEvent.Status, api.ContainerStopped)
		eventsGenerated.Done()
		return nil
	})
	defer containerChangeEventStream.Unsubscribe(eventStreamName)

	go func() {
		<-containerEvents
		eventsGenerated.Done()
	}()

	go func() {
		<-taskEvents
		eventsGenerated.Done()
	}()

	canTransition, transitions := task.startContainerTransitions(
		func(cont *api.Container, nextStatus api.ContainerStatus) {
			t.Error("Invalid code path. The transition function should not be invoked when transitioning container from CREATED -> STOPPED")
		})
	assert.True(t, canTransition)
	assert.Empty(t, transitions)
	eventsGenerated.Wait()
}

func TestWaitForContainerTransitionsForNonTerminalTask(t *testing.T) {
	acsMessages := make(chan acsTransition)
	dockerMessages := make(chan dockerContainerChange)
	task := &managedTask{
		acsMessages:    acsMessages,
		dockerMessages: dockerMessages,
		Task: &api.Task{
			Containers: []*api.Container{},
		},
	}

	transitionChange := make(chan bool, 2)
	transitionChangeContainer := make(chan string, 2)

	firstContainerName := "container1"
	secondContainerName := "container2"

	// populate the transitions map with transitions for two
	// containers. We expect two sets of events to be consumed
	// by `waitForContainerTransitions`
	transitions := make(map[string]api.ContainerStatus)
	transitions[firstContainerName] = api.ContainerRunning
	transitions[secondContainerName] = api.ContainerRunning

	go func() {
		// Send "transitions completed" messages. These are being
		// sent out of order for no particular reason. We should be
		// resilient to the ordering of these messages anyway.
		transitionChange <- true
		transitionChangeContainer <- secondContainerName
		transitionChange <- true
		transitionChangeContainer <- firstContainerName
	}()

	// waitForContainerTransitions will block until it recieves events
	// sent by the go routine defined above
	task.waitForContainerTransitions(transitions, transitionChange, transitionChangeContainer)
}

// TestWaitForContainerTransitionsForTerminalTask verifies that the
// `waitForContainerTransitions` method doesn't wait for any container
// transitions when the task's desired status is STOPPED and if all
// containers in the task are in PULLED state
func TestWaitForContainerTransitionsForTerminalTask(t *testing.T) {
	acsMessages := make(chan acsTransition)
	dockerMessages := make(chan dockerContainerChange)
	task := &managedTask{
		acsMessages:    acsMessages,
		dockerMessages: dockerMessages,
		Task: &api.Task{
			Containers:        []*api.Container{},
			KnownStatusUnsafe: api.TaskStopped,
		},
	}

	transitionChange := make(chan bool, 2)
	transitionChangeContainer := make(chan string, 2)

	firstContainerName := "container1"
	secondContainerName := "container2"
	transitions := make(map[string]api.ContainerStatus)
	transitions[firstContainerName] = api.ContainerPulled
	transitions[secondContainerName] = api.ContainerPulled

	// Event though there are two keys in the transitions map, send
	// only one event. This tests that `waitForContainerTransitions` doesn't
	// block to recieve two events and will still progress
	go func() {
		transitionChange <- true
		transitionChangeContainer <- secondContainerName
	}()
	task.waitForContainerTransitions(transitions, transitionChange, transitionChangeContainer)
}

func TestOnContainersUnableToTransitionStateForDesiredStoppedTask(t *testing.T) {
	taskEvents := make(chan api.TaskStateChange)
	task := &managedTask{
		Task: &api.Task{
			Containers:          []*api.Container{},
			DesiredStatusUnsafe: api.TaskStopped,
		},
		engine: &DockerTaskEngine{
			taskEvents: taskEvents,
		},
	}
	eventsGenerated := sync.WaitGroup{}
	eventsGenerated.Add(1)

	go func() {
		event := <-taskEvents
		assert.Equal(t, event.Reason, taskUnableToTransitionToStoppedReason)
		eventsGenerated.Done()
	}()

	task.onContainersUnableToTransitionState()
	eventsGenerated.Wait()

	assert.Equal(t, task.GetDesiredStatus(), api.TaskStopped)
}

func TestOnContainersUnableToTransitionStateForDesiredRunningTask(t *testing.T) {
	firstContainerName := "container1"
	firstContainer := &api.Container{
		KnownStatusUnsafe:   api.ContainerCreated,
		DesiredStatusUnsafe: api.ContainerRunning,
		Name:                firstContainerName,
	}
	task := &managedTask{
		Task: &api.Task{
			Containers: []*api.Container{
				firstContainer,
			},
			DesiredStatusUnsafe: api.TaskRunning,
		},
	}

	task.onContainersUnableToTransitionState()
	assert.Equal(t, task.GetDesiredStatus(), api.TaskStopped)
	assert.Equal(t, task.Containers[0].GetDesiredStatus(), api.ContainerStopped)
}

// TODO: Test progressContainers workflow
// TODO: Test handleStoppedToRunningContainerTransition

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
