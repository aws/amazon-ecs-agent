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
	utilsync "github.com/aws/amazon-ecs-agent/agent/utils/sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
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
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
)

func TestHandleEventError(t *testing.T) {
	testCases := []struct {
		Name                                  string
		EventStatus                           apicontainer.ContainerStatus
		CurrentContainerKnownStatus           apicontainer.ContainerStatus
		ImagePullBehavior                     config.ImagePullBehaviorType
		Error                                 apierrors.NamedError
		ExpectedContainerKnownStatusSet       bool
		ExpectedContainerKnownStatus          apicontainer.ContainerStatus
		ExpectedContainerDesiredStatusStopped bool
		ExpectedTaskDesiredStatusStopped      bool
		ExpectedOK                            bool
	}{
		{
			Name:                        "Stop timed out",
			EventStatus:                 apicontainer.ContainerStopped,
			CurrentContainerKnownStatus: apicontainer.ContainerRunning,
			Error: &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet: true,
			ExpectedContainerKnownStatus:    apicontainer.ContainerRunning,
			ExpectedOK:                      false,
		},
		{
			Name:                        "Retriable error with stop",
			EventStatus:                 apicontainer.ContainerStopped,
			CurrentContainerKnownStatus: apicontainer.ContainerRunning,
			Error: &dockerapi.CannotStopContainerError{
				FromError: errors.New(""),
			},
			ExpectedContainerKnownStatusSet: true,
			ExpectedContainerKnownStatus:    apicontainer.ContainerRunning,
			ExpectedOK:                      false,
		},
		{
			Name:                        "Unretriable error with Stop",
			EventStatus:                 apicontainer.ContainerStopped,
			CurrentContainerKnownStatus: apicontainer.ContainerRunning,
			Error: &dockerapi.CannotStopContainerError{
				FromError: &docker.ContainerNotRunning{},
			},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainer.ContainerStopped,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            true,
		},
		{
			Name:  "Pull failed",
			Error: &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet: true,
			EventStatus:                     apicontainer.ContainerPulled,
			ExpectedOK:                      true,
		},
		{
			Name:                        "Container vanished betweeen pull and running",
			EventStatus:                 apicontainer.ContainerRunning,
			CurrentContainerKnownStatus: apicontainer.ContainerPulled,
			Error: &ContainerVanishedError{},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainer.ContainerPulled,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Inspect failed during start",
			EventStatus:                 apicontainer.ContainerRunning,
			CurrentContainerKnownStatus: apicontainer.ContainerCreated,
			Error: &dockerapi.CannotInspectContainerError{
				FromError: errors.New("error"),
			},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainer.ContainerCreated,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Start timed out",
			EventStatus:                 apicontainer.ContainerRunning,
			CurrentContainerKnownStatus: apicontainer.ContainerCreated,
			Error: &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainer.ContainerCreated,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Inspect failed during create",
			EventStatus:                 apicontainer.ContainerCreated,
			CurrentContainerKnownStatus: apicontainer.ContainerPulled,
			Error: &dockerapi.CannotInspectContainerError{
				FromError: errors.New("error"),
			},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainer.ContainerPulled,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Create timed out",
			EventStatus:                 apicontainer.ContainerCreated,
			CurrentContainerKnownStatus: apicontainer.ContainerPulled,
			Error: &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainer.ContainerPulled,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:        "Pull image fails and task fails",
			EventStatus: apicontainer.ContainerPulled,
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
				assert.Equal(t, apicontainer.ContainerStopped, containerDesiredStatus,
					"desired status %s != %s", apicontainer.ContainerStopped.String(), containerDesiredStatus.String())
			}
			assert.Equal(t, tc.Error.ErrorName(), containerChange.container.ApplyingError.ErrorName())
		})
	}
}

func TestContainerNextState(t *testing.T) {
	testCases := []struct {
		containerCurrentStatus       apicontainer.ContainerStatus
		containerDesiredStatus       apicontainer.ContainerStatus
		expectedContainerStatus      apicontainer.ContainerStatus
		expectedTransitionActionable bool
		reason                       error
	}{
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running
		// The expected next status is Pulled
		{apicontainer.ContainerStatusNone, apicontainer.ContainerRunning, apicontainer.ContainerPulled, true, nil},
		// NONE -> RESOURCES_PROVISIONED transition is allowed and actionable, when desired
		// is Running. The exptected next status is Pulled
		{apicontainer.ContainerStatusNone, apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerPulled, true, nil},
		// NONE -> NONE transition is not be allowed and is not actionable,
		// when desired is Running
		{apicontainer.ContainerStatusNone, apicontainer.ContainerStatusNone, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// NONE -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{apicontainer.ContainerStatusNone, apicontainer.ContainerStopped, apicontainer.ContainerStopped, false, nil},
		// PULLED -> RUNNING transition is allowed and actionable, when desired is Running
		// The exptected next status is Created
		{apicontainer.ContainerPulled, apicontainer.ContainerRunning, apicontainer.ContainerCreated, true, nil},
		// PULLED -> RESOURCES_PROVISIONED transition is allowed and actionable, when desired
		// is Running. The exptected next status is Created
		{apicontainer.ContainerPulled, apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerCreated, true, nil},
		// PULLED -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerPulled, apicontainer.ContainerPulled, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// PULLED -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerPulled, apicontainer.ContainerStatusNone, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// PULLED -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{apicontainer.ContainerPulled, apicontainer.ContainerStopped, apicontainer.ContainerStopped, false, nil},
		// CREATED -> RUNNING transition is allowed and actionable, when desired is Running
		// The expected next status is Running
		{apicontainer.ContainerCreated, apicontainer.ContainerRunning, apicontainer.ContainerRunning, true, nil},
		// CREATED -> RESOURCES_PROVISIONED transition is allowed and actionable, when desired
		// is Running. The exptected next status is Running
		{apicontainer.ContainerCreated, apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerRunning, true, nil},
		// CREATED -> CREATED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerCreated, apicontainer.ContainerCreated, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// CREATED -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerCreated, apicontainer.ContainerStatusNone, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// CREATED -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerCreated, apicontainer.ContainerPulled, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// CREATED -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{apicontainer.ContainerCreated, apicontainer.ContainerStopped, apicontainer.ContainerStopped, false, nil},
		// RUNNING -> STOPPED transition is allowed and actionable, when desired is Running
		// The expected next status is STOPPED
		{apicontainer.ContainerRunning, apicontainer.ContainerStopped, apicontainer.ContainerStopped, true, nil},
		// RUNNING -> RUNNING transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerRunning, apicontainer.ContainerRunning, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RUNNING -> RESOURCES_PROVISIONED is allowed when steady state status is
		// RESOURCES_PROVISIONED and desired is RESOURCES_PROVISIONED
		{apicontainer.ContainerRunning, apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerResourcesProvisioned, true, nil},
		// RUNNING -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerRunning, apicontainer.ContainerStatusNone, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RUNNING -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerRunning, apicontainer.ContainerPulled, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RUNNING -> CREATED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerRunning, apicontainer.ContainerCreated, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},

		// RESOURCES_PROVISIONED -> RESOURCES_PROVISIONED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RESOURCES_PROVISIONED -> RUNNING transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerRunning, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RESOURCES_PROVISIONED -> STOPPED transition is allowed and actionable, when desired
		// is Running. The exptected next status is STOPPED
		{apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerStopped, apicontainer.ContainerStopped, true, nil},
		// RESOURCES_PROVISIONED -> NONE transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerStatusNone, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RESOURCES_PROVISIONED -> PULLED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerPulled, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// RESOURCES_PROVISIONED -> CREATED transition is not allowed and not actionable,
		// when desired is Running
		{apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerCreated, apicontainer.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
	}

	steadyStates := []apicontainer.ContainerStatus{apicontainer.ContainerRunning, apicontainer.ContainerResourcesProvisioned}

	for _, tc := range testCases {
		for _, steadyState := range steadyStates {
			t.Run(fmt.Sprintf("%s to %s Transition with Steady State %s",
				tc.containerCurrentStatus.String(), tc.containerDesiredStatus.String(), steadyState.String()), func(t *testing.T) {
				if tc.containerDesiredStatus == apicontainer.ContainerResourcesProvisioned &&
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
						DesiredStatusUnsafe: apitask.TaskRunning,
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
		containerCurrentStatus       apicontainer.ContainerStatus
		containerDesiredStatus       apicontainer.ContainerStatus
		containerDependentStatus     apicontainer.ContainerStatus
		dependencyCurrentStatus      apicontainer.ContainerStatus
		dependencySatisfiedStatus    apicontainer.ContainerStatus
		expectedContainerStatus      apicontainer.ContainerStatus
		expectedTransitionActionable bool
		reason                       error
	}{
		// NONE -> RUNNING transition is not allowed and not actionable, when pull depends on create and dependency is None
		{
			name: "pull depends on created, dependency is none",
			containerCurrentStatus:       apicontainer.ContainerStatusNone,
			containerDesiredStatus:       apicontainer.ContainerRunning,
			containerDependentStatus:     apicontainer.ContainerPulled,
			dependencyCurrentStatus:      apicontainer.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainer.ContainerCreated,
			expectedContainerStatus:      apicontainer.ContainerStatusNone,
			expectedTransitionActionable: false,
			reason: dependencygraph.ErrContainerDependencyNotResolved,
		},
		// NONE -> RUNNING transition is not allowed and not actionable, when desired is Running and dependency is Created
		{
			name: "pull depends on running, dependency is created",
			containerCurrentStatus:       apicontainer.ContainerStatusNone,
			containerDesiredStatus:       apicontainer.ContainerRunning,
			containerDependentStatus:     apicontainer.ContainerPulled,
			dependencyCurrentStatus:      apicontainer.ContainerCreated,
			dependencySatisfiedStatus:    apicontainer.ContainerRunning,
			expectedContainerStatus:      apicontainer.ContainerStatusNone,
			expectedTransitionActionable: false,
			reason: dependencygraph.ErrContainerDependencyNotResolved,
		},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Running
		// The expected next status is Pulled
		{
			name: "pull depends on running, dependency is running, next status is pulled",
			containerCurrentStatus:       apicontainer.ContainerStatusNone,
			containerDesiredStatus:       apicontainer.ContainerRunning,
			containerDependentStatus:     apicontainer.ContainerPulled,
			dependencyCurrentStatus:      apicontainer.ContainerRunning,
			dependencySatisfiedStatus:    apicontainer.ContainerRunning,
			expectedContainerStatus:      apicontainer.ContainerPulled,
			expectedTransitionActionable: true,
		},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Stopped
		// The expected next status is Pulled
		{
			name: "pull depends on running, dependency is stopped, next status is pulled",
			containerCurrentStatus:       apicontainer.ContainerStatusNone,
			containerDesiredStatus:       apicontainer.ContainerRunning,
			containerDependentStatus:     apicontainer.ContainerPulled,
			dependencyCurrentStatus:      apicontainer.ContainerStopped,
			dependencySatisfiedStatus:    apicontainer.ContainerRunning,
			expectedContainerStatus:      apicontainer.ContainerPulled,
			expectedTransitionActionable: true,
		},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is None and
		// dependent status is Running
		{
			name: "create depends on running, dependency is none, next status is pulled",
			containerCurrentStatus:       apicontainer.ContainerStatusNone,
			containerDesiredStatus:       apicontainer.ContainerRunning,
			containerDependentStatus:     apicontainer.ContainerCreated,
			dependencyCurrentStatus:      apicontainer.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainer.ContainerRunning,
			expectedContainerStatus:      apicontainer.ContainerPulled,
			expectedTransitionActionable: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dependencyName := "dependency"
			container := &apicontainer.Container{
				DesiredStatusUnsafe:       tc.containerDesiredStatus,
				KnownStatusUnsafe:         tc.containerCurrentStatus,
				TransitionDependenciesMap: make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet),
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
					DesiredStatusUnsafe: apitask.TaskRunning,
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
		containerCurrentStatus       apicontainer.ContainerStatus
		containerDesiredStatus       apicontainer.ContainerStatus
		dependencyCurrentStatus      apicontainer.ContainerStatus
		expectedContainerStatus      apicontainer.ContainerStatus
		expectedTransitionActionable bool
		reason                       error
	}{
		// NONE -> RUNNING transition is not allowed and not actionable, when desired is Running and dependency is None
		{apicontainer.ContainerStatusNone, apicontainer.ContainerRunning, apicontainer.ContainerStatusNone, apicontainer.ContainerStatusNone, false, dependencygraph.DependentContainerNotResolvedErr},
		// NONE -> RUNNING transition is not allowed and not actionable, when desired is Running and dependency is Created
		{apicontainer.ContainerStatusNone, apicontainer.ContainerRunning, apicontainer.ContainerCreated, apicontainer.ContainerStatusNone, false, dependencygraph.DependentContainerNotResolvedErr},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Running
		// The expected next status is Pulled
		{apicontainer.ContainerStatusNone, apicontainer.ContainerRunning, apicontainer.ContainerRunning, apicontainer.ContainerPulled, true, nil},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Stopped
		// The expected next status is Pulled
		{apicontainer.ContainerStatusNone, apicontainer.ContainerRunning, apicontainer.ContainerStopped, apicontainer.ContainerPulled, true, nil},
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
					DesiredStatusUnsafe: apitask.TaskRunning,
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
		containerCurrentStatus       apicontainer.ContainerStatus
		containerDesiredStatus       apicontainer.ContainerStatus
		expectedContainerStatus      apicontainer.ContainerStatus
		credentialsID                string
		useExecutionRole             bool
		expectedTransitionActionable bool
		expectedTransitionReason     error
	}{
		// NONE -> RUNNING transition is not allowed when container is waiting for credentials
		{apicontainer.ContainerStatusNone, apicontainer.ContainerRunning, apicontainer.ContainerStatusNone, "not_existed", true, false, dependencygraph.CredentialsNotResolvedErr},
		// NONE -> RUNNING transition is allowed when the required execution credentials existed
		{apicontainer.ContainerStatusNone, apicontainer.ContainerRunning, apicontainer.ContainerPulled, "existed", true, true, nil},
		// PULLED -> RUNNING transition is allowed even the credentials is required
		{apicontainer.ContainerPulled, apicontainer.ContainerRunning, apicontainer.ContainerCreated, "not_existed", true, true, nil},
		// NONE -> STOPPED transition is allowed even the credentials is required
		{apicontainer.ContainerStatusNone, apicontainer.ContainerStopped, apicontainer.ContainerStopped, "not_existed", true, false, nil},
		// NONE -> RUNNING transition is allowed when the container doesn't use execution credentials
		{apicontainer.ContainerStatusNone, apicontainer.ContainerRunning, apicontainer.ContainerPulled, "not_existed", false, true, nil},
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
					DesiredStatusUnsafe:    apitask.TaskRunning,
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
	steadyStates := []apicontainer.ContainerStatus{apicontainer.ContainerRunning, apicontainer.ContainerResourcesProvisioned}
	for _, steadyState := range steadyStates {
		t.Run(fmt.Sprintf("Steady State is %s", steadyState.String()), func(t *testing.T) {
			firstContainerName := "container1"
			firstContainer := apicontainer.NewContainerWithSteadyState(steadyState)
			firstContainer.KnownStatusUnsafe = apicontainer.ContainerCreated
			firstContainer.DesiredStatusUnsafe = apicontainer.ContainerRunning
			firstContainer.Name = firstContainerName

			secondContainerName := "container2"
			secondContainer := apicontainer.NewContainerWithSteadyState(steadyState)
			secondContainer.KnownStatusUnsafe = apicontainer.ContainerPulled
			secondContainer.DesiredStatusUnsafe = apicontainer.ContainerRunning
			secondContainer.Name = secondContainerName

			task := &managedTask{
				Task: &apitask.Task{
					Containers: []*apicontainer.Container{
						firstContainer,
						secondContainer,
					},
					DesiredStatusUnsafe: apitask.TaskRunning,
				},
				engine: &DockerTaskEngine{},
			}

			pauseContainerName := "pause"
			waitForAssertions := sync.WaitGroup{}
			if steadyState == apicontainer.ContainerResourcesProvisioned {
				pauseContainer := apicontainer.NewContainerWithSteadyState(steadyState)
				pauseContainer.KnownStatusUnsafe = apicontainer.ContainerRunning
				pauseContainer.DesiredStatusUnsafe = apicontainer.ContainerResourcesProvisioned
				pauseContainer.Name = pauseContainerName
				task.Containers = append(task.Containers, pauseContainer)
				waitForAssertions.Add(1)
			}

			waitForAssertions.Add(2)
			canTransition, transitions, _ := task.startContainerTransitions(
				func(cont *apicontainer.Container, nextStatus apicontainer.ContainerStatus) {
					if cont.Name == firstContainerName {
						assert.Equal(t, nextStatus, apicontainer.ContainerRunning, "Mismatch for first container next status")
					} else if cont.Name == secondContainerName {
						assert.Equal(t, nextStatus, apicontainer.ContainerCreated, "Mismatch for second container next status")
					} else if cont.Name == pauseContainerName {
						assert.Equal(t, nextStatus, apicontainer.ContainerResourcesProvisioned, "Mismatch for pause container next status")
					}
					waitForAssertions.Done()
				})
			waitForAssertions.Wait()
			assert.True(t, canTransition, "Mismatch for canTransition")
			assert.NotEmpty(t, transitions)
			if steadyState == apicontainer.ContainerResourcesProvisioned {
				assert.Len(t, transitions, 3)
				pauseContainerTransition, ok := transitions[pauseContainerName]
				assert.True(t, ok, "Expected pause container transition to be in the transitions map")
				assert.Equal(t, pauseContainerTransition, apicontainer.ContainerResourcesProvisioned, "Mismatch for pause container transition state")
			} else {
				assert.Len(t, transitions, 2)
			}
			firstContainerTransition, ok := transitions[firstContainerName]
			assert.True(t, ok, "Expected first container transition to be in the transitions map")
			assert.Equal(t, firstContainerTransition, apicontainer.ContainerRunning, "Mismatch for first container transition state")
			secondContainerTransition, ok := transitions[secondContainerName]
			assert.True(t, ok, "Expected second container transition to be in the transitions map")
			assert.Equal(t, secondContainerTransition, apicontainer.ContainerCreated, "Mismatch for second container transition state")
		})
	}
}

func TestStartContainerTransitionsWhenForwardTransitionIsNotPossible(t *testing.T) {
	firstContainerName := "container1"
	firstContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainer.ContainerRunning,
		DesiredStatusUnsafe: apicontainer.ContainerRunning,
		Name:                firstContainerName,
	}
	secondContainerName := "container2"
	secondContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainer.ContainerRunning,
		DesiredStatusUnsafe: apicontainer.ContainerRunning,
		Name:                secondContainerName,
	}
	pauseContainerName := "pause"
	pauseContainer := apicontainer.NewContainerWithSteadyState(apicontainer.ContainerResourcesProvisioned)
	pauseContainer.KnownStatusUnsafe = apicontainer.ContainerResourcesProvisioned
	pauseContainer.DesiredStatusUnsafe = apicontainer.ContainerResourcesProvisioned
	pauseContainer.Name = pauseContainerName
	task := &managedTask{
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{
				firstContainer,
				secondContainer,
				pauseContainer,
			},
			DesiredStatusUnsafe: apitask.TaskRunning,
		},
		engine: &DockerTaskEngine{},
	}

	canTransition, transitions, _ := task.startContainerTransitions(
		func(cont *apicontainer.Container, nextStatus apicontainer.ContainerStatus) {
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
		KnownStatusUnsafe:   apicontainer.ContainerCreated,
		DesiredStatusUnsafe: apicontainer.ContainerStopped,
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
			DesiredStatusUnsafe: apitask.TaskRunning,
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
		assert.Equal(t, containerChangeEvent.Status, apicontainer.ContainerStopped)
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
		func(cont *apicontainer.Container, nextStatus apicontainer.ContainerStatus) {
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
	transitions[firstContainerName] = apicontainer.ContainerRunning.String()
	transitions[secondContainerName] = apicontainer.ContainerRunning.String()

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
			KnownStatusUnsafe: apitask.TaskStopped,
		},
		ctx: ctx,
	}

	transitionChange := make(chan struct{}, 2)
	transitionChangeContainer := make(chan string, 2)

	firstContainerName := "container1"
	secondContainerName := "container2"
	transitions := make(map[string]string)
	transitions[firstContainerName] = apicontainer.ContainerPulled.String()
	transitions[secondContainerName] = apicontainer.ContainerPulled.String()

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
			DesiredStatusUnsafe: apitask.TaskStopped,
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

	assert.Equal(t, task.GetDesiredStatus(), apitask.TaskStopped)
}

func TestOnContainersUnableToTransitionStateForDesiredRunningTask(t *testing.T) {
	firstContainerName := "container1"
	firstContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainer.ContainerCreated,
		DesiredStatusUnsafe: apicontainer.ContainerRunning,
		Name:                firstContainerName,
	}
	task := &managedTask{
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{
				firstContainer,
			},
			DesiredStatusUnsafe: apitask.TaskRunning,
		},
	}

	task.onContainersUnableToTransitionState()
	assert.Equal(t, task.GetDesiredStatus(), apitask.TaskStopped)
	assert.Equal(t, task.Containers[0].GetDesiredStatus(), apicontainer.ContainerStopped)
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
		KnownStatusUnsafe: apicontainer.ContainerStopped,
		Name:              firstContainerName,
	}
	secondContainerName := "container2"
	secondContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainer.ContainerRunning,
		DesiredStatusUnsafe: apicontainer.ContainerRunning,
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

	taskEngine.containerStatusToTransitionFunction = map[apicontainer.ContainerStatus]transitionApplyFunc{
		apicontainer.ContainerStopped: transitionFunction,
	}

	// Received RUNNING event, known status is not STOPPED, expect this to
	// be a noop. Assertions in transitionFunction asserts that as well
	mTask.handleStoppedToRunningContainerTransition(
		apicontainer.ContainerRunning, secondContainer)

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
		assert.Equal(t, apicontainer.ContainerStopped, dockerMessage.event.Status,
			"Mismatch in event status")
		assert.Equal(t, firstContainerName, dockerMessage.container.Name,
			"Mismatch in container reference in event")
		waitForDockerMessageAssertions.Done()
	}()
	// Received RUNNING, known status is STOPPED, expect this to invoke
	// transition function once
	mTask.handleStoppedToRunningContainerTransition(
		apicontainer.ContainerRunning, firstContainer)

	// Wait for wait groups to be done
	waitForTransitionFunctionInvocation.Wait()
	waitForDockerMessageAssertions.Wait()

	// We now have an empty transition function map. Any further transitions
	// should be noops
	delete(taskEngine.containerStatusToTransitionFunction, apicontainer.ContainerStopped)
	// Simulate getting RUNNING event for a STOPPED container 10 times.
	// All of these should be noops. 10 is chosen arbitrarily. Any number > 0
	// should be fine here
	for i := 0; i < 10; i++ {
		mTask.handleStoppedToRunningContainerTransition(
			apicontainer.ContainerRunning, firstContainer)
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
	mTask.SetKnownStatus(apitask.TaskStopped)
	mTask.SetSentStatus(apitask.TaskStopped)
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
	mTask.SetKnownStatus(apitask.TaskStopped)
	mTask.SetSentStatus(apitask.TaskRunning)
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
			mTask.SetSentStatus(apitask.TaskStopped)
		} else if timesCalled > callsExpected {
			t.Errorf("Sleep called too many times, called %d but expected %d", timesCalled, callsExpected)
		}
	})
	assert.Equal(t, apitask.TaskRunning, mTask.GetSentStatus())

	// Expectations to verify that the task gets removed
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(
		map[string]*apicontainer.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mTask.cleanupTask(taskStoppedDuration)
	assert.Equal(t, apitask.TaskStopped, mTask.GetSentStatus())
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
	mTask.SetKnownStatus(apitask.TaskStopped)
	mTask.SetSentStatus(apitask.TaskRunning)

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
	assert.Equal(t, apitask.TaskRunning, mTask.GetSentStatus())

	// No cleanup expected
	mTask.cleanupTask(taskStoppedDuration)
	assert.Equal(t, apitask.TaskRunning, mTask.GetSentStatus())
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

	mTask.SetKnownStatus(apitask.TaskStopped)
	mTask.SetSentStatus(apitask.TaskStopped)
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
					KnownStatusUnsafe:   apitask.TaskRunning,
					DesiredStatusUnsafe: apitask.TaskRunning,
				},
				_time:       mockTime,
				acsMessages: make(chan acsTransition),
			}
			if tc.result {
				mockTime.EXPECT().AfterFunc(gomock.Any(), gomock.Any()).Return(mockTimer)
				mockTimer.EXPECT().Stop()
				go func() { task.acsMessages <- acsTransition{desiredStatus: apitask.TaskRunning} }()
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

	mTask.SetKnownStatus(apitask.TaskStopped)
	mTask.SetSentStatus(apitask.TaskStopped)
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
	mTask.SetKnownStatus(apitask.TaskStopped)
	mTask.SetSentStatus(apitask.TaskStopped)
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
	mTask.SetKnownStatus(apitask.TaskStopped)
	mTask.SetSentStatus(apitask.TaskStopped)
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

	mTask.SetKnownStatus(apitask.TaskRunning)
	mTask.SetSentStatus(apitask.TaskRunning)
	container := mTask.Containers[0]
	container.HealthCheckType = "docker"

	containerChange := dockerContainerChange{
		container: container,
		event: dockerapi.DockerContainerChangeEvent{
			Status: apicontainer.ContainerRunning,
			DockerContainerMetadata: dockerapi.DockerContainerMetadata{
				DockerID: "dockerID",
				Health: apicontainer.HealthStatus{
					Status: apicontainer.ContainerHealthy,
					Output: "health check succeed",
				},
			},
		},
	}

	mTask.handleContainerChange(containerChange)

	containerHealth := container.GetHealthStatus()
	assert.Equal(t, containerHealth.Status, apicontainer.ContainerHealthy)
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
		mockResource.EXPECT().ApplyTransition(taskresource.ResourceCreated).Return(nil),
		mockResource.EXPECT().GetName().AnyTimes(),
		mockResource.EXPECT().StatusString(taskresource.ResourceCreated).AnyTimes(),
	)
	assert.NoError(t, task.applyResourceState(mockResource, taskresource.ResourceCreated))
}

func TestApplyResourceStateFailures(t *testing.T) {
	testCases := []struct {
		Name      string
		ResStatus taskresource.ResourceStatus
		Error     error
	}{
		{
			Name:      "no valid state transition",
			ResStatus: taskresource.ResourceRemoved,
			Error:     errors.New("transition impossible"),
		},
		{
			Name:      "transition error",
			ResStatus: taskresource.ResourceCreated,
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

func getTestConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.TaskCPUMemLimit = config.ExplicitlyDisabled
	return cfg
}
