// +build unit

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
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/mocks"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	utilsync "github.com/aws/amazon-ecs-agent/agent/utils/sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
)

func TestHandleEventError(t *testing.T) {
	testCases := []struct {
		Name                                  string
		EventStatus                           apicontainerstatus.ContainerStatus
		CurrentContainerKnownStatus           apicontainerstatus.ContainerStatus
		ImagePullBehavior                     config.ImagePullBehaviorType
		Error                                 apierrors.NamedError
		ExpectedContainerKnownStatusSet       bool
		ExpectedContainerKnownStatus          apicontainerstatus.ContainerStatus
		ExpectedContainerDesiredStatusStopped bool
		ExpectedTaskDesiredStatusStopped      bool
		ExpectedOK                            bool
	}{
		{
			Name:                        "Stop timed out",
			EventStatus:                 apicontainerstatus.ContainerStopped,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerRunning,
			Error: &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet: true,
			ExpectedContainerKnownStatus:    apicontainerstatus.ContainerRunning,
			ExpectedOK:                      false,
		},
		{
			Name:                        "Retriable error with stop",
			EventStatus:                 apicontainerstatus.ContainerStopped,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerRunning,
			Error: &dockerapi.CannotStopContainerError{
				FromError: errors.New(""),
			},
			ExpectedContainerKnownStatusSet: true,
			ExpectedContainerKnownStatus:    apicontainerstatus.ContainerRunning,
			ExpectedOK:                      false,
		},
		{
			Name:                        "Unretriable error with Stop",
			EventStatus:                 apicontainerstatus.ContainerStopped,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerRunning,
			Error: &dockerapi.CannotStopContainerError{
				FromError: &docker.ContainerNotRunning{},
			},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainerstatus.ContainerStopped,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            true,
		},
		{
			Name:  "Pull failed",
			Error: &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet: true,
			EventStatus:                     apicontainerstatus.ContainerPulled,
			ExpectedOK:                      true,
		},
		{
			Name:                        "Container vanished betweeen pull and running",
			EventStatus:                 apicontainerstatus.ContainerRunning,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerPulled,
			Error: &ContainerVanishedError{},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainerstatus.ContainerPulled,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Inspect failed during start",
			EventStatus:                 apicontainerstatus.ContainerRunning,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerCreated,
			Error: &dockerapi.CannotInspectContainerError{
				FromError: errors.New("error"),
			},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainerstatus.ContainerCreated,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Start timed out",
			EventStatus:                 apicontainerstatus.ContainerRunning,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerCreated,
			Error: &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainerstatus.ContainerCreated,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Inspect failed during create",
			EventStatus:                 apicontainerstatus.ContainerCreated,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerPulled,
			Error: &dockerapi.CannotInspectContainerError{
				FromError: errors.New("error"),
			},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainerstatus.ContainerPulled,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Create timed out",
			EventStatus:                 apicontainerstatus.ContainerCreated,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerPulled,
			Error: &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainerstatus.ContainerPulled,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:        "Pull image fails and task fails",
			EventStatus: apicontainerstatus.ContainerPulled,
			Error: &dockerapi.CannotPullContainerError{
				FromError: errors.New("error"),
			},
			ImagePullBehavior:                config.ImagePullAlwaysBehavior,
			ExpectedContainerKnownStatusSet:  false,
			ExpectedTaskDesiredStatusStopped: true,
			ExpectedOK:                       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			container := &apicontainer.Container{
				KnownStatusUnsafe: tc.CurrentContainerKnownStatus,
			}
			containerChange := dockerContainerChange{
				container: container,
				event: dockerapi.DockerContainerChangeEvent{
					Status: tc.EventStatus,
					DockerContainerMetadata: dockerapi.DockerContainerMetadata{
						Error: tc.Error,
					},
				},
			}
			mtask := managedTask{
				Task: &apitask.Task{
					Arn: "task1",
				},
				engine: &DockerTaskEngine{},
				cfg:    &config.Config{ImagePullBehavior: tc.ImagePullBehavior},
			}
			ok := mtask.handleEventError(containerChange, tc.CurrentContainerKnownStatus)
			assert.Equal(t, tc.ExpectedOK, ok, "to proceed")
			if tc.ExpectedContainerKnownStatusSet {
				containerKnownStatus := containerChange.container.GetKnownStatus()
				assert.Equal(t, tc.ExpectedContainerKnownStatus, containerKnownStatus,
					"expected container known status %s != %s", tc.ExpectedContainerKnownStatus.String(), containerKnownStatus.String())
			}
			if tc.ExpectedContainerDesiredStatusStopped {
				containerDesiredStatus := containerChange.container.GetDesiredStatus()
				assert.Equal(t, apicontainerstatus.ContainerStopped, containerDesiredStatus,
					"desired status %s != %s", apicontainerstatus.ContainerStopped.String(), containerDesiredStatus.String())
			}
			assert.Equal(t, tc.Error.ErrorName(), containerChange.container.ApplyingError.ErrorName())
		})
	}
}

func TestContainerNextState(t *testing.T) {
	testCases := []struct {
		containerCurrentStatus       apicontainerstatus.ContainerStatus
		containerDesiredStatus       apicontainerstatus.ContainerStatus
		expectedContainerStatus      apicontainerstatus.ContainerStatus
		expectedTransitionActionable bool
		reason                       error
	}{
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running
		// The expected next status is Pulled
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerPulled, true, nil},
		// NONE -> RESOURCES_PROVISIONED transition is allowed and actionable, when desired
		// is Running. The exptected next status is Pulled
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerPulled, true, nil},
		// NONE -> NONE transition is not be allowed and is not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// NONE -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, false, nil},
		// PULLED -> RUNNING transition is allowed and actionable, when desired is Running
		// The exptected next status is Created
		{apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerCreated, true, nil},
		// PULLED -> RESOURCES_PROVISIONED transition is allowed and actionable, when desired
		// is Running. The exptected next status is Created
		{apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerCreated, true, nil},
		// PULLED -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// PULLED -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// PULLED -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, false, nil},
		// CREATED -> RUNNING transition is allowed and actionable, when desired is Running
		// The expected next status is Running
		{apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning, true, nil},
		// CREATED -> RESOURCES_PROVISIONED transition is allowed and actionable, when desired
		// is Running. The exptected next status is Running
		{apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerRunning, true, nil},
		// CREATED -> CREATED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// CREATED -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// CREATED -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// CREATED -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, false, nil},
		// RUNNING -> STOPPED transition is allowed and actionable, when desired is Running
		// The expected next status is STOPPED
		{apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, true, nil},
		// RUNNING -> RUNNING transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RUNNING -> RESOURCES_PROVISIONED is allowed when steady state status is
		// RESOURCES_PROVISIONED and desired is RESOURCES_PROVISIONED
		{apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerResourcesProvisioned, true, nil},
		// RUNNING -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RUNNING -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RUNNING -> CREATED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},

		// RESOURCES_PROVISIONED -> RESOURCES_PROVISIONED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RESOURCES_PROVISIONED -> RUNNING transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RESOURCES_PROVISIONED -> STOPPED transition is allowed and actionable, when desired
		// is Running. The exptected next status is STOPPED
		{apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, true, nil},
		// RESOURCES_PROVISIONED -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RESOURCES_PROVISIONED -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RESOURCES_PROVISIONED -> CREATED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
	}

	steadyStates := []apicontainerstatus.ContainerStatus{apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerResourcesProvisioned}

	for _, tc := range testCases {
		for _, steadyState := range steadyStates {
			t.Run(fmt.Sprintf("%s to %s Transition with Steady State %s",
				tc.containerCurrentStatus.String(), tc.containerDesiredStatus.String(), steadyState.String()), func(t *testing.T) {
				if tc.containerDesiredStatus == apicontainerstatus.ContainerResourcesProvisioned &&
					steadyState < tc.containerDesiredStatus {
					t.Skipf("Skipping because of unassumable steady state [%s] and desired state [%s]",
						steadyState.String(), tc.containerDesiredStatus.String())
				}
				container := apicontainer.NewContainerWithSteadyState(steadyState)
				container.DesiredStatusUnsafe = tc.containerDesiredStatus
				container.KnownStatusUnsafe = tc.containerCurrentStatus
				task := &managedTask{
					Task: &apitask.Task{
						Containers: []*apicontainer.Container{
							container,
						},
						DesiredStatusUnsafe: apitaskstatus.TaskRunning,
					},
					engine: &DockerTaskEngine{},
				}
				transition := task.containerNextState(container)
				t.Logf("%s %v %v", transition.nextState, transition.actionRequired, transition.reason)
				assert.Equal(t, tc.expectedContainerStatus, transition.nextState, "Mismatch for expected next state")
				assert.Equal(t, tc.expectedTransitionActionable, transition.actionRequired, "Mismatch for expected actionable flag")
				assert.Equal(t, tc.reason, transition.reason, "Mismatch for expected reason")
			})
		}
	}
}

func TestContainerNextStateWithTransitionDependencies(t *testing.T) {
	testCases := []struct {
		name                         string
		containerCurrentStatus       apicontainerstatus.ContainerStatus
		containerDesiredStatus       apicontainerstatus.ContainerStatus
		containerDependentStatus     apicontainerstatus.ContainerStatus
		dependencyCurrentStatus      apicontainerstatus.ContainerStatus
		dependencySatisfiedStatus    apicontainerstatus.ContainerStatus
		expectedContainerStatus      apicontainerstatus.ContainerStatus
		expectedTransitionActionable bool
		reason                       error
	}{
		// NONE -> RUNNING transition is not allowed and not actionable, when pull depends on create and dependency is None
		{
			name: "pull depends on created, dependency is none",
			containerCurrentStatus:       apicontainerstatus.ContainerStatusNone,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerPulled,
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerCreated,
			expectedContainerStatus:      apicontainerstatus.ContainerStatusNone,
			expectedTransitionActionable: false,
			reason: dependencygraph.ErrContainerDependencyNotResolved,
		},
		// NONE -> RUNNING transition is not allowed and not actionable, when desired is Running and dependency is Created
		{
			name: "pull depends on running, dependency is created",
			containerCurrentStatus:       apicontainerstatus.ContainerStatusNone,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerPulled,
			dependencyCurrentStatus:      apicontainerstatus.ContainerCreated,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerRunning,
			expectedContainerStatus:      apicontainerstatus.ContainerStatusNone,
			expectedTransitionActionable: false,
			reason: dependencygraph.ErrContainerDependencyNotResolved,
		},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Running
		// The expected next status is Pulled
		{
			name: "pull depends on running, dependency is running, next status is pulled",
			containerCurrentStatus:       apicontainerstatus.ContainerStatusNone,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerPulled,
			dependencyCurrentStatus:      apicontainerstatus.ContainerRunning,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerRunning,
			expectedContainerStatus:      apicontainerstatus.ContainerPulled,
			expectedTransitionActionable: true,
		},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Stopped
		// The expected next status is Pulled
		{
			name: "pull depends on running, dependency is stopped, next status is pulled",
			containerCurrentStatus:       apicontainerstatus.ContainerStatusNone,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerPulled,
			dependencyCurrentStatus:      apicontainerstatus.ContainerStopped,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerRunning,
			expectedContainerStatus:      apicontainerstatus.ContainerPulled,
			expectedTransitionActionable: true,
		},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is None and
		// dependent status is Running
		{
			name: "create depends on running, dependency is none, next status is pulled",
			containerCurrentStatus:       apicontainerstatus.ContainerStatusNone,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerCreated,
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerRunning,
			expectedContainerStatus:      apicontainerstatus.ContainerPulled,
			expectedTransitionActionable: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dependencyName := "dependency"
			container := &apicontainer.Container{
				DesiredStatusUnsafe:       tc.containerDesiredStatus,
				KnownStatusUnsafe:         tc.containerCurrentStatus,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			}
			container.BuildContainerDependency(dependencyName, tc.dependencySatisfiedStatus, tc.containerDependentStatus)
			dependency := &apicontainer.Container{
				Name:              dependencyName,
				KnownStatusUnsafe: tc.dependencyCurrentStatus,
			}
			task := &managedTask{
				Task: &apitask.Task{
					Containers: []*apicontainer.Container{
						container,
						dependency,
					},
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				engine: &DockerTaskEngine{},
			}
			transition := task.containerNextState(container)
			assert.Equal(t, tc.expectedContainerStatus, transition.nextState,
				"Expected next state [%s] != Retrieved next state [%s]",
				tc.expectedContainerStatus.String(), transition.nextState.String())
			assert.Equal(t, tc.expectedTransitionActionable, transition.actionRequired, "transition actionable")
			assert.Equal(t, tc.reason, transition.reason, "transition possible")
		})
	}
}

func TestContainerNextStateWithDependencies(t *testing.T) {
	testCases := []struct {
		containerCurrentStatus       apicontainerstatus.ContainerStatus
		containerDesiredStatus       apicontainerstatus.ContainerStatus
		dependencyCurrentStatus      apicontainerstatus.ContainerStatus
		expectedContainerStatus      apicontainerstatus.ContainerStatus
		expectedTransitionActionable bool
		reason                       error
	}{
		// NONE -> RUNNING transition is not allowed and not actionable, when desired is Running and dependency is None
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, false, dependencygraph.DependentContainerNotResolvedErr},
		// NONE -> RUNNING transition is not allowed and not actionable, when desired is Running and dependency is Created
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerCreated, apicontainerstatus.ContainerStatusNone, false, dependencygraph.DependentContainerNotResolvedErr},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Running
		// The expected next status is Pulled
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerPulled, true, nil},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Stopped
		// The expected next status is Pulled
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerPulled, true, nil},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s to %s Transition",
			tc.containerCurrentStatus.String(), tc.containerDesiredStatus.String()), func(t *testing.T) {
			dependencyName := "dependency"
			container := &apicontainer.Container{
				DesiredStatusUnsafe:     tc.containerDesiredStatus,
				KnownStatusUnsafe:       tc.containerCurrentStatus,
				SteadyStateDependencies: []string{dependencyName},
			}
			dependency := &apicontainer.Container{
				Name:              dependencyName,
				KnownStatusUnsafe: tc.dependencyCurrentStatus,
			}
			task := &managedTask{
				Task: &apitask.Task{
					Containers: []*apicontainer.Container{
						container,
						dependency,
					},
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				engine: &DockerTaskEngine{},
			}
			transition := task.containerNextState(container)
			assert.Equal(t, tc.expectedContainerStatus, transition.nextState,
				"Expected next state [%s] != Retrieved next state [%s]",
				tc.expectedContainerStatus.String(), transition.nextState.String())
			assert.Equal(t, tc.expectedTransitionActionable, transition.actionRequired, "transition actionable")
			assert.Equal(t, tc.reason, transition.reason, "transition possible")
		})
	}
}

func TestContainerNextStateWithPullCredentials(t *testing.T) {
	testCases := []struct {
		containerCurrentStatus       apicontainerstatus.ContainerStatus
		containerDesiredStatus       apicontainerstatus.ContainerStatus
		expectedContainerStatus      apicontainerstatus.ContainerStatus
		credentialsID                string
		useExecutionRole             bool
		expectedTransitionActionable bool
		expectedTransitionReason     error
	}{
		// NONE -> RUNNING transition is not allowed when container is waiting for credentials
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStatusNone, "not_existed", true, false, dependencygraph.CredentialsNotResolvedErr},
		// NONE -> RUNNING transition is allowed when the required execution credentials existed
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerPulled, "existed", true, true, nil},
		// PULLED -> RUNNING transition is allowed even the credentials is required
		{apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerCreated, "not_existed", true, true, nil},
		// NONE -> STOPPED transition is allowed even the credentials is required
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, "not_existed", true, false, nil},
		// NONE -> RUNNING transition is allowed when the container doesn't use execution credentials
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerPulled, "not_existed", false, true, nil},
	}

	taskEngine := &DockerTaskEngine{
		credentialsManager: credentials.NewManager(),
	}

	err := taskEngine.credentialsManager.SetTaskCredentials(credentials.TaskIAMRoleCredentials{
		ARN: "taskarn",
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			CredentialsID:   "existed",
			SessionToken:    "token",
			AccessKeyID:     "id",
			SecretAccessKey: "accesskey",
		},
	})
	assert.NoError(t, err, "setting task credentials failed")

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s to %s transition with useExecutionRole %v and credentials %s",
			tc.containerCurrentStatus.String(),
			tc.containerDesiredStatus.String(),
			tc.useExecutionRole,
			tc.credentialsID), func(t *testing.T) {
			container := &apicontainer.Container{
				DesiredStatusUnsafe: tc.containerDesiredStatus,
				KnownStatusUnsafe:   tc.containerCurrentStatus,
				RegistryAuthentication: &apicontainer.RegistryAuthenticationData{
					Type: "ecr",
					ECRAuthData: &apicontainer.ECRAuthData{
						UseExecutionRole: tc.useExecutionRole,
					},
				},
			}

			task := &managedTask{
				Task: &apitask.Task{
					Containers: []*apicontainer.Container{
						container,
					},
					ExecutionCredentialsID: tc.credentialsID,
					DesiredStatusUnsafe:    apitaskstatus.TaskRunning,
				},
				engine:             taskEngine,
				credentialsManager: taskEngine.credentialsManager,
			}

			transition := task.containerNextState(container)
			assert.Equal(t, tc.expectedContainerStatus, transition.nextState, "Mismatch container status")
			assert.Equal(t, tc.expectedTransitionReason, transition.reason, "Mismatch transition possible")
			assert.Equal(t, tc.expectedTransitionActionable, transition.actionRequired, "Mismatch transition actionalbe")
		})
	}
}

func TestStartContainerTransitionsWhenForwardTransitionPossible(t *testing.T) {
	steadyStates := []apicontainerstatus.ContainerStatus{apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerResourcesProvisioned}
	for _, steadyState := range steadyStates {
		t.Run(fmt.Sprintf("Steady State is %s", steadyState.String()), func(t *testing.T) {
			firstContainerName := "container1"
			firstContainer := apicontainer.NewContainerWithSteadyState(steadyState)
			firstContainer.KnownStatusUnsafe = apicontainerstatus.ContainerCreated
			firstContainer.DesiredStatusUnsafe = apicontainerstatus.ContainerRunning
			firstContainer.Name = firstContainerName

			secondContainerName := "container2"
			secondContainer := apicontainer.NewContainerWithSteadyState(steadyState)
			secondContainer.KnownStatusUnsafe = apicontainerstatus.ContainerPulled
			secondContainer.DesiredStatusUnsafe = apicontainerstatus.ContainerRunning
			secondContainer.Name = secondContainerName

			task := &managedTask{
				Task: &apitask.Task{
					Containers: []*apicontainer.Container{
						firstContainer,
						secondContainer,
					},
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				engine: &DockerTaskEngine{},
			}

			pauseContainerName := "pause"
			waitForAssertions := sync.WaitGroup{}
			if steadyState == apicontainerstatus.ContainerResourcesProvisioned {
				pauseContainer := apicontainer.NewContainerWithSteadyState(steadyState)
				pauseContainer.KnownStatusUnsafe = apicontainerstatus.ContainerRunning
				pauseContainer.DesiredStatusUnsafe = apicontainerstatus.ContainerResourcesProvisioned
				pauseContainer.Name = pauseContainerName
				task.Containers = append(task.Containers, pauseContainer)
				waitForAssertions.Add(1)
			}

			waitForAssertions.Add(2)
			canTransition, transitions, _ := task.startContainerTransitions(
				func(cont *apicontainer.Container, nextStatus apicontainerstatus.ContainerStatus) {
					if cont.Name == firstContainerName {
						assert.Equal(t, nextStatus, apicontainerstatus.ContainerRunning, "Mismatch for first container next status")
					} else if cont.Name == secondContainerName {
						assert.Equal(t, nextStatus, apicontainerstatus.ContainerCreated, "Mismatch for second container next status")
					} else if cont.Name == pauseContainerName {
						assert.Equal(t, nextStatus, apicontainerstatus.ContainerResourcesProvisioned, "Mismatch for pause container next status")
					}
					waitForAssertions.Done()
				})
			waitForAssertions.Wait()
			assert.True(t, canTransition, "Mismatch for canTransition")
			assert.NotEmpty(t, transitions)
			if steadyState == apicontainerstatus.ContainerResourcesProvisioned {
				assert.Len(t, transitions, 3)
				pauseContainerTransition, ok := transitions[pauseContainerName]
				assert.True(t, ok, "Expected pause container transition to be in the transitions map")
				assert.Equal(t, pauseContainerTransition, apicontainerstatus.ContainerResourcesProvisioned, "Mismatch for pause container transition state")
			} else {
				assert.Len(t, transitions, 2)
			}
			firstContainerTransition, ok := transitions[firstContainerName]
			assert.True(t, ok, "Expected first container transition to be in the transitions map")
			assert.Equal(t, firstContainerTransition, apicontainerstatus.ContainerRunning, "Mismatch for first container transition state")
			secondContainerTransition, ok := transitions[secondContainerName]
			assert.True(t, ok, "Expected second container transition to be in the transitions map")
			assert.Equal(t, secondContainerTransition, apicontainerstatus.ContainerCreated, "Mismatch for second container transition state")
		})
	}
}

func TestStartContainerTransitionsWhenForwardTransitionIsNotPossible(t *testing.T) {
	firstContainerName := "container1"
	firstContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		Name:                firstContainerName,
	}
	secondContainerName := "container2"
	secondContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		Name:                secondContainerName,
	}
	pauseContainerName := "pause"
	pauseContainer := apicontainer.NewContainerWithSteadyState(apicontainerstatus.ContainerResourcesProvisioned)
	pauseContainer.KnownStatusUnsafe = apicontainerstatus.ContainerResourcesProvisioned
	pauseContainer.DesiredStatusUnsafe = apicontainerstatus.ContainerResourcesProvisioned
	pauseContainer.Name = pauseContainerName
	task := &managedTask{
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{
				firstContainer,
				secondContainer,
				pauseContainer,
			},
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		},
		engine: &DockerTaskEngine{},
	}

	canTransition, transitions, _ := task.startContainerTransitions(
		func(cont *apicontainer.Container, nextStatus apicontainerstatus.ContainerStatus) {
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
	firstContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainerstatus.ContainerCreated,
		DesiredStatusUnsafe: apicontainerstatus.ContainerStopped,
		Name:                firstContainerName,
	}

	containerChangeEventStream := eventstream.NewEventStream(eventStreamName, context.Background())
	containerChangeEventStream.StartListening()

	stateChangeEvents := make(chan statechange.Event)

	task := &managedTask{
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{
				firstContainer,
			},
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		},
		engine: &DockerTaskEngine{
			containerChangeEventStream: containerChangeEventStream,
			stateChangeEvents:          stateChangeEvents,
		},
		stateChangeEvents:          stateChangeEvents,
		containerChangeEventStream: containerChangeEventStream,
		dockerMessages:             make(chan dockerContainerChange),
	}

	eventsGenerated := sync.WaitGroup{}
	eventsGenerated.Add(2)
	containerChangeEventStream.Subscribe(eventStreamName, func(events ...interface{}) error {
		assert.NotNil(t, events)
		assert.Len(t, events, 1)
		event := events[0]
		containerChangeEvent, ok := event.(dockerapi.DockerContainerChangeEvent)
		assert.True(t, ok)
		assert.Equal(t, containerChangeEvent.Status, apicontainerstatus.ContainerStopped)
		eventsGenerated.Done()
		return nil
	})
	defer containerChangeEventStream.Unsubscribe(eventStreamName)

	// account for container and task state change events for Submit* API
	go func() {
		<-stateChangeEvents
		<-stateChangeEvents
		eventsGenerated.Done()
	}()

	go task.waitEvent(nil)
	canTransition, transitions, _ := task.startContainerTransitions(
		func(cont *apicontainer.Container, nextStatus apicontainerstatus.ContainerStatus) {
			t.Error("Invalid code path. The transition function should not be invoked when transitioning container from CREATED -> STOPPED")
		})
	assert.True(t, canTransition)
	assert.Empty(t, transitions)
	eventsGenerated.Wait()
}

func TestWaitForContainerTransitionsForNonTerminalTask(t *testing.T) {
	acsMessages := make(chan acsTransition)
	dockerMessages := make(chan dockerContainerChange)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	task := &managedTask{
		ctx:            ctx,
		acsMessages:    acsMessages,
		dockerMessages: dockerMessages,
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{},
		},
	}

	transitionChange := make(chan struct{}, 2)
	transitionChangeContainer := make(chan string, 2)

	firstContainerName := "container1"
	secondContainerName := "container2"

	// populate the transitions map with transitions for two
	// containers. We expect two sets of events to be consumed
	// by `waitForContainerTransition`
	transitions := make(map[string]string)
	transitions[firstContainerName] = apicontainerstatus.ContainerRunning.String()
	transitions[secondContainerName] = apicontainerstatus.ContainerRunning.String()

	go func() {
		// Send "transitions completed" messages. These are being
		// sent out of order for no particular reason. We should be
		// resilient to the ordering of these messages anyway.
		transitionChange <- struct{}{}
		transitionChangeContainer <- secondContainerName
		transitionChange <- struct{}{}
		transitionChangeContainer <- firstContainerName
	}()

	// waitForTransition will block until it receives events
	// sent by the go routine defined above
	task.waitForTransition(transitions, transitionChange, transitionChangeContainer)
}

// TestWaitForContainerTransitionsForTerminalTask verifies that the
// `waitForContainerTransition` method doesn't wait for any container
// transitions when the task's desired status is STOPPED and if all
// containers in the task are in PULLED state
func TestWaitForContainerTransitionsForTerminalTask(t *testing.T) {
	acsMessages := make(chan acsTransition)
	dockerMessages := make(chan dockerContainerChange)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	task := &managedTask{
		acsMessages:    acsMessages,
		dockerMessages: dockerMessages,
		Task: &apitask.Task{
			Containers:        []*apicontainer.Container{},
			KnownStatusUnsafe: apitaskstatus.TaskStopped,
		},
		ctx: ctx,
	}

	transitionChange := make(chan struct{}, 2)
	transitionChangeContainer := make(chan string, 2)

	firstContainerName := "container1"
	secondContainerName := "container2"
	transitions := make(map[string]string)
	transitions[firstContainerName] = apicontainerstatus.ContainerPulled.String()
	transitions[secondContainerName] = apicontainerstatus.ContainerPulled.String()

	// Event though there are two keys in the transitions map, send
	// only one event. This tests that `waitForContainerTransition` doesn't
	// block to receive two events and will still progress
	go func() {
		transitionChange <- struct{}{}
		transitionChangeContainer <- secondContainerName
	}()
	task.waitForTransition(transitions, transitionChange, transitionChangeContainer)
}

func TestOnContainersUnableToTransitionStateForDesiredStoppedTask(t *testing.T) {
	stateChangeEvents := make(chan statechange.Event)
	task := &managedTask{
		Task: &apitask.Task{
			Containers:          []*apicontainer.Container{},
			DesiredStatusUnsafe: apitaskstatus.TaskStopped,
		},
		engine: &DockerTaskEngine{
			stateChangeEvents: stateChangeEvents,
		},
		stateChangeEvents: stateChangeEvents,
	}
	eventsGenerated := sync.WaitGroup{}
	eventsGenerated.Add(1)

	go func() {
		event := <-stateChangeEvents
		assert.Equal(t, event.(api.TaskStateChange).Reason, taskUnableToTransitionToStoppedReason)
		eventsGenerated.Done()
	}()

	task.onContainersUnableToTransitionState()
	eventsGenerated.Wait()

	assert.Equal(t, task.GetDesiredStatus(), apitaskstatus.TaskStopped)
}

func TestOnContainersUnableToTransitionStateForDesiredRunningTask(t *testing.T) {
	firstContainerName := "container1"
	firstContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainerstatus.ContainerCreated,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		Name:                firstContainerName,
	}
	task := &managedTask{
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{
				firstContainer,
			},
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		},
	}

	task.onContainersUnableToTransitionState()
	assert.Equal(t, task.GetDesiredStatus(), apitaskstatus.TaskStopped)
	assert.Equal(t, task.Containers[0].GetDesiredStatus(), apicontainerstatus.ContainerStopped)
}

// TODO: Test progressContainers workflow

func TestHandleStoppedToSteadyStateTransition(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStateManager := mock_statemanager.NewMockStateManager(ctrl)
	defer ctrl.Finish()

	taskEngine := &DockerTaskEngine{
		saver: mockStateManager,
	}
	firstContainerName := "container1"
	firstContainer := &apicontainer.Container{
		KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
		Name:              firstContainerName,
	}
	secondContainerName := "container2"
	secondContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		Name:                secondContainerName,
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	mTask := &managedTask{
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{
				firstContainer,
				secondContainer,
			},
			Arn: "task1",
		},
		engine:         taskEngine,
		acsMessages:    make(chan acsTransition),
		dockerMessages: make(chan dockerContainerChange),
		saver:          taskEngine.saver,
		ctx:            ctx,
	}
	taskEngine.managedTasks = make(map[string]*managedTask)
	taskEngine.managedTasks["task1"] = mTask

	var waitForTransitionFunctionInvocation sync.WaitGroup
	waitForTransitionFunctionInvocation.Add(1)
	transitionFunction := func(task *apitask.Task, container *apicontainer.Container) dockerapi.DockerContainerMetadata {
		assert.Equal(t, firstContainerName, container.Name,
			"Mismatch in container reference in transition function")
		waitForTransitionFunctionInvocation.Done()
		return dockerapi.DockerContainerMetadata{}
	}

	taskEngine.containerStatusToTransitionFunction = map[apicontainerstatus.ContainerStatus]transitionApplyFunc{
		apicontainerstatus.ContainerStopped: transitionFunction,
	}

	// Received RUNNING event, known status is not STOPPED, expect this to
	// be a noop. Assertions in transitionFunction asserts that as well
	mTask.handleStoppedToRunningContainerTransition(
		apicontainerstatus.ContainerRunning, secondContainer)

	// Start building preconditions and assertions for STOPPED -> RUNNING
	// transition that will be triggered by next invocation of
	// handleStoppedToRunningContainerTransition

	// We expect state manager Save to be invoked on container transition
	// for the next transition
	mockStateManager.EXPECT().Save()
	// This wait group ensures that a docker message is generated as a
	// result of the transition function
	var waitForDockerMessageAssertions sync.WaitGroup
	waitForDockerMessageAssertions.Add(1)
	go func() {
		dockerMessage := <-mTask.dockerMessages
		assert.Equal(t, apicontainerstatus.ContainerStopped, dockerMessage.event.Status,
			"Mismatch in event status")
		assert.Equal(t, firstContainerName, dockerMessage.container.Name,
			"Mismatch in container reference in event")
		waitForDockerMessageAssertions.Done()
	}()
	// Received RUNNING, known status is STOPPED, expect this to invoke
	// transition function once
	mTask.handleStoppedToRunningContainerTransition(
		apicontainerstatus.ContainerRunning, firstContainer)

	// Wait for wait groups to be done
	waitForTransitionFunctionInvocation.Wait()
	waitForDockerMessageAssertions.Wait()

	// We now have an empty transition function map. Any further transitions
	// should be noops
	delete(taskEngine.containerStatusToTransitionFunction, apicontainerstatus.ContainerStopped)
	// Simulate getting RUNNING event for a STOPPED container 10 times.
	// All of these should be noops. 10 is chosen arbitrarily. Any number > 0
	// should be fine here
	for i := 0; i < 10; i++ {
		mTask.handleStoppedToRunningContainerTransition(
			apicontainerstatus.ContainerRunning, firstContainer)
	}
}

func TestCleanupTask(t *testing.T) {
	cfg := getTestConfig()
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	mockImageManager := mock_engine.NewMockImageManager(ctrl)
	mockResource := mock_taskresource.NewMockTaskResource(ctrl)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	taskEngine := &DockerTaskEngine{
		ctx:          ctx,
		cfg:          &cfg,
		saver:        statemanager.NewNoopStateManager(),
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
		cfg:   taskEngine.cfg,
		saver: taskEngine.saver,
	}
	mTask.Task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	mTask.AddResource("mockResource", mockResource)
	mTask.SetKnownStatus(apitaskstatus.TaskStopped)
	mTask.SetSentStatus(apitaskstatus.TaskStopped)
	container := mTask.Containers[0]
	dockerContainer := &apicontainer.DockerContainer{
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
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*apicontainer.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mockResource.EXPECT().Cleanup()
	mockResource.EXPECT().GetName()
	mTask.cleanupTask(taskStoppedDuration)
}

func TestCleanupTaskWaitsForStoppedSent(t *testing.T) {
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
		saver:        statemanager.NewNoopStateManager(),
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
		cfg:   taskEngine.cfg,
		saver: taskEngine.saver,
	}
	mTask.SetKnownStatus(apitaskstatus.TaskStopped)
	mTask.SetSentStatus(apitaskstatus.TaskRunning)
	container := mTask.Containers[0]
	dockerContainer := &apicontainer.DockerContainer{
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
			mTask.SetSentStatus(apitaskstatus.TaskStopped)
		} else if timesCalled > callsExpected {
			t.Errorf("Sleep called too many times, called %d but expected %d", timesCalled, callsExpected)
		}
	})
	assert.Equal(t, apitaskstatus.TaskRunning, mTask.GetSentStatus())

	// Expectations to verify that the task gets removed
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(
		map[string]*apicontainer.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mTask.cleanupTask(taskStoppedDuration)
	assert.Equal(t, apitaskstatus.TaskStopped, mTask.GetSentStatus())
}

func TestCleanupTaskGivesUpIfWaitingTooLong(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	mockImageManager := mock_engine.NewMockImageManager(ctrl)
	defer ctrl.Finish()

	cfg := getTestConfig()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	taskEngine := &DockerTaskEngine{
		ctx:          ctx,
		cfg:          &cfg,
		saver:        statemanager.NewNoopStateManager(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mTask := &managedTask{
		ctx:            ctx,
		cancel:         cancel,
		Task:           testdata.LoadTask("sleep5"),
		_time:          mockTime,
		engine:         taskEngine,
		acsMessages:    make(chan acsTransition),
		dockerMessages: make(chan dockerContainerChange),
		cfg:            taskEngine.cfg,
		saver:          taskEngine.saver,
	}
	mTask.SetKnownStatus(apitaskstatus.TaskStopped)
	mTask.SetSentStatus(apitaskstatus.TaskRunning)

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
	assert.Equal(t, apitaskstatus.TaskRunning, mTask.GetSentStatus())

	// No cleanup expected
	mTask.cleanupTask(taskStoppedDuration)
	assert.Equal(t, apitaskstatus.TaskRunning, mTask.GetSentStatus())
}

func TestCleanupTaskENIs(t *testing.T) {
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
		saver:        statemanager.NewNoopStateManager(),
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
		cfg:   taskEngine.cfg,
		saver: taskEngine.saver,
	}
	mTask.SetTaskENI(&apieni.ENI{
		ID: "TestCleanupTaskENIs",
		IPV4Addresses: []*apieni.ENIIPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		},
		MacAddress: mac,
		IPV6Addresses: []*apieni.ENIIPV6Address{
			{
				Address: ipv6,
			},
		},
	})

	mTask.SetKnownStatus(apitaskstatus.TaskStopped)
	mTask.SetSentStatus(apitaskstatus.TaskStopped)
	container := mTask.Containers[0]
	dockerContainer := &apicontainer.DockerContainer{
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
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*apicontainer.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mockState.EXPECT().RemoveENIAttachment(mac)
	mTask.cleanupTask(taskStoppedDuration)
}

func TestTaskWaitForExecutionCredentials(t *testing.T) {
	tcs := []struct {
		errs   []error
		result bool
		msg    string
	}{
		{
			errs: []error{
				dependencygraph.CredentialsNotResolvedErr,
				dependencygraph.ContainerPastDesiredStatusErr,
				fmt.Errorf("other error"),
			},
			result: true,
			msg:    "managed task should wait for credentials if the credentials dependency is not resolved",
		},
		{
			result: false,
			msg:    "managed task does not need to wait for credentials if there is no error",
		},
		{
			errs: []error{
				dependencygraph.ContainerPastDesiredStatusErr,
				dependencygraph.DependentContainerNotResolvedErr,
				fmt.Errorf("other errors"),
			},
			result: false,
			msg:    "managed task does not need to wait for credentials if there is no credentials dependency error",
		},
	}

	for _, tc := range tcs {
		t.Run(fmt.Sprintf("%v", tc.errs), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockTime := mock_ttime.NewMockTime(ctrl)
			mockTimer := mock_ttime.NewMockTimer(ctrl)
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			task := &managedTask{
				ctx: ctx,
				Task: &apitask.Task{
					KnownStatusUnsafe:   apitaskstatus.TaskRunning,
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				_time:       mockTime,
				acsMessages: make(chan acsTransition),
			}
			if tc.result {
				mockTime.EXPECT().AfterFunc(gomock.Any(), gomock.Any()).Return(mockTimer)
				mockTimer.EXPECT().Stop()
				go func() { task.acsMessages <- acsTransition{desiredStatus: apitaskstatus.TaskRunning} }()
			}

			assert.Equal(t, tc.result, task.waitForExecutionCredentialsFromACS(tc.errs), tc.msg)
		})
	}
}

func TestCleanupTaskWithInvalidInterval(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	mockImageManager := mock_engine.NewMockImageManager(ctrl)
	defer ctrl.Finish()

	cfg := getTestConfig()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	taskEngine := &DockerTaskEngine{
		ctx:          ctx,
		cfg:          &cfg,
		saver:        statemanager.NewNoopStateManager(),
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
		cfg:   taskEngine.cfg,
		saver: taskEngine.saver,
	}

	mTask.SetKnownStatus(apitaskstatus.TaskStopped)
	mTask.SetSentStatus(apitaskstatus.TaskStopped)
	container := mTask.Containers[0]
	dockerContainer := &apicontainer.DockerContainer{
		DockerName: "dockerContainer",
	}

	// Expectations for triggering cleanup
	now := mTask.GetKnownStatusTime()
	taskStoppedDuration := -1 * time.Minute
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

func TestCleanupTaskWithResourceHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	mockImageManager := mock_engine.NewMockImageManager(ctrl)
	defer ctrl.Finish()

	cfg := getTestConfig()
	cfg.TaskCPUMemLimit = config.ExplicitlyEnabled
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	taskEngine := &DockerTaskEngine{
		ctx:          ctx,
		cfg:          &cfg,
		saver:        statemanager.NewNoopStateManager(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mockResource := mock_taskresource.NewMockTaskResource(ctrl)
	mTask := &managedTask{
		ctx:                      ctx,
		cancel:                   cancel,
		Task:                     testdata.LoadTask("sleep5TaskCgroup"),
		_time:                    mockTime,
		engine:                   taskEngine,
		acsMessages:              make(chan acsTransition),
		dockerMessages:           make(chan dockerContainerChange),
		resourceStateChangeEvent: make(chan resourceStateChange),
		cfg:   taskEngine.cfg,
		saver: taskEngine.saver,
	}
	mTask.Task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	mTask.AddResource("mockResource", mockResource)
	mTask.SetKnownStatus(apitaskstatus.TaskStopped)
	mTask.SetSentStatus(apitaskstatus.TaskStopped)
	container := mTask.Containers[0]
	dockerContainer := &apicontainer.DockerContainer{
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
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*apicontainer.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mockResource.EXPECT().GetName()
	mockResource.EXPECT().Cleanup().Return(nil)
	mTask.cleanupTask(taskStoppedDuration)
}

func TestCleanupTaskWithResourceErrorPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	mockImageManager := mock_engine.NewMockImageManager(ctrl)
	defer ctrl.Finish()

	cfg := getTestConfig()
	cfg.TaskCPUMemLimit = config.ExplicitlyEnabled
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	taskEngine := &DockerTaskEngine{
		ctx:          ctx,
		cfg:          &cfg,
		saver:        statemanager.NewNoopStateManager(),
		state:        mockState,
		client:       mockClient,
		imageManager: mockImageManager,
	}
	mockResource := mock_taskresource.NewMockTaskResource(ctrl)
	mTask := &managedTask{
		ctx:                      ctx,
		cancel:                   cancel,
		Task:                     testdata.LoadTask("sleep5TaskCgroup"),
		_time:                    mockTime,
		engine:                   taskEngine,
		acsMessages:              make(chan acsTransition),
		dockerMessages:           make(chan dockerContainerChange),
		resourceStateChangeEvent: make(chan resourceStateChange),
		cfg:   taskEngine.cfg,
		saver: taskEngine.saver,
	}
	mTask.Task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	mTask.AddResource("mockResource", mockResource)
	mTask.SetKnownStatus(apitaskstatus.TaskStopped)
	mTask.SetSentStatus(apitaskstatus.TaskStopped)
	container := mTask.Containers[0]
	dockerContainer := &apicontainer.DockerContainer{
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
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*apicontainer.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mockResource.EXPECT().GetName()
	mockResource.EXPECT().Cleanup().Return(errors.New("cleanup error"))
	mTask.cleanupTask(taskStoppedDuration)
}

func TestHandleContainerChangeUpdateContainerHealth(t *testing.T) {
	eventStreamName := "TestHandleContainerChangeUpdateContainerHealth"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	containerChangeEventStream := eventstream.NewEventStream(eventStreamName, ctx)
	containerChangeEventStream.StartListening()

	mTask := &managedTask{
		Task: testdata.LoadTask("sleep5TaskCgroup"),
		containerChangeEventStream: containerChangeEventStream,
		stateChangeEvents:          make(chan statechange.Event),
	}
	// Disgard all the statechange events
	defer discardEvents(mTask.stateChangeEvents)()

	mTask.SetKnownStatus(apitaskstatus.TaskRunning)
	mTask.SetSentStatus(apitaskstatus.TaskRunning)
	container := mTask.Containers[0]
	container.HealthCheckType = "docker"

	containerChange := dockerContainerChange{
		container: container,
		event: dockerapi.DockerContainerChangeEvent{
			Status: apicontainerstatus.ContainerRunning,
			DockerContainerMetadata: dockerapi.DockerContainerMetadata{
				DockerID: "dockerID",
				Health: apicontainer.HealthStatus{
					Status: apicontainerstatus.ContainerHealthy,
					Output: "health check succeed",
				},
			},
		},
	}

	mTask.handleContainerChange(containerChange)

	containerHealth := container.GetHealthStatus()
	assert.Equal(t, containerHealth.Status, apicontainerstatus.ContainerHealthy)
	assert.Equal(t, containerHealth.Output, "health check succeed")
}

func TestWaitForHostResources(t *testing.T) {
	taskStopWG := utilsync.NewSequentialWaitGroup()
	taskStopWG.Add(1, 1)
	ctx, cancel := context.WithCancel(context.Background())

	mtask := &managedTask{
		ctx:        ctx,
		cancel:     cancel,
		taskStopWG: taskStopWG,
		Task: &apitask.Task{
			StartSequenceNumber: 1,
		},
	}

	var waitForHostResourcesWG sync.WaitGroup
	waitForHostResourcesWG.Add(1)
	go func() {
		mtask.waitForHostResources()
		waitForHostResourcesWG.Done()
	}()

	taskStopWG.Done(1)
	waitForHostResourcesWG.Wait()
}

func TestWaitForResourceTransition(t *testing.T) {
	task := &managedTask{
		Task: &apitask.Task{
			ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		},
	}
	transition := make(chan struct{}, 1)
	transitionChangeResource := make(chan string, 1)
	resName := "cgroup"
	// populate the transitions map with transition for the
	// resource. We expect the event to be consumed
	// by `waitForTransition`
	transitions := make(map[string]string)
	transitions[resName] = "ResourceCreated"

	go func() {
		// Send "transition complete" message
		transition <- struct{}{}
		transitionChangeResource <- resName
	}()

	// waitForTransition will block until it receives the event
	// sent by the go routine defined above
	task.waitForTransition(transitions, transition, transitionChangeResource)
}

func TestApplyResourceStateHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockResource := mock_taskresource.NewMockTaskResource(ctrl)
	task := &managedTask{
		Task: &apitask.Task{
			Arn:                "arn",
			ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		},
	}
	gomock.InOrder(
		mockResource.EXPECT().GetName(),
		mockResource.EXPECT().ApplyTransition(resourcestatus.ResourceCreated).Return(nil),
		mockResource.EXPECT().GetName().AnyTimes(),
		mockResource.EXPECT().StatusString(resourcestatus.ResourceCreated).AnyTimes(),
	)
	assert.NoError(t, task.applyResourceState(mockResource, resourcestatus.ResourceCreated))
}

func TestApplyResourceStateFailures(t *testing.T) {
	testCases := []struct {
		Name      string
		ResStatus resourcestatus.ResourceStatus
		Error     error
	}{
		{
			Name:      "no valid state transition",
			ResStatus: resourcestatus.ResourceRemoved,
			Error:     errors.New("transition impossible"),
		},
		{
			Name:      "transition error",
			ResStatus: resourcestatus.ResourceCreated,
			Error:     errors.New("transition failed"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockResource := mock_taskresource.NewMockTaskResource(ctrl)
			task := &managedTask{
				Task: &apitask.Task{
					Arn:                "arn",
					ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
				},
			}
			gomock.InOrder(
				mockResource.EXPECT().GetName(),
				mockResource.EXPECT().ApplyTransition(tc.ResStatus).Return(tc.Error),
				mockResource.EXPECT().GetName().AnyTimes(),
				mockResource.EXPECT().StatusString(tc.ResStatus).AnyTimes(),
			)
			assert.Error(t, task.applyResourceState(mockResource, tc.ResStatus))
		})
	}
}

func TestHandleVolumeResourceStateChangeAndSave(t *testing.T) {
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
			KnownStatus:        resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			DesiredKnownStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			Err:                errors.New("transition error"),
			ChangedKnownStatus: resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			TaskDesiredStatus:  apitaskstatus.TaskStopped,
		},
		{
			Name:               "steady state transition",
			KnownStatus:        resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			DesiredKnownStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			Err:                nil,
			ChangedKnownStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			TaskDesiredStatus:  apitaskstatus.TaskRunning,
		},
	}
	volumeName := "vol"
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockSaver := mock_statemanager.NewMockStateManager(ctrl)
			res := &volume.VolumeResource{Name: volumeName}
			res.SetKnownStatus(tc.KnownStatus)
			mtask := managedTask{
				Task: &apitask.Task{
					Arn:                 "task1",
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				engine: &DockerTaskEngine{},
			}
			mtask.AddResource(volumeName, res)
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

func TestHandleVolumeResourceStateChangeNoSave(t *testing.T) {
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
			KnownStatus:        resourcestatus.ResourceStatus(volume.VolumeCreated),
			DesiredKnownStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			Err:                nil,
			ChangedKnownStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			TaskDesiredStatus:  apitaskstatus.TaskRunning,
		},
		{
			Name:               "transition state less than known status",
			DesiredKnownStatus: resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			Err:                nil,
			KnownStatus:        resourcestatus.ResourceStatus(volume.VolumeCreated),
			ChangedKnownStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			TaskDesiredStatus:  apitaskstatus.TaskRunning,
		},
	}
	volumeName := "vol"
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			res := &volume.VolumeResource{Name: volumeName}
			res.SetKnownStatus(tc.KnownStatus)
			mtask := managedTask{
				Task: &apitask.Task{
					Arn:                 "task1",
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
			}
			mtask.AddResource(volumeName, res)
			mtask.handleResourceStateChange(resourceStateChange{
				res, tc.DesiredKnownStatus, tc.Err,
			})
			assert.Equal(t, tc.ChangedKnownStatus, res.GetKnownStatus())
			assert.Equal(t, tc.TaskDesiredStatus, mtask.GetDesiredStatus())
		})
	}
}

func TestVolumeResourceNextState(t *testing.T) {
	testCases := []struct {
		Name             string
		ResKnownStatus   resourcestatus.ResourceStatus
		ResDesiredStatus resourcestatus.ResourceStatus
		NextState        resourcestatus.ResourceStatus
		ActionRequired   bool
	}{
		{
			Name:             "next state happy path",
			ResKnownStatus:   resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			ResDesiredStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			NextState:        resourcestatus.ResourceStatus(volume.VolumeCreated),
			ActionRequired:   true,
		},
		{
			Name:             "desired terminal",
			ResKnownStatus:   resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			ResDesiredStatus: resourcestatus.ResourceStatus(volume.VolumeRemoved),
			NextState:        resourcestatus.ResourceStatus(volume.VolumeRemoved),
			ActionRequired:   false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			res := volume.VolumeResource{}
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

func TestStartVolumeResourceTransitionsHappyPath(t *testing.T) {
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
			ResKnownStatus:   resourcestatus.ResourceStatus(volume.VolumeStatusNone),
			ResDesiredStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			TransitionStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			StatusString:     "CREATED",
			CanTransition:    true,
			TransitionsLen:   1,
		},
	}
	volumeName := "vol"
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			res := &volume.VolumeResource{Name: volumeName}
			res.SetKnownStatus(tc.ResKnownStatus)
			res.SetDesiredStatus(tc.ResDesiredStatus)

			task := &managedTask{
				Task: &apitask.Task{
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
			}
			task.AddResource(volumeName, res)
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
			resTransition, ok := transitions[volumeName]
			assert.True(t, ok)
			assert.Equal(t, resTransition, tc.StatusString)
		})
	}
}

func TestStartVolumeResourceTransitionsEmpty(t *testing.T) {
	testCases := []struct {
		Name          string
		KnownStatus   resourcestatus.ResourceStatus
		DesiredStatus resourcestatus.ResourceStatus
		CanTransition bool
	}{
		{
			Name:          "known < desired",
			KnownStatus:   resourcestatus.ResourceStatus(volume.VolumeCreated),
			DesiredStatus: resourcestatus.ResourceStatus(volume.VolumeRemoved),
			CanTransition: true,
		},
		{
			Name:          "known equals desired",
			KnownStatus:   resourcestatus.ResourceStatus(volume.VolumeCreated),
			DesiredStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			CanTransition: false,
		},
		{
			Name:          "known > desired",
			KnownStatus:   resourcestatus.ResourceStatus(volume.VolumeRemoved),
			DesiredStatus: resourcestatus.ResourceStatus(volume.VolumeCreated),
			CanTransition: false,
		},
	}
	volumeName := "vol"
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			res := &volume.VolumeResource{Name: volumeName}
			res.SetKnownStatus(tc.KnownStatus)
			res.SetDesiredStatus(tc.DesiredStatus)

			mtask := &managedTask{
				Task: &apitask.Task{
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				ctx: ctx,
				resourceStateChangeEvent: make(chan resourceStateChange),
			}
			mtask.Task.AddResource(volumeName, res)
			canTransition, transitions := mtask.startResourceTransitions(
				func(resource taskresource.TaskResource, nextStatus resourcestatus.ResourceStatus) {
					t.Error("Transition function should not be called when no transitions are possible")
				})
			assert.Equal(t, tc.CanTransition, canTransition)
			assert.Empty(t, transitions)
		})
	}
}

func getTestConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.TaskCPUMemLimit = config.ExplicitlyDisabled
	return cfg
}
