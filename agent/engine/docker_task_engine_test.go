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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/asm"
	mock_asm_factory "github.com/aws/amazon-ecs-agent/agent/asm/factory/mocks"
	mock_secretsmanageriface "github.com/aws/amazon-ecs-agent/agent/asm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_containermetadata "github.com/aws/amazon-ecs-agent/agent/containermetadata/mocks"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_ecscni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	mock_execcmdagent "github.com/aws/amazon-ecs-agent/agent/engine/execcmd/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	mock_engineserviceconnect "github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect/mock"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	mock_ssm_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
	mock_ssmiface "github.com/aws/amazon-ecs-agent/agent/ssm/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmauth"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmsecret"
	mock_taskresource "github.com/aws/amazon-ecs-agent/agent/taskresource/mocks"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/ssmsecret"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	mock_ttime "github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/golang/mock/gomock"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	credentialsID               = "credsid"
	ipv4                        = "10.0.0.1"
	gatewayIPv4                 = "10.0.0.2/20"
	mac                         = "1.2.3.4"
	ipv6                        = "f0:234:23"
	dockerContainerName         = "docker-container-name"
	containerPid                = 123
	containerPid2               = 456
	taskIP                      = "169.254.170.3"
	exitCode                    = 1
	labelsTaskARN               = "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe"
	taskSteadyStatePollInterval = time.Millisecond
	secretID                    = "meaning-of-life"
	region                      = "us-west-2"
	username                    = "irene"
	password                    = "sher"
	ignoredUID                  = "1337"
	proxyIngressPort            = "15000"
	proxyEgressPort             = "15001"
	appPort                     = "9000"
	egressIgnoredIP             = "169.254.169.254"
	expectedDelaySeconds        = 10
	expectedDelay               = expectedDelaySeconds * time.Second
	networkBridgeIP             = "bridgeIP"
	networkModeBridge           = "bridge"
	networkModeAWSVPC           = "awsvpc"
	testTaskARN                 = "arn:aws:ecs:region:account-id:task/task-id"
	containerNetworkMode        = "none"
	serviceConnectContainerName = "service-connect"
)

var (
	defaultConfig config.Config
	nsResult      = mockSetupNSResult()

	mockENI = &apieni.ENI{
		ID: "eni-id",
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
		SubnetGatewayIPV4Address: gatewayIPv4,
	}

	// createdContainerName is used to save the name of the created
	// container from the validateContainerRunWorkflow method. This
	// variable should never be accessed directly.
	// The `getCreatedContainerName` and `setCreatedContainerName`
	// methods should be used instead.
	createdContainerName string
	// createdContainerNameLock guards access to the createdContainerName
	// var.
	createdContainerNameLock sync.Mutex
)

func init() {
	defaultConfig = config.DefaultConfig()
	defaultConfig.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
}

func getCreatedContainerName() string {
	createdContainerNameLock.Lock()
	defer createdContainerNameLock.Unlock()

	return createdContainerName
}

func setCreatedContainerName(name string) {
	createdContainerNameLock.Lock()
	defer createdContainerNameLock.Unlock()

	createdContainerName = name
}

func mocks(t *testing.T, ctx context.Context, cfg *config.Config) (*gomock.Controller,
	*mock_dockerapi.MockDockerClient, *mock_ttime.MockTime, TaskEngine,
	*mock_credentials.MockManager, *mock_engine.MockImageManager, *mock_containermetadata.MockManager,
	*mock_engineserviceconnect.MockManager) {
	ctrl := gomock.NewController(t)
	client := mock_dockerapi.NewMockDockerClient(ctrl)
	mockTime := mock_ttime.NewMockTime(ctrl)
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	containerChangeEventStream := eventstream.NewEventStream("TESTTASKENGINE", ctx)
	containerChangeEventStream.StartListening()
	imageManager := mock_engine.NewMockImageManager(ctrl)
	metadataManager := mock_containermetadata.NewMockManager(ctrl)
	execCmdMgr := mock_execcmdagent.NewMockManager(ctrl)

	taskEngine := NewTaskEngine(cfg, client, credentialsManager, containerChangeEventStream,
		imageManager, dockerstate.NewTaskEngineState(), metadataManager, nil, execCmdMgr, nil)
	taskEngine.(*DockerTaskEngine)._time = mockTime
	taskEngine.(*DockerTaskEngine).ctx = ctx
	taskEngine.(*DockerTaskEngine).stopContainerBackoffMin = time.Millisecond
	taskEngine.(*DockerTaskEngine).stopContainerBackoffMax = time.Millisecond * 2
	serviceConnectManager := mock_engineserviceconnect.NewMockManager(ctrl)
	taskEngine.(*DockerTaskEngine).serviceconnectManager = serviceConnectManager
	return ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, metadataManager, serviceConnectManager
}

func mockSetupNSResult() *current.Result {
	_, ip, _ := net.ParseCIDR(taskIP + "/32")
	return &current.Result{
		IPs: []*current.IPConfig{
			{
				Address: *ip,
			},
		},
	}
}

func TestBatchContainerHappyPath(t *testing.T) {
	testcases := []struct {
		name                    string
		metadataCreateError     error
		metadataUpdateError     error
		metadataCleanError      error
		taskCPULimit            config.Conditional
		execCommandAgentEnabled bool
	}{
		{
			name:                "Metadata Manager Succeeds",
			metadataCreateError: nil,
			metadataUpdateError: nil,
			metadataCleanError:  nil,
			taskCPULimit:        config.ExplicitlyDisabled,
		},
		{
			name:                    "ExecCommandAgent is started",
			metadataCreateError:     nil,
			metadataUpdateError:     nil,
			metadataCleanError:      nil,
			taskCPULimit:            config.ExplicitlyDisabled,
			execCommandAgentEnabled: true,
		},
		{
			name:                "Metadata Manager Fails to Create, Update and Cleanup",
			metadataCreateError: errors.New("create metadata error"),
			metadataUpdateError: errors.New("update metadata error"),
			metadataCleanError:  errors.New("clean metadata error"),
			taskCPULimit:        config.ExplicitlyDisabled,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			metadataConfig := defaultConfig
			metadataConfig.TaskCPUMemLimit.Value = tc.taskCPULimit
			metadataConfig.ContainerMetadataEnabled = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, metadataManager, serviceConnectManager := mocks(
				t, ctx, &metadataConfig)
			execCmdMgr := mock_execcmdagent.NewMockManager(ctrl)
			taskEngine.(*DockerTaskEngine).execCmdMgr = execCmdMgr
			defer ctrl.Finish()

			roleCredentials := credentials.TaskIAMRoleCredentials{
				IAMRoleCredentials: credentials.IAMRoleCredentials{CredentialsID: "credsid"},
			}
			credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(roleCredentials, true).AnyTimes()
			credentialsManager.EXPECT().RemoveCredentials(credentialsID)

			sleepTask := testdata.LoadTask("sleep5")
			if tc.execCommandAgentEnabled && len(sleepTask.Containers) > 0 {
				enableExecCommandAgentForContainer(sleepTask.Containers[0], apicontainer.ManagedAgentState{})
			}
			sleepTask.SetCredentialsID(credentialsID)
			eventStream := make(chan dockerapi.DockerContainerChangeEvent)
			// containerEventsWG is used to force the test to wait until the container created and started
			// events are processed
			containerEventsWG := sync.WaitGroup{}

			client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
			serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
			containerName := make(chan string)
			go func() {
				name := <-containerName
				setCreatedContainerName(name)
			}()

			for _, container := range sleepTask.Containers {
				validateContainerRunWorkflow(t, container, sleepTask, imageManager,
					client, &roleCredentials, &containerEventsWG,
					eventStream, containerName, func() {
						metadataManager.EXPECT().Create(gomock.Any(), gomock.Any(),
							gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.metadataCreateError)
						metadataManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(),
							gomock.Any()).Return(tc.metadataUpdateError)

						if tc.execCommandAgentEnabled {
							execCmdMgr.EXPECT().InitializeContainer(gomock.Any(), container, gomock.Any()).Times(1)
							// TODO: [ecs-exec] validate call control plane to report ExecCommandAgent SUCCESS/FAIL here
							execCmdMgr.EXPECT().StartAgent(gomock.Any(), client, sleepTask, sleepTask.Containers[0], containerID)
						}
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
			waitForStopEvents(t, taskEngine.StateChangeEvents(), true, tc.execCommandAgentEnabled)
			// This ensures that managedTask.waitForStopReported makes progress
			sleepTask.SetSentStatus(apitaskstatus.TaskStopped)
			// Extra events should not block forever; duplicate acs and docker events are possible
			go func() { eventStream <- createDockerEvent(apicontainerstatus.ContainerStopped) }()
			go func() { eventStream <- createDockerEvent(apicontainerstatus.ContainerStopped) }()

			sleepTaskStop := testdata.LoadTask("sleep5")

			sleepTaskStop.SetCredentialsID(credentialsID)
			sleepTaskStop.SetDesiredStatus(apitaskstatus.TaskStopped)
			taskEngine.AddTask(sleepTaskStop)
			// As above, duplicate events should not be a problem
			taskEngine.AddTask(sleepTaskStop)
			taskEngine.AddTask(sleepTaskStop)
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

// TestRemoveEvents tests if the task engine can handle task events while the task is being
// cleaned up. This test ensures that there's no regression in the task engine and ensures
// there's no deadlock as seen in #313
func TestRemoveEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	// containerEventsWG is used to force the test to wait until the container created and started
	// events are processed
	containerEventsWG := sync.WaitGroup{}
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	client.EXPECT().StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	containerName := make(chan string)
	go func() {
		name := <-containerName
		setCreatedContainerName(name)
	}()

	for _, container := range sleepTask.Containers {
		validateContainerRunWorkflow(t, container, sleepTask, imageManager,
			client, nil, &containerEventsWG,
			eventStream, containerName, func() {
			})
	}

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

	waitForStopEvents(t, taskEngine.StateChangeEvents(), true, false)
	sleepTaskStop := testdata.LoadTask("sleep5")
	sleepTaskStop.SetDesiredStatus(apitaskstatus.TaskStopped)
	taskEngine.AddTask(sleepTaskStop)

	client.EXPECT().RemoveContainer(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(ctx interface{}, removedContainerName string, timeout time.Duration) {
			assert.Equal(t, containerID, removedContainerName,
				"Container name mismatch")

			// Emit a couple of events for the task before cleanup finishes. This forces
			// discardEventsUntil to be invoked and should test the code path that
			// caused the deadlock, which was fixed with #320
			eventStream <- createDockerEvent(apicontainerstatus.ContainerStopped)
			eventStream <- createDockerEvent(apicontainerstatus.ContainerStopped)
		}).Return(nil)

	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any())

	// This ensures that managedTask.waitForStopReported makes progress
	sleepTask.SetSentStatus(apitaskstatus.TaskStopped)
	// trigger cleanup
	cleanup <- time.Now()
	// Wait for the task to actually be dead; if we just fallthrough immediately,
	// the remove might not have happened (expectation failure)
	for {
		tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
		if len(tasks) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStartTimeoutThenStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, testTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	testTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	testTime.EXPECT().After(gomock.Any())
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil)
	for _, container := range sleepTask.Containers {
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
		client.EXPECT().PullImage(gomock.Any(), container.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{})

		imageManager.EXPECT().RecordContainerReference(container)
		imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false)
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx interface{}, x, y, z, timeout interface{}) {
				go func() { eventStream <- createDockerEvent(apicontainerstatus.ContainerCreated) }()
			}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID})

		client.EXPECT().StartContainer(gomock.Any(), containerID, defaultConfig.ContainerStartTimeout).Return(dockerapi.DockerContainerMetadata{
			Error: &dockerapi.DockerTimeoutError{},
		})
	}

	// Start timeout triggers a container stop as we force stop containers
	// when startcontainer times out. See #1043 for details
	client.EXPECT().StopContainer(gomock.Any(), containerID, gomock.Any()).Return(dockerapi.DockerContainerMetadata{
		Error: dockerapi.CannotStartContainerError{fmt.Errorf("cannot start container")},
	}).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	stateChangeEvents := taskEngine.StateChangeEvents()
	taskEngine.AddTask(sleepTask)
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false, false)

	// Now surprise surprise, it actually did start!
	eventStream <- createDockerEvent(apicontainerstatus.ContainerRunning)
	// However, if it starts again, we should not see it be killed; no additional expect
	eventStream <- createDockerEvent(apicontainerstatus.ContainerRunning)
	eventStream <- createDockerEvent(apicontainerstatus.ContainerRunning)

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}
}

func TestSteadyStatePoll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, testTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	taskEngine.(*DockerTaskEngine).taskSteadyStatePollInterval = taskSteadyStatePollInterval
	containerEventsWG := sync.WaitGroup{}
	sleepTask := testdata.LoadTask("sleep5")
	sleepTask.Arn = uuid.New()
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)

	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	containerName := make(chan string)
	go func() {
		<-containerName
	}()

	// set up expectations for each container in the task calling create + start
	for _, container := range sleepTask.Containers {
		validateContainerRunWorkflow(t, container, sleepTask, imageManager,
			client, nil, &containerEventsWG,
			eventStream, containerName, func() {
			})
	}
	testTime.EXPECT().Now().Return(time.Now()).MinTimes(1)

	var wg sync.WaitGroup
	wg.Add(1)

	client.EXPECT().DescribeContainer(gomock.Any(), containerID).Return(
		apicontainerstatus.ContainerStopped,
		dockerapi.DockerContainerMetadata{
			DockerID: containerID,
		}).Do(func(ctx interface{}, x interface{}) {
		wg.Done()
	})
	client.EXPECT().DescribeContainer(gomock.Any(), containerID).Return(
		apicontainerstatus.ContainerStopped,
		dockerapi.DockerContainerMetadata{
			DockerID: containerID,
		}).AnyTimes()
	client.EXPECT().StopContainer(gomock.Any(), containerID, 30*time.Second).AnyTimes()

	err := taskEngine.Init(ctx) // start the task engine
	assert.NoError(t, err)
	taskEngine.AddTask(sleepTask) // actually add the task we created
	waitForRunningEvents(t, taskEngine.StateChangeEvents())
	containerMap, ok := taskEngine.(*DockerTaskEngine).State().ContainerMapByArn(sleepTask.Arn)
	assert.True(t, ok)
	dockerContainer, ok := containerMap[sleepTask.Containers[0].Name]
	assert.True(t, ok)

	// Wait for container create and start events to be processed
	containerEventsWG.Wait()
	wg.Wait()

	cleanup := make(chan time.Time)
	defer close(cleanup)
	testTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
	client.EXPECT().RemoveContainer(gomock.Any(), dockerContainer.DockerID, dockerclient.RemoveContainerTimeout).Return(nil)
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any()).Return(nil)

	waitForStopEvents(t, taskEngine.StateChangeEvents(), false, false)
	// trigger cleanup, this ensures all the goroutines were finished
	sleepTask.SetSentStatus(apitaskstatus.TaskStopped)
	cleanup <- time.Now()
	for {
		tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
		if len(tasks) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStopWithPendingStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, testTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()
	testTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	testTime.EXPECT().After(gomock.Any()).AnyTimes()

	sleepTask1 := testdata.LoadTask("sleep5")
	sleepTask1.StartSequenceNumber = 5
	sleepTask2 := testdata.LoadTask("sleep5")
	sleepTask2.Arn = "arn2"
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)

	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	stateChangeEvents := taskEngine.StateChangeEvents()

	defer discardEvents(stateChangeEvents)()

	pullDone := make(chan bool)
	pullInvoked := make(chan bool)
	client.EXPECT().PullImage(gomock.Any(), gomock.Any(), nil, gomock.Any()).Do(func(w, x, y, z interface{}) {
		pullInvoked <- true
		<-pullDone
	}).MaxTimes(2)

	imageManager.EXPECT().RecordContainerReference(gomock.Any()).AnyTimes()
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).AnyTimes()

	taskEngine.AddTask(sleepTask2)
	<-pullInvoked
	stopSleep2 := testdata.LoadTask("sleep5")
	stopSleep2.Arn = "arn2"
	stopSleep2.SetDesiredStatus(apitaskstatus.TaskStopped)
	stopSleep2.StopSequenceNumber = 4
	taskEngine.AddTask(stopSleep2)

	taskEngine.AddTask(sleepTask1)
	stopSleep1 := testdata.LoadTask("sleep5")
	stopSleep1.SetDesiredStatus(apitaskstatus.TaskStopped)
	stopSleep1.StopSequenceNumber = 5
	taskEngine.AddTask(stopSleep1)
	pullDone <- true
	// this means the PullImage is only called once due to the task is stopped before it
	// gets the pull image lock
}

func TestCreateContainerSaveDockerIDAndName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	taskEngine.SetDataClient(dataClient)

	sleepTask := testdata.LoadTask("sleep5")
	sleepTask.Arn = testTaskARN
	sleepContainer, _ := sleepTask.ContainerByName("sleep5")
	sleepContainer.TaskARNUnsafe = testTaskARN

	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
	client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{
		DockerID: testDockerID,
	})
	metadata := taskEngine.createContainer(sleepTask, sleepContainer)
	require.NoError(t, metadata.Error)

	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	require.Len(t, containers, 1)
	assert.Equal(t, testDockerID, containers[0].DockerID)
	assert.Contains(t, containers[0].DockerName, sleepContainer.Name)
}

func TestCreateContainerMetadata(t *testing.T) {
	testcases := []struct {
		name  string
		info  types.Info
		error error
	}{
		{
			name:  "Selinux Security Option",
			info:  types.Info{SecurityOptions: []string{"selinux"}},
			error: nil,
		},
		{
			name:  "Docker Info Error",
			info:  types.Info{},
			error: errors.New("Error getting docker info"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, _, privateTaskEngine, _, _, metadataManager, _ := mocks(t, ctx, &config.Config{})
			defer ctrl.Finish()

			taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
			taskEngine.cfg.ContainerMetadataEnabled = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}

			sleepTask := testdata.LoadTask("sleep5")
			sleepContainer, _ := sleepTask.ContainerByName("sleep5")

			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil)
			client.EXPECT().Info(ctx, dockerclient.InfoTimeout).Return(tc.info, tc.error)
			metadataManager.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), tc.info.SecurityOptions)
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())

			metadata := taskEngine.createContainer(sleepTask, sleepContainer)
			assert.NoError(t, metadata.Error)
		})
	}
}

func TestCreateContainerMergesLabels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	testTask := &apitask.Task{
		Arn:     labelsTaskARN,
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
				DockerConfig: apicontainer.DockerConfig{
					Config: aws.String(`{"Labels":{"key":"value"}}`),
				},
			},
		},
	}
	expectedConfig, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	expectedConfig.Labels = map[string]string{
		"com.amazonaws.ecs.task-arn":                labelsTaskARN,
		"com.amazonaws.ecs.container-name":          "c1",
		"com.amazonaws.ecs.task-definition-family":  "myFamily",
		"com.amazonaws.ecs.task-definition-version": "1",
		"com.amazonaws.ecs.cluster":                 "",
		"key":                                       "value",
	}
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
	client.EXPECT().CreateContainer(gomock.Any(), expectedConfig, gomock.Any(), gomock.Any(), gomock.Any())
	taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
}

// TestCreateContainerAddV3EndpointIDToState tests that in createContainer, when the
// container's v3 endpoint id is set, we will add mappings to engine state
func TestCreateContainerAddV3EndpointIDToState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)

	testContainer := &apicontainer.Container{
		Name:         "c1",
		V3EndpointID: "v3EndpointID",
	}

	testTask := &apitask.Task{
		Arn:     "myTaskArn",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			testContainer,
		},
	}

	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
	// V3EndpointID mappings are only added to state when dockerID is available. So return one here.
	client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerapi.DockerContainerMetadata{
		DockerID: "dockerID",
	})
	taskEngine.createContainer(testTask, testContainer)

	// check that we have added v3 endpoint mappings to state
	state := taskEngine.state

	addedTaskARN, ok := state.TaskARNByV3EndpointID("v3EndpointID")
	assert.True(t, ok)
	assert.Equal(t, testTask.Arn, addedTaskARN)

	addedDockerID, ok := state.DockerIDByV3EndpointID("v3EndpointID")
	assert.True(t, ok)
	assert.Equal(t, "dockerID", addedDockerID)
}

// TestTaskTransitionWhenStopContainerTimesout tests that task transitions to stopped
// only when terminal events are received from docker event stream when
// StopContainer times out
func TestTaskTransitionWhenStopContainerTimesout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	containerStopTimeoutError := dockerapi.DockerContainerMetadata{
		Error: &dockerapi.DockerTimeoutError{
			Transition: "stop",
			Duration:   30 * time.Second,
		},
	}
	for _, container := range sleepTask.Containers {
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
		client.EXPECT().PullImage(gomock.Any(), container.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{})
		imageManager.EXPECT().RecordContainerReference(container)
		imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false)
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil)

		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(ctx interface{}, x, y, z, timeout interface{}) {
				go func() { eventStream <- createDockerEvent(apicontainerstatus.ContainerCreated) }()
			}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID})

		gomock.InOrder(
			client.EXPECT().StartContainer(gomock.Any(), containerID, defaultConfig.ContainerStartTimeout).Do(
				func(ctx interface{}, id string, timeout time.Duration) {
					go func() {
						eventStream <- createDockerEvent(apicontainerstatus.ContainerRunning)
					}()
				}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID}),

			// Validate that timeouts are retried exactly 3 times
			client.EXPECT().StopContainer(gomock.Any(), containerID, gomock.Any()).
				Return(containerStopTimeoutError).
				Times(5),

			client.EXPECT().SystemPing(gomock.Any(), gomock.Any()).Return(dockerapi.PingResponse{}).
				Times(1),
		)
	}

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	go taskEngine.AddTask(sleepTask)
	// wait for task running
	waitForRunningEvents(t, taskEngine.StateChangeEvents())
	// Set the task desired status to be stopped and StopContainer will be called
	updateSleepTask := testdata.LoadTask("sleep5")
	updateSleepTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(updateSleepTask)

	// StopContainer was called again and received stop event from docker event stream
	// Expect it to go to stopped
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false, false)
}

// TestTaskTransitionWhenStopContainerReturnsUnretriableError tests if the task transitions
// to stopped without retrying stopping the container in the task when the initial
// stop container call returns an unretriable error from docker, specifically the
// NoSuchContainer error
func TestTaskTransitionWhenStopContainerReturnsUnretriableError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	containerEventsWG := sync.WaitGroup{}
	for _, container := range sleepTask.Containers {
		gomock.InOrder(
			imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes(),
			client.EXPECT().PullImage(gomock.Any(), container.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}),
			imageManager.EXPECT().RecordContainerReference(container),
			imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false),
			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
			// Simulate successful create container
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(ctx interface{}, x, y, z, timeout interface{}) {
					containerEventsWG.Add(1)
					go func() {
						eventStream <- createDockerEvent(apicontainerstatus.ContainerCreated)
						containerEventsWG.Done()
					}()
				}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID}),
			// Simulate successful start container
			client.EXPECT().StartContainer(gomock.Any(), containerID, defaultConfig.ContainerStartTimeout).Do(
				func(ctx interface{}, id string, timeout time.Duration) {
					containerEventsWG.Add(1)
					go func() {
						eventStream <- createDockerEvent(apicontainerstatus.ContainerRunning)
						containerEventsWG.Done()
					}()
				}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID}),
			// StopContainer errors out. However, since this is a known unretriable error,
			// the task engine should not retry stopping the container and move on.
			// If there's a delay in task engine's processing of the ContainerRunning
			// event, StopContainer will be invoked again as the engine considers it
			// as a stopped container coming back. MinTimes() should guarantee that
			// StopContainer is invoked at least once and in protecting agasint a test
			// failure when there's a delay in task engine processing the ContainerRunning
			// event.
			client.EXPECT().StopContainer(gomock.Any(), containerID, gomock.Any()).Return(dockerapi.DockerContainerMetadata{
				Error: dockerapi.CannotStopContainerError{dockerapi.NoSuchContainerError{}},
			}).MinTimes(1),
			client.EXPECT().SystemPing(gomock.Any(), gomock.Any()).
				Return(dockerapi.PingResponse{}).
				Times(1),
		)
	}

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	go taskEngine.AddTask(sleepTask)
	// wait for task running
	waitForRunningEvents(t, taskEngine.StateChangeEvents())
	containerEventsWG.Wait()
	// Set the task desired status to be stopped and StopContainer will be called
	updateSleepTask := testdata.LoadTask("sleep5")
	updateSleepTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(updateSleepTask)
	// StopContainer was called again and received stop event from docker event stream
	// Expect it to go to stopped
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false, false)
}

// TestTaskTransitionWhenStopContainerReturnsTransientErrorBeforeSucceeding tests if the task
// transitions to stopped only after receiving the container stopped event from docker when
// the initial stop container call fails with an unknown error.
func TestTaskTransitionWhenStopContainerReturnsTransientErrorBeforeSucceeding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	containerStoppingError := dockerapi.DockerContainerMetadata{
		Error: dockerapi.CannotStopContainerError{errors.New("Error stopping container")},
	}
	for _, container := range sleepTask.Containers {
		gomock.InOrder(
			imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes(),
			client.EXPECT().PullImage(gomock.Any(), container.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}),
			imageManager.EXPECT().RecordContainerReference(container),
			imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false),
			// Simulate successful create container
			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
				dockerapi.DockerContainerMetadata{DockerID: containerID}),
			// Simulate successful start container
			client.EXPECT().StartContainer(gomock.Any(), containerID, defaultConfig.ContainerStartTimeout).Return(
				dockerapi.DockerContainerMetadata{DockerID: containerID}),
			// StopContainer errors out a couple of times
			client.EXPECT().StopContainer(gomock.Any(), containerID, gomock.Any()).Return(containerStoppingError).Times(4),
			// Since task is not in steady state, progressContainers causes
			// another invocation of StopContainer. Return the 'succeed' response,
			// which should cause the task engine to stop invoking this again and
			// transition the task to stopped.
			client.EXPECT().StopContainer(gomock.Any(), containerID, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}),
		)
	}

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	go taskEngine.AddTask(sleepTask)
	// wait for task running
	waitForRunningEvents(t, taskEngine.StateChangeEvents())
	// Set the task desired status to be stopped and StopContainer will be called
	updateSleepTask := testdata.LoadTask("sleep5")
	updateSleepTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(updateSleepTask)
	// StopContainer invocation should have caused it to stop eventually.
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false, false)
}

func TestGetTaskByArn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	// Need a mock client as AddTask not only adds a task to the engine, but
	// also causes the engine to progress the task.
	ctrl, client, mockTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).AnyTimes()
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	defer taskEngine.Disable()
	sleepTask := testdata.LoadTask("sleep5")
	sleepTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	sleepTaskArn := sleepTask.Arn
	sleepTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	taskEngine.AddTask(sleepTask)

	_, found := taskEngine.GetTaskByArn(sleepTaskArn)
	assert.True(t, found, "Task %s not found", sleepTaskArn)

	_, found = taskEngine.GetTaskByArn(sleepTaskArn + "arn")
	assert.False(t, found, "Task with invalid arn found in the task engine")
}

func TestProvisionContainerResourcesAwsvpcSetPausePIDInVolumeResources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()
	taskEngine.SetDataClient(dataClient)

	mockNamespaceHelper := mock_ecscni.NewMockNamespaceHelper(ctrl)
	taskEngine.(*DockerTaskEngine).namespaceHelper = mockNamespaceHelper

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	testTask := testdata.LoadTask("sleep5")
	pauseContainer := &apicontainer.Container{
		Name: "pausecontainer",
		Type: apicontainer.ContainerCNIPause,
	}
	testTask.Containers = append(testTask.Containers, pauseContainer)
	testTask.AddTaskENI(mockENI)
	testTask.NetworkMode = apitask.AWSVPCNetworkMode
	volRes := &taskresourcevolume.VolumeResource{}
	testTask.ResourcesMapUnsafe = map[string][]taskresource.TaskResource{
		"dockerVolume": {volRes},
	}
	taskEngine.(*DockerTaskEngine).State().AddTask(testTask)
	taskEngine.(*DockerTaskEngine).State().AddContainer(&apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: dockerContainerName,
		Container:  pauseContainer,
	}, testTask)

	gomock.InOrder(
		dockerClient.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    containerID,
				State: &types.ContainerState{Pid: containerPid},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetworkMode,
				},
			},
		}, nil),
		mockCNIClient.EXPECT().SetupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nsResult, nil),
		mockNamespaceHelper.EXPECT().ConfigureTaskNamespaceRouting(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
	)

	require.Nil(t, taskEngine.(*DockerTaskEngine).provisionContainerResources(testTask, pauseContainer).Error)
	assert.Equal(t, strconv.Itoa(containerPid), volRes.GetPauseContainerPID())
	assert.Equal(t, taskIP, testTask.GetLocalIPAddress())
	savedTasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, savedTasks, 1)
}

func TestProvisionContainerResourcesAwsvpcInspectError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	testTask := testdata.LoadTask("sleep5")
	pauseContainer := &apicontainer.Container{
		Name: "pausecontainer",
		Type: apicontainer.ContainerCNIPause,
	}
	testTask.Containers = append(testTask.Containers, pauseContainer)
	testTask.AddTaskENI(mockENI)
	testTask.NetworkMode = apitask.AWSVPCNetworkMode
	taskEngine.(*DockerTaskEngine).State().AddTask(testTask)
	taskEngine.(*DockerTaskEngine).State().AddContainer(&apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: dockerContainerName,
		Container:  pauseContainer,
	}, testTask)

	dockerClient.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).Return(nil, errors.New("test error"))

	assert.NotNil(t, taskEngine.(*DockerTaskEngine).provisionContainerResources(testTask, pauseContainer).Error)
}

func TestProvisionContainerResourcesAwsvpcMissingCNIResponseError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	testTask := testdata.LoadTask("sleep5")
	pauseContainer := &apicontainer.Container{
		Name: "pausecontainer",
		Type: apicontainer.ContainerCNIPause,
	}
	testTask.Containers = append(testTask.Containers, pauseContainer)
	testTask.AddTaskENI(mockENI)
	testTask.NetworkMode = apitask.AWSVPCNetworkMode
	taskEngine.(*DockerTaskEngine).State().AddTask(testTask)
	taskEngine.(*DockerTaskEngine).State().AddContainer(&apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: dockerContainerName,
		Container:  pauseContainer,
	}, testTask)

	dockerClient.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).Return(&types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    containerID,
			State: &types.ContainerState{Pid: containerPid},
			HostConfig: &dockercontainer.HostConfig{
				NetworkMode: containerNetworkMode,
			},
		},
	}, nil)
	mockCNIClient.EXPECT().SetupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	actualErr := taskEngine.(*DockerTaskEngine).provisionContainerResources(testTask, pauseContainer).Error

	assert.NotNil(t, actualErr)
	assert.True(t, strings.Contains(actualErr.Error(), "empty result from network namespace setup"))
}

// TestStopPauseContainerCleanupCalledAwsvpc tests when stopping the pause container
// its network namespace should be cleaned up first
func TestStopPauseContainerCleanupCalledAwsvpc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	testTask := testdata.LoadTask("sleep5")
	pauseContainer := &apicontainer.Container{
		Name:                "pausecontainer",
		Type:                apicontainer.ContainerCNIPause,
		DesiredStatusUnsafe: apicontainerstatus.ContainerStopped,
	}
	testTask.Containers = append(testTask.Containers, pauseContainer)
	testTask.AddTaskENI(mockENI)
	testTask.NetworkMode = apitask.AWSVPCNetworkMode
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
	taskEngine.(*DockerTaskEngine).State().AddTask(testTask)
	taskEngine.(*DockerTaskEngine).State().AddContainer(&apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: dockerContainerName,
		Container:  pauseContainer,
	}, testTask)

	gomock.InOrder(
		dockerClient.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    containerID,
				State: &types.ContainerState{Pid: containerPid},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetworkMode,
				},
			},
		}, nil),
		mockCNIClient.EXPECT().CleanupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		dockerClient.EXPECT().StopContainer(gomock.Any(),
			containerID,
			defaultConfig.DockerStopTimeout,
		).Return(dockerapi.DockerContainerMetadata{}),
	)

	taskEngine.(*DockerTaskEngine).stopContainer(testTask, pauseContainer)
	require.True(t, pauseContainer.IsContainerTornDown())
}

// TestStopPauseContainerCleanupDelayAwsvpc tests when stopping the pause container
// its network namespace should be cleaned up first
func TestStopPauseContainerCleanupDelayAwsvpc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	cfg := config.DefaultConfig()
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.ENIPauseContainerCleanupDelaySeconds = expectedDelaySeconds

	delayedChan := make(chan time.Duration, 1)
	ctrl, dockerClient, _, taskEngine, _, _, _, _ := mocks(t, ctx, &cfg)
	taskEngine.(*DockerTaskEngine).handleDelay = func(d time.Duration) {
		delayedChan <- d
	}

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	testTask := testdata.LoadTask("sleep5")
	pauseContainer := &apicontainer.Container{
		Name:                "pausecontainer",
		Type:                apicontainer.ContainerCNIPause,
		DesiredStatusUnsafe: apicontainerstatus.ContainerStopped,
	}
	testTask.Containers = append(testTask.Containers, pauseContainer)
	testTask.AddTaskENI(mockENI)
	testTask.NetworkMode = apitask.AWSVPCNetworkMode
	taskEngine.(*DockerTaskEngine).State().AddTask(testTask)
	taskEngine.(*DockerTaskEngine).State().AddContainer(&apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: dockerContainerName,
		Container:  pauseContainer,
	}, testTask)

	gomock.InOrder(
		dockerClient.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    containerID,
				State: &types.ContainerState{Pid: containerPid},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetworkMode,
				},
			},
		}, nil),
		mockCNIClient.EXPECT().CleanupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
		dockerClient.EXPECT().StopContainer(gomock.Any(),
			containerID,
			defaultConfig.DockerStopTimeout,
		).Return(dockerapi.DockerContainerMetadata{}),
	)

	taskEngine.(*DockerTaskEngine).stopContainer(testTask, pauseContainer)

	select {
	case actualDelay := <-delayedChan:
		assert.Equal(t, expectedDelay, actualDelay)
		require.True(t, pauseContainer.IsContainerTornDown())
	default:
		assert.Fail(t, "engine.handleDelay wasn't called")
	}
}

// TestCheckTearDownPauseContainerAwsvpc that the pause container teardown works and is idempotent
func TestCheckTearDownPauseContainerAwsvpc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	testTask := testdata.LoadTask("sleep5")
	pauseContainer := &apicontainer.Container{
		Name:                "pausecontainer",
		Type:                apicontainer.ContainerCNIPause,
		DesiredStatusUnsafe: apicontainerstatus.ContainerStopped,
	}
	testTask.Containers = append(testTask.Containers, pauseContainer)
	testTask.AddTaskENI(mockENI)
	testTask.NetworkMode = apitask.AWSVPCNetworkMode
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
	taskEngine.(*DockerTaskEngine).State().AddTask(testTask)
	taskEngine.(*DockerTaskEngine).State().AddContainer(&apicontainer.DockerContainer{
		DockerID:   containerID,
		DockerName: dockerContainerName,
		Container:  pauseContainer,
	}, testTask)

	gomock.InOrder(
		dockerClient.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).Return(&types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    containerID,
				State: &types.ContainerState{Pid: containerPid},
				HostConfig: &dockercontainer.HostConfig{
					NetworkMode: containerNetworkMode,
				},
			},
		}, nil).MaxTimes(1),
		mockCNIClient.EXPECT().CleanupNS(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).MaxTimes(1),
	)

	taskEngine.(*DockerTaskEngine).checkTearDownPauseContainer(testTask)
	require.True(t, pauseContainer.IsContainerTornDown())

	// Invoke one more time to check for idempotency (mocks configured with maxTimes = 1)
	taskEngine.(*DockerTaskEngine).checkTearDownPauseContainer(testTask)
}

// TestTaskWithCircularDependency tests the task with containers of which the
// dependencies can't be resolved
func TestTaskWithCircularDependency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().ContainerEvents(gomock.Any())
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()

	task := testdata.LoadTask("circular_dependency")

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	events := taskEngine.StateChangeEvents()
	go taskEngine.AddTask(task)
	event := <-events
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskStopped, "Expected task to move to stopped directly")
	_, ok := taskEngine.(*DockerTaskEngine).state.TaskByArn(task.Arn)
	assert.True(t, ok, "Task state should be added to the agent state")

	_, ok = taskEngine.(*DockerTaskEngine).managedTasks[task.Arn]
	assert.False(t, ok, "Task should not be added to task manager for processing")
}

// TestCreateContainerOnAgentRestart tests when agent restarts it should use the
// docker container name restored from agent state file to create the container
func TestCreateContainerOnAgentRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, _, _, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()

	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	state := taskEngine.State()
	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer, _ := sleepTask.ContainerByName("sleep5")
	// Store the generated container name to state
	state.AddContainer(&apicontainer.DockerContainer{DockerID: "dockerID", DockerName: "docker_container_name", Container: sleepContainer}, sleepTask)

	gomock.InOrder(
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), "docker_container_name", gomock.Any()),
	)

	metadata := taskEngine.createContainer(sleepTask, sleepContainer)
	if metadata.Error != nil {
		t.Error("Unexpected error", metadata.Error)
	}
}

func TestPullCNIImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, privateTaskEngine, _, _, _, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)

	container := &apicontainer.Container{
		Type: apicontainer.ContainerCNIPause,
	}
	task := &apitask.Task{
		Containers: []*apicontainer.Container{container},
	}
	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, dockerapi.DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestPullNormalImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, imageManager, _, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	taskEngine._time = nil
	imageName := "image"
	container := &apicontainer.Container{
		Type:  apicontainer.ContainerNormal,
		Image: imageName,
	}
	task := &apitask.Task{
		Containers: []*apicontainer.Container{container},
	}
	imageState := &image.ImageState{
		Image: &image.Image{ImageID: "id"},
	}

	client.EXPECT().PullImage(gomock.Any(), imageName, nil, gomock.Any())
	imageManager.EXPECT().RecordContainerReference(container)
	imageManager.EXPECT().GetImageStateFromImageName(imageName).Return(imageState, true)
	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, dockerapi.DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestPullImageWithImagePullOnceBehavior(t *testing.T) {
	testcases := []struct {
		name          string
		pullSucceeded bool
	}{
		{
			name:          "PullSucceeded is true",
			pullSucceeded: true,
		},
		{
			name:          "PullSucceeded is false",
			pullSucceeded: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, _, privateTaskEngine, _, imageManager, _, _ := mocks(t, ctx, &config.Config{ImagePullBehavior: config.ImagePullOnceBehavior})
			defer ctrl.Finish()
			taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
			taskEngine._time = nil
			imageName := "image"
			container := &apicontainer.Container{
				Type:  apicontainer.ContainerNormal,
				Image: imageName,
			}
			task := &apitask.Task{
				Containers: []*apicontainer.Container{container},
			}
			imageState := &image.ImageState{
				Image:         &image.Image{ImageID: "id"},
				PullSucceeded: tc.pullSucceeded,
			}
			if !tc.pullSucceeded {
				client.EXPECT().PullImage(gomock.Any(), imageName, nil, gomock.Any())
			}
			imageManager.EXPECT().RecordContainerReference(container)
			imageManager.EXPECT().GetImageStateFromImageName(imageName).Return(imageState, true).Times(2)
			metadata := taskEngine.pullContainer(task, container)
			assert.Equal(t, dockerapi.DockerContainerMetadata{}, metadata, "expected empty metadata")
		})
	}
}

func TestPullImageWithImagePullPreferCachedBehaviorWithCachedImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, imageManager, _, _ := mocks(t, ctx, &config.Config{ImagePullBehavior: config.ImagePullPreferCachedBehavior})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	taskEngine._time = nil
	imageName := "image"
	container := &apicontainer.Container{
		Type:  apicontainer.ContainerNormal,
		Image: imageName,
	}
	task := &apitask.Task{
		Containers: []*apicontainer.Container{container},
	}
	imageState := &image.ImageState{
		Image: &image.Image{ImageID: "id"},
	}
	client.EXPECT().InspectImage(imageName).Return(nil, nil)
	imageManager.EXPECT().RecordContainerReference(container)
	imageManager.EXPECT().GetImageStateFromImageName(imageName).Return(imageState, true)
	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, dockerapi.DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestPullImageWithImagePullPreferCachedBehaviorWithoutCachedImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, imageManager, _, _ := mocks(t, ctx, &config.Config{ImagePullBehavior: config.ImagePullPreferCachedBehavior})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	taskEngine._time = nil
	imageName := "image"
	container := &apicontainer.Container{
		Type:  apicontainer.ContainerNormal,
		Image: imageName,
	}
	task := &apitask.Task{
		Containers: []*apicontainer.Container{container},
	}
	imageState := &image.ImageState{
		Image: &image.Image{ImageID: "id"},
	}
	client.EXPECT().InspectImage(imageName).Return(nil, errors.New("error"))
	client.EXPECT().PullImage(gomock.Any(), imageName, nil, gomock.Any())
	imageManager.EXPECT().RecordContainerReference(container)
	imageManager.EXPECT().GetImageStateFromImageName(imageName).Return(imageState, true)
	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, dockerapi.DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestUpdateContainerReference(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, privateTaskEngine, _, imageManager, _, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	taskEngine._time = nil
	imageName := "image"
	container := &apicontainer.Container{
		Type:  apicontainer.ContainerNormal,
		Image: imageName,
	}
	task := &apitask.Task{
		Containers: []*apicontainer.Container{container},
	}
	imageState := &image.ImageState{
		Image: &image.Image{ImageID: "id"},
	}

	imageManager.EXPECT().RecordContainerReference(container)
	imageManager.EXPECT().GetImageStateFromImageName(imageName).Return(imageState, true)
	taskEngine.updateContainerReference(true, container, task.Arn)
	assert.True(t, imageState.PullSucceeded, "PullSucceeded set to false")
}

// TestPullAndUpdateContainerReference checks whether a container is added to task engine state when
// Test # | Image availability  | DependentContainersPullUpfront | ImagePullBehavior
// -----------------------------------------------------------------------------------
//
//	1  |       remote        |              enabled           |      default
//	2  |       remote        |              disabled          |      default
//	3  |       local         |              enabled           |      default
//	4  |       local         |              enabled           |       once
//	5  |       local         |              enabled           |    prefer-cached
//	6  |       local         |              enabled           |       always
func TestPullAndUpdateContainerReference(t *testing.T) {
	testcases := []struct {
		Name                 string
		ImagePullUpfront     config.BooleanDefaultFalse
		ImagePullBehavior    config.ImagePullBehaviorType
		ImageState           *image.ImageState
		ImageInspect         *types.ImageInspect
		InspectImage         bool
		NumOfPulledContainer int
		PullImageErr         apierrors.NamedError
	}{
		{
			Name:              "DependentContainersPullUpfrontEnabledWithRemoteImage",
			ImagePullUpfront:  config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			ImagePullBehavior: config.ImagePullDefaultBehavior,
			ImageState: &image.ImageState{
				Image: &image.Image{ImageID: "id"},
			},
			InspectImage:         false,
			NumOfPulledContainer: 1,
			PullImageErr:         nil,
		},
		{
			Name:              "DependentContainersPullUpfrontDisabledWithRemoteImage",
			ImagePullUpfront:  config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled},
			ImagePullBehavior: config.ImagePullDefaultBehavior,
			ImageState: &image.ImageState{
				Image: &image.Image{ImageID: "id"},
			},
			InspectImage:         false,
			NumOfPulledContainer: 1,
			PullImageErr:         nil,
		},
		{
			Name:                 "DependentContainersPullUpfrontEnabledWithCachedImage",
			ImagePullUpfront:     config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			ImagePullBehavior:    config.ImagePullDefaultBehavior,
			ImageState:           nil,
			ImageInspect:         nil,
			InspectImage:         true,
			NumOfPulledContainer: 1,
			PullImageErr:         dockerapi.CannotPullContainerError{fmt.Errorf("error")},
		},
		{
			Name:                 "DependentContainersPullUpfrontEnabledAndImagePullOnceBehavior",
			ImagePullUpfront:     config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			ImagePullBehavior:    config.ImagePullOnceBehavior,
			ImageState:           nil,
			ImageInspect:         nil,
			InspectImage:         true,
			NumOfPulledContainer: 1,
			PullImageErr:         dockerapi.CannotPullContainerError{fmt.Errorf("error")},
		},
		{
			Name:                 "DependentContainersPullUpfrontEnabledAndImagePullPreferCachedBehavior",
			ImagePullUpfront:     config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			ImagePullBehavior:    config.ImagePullPreferCachedBehavior,
			ImageState:           nil,
			ImageInspect:         nil,
			InspectImage:         true,
			NumOfPulledContainer: 1,
			PullImageErr:         dockerapi.CannotPullContainerError{fmt.Errorf("error")},
		},
		{
			Name:                 "DependentContainersPullUpfrontEnabledAndImagePullAlwaysBehavior",
			ImagePullUpfront:     config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			ImagePullBehavior:    config.ImagePullAlwaysBehavior,
			ImageState:           nil,
			ImageInspect:         nil,
			InspectImage:         false,
			NumOfPulledContainer: 0,
			PullImageErr:         dockerapi.CannotPullContainerError{fmt.Errorf("error")},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			cfg := &config.Config{
				DependentContainersPullUpfront: tc.ImagePullUpfront,
				ImagePullBehavior:              tc.ImagePullBehavior,
			}
			ctrl, client, _, privateTaskEngine, _, imageManager, _, _ := mocks(t, ctx, cfg)
			defer ctrl.Finish()

			taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
			taskEngine._time = nil
			imageName := "image"
			taskArn := "taskArn"
			container := &apicontainer.Container{
				Type:      apicontainer.ContainerNormal,
				Image:     imageName,
				Essential: true,
			}

			task := &apitask.Task{
				Arn:        taskArn,
				Containers: []*apicontainer.Container{container},
			}

			client.EXPECT().PullImage(gomock.Any(), imageName, nil, gomock.Any()).
				Return(dockerapi.DockerContainerMetadata{Error: tc.PullImageErr})

			if tc.InspectImage {
				client.EXPECT().InspectImage(imageName).Return(tc.ImageInspect, nil)
			}

			imageManager.EXPECT().RecordContainerReference(container)
			imageManager.EXPECT().GetImageStateFromImageName(imageName).Return(tc.ImageState, false)
			metadata := taskEngine.pullAndUpdateContainerReference(task, container)
			pulledContainersMap, _ := taskEngine.State().PulledContainerMapByArn(taskArn)
			require.Len(t, pulledContainersMap, tc.NumOfPulledContainer)
			assert.Equal(t, dockerapi.DockerContainerMetadata{Error: tc.PullImageErr},
				metadata, "expected metadata with error")
		})
	}
}

// TestMetadataFileUpdatedAgentRestart checks whether metadataManager.Update(...) is
// invoked in the path DockerTaskEngine.Init() -> .synchronizeState() -> .updateMetadataFile(...)
// for the following case:
// agent starts, container created, metadata file created, agent restarted, container recovered
// during task engine init, metadata file updated
func TestMetadataFileUpdatedAgentRestart(t *testing.T) {
	conf := &defaultConfig
	conf.ContainerMetadataEnabled = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, imageManager, metadataManager, serviceConnectManager := mocks(t, ctx, conf)
	defer ctrl.Finish()

	var metadataUpdateWG sync.WaitGroup
	metadataUpdateWG.Add(1)
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	assert.True(t, taskEngine.cfg.ContainerMetadataEnabled.Enabled(), "ContainerMetadataEnabled set to false.")

	taskEngine._time = nil
	state := taskEngine.State()
	task := testdata.LoadTask("sleep5")
	container, _ := task.ContainerByName("sleep5")
	assert.False(t, container.MetadataFileUpdated)
	container.SetKnownStatus(apicontainerstatus.ContainerRunning)
	dockerContainer := &apicontainer.DockerContainer{DockerID: containerID, Container: container}
	expectedTaskARN := task.Arn
	expectedDockerID := dockerContainer.DockerID
	expectedContainerName := container.Name

	state.AddTask(task)
	state.AddContainer(dockerContainer, task)
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	_, watcherCancel := context.WithTimeout(context.Background(), time.Second)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().Do(func() {
		watcherCancel()
	}).AnyTimes()
	client.EXPECT().DescribeContainer(gomock.Any(), gomock.Any())
	imageManager.EXPECT().RecordContainerReference(gomock.Any())

	metadataManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(ctx interface{}, dockerID string, task *apitask.Task, containerName string) {
			assert.Equal(t, expectedTaskARN, task.Arn)
			assert.Equal(t, expectedContainerName, containerName)
			assert.Equal(t, expectedDockerID, dockerID)
			metadataUpdateWG.Done()
		})

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	defer taskEngine.Disable()
	metadataUpdateWG.Wait()
}

// TestTaskUseExecutionRolePullECRImage tests the agent will use the execution role
// credentials to pull from an ECR repository
func TestTaskUseExecutionRolePullECRImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, _, _ := mocks(
		t, ctx, &defaultConfig)
	defer ctrl.Finish()

	credentialsID := "execution role"
	accessKeyID := "akid"
	secretAccessKey := "sakid"
	sessionToken := "token"
	executionRoleCredentials := credentials.IAMRoleCredentials{
		CredentialsID:   credentialsID,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
	}

	testTask := testdata.LoadTask("sleep5")
	// Configure the task and container to use execution role
	testTask.SetExecutionRoleCredentialsID(credentialsID)
	testTask.Containers[0].RegistryAuthentication = &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			UseExecutionRole: true,
		},
	}
	container := testTask.Containers[0]

	mockTime.EXPECT().Now().AnyTimes()
	credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(credentials.TaskIAMRoleCredentials{
		ARN:                "",
		IAMRoleCredentials: executionRoleCredentials,
	}, true)
	client.EXPECT().PullImage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(ctx interface{}, image string, auth *apicontainer.RegistryAuthenticationData, timeout interface{}) {
			assert.Equal(t, container.Image, image)
			assert.Equal(t, auth.ECRAuthData.GetPullCredentials(), executionRoleCredentials)
		}).Return(dockerapi.DockerContainerMetadata{})
	imageManager.EXPECT().RecordContainerReference(container).Return(nil)
	imageManager.EXPECT().GetImageStateFromImageName(container.Image)

	taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
}

// TestTaskUseExecutionRolePullPrivateRegistryImage tests the agent will use the
// execution role credentials to pull from a private repository
func TestTaskUseExecutionRolePullPrivateRegistryImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, _, _ := mocks(
		t, ctx, &defaultConfig)
	defer ctrl.Finish()

	credentialsID := "execution role"
	accessKeyID := "akid"
	secretAccessKey := "sakid"
	sessionToken := "token"
	executionRoleCredentials := credentials.IAMRoleCredentials{
		CredentialsID:   credentialsID,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
	}
	testTask := testdata.LoadTask("sleep5")
	// Configure the task and container to use execution role
	testTask.SetExecutionRoleCredentialsID(credentialsID)
	asmAuthData := &apicontainer.ASMAuthData{
		CredentialsParameter: secretID,
		Region:               region,
	}
	testTask.Containers[0].RegistryAuthentication = &apicontainer.RegistryAuthenticationData{
		Type:        "asm",
		ASMAuthData: asmAuthData,
	}
	requiredASMResources := []*apicontainer.ASMAuthData{asmAuthData}
	asmClientCreator := mock_asm_factory.NewMockClientCreator(ctrl)
	asmAuthRes := asmauth.NewASMAuthResource(testTask.Arn, requiredASMResources,
		credentialsID, credentialsManager, asmClientCreator)
	testTask.ResourcesMapUnsafe = map[string][]taskresource.TaskResource{
		asmauth.ResourceName: {asmAuthRes},
	}
	mockASMClient := mock_secretsmanageriface.NewMockSecretsManagerAPI(ctrl)
	asmAuthDataBytes, _ := json.Marshal(&asm.AuthDataValue{
		Username: aws.String(username),
		Password: aws.String(password),
	})
	asmAuthDataVal := string(asmAuthDataBytes)
	asmSecretValue := &secretsmanager.GetSecretValueOutput{
		SecretString: aws.String(asmAuthDataVal),
	}

	gomock.InOrder(
		credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(
			credentials.TaskIAMRoleCredentials{
				ARN:                "",
				IAMRoleCredentials: executionRoleCredentials,
			}, true),
		asmClientCreator.EXPECT().NewASMClient(region, executionRoleCredentials).Return(mockASMClient),
		mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Return(asmSecretValue, nil),
	)
	require.NoError(t, asmAuthRes.Create())
	container := testTask.Containers[0]

	mockTime.EXPECT().Now().AnyTimes()
	client.EXPECT().PullImage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(ctx interface{}, image string, auth *apicontainer.RegistryAuthenticationData, timeout interface{}) {
			assert.Equal(t, container.Image, image)
			dac := auth.ASMAuthData.GetDockerAuthConfig()
			assert.Equal(t, username, dac.Username)
			assert.Equal(t, password, dac.Password)
		}).Return(dockerapi.DockerContainerMetadata{})
	imageManager.EXPECT().RecordContainerReference(container).Return(nil)
	imageManager.EXPECT().GetImageStateFromImageName(container.Image)

	ret := taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
	assert.Nil(t, ret.Error)
}

// TestTaskUseExecutionRolePullPrivateRegistryImageNoASMResource tests the
// docker task engine code path for returning error for missing ASM resource
func TestTaskUseExecutionRolePullPrivateRegistryImageNoASMResource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, mockTime, taskEngine, _, _, _, _ := mocks(
		t, ctx, &defaultConfig)
	defer ctrl.Finish()

	testTask := testdata.LoadTask("sleep5")
	// Configure the task and container to use execution role
	testTask.SetExecutionRoleCredentialsID(credentialsID)
	asmAuthData := &apicontainer.ASMAuthData{
		CredentialsParameter: secretID,
		Region:               region,
	}
	testTask.Containers[0].RegistryAuthentication = &apicontainer.RegistryAuthenticationData{
		Type:        "asm",
		ASMAuthData: asmAuthData,
	}

	// no asm auth resource in task
	testTask.ResourcesMapUnsafe = map[string][]taskresource.TaskResource{}

	container := testTask.Containers[0]
	mockTime.EXPECT().Now().AnyTimes()

	// ensure pullContainer returns error
	ret := taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
	assert.NotNil(t, ret.Error)
}

// TestNewTaskTransitionOnRestart tests the agent will process the task recorded in
// the state file on restart
func TestNewTaskTransitionOnRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, _, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockTime.EXPECT().Now().AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	client.EXPECT().Version(gomock.Any(), gomock.Any()).MaxTimes(1)
	client.EXPECT().ContainerEvents(gomock.Any()).MaxTimes(1)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	state := dockerTaskEngine.State()
	testTask := testdata.LoadTask("sleep5")
	// add the task to the state to simulate the agent restored the state on restart
	state.AddTask(testTask)
	// Set the task to be stopped so that the process can done quickly
	testTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	dockerTaskEngine.synchronizeState()
	_, ok := dockerTaskEngine.managedTasks[testTask.Arn]
	assert.True(t, ok, "task wasnot started")
}

// TestTaskWaitForHostResourceOnRestart tests task stopped by acs but hasn't
// reached stopped should block the later task to start
func TestTaskWaitForHostResourceOnRestart(t *testing.T) {
	// Task 1 stopped by backend
	taskStoppedByACS := testdata.LoadTask("sleep5")
	taskStoppedByACS.SetDesiredStatus(apitaskstatus.TaskStopped)
	taskStoppedByACS.SetStopSequenceNumber(1)
	taskStoppedByACS.SetKnownStatus(apitaskstatus.TaskRunning)
	// Task 2 has essential container stopped
	taskEssentialContainerStopped := testdata.LoadTask("sleep5")
	taskEssentialContainerStopped.Arn = "task_Essential_Container_Stopped"
	taskEssentialContainerStopped.SetDesiredStatus(apitaskstatus.TaskStopped)
	taskEssentialContainerStopped.SetKnownStatus(apitaskstatus.TaskRunning)
	// Normal task 3 needs to be started
	taskNotStarted := testdata.LoadTask("sleep5")
	taskNotStarted.Arn = "task_Not_started"

	conf := &defaultConfig
	conf.ContainerMetadataEnabled = config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, conf)
	defer ctrl.Finish()

	client.EXPECT().Version(gomock.Any(), gomock.Any()).MaxTimes(1)
	client.EXPECT().ContainerEvents(gomock.Any()).MaxTimes(1)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()

	err := privateTaskEngine.Init(ctx)
	assert.NoError(t, err)

	taskEngine := privateTaskEngine.(*DockerTaskEngine)
	taskEngine.State().AddTask(taskStoppedByACS)
	taskEngine.State().AddTask(taskNotStarted)
	taskEngine.State().AddTask(taskEssentialContainerStopped)

	taskEngine.State().AddContainer(&apicontainer.DockerContainer{
		Container:  taskStoppedByACS.Containers[0],
		DockerID:   containerID + "1",
		DockerName: dockerContainerName + "1",
	}, taskStoppedByACS)
	taskEngine.State().AddContainer(&apicontainer.DockerContainer{
		Container:  taskNotStarted.Containers[0],
		DockerID:   containerID + "2",
		DockerName: dockerContainerName + "2",
	}, taskNotStarted)
	taskEngine.State().AddContainer(&apicontainer.DockerContainer{
		Container:  taskEssentialContainerStopped.Containers[0],
		DockerID:   containerID + "3",
		DockerName: dockerContainerName + "3",
	}, taskEssentialContainerStopped)

	// these are performed in synchronizeState on restart
	client.EXPECT().DescribeContainer(gomock.Any(), gomock.Any()).Return(apicontainerstatus.ContainerRunning, dockerapi.DockerContainerMetadata{
		DockerID: containerID,
	}).Times(3)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Times(3)
	// start the two tasks
	taskEngine.synchronizeState()

	var waitStopWG sync.WaitGroup
	waitStopWG.Add(1)
	go func() {
		// This is to confirm the other task is waiting
		time.Sleep(1 * time.Second)
		// Remove the task sequence number 1 from waitgroup
		taskEngine.taskStopGroup.Done(1)
		waitStopWG.Done()
	}()

	// task with sequence number 2 should wait until 1 is removed from the waitgroup
	taskEngine.taskStopGroup.Wait(2)
	waitStopWG.Wait()
}

// TestPullStartedStoppedAtWasSetCorrectly tests the PullStartedAt and PullStoppedAt
// was set correctly
func TestPullStartedStoppedAtWasSetCorrectly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	testTask := &apitask.Task{
		Arn: "taskArn",
	}
	container := &apicontainer.Container{
		Image: "image1",
	}
	startTime1 := time.Now()
	startTime2 := startTime1.Add(time.Second)
	startTime3 := startTime2.Add(time.Second)
	stopTime1 := startTime3.Add(time.Second)
	stopTime2 := stopTime1.Add(time.Second)
	stopTime3 := stopTime2.Add(time.Second)

	client.EXPECT().PullImage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(3)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Times(3)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false).Times(3)

	gomock.InOrder(
		// three container pull start timestamp
		mockTime.EXPECT().Now().Return(startTime1),
		mockTime.EXPECT().Now().Return(startTime2),
		mockTime.EXPECT().Now().Return(startTime3),

		// threre container pull stop timestamp
		mockTime.EXPECT().Now().Return(stopTime1),
		mockTime.EXPECT().Now().Return(stopTime2),
		mockTime.EXPECT().Now().Return(stopTime3),
	)

	// Pull three images, the PullStartedAt should be the pull of the first container
	// and PullStoppedAt should be the pull completion of the last container
	taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
	taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
	taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)

	assert.Equal(t, testTask.PullStartedAtUnsafe, startTime1)
	assert.Equal(t, testTask.PullStoppedAtUnsafe, stopTime3)
}

// TestPullStoppedAtWasSetCorrectlyWhenPullFail tests the PullStoppedAt was set
// correctly when the pull failed
func TestPullStoppedAtWasSetCorrectlyWhenPullFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	testTask := &apitask.Task{
		Arn: "taskArn",
	}
	container := &apicontainer.Container{
		Image: "image1",
	}

	startTime1 := time.Now()
	startTime2 := startTime1.Add(time.Second)
	startTime3 := startTime2.Add(time.Second)
	stopTime1 := startTime3.Add(time.Second)
	stopTime2 := stopTime1.Add(time.Second)
	stopTime3 := stopTime2.Add(time.Second)

	gomock.InOrder(
		client.EXPECT().PullImage(gomock.Any(), container.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}),
		client.EXPECT().PullImage(gomock.Any(), container.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{}),
		client.EXPECT().PullImage(gomock.Any(), container.Image, nil, gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{Error: dockerapi.CannotPullContainerError{fmt.Errorf("error")}}),
	)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Times(3)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false).Times(3)
	gomock.InOrder(
		// three container pull start timestamp
		mockTime.EXPECT().Now().Return(startTime1),
		mockTime.EXPECT().Now().Return(startTime2),
		mockTime.EXPECT().Now().Return(startTime3),

		// threre container pull stop timestamp
		mockTime.EXPECT().Now().Return(stopTime1),
		mockTime.EXPECT().Now().Return(stopTime2),
		mockTime.EXPECT().Now().Return(stopTime3),
	)

	// Pull three images, the PullStartedAt should be the pull of the first container
	// and PullStoppedAt should be the pull completion of the last container
	taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
	taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
	taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)

	assert.Equal(t, testTask.PullStartedAtUnsafe, startTime1)
	assert.Equal(t, testTask.PullStoppedAtUnsafe, stopTime3)
}

func TestSynchronizeContainerStatus(t *testing.T) {
	testContainerName := "c1"
	testDockerID := "1234"
	testServiceConnectContainerName := "service-connect"
	testLabels := map[string]string{
		"name": "metadata",
	}
	testVolumes := []types.MountPoint{
		{
			Name:        "volume",
			Source:      "/src/vol",
			Destination: "/vol",
		},
	}
	testAppContainer := &apicontainer.Container{
		Name: testContainerName,
		Type: apicontainer.ContainerNormal,
	}
	testCases := []struct {
		name                       string
		serviceConnectEnabled      bool
		addPauseContainer          bool
		pauseContainerName         string
		pauseContainerPortBindings []apicontainer.PortBinding
		networkMode                string
	}{
		{
			name:                  "Service connect bridge mode with matched pause container",
			serviceConnectEnabled: true,
			addPauseContainer:     true,
			pauseContainerName:    fmt.Sprintf("%s-%s", apitask.NetworkPauseContainerName, testContainerName),
			pauseContainerPortBindings: []apicontainer.PortBinding{
				{
					ContainerPort: aws.Uint16(8080),
				},
			},
			networkMode: networkModeBridge,
		},
		{
			name:                  "Default task",
			serviceConnectEnabled: false,
			addPauseContainer:     false,
			networkMode:           networkModeAWSVPC,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, _, taskEngine, _, imageManager, _, _ := mocks(t, ctx, &defaultConfig)
			defer ctrl.Finish()

			testTask := &apitask.Task{
				Containers:  []*apicontainer.Container{testAppContainer},
				NetworkMode: tc.networkMode,
			}

			dockerContainer := &apicontainer.DockerContainer{
				DockerID:   testDockerID,
				DockerName: testContainerName,
				Container:  testAppContainer,
			}
			testCreated := time.Now()
			gomock.InOrder(
				client.EXPECT().DescribeContainer(gomock.Any(), testDockerID).Return(apicontainerstatus.ContainerRunning,
					dockerapi.DockerContainerMetadata{
						Labels:    testLabels,
						DockerID:  testDockerID,
						CreatedAt: testCreated,
						Volumes:   testVolumes,
					}),
				imageManager.EXPECT().RecordContainerReference(dockerContainer.Container),
			)

			if tc.serviceConnectEnabled {
				testTask.ServiceConnectConfig = &serviceconnect.Config{
					ContainerName: "service-connect",
				}
				scContainer := &apicontainer.Container{
					Name: testServiceConnectContainerName,
				}
				testTask.Containers = append(testTask.Containers, scContainer)
			}
			pauseContainer := &apicontainer.Container{}
			if tc.addPauseContainer {
				pauseContainer.Name = tc.pauseContainerName
				pauseContainer.Type = apicontainer.ContainerCNIPause
				pauseContainer.SetKnownPortBindings(tc.pauseContainerPortBindings)
				testTask.Containers = append(testTask.Containers, pauseContainer)
			}
			taskEngine.(*DockerTaskEngine).synchronizeContainerStatus(dockerContainer, testTask)
			assert.Equal(t, testCreated, dockerContainer.Container.GetCreatedAt())
			assert.Equal(t, testLabels, dockerContainer.Container.GetLabels())
			assert.Equal(t, testVolumes, dockerContainer.Container.GetVolumes())

			if tc.serviceConnectEnabled && tc.addPauseContainer {
				assert.Equal(t, tc.pauseContainerPortBindings, dockerContainer.Container.GetKnownPortBindings())
			}
		})
	}
}

// TestHandleDockerHealthEvent tests the docker health event will only cause the
// container health status change
func TestHandleDockerHealthEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	state := taskEngine.(*DockerTaskEngine).State()
	testTask := testdata.LoadTask("sleep5")
	testContainer := testTask.Containers[0]
	testContainer.HealthCheckType = "docker"

	state.AddTask(testTask)
	state.AddContainer(&apicontainer.DockerContainer{DockerID: "id",
		DockerName: "container_name",
		Container:  testContainer,
	}, testTask)

	taskEngine.(*DockerTaskEngine).handleDockerEvent(dockerapi.DockerContainerChangeEvent{
		Status: apicontainerstatus.ContainerRunning,
		Type:   apicontainer.ContainerHealthEvent,
		DockerContainerMetadata: dockerapi.DockerContainerMetadata{
			DockerID: "id",
			Health: apicontainer.HealthStatus{
				Status: apicontainerstatus.ContainerHealthy,
			},
		},
	})
	assert.Equal(t, testContainer.Health.Status, apicontainerstatus.ContainerHealthy)
}

func TestContainerMetadataUpdatedOnRestart(t *testing.T) {
	dockerID := "dockerID_created"
	labels := map[string]string{
		"name": "metadata",
	}
	testCases := []struct {
		stage        string
		status       apicontainerstatus.ContainerStatus
		created      time.Time
		started      time.Time
		finished     time.Time
		portBindings []apicontainer.PortBinding
		exitCode     *int
		err          dockerapi.DockerStateError
	}{
		{
			stage:   "created",
			status:  apicontainerstatus.ContainerCreated,
			created: time.Now(),
		},
		{
			stage:   "started",
			status:  apicontainerstatus.ContainerRunning,
			started: time.Now(),
			portBindings: []apicontainer.PortBinding{
				{
					ContainerPort: aws.Uint16(80),
					HostPort:      80,
					BindIP:        "0.0.0.0/0",
					Protocol:      apicontainer.TransportProtocolTCP,
				},
			},
		},
		{
			stage:    "stopped",
			finished: time.Now(),
			exitCode: aws.Int(1),
		},
		{
			stage:    "failed",
			status:   apicontainerstatus.ContainerStopped,
			err:      dockerapi.NewDockerStateError("error"),
			exitCode: aws.Int(1),
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Agent restarted during container: %s", tc.stage), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			ctrl, client, _, taskEngine, _, imageManager, _, _ := mocks(t, ctx, &defaultConfig)
			defer ctrl.Finish()
			dockerContainer := &apicontainer.DockerContainer{
				DockerID:   dockerID,
				DockerName: fmt.Sprintf("docker%s", tc.stage),
				Container:  &apicontainer.Container{},
			}
			task := &apitask.Task{}

			if tc.stage == "created" {
				dockerContainer.DockerID = ""
				task.Volumes = []apitask.TaskVolume{
					{
						Name:   "empty",
						Volume: &taskresourcevolume.LocalDockerVolume{},
					},
				}
				client.EXPECT().InspectContainer(gomock.Any(), dockerContainer.DockerName, gomock.Any()).Return(&types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						ID:      dockerID,
						Created: (tc.created).Format(time.RFC3339),
						State: &types.ContainerState{
							Health: &types.Health{},
						},
						HostConfig: &dockercontainer.HostConfig{
							NetworkMode: containerNetworkMode,
						},
					},
					Config: &dockercontainer.Config{
						Labels: labels,
					},
				}, nil)
				imageManager.EXPECT().RecordContainerReference(dockerContainer.Container).AnyTimes()
			} else {
				client.EXPECT().DescribeContainer(gomock.Any(), dockerID).Return(tc.status, dockerapi.DockerContainerMetadata{
					Labels:       labels,
					DockerID:     dockerID,
					CreatedAt:    tc.created,
					StartedAt:    tc.started,
					FinishedAt:   tc.finished,
					PortBindings: tc.portBindings,
					ExitCode:     tc.exitCode,
					Error:        tc.err,
				})
				imageManager.EXPECT().RecordContainerReference(dockerContainer.Container).AnyTimes()
			}

			taskEngine.(*DockerTaskEngine).synchronizeContainerStatus(dockerContainer, task)
			assert.Equal(t, labels, dockerContainer.Container.GetLabels())
			assert.Equal(t, (tc.created).Format(time.RFC3339), (dockerContainer.Container.GetCreatedAt()).Format(time.RFC3339))
			assert.Equal(t, (tc.started).Format(time.RFC3339), (dockerContainer.Container.GetStartedAt()).Format(time.RFC3339))
			assert.Equal(t, (tc.finished).Format(time.RFC3339), (dockerContainer.Container.GetFinishedAt()).Format(time.RFC3339))
			if tc.stage == "started" {
				assert.Equal(t, aws.Uint16(80), dockerContainer.Container.KnownPortBindingsUnsafe[0].ContainerPort)
			}
			if tc.stage == "finished" {
				assert.False(t, task.GetExecutionStoppedAt().IsZero())
				assert.Equal(t, tc.exitCode, dockerContainer.Container.GetKnownExitCode())
			}
			if tc.stage == "failed" {
				assert.Equal(t, tc.exitCode, dockerContainer.Container.GetKnownExitCode())
				assert.NotNil(t, dockerContainer.Container.ApplyingError)
			}
		})
	}
}

// TestContainerProgressParallize tests the container can be processed parallelly
func TestContainerProgressParallize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, testTime, taskEngine, _, imageManager, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	stateChangeEvents := taskEngine.StateChangeEvents()
	eventStream := make(chan dockerapi.DockerContainerChangeEvent)
	state := taskEngine.(*DockerTaskEngine).State()

	fastPullImage := "fast-pull-image"
	slowPullImage := "slow-pull-image"

	testTask := testdata.LoadTask("sleep5")

	containerTwo := &apicontainer.Container{
		Name:  fastPullImage,
		Image: fastPullImage,
	}

	testTask.Containers = append(testTask.Containers, containerTwo)
	testTask.Containers[0].Image = slowPullImage
	testTask.Containers[0].Name = slowPullImage

	var fastContainerDockerName string
	var slowContainerDockerName string
	fastContainerDockerID := "fast-pull-container-id"
	slowContainerDockerID := "slow-pull-container-id"

	var waitForFastPullContainer sync.WaitGroup
	waitForFastPullContainer.Add(1)

	client.EXPECT().Version(gomock.Any(), gomock.Any()).Return("17.12.0", nil).AnyTimes()
	testTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Return(nil).AnyTimes()
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false).AnyTimes()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	client.EXPECT().PullImage(gomock.Any(), fastPullImage, gomock.Any(), gomock.Any())
	client.EXPECT().PullImage(gomock.Any(), slowPullImage, gomock.Any(), gomock.Any()).Do(
		func(ctx interface{}, image interface{}, auth interface{}, timeout interface{}) {
			waitForFastPullContainer.Wait()
		})
	client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(ctx interface{}, cfg interface{}, hostconfig interface{}, name string, duration interface{}) {
			if strings.Contains(name, slowPullImage) {
				slowContainerDockerName = name
				state.AddContainer(&apicontainer.DockerContainer{
					DockerID:   slowContainerDockerID,
					DockerName: slowContainerDockerName,
					Container:  testTask.Containers[0],
				}, testTask)
				go func() {
					event := createDockerEvent(apicontainerstatus.ContainerCreated)
					event.DockerID = slowContainerDockerID
					eventStream <- event
				}()
			} else if strings.Contains(name, fastPullImage) {
				fastContainerDockerName = name
				state.AddTask(testTask)
				state.AddContainer(&apicontainer.DockerContainer{
					DockerID:   fastContainerDockerID,
					DockerName: fastContainerDockerName,
					Container:  testTask.Containers[1],
				}, testTask)
				go func() {
					event := createDockerEvent(apicontainerstatus.ContainerCreated)
					event.DockerID = fastContainerDockerID
					eventStream <- event
				}()
			} else {
				t.Fatalf("Got unexpected name for creating container: %s", name)
			}
		}).Times(2)
	client.EXPECT().StartContainer(gomock.Any(), fastContainerDockerID, gomock.Any()).Do(
		func(ctx interface{}, id string, duration interface{}) {
			go func() {
				event := createDockerEvent(apicontainerstatus.ContainerRunning)
				event.DockerID = fastContainerDockerID
				eventStream <- event
			}()
		})
	client.EXPECT().StartContainer(gomock.Any(), slowContainerDockerID, gomock.Any()).Do(
		func(ctx interface{}, id string, duration interface{}) {
			go func() {
				event := createDockerEvent(apicontainerstatus.ContainerRunning)
				event.DockerID = slowContainerDockerID
				eventStream <- event
			}()
		})
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()

	taskEngine.Init(ctx)
	taskEngine.AddTask(testTask)

	// Expect the fast pulled container to be running firs
	fastPullContainerRunning := false
	for event := range stateChangeEvents {
		containerEvent, ok := event.(api.ContainerStateChange)
		if ok && containerEvent.Status == apicontainerstatus.ContainerRunning {
			if containerEvent.ContainerName == fastPullImage {
				fastPullContainerRunning = true
				// The second container should start processing now
				waitForFastPullContainer.Done()
				continue
			}
			assert.True(t, fastPullContainerRunning, "got the slower pulled container running events first")
			continue
		}

		taskEvent, ok := event.(api.TaskStateChange)
		if ok && taskEvent.Status == apitaskstatus.TaskRunning {
			break
		}
		t.Errorf("Got unexpected task event: %v", taskEvent.String())
	}
	defer discardEvents(stateChangeEvents)()
	// stop and clean up the task
	cleanup := make(chan time.Time)
	client.EXPECT().StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		dockerapi.DockerContainerMetadata{DockerID: fastContainerDockerID}).AnyTimes()
	client.EXPECT().StopContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		dockerapi.DockerContainerMetadata{DockerID: slowContainerDockerID}).AnyTimes()
	testTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
	client.EXPECT().RemoveContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any()).Return(nil).Times(2)

	containerStoppedEvent := createDockerEvent(apicontainerstatus.ContainerStopped)
	containerStoppedEvent.DockerID = slowContainerDockerID
	eventStream <- containerStoppedEvent

	testTask.SetSentStatus(apitaskstatus.TaskStopped)
	cleanup <- time.Now()
	for {
		tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
		if len(tasks) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSynchronizeResource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, _, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockTime.EXPECT().Now().AnyTimes()
	client.EXPECT().Version(gomock.Any(), gomock.Any()).MaxTimes(1)
	client.EXPECT().ContainerEvents(gomock.Any()).MaxTimes(1)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	state := dockerTaskEngine.State()
	cgroupResource := mock_taskresource.NewMockTaskResource(ctrl)
	testTask := testdata.LoadTask("sleep5")
	testTask.ResourcesMapUnsafe = map[string][]taskresource.TaskResource{
		"cgroup": {
			cgroupResource,
		},
	}
	// add the task to the state to simulate the agent restored the state on restart
	state.AddTask(testTask)
	cgroupResource.EXPECT().Initialize(gomock.Any(), gomock.Any(), gomock.Any())
	cgroupResource.EXPECT().SetDesiredStatus(gomock.Any()).MaxTimes(1)
	cgroupResource.EXPECT().GetDesiredStatus().MaxTimes(2)
	cgroupResource.EXPECT().TerminalStatus().MaxTimes(1)
	cgroupResource.EXPECT().SteadyState().MaxTimes(1)
	cgroupResource.EXPECT().GetKnownStatus().MaxTimes(1)
	cgroupResource.EXPECT().GetName().AnyTimes().Return("cgroup")
	cgroupResource.EXPECT().StatusString(gomock.Any()).AnyTimes()

	// Set the task to be stopped so that the process can done quickly
	testTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	dockerTaskEngine.synchronizeState()
}

func TestSynchronizeENIAttachment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, _, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockTime.EXPECT().Now().AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	client.EXPECT().Version(gomock.Any(), gomock.Any()).MaxTimes(1)
	client.EXPECT().ContainerEvents(gomock.Any()).MaxTimes(1)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	state := dockerTaskEngine.State()
	testTask := testdata.LoadTask("sleep5")
	expiresAt := time.Now().Unix() + 1
	attachment := &apieni.ENIAttachment{
		TaskARN:       "TaskARN",
		AttachmentARN: "AttachmentARN",
		MACAddress:    "MACAddress",
		Status:        apieni.ENIAttachmentNone,
		ExpiresAt:     time.Unix(expiresAt, 0),
	}
	state.AddENIAttachment(attachment)

	state.AddTask(testTask)
	testTask.SetDesiredStatus(apitaskstatus.TaskStopped)
	dockerTaskEngine.synchronizeState()

	// If the below call doesn't panic on NPE, it means the ENI attachment has been properly initialized in synchronizeState.
	attachment.StopAckTimer()
}

func TestSynchronizeENIAttachmentRemoveData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _, serviceConnectManager := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	client.EXPECT().ContainerEvents(gomock.Any()).MaxTimes(1)
	serviceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	taskEngine.(*DockerTaskEngine).dataClient = dataClient
	dockerTaskEngine := taskEngine.(*DockerTaskEngine)

	attachment := &apieni.ENIAttachment{
		TaskARN:          "TaskARN",
		AttachmentARN:    testAttachmentArn,
		MACAddress:       "MACAddress",
		Status:           apieni.ENIAttachmentNone,
		AttachStatusSent: false,
	}

	// eni attachment data is removed if AttachStatusSent is unset
	dockerTaskEngine.state.AddENIAttachment(attachment)
	assert.NoError(t, dockerTaskEngine.dataClient.SaveENIAttachment(attachment))

	dockerTaskEngine.synchronizeState()
	attachments, err := dockerTaskEngine.dataClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, attachments, 0)
}

func TestTaskSecretsEnvironmentVariables(t *testing.T) {
	// metadata required for createContainer workflow validation
	taskARN := "secretsTask"
	taskFamily := "secretsTaskFamily"
	taskVersion := "1"
	taskContainerName := "secretsContainer"

	// metadata required for ssm secret resource validation
	ssmSecretName := "mySSMSecret"
	ssmSecretValueFrom := "ssm/mySecret"
	ssmSecretRetrievedValue := "mySSMSecretValue"
	ssmSecretRegion := "us-west-2"

	// metadata required for asm secret resource validation
	asmSecretName := "myASMSecret"
	asmSecretValueFrom := "arn:aws:secretsmanager:region:account-id:secret:" + asmSecretName
	asmSecretRetrievedValue := "myASMSecretValue"
	asmSecretRegion := "us-west-2"
	asmSecretKey := asmSecretValueFrom + "_" + asmSecretRegion

	ssmExpectedEnvVar := ssmSecretName + "=" + ssmSecretRetrievedValue
	asmExpectedEnvVar := asmSecretName + "=" + asmSecretRetrievedValue

	testCases := []struct {
		name        string
		secrets     []apicontainer.Secret
		ssmSecret   apicontainer.Secret
		asmSecret   apicontainer.Secret
		expectedEnv []string
	}{
		{
			name: "ASMSecretAsEnv",
			secrets: []apicontainer.Secret{
				{
					Name:      ssmSecretName,
					ValueFrom: ssmSecretValueFrom,
					Region:    ssmSecretRegion,
					Target:    "LOG_DRIVER",
					Provider:  "ssm",
				},
				{
					Name:      asmSecretName,
					ValueFrom: asmSecretValueFrom,
					Region:    asmSecretRegion,
					Type:      "ENVIRONMENT_VARIABLE",
					Provider:  "asm",
				},
			},
			ssmSecret: apicontainer.Secret{
				Name:      ssmSecretName,
				ValueFrom: ssmSecretValueFrom,
				Region:    ssmSecretRegion,
				Target:    "LOG_DRIVER",
				Provider:  "ssm",
			},
			asmSecret: apicontainer.Secret{
				Name:      asmSecretName,
				ValueFrom: asmSecretValueFrom,
				Region:    asmSecretRegion,
				Type:      "ENVIRONMENT_VARIABLE",
				Provider:  "asm",
			},
			expectedEnv: []string{asmExpectedEnvVar},
		},
		{
			name: "SSMSecretAsEnv",
			secrets: []apicontainer.Secret{
				{
					Name:      ssmSecretName,
					ValueFrom: ssmSecretValueFrom,
					Region:    ssmSecretRegion,
					Type:      "ENVIRONMENT_VARIABLE",
					Provider:  "ssm",
				},
				{
					Name:      asmSecretName,
					ValueFrom: asmSecretValueFrom,
					Region:    asmSecretRegion,
					Target:    "LOG_DRIVER",
					Provider:  "asm",
				},
			},
			ssmSecret: apicontainer.Secret{
				Name:      ssmSecretName,
				ValueFrom: ssmSecretValueFrom,
				Region:    ssmSecretRegion,
				Type:      "ENVIRONMENT_VARIABLE",
				Provider:  "ssm",
			},
			asmSecret: apicontainer.Secret{
				Name:      asmSecretName,
				ValueFrom: asmSecretValueFrom,
				Region:    asmSecretRegion,
				Target:    "LOG_DRIVER",
				Provider:  "asm",
			},
			expectedEnv: []string{ssmExpectedEnvVar},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, mockTime, taskEngine, credentialsManager, _, _, _ := mocks(t, ctx, &defaultConfig)
			defer ctrl.Finish()

			// sample test
			testTask := &apitask.Task{
				Arn:     taskARN,
				Family:  taskFamily,
				Version: taskVersion,
				Containers: []*apicontainer.Container{
					{
						Name:    taskContainerName,
						Secrets: tc.secrets,
					},
				},
			}

			// metadata required for execution role authentication workflow
			credentialsID := "execution role"
			executionRoleCredentials := credentials.IAMRoleCredentials{
				CredentialsID: credentialsID,
			}
			taskIAMcreds := credentials.TaskIAMRoleCredentials{
				IAMRoleCredentials: executionRoleCredentials,
			}

			// configure the task and container to use execution role
			testTask.SetExecutionRoleCredentialsID(credentialsID)

			// validate base config
			expectedConfig, err := testTask.DockerConfig(testTask.Containers[0], defaultDockerClientAPIVersion)
			if err != nil {
				t.Fatal(err)
			}

			expectedConfig.Labels = map[string]string{
				"com.amazonaws.ecs.task-arn":                taskARN,
				"com.amazonaws.ecs.container-name":          taskContainerName,
				"com.amazonaws.ecs.task-definition-family":  taskFamily,
				"com.amazonaws.ecs.task-definition-version": taskVersion,
				"com.amazonaws.ecs.cluster":                 "",
			}

			// required to validate container config includes secrets as environment variables
			expectedConfig.Env = tc.expectedEnv

			// required for validating ssm workflows
			ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)
			mockSSMClient := mock_ssmiface.NewMockSSMClient(ctrl)

			ssmRequirements := map[string][]apicontainer.Secret{
				ssmSecretRegion: []apicontainer.Secret{
					tc.ssmSecret,
				},
			}

			ssmSecretRes := ssmsecret.NewSSMSecretResource(
				testTask.Arn,
				ssmRequirements,
				credentialsID,
				credentialsManager,
				ssmClientCreator)

			// required for validating asm workflows
			asmClientCreator := mock_asm_factory.NewMockClientCreator(ctrl)
			mockASMClient := mock_secretsmanageriface.NewMockSecretsManagerAPI(ctrl)

			asmRequirements := map[string]apicontainer.Secret{
				asmSecretKey: tc.asmSecret,
			}

			asmSecretRes := asmsecret.NewASMSecretResource(
				testTask.Arn,
				asmRequirements,
				credentialsID,
				credentialsManager,
				asmClientCreator)

			testTask.ResourcesMapUnsafe = map[string][]taskresource.TaskResource{
				ssmsecret.ResourceName: {ssmSecretRes},
				asmsecret.ResourceName: {asmSecretRes},
			}

			ssmClientOutput := &ssm.GetParametersOutput{
				InvalidParameters: []*string{},
				Parameters: []*ssm.Parameter{
					&ssm.Parameter{
						Name:  aws.String(ssmSecretValueFrom),
						Value: aws.String(ssmSecretRetrievedValue),
					},
				},
			}

			asmClientOutput := &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String(asmSecretRetrievedValue),
			}

			reqSecretNames := []*string{aws.String(ssmSecretValueFrom)}

			credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(taskIAMcreds, true).Times(2)
			ssmClientCreator.EXPECT().NewSSMClient(region, executionRoleCredentials).Return(mockSSMClient)
			asmClientCreator.EXPECT().NewASMClient(region, executionRoleCredentials).Return(mockASMClient)

			mockSSMClient.EXPECT().GetParameters(gomock.Any()).Do(func(in *ssm.GetParametersInput) {
				assert.Equal(t, in.Names, reqSecretNames)
			}).Return(ssmClientOutput, nil).Times(1)

			mockASMClient.EXPECT().GetSecretValue(gomock.Any()).Do(func(in *secretsmanager.GetSecretValueInput) {
				assert.Equal(t, asmSecretValueFrom, aws.StringValue(in.SecretId))
			}).Return(asmClientOutput, nil).Times(1)

			require.NoError(t, ssmSecretRes.Create())
			require.NoError(t, asmSecretRes.Create())

			mockTime.EXPECT().Now().AnyTimes()
			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()

			// test validates that the expectedConfig includes secrets are appended as
			// environment varibles
			client.EXPECT().CreateContainer(gomock.Any(), expectedConfig, gomock.Any(), gomock.Any(), gomock.Any())
			ret := taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
			assert.Nil(t, ret.Error)

		})
	}
}

// TestCreateContainerAddFirelensLogDriverConfig tests that in createContainer, when the
// container is using firelens log driver, its logConfig is properly set.
func TestCreateContainerAddFirelensLogDriverConfig(t *testing.T) {
	taskName := "logSenderTask"
	taskARN := "arn:aws:ecs:region:account-id:task/task-id"
	taskID := "task-id"
	taskFamily := "logSenderTaskFamily"
	taskVersion := "1"
	logDriverTypeFirelens := "awsfirelens"
	dataLogDriverPath := "/data/firelens/"
	dataLogDriverSocketPath := "/socket/fluent.sock"
	socketPathPrefix := "unix://"
	networkModeBridge := "bridge"
	networkModeAWSVPC := "awsvpc"
	bridgeIPAddr := "bridgeIP"
	envVarBridgeMode := "FLUENT_HOST=bridgeIP"
	envVarPort := "FLUENT_PORT=24224"
	envVarAWSVPCMode := "FLUENT_HOST=127.0.0.1"
	eniIPv4Address := "10.0.0.2"
	getTask := func(logDriverType string, networkMode string) *apitask.Task {
		rawHostConfigInput := dockercontainer.HostConfig{
			LogConfig: dockercontainer.LogConfig{
				Type: logDriverType,
				Config: map[string]string{
					"key1":                    "value1",
					"key2":                    "value2",
					"log-driver-buffer-limit": "10000",
				},
			},
		}
		rawHostConfig, err := json.Marshal(&rawHostConfigInput)
		require.NoError(t, err)
		return &apitask.Task{
			Arn:         taskARN,
			Version:     taskVersion,
			Family:      taskFamily,
			NetworkMode: networkMode,
			Containers: []*apicontainer.Container{
				{
					Name: taskName,
					DockerConfig: apicontainer.DockerConfig{
						HostConfig: func() *string {
							s := string(rawHostConfig)
							return &s
						}(),
					},
					NetworkModeUnsafe: networkMode,
				},
				{
					Name: "test-container",
					FirelensConfig: &apicontainer.FirelensConfig{
						Type: "fluentd",
					},
					NetworkModeUnsafe: networkMode,
					NetworkSettingsUnsafe: &types.NetworkSettings{
						DefaultNetworkSettings: types.DefaultNetworkSettings{
							IPAddress: bridgeIPAddr,
						},
					},
				},
			},
		}
	}
	getTaskWithENI := func(logDriverType string, networkMode string) *apitask.Task {
		rawHostConfigInput := dockercontainer.HostConfig{
			LogConfig: dockercontainer.LogConfig{
				Type: logDriverType,
				Config: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		}
		rawHostConfig, err := json.Marshal(&rawHostConfigInput)
		require.NoError(t, err)
		return &apitask.Task{
			Arn:         taskARN,
			Version:     taskVersion,
			Family:      taskFamily,
			NetworkMode: networkMode,
			ENIs: []*apieni.ENI{
				{
					IPV4Addresses: []*apieni.ENIIPV4Address{
						{
							Address: eniIPv4Address,
						},
					},
				},
			},
			Containers: []*apicontainer.Container{
				{
					Name: taskName,
					DockerConfig: apicontainer.DockerConfig{
						HostConfig: func() *string {
							s := string(rawHostConfig)
							return &s
						}(),
					},
					NetworkModeUnsafe: networkMode,
				},
				{
					Name: "test-container",
					FirelensConfig: &apicontainer.FirelensConfig{
						Type: "fluentd",
					},
					NetworkModeUnsafe: networkMode,
					NetworkSettingsUnsafe: &types.NetworkSettings{
						DefaultNetworkSettings: types.DefaultNetworkSettings{
							IPAddress: bridgeIPAddr,
						},
					},
				},
			},
		}
	}
	testCases := []struct {
		name                           string
		task                           *apitask.Task
		expectedLogConfigType          string
		expectedLogConfigTag           string
		expectedLogConfigFluentAddress string
		expectedFluentdAsyncConnect    string
		expectedSubSecondPrecision     string
		expectedBufferLimit            string
		expectedIPAddress              string
		expectedPort                   string
	}{
		{
			name:                           "test container that uses firelens log driver with default mode",
			task:                           getTask(logDriverTypeFirelens, ""),
			expectedLogConfigType:          logDriverTypeFluentd,
			expectedLogConfigTag:           taskName + "-firelens-" + taskID,
			expectedFluentdAsyncConnect:    strconv.FormatBool(true),
			expectedSubSecondPrecision:     strconv.FormatBool(true),
			expectedBufferLimit:            "10000",
			expectedLogConfigFluentAddress: socketPathPrefix + filepath.Join(defaultConfig.DataDirOnHost, dataLogDriverPath, taskID, dataLogDriverSocketPath),
			expectedIPAddress:              envVarBridgeMode,
			expectedPort:                   envVarPort,
		},
		{
			name:                           "test container that uses firelens log driver with bridge mode",
			task:                           getTask(logDriverTypeFirelens, networkModeBridge),
			expectedLogConfigType:          logDriverTypeFluentd,
			expectedLogConfigTag:           taskName + "-firelens-" + taskID,
			expectedFluentdAsyncConnect:    strconv.FormatBool(true),
			expectedSubSecondPrecision:     strconv.FormatBool(true),
			expectedBufferLimit:            "10000",
			expectedLogConfigFluentAddress: socketPathPrefix + filepath.Join(defaultConfig.DataDirOnHost, dataLogDriverPath, taskID, dataLogDriverSocketPath),
			expectedIPAddress:              envVarBridgeMode,
			expectedPort:                   envVarPort,
		},
		{
			name:                           "test container that uses firelens log driver with awsvpc mode",
			task:                           getTaskWithENI(logDriverTypeFirelens, networkModeAWSVPC),
			expectedLogConfigType:          logDriverTypeFluentd,
			expectedLogConfigTag:           taskName + "-firelens-" + taskID,
			expectedFluentdAsyncConnect:    strconv.FormatBool(true),
			expectedSubSecondPrecision:     strconv.FormatBool(true),
			expectedBufferLimit:            "",
			expectedLogConfigFluentAddress: socketPathPrefix + filepath.Join(defaultConfig.DataDirOnHost, dataLogDriverPath, taskID, dataLogDriverSocketPath),
			expectedIPAddress:              envVarAWSVPCMode,
			expectedPort:                   envVarPort,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
			defer ctrl.Finish()

			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(ctx context.Context,
					config *dockercontainer.Config,
					hostConfig *dockercontainer.HostConfig,
					name string,
					timeout time.Duration) {
					assert.Equal(t, tc.expectedLogConfigType, hostConfig.LogConfig.Type)
					assert.Equal(t, tc.expectedLogConfigTag, hostConfig.LogConfig.Config["tag"])
					assert.Equal(t, tc.expectedLogConfigFluentAddress, hostConfig.LogConfig.Config["fluentd-address"])
					assert.Equal(t, tc.expectedFluentdAsyncConnect, hostConfig.LogConfig.Config["fluentd-async-connect"])
					assert.Equal(t, tc.expectedSubSecondPrecision, hostConfig.LogConfig.Config["fluentd-sub-second-precision"])
					assert.Equal(t, tc.expectedBufferLimit, hostConfig.LogConfig.Config["fluentd-buffer-limit"])
					assert.Contains(t, config.Env, tc.expectedIPAddress)
					assert.Contains(t, config.Env, tc.expectedPort)
				})
			ret := taskEngine.(*DockerTaskEngine).createContainer(tc.task, tc.task.Containers[0])
			assert.NoError(t, ret.Error)
		})

	}
}

func TestCreateFirelensContainerSetFluentdUID(t *testing.T) {
	testTask := &apitask.Task{
		Arn: "arn:aws:ecs:region:account-id:task/test-task-arn",
		Containers: []*apicontainer.Container{
			{
				Name: "test-container",
				FirelensConfig: &apicontainer.FirelensConfig{
					Type: "fluentd",
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
	client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context,
			config *dockercontainer.Config,
			hostConfig *dockercontainer.HostConfig,
			name string,
			timeout time.Duration) {
			assert.Contains(t, config.Env, "FLUENT_UID=0")
		})
	ret := taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
	assert.NoError(t, ret.Error)
}

func TestGetBridgeIP(t *testing.T) {
	networkDefaultIP := "defaultIP"
	getNetwork := func(defaultIP string, bridgeIP string, networkMode string) *types.NetworkSettings {
		endPoint := network.EndpointSettings{
			IPAddress: bridgeIP,
		}
		return &types.NetworkSettings{
			DefaultNetworkSettings: types.DefaultNetworkSettings{
				IPAddress: defaultIP,
			},
			Networks: map[string]*network.EndpointSettings{
				networkMode: &endPoint,
			},
		}
	}
	testCases := []struct {
		defaultIP         string
		bridgeIP          string
		networkMode       string
		expectedOk        bool
		expectedIPAddress string
	}{
		{
			defaultIP:         networkDefaultIP,
			bridgeIP:          networkBridgeIP,
			networkMode:       networkModeBridge,
			expectedOk:        true,
			expectedIPAddress: networkDefaultIP,
		},
		{
			defaultIP:         "",
			bridgeIP:          networkBridgeIP,
			networkMode:       networkModeBridge,
			expectedOk:        true,
			expectedIPAddress: networkBridgeIP,
		},
		{
			defaultIP:         "",
			bridgeIP:          networkBridgeIP,
			networkMode:       networkModeAWSVPC,
			expectedOk:        false,
			expectedIPAddress: "",
		},
		{
			defaultIP:         "",
			bridgeIP:          "",
			networkMode:       networkModeBridge,
			expectedOk:        false,
			expectedIPAddress: "",
		},
	}

	for _, tc := range testCases {
		IPAddress, ok := getContainerHostIP(getNetwork(tc.defaultIP, tc.bridgeIP, tc.networkMode))
		assert.Equal(t, tc.expectedOk, ok)
		assert.Equal(t, tc.expectedIPAddress, IPAddress)
	}
}

func TestStartFirelensContainerRetryForContainerIP(t *testing.T) {
	dockerMetaDataWithoutNetworkSettings := dockerapi.DockerContainerMetadata{
		DockerID: containerID,
		Volumes: []types.MountPoint{
			{
				Name:        "volume",
				Source:      "/src/vol",
				Destination: "/vol",
			},
		},
	}
	rawHostConfigInput := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
			Type: "fluentd",
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}
	jsonBaseWithoutNetwork := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    containerID,
			State: &types.ContainerState{Pid: containerPid},
			HostConfig: &dockercontainer.HostConfig{
				NetworkMode: containerNetworkMode,
			},
		},
	}

	jsonBaseWithNetwork := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    containerID,
			State: &types.ContainerState{Pid: containerPid},
			HostConfig: &dockercontainer.HostConfig{
				NetworkMode: containerNetworkMode,
			},
		},
		NetworkSettings: &types.NetworkSettings{
			DefaultNetworkSettings: types.DefaultNetworkSettings{
				IPAddress: networkBridgeIP,
			},
			Networks: map[string]*network.EndpointSettings{
				apitask.BridgeNetworkMode: &network.EndpointSettings{
					IPAddress: networkBridgeIP,
				},
			},
		},
	}
	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	require.NoError(t, err)
	testTask := &apitask.Task{
		Arn:     "arn:aws:ecs:region:account-id:task/task-id",
		Version: "1",
		Family:  "logSenderTaskFamily",
		Containers: []*apicontainer.Container{
			{
				Name: "logSenderTask",
				DockerConfig: apicontainer.DockerConfig{
					HostConfig: func() *string {
						s := string(rawHostConfig)
						return &s
					}(),
				},
				NetworkModeUnsafe: apitask.BridgeNetworkMode,
			},
			{
				Name: "test-container",
				FirelensConfig: &apicontainer.FirelensConfig{
					Type: "fluentd",
				},
				NetworkModeUnsafe: apitask.BridgeNetworkMode,
			},
		},
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()
	taskEngine.(*DockerTaskEngine).state.AddTask(testTask)
	taskEngine.(*DockerTaskEngine).state.AddContainer(&apicontainer.DockerContainer{
		Container:  testTask.Containers[1],
		DockerName: dockerContainerName,
		DockerID:   containerID,
	}, testTask)

	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
	client.EXPECT().StartContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(dockerMetaDataWithoutNetworkSettings).AnyTimes()
	gomock.InOrder(
		client.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).
			Return(jsonBaseWithoutNetwork, nil),
		client.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).
			Return(jsonBaseWithoutNetwork, nil),
		client.EXPECT().InspectContainer(gomock.Any(), containerID, gomock.Any()).
			Return(jsonBaseWithNetwork, nil),
	)
	ret := taskEngine.(*DockerTaskEngine).startContainer(testTask, testTask.Containers[1])
	assert.NoError(t, ret.Error)
	assert.Equal(t, jsonBaseWithNetwork.NetworkSettings, ret.NetworkSettings)
}

func TestStartExecAgent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	nowTime := time.Now()
	ctrl, client, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	execCmdMgr := mock_execcmdagent.NewMockManager(ctrl)
	dockerTaskEngine.execCmdMgr = execCmdMgr
	defer ctrl.Finish()
	const (
		testContainerId = "123"
	)
	testCases := []struct {
		execCommandAgentEnabled bool
		expectContainerEvent    bool
		execAgentStatus         apicontainerstatus.ManagedAgentStatus
		execAgentInitFailed     bool
		execAgentStartError     error
	}{
		{
			execCommandAgentEnabled: false,
			expectContainerEvent:    false,
			execAgentStatus:         apicontainerstatus.ManagedAgentStopped,
			execAgentInitFailed:     false,
		},
		{
			execCommandAgentEnabled: true,
			expectContainerEvent:    true,
			execAgentStatus:         apicontainerstatus.ManagedAgentRunning,
			execAgentInitFailed:     false,
		},
		{
			execCommandAgentEnabled: true,
			expectContainerEvent:    true,
			execAgentStatus:         apicontainerstatus.ManagedAgentStopped,
			execAgentStartError:     errors.New("mock error"),
		},
		{
			execCommandAgentEnabled: true,
			expectContainerEvent:    false,
			execAgentStatus:         apicontainerstatus.ManagedAgentStopped,
			execAgentInitFailed:     true,
		},
	}
	for _, tc := range testCases {
		stateChangeEvents := taskEngine.StateChangeEvents()
		testTask := &apitask.Task{
			Arn: "arn:aws:ecs:region:account-id:task/test-task-arn",
			Containers: []*apicontainer.Container{
				{
					Name:              "test-container",
					RuntimeID:         testContainerId,
					KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
				},
			},
		}

		if tc.execCommandAgentEnabled {
			enableExecCommandAgentForContainer(testTask.Containers[0], apicontainer.ManagedAgentState{
				LastStartedAt: nowTime,
				Status:        tc.execAgentStatus,
				InitFailed:    tc.execAgentInitFailed,
			})
		}
		mTestTask := &managedTask{
			Task:              testTask,
			engine:            dockerTaskEngine,
			ctx:               ctx,
			stateChangeEvents: stateChangeEvents,
		}

		dockerTaskEngine.state.AddTask(testTask)
		dockerTaskEngine.managedTasks[testTask.Arn] = mTestTask

		// check for expected taskEvent in stateChangeEvents
		waitDone := make(chan struct{})
		var reason string
		if tc.expectContainerEvent {
			reason = "ExecuteCommandAgent started"
		}
		if tc.execAgentStartError != nil {
			reason = tc.execAgentStartError.Error()
		}
		expectedManagedAgent := apicontainer.ManagedAgent{
			ManagedAgentState: apicontainer.ManagedAgentState{
				Status:     tc.execAgentStatus,
				InitFailed: tc.execAgentInitFailed,
				Reason:     reason,
			},
		}
		go checkManagedAgentEvents(t, tc.expectContainerEvent, stateChangeEvents, expectedManagedAgent, waitDone)

		client.EXPECT().StartContainer(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			dockerapi.DockerContainerMetadata{DockerID: containerID}).AnyTimes()
		if tc.execCommandAgentEnabled {
			execCmdMgr.EXPECT().InitializeContainer(gomock.Any(), testTask.Containers[0], gomock.Any()).AnyTimes()
			if !tc.execAgentInitFailed {
				execCmdMgr.EXPECT().StartAgent(gomock.Any(), client, testTask, testTask.Containers[0], testContainerId).
					Return(tc.execAgentStartError).
					AnyTimes()
			}
		}
		ret := taskEngine.(*DockerTaskEngine).startContainer(testTask, testTask.Containers[0])
		assert.NoError(t, ret.Error)

		timeout := false
		select {
		case <-waitDone:
		case <-time.After(time.Second):
			timeout = true
		}
		assert.False(t, timeout)
	}
}

func TestMonitorExecAgentRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	execCmdMgr := mock_execcmdagent.NewMockManager(ctrl)
	dockerTaskEngine.execCmdMgr = execCmdMgr
	dockerTaskEngine.monitorExecAgentsInterval = 2 * time.Millisecond
	defer ctrl.Finish()
	const (
		testContainerId = "123"
	)
	testCases := []struct {
		containerStatus                apicontainerstatus.ContainerStatus
		execCommandAgentState          apicontainer.ManagedAgentState
		execAgentStatus                apicontainerstatus.ManagedAgentStatus
		restartStatus                  execcmd.RestartStatus
		simulateBadContainerId         bool
		expectedRestartInUnhealthyCall bool
		expectContainerEvent           bool
	}{
		{
			containerStatus:      apicontainerstatus.ContainerStopped,
			execAgentStatus:      apicontainerstatus.ManagedAgentStopped,
			restartStatus:        execcmd.NotRestarted,
			expectContainerEvent: false,
		},
		{
			containerStatus:        apicontainerstatus.ContainerRunning,
			simulateBadContainerId: true,
			execAgentStatus:        apicontainerstatus.ManagedAgentStopped,
			restartStatus:          execcmd.NotRestarted,
			expectContainerEvent:   false,
		},
		{
			containerStatus:      apicontainerstatus.ContainerRunning,
			execAgentStatus:      apicontainerstatus.ManagedAgentRunning,
			restartStatus:        execcmd.NotRestarted,
			expectContainerEvent: false,
		},
		{
			containerStatus:      apicontainerstatus.ContainerRunning,
			execAgentStatus:      apicontainerstatus.ManagedAgentRunning,
			restartStatus:        execcmd.Restarted,
			expectContainerEvent: true,
		},
	}
	for _, tc := range testCases {
		nowTime := time.Now()
		stateChangeEvents := taskEngine.StateChangeEvents()
		testTask := &apitask.Task{
			Arn: "arn:aws:ecs:region:account-id:task/test-task-arn",
			Containers: []*apicontainer.Container{
				{
					Name:              "test-container",
					RuntimeID:         testContainerId,
					KnownStatusUnsafe: tc.containerStatus,
				},
			},
		}

		enableExecCommandAgentForContainer(testTask.Containers[0], apicontainer.ManagedAgentState{
			LastStartedAt: nowTime,
			Status:        tc.execAgentStatus,
		})

		mTestTask := &managedTask{
			Task:              testTask,
			engine:            dockerTaskEngine,
			ctx:               ctx,
			stateChangeEvents: stateChangeEvents,
		}

		dockerTaskEngine.state.AddTask(testTask)

		if tc.simulateBadContainerId {
			testTask.Containers[0].RuntimeID = ""
		}
		if tc.containerStatus == apicontainerstatus.ContainerRunning && !tc.simulateBadContainerId {
			execCmdMgr.EXPECT().RestartAgentIfStopped(dockerTaskEngine.ctx, dockerTaskEngine.client, testTask,
				testTask.Containers[0], testContainerId).
				Return(tc.restartStatus, nil).
				Times(1)
		}

		// check for expected containerEvent in stateChangeEvents
		waitDone := make(chan struct{})
		expectedManagedAgent := apicontainer.ManagedAgent{
			ManagedAgentState: apicontainer.ManagedAgentState{
				Status: apicontainerstatus.ManagedAgentRunning,
				Reason: "ExecuteCommandAgent restarted",
			},
		}
		// only if we expect restart will we also expect a managed agent container event
		go checkManagedAgentEvents(t, tc.expectContainerEvent, stateChangeEvents, expectedManagedAgent, waitDone)

		taskEngine.(*DockerTaskEngine).monitorExecAgentRunning(ctx, mTestTask, testTask.Containers[0])

		timeout := false
		select {
		case <-waitDone:
		case <-time.After(time.Second):
			timeout = true
		}
		assert.False(t, timeout)
	}
}

func TestMonitorExecAgentProcesses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	nowTime := time.Now()
	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	execCmdMgr := mock_execcmdagent.NewMockManager(ctrl)
	dockerTaskEngine.execCmdMgr = execCmdMgr
	dockerTaskEngine.monitorExecAgentsInterval = 2 * time.Millisecond
	defer ctrl.Finish()

	testCases := []struct {
		execAgentStatus      apicontainerstatus.ManagedAgentStatus
		expectContainerEvent bool
		execAgentInitfailed  bool
	}{
		{
			execAgentStatus:      apicontainerstatus.ManagedAgentRunning,
			expectContainerEvent: true,
			execAgentInitfailed:  false,
		},
		{
			execAgentStatus:      apicontainerstatus.ManagedAgentStopped,
			expectContainerEvent: false,
			execAgentInitfailed:  true,
		},
	}
	for _, tc := range testCases {
		stateChangeEvents := taskEngine.StateChangeEvents()
		testTask := &apitask.Task{
			Arn: "arn:aws:ecs:region:account-id:task/test-task-arn",
			Containers: []*apicontainer.Container{
				{
					Name:              "test-container",
					RuntimeID:         "runtime-ID",
					KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
				},
			},
			KnownStatusUnsafe: apitaskstatus.TaskRunning,
		}
		enableExecCommandAgentForContainer(testTask.Containers[0], apicontainer.ManagedAgentState{
			LastStartedAt: nowTime,
			Status:        apicontainerstatus.ManagedAgentRunning,
			InitFailed:    tc.execAgentInitfailed,
		})
		mTestTask := &managedTask{
			Task:              testTask,
			engine:            dockerTaskEngine,
			ctx:               ctx,
			stateChangeEvents: stateChangeEvents,
		}
		dockerTaskEngine.state.AddTask(testTask)
		dockerTaskEngine.managedTasks[testTask.Arn] = mTestTask
		restartCtx, restartCancel := context.WithTimeout(context.Background(), time.Second)
		defer restartCancel()
		// return execcmd.Restarted to ensure container event emission

		if !tc.execAgentInitfailed {
			execCmdMgr.EXPECT().RestartAgentIfStopped(dockerTaskEngine.ctx, dockerTaskEngine.client, testTask, testTask.Containers[0], testTask.Containers[0].RuntimeID).
				DoAndReturn(
					func(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) (execcmd.RestartStatus, error) {
						defer restartCancel()
						return execcmd.Restarted, nil
					}).
				Times(1)
		}

		expectContainerEvent := tc.expectContainerEvent
		waitDone := make(chan struct{})
		expectedManagedAgent := apicontainer.ManagedAgent{
			Name: execcmd.ExecuteCommandAgentName,
			ManagedAgentState: apicontainer.ManagedAgentState{
				Status:        tc.execAgentStatus,
				Reason:        "ExecuteCommandAgent restarted",
				LastStartedAt: nowTime,
			},
		}

		go checkManagedAgentEvents(t, expectContainerEvent, stateChangeEvents, expectedManagedAgent, waitDone)

		dockerTaskEngine.monitorExecAgentProcesses(dockerTaskEngine.ctx)
		<-restartCtx.Done()
		time.Sleep(5 * time.Millisecond)

		timeout := false
		select {
		case <-waitDone:
		case <-time.After(time.Second):
			timeout = true
		}

		assert.False(t, timeout)
	}
}

func TestMonitorExecAgentProcessExecDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	execCmdMgr := mock_execcmdagent.NewMockManager(ctrl)
	dockerTaskEngine.execCmdMgr = execCmdMgr
	defer ctrl.Finish()
	tt := []struct {
		execCommandAgentEnabled bool
		taskStatus              apitaskstatus.TaskStatus
	}{
		{
			execCommandAgentEnabled: false,
			taskStatus:              apitaskstatus.TaskRunning,
		},
		{
			execCommandAgentEnabled: true,
			taskStatus:              apitaskstatus.TaskStopped,
		},
	}
	for _, test := range tt {
		testTask := &apitask.Task{
			Arn: "arn:aws:ecs:region:account-id:task/test-task-arn",
			Containers: []*apicontainer.Container{
				{
					Name:              "test-container",
					RuntimeID:         "runtime-ID",
					KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
				},
			},
			KnownStatusUnsafe: test.taskStatus,
		}
		if test.execCommandAgentEnabled {
			enableExecCommandAgentForContainer(testTask.Containers[0], apicontainer.ManagedAgentState{})
		}
		dockerTaskEngine.state.AddTask(testTask)
		dockerTaskEngine.managedTasks[testTask.Arn] = &managedTask{Task: testTask}
		dockerTaskEngine.monitorExecAgentProcesses(ctx)
		// absence of top container expect call indicates it shouldn't have been called
		time.Sleep(10 * time.Millisecond)
	}
}
func TestMonitorExecAgentsMultipleContainers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	execCmdMgr := mock_execcmdagent.NewMockManager(ctrl)
	dockerTaskEngine.execCmdMgr = execCmdMgr
	dockerTaskEngine.monitorExecAgentsInterval = 2 * time.Millisecond
	defer ctrl.Finish()
	stateChangeEvents := taskEngine.StateChangeEvents()

	testTask := &apitask.Task{
		Arn: "arn:aws:ecs:region:account-id:task/test-task-arn",
		Containers: []*apicontainer.Container{
			{
				Name:              "test-container1",
				RuntimeID:         "runtime-ID1",
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
			{
				Name:              "test-container2",
				RuntimeID:         "runtime-ID2",
				KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
	}

	for _, c := range testTask.Containers {
		enableExecCommandAgentForContainer(c, apicontainer.ManagedAgentState{})
	}

	mTestTask := &managedTask{
		Task:              testTask,
		engine:            dockerTaskEngine,
		ctx:               ctx,
		stateChangeEvents: stateChangeEvents,
	}

	dockerTaskEngine.state.AddTask(testTask)
	dockerTaskEngine.managedTasks[testTask.Arn] = mTestTask
	wg := &sync.WaitGroup{}
	numContainers := len(testTask.Containers)
	wg.Add(numContainers)

	for i := 0; i < numContainers; i++ {
		execCmdMgr.EXPECT().RestartAgentIfStopped(dockerTaskEngine.ctx, dockerTaskEngine.client, testTask, testTask.Containers[i], testTask.Containers[i].RuntimeID).
			DoAndReturn(
				func(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) (execcmd.RestartStatus, error) {
					defer wg.Done()
					defer discardEvents(stateChangeEvents)()
					return execcmd.NotRestarted, nil
				}).
			Times(1)

	}
	taskEngine.(*DockerTaskEngine).monitorExecAgentProcesses(dockerTaskEngine.ctx)

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	timeout := false
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		timeout = true
	}
	assert.False(t, timeout)

}

func TestPeriodicExecAgentsMonitoring(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, taskEngine, _, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()
	execAgentPID := "1234"
	testTask := &apitask.Task{
		Arn: "arn:aws:ecs:region:account-id:task/test-task-arn",
		Containers: []*apicontainer.Container{
			{
				Name:      "test-container",
				RuntimeID: "runtime-ID",
			},
		},
	}
	enableExecCommandAgentForContainer(testTask.Containers[0], apicontainer.ManagedAgentState{
		Metadata: map[string]interface{}{
			"PID": execAgentPID,
		}})
	taskEngine.(*DockerTaskEngine).monitorExecAgentsInterval = 2 * time.Millisecond
	taskEngine.(*DockerTaskEngine).state.AddTask(testTask)
	taskEngine.(*DockerTaskEngine).managedTasks[testTask.Arn] = &managedTask{Task: testTask}
	topCtx, topCancel := context.WithTimeout(context.Background(), time.Second)
	defer topCancel()
	go taskEngine.(*DockerTaskEngine).startPeriodicExecAgentsMonitoring(ctx)
	<-topCtx.Done()
	time.Sleep(5 * time.Millisecond)
	execCmdAgent, ok := testTask.Containers[0].GetManagedAgentByName(execcmd.ExecuteCommandAgentName)
	assert.True(t, ok)
	execMD := execcmd.MapToAgentMetadata(execCmdAgent.Metadata)
	assert.Equal(t, execAgentPID, execMD.PID)
}

func TestCreateContainerWithExecAgent(t *testing.T) {
	testcases := []struct {
		name                 string
		error                error
		expectContainerEvent bool
		execAgentInitFailed  bool
		execAgentStatus      apicontainerstatus.ManagedAgentStatus
	}{
		{
			name:                 "ExecAgent config mount success",
			error:                nil,
			expectContainerEvent: false,
			execAgentInitFailed:  false,
		},
		{
			name:                 "ExecAgent config mount Error",
			error:                errors.New("mount error"),
			expectContainerEvent: true,
			execAgentInitFailed:  true,
			execAgentStatus:      apicontainerstatus.ManagedAgentStopped,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, client, _, engine, _, _, _, _ := mocks(t, ctx, &config.Config{})
			defer ctrl.Finish()
			taskEngine, _ := engine.(*DockerTaskEngine)
			stateChangeEvents := engine.StateChangeEvents()
			execCmdMgr := mock_execcmdagent.NewMockManager(ctrl)
			taskEngine.execCmdMgr = execCmdMgr
			sleepTask := testdata.LoadTask("sleep5")
			sleepContainer, _ := sleepTask.ContainerByName("sleep5")
			enableExecCommandAgentForContainer(sleepContainer, apicontainer.ManagedAgentState{
				Status:     tc.execAgentStatus,
				InitFailed: tc.execAgentInitFailed,
			})

			mTestTask := &managedTask{
				Task:              sleepTask,
				engine:            taskEngine,
				ctx:               ctx,
				stateChangeEvents: stateChangeEvents,
			}

			taskEngine.state.AddTask(sleepTask)
			taskEngine.managedTasks[sleepTask.Arn] = mTestTask

			waitDone := make(chan struct{})
			var reason string
			if tc.error != nil {
				reason = fmt.Sprintf("ExecuteCommandAgent Initialization failed - %v", tc.error)
			}
			expectedManagedAgent := apicontainer.ManagedAgent{
				ManagedAgentState: apicontainer.ManagedAgentState{
					Status: apicontainerstatus.ManagedAgentStopped,
					Reason: reason,
				},
			}

			go checkManagedAgentEvents(t, tc.expectContainerEvent, stateChangeEvents, expectedManagedAgent, waitDone)
			execCmdMgr.EXPECT().InitializeContainer(gomock.Any(), sleepContainer, gomock.Any()).Return(tc.error)
			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil)
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
			metadata := taskEngine.createContainer(sleepTask, sleepContainer)
			assert.NoError(t, metadata.Error)

			timeout := false
			select {
			case <-waitDone:
			case <-time.After(time.Second):
				timeout = true
			}
			assert.False(t, timeout)
		})
	}
}
