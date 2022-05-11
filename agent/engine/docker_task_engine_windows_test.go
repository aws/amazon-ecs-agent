//go:build windows && unit
// +build windows,unit

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
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	mock_ecscni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	mock_s3_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_ssm_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	containerNetNS = "container:abcd"
)

func TestDeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	task := &apitask.Task{
		Arn: testTaskARN,
	}
	require.NoError(t, dataClient.SaveTask(task))

	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	taskEngine := &DockerTaskEngine{
		state:      mockState,
		dataClient: dataClient,
		cfg:        &defaultConfig,
		ctx:        ctx,
	}

	gomock.InOrder(
		mockState.EXPECT().RemoveTask(task),
	)

	taskEngine.deleteTask(task)
	tasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestCredentialSpecResourceTaskFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, credentialsManager, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	// metadata required for createContainer workflow validation
	credentialSpecTaskARN := "credentialSpecTask"
	credentialSpecTaskFamily := "credentialSpecFamily"
	credentialSpecTaskVersion := "1"
	credentialSpecTaskContainerName := "credentialSpecContainer"

	c := &apicontainer.Container{
		Name: credentialSpecTaskContainerName,
	}
	credentialspecFile := "credentialspec:file://gmsa_gmsa-acct.json"
	targetCredentialspecFile := "credentialspec=file://gmsa_gmsa-acct.json"
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	c.DockerConfig.HostConfig = &hostConfig

	// sample test
	testTask := &apitask.Task{
		Arn:        credentialSpecTaskARN,
		Family:     credentialSpecTaskFamily,
		Version:    credentialSpecTaskVersion,
		Containers: []*apicontainer.Container{c},
	}

	// metadata required for execution role authentication workflow
	credentialsID := "execution role"

	// configure the task and container to use execution role
	testTask.SetExecutionRoleCredentialsID(credentialsID)

	// validate base config
	expectedConfig, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Fatal(err)
	}

	expectedConfig.Labels = map[string]string{
		"com.amazonaws.ecs.task-arn":                credentialSpecTaskARN,
		"com.amazonaws.ecs.container-name":          credentialSpecTaskContainerName,
		"com.amazonaws.ecs.task-definition-family":  credentialSpecTaskFamily,
		"com.amazonaws.ecs.task-definition-version": credentialSpecTaskVersion,
		"com.amazonaws.ecs.cluster":                 "",
	}

	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)

	credentialSpecReq := []string{credentialspecFile}

	credentialSpecRes, cerr := credentialspec.NewCredentialSpecResource(
		testTask.Arn,
		defaultConfig.AWSRegion,
		credentialSpecReq,
		credentialsID,
		credentialsManager,
		ssmClientCreator,
		s3ClientCreator)
	assert.NoError(t, cerr)

	credSpecdata := map[string]string{
		credentialspecFile: targetCredentialspecFile,
	}
	credentialSpecRes.CredSpecMap = credSpecdata

	testTask.ResourcesMapUnsafe = map[string][]taskresource.TaskResource{
		credentialspec.ResourceName: {credentialSpecRes},
	}

	mockTime.EXPECT().Now().AnyTimes()
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()

	client.EXPECT().CreateContainer(gomock.Any(), expectedConfig, gomock.Any(), gomock.Any(), gomock.Any())

	ret := taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
	assert.Nil(t, ret.Error)
}

func TestCredentialSpecResourceTaskFileErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, credentialsManager, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	// metadata required for createContainer workflow validation
	credentialSpecTaskARN := "credentialSpecTask"
	credentialSpecTaskFamily := "credentialSpecFamily"
	credentialSpecTaskVersion := "1"
	credentialSpecTaskContainerName := "credentialSpecContainer"

	c := &apicontainer.Container{
		Name: credentialSpecTaskContainerName,
	}
	credentialspecFile := "credentialspec:file://gmsa_gmsa-acct.json"
	targetCredentialspecFile := "credentialspec=file://gmsa_gmsa-acct.json"
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	c.DockerConfig.HostConfig = &hostConfig

	// sample test
	testTask := &apitask.Task{
		Arn:        credentialSpecTaskARN,
		Family:     credentialSpecTaskFamily,
		Version:    credentialSpecTaskVersion,
		Containers: []*apicontainer.Container{c},
	}

	// metadata required for execution role authentication workflow
	credentialsID := "execution role"

	// configure the task and container to use execution role
	testTask.SetExecutionRoleCredentialsID(credentialsID)

	// validate base config
	expectedConfig, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Fatal(err)
	}

	expectedConfig.Labels = map[string]string{
		"com.amazonaws.ecs.task-arn":                credentialSpecTaskARN,
		"com.amazonaws.ecs.container-name":          credentialSpecTaskContainerName,
		"com.amazonaws.ecs.task-definition-family":  credentialSpecTaskFamily,
		"com.amazonaws.ecs.task-definition-version": credentialSpecTaskVersion,
		"com.amazonaws.ecs.cluster":                 "",
	}

	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)

	credentialSpecReq := []string{credentialspecFile}

	credentialSpecRes, cerr := credentialspec.NewCredentialSpecResource(
		testTask.Arn,
		defaultConfig.AWSRegion,
		credentialSpecReq,
		credentialsID,
		credentialsManager,
		ssmClientCreator,
		s3ClientCreator)
	assert.NoError(t, cerr)

	credSpecdata := map[string]string{
		credentialspecFile: targetCredentialspecFile,
	}
	credentialSpecRes.CredSpecMap = credSpecdata

	mockTime.EXPECT().Now().AnyTimes()
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()

	ret := taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
	assert.Error(t, ret.Error)
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
	assert.Equal(t, containerNetNS, cniConfig.ContainerNetNS)
	assert.Equal(t, mac, cniConfig.ID, "ID should be set to the mac of eni")
	// We expect 2 NetworkConfig objects in the cni Config wrapper object:
	// Config for task ns setup.
	// Config for ecs-bridge setup for the task.
	require.Len(t, cniConfig.NetworkConfigs, 2)
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

		// Then execute commands inside the pause namespace
		client.EXPECT().CreateContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.IDResponse{ID: containerID}, nil),
		client.EXPECT().StartContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		client.EXPECT().InspectContainerExec(gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.ContainerExecInspect{
			ExitCode: 0,
			Running:  false,
		}, nil),

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
		// CNI plugins need to be invoked for sleep container. Therefore we will inspect the started sleep container
		client.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    containerID,
				State: &types.ContainerState{Pid: 25},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetNS,
				},
			},
		}, nil),
		// Invoke the CNI plugins for the sleep container
		mockCNIClient.EXPECT().SetupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nsResult, nil),
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
		dockerClient.EXPECT().CreateContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.IDResponse{ID: containerID}, nil),
		dockerClient.EXPECT().StartContainerExec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		dockerClient.EXPECT().InspectContainerExec(gomock.Any(), gomock.Any(), gomock.Any()).Return(&types.ContainerExecInspect{
			ExitCode: 0,
			Running:  false,
		}, nil),
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
	dockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    sleepContainerID1,
				State: &types.ContainerState{Pid: containerPid},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetNS,
				},
			},
		}, nil)
	cniClient.EXPECT().SetupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nsResult, nil)
	dockerClient.EXPECT().StartContainer(gomock.Any(), sleepContainerID2, defaultConfig.ContainerStartTimeout).Return(
		dockerapi.DockerContainerMetadata{DockerID: sleepContainerID2})
	dockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    sleepContainerID2,
				State: &types.ContainerState{Pid: containerPid},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetNS,
				},
			},
		}, nil)
	cniClient.EXPECT().SetupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nsResult, nil)

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
