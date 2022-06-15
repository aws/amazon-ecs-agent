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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"

	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"

	"github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
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

	testTaskDefFamily  = "testFamily"
	testTaskDefVersion = "1"
	containerNetNS     = "none"
)

func init() {
	defaultConfig = config.DefaultConfig()
	defaultConfig.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
}

// TestResourceContainerProgression tests the container progression based on a
// resource dependency
func TestResourceContainerProgression(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer := sleepTask.Containers[0]

	sleepContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	sleepContainer.BuildResourceDependency("cgroup", resourcestatus.ResourceCreated, apicontainerstatus.ContainerPulled)

	mockControl := mock_control.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	taskID := sleepTask.GetID()
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
		mockControl.EXPECT().Create(gomock.Any()).Return(nil),
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
	waitForStopEvents(t, taskEngine.StateChangeEvents(), true, false)
}

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	mockControl := mock_control.NewMockControl(ctrl)
	cgroupResource := cgroup.NewCgroupResource("", mockControl, nil, "cgroupRoot", "", specs.LinuxResources{})
	task := &apitask.Task{
		Arn: testTaskARN,
		ENIs: []*apieni.ENI{
			{
				MacAddress: mac,
			},
		},
	}
	task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	task.AddResource("cgroup", cgroupResource)
	require.NoError(t, dataClient.SaveTask(task))

	cfg := defaultConfig
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyEnabled
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)

	taskEngine := &DockerTaskEngine{
		state:      mockState,
		dataClient: dataClient,
		cfg:        &cfg,
	}

	attachment := &apieni.ENIAttachment{
		TaskARN:          "TaskARN",
		AttachmentARN:    testAttachmentArn,
		MACAddress:       "MACAddress",
		Status:           apieni.ENIAttachmentNone,
		AttachStatusSent: true,
	}

	gomock.InOrder(
		mockControl.EXPECT().Remove("cgroupRoot").Return(nil),
		mockState.EXPECT().RemoveTask(task),
		mockState.EXPECT().ENIByMac(gomock.Any()).Return(attachment, true),
		mockState.EXPECT().RemoveENIAttachment(mac),
	)

	assert.NoError(t, taskEngine.dataClient.SaveENIAttachment(attachment))
	attachments, err := taskEngine.dataClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, attachments, 1)

	taskEngine.deleteTask(task)
	tasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
	attachments, err = taskEngine.dataClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, attachments, 0)
}

func TestDeleteTaskBranchENIEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockControl := mock_control.NewMockControl(ctrl)
	cgroupResource := cgroup.NewCgroupResource("", mockControl, nil, "cgroupRoot", "", specs.LinuxResources{})
	task := &apitask.Task{
		Arn: testTaskARN,
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
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyEnabled
	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)

	taskEngine := &DockerTaskEngine{
		state:      mockState,
		cfg:        &cfg,
		dataClient: data.NewNoopClient(),
	}

	gomock.InOrder(
		mockControl.EXPECT().Remove("cgroupRoot").Return(nil),
		mockState.EXPECT().RemoveTask(task),
	)

	taskEngine.deleteTask(task)
}

// TestResourceContainerProgressionFailure ensures that task moves to STOPPED when
// resource creation fails
func TestResourceContainerProgressionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()
	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer := sleepTask.Containers[0]

	sleepContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	sleepContainer.BuildResourceDependency("cgroup", resourcestatus.ResourceCreated, apicontainerstatus.ContainerPulled)

	mockControl := mock_control.NewMockControl(ctrl)
	taskID := sleepTask.GetID()
	cgroupRoot := fmt.Sprintf("/ecs/%s", taskID)
	cgroupResource := cgroup.NewCgroupResource(sleepTask.Arn, mockControl, nil, cgroupRoot, cgroupMountPath, specs.LinuxResources{})

	sleepTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	sleepTask.AddResource("cgroup", cgroupResource)
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	gomock.InOrder(
		// resource creation failure
		mockControl.EXPECT().Exists(gomock.Any()).Return(false),
		mockControl.EXPECT().Create(gomock.Any()).Return(errors.New("cgroup create error")),
	)
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	taskEngine.AddTask(sleepTask)
	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()
	waitForStopEvents(t, taskEngine.StateChangeEvents(), true, false)
}

func TestTaskCPULimitHappyPath(t *testing.T) {
	testcases := []struct {
		name                string
		metadataCreateError error
		metadataUpdateError error
		metadataCleanError  error
		taskCPULimit        config.BooleanDefaultTrue
	}{
		{
			name:                "Task CPU Limit Succeeds",
			metadataCreateError: nil,
			metadataUpdateError: nil,
			metadataCleanError:  nil,
			taskCPULimit:        config.BooleanDefaultTrue{Value: config.ExplicitlyEnabled},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			metadataConfig := defaultConfig
			metadataConfig.TaskCPUMemLimit = tc.taskCPULimit
			metadataConfig.ContainerMetadataEnabled = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, metadataManager, _ := mocks(
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
			taskID := sleepTask.GetID()
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
				mockControl.EXPECT().Create(gomock.Any()).Return(nil)
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
			waitForStopEvents(t, taskEngine.StateChangeEvents(), true, false)
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
					assert.Equal(t, containerID, removedContainerName,
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
			ctrl, client, mockTime, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
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

func TestBuildCNIConfigFromTaskContainer(t *testing.T) {
	config := defaultConfig
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, taskEngine, _, _, _, _ := mocks(t, ctx, &config)
	defer ctrl.Finish()

	testTask := testdata.LoadTask("sleep5")
	testTask.AddTaskENI(mockENI)
	testTask.SetAppMesh(&appmesh.AppMesh{
		IgnoredUID:       ignoredUID,
		ProxyIngressPort: proxyIngressPort,
		ProxyEgressPort:  proxyEgressPort,
		AppPorts: []string{
			appPort,
		},
		EgressIgnoredIPs: []string{
			egressIgnoredIP,
		},
	})
	containerInspectOutput := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    containerID,
			State: &types.ContainerState{Pid: containerPid},
			HostConfig: &dockercontainer.HostConfig{
				NetworkMode: containerNetNS,
			},
		},
	}

	cniConfig, err := taskEngine.(*DockerTaskEngine).buildCNIConfigFromTaskContainer(testTask, containerInspectOutput, true)
	assert.NoError(t, err)
	assert.Equal(t, containerID, cniConfig.ContainerID)
	assert.Equal(t, strconv.Itoa(containerPid), cniConfig.ContainerPID)
	assert.Equal(t, mac, cniConfig.ID, "ID should be set to the mac of eni")
	// We expect 3 NetworkConfig objects in the cni Config wrapper object:
	// ENI, Bridge and Appmesh
	require.Len(t, cniConfig.NetworkConfigs, 3)
}

// TestTaskWithSteadyStateResourcesProvisioned tests container and task transitions
// when the steady state for the pause container is set to RESOURCES_PROVISIONED and
// the steady state for the normal container is set to RUNNING
func TestTaskWithSteadyStateResourcesProvisioned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	// sleep5 contains a single 'sleep' container, with DesiredStatus == RUNNING
	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer := sleepTask.Containers[0]
	sleepContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	sleepContainer.BuildContainerDependency("pause", apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerPulled)
	// Add a second container with DesiredStatus == RESOURCES_PROVISIONED and
	// steadyState == RESOURCES_PROVISIONED
	pauseContainer := apicontainer.NewContainerWithSteadyState(apicontainerstatus.ContainerResourcesProvisioned)
	pauseContainer.Name = "pause"
	pauseContainer.Image = "pause"
	pauseContainer.CPU = 10
	pauseContainer.Memory = 10
	pauseContainer.Essential = true
	pauseContainer.Type = apicontainer.ContainerCNIPause
	sleepTask.Containers = append(sleepTask.Containers, pauseContainer)
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	// containerEventsWG is used to force the test to wait until the container created and started
	// events are processed
	containerEventsWG := sync.WaitGroup{}

	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	// We cannot rely on the order of pulls between images as they can still be downloaded in
	// parallel. The dependency graph enforcement comes into effect for CREATED transitions.
	// Hence, do not enforce the order of invocation of these calls
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	client.EXPECT().PullImage(gomock.Any(), sleepContainer.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{})
	imageManager.EXPECT().RecordContainerReference(sleepContainer).Return(nil)
	imageManager.EXPECT().GetImageStateFromImageName(sleepContainer.Image).Return(nil, false)

	gomock.InOrder(
		// Ensure that the pause container is created first
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx interface{}, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig, containerName string, z time.Duration) {
				sleepTask.AddTaskENI(mockENI)
				sleepTask.SetAppMesh(&appmesh.AppMesh{
					IgnoredUID:       ignoredUID,
					ProxyIngressPort: proxyIngressPort,
					ProxyEgressPort:  proxyEgressPort,
					AppPorts: []string{
						appPort,
					},
					EgressIgnoredIPs: []string{
						egressIgnoredIP,
					},
				})
				assert.Equal(t, "none", string(hostConfig.NetworkMode))
				assert.True(t, strings.Contains(containerName, pauseContainer.Name))
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(apicontainerstatus.ContainerCreated)
					containerEventsWG.Done()
				}()
			}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID + ":" + pauseContainer.Name}),
		// Ensure that the pause container is started after it's created
		client.EXPECT().StartContainer(gomock.Any(), containerID+":"+pauseContainer.Name, defaultConfig.ContainerStartTimeout).Do(
			func(ctx interface{}, id string, timeout time.Duration) {
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(apicontainerstatus.ContainerRunning)
					containerEventsWG.Done()
				}()
			}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID + ":" + pauseContainer.Name}),
		client.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    containerID,
				State: &types.ContainerState{Pid: 23},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetNS,
				},
			},
		}, nil),
		// Then setting up the pause container network namespace
		mockCNIClient.EXPECT().SetupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nsResult, nil),

		// Once the pause container is started, sleep container will be created
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx interface{}, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig, containerName string, z time.Duration) {
				assert.True(t, strings.Contains(containerName, sleepContainer.Name))
				assert.Equal(t, "container:"+containerID+":"+pauseContainer.Name, string(hostConfig.NetworkMode))
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
	taskARNByIP, ok := taskEngine.(*DockerTaskEngine).state.GetTaskByIPAddress(taskIP)
	assert.True(t, ok)
	assert.Equal(t, sleepTask.Arn, taskARNByIP)
	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()
	client.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    containerID,
				State: &types.ContainerState{Pid: 23},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetNS,
				},
			},
		}, nil)
	mockCNIClient.EXPECT().CleanupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	client.EXPECT().StopContainer(gomock.Any(), containerID+":"+pauseContainer.Name, gomock.Any()).MinTimes(1)
	mockCNIClient.EXPECT().ReleaseIPResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).MaxTimes(1)

	// Simulate a container stop event from docker
	eventStream <- dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerStopped,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: containerID + ":" + sleepContainer.Name,
			ExitCode: aws.Int(exitCode),
		},
	}
	waitForStopEvents(t, taskEngine.StateChangeEvents(), true, false)
}

func TestPauseContainerHappyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, mockTime, taskEngine, _, imageManager, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = cniClient
	taskEngine.(*DockerTaskEngine).taskSteadyStatePollInterval = taskSteadyStatePollInterval
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	sleepTask := testdata.LoadTask("sleep5TwoContainers")
	sleepContainer1 := sleepTask.Containers[0]
	sleepContainer1.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	sleepContainer2 := sleepTask.Containers[1]
	sleepContainer2.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)

	// Add eni information to the task so the task can add dependency of pause container
	sleepTask.AddTaskENI(mockENI)

	sleepTask.SetAppMesh(&appmesh.AppMesh{
		IgnoredUID:       ignoredUID,
		ProxyIngressPort: proxyIngressPort,
		ProxyEgressPort:  proxyEgressPort,
		AppPorts: []string{
			appPort,
		},
		EgressIgnoredIPs: []string{
			egressIgnoredIP,
		},
	})

	dockerClient.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)

	sleepContainerID1 := containerID + "1"
	sleepContainerID2 := containerID + "2"
	pauseContainerID := "pauseContainerID"
	// Pause container will be launched first
	gomock.InOrder(
		dockerClient.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		dockerClient.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx interface{}, config *dockercontainer.Config, x, y, z interface{}) {
				name, ok := config.Labels[labelPrefix+"container-name"]
				assert.True(t, ok)
				assert.Equal(t, apitask.NetworkPauseContainerName, name)
			}).Return(dockerapi.DockerContainerMetadata{DockerID: "pauseContainerID"}),
		dockerClient.EXPECT().StartContainer(gomock.Any(), pauseContainerID, defaultConfig.ContainerStartTimeout).Return(
			dockerapi.DockerContainerMetadata{DockerID: "pauseContainerID"}),
		dockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			&types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:    pauseContainerID,
					State: &types.ContainerState{Pid: containerPid},
					HostConfig: &dockercontainer.HostConfig{
						NetworkMode: containerNetNS,
					},
				},
			}, nil),
		cniClient.EXPECT().SetupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nsResult, nil),
	)

	// For the other container
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	dockerClient.EXPECT().PullImage(gomock.Any(), gomock.Any(), nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}).Times(2)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Return(nil).Times(2)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false).Times(2)
	dockerClient.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).Times(2)

	dockerClient.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{DockerID: sleepContainerID1})
	dockerClient.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{DockerID: sleepContainerID2})

	dockerClient.EXPECT().StartContainer(gomock.Any(), sleepContainerID1, defaultConfig.ContainerStartTimeout).Return(
		dockerapi.DockerContainerMetadata{DockerID: sleepContainerID1})
	dockerClient.EXPECT().StartContainer(gomock.Any(), sleepContainerID2, defaultConfig.ContainerStartTimeout).Return(
		dockerapi.DockerContainerMetadata{DockerID: sleepContainerID2})

	cleanup := make(chan time.Time)
	defer close(cleanup)
	mockTime.EXPECT().Now().Return(time.Now()).MinTimes(1)
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), sleepContainerID1).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), sleepContainerID2).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), pauseContainerID).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	taskEngine.AddTask(sleepTask)
	stateChangeEvents := taskEngine.StateChangeEvents()
	verifyTaskIsRunning(stateChangeEvents, sleepTask)

	var wg sync.WaitGroup
	wg.Add(1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
	gomock.InOrder(
		dockerClient.EXPECT().StopContainer(gomock.Any(), sleepContainerID2, gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{DockerID: sleepContainerID2}),

		dockerClient.EXPECT().InspectContainer(gomock.Any(), pauseContainerID, gomock.Any()).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    pauseContainerID,
				State: &types.ContainerState{Pid: containerPid},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetNS,
				},
			},
		}, nil),
		cniClient.EXPECT().CleanupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),

		dockerClient.EXPECT().StopContainer(gomock.Any(), pauseContainerID, gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{DockerID: pauseContainerID}),

		cniClient.EXPECT().ReleaseIPResource(gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx context.Context, cfg *ecscni.Config, timeout time.Duration) {
				wg.Done()
			}).Return(nil),
	)

	dockerClient.EXPECT().RemoveContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(3)
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any()).Return(nil).Times(2)

	// Simulate a container stop event from docker
	eventStream <- dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerStopped,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: sleepContainerID1,
			ExitCode: aws.Int(exitCode),
		},
	}

	verifyTaskIsStopped(stateChangeEvents, sleepTask)
	sleepTask.SetSentStatus(apitaskstatus.TaskStopped)
	cleanup <- time.Now()
	for {
		tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
		if len(tasks) == 0 {
			break
		}
		t.Logf("Found %d tasks in the engine; first task arn: %s", len(tasks), tasks[0].Arn)
		fmt.Printf("Found %d tasks in the engine; first task arn: %s\n", len(tasks), tasks[0].Arn)
		time.Sleep(5 * time.Millisecond)
	}
	wg.Wait()
}

func TestContainersWithServiceConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, mockTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	appnetClient := mock_api.NewMockAppnetClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = cniClient
	taskEngine.(*DockerTaskEngine).appnetClient = appnetClient
	taskEngine.(*DockerTaskEngine).taskSteadyStatePollInterval = taskSteadyStatePollInterval
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	sleepTask := testdata.LoadTask("sleep5TwoContainers")
	sleepTask.NetworkMode = apitask.AWSVPCNetworkMode
	sleepContainer1 := sleepTask.Containers[0]
	sleepContainer1.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	sleepContainer2 := sleepTask.Containers[1]
	sleepContainer2.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)

	// Inject mock SC config
	sleepTask.ServiceConnectConfig = &serviceconnect.Config{
		ContainerName: "service-connect",
		DNSConfig: []serviceconnect.DNSConfigEntry{
			{
				HostName: "host1.my.corp",
				Address:  "169.254.1.1",
			},
			{
				HostName: "host1.my.corp",
				Address:  "ff06::c4",
			},
		},
	}
	dockerConfig := dockercontainer.Config{
		Healthcheck: &dockercontainer.HealthConfig{
			Test:     []string{"echo", "ok"},
			Interval: time.Millisecond,
			Timeout:  time.Second,
			Retries:  1,
		},
	}

	rawConfig, err := json.Marshal(&dockerConfig)
	if err != nil {
		t.Fatal(err)
	}
	sleepTask.Containers = append(sleepTask.Containers, &apicontainer.Container{
		Name:            sleepTask.ServiceConnectConfig.ContainerName,
		HealthCheckType: apicontainer.DockerHealthCheckType,
		DockerConfig: apicontainer.DockerConfig{
			Config: aws.String(string(rawConfig)),
		},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	})

	// Add eni information to the task so the task can add dependency of pause container
	sleepTask.AddTaskENI(mockENI)

	dockerClient.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)

	sleepContainerID1 := containerID + "1"
	sleepContainerID2 := containerID + "2"
	scContainerID := "serviceConnectID"
	pauseContainerID := "pauseContainerID"
	// Pause container will be launched first
	gomock.InOrder(
		dockerClient.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		serviceConnectManager.EXPECT().AugmentTaskContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		dockerClient.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{DockerID: "pauseContainerID"}),
		dockerClient.EXPECT().StartContainer(gomock.Any(), pauseContainerID, defaultConfig.ContainerStartTimeout).Return(
			dockerapi.DockerContainerMetadata{DockerID: "pauseContainerID"}),
		dockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			&types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:    pauseContainerID,
					State: &types.ContainerState{Pid: containerPid},
					HostConfig: &dockercontainer.HostConfig{
						NetworkMode: containerNetNS,
					},
				},
			}, nil),
		cniClient.EXPECT().SetupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nsResult, nil),
	)

	// For the other container
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	dockerClient.EXPECT().PullImage(gomock.Any(), gomock.Any(), nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}).Times(3)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Return(nil).Times(3)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false).Times(3)
	dockerClient.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).Times(3)
	serviceConnectManager.EXPECT().AugmentTaskContainer(gomock.Any(), gomock.Any(), gomock.Any()).Times(3)
	dockerClient.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{DockerID: scContainerID})
	dockerClient.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{DockerID: sleepContainerID1})
	dockerClient.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{DockerID: sleepContainerID2})
	dockerClient.EXPECT().StartContainer(gomock.Any(), scContainerID, defaultConfig.ContainerStartTimeout).Return(
		dockerapi.DockerContainerMetadata{
			DockerID: scContainerID,
			Health:   apicontainer.HealthStatus{Status: apicontainerstatus.ContainerHealthy},
		})
	dockerClient.EXPECT().StartContainer(gomock.Any(), sleepContainerID1, defaultConfig.ContainerStartTimeout).Return(
		dockerapi.DockerContainerMetadata{DockerID: sleepContainerID1})
	dockerClient.EXPECT().StartContainer(gomock.Any(), sleepContainerID2, defaultConfig.ContainerStartTimeout).Return(
		dockerapi.DockerContainerMetadata{DockerID: sleepContainerID2})

	cleanup := make(chan time.Time)
	defer close(cleanup)
	mockTime.EXPECT().Now().Return(time.Now()).MinTimes(1)
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), scContainerID).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), sleepContainerID1).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), sleepContainerID2).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), pauseContainerID).AnyTimes()

	err = taskEngine.Init(ctx)
	assert.NoError(t, err)
	taskEngine.AddTask(sleepTask)
	stateChangeEvents := taskEngine.StateChangeEvents()
	verifyTaskIsRunning(stateChangeEvents, sleepTask)

	var wg sync.WaitGroup
	wg.Add(1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
	gomock.InOrder(
		appnetClient.EXPECT().DrainInboundConnections(gomock.Any()).MaxTimes(1),
		dockerClient.EXPECT().StopContainer(gomock.Any(), sleepContainerID2, gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{DockerID: sleepContainerID2}),
		dockerClient.EXPECT().StopContainer(gomock.Any(), scContainerID, gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{DockerID: scContainerID}),
		dockerClient.EXPECT().InspectContainer(gomock.Any(), pauseContainerID, gomock.Any()).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    pauseContainerID,
				State: &types.ContainerState{Pid: containerPid},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetNS,
				},
			},
		}, nil),
		cniClient.EXPECT().CleanupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),

		dockerClient.EXPECT().StopContainer(gomock.Any(), pauseContainerID, gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{DockerID: pauseContainerID}),

		cniClient.EXPECT().ReleaseIPResource(gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx context.Context, cfg *ecscni.Config, timeout time.Duration) {
				wg.Done()
			}).Return(nil),
	)

	dockerClient.EXPECT().RemoveContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(4)
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any()).Return(nil).Times(3)

	// Simulate a container stop event from docker
	eventStream <- dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerStopped,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: sleepContainerID1,
			ExitCode: aws.Int(exitCode),
		},
	}

	verifyTaskIsStopped(stateChangeEvents, sleepTask)

	sleepTask.SetSentStatus(apitaskstatus.TaskStopped)
	cleanup <- time.Now()
	for {
		tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
		if len(tasks) == 0 {
			break
		}
		t.Logf("Found %d tasks in the engine; first task arn: %s", len(tasks), tasks[0].Arn)
		fmt.Printf("Found %d tasks in the engine; first task arn: %s\n", len(tasks), tasks[0].Arn)
		time.Sleep(5 * time.Millisecond)
	}
	wg.Wait()

}

// TestContainersWithServiceConnect_BridgeMode verifies the start/stop of a bridge mode SC task
func TestContainersWithServiceConnect_BridgeMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, mockTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = cniClient
	taskEngine.(*DockerTaskEngine).taskSteadyStatePollInterval = taskSteadyStatePollInterval
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	sleepTask := testdata.LoadTask("sleep5PortMappings")
	sleepContainer := sleepTask.Containers[0]
	sleepContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)

	// Inject mock SC config
	sleepTask.ServiceConnectConfig = &serviceconnect.Config{
		ContainerName: "service-connect",
		IngressConfig: []serviceconnect.IngressConfigEntry{
			{
				ListenerName: "testListener1", // bridge mode default - ephemeral listener host port
				ListenerPort: 15000,
			},
		},
		EgressConfig: &serviceconnect.EgressConfig{
			ListenerName: "testEgressListener",
			ListenerPort: 0, // Presently this should always get ephemeral port
		},
		DNSConfig: []serviceconnect.DNSConfigEntry{
			{
				HostName: "host1.my.corp",
				Address:  "169.254.1.1",
			},
			{
				HostName: "host1.my.corp",
				Address:  "ff06::c4",
			},
		},
	}

	// if we create a dockercontainer.Config.Healthcheck variable and marshal it, dockercontainer.Config.Env gets set to empty
	// and will later override the internal env vars that Agent populates for the container.
	// In real world, the container env vars in task def are marshaled into container.Environment isntead of docker Config.Env.
	// it gets merged with internal env vars, and eventually get assigned to docker Config.Env
	healthCheckString := "{\"Healthcheck\":{\"Test\":[\"echo\",\"ok\"],\"Interval\":1000000,\"Timeout\":1000000000,\"Retries\":1}}"
	sleepTask.Containers = append(sleepTask.Containers, &apicontainer.Container{
		Name:                      sleepTask.ServiceConnectConfig.ContainerName,
		HealthCheckType:           apicontainer.DockerHealthCheckType,
		DockerConfig:              apicontainer.DockerConfig{Config: aws.String(healthCheckString)},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
		DesiredStatusUnsafe:       apicontainerstatus.ContainerRunning,
	})

	dockerClient.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)

	sleepContainerID := containerID + "1"
	scContainerID := "serviceConnectID"
	sleepPauseContainerID := "sleepPauseContainerID"
	scPauseContainerID := "pauseContainerID"

	// Sleep and SC pause containers can be created and started in parallel, but sleepPause.RESOURCES_PROVISIONED depends on
	// SCPause.RUNNING (verified in the InOrder block down below)

	// For both pause containers
	serviceConnectManager.EXPECT().AugmentTaskContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
	dockerClient.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).Times(2)
	dockerClient.EXPECT().CreateContainer(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx interface{}, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig, dockerContainerName string, z interface{}) dockerapi.DockerContainerMetadata {
			if strings.Contains(dockerContainerName, "internalecspause-service-connect") {
				verifyServiceConnectPauseContainerBridgeMode(t, ctx, config, hostConfig, dockerContainerName, z)
				return dockerapi.DockerContainerMetadata{DockerID: scPauseContainerID}
			} else if strings.Contains(dockerContainerName, "internalecspause-sleep5") {
				verifyServiceConnectSleepPauseContainerBridgeMode(t, ctx, config, hostConfig, dockerContainerName, z)
				return dockerapi.DockerContainerMetadata{DockerID: sleepPauseContainerID}
			}
			assert.FailNow(t, fmt.Sprintf("Unexpected container %s", dockerContainerName))
			return dockerapi.DockerContainerMetadata{}
		}).Times(2)

	// For sleep pause container -> RUNNING
	dockerClient.EXPECT().StartContainer(gomock.Any(), sleepPauseContainerID, defaultConfig.ContainerStartTimeout).Return(
		dockerapi.DockerContainerMetadata{
			DockerID: sleepPauseContainerID,
			NetworkSettings: &types.NetworkSettings{
				DefaultNetworkSettings: types.DefaultNetworkSettings{IPAddress: "1.2.3.4"},
			}},
	)

	// For SC pause container -> RESOURCES_PROVISIONED
	dockerClient.EXPECT().InspectContainer(gomock.Any(), scPauseContainerID, gomock.Any()).Return(
		&types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: scPauseContainerID}}, nil)

	gomock.InOrder(
		// sleepPause.RESOURCES_PROVISIONED depends on SCPause.RUNNING
		dockerClient.EXPECT().StartContainer(gomock.Any(), scPauseContainerID, defaultConfig.ContainerStartTimeout).Return(
			dockerapi.DockerContainerMetadata{
				DockerID: scPauseContainerID,
				NetworkSettings: &types.NetworkSettings{
					DefaultNetworkSettings: types.DefaultNetworkSettings{IPAddress: "1.2.3.5"},
				}},
		),
		dockerClient.EXPECT().InspectContainer(gomock.Any(), sleepPauseContainerID, gomock.Any()).Return(
			&types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: sleepPauseContainerID}}, nil),

		// SC container should only start after pause containers are done
		dockerClient.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{DockerID: scContainerID}),
		dockerClient.EXPECT().StartContainer(gomock.Any(), scContainerID, defaultConfig.ContainerStartTimeout).Return(
			dockerapi.DockerContainerMetadata{
				DockerID: scContainerID,
				Health:   apicontainer.HealthStatus{Status: apicontainerstatus.ContainerHealthy},
			}),

		// sleep container should only start after SC container
		dockerClient.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{DockerID: sleepContainerID}),
		dockerClient.EXPECT().StartContainer(gomock.Any(), sleepContainerID, defaultConfig.ContainerStartTimeout).Return(
			dockerapi.DockerContainerMetadata{DockerID: sleepContainerID}),
	)

	// For SC and sleep container - those calls can happen in parallel
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	dockerClient.EXPECT().PullImage(gomock.Any(), gomock.Any(), nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}).Times(2)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Return(nil).Times(2)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false).Times(2)
	dockerClient.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).Times(2)
	serviceConnectManager.EXPECT().AugmentTaskContainer(gomock.Any(), gomock.Any(), gomock.Any()).Times(2)

	cleanup := make(chan time.Time)
	defer close(cleanup)
	mockTime.EXPECT().Now().Return(time.Now()).MinTimes(1)
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), scContainerID).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), sleepContainerID).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), scPauseContainerID).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(gomock.Any(), sleepPauseContainerID).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	taskEngine.AddTask(sleepTask)
	stateChangeEvents := taskEngine.StateChangeEvents()
	verifyTaskIsRunning(stateChangeEvents, sleepTask)

	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
	// sleep container should stop first, followed by SC container, and finally pause containers
	gomock.InOrder(
		dockerClient.EXPECT().StopContainer(gomock.Any(), sleepContainerID, gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{DockerID: sleepContainerID}),
		dockerClient.EXPECT().StopContainer(gomock.Any(), scContainerID, gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{DockerID: scContainerID}),
	)
	dockerClient.EXPECT().StopContainer(gomock.Any(), scPauseContainerID, gomock.Any()).Return(
		dockerapi.DockerContainerMetadata{DockerID: scPauseContainerID})
	dockerClient.EXPECT().StopContainer(gomock.Any(), sleepPauseContainerID, gomock.Any()).Return(
		dockerapi.DockerContainerMetadata{DockerID: sleepPauseContainerID})

	dockerClient.EXPECT().RemoveContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(4)
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any()).Return(nil).AnyTimes()

	// Set task desired status to STOPPED for triggering container stop sequence
	sleepTask.SetDesiredStatus(apitaskstatus.TaskStopped)

	verifyTaskIsStopped(stateChangeEvents, sleepTask)
	sleepTask.SetSentStatus(apitaskstatus.TaskStopped)
	cleanup <- time.Now()
	for {
		tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
		if len(tasks) == 0 {
			break
		}
		t.Logf("Found %d tasks in the engine; first task arn: %s", len(tasks), tasks[0].Arn)
		fmt.Printf("Found %d tasks in the engine; first task arn: %s\n", len(tasks), tasks[0].Arn)
		time.Sleep(5 * time.Millisecond)
	}
}

func verifyServiceConnectSleepPauseContainerBridgeMode(t *testing.T, ctx interface{}, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig, y, z interface{}) {
	name, ok := config.Labels[labelPrefix+"container-name"]
	assert.True(t, ok)
	assert.Equal(t, fmt.Sprintf("%s-%s", apitask.NetworkPauseContainerName, "sleep5"), name)
	// verify host config network mode
	assert.Equal(t, dockercontainer.NetworkMode(apitask.BridgeNetworkMode), hostConfig.NetworkMode)
	// verify host config port bindings
	assert.NotNil(t, hostConfig.PortBindings)
	assert.Equal(t, 1, len(hostConfig.PortBindings))
	bindings, ok := hostConfig.PortBindings["8080/tcp"]
	assert.True(t, ok)
	assert.Equal(t, 1, len(bindings))
	assert.Equal(t, "0", bindings[0].HostPort)
	// verify container config port exposed
	assert.NotNil(t, config.ExposedPorts)
	assert.Equal(t, 1, len(config.ExposedPorts))
	_, ok = config.ExposedPorts["8080/tcp"]
	assert.True(t, ok)
}

func verifyServiceConnectPauseContainerBridgeMode(t *testing.T, ctx interface{}, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig, y, z interface{}) {
	name, ok := config.Labels[labelPrefix+"container-name"]
	assert.True(t, ok)
	assert.Equal(t, fmt.Sprintf("%s-%s", apitask.NetworkPauseContainerName, "service-connect"), name)
	// verify host config network mode
	assert.Equal(t, dockercontainer.NetworkMode(apitask.BridgeNetworkMode), hostConfig.NetworkMode)
	// verify host config port bindings
	assert.NotNil(t, hostConfig.PortBindings)
	assert.Equal(t, 1, len(hostConfig.PortBindings))
	bindings, ok := hostConfig.PortBindings["15000/tcp"]
	assert.True(t, ok)
	assert.Equal(t, 1, len(bindings))
	assert.Equal(t, "0", bindings[0].HostPort)
	// verify container config port exposed
	assert.NotNil(t, config.ExposedPorts)
	assert.Equal(t, 2, len(config.ExposedPorts)) // 2 because egress container port is also exposed
	_, ok = config.ExposedPorts["15000/tcp"]
	assert.True(t, ok)
}
