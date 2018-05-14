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

	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/golang/mock/gomock"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/stretchr/testify/assert"
)

// These tests use cgroup resource, which is linux specific.
// generic resource's(eg.volume) tests should be added to common test file.
func TestHandleResourceStateChangeAndSave(t *testing.T) {
	testCases := []struct {
		Name               string
		KnownStatus        taskresource.ResourceStatus
		DesiredKnownStatus taskresource.ResourceStatus
		Err                error
		ChangedKnownStatus taskresource.ResourceStatus
		TaskDesiredStatus  apitask.TaskStatus
	}{
		{
			Name:               "error while steady state transition",
			KnownStatus:        taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			DesiredKnownStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			Err:                errors.New("transition error"),
			ChangedKnownStatus: taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			TaskDesiredStatus:  apitask.TaskStopped,
		},
		{
			Name:               "steady state transition",
			KnownStatus:        taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			DesiredKnownStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			Err:                nil,
			ChangedKnownStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			TaskDesiredStatus:  apitask.TaskRunning,
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
					DesiredStatusUnsafe: apitask.TaskRunning,
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
		KnownStatus        taskresource.ResourceStatus
		DesiredKnownStatus taskresource.ResourceStatus
		Err                error
		ChangedKnownStatus taskresource.ResourceStatus
		TaskDesiredStatus  apitask.TaskStatus
	}{
		{
			Name:               "steady state transition already done",
			KnownStatus:        taskresource.ResourceStatus(cgroup.CgroupCreated),
			DesiredKnownStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			Err:                nil,
			ChangedKnownStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			TaskDesiredStatus:  apitask.TaskRunning,
		},
		{
			Name:               "transition state less than known status",
			DesiredKnownStatus: taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			Err:                nil,
			KnownStatus:        taskresource.ResourceStatus(cgroup.CgroupCreated),
			ChangedKnownStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			TaskDesiredStatus:  apitask.TaskRunning,
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
					DesiredStatusUnsafe: apitask.TaskRunning,
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
		ResKnownStatus   taskresource.ResourceStatus
		ResDesiredStatus taskresource.ResourceStatus
		NextState        taskresource.ResourceStatus
		ActionRequired   bool
	}{
		{
			Name:             "next state happy path",
			ResKnownStatus:   taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			ResDesiredStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			NextState:        taskresource.ResourceStatus(cgroup.CgroupCreated),
			ActionRequired:   true,
		},
		{
			Name:             "desired terminal",
			ResKnownStatus:   taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			ResDesiredStatus: taskresource.ResourceStatus(cgroup.CgroupRemoved),
			NextState:        taskresource.ResourceStatus(cgroup.CgroupRemoved),
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
		ResKnownStatus   taskresource.ResourceStatus
		ResDesiredStatus taskresource.ResourceStatus
		TransitionStatus taskresource.ResourceStatus
		StatusString     string
		CanTransition    bool
		TransitionsLen   int
	}{
		{
			Name:             "none to created",
			ResKnownStatus:   taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			ResDesiredStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			TransitionStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
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
					DesiredStatusUnsafe: apitask.TaskRunning,
				},
			}
			task.AddResource("cgroup", res)
			wg := sync.WaitGroup{}
			wg.Add(1)
			canTransition, transitions := task.startResourceTransitions(
				func(resource taskresource.TaskResource, nextStatus taskresource.ResourceStatus) {
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
		KnownStatus   taskresource.ResourceStatus
		DesiredStatus taskresource.ResourceStatus
		CanTransition bool
	}{
		{
			Name:          "known < desired",
			KnownStatus:   taskresource.ResourceStatus(cgroup.CgroupCreated),
			DesiredStatus: taskresource.ResourceStatus(cgroup.CgroupRemoved),
			CanTransition: true,
		},
		{
			Name:          "known equals desired",
			KnownStatus:   taskresource.ResourceStatus(cgroup.CgroupCreated),
			DesiredStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
			CanTransition: false,
		},
		{
			Name:          "known > desired",
			KnownStatus:   taskresource.ResourceStatus(cgroup.CgroupRemoved),
			DesiredStatus: taskresource.ResourceStatus(cgroup.CgroupCreated),
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
					DesiredStatusUnsafe: apitask.TaskRunning,
				},
				ctx: ctx,
				resourceStateChangeEvent: make(chan resourceStateChange),
			}
			mtask.Task.AddResource("cgroup", res)
			canTransition, transitions := mtask.startResourceTransitions(
				func(resource taskresource.TaskResource, nextStatus taskresource.ResourceStatus) {
					t.Error("Transition function should not be called when no transitions are possible")
				})
			assert.Equal(t, tc.CanTransition, canTransition)
			assert.Empty(t, transitions)
		})
	}
}
