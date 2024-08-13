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
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/dependencygraph"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	mock_taskresource "github.com/aws/amazon-ecs-agent/agent/taskresource/mocks"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	apiresource "github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/container/restart"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"
	mock_csiclient "github.com/aws/amazon-ecs-agent/ecs-agent/csiclient/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	mock_ttime "github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime/mocks"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
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
			Name:                            "Stop timed out",
			EventStatus:                     apicontainerstatus.ContainerStopped,
			CurrentContainerKnownStatus:     apicontainerstatus.ContainerRunning,
			Error:                           &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet: true,
			ExpectedContainerKnownStatus:    apicontainerstatus.ContainerStopped,
			ExpectedOK:                      true,
		},
		{
			Name:                        "Retriable error with stop",
			EventStatus:                 apicontainerstatus.ContainerStopped,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerRunning,
			Error: &dockerapi.CannotStopContainerError{
				FromError: errors.New(""),
			},
			ExpectedContainerKnownStatusSet: true,
			ExpectedContainerKnownStatus:    apicontainerstatus.ContainerStopped,
			ExpectedOK:                      true,
		},
		{
			Name:                        "Unretriable error with Stop",
			EventStatus:                 apicontainerstatus.ContainerStopped,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerRunning,
			Error: &dockerapi.CannotStopContainerError{
				FromError: dockerapi.NoSuchContainerError{},
			},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainerstatus.ContainerStopped,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            true,
		},
		{
			Name:              "Manifest pull failed - default pull behavior",
			EventStatus:       apicontainerstatus.ContainerManifestPulled,
			ImagePullBehavior: config.ImagePullDefaultBehavior,
			Error:             &dockerapi.DockerTimeoutError{},
			ExpectedOK:        true,
		},
		{
			Name:                             "Manifest pull failed - always pull behavior",
			EventStatus:                      apicontainerstatus.ContainerManifestPulled,
			ImagePullBehavior:                config.ImagePullAlwaysBehavior,
			Error:                            &dockerapi.DockerTimeoutError{},
			ExpectedTaskDesiredStatusStopped: true,
			ExpectedOK:                       false,
		},
		{
			Name:                             "Manifest pull failed - once pull behavior",
			EventStatus:                      apicontainerstatus.ContainerManifestPulled,
			ImagePullBehavior:                config.ImagePullOnceBehavior,
			Error:                            &dockerapi.DockerTimeoutError{},
			ExpectedTaskDesiredStatusStopped: true,
			ExpectedOK:                       false,
		},
		{
			Name:              "Manifest pull failed - prefer-cached pull behavior",
			EventStatus:       apicontainerstatus.ContainerManifestPulled,
			ImagePullBehavior: config.ImagePullPreferCachedBehavior,
			Error:             &dockerapi.DockerTimeoutError{},
			ExpectedOK:        true,
		},
		{
			Name:                            "Pull failed",
			Error:                           &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet: true,
			EventStatus:                     apicontainerstatus.ContainerPulled,
			ExpectedOK:                      true,
		},
		{
			Name:                                  "Container vanished between pull and running",
			EventStatus:                           apicontainerstatus.ContainerRunning,
			CurrentContainerKnownStatus:           apicontainerstatus.ContainerPulled,
			Error:                                 &ContainerVanishedError{},
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
			Name:                                  "Start timed out",
			EventStatus:                           apicontainerstatus.ContainerRunning,
			CurrentContainerKnownStatus:           apicontainerstatus.ContainerCreated,
			Error:                                 &dockerapi.DockerTimeoutError{},
			ExpectedContainerKnownStatusSet:       true,
			ExpectedContainerKnownStatus:          apicontainerstatus.ContainerCreated,
			ExpectedContainerDesiredStatusStopped: true,
			ExpectedOK:                            false,
		},
		{
			Name:                        "Start failed with EOF error",
			EventStatus:                 apicontainerstatus.ContainerRunning,
			CurrentContainerKnownStatus: apicontainerstatus.ContainerCreated,
			Error: &dockerapi.CannotStartContainerError{
				FromError: errors.New("error during connect: Post http://%2Fvar%2Frun%2Fdocker.sock/v1.19/containers/containerid/start: EOF"),
			},
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
			Name:                                  "Create timed out",
			EventStatus:                           apicontainerstatus.ContainerCreated,
			CurrentContainerKnownStatus:           apicontainerstatus.ContainerPulled,
			Error:                                 &dockerapi.DockerTimeoutError{},
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := mock_dockerapi.NewMockDockerClient(ctrl)

			if tc.EventStatus == apicontainerstatus.ContainerStopped {
				client.EXPECT().SystemPing(gomock.Any(), gomock.Any()).Return(dockerapi.PingResponse{}).
					Times(1)
			}

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
				engine:       &DockerTaskEngine{},
				cfg:          &config.Config{ImagePullBehavior: tc.ImagePullBehavior},
				dockerClient: client,
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
		reason                       dependencygraph.DependencyError
	}{
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running
		// The expected next status is MANIFEST_PULLED
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerManifestPulled, true, nil},
		// NONE -> RESOURCES_PROVISIONED transition is allowed and actionable, when desired
		// is Running. The expected next status is MANIFEST_PULLED
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerManifestPulled, true, nil},
		// NONE -> NONE transition is not be allowed and is not actionable,
		// when desired is Running
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// NONE -> STOPPED transition will result in STOPPED and is allowed, but not
		// actionable, when desired is STOPPED
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, false, nil},
		// MANIFEST_PULLED -> PULLED transition is allowed and actionable, when desired is Running
		{apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerPulled, true, nil},
		// MANIFEST_PULLED -> PULLED transition is allowed and actionable, when desired is RESOURCES_PROVISIONED
		{apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerPulled, true, nil},
		// MANIFEST_PULLED -> MANIFEST_PULLED transition is not allowed and not actionable
		{apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// MANIFEST_PULLED -> NONE transition is not allowed and not actionable
		{apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStatusNone, false, dependencygraph.ContainerPastDesiredStatusErr},
		// MANIFEST_PULLED -> STOPPED transition will result in STOPPED and is allowed, but not actionable
		{apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, false, nil},
		// PULLED -> RUNNING transition is allowed and actionable, when desired is Running
		// The expected next status is Created
		{apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerCreated, true, nil},
		// PULLED -> RESOURCES_PROVISIONED transition is allowed and actionable, when desired
		// is Running. The expected next status is Created
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
		// is Running. The expected next status is Running
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
		// is Running. The expected next status is STOPPED
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
		// NONE -> RUNNING transition is not allowed and not actionable, when manifest_pull depends on create and dependency is None
		{
			name:                         "manifest_pull depends on created, dependency is none",
			containerCurrentStatus:       apicontainerstatus.ContainerStatusNone,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerManifestPulled,
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerCreated,
			expectedContainerStatus:      apicontainerstatus.ContainerStatusNone,
			expectedTransitionActionable: false,
			reason:                       dependencygraph.ErrContainerDependencyNotResolved,
		},
		// NONE -> RUNNING transition is not allowed and not actionable, when desired is Running and dependency is Created
		{
			name:                         "manifest_pull depends on running, dependency is created",
			containerCurrentStatus:       apicontainerstatus.ContainerStatusNone,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerManifestPulled,
			dependencyCurrentStatus:      apicontainerstatus.ContainerCreated,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerRunning,
			expectedContainerStatus:      apicontainerstatus.ContainerStatusNone,
			expectedTransitionActionable: false,
			reason:                       dependencygraph.ErrContainerDependencyNotResolved,
		},
		// NONE -> RUNNING transition is not allowed and not actionable, when pull depends on create and dependency is None
		{
			name:                         "pull depends on created, current is manifest_pulled, dependency is none",
			containerCurrentStatus:       apicontainerstatus.ContainerManifestPulled,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerPulled,
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerCreated,
			expectedContainerStatus:      apicontainerstatus.ContainerStatusNone,
			expectedTransitionActionable: false,
			reason:                       dependencygraph.ErrContainerDependencyNotResolved,
		},
		// NONE -> RUNNING transition is not allowed and not actionable, when desired is Running and dependency is Created
		{
			name:                         "pull depends on running, current is manifest_pulled, dependency is created",
			containerCurrentStatus:       apicontainerstatus.ContainerManifestPulled,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerPulled,
			dependencyCurrentStatus:      apicontainerstatus.ContainerCreated,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerRunning,
			expectedContainerStatus:      apicontainerstatus.ContainerStatusNone,
			expectedTransitionActionable: false,
			reason:                       dependencygraph.ErrContainerDependencyNotResolved,
		},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Running
		// The expected next status is Pulled
		{
			name:                         "pull depends on running, dependency is running, next status is pulled",
			containerCurrentStatus:       apicontainerstatus.ContainerManifestPulled,
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
			name:                         "pull depends on running, dependency is stopped, next status is pulled",
			containerCurrentStatus:       apicontainerstatus.ContainerManifestPulled,
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
			name:                         "create depends on running, dependency is none, next status is manifest_pulled",
			containerCurrentStatus:       apicontainerstatus.ContainerStatusNone,
			containerDesiredStatus:       apicontainerstatus.ContainerRunning,
			containerDependentStatus:     apicontainerstatus.ContainerCreated,
			dependencyCurrentStatus:      apicontainerstatus.ContainerStatusNone,
			dependencySatisfiedStatus:    apicontainerstatus.ContainerRunning,
			expectedContainerStatus:      apicontainerstatus.ContainerManifestPulled,
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
		// The expected next status is MANIFEST_PULLED
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerManifestPulled, true, nil},
		// NONE -> RUNNING transition is allowed and actionable, when desired is Running and dependency is Stopped
		// The expected next status is MANIFEST_PULLED
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerManifestPulled, true, nil},
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
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerManifestPulled, "existed", true, true, nil},
		// MANIFEST_PULLED -> RUNNING transition is not allowed when the credentials don't exist
		{apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerStatusNone, "not_existed", true, false, dependencygraph.CredentialsNotResolvedErr},
		// MANIFEST_PULLED -> RUNNING transition is allowed when the credentials exist
		{apicontainerstatus.ContainerManifestPulled, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerPulled, "existed", true, true, nil},
		// PULLED -> RUNNING transition is allowed even the credentials is required
		{apicontainerstatus.ContainerPulled, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerCreated, "not_existed", true, true, nil},
		// NONE -> STOPPED transition is allowed even the credentials is required
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped, "not_existed", true, false, nil},
		// NONE -> RUNNING transition is allowed when the container doesn't use execution credentials
		{apicontainerstatus.ContainerStatusNone, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerManifestPulled, "not_existed", false, true, nil},
	}

	taskEngine := &DockerTaskEngine{
		credentialsManager: credentials.NewManager(),
	}

	err := taskEngine.credentialsManager.SetTaskCredentials(&credentials.TaskIAMRoleCredentials{
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
			assert.Equal(t, tc.expectedTransitionActionable, transition.actionRequired, "Mismatch transition actionable")
		})
	}
}

func TestContainerNextStateWithAvoidingDanglingContainers(t *testing.T) {
	container := &apicontainer.Container{
		DesiredStatusUnsafe:       apicontainerstatus.ContainerStopped,
		KnownStatusUnsafe:         apicontainerstatus.ContainerCreated,
		AppliedStatus:             apicontainerstatus.ContainerRunning,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}

	task := &managedTask{
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{
				container,
			},
			DesiredStatusUnsafe: apitaskstatus.TaskStopped,
		},
		engine: &DockerTaskEngine{},
	}
	transition := task.containerNextState(container)
	assert.Equal(t, apicontainerstatus.ContainerStatusNone, transition.nextState,
		"Expected next state [%s] != Retrieved next state [%s]",
		apicontainerstatus.ContainerStatusNone.String(), transition.nextState.String())
	assert.Equal(t, false, transition.actionRequired, "Mismatch transition actionable")
	assert.Equal(t, nil, transition.reason, "Mismatch transition possible")
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
			canTransition, _, transitions, _ := task.startContainerTransitions(
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

	canTransition, _, transitions, _ := task.startContainerTransitions(
		func(cont *apicontainer.Container, nextStatus apicontainerstatus.ContainerStatus) {
			t.Error("Transition function should not be called when no transitions are possible")
		})
	assert.False(t, canTransition)
	assert.Empty(t, transitions)
}

func TestStartContainerTransitionsWithTerminalError(t *testing.T) {
	firstContainerName := "container1"
	firstContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainerstatus.ContainerStopped,
		DesiredStatusUnsafe: apicontainerstatus.ContainerStopped,
		KnownExitCodeUnsafe: aws.Int(1), // This simulated the container has stopped unsuccessfully
		Name:                firstContainerName,
	}
	secondContainerName := "container2"
	secondContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainerstatus.ContainerCreated,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		Name:                secondContainerName,
		DependsOnUnsafe: []apicontainer.DependsOn{
			{
				ContainerName: firstContainerName,
				Condition:     "SUCCESS", // This means this condition can never be fulfilled since container1 has exited with non-zero code
			},
		},
	}
	thirdContainerName := "container3"
	thirdContainer := &apicontainer.Container{
		KnownStatusUnsafe:   apicontainerstatus.ContainerCreated,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		Name:                thirdContainerName,
		DependsOnUnsafe: []apicontainer.DependsOn{
			{
				ContainerName: secondContainerName,
				Condition:     "SUCCESS", // This means this condition can never be fulfilled since container2 has exited with non-zero code
			},
		},
	}
	dockerMessagesChan := make(chan dockerContainerChange)
	task := &managedTask{
		Task: &apitask.Task{
			Containers: []*apicontainer.Container{
				firstContainer,
				secondContainer,
				thirdContainer,
			},
			DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		},
		engine:         &DockerTaskEngine{},
		dockerMessages: dockerMessagesChan,
	}

	canTransition, _, transitions, errs := task.startContainerTransitions(
		func(cont *apicontainer.Container, nextStatus apicontainerstatus.ContainerStatus) {
			t.Error("Transition function should not be called when no transitions are possible")
		})
	assert.False(t, canTransition)
	assert.Empty(t, transitions)
	assert.Equal(t, 3, len(errs)) // first error is just indicating container1 is at desired status, following errors should be terminal
	assert.False(t, errs[0].(dependencygraph.DependencyError).IsTerminal(), "Error should NOT  be terminal")
	assert.True(t, errs[1].(dependencygraph.DependencyError).IsTerminal(), "Error should be terminal")
	assert.True(t, errs[2].(dependencygraph.DependencyError).IsTerminal(), "Error should be terminal")

	stoppedMessages := make(map[string]dockerContainerChange)
	// verify we are sending STOPPED message
	for i := 0; i < 2; i++ {
		select {
		case msg := <-dockerMessagesChan:
			stoppedMessages[msg.container.Name] = msg
		case <-time.After(time.Second):
			t.Fatal("Timed out waiting for docker messages")
			break
		}
	}

	assert.Equal(t, secondContainer, stoppedMessages[secondContainerName].container)
	assert.Equal(t, apicontainerstatus.ContainerStopped, stoppedMessages[secondContainerName].event.Status)
	assert.Error(t, stoppedMessages[secondContainerName].event.DockerContainerMetadata.Error)
	assert.Equal(t, 143, *stoppedMessages[secondContainerName].event.DockerContainerMetadata.ExitCode)

	assert.Equal(t, thirdContainer, stoppedMessages[thirdContainerName].container)
	assert.Equal(t, apicontainerstatus.ContainerStopped, stoppedMessages[thirdContainerName].event.Status)
	assert.Error(t, stoppedMessages[thirdContainerName].event.DockerContainerMetadata.Error)
	assert.Equal(t, 143, *stoppedMessages[thirdContainerName].event.DockerContainerMetadata.ExitCode)
}

func TestStartContainerTransitionsInvokesHandleContainerChange(t *testing.T) {
	eventStreamName := "TESTTASKENGINE"

	// Create a container with the intent to do
	// CREATED -> STOPPED transition. This triggers
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

	hostResourceManager := NewHostResourceManager(getTestHostResources())
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
			dataClient:                 data.NewNoopClient(),
			hostResourceManager:        &hostResourceManager,
		},
		stateChangeEvents:          stateChangeEvents,
		containerChangeEventStream: containerChangeEventStream,
		dockerMessages:             make(chan dockerContainerChange),
		ctx:                        context.TODO(),
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
	canTransition, _, transitions, _ := task.startContainerTransitions(
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
	hostResourceManager := NewHostResourceManager(getTestHostResources())
	task := &managedTask{
		Task: &apitask.Task{
			Containers:          []*apicontainer.Container{},
			DesiredStatusUnsafe: apitaskstatus.TaskStopped,
		},
		engine: &DockerTaskEngine{
			stateChangeEvents:   stateChangeEvents,
			hostResourceManager: &hostResourceManager,
		},
		stateChangeEvents: stateChangeEvents,
		ctx:               context.TODO(),
	}
	eventsGenerated := sync.WaitGroup{}
	eventsGenerated.Add(1)

	go func() {
		event := <-stateChangeEvents
		assert.Equal(t, event.(api.TaskStateChange).Reason, taskUnableToTransitionToStoppedReason)
		eventsGenerated.Done()
	}()

	task.handleContainersUnableToTransitionState()
	eventsGenerated.Wait()

	assert.Equal(t, task.GetDesiredStatus(), apitaskstatus.TaskStopped)
}

func TestOnContainersUnableToTransitionStateForDesiredRunningTask(t *testing.T) {
	for _, tc := range []struct {
		knownStatus                    apicontainerstatus.ContainerStatus
		expectedContainerDesiredStatus apicontainerstatus.ContainerStatus
		expectedTaskDesiredStatus      apitaskstatus.TaskStatus
	}{
		{
			knownStatus:                    apicontainerstatus.ContainerCreated,
			expectedContainerDesiredStatus: apicontainerstatus.ContainerStopped,
			expectedTaskDesiredStatus:      apitaskstatus.TaskStopped,
		},
		{
			knownStatus:                    apicontainerstatus.ContainerRunning,
			expectedContainerDesiredStatus: apicontainerstatus.ContainerRunning,
			expectedTaskDesiredStatus:      apitaskstatus.TaskRunning,
		},
	} {
		t.Run(fmt.Sprintf("Essential container with knownStatus=%s", tc.knownStatus.String()), func(t *testing.T) {
			firstContainerName := "container1"
			firstContainer := &apicontainer.Container{
				KnownStatusUnsafe:   tc.knownStatus,
				DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
				Name:                firstContainerName,
				Essential:           true, // setting this to true since at least one container in the task must be essential.
			}
			task := &managedTask{
				Task: &apitask.Task{
					Containers: []*apicontainer.Container{
						firstContainer,
					},
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				engine: &DockerTaskEngine{
					dataClient: data.NewNoopClient(),
				},
				ctx: context.TODO(),
			}

			task.handleContainersUnableToTransitionState()
			assert.Equal(t, tc.expectedTaskDesiredStatus, task.GetDesiredStatus())
			assert.Equal(t, tc.expectedContainerDesiredStatus, task.Containers[0].GetDesiredStatus())
		})
	}
}

// TODO: Test progressContainers workflow

func TestHandleStoppedToSteadyStateTransition(t *testing.T) {
	taskEngine := &DockerTaskEngine{}
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
		dataClient:   data.NewNoopClient(),
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
	mTask.AddTaskENI(&ni.NetworkInterface{
		ID: "TestCleanupTaskENIs",
		IPV4Addresses: []*ni.IPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		},
		MacAddress: mac,
		IPV6Addresses: []*ni.IPV6Address{
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
	mockState.EXPECT().ENIByMac(mac)
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
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			task := &managedTask{
				ctx:    ctx,
				engine: &DockerTaskEngine{},
				Task: &apitask.Task{
					KnownStatusUnsafe:   apitaskstatus.TaskRunning,
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
				},
				acsMessages: make(chan acsTransition),
			}
			if tc.result {
				go func() { task.acsMessages <- acsTransition{desiredStatus: apitaskstatus.TaskRunning} }()
			}

			assert.Equal(t, tc.result, task.isWaitingForACSExecutionCredentials(tc.errs), tc.msg)
		})
	}
}

func TestCleanupTaskWithExecutionCredentials(t *testing.T) {
	cfg := getTestConfig()
	ctrl := gomock.NewController(t)
	mockTime := mock_ttime.NewMockTime(ctrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	mockImageManager := mock_engine.NewMockImageManager(ctrl)
	mockCredentialsManager := mock_credentials.NewMockManager(ctrl)
	mockResource := mock_taskresource.NewMockTaskResource(ctrl)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	taskEngine := &DockerTaskEngine{
		ctx:                ctx,
		cfg:                &cfg,
		dataClient:         data.NewNoopClient(),
		state:              mockState,
		client:             mockClient,
		imageManager:       mockImageManager,
		credentialsManager: mockCredentialsManager,
	}
	mTask := &managedTask{
		ctx:                      ctx,
		cancel:                   cancel,
		Task:                     testdata.LoadTask("sleep5"),
		credentialsManager:       mockCredentialsManager,
		_time:                    mockTime,
		engine:                   taskEngine,
		acsMessages:              make(chan acsTransition),
		dockerMessages:           make(chan dockerContainerChange),
		resourceStateChangeEvent: make(chan resourceStateChange),
		cfg:                      taskEngine.cfg,
	}

	mTask.Task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	mTask.Task.SetExecutionRoleCredentialsID("executionRoleCredentialsId")
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

	// Expectations to verify the execution credentials get removed
	mockCredentialsManager.EXPECT().RemoveCredentials("executionRoleCredentialsId")

	// Expectations to verify that the task gets removed
	mockState.EXPECT().ContainerMapByArn(mTask.Arn).Return(map[string]*apicontainer.DockerContainer{container.Name: dockerContainer}, true)
	mockClient.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(nil)
	mockImageManager.EXPECT().RemoveContainerReferenceFromImageState(container).Return(nil)
	mockState.EXPECT().RemoveTask(mTask.Task)
	mockResource.EXPECT().Cleanup()
	mockResource.EXPECT().GetName()
	mTask.cleanupTask(taskStoppedDuration)
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
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyEnabled
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
		cfg:                      taskEngine.cfg,
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
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyEnabled
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
		cfg:                      taskEngine.cfg,
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
		Task:                       testdata.LoadTask("sleep5TaskCgroup"),
		containerChangeEventStream: containerChangeEventStream,
		stateChangeEvents:          make(chan statechange.Event),
		ctx:                        context.TODO(),
		engine: &DockerTaskEngine{
			dataClient: data.NewNoopClient(),
		},
	}
	// Discard all the statechange events
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
	assert.Equal(t, apicontainerstatus.ContainerHealthy, containerHealth.Status)
	assert.Equal(t, "health check succeed", containerHealth.Output)
}

func TestHandleContainerChangeUpdateMetadataRedundant(t *testing.T) {
	eventStreamName := "TestHandleContainerChangeUpdateContainerHealth"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	containerChangeEventStream := eventstream.NewEventStream(eventStreamName, ctx)
	containerChangeEventStream.StartListening()

	mTask := &managedTask{
		Task:                       testdata.LoadTask("sleep5TaskCgroup"),
		containerChangeEventStream: containerChangeEventStream,
		stateChangeEvents:          make(chan statechange.Event),
		ctx:                        context.TODO(),
		engine: &DockerTaskEngine{
			dataClient: data.NewNoopClient(),
		},
	}
	// Discard all the statechange events
	defer discardEvents(mTask.stateChangeEvents)()

	mTask.SetKnownStatus(apitaskstatus.TaskRunning)
	mTask.SetSentStatus(apitaskstatus.TaskRunning)
	container := mTask.Containers[0]
	container.HealthCheckType = "docker"
	// Container already in RUNNING status
	container.SetKnownStatus(apicontainerstatus.ContainerRunning)

	timeNow := time.Now()
	exitCode := exitcodes.ExitError
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
				ExitCode:  &exitCode,
				CreatedAt: timeNow,
			},
		},
	}

	mTask.handleContainerChange(containerChange)

	containerHealth := container.GetHealthStatus()
	assert.Equal(t, apicontainerstatus.ContainerHealthy, containerHealth.Status)
	assert.Equal(t, "health check succeed", containerHealth.Output)
	containerExitCode := container.GetKnownExitCode()
	assert.Equal(t, exitCode, *containerExitCode)
	containerCreateTime := container.GetCreatedAt()
	assert.Equal(t, timeNow, containerCreateTime)
}

func waitForTaskDesiredStatus(mTask *managedTask, status apitaskstatus.TaskStatus) {
	for i := 0; i < 40; i++ {
		taskStatus := mTask.GetDesiredStatus()
		if taskStatus == status {
			return
		}
		time.Sleep(time.Millisecond * 250)
	}
}

func waitForRestartCount(container *apicontainer.Container, count int) {
	for i := 0; i < 40; i++ {
		restartCount := container.RestartTracker.GetRestartCount()
		if restartCount == count {
			return
		}
		time.Sleep(time.Millisecond * 250)
	}
}

func TestHandleContainerChangeStopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	containerChangeEventStream := eventstream.NewEventStream(t.Name(), ctx)
	containerChangeEventStream.StartListening()

	hostResourceManager := NewHostResourceManager(getTestHostResources())
	mTask := &managedTask{
		Task:                       testdata.LoadTask("sleep5"),
		containerChangeEventStream: containerChangeEventStream,
		stateChangeEvents:          make(chan statechange.Event),
		ctx:                        context.TODO(),
		engine: &DockerTaskEngine{
			dataClient:          data.NewNoopClient(),
			hostResourceManager: &hostResourceManager,
		},
	}
	// Discard all the statechange events
	defer discardEvents(mTask.stateChangeEvents)()

	mTask.SetKnownStatus(apitaskstatus.TaskRunning)
	mTask.SetSentStatus(apitaskstatus.TaskRunning)
	container := mTask.Containers[0]

	containerChange := dockerContainerChange{
		container: container,
		event: dockerapi.DockerContainerChangeEvent{
			Status: apicontainerstatus.ContainerStopped,
		},
	}

	mTask.handleContainerChange(containerChange)
	waitForTaskDesiredStatus(mTask, apitaskstatus.TaskStopped)
	assert.Equal(t, apitaskstatus.TaskStopped.String(), mTask.GetDesiredStatus().String(), "Expected task to change to stopped after container exit, since there is no restart policy")
}

func TestHandleContainerChangeStopped_WithRestartPolicy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	containerChangeEventStream := eventstream.NewEventStream(t.Name(), ctx)
	containerChangeEventStream.StartListening()

	ctrl := gomock.NewController(t)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	defer ctrl.Finish()

	cfg := getTestConfig()
	hostResourceManager := NewHostResourceManager(getTestHostResources())
	mTask := &managedTask{
		Task:                       testdata.LoadTask("sleep5RestartPolicy"),
		containerChangeEventStream: containerChangeEventStream,
		stateChangeEvents:          make(chan statechange.Event),
		ctx:                        context.TODO(),
		engine: &DockerTaskEngine{
			ctx:                 context.TODO(),
			cfg:                 &cfg,
			dataClient:          data.NewNoopClient(),
			hostResourceManager: &hostResourceManager,
			client:              mockClient,
		},
	}
	// Discard all the statechange events
	defer discardEvents(mTask.stateChangeEvents)()

	mTask.SetKnownStatus(apitaskstatus.TaskRunning)
	mTask.SetSentStatus(apitaskstatus.TaskRunning)
	container := mTask.Containers[0]
	container.RestartTracker = restart.NewRestartTracker(*container.RestartPolicy)

	exitCode := int(100)
	containerChange := dockerContainerChange{
		container: container,
		event: dockerapi.DockerContainerChangeEvent{
			Status: apicontainerstatus.ContainerStopped,
			DockerContainerMetadata: dockerapi.DockerContainerMetadata{
				ExitCode: &exitCode,
			},
		},
	}

	mockClient.EXPECT().StartContainer(gomock.Any(), container.RuntimeID, gomock.Any()).Return(dockerapi.DockerContainerMetadata{})
	assert.Equal(t, 0, container.RestartTracker.GetRestartCount(), "Before stop event, restart count should be 0")
	mTask.handleContainerChange(containerChange)
	// wait for restart count to increment
	waitForRestartCount(container, 1)
	assert.Equal(t, 1, container.RestartTracker.GetRestartCount(), "After stop event, container should have been restarted")
	assert.Equal(t, apitaskstatus.TaskRunning.String(), mTask.GetDesiredStatus().String(), "Expected task to be RUNNING since exited container should have restarted and task should be running")
}

func TestHandleContainerChangeStopped_WithRestartPolicy_RestartFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	containerChangeEventStream := eventstream.NewEventStream(t.Name(), ctx)
	containerChangeEventStream.StartListening()

	ctrl := gomock.NewController(t)
	mockClient := mock_dockerapi.NewMockDockerClient(ctrl)
	defer ctrl.Finish()

	cfg := getTestConfig()
	hostResourceManager := NewHostResourceManager(getTestHostResources())
	mTask := &managedTask{
		Task:                       testdata.LoadTask("sleep5RestartPolicy"),
		containerChangeEventStream: containerChangeEventStream,
		stateChangeEvents:          make(chan statechange.Event),
		ctx:                        context.TODO(),
		engine: &DockerTaskEngine{
			ctx:                 context.TODO(),
			cfg:                 &cfg,
			dataClient:          data.NewNoopClient(),
			hostResourceManager: &hostResourceManager,
			client:              mockClient,
		},
	}
	// Discard all the statechange events
	defer discardEvents(mTask.stateChangeEvents)()

	mTask.SetKnownStatus(apitaskstatus.TaskRunning)
	mTask.SetSentStatus(apitaskstatus.TaskRunning)
	container := mTask.Containers[0]
	container.RestartTracker = restart.NewRestartTracker(*container.RestartPolicy)

	exitCode := int(100)
	containerChange := dockerContainerChange{
		container: container,
		event: dockerapi.DockerContainerChangeEvent{
			Status: apicontainerstatus.ContainerStopped,
			DockerContainerMetadata: dockerapi.DockerContainerMetadata{
				ExitCode: &exitCode,
			},
		},
	}

	mockClient.EXPECT().StartContainer(gomock.Any(), container.RuntimeID, gomock.Any()).Return(dockerapi.DockerContainerMetadata{
		Error: dockerapi.CannotStartContainerError{fmt.Errorf("cannot start container")},
	})
	assert.Equal(t, 0, container.RestartTracker.GetRestartCount(), "Before stop event, restart count should be 0")
	mTask.handleContainerChange(containerChange)
	// wait for restart count to increment
	waitForRestartCount(container, 1)
	assert.Equal(t, 1, container.RestartTracker.GetRestartCount(), "After stop event, container should have been restarted")
	waitForTaskDesiredStatus(mTask, apitaskstatus.TaskStopped)
	assert.Equal(t, apitaskstatus.TaskStopped.String(), mTask.GetDesiredStatus().String(), "Expected task to be STOPPED since container had a restart policy, did restart, but the restart failed")
}

func TestHandleContainerChangeStopped_WithRestartPolicy_DesiredStopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	containerChangeEventStream := eventstream.NewEventStream(t.Name(), ctx)
	containerChangeEventStream.StartListening()

	cfg := getTestConfig()
	hostResourceManager := NewHostResourceManager(getTestHostResources())
	mTask := &managedTask{
		Task:                       testdata.LoadTask("sleep5RestartPolicy"),
		containerChangeEventStream: containerChangeEventStream,
		stateChangeEvents:          make(chan statechange.Event),
		ctx:                        context.TODO(),
		engine: &DockerTaskEngine{
			ctx:                 context.TODO(),
			cfg:                 &cfg,
			dataClient:          data.NewNoopClient(),
			hostResourceManager: &hostResourceManager,
		},
	}
	// Discard all the statechange events
	defer discardEvents(mTask.stateChangeEvents)()

	mTask.SetKnownStatus(apitaskstatus.TaskRunning)
	mTask.SetSentStatus(apitaskstatus.TaskRunning)
	container := mTask.Containers[0]
	container.RestartTracker = restart.NewRestartTracker(*container.RestartPolicy)

	exitCode := int(100)
	containerChange := dockerContainerChange{
		container: container,
		event: dockerapi.DockerContainerChangeEvent{
			Status: apicontainerstatus.ContainerStopped,
			DockerContainerMetadata: dockerapi.DockerContainerMetadata{
				ExitCode: &exitCode,
			},
		},
	}

	// Set desired status of container to stopped, this simulates what would happen
	// if user called the ecs.StopTask API.
	container.SetDesiredStatus(apicontainerstatus.ContainerStopped)

	assert.Equal(t, 0, container.RestartTracker.GetRestartCount(), "Before stop event, restart count should be 0")
	mTask.handleContainerChange(containerChange)
	// since container is not restarting, expect task status to change to STOPPED
	waitForTaskDesiredStatus(mTask, apitaskstatus.TaskStopped)
	assert.Equal(t, apitaskstatus.TaskStopped.String(), mTask.GetDesiredStatus().String(), "Expected task to change to stopped after container exit, since restart policy did not trigger")
	assert.Equal(t, 0, container.RestartTracker.GetRestartCount(), "After stop event, container should NOT have been restarted")
}

func TestWaitForResourceTransition(t *testing.T) {
	task := &managedTask{
		Task: &apitask.Task{
			ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		},
		ctx: context.TODO(),
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
		ctx: context.TODO(),
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
				ctx: context.TODO(),
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
			res := &volume.VolumeResource{Name: volumeName}
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
			mtask.AddResource(volumeName, res)
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
				ctx:                      ctx,
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
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	return cfg
}

// TestContainerNextStateDependsStoppedContainer tests the container that has
// dependency on other containers' stopped status should wait for other container
// to be stopped before it can be stopped
func TestContainerNextStateDependsStoppedContainer(t *testing.T) {
	testCases := []struct {
		// Known status of the dependent container
		knownStatus    apicontainerstatus.ContainerStatus
		actionRequired bool
		err            error
	}{
		{
			knownStatus:    apicontainerstatus.ContainerRunning,
			actionRequired: false,
			err:            dependencygraph.ErrContainerDependencyNotResolved,
		},
		{
			knownStatus:    apicontainerstatus.ContainerStopped,
			actionRequired: true,
			err:            nil,
		},
	}

	containerRunning := &apicontainer.Container{
		Name:              "containerrunning",
		KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
	}
	containerToBeStopped := &apicontainer.Container{
		Name:                "containertostop",
		KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
		DesiredStatusUnsafe: apicontainerstatus.ContainerStopped,
	}

	// Add a dependency of the container's stopped status
	containerToBeStopped.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	containerToBeStopped.BuildContainerDependency(containerRunning.Name, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped)

	mtask := managedTask{
		Task: &apitask.Task{
			Arn:        "task1",
			Containers: []*apicontainer.Container{containerRunning, containerToBeStopped},
		},
		engine: &DockerTaskEngine{},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Transition container to stopped but has dependent container container in %s", tc.knownStatus.String()), func(t *testing.T) {
			containerRunning.SetKnownStatus(tc.knownStatus)
			transition := mtask.containerNextState(containerToBeStopped)
			assert.Equal(t, tc.actionRequired, transition.actionRequired)
			assert.Equal(t, tc.err, transition.reason)
		})
	}
}

// TestTaskWaitForHostResources tests task queuing behavior based on available host resources
func TestTaskWaitForHostResources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// 1 vCPU available on host
	hostResourceManager := NewHostResourceManager(getTestHostResources())
	taskEngine := &DockerTaskEngine{
		managedTasks:           make(map[string]*managedTask),
		monitorQueuedTaskEvent: make(chan struct{}, 1),
		hostResourceManager:    &hostResourceManager,
	}
	go taskEngine.monitorQueuedTasks(ctx)
	// 3 tasks requesting 0.5 vCPUs each
	tasks := []*apitask.Task{}
	for i := 0; i < 3; i++ {
		task := testdata.LoadTask("sleep5")
		task.Arn = fmt.Sprintf("arn%d", i)
		task.CPU = float64(0.5)
		mtask := &managedTask{
			Task:                      task,
			engine:                    taskEngine,
			consumedHostResourceEvent: make(chan struct{}, 1),
		}
		tasks = append(tasks, task)
		taskEngine.managedTasks[task.Arn] = mtask
	}

	// acquire for host resources order arn0, arn1, arn2
	go func() {
		taskEngine.managedTasks["arn0"].waitForHostResources()
		taskEngine.managedTasks["arn1"].waitForHostResources()
		taskEngine.managedTasks["arn2"].waitForHostResources()
	}()
	time.Sleep(500 * time.Millisecond)

	// Verify waiting queue is waiting at arn2
	topTask, err := taskEngine.topTask()
	assert.NoError(t, err)
	assert.Equal(t, topTask.Arn, "arn2")

	// Remove 1 task
	taskResources := taskEngine.managedTasks["arn0"].ToHostResources()
	taskEngine.hostResourceManager.release("arn0", taskResources)
	taskEngine.wakeUpTaskQueueMonitor()

	time.Sleep(500 * time.Millisecond)

	// Verify arn2 got dequeued
	topTask, err = taskEngine.topTask()
	assert.Error(t, err)
}

func TestUnstageVolumes(t *testing.T) {
	tcs := []struct {
		name      string
		err       error
		numErrors int
	}{
		{
			name:      "Success",
			err:       nil,
			numErrors: 0,
		},
		{
			name:      "Failure",
			err:       errors.New("unable to unstage volume"),
			numErrors: 1,
		},
		{
			name:      "TimeoutFailure",
			err:       errors.New("rpc error: code = DeadlineExceeded desc = context deadline exceeded"),
			numErrors: 1,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			mtask := &managedTask{
				Task: &apitask.Task{
					ResourcesMapUnsafe:  make(map[string][]taskresource.TaskResource),
					DesiredStatusUnsafe: apitaskstatus.TaskRunning,
					Volumes: []apitask.TaskVolume{
						{
							Name: taskresourcevolume.TestVolumeName,
							Type: apiresource.EBSTaskAttach,
							Volume: &taskresourcevolume.EBSTaskVolumeConfig{
								VolumeId:             taskresourcevolume.TestVolumeId,
								VolumeName:           taskresourcevolume.TestVolumeId,
								VolumeSizeGib:        taskresourcevolume.TestVolumeSizeGib,
								SourceVolumeHostPath: taskresourcevolume.TestSourceVolumeHostPath,
								DeviceName:           taskresourcevolume.TestDeviceName,
								FileSystem:           taskresourcevolume.TestFileSystem,
							},
						},
					},
				},
				ctx:                      ctx,
				resourceStateChangeEvent: make(chan resourceStateChange),
			}
			mockCsiClient := mock_csiclient.NewMockCSIClient(mockCtrl)
			mockCsiClient.EXPECT().NodeUnstageVolume(gomock.Any(), "vol-12345", "/mnt/ecs/ebs/taskarn_vol-12345").Return(tc.err).MinTimes(1).MaxTimes(5)

			errors := mtask.UnstageVolumes(mockCsiClient)
			assert.Len(t, errors, tc.numErrors)
		})
	}
}
