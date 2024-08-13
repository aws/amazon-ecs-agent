//go:build linux && unit
// +build linux,unit

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

package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	mock_ttime "github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// These tests use cgroup resource, which is linux specific.
// generic resources (e.g. volume) tests should be added to common test file.
func TestHandleResourceStateChangeAndSave(t *testing.T) {
	testCases := []struct {
		Name               string
		KnownStatus        resourcestatus.ResourceStatus
		DesiredKnownStatus resourcestatus.ResourceStatus
		Err                error
		ChangedKnownStatus resourcestatus.ResourceStatus
		TaskDesiredStatus  apitaskstatus.TaskStatus
	}{
		{
			Name:               "error while steady state transition",
			KnownStatus:        resourcestatus.ResourceStatus(cgroup.CgroupStatusNone),
			DesiredKnownStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			Err:                errors.New("transition error"),
			ChangedKnownStatus: resourcestatus.ResourceStatus(cgroup.CgroupStatusNone),
			TaskDesiredStatus:  apitaskstatus.TaskStopped,
		},
		{
			Name:               "steady state transition",
			KnownStatus:        resourcestatus.ResourceStatus(cgroup.CgroupStatusNone),
			DesiredKnownStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			Err:                nil,
			ChangedKnownStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			TaskDesiredStatus:  apitaskstatus.TaskRunning,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			res := &cgroup.CgroupResource{}
			res.SetKnownStatus(tc.KnownStatus)
			mtask := managedTask{
				Task: &apitask.Task{
					Arn:                 "task1",
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				engine: &DockerTaskEngine{
					dataClient: data.NewNoopClient(),
				},
			}
			mtask.AddResource("cgroup", res)
			mtask.handleResourceStateChange(resourceStateChange{
				res, tc.DesiredKnownStatus, tc.Err,
			})
			assert.Equal(t, tc.ChangedKnownStatus, res.GetKnownStatus())
			assert.Equal(t, tc.TaskDesiredStatus, mtask.GetDesiredStatus())
		})
	}
}

func TestHandleResourceStateChangeNoSave(t *testing.T) {
	testCases := []struct {
		Name               string
		KnownStatus        resourcestatus.ResourceStatus
		DesiredKnownStatus resourcestatus.ResourceStatus
		Err                error
		ChangedKnownStatus resourcestatus.ResourceStatus
		TaskDesiredStatus  apitaskstatus.TaskStatus
	}{
		{
			Name:               "steady state transition already done",
			KnownStatus:        resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			DesiredKnownStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			Err:                nil,
			ChangedKnownStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			TaskDesiredStatus:  apitaskstatus.TaskRunning,
		},
		{
			Name:               "transition state less than known status",
			DesiredKnownStatus: resourcestatus.ResourceStatus(cgroup.CgroupStatusNone),
			Err:                nil,
			KnownStatus:        resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			ChangedKnownStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			TaskDesiredStatus:  apitaskstatus.TaskRunning,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			res := &cgroup.CgroupResource{}
			res.SetKnownStatus(tc.KnownStatus)
			mtask := managedTask{
				Task: &apitask.Task{
					Arn:                 "task1",
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
			}
			mtask.AddResource("cgroup", res)
			mtask.handleResourceStateChange(resourceStateChange{
				res, tc.DesiredKnownStatus, tc.Err,
			})
			assert.Equal(t, tc.ChangedKnownStatus, res.GetKnownStatus())
			assert.Equal(t, tc.TaskDesiredStatus, mtask.GetDesiredStatus())
		})
	}
}

func TestResourceNextState(t *testing.T) {
	testCases := []struct {
		Name             string
		ResKnownStatus   resourcestatus.ResourceStatus
		ResDesiredStatus resourcestatus.ResourceStatus
		NextState        resourcestatus.ResourceStatus
		ActionRequired   bool
	}{
		{
			Name:             "next state happy path",
			ResKnownStatus:   resourcestatus.ResourceStatus(cgroup.CgroupStatusNone),
			ResDesiredStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			NextState:        resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			ActionRequired:   true,
		},
		{
			Name:             "desired terminal",
			ResKnownStatus:   resourcestatus.ResourceStatus(cgroup.CgroupStatusNone),
			ResDesiredStatus: resourcestatus.ResourceStatus(cgroup.CgroupRemoved),
			NextState:        resourcestatus.ResourceStatus(cgroup.CgroupRemoved),
			ActionRequired:   false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			res := cgroup.CgroupResource{}
			res.SetKnownStatus(tc.ResKnownStatus)
			res.SetDesiredStatus(tc.ResDesiredStatus)
			mtask := managedTask{
				Task: &apitask.Task{},
			}
			transition := mtask.resourceNextState(&res)
			assert.Equal(t, tc.NextState, transition.nextState)
			assert.Equal(t, tc.ActionRequired, transition.actionRequired)
		})
	}
}

func TestStartResourceTransitionsHappyPath(t *testing.T) {
	testCases := []struct {
		Name             string
		ResKnownStatus   resourcestatus.ResourceStatus
		ResDesiredStatus resourcestatus.ResourceStatus
		TransitionStatus resourcestatus.ResourceStatus
		StatusString     string
		CanTransition    bool
		TransitionsLen   int
	}{
		{
			Name:             "none to created",
			ResKnownStatus:   resourcestatus.ResourceStatus(cgroup.CgroupStatusNone),
			ResDesiredStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			TransitionStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			StatusString:     "CREATED",
			CanTransition:    true,
			TransitionsLen:   1,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			res := &cgroup.CgroupResource{}
			res.SetKnownStatus(tc.ResKnownStatus)
			res.SetDesiredStatus(tc.ResDesiredStatus)

			task := &managedTask{
				Task: &apitask.Task{
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
			}
			task.AddResource("cgroup", res)
			wg := sync.WaitGroup{}
			wg.Add(1)
			canTransition, transitions := task.startResourceTransitions(
				func(resource taskresource.TaskResource, nextStatus resourcestatus.ResourceStatus) {
					assert.Equal(t, nextStatus, tc.TransitionStatus)
					wg.Done()
				})
			wg.Wait()
			assert.Equal(t, tc.CanTransition, canTransition)
			assert.Len(t, transitions, tc.TransitionsLen)
			resTransition, ok := transitions["cgroup"]
			assert.True(t, ok)
			assert.Equal(t, resTransition, tc.StatusString)
		})
	}
}

func TestStartResourceTransitionsEmpty(t *testing.T) {
	testCases := []struct {
		Name          string
		KnownStatus   resourcestatus.ResourceStatus
		DesiredStatus resourcestatus.ResourceStatus
		CanTransition bool
	}{
		{
			Name:          "known < desired",
			KnownStatus:   resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			DesiredStatus: resourcestatus.ResourceStatus(cgroup.CgroupRemoved),
			CanTransition: true,
		},
		{
			Name:          "known equals desired",
			KnownStatus:   resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			DesiredStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			CanTransition: false,
		},
		{
			Name:          "known > desired",
			KnownStatus:   resourcestatus.ResourceStatus(cgroup.CgroupRemoved),
			DesiredStatus: resourcestatus.ResourceStatus(cgroup.CgroupCreated),
			CanTransition: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			res := &cgroup.CgroupResource{}
			res.SetKnownStatus(tc.KnownStatus)
			res.SetDesiredStatus(tc.DesiredStatus)

			mtask := &managedTask{
				Task: &apitask.Task{
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				ctx:                      ctx,
				resourceStateChangeEvent: make(chan resourceStateChange),
			}
			mtask.Task.AddResource("cgroup", res)
			canTransition, transitions := mtask.startResourceTransitions(
				func(resource taskresource.TaskResource, nextStatus resourcestatus.ResourceStatus) {
					t.Error("Transition function should not be called when no transitions are possible")
				})
			assert.Equal(t, tc.CanTransition, canTransition)
			assert.Empty(t, transitions)
		})
	}
}

// TestEFSNextStateWithTransitionDependencies verifies the dependencies are resolved correctly for task resource
func TestEFSVolumeNextStateWithTransitionDependencies(t *testing.T) {
	testCases := []struct {
		name                         string
		resCurrentStatus             resourcestatus.ResourceStatus
		resDesiredStatus             resourcestatus.ResourceStatus
		resDependentStatus           resourcestatus.ResourceStatus
		dependencyCurrentStatus      apicontainerstatus.ContainerStatus
		dependencySatisfiedStatus    apicontainerstatus.ContainerStatus
		expectedResourceStatus       resourcestatus.ResourceStatus
		expectedTransitionActionable bool
		reason                       error
	}{
		// NONE -> CREATED transition is not allowed and not actionable
		{
			name:                         "created depends on resourceProvisioned, dependency is none",
			resCurrentStatus:             resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			resDesiredStatus:             resourcestatus.ResourceStatus(volume.VolumeCreated),
			resDependentStatus:           resourcestatus.ResourceStatus(volume.VolumeCreated),
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerResourcesProvisioned,
			expectedResourceStatus:       resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			expectedTransitionActionable: false,
			reason:                       dependencygraph.ErrContainerDependencyNotResolvedForResource,
		},
		// NONE -> CREATED transition is allowed and actionable
		{
			name:                         "created depends on resourceProvisioned, dependency is resourceProvisioned",
			resCurrentStatus:             resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			resDesiredStatus:             resourcestatus.ResourceStatus(volume.VolumeCreated),
			resDependentStatus:           resourcestatus.ResourceStatus(volume.VolumeCreated),
			dependencyCurrentStatus:      apicontainerstatus.ContainerResourcesProvisioned,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerResourcesProvisioned,
			expectedResourceStatus:       resourcestatus.ResourceStatus(volume.VolumeCreated),
			expectedTransitionActionable: true,
		},
		// CREATED -> REMOVED transition is allowed and actionable
		{
			name:                         "removed depends on stopped, dependency is stopped",
			resCurrentStatus:             resourcestatus.ResourceStatus(volume.VolumeCreated),
			resDesiredStatus:             resourcestatus.ResourceStatus(volume.VolumeRemoved),
			resDependentStatus:           resourcestatus.ResourceStatus(volume.VolumeRemoved),
			dependencyCurrentStatus:      apicontainerstatus.ContainerStopped,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerStopped,
			expectedResourceStatus:       resourcestatus.ResourceStatus(volume.VolumeRemoved),
			expectedTransitionActionable: false,
		},
		// NONE -> REMOVED transition is allowed and not actionable
		{
			name:                         "created depends on created, desired is stopped, dependency is created",
			resCurrentStatus:             resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			resDesiredStatus:             resourcestatus.ResourceStatus(volume.VolumeRemoved),
			resDependentStatus:           resourcestatus.ResourceStatus(volume.VolumeCreated),
			dependencyCurrentStatus:      apicontainerstatus.ContainerCreated,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerCreated,
			expectedResourceStatus:       resourcestatus.ResourceStatus(volume.VolumeRemoved),
			expectedTransitionActionable: false,
		},
		// NONE -> REMOVED transition is not allowed and not actionable
		{
			name:                         "created depends on created, desired is stopped, dependency is none",
			resCurrentStatus:             resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			resDesiredStatus:             resourcestatus.ResourceStatus(volume.VolumeRemoved),
			resDependentStatus:           resourcestatus.ResourceStatus(volume.VolumeCreated),
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerCreated,
			expectedResourceStatus:       resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			expectedTransitionActionable: false,
			reason:                       dependencygraph.ErrContainerDependencyNotResolvedForResource,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			res, _ := volume.NewVolumeResource(ctx, "task1", apitask.EFSVolumeType, "volume1", "", false, "", nil, nil, nil)
			dependencyName := "dependency"
			dependency := &apicontainer.Container{
				Name:              dependencyName,
				KnownStatusUnsafe: tc.dependencyCurrentStatus,
			}
			res.BuildContainerDependency(dependencyName, tc.dependencySatisfiedStatus, tc.resDependentStatus)

			res.SetKnownStatus(tc.resCurrentStatus)
			res.SetDesiredStatus(tc.resDesiredStatus)
			mtask := managedTask{
				Task: &apitask.Task{
					Containers: []*apicontainer.Container{
						dependency,
					},
				},
			}
			transition := mtask.resourceNextState(res)
			assert.Equal(t, tc.expectedResourceStatus, transition.nextState,
				"Expected next state [%s] != Retrieved next state [%s]",
				res.StatusString(tc.expectedResourceStatus), res.StatusString(transition.nextState))
			assert.Equal(t, tc.expectedTransitionActionable, transition.actionRequired, "transition actionable")
			assert.Equal(t, tc.reason, transition.reason, "transition possible")
		})
	}
}

func TestCleanupExecEnabledTask(t *testing.T) {
	cfg := getTestConfig()
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	mockImageManager := mock_engine.NewMockImageManager(ctrl)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	taskEngine := &DockerTaskEngine{
		ctx:          ctx,
		cfg:          &cfg,
		dataClient:   data.NewNoopClient(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mTask := &managedTask{
		ctx:                      ctx,
		cancel:                   cancel,
		Task:                     testdata.LoadTask("sleep5"),
		_time:                    mockTime,
		engine:                   taskEngine,
		acsMessages:              make(chan acsTransition),
		dockerMessages:           make(chan dockerContainerChange),
		resourceStateChangeEvent: make(chan resourceStateChange),
		cfg:                      taskEngine.cfg,
	}
	container := mTask.Containers[0]
	enableExecCommandAgentForContainer(container, apicontainer.ManagedAgentState{})
	mTask.SetKnownStatus(apitaskstatus.TaskStopped)
	mTask.SetSentStatus(apitaskstatus.TaskStopped)

	dockerContainer := &apicontainer.DockerContainer{
		DockerName: "dockerContainer",
	}
	tID := mTask.Task.GetID()
	removeAll = func(path string) error {
		assert.Equal(t, fmt.Sprintf("/log/exec/%s", tID), path)
		return nil
	}
	defer func() {
		removeAll = os.RemoveAll
	}()
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
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*apicontainer.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mTask.cleanupTask(taskStoppedDuration)
}
