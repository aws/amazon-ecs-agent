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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	mock_statemanager "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control/mock_control"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/ssmsecret"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cgroupMountPath = "/sys/fs/cgroup"

	testTaskARN        = "arn:aws:ecs:region:account-id:task/task-id"
	testTaskDefFamily  = "testFamily"
	testTaskDefVersion = "1"
)

func init() {
	defaultConfig = config.DefaultConfig()
	defaultConfig.TaskCPUMemLimit = config.ExplicitlyDisabled
}

// TestResourceContainerProgression tests the container progression based on a
// resource dependency
func TestResourceContainerProgression(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer := sleepTask.Containers[0]

	sleepContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	sleepContainer.BuildResourceDependency("cgroup", resourcestatus.ResourceCreated, apicontainerstatus.ContainerPulled)

	mockControl := mock_control.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	taskID, err := sleepTask.GetID()
	assert.NoError(t, err)
	cgroupMemoryPath := fmt.Sprintf("/sys/fs/cgroup/memory/ecs/%s/memory.use_hierarchy", taskID)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)
	cgroupResource := cgroup.NewCgroupResource(sleepTask.Arn, mockControl, mockIO, cgroupRoot, cgroupMountPath, specs.LinuxResources{})

	sleepTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	sleepTask.AddResource("cgroup", cgroupResource)
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	// containerEventsWG is used to force the test to wait until the container created and started
	// events are processed
	containerEventsWG := sync.WaitGroup{}
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	gomock.InOrder(
		// Ensure that the resource is created first
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(nil, nil),
		mockIO.EXPECT().WriteFile(cgroupMemoryPath, gomock.Any(), gomock.Any()).Return(nil),
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes(),
		client.EXPECT().PullImage(gomock.Any(), sleepContainer.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}),
		imageManager.EXPECT().RecordContainerReference(sleepContainer).Return(nil),
		imageManager.EXPECT().GetImageStateFromImageName(sleepContainer.Image).Return(nil, false),
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx interface{}, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig, containerName string, z time.Duration) {
				assert.True(t, strings.Contains(containerName, sleepContainer.Name))
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(apicontainerstatus.ContainerCreated)
					containerEventsWG.Done()
				}()
			}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID + ":" + sleepContainer.Name}),
		// Next, the sleep container is started
		client.EXPECT().StartContainer(gomock.Any(), containerID+":"+sleepContainer.Name, defaultConfig.ContainerStartTimeout).Do(
			func(ctx interface{}, id string, timeout time.Duration) {
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(apicontainerstatus.ContainerRunning)
					containerEventsWG.Done()
				}()
			}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID + ":" + sleepContainer.Name}),
	)

	addTaskToEngine(t, ctx, taskEngine, sleepTask, mockTime, &containerEventsWG)

	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()

	// Simulate a container stop event from docker
	eventStream <- dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerStopped,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: containerID + ":" + sleepContainer.Name,
			ExitCode: aws.Int(exitCode),
		},
	}
	waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
}

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	cgroupResource := cgroup.NewCgroupResource("", mockControl, nil, "cgroupRoot", "", specs.LinuxResources{})
	task := &apitask.Task{
		ENIs: []*apieni.ENI{
			{
				MacAddress: mac,
			},
		},
	}
	task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	task.AddResource("cgroup", cgroupResource)
	cfg := defaultConfig
	cfg.TaskCPUMemLimit = config.ExplicitlyEnabled
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockSaver := mock_statemanager.NewMockStateManager(ctrl)

	taskEngine := &DockerTaskEngine{
		state: mockState,
		saver: mockSaver,
		cfg:   &cfg,
	}

	gomock.InOrder(
		mockControl.EXPECT().Remove("cgroupRoot").Return(nil),
		mockState.EXPECT().RemoveTask(task),
		mockState.EXPECT().RemoveENIAttachment(mac),
		mockSaver.EXPECT().Save(),
	)

	taskEngine.deleteTask(task)
}

func TestDeleteTaskBranchENIEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	cgroupResource := cgroup.NewCgroupResource("", mockControl, nil, "cgroupRoot", "", specs.LinuxResources{})
	task := &apitask.Task{
		ENIs: []*apieni.ENI{
			{
				MacAddress:                   mac,
				InterfaceAssociationProtocol: apieni.VLANInterfaceAssociationProtocol,
			},
		},
	}
	task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	task.AddResource("cgroup", cgroupResource)
	cfg := defaultConfig
	cfg.TaskCPUMemLimit = config.ExplicitlyEnabled
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	mockSaver := mock_statemanager.NewMockStateManager(ctrl)

	taskEngine := &DockerTaskEngine{
		state: mockState,
		saver: mockSaver,
		cfg:   &cfg,
	}

	gomock.InOrder(
		mockControl.EXPECT().Remove("cgroupRoot").Return(nil),
		mockState.EXPECT().RemoveTask(task),
		mockSaver.EXPECT().Save(),
	)

	taskEngine.deleteTask(task)
}

// TestResourceContainerProgressionFailure ensures that task moves to STOPPED when
// resource creation fails
func TestResourceContainerProgressionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()
	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer := sleepTask.Containers[0]

	sleepContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	sleepContainer.BuildResourceDependency("cgroup", resourcestatus.ResourceCreated, apicontainerstatus.ContainerPulled)

	mockControl := mock_control.NewMockControl(ctrl)
	taskID, err := sleepTask.GetID()
	assert.NoError(t, err)
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)
	cgroupResource := cgroup.NewCgroupResource(sleepTask.Arn, mockControl, nil, cgroupRoot, cgroupMountPath, specs.LinuxResources{})

	sleepTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	sleepTask.AddResource("cgroup", cgroupResource)
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	gomock.InOrder(
		// resource creation failure
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(nil, errors.New("cgroup create error")),
	)
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()

	err = taskEngine.Init(ctx)
	assert.NoError(t, err)

	taskEngine.AddTask(sleepTask)
	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()
	waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
}

func TestTaskCPULimitHappyPath(t *testing.T) {
	testcases := []struct {
		name                string
		metadataCreateError error
		metadataUpdateError error
		metadataCleanError  error
		taskCPULimit        config.Conditional
	}{
		{
			name:                "Task CPU Limit Succeeds",
			metadataCreateError: nil,
			metadataUpdateError: nil,
			metadataCleanError:  nil,
			taskCPULimit:        config.ExplicitlyEnabled,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			metadataConfig := defaultConfig
			metadataConfig.TaskCPUMemLimit = tc.taskCPULimit
			metadataConfig.ContainerMetadataEnabled = true
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, metadataManager := mocks(
				t, ctx, &metadataConfig)
			defer ctrl.Finish()

			roleCredentials := credentials.TaskIAMRoleCredentials{
				IAMRoleCredentials: credentials.IAMRoleCredentials{CredentialsID: "credsid"},
			}
			credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(roleCredentials, true).AnyTimes()
			credentialsManager.EXPECT().RemoveCredentials(credentialsID)

			sleepTask := testdata.LoadTask("sleep5")
			sleepTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
			sleepContainer := sleepTask.Containers[0]
			sleepContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
			sleepTask.SetCredentialsID(credentialsID)
			eventStream := make(chan dockerapi.DockerContainerChangeEvent)
			// containerEventsWG is used to force the test to wait until the container created and started
			// events are processed
			containerEventsWG := sync.WaitGroup{}

			client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
			containerName := make(chan string)
			go func() {
				name := <-containerName
				setCreatedContainerName(name)
			}()
			mockControl := mock_control.NewMockControl(ctrl)
			mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
			taskID, err := sleepTask.GetID()
			assert.NoError(t, err)
			cgroupMemoryPath := fmt.Sprintf("/sys/fs/cgroup/memory/ecs/%s/memory.use_hierarchy", taskID)
			if tc.taskCPULimit.Enabled() {
				// TODO Currently, the resource Setup() method gets invoked multiple
				// times for a task. This is really a bug and a fortunate occurrence
				// that cgroup creation APIs behave idempotently.
				//
				// This should be modified so that 'Setup' is invoked exactly once
				// by moving the cgroup creation to a "resource setup" step in the
				// task life-cycle and performing the setup only in this stage
				taskEngine.(*DockerTaskEngine).resourceFields = &taskresource.ResourceFields{
					Control: mockControl,
					ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
						IOUtil: mockIO,
					},
				}
				mockControl.EXPECT().Exists(gomock.Any()).Return(false)
				mockControl.EXPECT().Create(gomock.Any()).Return(nil, nil)
				mockIO.EXPECT().WriteFile(cgroupMemoryPath, gomock.Any(), gomock.Any()).Return(nil)
			}

			for _, container := range sleepTask.Containers {
				validateContainerRunWorkflow(t, container, sleepTask, imageManager,
					client, &roleCredentials, &containerEventsWG,
					eventStream, containerName, func() {
						metadataManager.EXPECT().Create(gomock.Any(), gomock.Any(),
							gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.metadataCreateError)
						metadataManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(),
							gomock.Any()).Return(tc.metadataUpdateError)
					})
			}

			client.EXPECT().Info(gomock.Any(), gomock.Any()).Return(
				types.Info{}, nil)
			addTaskToEngine(t, ctx, taskEngine, sleepTask, mockTime, &containerEventsWG)
			cleanup := make(chan time.Time, 1)
			defer close(cleanup)
			mockTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
			client.EXPECT().DescribeContainer(gomock.Any(), gomock.Any()).AnyTimes()
			// Simulate a container stop event from docker
			eventStream <- dockerapi.DockerContainerChangeEvent{
				Status: apicontainerstatus.ContainerStopped,
				DockerContainerMetadata: dockerapi.DockerContainerMetadata{
					DockerID: containerID,
					ExitCode: aws.Int(exitCode),
				},
			}

			// StopContainer might be invoked if the test execution is slow, during
			// the cleanup phase. Account for that.
			client.EXPECT().StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
				dockerapi.DockerContainerMetadata{DockerID: containerID}).AnyTimes()
			waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
			// This ensures that managedTask.waitForStopReported makes progress
			sleepTask.SetSentStatus(apitaskstatus.TaskStopped)
			// Extra events should not block forever; duplicate acs and docker events are possible
			go func() { eventStream <- createDockerEvent(apicontainerstatus.ContainerStopped) }()
			go func() { eventStream <- createDockerEvent(apicontainerstatus.ContainerStopped) }()

			sleepTaskStop := testdata.LoadTask("sleep5")
			sleepTaskStop.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
			sleepContainer = sleepTaskStop.Containers[0]
			sleepContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
			sleepTaskStop.SetCredentialsID(credentialsID)
			sleepTaskStop.SetDesiredStatus(apitaskstatus.TaskStopped)
			taskEngine.AddTask(sleepTaskStop)
			// As above, duplicate events should not be a problem
			taskEngine.AddTask(sleepTaskStop)
			taskEngine.AddTask(sleepTaskStop)
			cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)
			if tc.taskCPULimit.Enabled() {
				mockControl.EXPECT().Remove(cgroupRoot).Return(nil)
			}
			// Expect a bunch of steady state 'poll' describes when we trigger cleanup
			client.EXPECT().RemoveContainer(gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(ctx interface{}, removedContainerName string, timeout time.Duration) {
					assert.Equal(t, getCreatedContainerName(), removedContainerName,
						"Container name mismatch")
				}).Return(nil)

			imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any())
			metadataManager.EXPECT().Clean(gomock.Any()).Return(tc.metadataCleanError)
			// trigger cleanup
			cleanup <- time.Now()
			go func() { eventStream <- createDockerEvent(apicontainerstatus.ContainerStopped) }()
			// Wait for the task to actually be dead; if we just fallthrough immediately,
			// the remove might not have happened (expectation failure)
			for {
				tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
				if len(tasks) == 0 {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}

func TestCreateFirelensContainer(t *testing.T) {
	rawHostConfigInput := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
			Type: logDriverTypeFirelens,
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	require.NoError(t, err)

	getTask := func(firelensConfigType string) *apitask.Task {
		task := &apitask.Task{
			Arn:     testTaskARN,
			Family:  testTaskDefFamily,
			Version: testTaskDefVersion,
			Containers: []*apicontainer.Container{
				{
					Name: "firelens",
					FirelensConfig: &apicontainer.FirelensConfig{
						Type: firelensConfigType,
						Options: map[string]string{
							"enable-ecs-log-metadata": "true",
							"config-file-type":        "s3",
							"config-file-value":       "arn:aws:s3:::bucket/key",
						},
					},
				},
				{
					Name: "logsender",
					Secrets: []apicontainer.Secret{
						{
							Name:      "secret-name",
							ValueFrom: "secret-value-from",
							Provider:  apicontainer.SecretProviderSSM,
							Target:    apicontainer.SecretTargetLogDriver,
							Region:    "us-west-2",
						},
					},
					DockerConfig: apicontainer.DockerConfig{
						HostConfig: func(s string) *string {
							return &s
						}(string(rawHostConfig)),
					},
				},
			},
		}

		task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
		ssmRes := &ssmsecret.SSMSecretResource{}
		ssmRes.SetCachedSecretValue("secret-value-from_us-west-2", "secret-val")
		task.AddResource(ssmsecret.ResourceName, ssmRes)
		return task
	}

	testCases := []struct {
		name                        string
		task                        *apitask.Task
		expectedGeneratedConfigBind string
		expectedS3ConfigBind        string
		expectedSocketBind          string
		expectedLogOptionEnv        string
	}{
		{
			name:                        "test create fluentd firelens container",
			task:                        getTask(firelens.FirelensConfigTypeFluentd),
			expectedGeneratedConfigBind: defaultConfig.DataDirOnHost + "/data/firelens/task-id/config/fluent.conf:/fluentd/etc/fluent.conf",
			expectedS3ConfigBind:        defaultConfig.DataDirOnHost + "/data/firelens/task-id/config/external.conf:/fluentd/etc/external.conf",
			expectedSocketBind:          defaultConfig.DataDirOnHost + "/data/firelens/task-id/socket/:/var/run/",
			expectedLogOptionEnv:        "secret-name_1=secret-val",
		},
		{
			name:                        "test create fluentbit firelens container",
			task:                        getTask(firelens.FirelensConfigTypeFluentbit),
			expectedGeneratedConfigBind: defaultConfig.DataDirOnHost + "/data/firelens/task-id/config/fluent.conf:/fluent-bit/etc/fluent-bit.conf",
			expectedS3ConfigBind:        defaultConfig.DataDirOnHost + "/data/firelens/task-id/config/external.conf:/fluent-bit/etc/external.conf",
			expectedSocketBind:          defaultConfig.DataDirOnHost + "/data/firelens/task-id/socket/:/var/run/",
			expectedLogOptionEnv:        "secret-name_1=secret-val",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, mockTime, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
			defer ctrl.Finish()

			mockTime.EXPECT().Now().AnyTimes()
			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(ctx context.Context,
					config *dockercontainer.Config,
					hostConfig *dockercontainer.HostConfig,
					name string,
					timeout time.Duration) {
					assert.Contains(t, hostConfig.Binds, tc.expectedGeneratedConfigBind)
					assert.Contains(t, hostConfig.Binds, tc.expectedS3ConfigBind)
					assert.Contains(t, hostConfig.Binds, tc.expectedSocketBind)
					assert.Contains(t, config.Env, tc.expectedLogOptionEnv)
				})
			ret := taskEngine.(*DockerTaskEngine).createContainer(tc.task, tc.task.Containers[0])
			assert.NoError(t, ret.Error)
		})
	}
}
