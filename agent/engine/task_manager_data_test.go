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

package engine

import (
	"context"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	resourcetype "github.com/aws/amazon-ecs-agent/agent/taskresource/types"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	utilsync "github.com/aws/amazon-ecs-agent/agent/utils/sync"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
This file contains unit tests that specifically check the data persisting behavior of the task manager.
*/

const (
	dataTestVolumeName     = "test-vol"
	dataTestContainerName1 = "test-name-1"
	dataTestContainerName2 = "test-name-2"
)

var (
	testErr = errors.New("test error")
)

func TestHandleDesiredStatusChangeSaveData(t *testing.T) {
	testCases := []struct {
		name                string
		desiredStatus       apitaskstatus.TaskStatus
		targetDesiredStatus apitaskstatus.TaskStatus
		shouldSave          bool
	}{
		{
			name:                "non-redundant desired status change is saved",
			desiredStatus:       apitaskstatus.TaskRunning,
			targetDesiredStatus: apitaskstatus.TaskStopped,
			shouldSave:          true,
		},
		{
			name:                "redundant desired status change is saved",
			desiredStatus:       apitaskstatus.TaskStopped,
			targetDesiredStatus: apitaskstatus.TaskStopped,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dataClient := newTestDataClient(t)

			mtask := managedTask{
				Task: &apitask.Task{
					Arn:                 testTaskARN,
					DesiredStatusUnsafe: tc.desiredStatus,
				},
				engine: &DockerTaskEngine{
					dataClient: dataClient,
				},
				taskStopWG: utilsync.NewSequentialWaitGroup(),
			}
			mtask.handleDesiredStatusChange(tc.targetDesiredStatus, int64(1))
			tasks, err := dataClient.GetTasks()
			require.NoError(t, err)
			if tc.shouldSave {
				assert.Len(t, tasks, 1)
			} else {
				assert.Len(t, tasks, 0)
			}
		})
	}
}

func TestHandleContainerStateChangeSaveData(t *testing.T) {
	testCases := []struct {
		name                string
		knownState          apicontainerstatus.ContainerStatus
		nextState           apicontainerstatus.ContainerStatus
		taskKnownState      apitaskstatus.TaskStatus
		taskDesiredState    apitaskstatus.TaskStatus
		err                 error
		shouldSaveContainer bool
	}{
		{
			name:                "non-redundant container state change is saved",
			knownState:          apicontainerstatus.ContainerCreated,
			nextState:           apicontainerstatus.ContainerRunning,
			taskKnownState:      apitaskstatus.TaskCreated,
			taskDesiredState:    apitaskstatus.TaskRunning,
			shouldSaveContainer: true,
		},
		{
			name:                "non-redundant container state change with error is saved",
			knownState:          apicontainerstatus.ContainerPulled,
			nextState:           apicontainerstatus.ContainerCreated,
			taskKnownState:      apitaskstatus.TaskPulled,
			taskDesiredState:    apitaskstatus.TaskCreated,
			err:                 testErr,
			shouldSaveContainer: true,
		},
		{
			name:             "redundant container state change is not saved",
			knownState:       apicontainerstatus.ContainerCreated,
			nextState:        apicontainerstatus.ContainerCreated,
			taskKnownState:   apitaskstatus.TaskCreated,
			taskDesiredState: apitaskstatus.TaskCreated,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			eventStreamName := "TestHandleContainerStateChangeSaveData"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			containerChangeEventStream := eventstream.NewEventStream(eventStreamName, ctx)
			containerChangeEventStream.StartListening()

			dataClient := newTestDataClient(t)

			mTask := managedTask{
				Task: &apitask.Task{
					Arn:                 testTaskARN,
					DesiredStatusUnsafe: tc.taskDesiredState,
					KnownStatusUnsafe:   tc.taskKnownState,
					Containers: []*apicontainer.Container{
						newTestContainer(dataTestContainerName1, tc.knownState, tc.nextState),
						newTestContainer(dataTestContainerName2, tc.knownState, tc.nextState),
					},
				},
				engine: &DockerTaskEngine{
					dataClient: dataClient,
				},
				ctx:                        context.TODO(),
				containerChangeEventStream: containerChangeEventStream,
				stateChangeEvents:          make(chan statechange.Event),
			}
			defer discardEvents(mTask.stateChangeEvents)()

			containerChange := dockerContainerChange{
				container: mTask.Containers[0],
				event: dockerapi.DockerContainerChangeEvent{
					Status:                  tc.nextState,
					DockerContainerMetadata: dockerapi.DockerContainerMetadata{},
				},
			}
			mTask.handleContainerChange(containerChange)
			containers, err := dataClient.GetContainers()
			require.NoError(t, err)
			if tc.shouldSaveContainer {
				assert.Len(t, containers, 1)
			} else {
				assert.Len(t, containers, 0)
			}

			// All test cases in this test are made such that the container state change is not
			// causing task state change. Check that the task is not saved in these case.
			tasks, err := dataClient.GetTasks()
			assert.Len(t, tasks, 0)
		})
	}
}

func TestHandleContainerChangeWithTaskStateChangeSaveData(t *testing.T) {
	eventStreamName := "TestHandleContainerChangeWithTaskStateChangeSaveData"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	containerChangeEventStream := eventstream.NewEventStream(eventStreamName, ctx)
	containerChangeEventStream.StartListening()

	dataClient := newTestDataClient(t)

	mTask := managedTask{
		Task: &apitask.Task{
			Arn:                 testTaskARN,
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
			KnownStatusUnsafe:   apitaskstatus.TaskCreated,
			Containers: []*apicontainer.Container{
				newTestContainer(dataTestContainerName1, apicontainerstatus.ContainerCreated,
					apicontainerstatus.ContainerRunning),
			},
		},
		engine: &DockerTaskEngine{
			dataClient: dataClient,
		},
		ctx:                        context.TODO(),
		containerChangeEventStream: containerChangeEventStream,
		stateChangeEvents:          make(chan statechange.Event),
	}
	defer discardEvents(mTask.stateChangeEvents)()

	containerChange := dockerContainerChange{
		container: mTask.Containers[0],
		event: dockerapi.DockerContainerChangeEvent{
			Status:                  apicontainerstatus.ContainerRunning,
			DockerContainerMetadata: dockerapi.DockerContainerMetadata{},
		},
	}
	mTask.handleContainerChange(containerChange)
	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)

	tasks, err := dataClient.GetTasks()
	assert.Len(t, tasks, 1)
}

func TestHandleResourceStateChangeSaveData(t *testing.T) {
	testCases := []struct {
		name       string
		knownState resourcestatus.ResourceStatus
		nextState  resourcestatus.ResourceStatus
		err        error
		shouldSave bool
	}{
		{
			name:       "non-redundant resource state change is saved",
			knownState: resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			nextState:  resourcestatus.ResourceStatus(volume.VolumeCreated),
			shouldSave: true,
		},
		{
			name:       "redundant resource state change is not saved",
			knownState: resourcestatus.ResourceStatus(volume.VolumeCreated),
			nextState:  resourcestatus.ResourceStatus(volume.VolumeCreated),
		},
		{
			name:       "non-redundant resource state change with error is saved",
			shouldSave: true,
			knownState: resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			nextState:  resourcestatus.ResourceStatus(volume.VolumeCreated),
			err:        testErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dataClient := newTestDataClient(t)

			res := &volume.VolumeResource{Name: dataTestVolumeName}
			res.SetKnownStatus(tc.knownState)
			mTask := managedTask{
				Task: &apitask.Task{
					Arn:                 testTaskARN,
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				engine: &DockerTaskEngine{
					dataClient: dataClient,
				},
			}
			mTask.AddResource(resourcetype.DockerVolumeKey, res)
			mTask.handleResourceStateChange(resourceStateChange{
				res, tc.nextState, tc.err,
			})

			tasks, err := dataClient.GetTasks()
			require.NoError(t, err)
			if tc.shouldSave {
				assert.Len(t, tasks, 1)
			} else {
				assert.Len(t, tasks, 0)
			}
		})
	}
}

func newTestContainer(name string, knownStatus, desiredStatus apicontainerstatus.ContainerStatus) *apicontainer.Container {
	return &apicontainer.Container{
		Name:                name,
		TaskARNUnsafe:       testTaskARN,
		KnownStatusUnsafe:   knownStatus,
		DesiredStatusUnsafe: desiredStatus,
	}
}
