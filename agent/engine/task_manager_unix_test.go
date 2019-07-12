// +build linux,unit

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
	"sync"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	mock_statemanager "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/efs"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/golang/mock/gomock"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/stretchr/testify/assert"
)

// These tests use cgroup resource, which is linux specific.
// generic resource's(eg.volume) tests should be added to common test file.
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
			mockSaver := mock_statemanager.NewMockStateManager(ctrl)
			res := &cgroup.CgroupResource{}
			res.SetKnownStatus(tc.KnownStatus)
			mtask := managedTask{
				Task: &apitask.Task{
					Arn:                 "task1",
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				engine: &DockerTaskEngine{},
			}
			mtask.AddResource("cgroup", res)
			mtask.engine.SetSaver(mockSaver)
			gomock.InOrder(
				mockSaver.EXPECT().Save(),
			)
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

// TestEFSResourceNextState uses EFS as an example of task resource with dependency on task network
func TestEFSResourceNextState(t *testing.T) {
	testCases := []struct {
		Name             string
		ResKnownStatus   resourcestatus.ResourceStatus
		ResAppliedStatus resourcestatus.ResourceStatus
		ResDesiredStatus resourcestatus.ResourceStatus
		NextState        resourcestatus.ResourceStatus
		ActionRequired   bool
	}{
		// None => Created
		{"none to created", resourcestatus.ResourceStatus(efs.EFSStatusNone), resourcestatus.ResourceStatus(efs.EFSStatusNone), resourcestatus.ResourceStatus(efs.EFSCreated), resourcestatus.ResourceStatus(efs.EFSCreated), true},
		// None => Removed, applied status is None
		{"none to removed", resourcestatus.ResourceStatus(efs.EFSStatusNone), resourcestatus.ResourceStatus(efs.EFSStatusNone), resourcestatus.ResourceStatus(efs.EFSRemoved), resourcestatus.ResourceStatus(efs.EFSRemoved), false},
		// None => Removed, applied status is Created
		{"none to removed, applied created", resourcestatus.ResourceStatus(efs.EFSStatusNone), resourcestatus.ResourceStatus(efs.EFSCreated), resourcestatus.ResourceStatus(efs.EFSRemoved), resourcestatus.ResourceStatus(efs.EFSStatusNone), false},
		// Created => Created
		{"created to created", resourcestatus.ResourceStatus(efs.EFSCreated), resourcestatus.ResourceStatus(efs.EFSStatusNone), resourcestatus.ResourceStatus(efs.EFSCreated), resourcestatus.ResourceStatus(efs.EFSStatusNone), false},
		// Created => Removed
		{"created to removed", resourcestatus.ResourceStatus(efs.EFSCreated), resourcestatus.ResourceStatus(efs.EFSStatusNone), resourcestatus.ResourceStatus(efs.EFSRemoved), resourcestatus.ResourceStatus(efs.EFSRemoved), true},
		// Removed => Created
		{"removed to created", resourcestatus.ResourceStatus(efs.EFSRemoved), resourcestatus.ResourceStatus(efs.EFSStatusNone), resourcestatus.ResourceStatus(efs.EFSCreated), resourcestatus.ResourceStatus(efs.EFSStatusNone), false},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			res := efs.EFSResource{}
			res.SetKnownStatus(tc.ResKnownStatus)
			res.SetDesiredStatus(tc.ResDesiredStatus)
			res.SetAppliedStatus(tc.ResAppliedStatus)
			mtask := managedTask{
				Task: &apitask.Task{},
			}
			transition := mtask.resourceNextState(&res)
			assert.Equal(t, tc.NextState, transition.nextState)
			assert.Equal(t, tc.ActionRequired, transition.actionRequired)
		})
	}
}

//TestEFSNextStateWithTransitionDependencies verifies the dependencies are resolved correctly for task resource
func TestEFSNextStateWithTransitionDependencies(t *testing.T) {
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
			resCurrentStatus:             resourcestatus.ResourceStatus(efs.EFSStatusNone),
			resDesiredStatus:             resourcestatus.ResourceStatus(efs.EFSCreated),
			resDependentStatus:           resourcestatus.ResourceStatus(efs.EFSCreated),
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerResourcesProvisioned,
			expectedResourceStatus:       resourcestatus.ResourceStatus(efs.EFSStatusNone),
			expectedTransitionActionable: false,
			reason:                       dependencygraph.ErrContainerDependencyNotResolvedForResource,
		},
		// NONE -> CREATED transition is allowed and actionable
		{
			name:                         "created depends on resourceProvisioned, dependency is resourceProvisioned",
			resCurrentStatus:             resourcestatus.ResourceStatus(efs.EFSStatusNone),
			resDesiredStatus:             resourcestatus.ResourceStatus(efs.EFSCreated),
			resDependentStatus:           resourcestatus.ResourceStatus(efs.EFSCreated),
			dependencyCurrentStatus:      apicontainerstatus.ContainerResourcesProvisioned,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerResourcesProvisioned,
			expectedResourceStatus:       resourcestatus.ResourceStatus(efs.EFSCreated),
			expectedTransitionActionable: true,
		},
		// CREATED -> REMOVED transition is allowed and actionable
		{
			name:                         "removed depends on stopped, dependency is stopped",
			resCurrentStatus:             resourcestatus.ResourceStatus(efs.EFSCreated),
			resDesiredStatus:             resourcestatus.ResourceStatus(efs.EFSRemoved),
			resDependentStatus:           resourcestatus.ResourceStatus(efs.EFSRemoved),
			dependencyCurrentStatus:      apicontainerstatus.ContainerStopped,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerStopped,
			expectedResourceStatus:       resourcestatus.ResourceStatus(efs.EFSRemoved),
			expectedTransitionActionable: true,
		},
		// NONE -> REMOVED transition is allowed and not actionable
		{
			name:                         "created depends on created, desired is stopped, dependency is created",
			resCurrentStatus:             resourcestatus.ResourceStatus(efs.EFSStatusNone),
			resDesiredStatus:             resourcestatus.ResourceStatus(efs.EFSRemoved),
			resDependentStatus:           resourcestatus.ResourceStatus(efs.EFSCreated),
			dependencyCurrentStatus:      apicontainerstatus.ContainerCreated,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerCreated,
			expectedResourceStatus:       resourcestatus.ResourceStatus(efs.EFSRemoved),
			expectedTransitionActionable: false,
		},
		// NONE -> REMOVED transition is not allowed and not actionable
		{
			name:                         "created depends on created, desired is stopped, dependency is none",
			resCurrentStatus:             resourcestatus.ResourceStatus(efs.EFSStatusNone),
			resDesiredStatus:             resourcestatus.ResourceStatus(efs.EFSRemoved),
			resDependentStatus:           resourcestatus.ResourceStatus(efs.EFSCreated),
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerCreated,
			expectedResourceStatus:       resourcestatus.ResourceStatus(efs.EFSStatusNone),
			expectedTransitionActionable: false,
			reason:                       dependencygraph.ErrContainerDependencyNotResolvedForResource,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := efs.NewEFSResource("task1", "volume1", nil, nil, false, "", "", []string{}, false, "", "")
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
