// +build !integration
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
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata/mocks"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/ecscni/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/resources/mock_resources"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/containernetworking/cni/pkg/types/current"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"golang.org/x/net/context"
)

const (
	credentialsID       = "credsid"
	ipv4                = "10.0.0.1"
	mac                 = "1.2.3.4"
	ipv6                = "f0:234:23"
	dockerContainerName = "docker-container-name"
	containerPid        = 123
	taskIP              = "169.254.170.3"
	exitCode            = 1
	labelsTaskARN       = "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe"
)

var (
	defaultConfig config.Config
	nsResult      = mockSetupNSResult()

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
	defaultConfig.TaskCPUMemLimit = config.ExplicitlyDisabled
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
	*MockDockerClient, *mock_ttime.MockTime, TaskEngine,
	*mock_credentials.MockManager, *MockImageManager, *mock_containermetadata.MockManager) {
	ctrl := gomock.NewController(t)
	client := NewMockDockerClient(ctrl)
	mockTime := mock_ttime.NewMockTime(ctrl)
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	containerChangeEventStream := eventstream.NewEventStream("TESTTASKENGINE", ctx)
	containerChangeEventStream.StartListening()
	imageManager := NewMockImageManager(ctrl)
	metadataManager := mock_containermetadata.NewMockManager(ctrl)
	mockResource := mock_resources.NewMockResource(ctrl)

	taskEngine := NewTaskEngine(cfg, client, credentialsManager, containerChangeEventStream,
		imageManager, dockerstate.NewTaskEngineState(), metadataManager, mockResource)
	taskEngine.(*DockerTaskEngine)._time = mockTime

	return ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, metadataManager
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
		name                string
		metadataCreateError error
		metadataUpdateError error
		metadataCleanError  error
		taskCPULimit        config.Conditional
	}{
		{
			name:                "Metadata Manager Succeeds",
			metadataCreateError: nil,
			metadataUpdateError: nil,
			metadataCleanError:  nil,
			taskCPULimit:        config.ExplicitlyDisabled,
		},
		{
			name:                "Metadata Manager Fails to Create, Update and Cleanup",
			metadataCreateError: errors.New("create metadata error"),
			metadataUpdateError: errors.New("update metadata error"),
			metadataCleanError:  errors.New("clean metadata error"),
			taskCPULimit:        config.ExplicitlyDisabled,
		},
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
			sleepTask.SetCredentialsID(credentialsID)
			eventStream := make(chan DockerContainerChangeEvent)
			// containerEventsWG is used to force the test to wait until the container created and started
			// events are processed
			containerEventsWG := sync.WaitGroup{}

			client.EXPECT().Version().Return("1.12.6", nil)
			client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
			containerName := make(chan string)
			go func() {
				name := <-containerName
				setCreatedContainerName(name)
			}()
			mockResource := mock_resources.NewMockResource(ctrl)
			if tc.taskCPULimit.Enabled() {
				taskEngine.(*DockerTaskEngine).resource = mockResource
				// TODO Currently, the resource Setup() method gets invoked multiple
				// times for a task. This is really a bug and a fortunate occurrence
				// that cgroup creation APIs behave idempotently.
				//
				// This should be modified so that 'Setup' is invoked exactly once
				// by moving the cgroup creation to a "resource setup" step in the
				// task life-cycle and performing the setup only in this stage
				mockResource.EXPECT().Setup(sleepTask).Return(nil).MinTimes(1)
			}

			for _, container := range sleepTask.Containers {
				validateContainerRunWorkflow(t, container, sleepTask, imageManager,
					client, &roleCredentials, containerEventsWG,
					eventStream, containerName, func() {
						metadataManager.EXPECT().Create(gomock.Any(), gomock.Any(),
							gomock.Any(), gomock.Any()).Return(tc.metadataCreateError)
						metadataManager.EXPECT().Update(gomock.Any(), gomock.Any(),
							gomock.Any()).Return(tc.metadataUpdateError)
					})
			}

			addTaskToEngine(t, ctx, taskEngine, sleepTask, mockTime, containerEventsWG)
			cleanup := make(chan time.Time, 1)
			defer close(cleanup)
			mockTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
			client.EXPECT().DescribeContainer(gomock.Any()).AnyTimes()
			// Simulate a container stop event from docker
			eventStream <- DockerContainerChangeEvent{
				Status: api.ContainerStopped,
				DockerContainerMetadata: DockerContainerMetadata{
					DockerID: containerID,
					ExitCode: aws.Int(exitCode),
				},
			}

			// StopContainer might be invoked if the test execution is slow, during
			// the cleanup phase. Account for that.
			client.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(
				DockerContainerMetadata{DockerID: containerID}).AnyTimes()
			waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
			// This ensures that managedTask.waitForStopReported makes progress
			sleepTask.SetSentStatus(api.TaskStopped)
			// Extra events should not block forever; duplicate acs and docker events are possible
			go func() { eventStream <- createDockerEvent(api.ContainerStopped) }()
			go func() { eventStream <- createDockerEvent(api.ContainerStopped) }()

			sleepTaskStop := testdata.LoadTask("sleep5")
			sleepTaskStop.SetCredentialsID(credentialsID)
			sleepTaskStop.SetDesiredStatus(api.TaskStopped)
			taskEngine.AddTask(sleepTaskStop)
			// As above, duplicate events should not be a problem
			taskEngine.AddTask(sleepTaskStop)
			taskEngine.AddTask(sleepTaskStop)

			if tc.taskCPULimit.Enabled() {
				mockResource.EXPECT().Cleanup(sleepTask).Return(nil)
			}
			// Expect a bunch of steady state 'poll' describes when we trigger cleanup
			client.EXPECT().RemoveContainer(gomock.Any(), gomock.Any()).Do(
				func(removedContainerName string, timeout time.Duration) {
					assert.Equal(t, getCreatedContainerName(), removedContainerName,
						"Container name mismatch")
				}).Return(nil)

			imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any())
			metadataManager.EXPECT().Clean(gomock.Any()).Return(tc.metadataCleanError)
			// trigger cleanup
			cleanup <- time.Now()
			go func() { eventStream <- createDockerEvent(api.ContainerStopped) }()
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

// TestTaskWithSteadyStateResourcesProvisioned tests container and task transitions
// when the steady state for the pause container is set to RESOURCES_PROVISIONED and
// the steady state for the normal container is set to RUNNING
func TestTaskWithSteadyStateResourcesProvisioned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	// sleep5 contains a single 'sleep' container, with DesiredStatus == RUNNING
	sleepTask := testdata.LoadTask("sleep5")
	sleepTask.Containers[0].TransitionDependencySet.ContainerDependencies = []api.ContainerDependency{
		{
			ContainerName:   "pause",
			SatisfiedStatus: api.ContainerRunning,
			DependentStatus: api.ContainerPulled,
		}}
	sleepContainer := sleepTask.Containers[0]
	// Add a second container with DesiredStatus == RESOURCES_PROVISIONED and
	// steadyState == RESOURCES_PROVISIONED
	pauseContainer := api.NewContainerWithSteadyState(api.ContainerResourcesProvisioned)
	pauseContainer.Name = "pause"
	pauseContainer.Image = "pause"
	pauseContainer.CPU = 10
	pauseContainer.Memory = 10
	pauseContainer.Essential = true
	pauseContainer.Type = api.ContainerCNIPause
	pauseContainer.DesiredStatusUnsafe = api.ContainerRunning
	sleepTask.Containers = append(sleepTask.Containers, pauseContainer)
	eventStream := make(chan DockerContainerChangeEvent)
	// containerEventsWG is used to force the test to wait until the container created and started
	// events are processed
	containerEventsWG := sync.WaitGroup{}

	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	// We cannot rely on the order of pulls between images as they can still be downloaded in
	// parallel. The dependency graph enforcement comes into effect for CREATED transitions.
	// Hence, do not enforce the order of invocation of these calls
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	client.EXPECT().PullImage(sleepContainer.Image, nil).Return(DockerContainerMetadata{})
	imageManager.EXPECT().RecordContainerReference(sleepContainer).Return(nil)
	imageManager.EXPECT().GetImageStateFromImageName(sleepContainer.Image).Return(nil)

	gomock.InOrder(
		// Ensure that the pause container is created first
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(config *docker.Config, hostConfig *docker.HostConfig, containerName string, z time.Duration) {
				sleepTask.SetTaskENI(&api.ENI{
					ID: "TestTaskWithSteadyStateResourcesProvisioned",
					IPV4Addresses: []*api.ENIIPV4Address{
						{
							Primary: true,
							Address: ipv4,
						},
					},
					MacAddress: mac,
					IPV6Addresses: []*api.ENIIPV6Address{
						{
							Address: ipv6,
						},
					},
				})
				assert.Equal(t, "none", hostConfig.NetworkMode)
				assert.True(t, strings.Contains(containerName, pauseContainer.Name))
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerCreated)
					containerEventsWG.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: containerID + ":" + pauseContainer.Name}),
		// Ensure that the pause container is started after it's created
		client.EXPECT().StartContainer(containerID+":"+pauseContainer.Name, startContainerTimeout).Do(
			func(id string, timeout time.Duration) {
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerRunning)
					containerEventsWG.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: containerID + ":" + pauseContainer.Name}),
		client.EXPECT().InspectContainer(gomock.Any(), gomock.Any()).Return(&docker.Container{
			ID:    containerID,
			State: docker.State{Pid: 23},
		}, nil),
		// Then setting up the pause container network namespace
		mockCNIClient.EXPECT().SetupNS(gomock.Any()).Return(nsResult, nil),

		// Once the pause container is started, sleep container will be created
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(config *docker.Config, hostConfig *docker.HostConfig, containerName string, z time.Duration) {
				assert.True(t, strings.Contains(containerName, sleepContainer.Name))
				assert.Equal(t, "container:"+containerID+":"+pauseContainer.Name, hostConfig.NetworkMode)
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerCreated)
					containerEventsWG.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: containerID + ":" + sleepContainer.Name}),
		// Next, the sleep container is started
		client.EXPECT().StartContainer(containerID+":"+sleepContainer.Name, startContainerTimeout).Do(
			func(id string, timeout time.Duration) {
				containerEventsWG.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerRunning)
					containerEventsWG.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: containerID + ":" + sleepContainer.Name}),
	)

	addTaskToEngine(t, ctx, taskEngine, sleepTask, mockTime, containerEventsWG)
	taskARNByIP, ok := taskEngine.(*DockerTaskEngine).state.GetTaskByIPAddress(taskIP)
	assert.True(t, ok)
	assert.Equal(t, sleepTask.Arn, taskARNByIP)
	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()
	client.EXPECT().InspectContainer(gomock.Any(), gomock.Any()).Return(&docker.Container{
		ID:    containerID,
		State: docker.State{Pid: 23},
	}, nil)
	mockCNIClient.EXPECT().CleanupNS(gomock.Any()).Return(nil)
	client.EXPECT().StopContainer(containerID+":"+pauseContainer.Name, gomock.Any()).MinTimes(1)
	mockCNIClient.EXPECT().ReleaseIPResource(gomock.Any()).Return(nil).MaxTimes(1)

	// Simulate a container stop event from docker
	eventStream <- DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: DockerContainerMetadata{
			DockerID: containerID + ":" + sleepContainer.Name,
			ExitCode: aws.Int(exitCode),
		},
	}
	waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
}

// TestRemoveEvents tests if the task engine can handle task events while the task is being
// cleaned up. This test ensures that there's no regression in the task engine and ensures
// there's no deadlock as seen in #313
func TestRemoveEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)
	// containerEventsWG is used to force the test to wait until the container created and started
	// events are processed
	containerEventsWG := sync.WaitGroup{}
	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	containerName := make(chan string)
	go func() {
		name := <-containerName
		setCreatedContainerName(name)
	}()

	for _, container := range sleepTask.Containers {
		validateContainerRunWorkflow(t, container, sleepTask, imageManager,
			client, nil, containerEventsWG,
			eventStream, containerName, func() {
			})
	}

	addTaskToEngine(t, ctx, taskEngine, sleepTask, mockTime, containerEventsWG)
	cleanup := make(chan time.Time, 1)
	defer close(cleanup)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
	client.EXPECT().DescribeContainer(gomock.Any()).AnyTimes()

	// Simulate a container stop event from docker
	eventStream <- DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: DockerContainerMetadata{
			DockerID: containerID,
			ExitCode: aws.Int(exitCode),
		},
	}

	waitForStopEvents(t, taskEngine.StateChangeEvents(), true)
	sleepTaskStop := testdata.LoadTask("sleep5")
	sleepTaskStop.SetDesiredStatus(api.TaskStopped)
	taskEngine.AddTask(sleepTaskStop)

	client.EXPECT().RemoveContainer(gomock.Any(), gomock.Any()).Do(
		func(removedContainerName string, timeout time.Duration) {
			assert.Equal(t, getCreatedContainerName(), removedContainerName,
				"Container name mismatch")

			// Emit a couple of events for the task before cleanup finishes. This forces
			// discardEventsUntil to be invoked and should test the code path that
			// caused the deadlock, which was fixed with #320
			eventStream <- createDockerEvent(api.ContainerStopped)
			eventStream <- createDockerEvent(api.ContainerStopped)
		}).Return(nil)

	client.EXPECT().StopContainer(gomock.Any(), gomock.Any()).AnyTimes()
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any())

	// This ensures that managedTask.waitForStopReported makes progress
	sleepTask.SetSentStatus(api.TaskStopped)
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
	ctrl, client, testTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)
	testTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	testTime.EXPECT().After(gomock.Any())
	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil)
	for _, container := range sleepTask.Containers {
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
		client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{})

		imageManager.EXPECT().RecordContainerReference(container)
		imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil)
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(x, y, z, timeout interface{}) {
				go func() { eventStream <- createDockerEvent(api.ContainerCreated) }()
			}).Return(DockerContainerMetadata{DockerID: containerID})

		client.EXPECT().StartContainer(containerID, startContainerTimeout).Return(DockerContainerMetadata{
			Error: &DockerTimeoutError{},
		})
	}

	// Start timeout triggers a container stop as we force stop containers
	// when startcontainer times out. See #1043 for details
	client.EXPECT().StopContainer(containerID, gomock.Any()).Return(DockerContainerMetadata{
		Error: CannotStartContainerError{fmt.Errorf("cannot start container")},
	}).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	stateChangeEvents := taskEngine.StateChangeEvents()
	taskEngine.AddTask(sleepTask)
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false)

	// Now surprise surprise, it actually did start!
	eventStream <- createDockerEvent(api.ContainerRunning)
	// However, if it starts again, we should not see it be killed; no additional expect
	eventStream <- createDockerEvent(api.ContainerRunning)
	eventStream <- createDockerEvent(api.ContainerRunning)

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}
}

func TestSteadyStatePoll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, testTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	containerEventsWG := sync.WaitGroup{}
	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)
	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	containerName := make(chan string)
	go func() {
		<-containerName
	}()

	// set up expectations for each container in the task calling create + start
	for _, container := range sleepTask.Containers {
		validateContainerRunWorkflow(t, container, sleepTask, imageManager,
			client, nil, containerEventsWG,
			eventStream, containerName, func() {
			})
	}

	steadyStateVerify := make(chan time.Time, 10) // channel to trigger a "steady state verify" action
	testTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	testTime.EXPECT().After(steadyStateTaskVerifyInterval).Return(steadyStateVerify).AnyTimes()

	err := taskEngine.Init(ctx) // start the task engine
	assert.NoError(t, err)
	taskEngine.AddTask(sleepTask) // actually add the task we created
	waitForRunningEvents(t, taskEngine.StateChangeEvents())
	containerMap, ok := taskEngine.(*DockerTaskEngine).State().ContainerMapByArn(sleepTask.Arn)
	assert.True(t, ok)
	dockerContainer, ok := containerMap[sleepTask.Containers[0].Name]
	assert.True(t, ok)

	// Two steady state oks, one stop
	gomock.InOrder(
		client.EXPECT().DescribeContainer(containerID).Return(
			api.ContainerRunning,
			DockerContainerMetadata{
				DockerID: containerID,
			}).Times(2),
		client.EXPECT().DescribeContainer(containerID).Return(
			api.ContainerStopped,
			DockerContainerMetadata{
				DockerID: containerID,
			}).MinTimes(1),
		// the engine *may* call StopContainer even though it's already stopped
		client.EXPECT().StopContainer(containerID, stopContainerTimeout).AnyTimes(),
	)
	// Wait for container create and start events to be processed
	containerEventsWG.Wait()

	cleanup := make(chan time.Time)
	defer close(cleanup)
	testTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
	client.EXPECT().RemoveContainer(dockerContainer.DockerName, removeContainerTimeout).Return(nil)
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any()).Return(nil)

	// trigger steady state verification
	for i := 0; i < 10; i++ {
		steadyStateVerify <- time.Now()
	}

	// StopContainer might be invoked if the test execution is slow, during
	// the cleanup phase. Account for that.
	client.EXPECT().StopContainer(gomock.Any(), gomock.Any()).Return(
		DockerContainerMetadata{DockerID: containerID}).AnyTimes()
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false)
	close(steadyStateVerify)
	// trigger cleanup, this ensures all the goroutines were finished
	sleepTask.SetSentStatus(api.TaskStopped)
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
	ctrl, client, testTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()
	testTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	testTime.EXPECT().After(gomock.Any()).AnyTimes()

	sleepTask1 := testdata.LoadTask("sleep5")
	sleepTask1.StartSequenceNumber = 5
	sleepTask2 := testdata.LoadTask("sleep5")
	sleepTask2.Arn = "arn2"
	eventStream := make(chan DockerContainerChangeEvent)

	client.EXPECT().Version().Return("1.7.0", nil)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	stateChangeEvents := taskEngine.StateChangeEvents()

	defer discardEvents(stateChangeEvents)()

	pullDone := make(chan bool)
	pullInvoked := make(chan bool)
	client.EXPECT().PullImage(gomock.Any(), nil).Do(func(x, y interface{}) {
		pullInvoked <- true
		<-pullDone
	}).MaxTimes(2)

	imageManager.EXPECT().RecordContainerReference(gomock.Any()).AnyTimes()
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).AnyTimes()

	taskEngine.AddTask(sleepTask2)
	<-pullInvoked
	stopSleep2 := testdata.LoadTask("sleep5")
	stopSleep2.Arn = "arn2"
	stopSleep2.SetDesiredStatus(api.TaskStopped)
	stopSleep2.StopSequenceNumber = 4
	taskEngine.AddTask(stopSleep2)

	taskEngine.AddTask(sleepTask1)
	stopSleep1 := testdata.LoadTask("sleep5")
	stopSleep1.SetDesiredStatus(api.TaskStopped)
	stopSleep1.StopSequenceNumber = 5
	taskEngine.AddTask(stopSleep1)
	pullDone <- true
	// this means the PullImage is only called once due to the task is stopped before it
	// gets the pull image lock
}

func TestCreateContainerForceSave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, _, _ := mocks(t, ctx, &config.Config{})
	saver := mock_statemanager.NewMockStateManager(ctrl)
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	taskEngine.SetSaver(saver)

	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer, _ := sleepTask.ContainerByName("sleep5")
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
	gomock.InOrder(
		saver.EXPECT().ForceSave().Do(func() interface{} {
			task, ok := taskEngine.state.TaskByArn(sleepTask.Arn)
			assert.True(t, ok, "Expected task with ARN: ", sleepTask.Arn)
			assert.NotNil(t, task, "Expected task with ARN: ", sleepTask.Arn)
			_, ok = task.ContainerByName("sleep5")
			assert.True(t, ok, "Expected container sleep5")
			return nil
		}),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()),
	)

	metadata := taskEngine.createContainer(sleepTask, sleepContainer)
	if metadata.Error != nil {
		t.Error("Unexpected error", metadata.Error)
	}
}

func TestCreateContainerMergesLabels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	testTask := &api.Task{
		Arn:     labelsTaskARN,
		Family:  "myFamily",
		Version: "1",
		Containers: []*api.Container{
			{
				Name: "c1",
				DockerConfig: api.DockerConfig{
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
		"key": "value",
	}
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil).AnyTimes()
	client.EXPECT().CreateContainer(expectedConfig, gomock.Any(), gomock.Any(), gomock.Any())
	taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
}

// TestTaskTransitionWhenStopContainerTimesout tests that task transitions to stopped
// only when terminal events are received from docker event stream when
// StopContainer times out
func TestTaskTransitionWhenStopContainerTimesout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)
	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	containerStopTimeoutError := DockerContainerMetadata{
		Error: &DockerTimeoutError{
			transition: "stop",
			duration:   stopContainerTimeout,
		},
	}
	dockerEventSent := make(chan int)
	for _, container := range sleepTask.Containers {
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
		client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{})
		imageManager.EXPECT().RecordContainerReference(container)
		imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil)
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil)

		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(x, y, z, timeout interface{}) {
				go func() { eventStream <- createDockerEvent(api.ContainerCreated) }()
			}).Return(DockerContainerMetadata{DockerID: containerID})

		gomock.InOrder(
			client.EXPECT().StartContainer(containerID, startContainerTimeout).Do(
				func(id string, timeout time.Duration) {
					go func() {
						eventStream <- createDockerEvent(api.ContainerRunning)
					}()
				}).Return(DockerContainerMetadata{DockerID: containerID}),

			// StopContainer times out
			client.EXPECT().StopContainer(containerID, gomock.Any()).Return(containerStopTimeoutError),
			// Since task is not in steady state, progressContainers causes
			// another invocation of StopContainer. Return a timeout error
			// for that as well.
			client.EXPECT().StopContainer(containerID, gomock.Any()).Do(
				func(id string, timeout time.Duration) {
					go func() {
						dockerEventSent <- 1
						// Emit 'ContainerStopped' event to the container event stream
						// This should cause the container and the task to transition
						// to 'STOPPED'
						eventStream <- createDockerEvent(api.ContainerStopped)
					}()
				}).Return(containerStopTimeoutError).MinTimes(1),
		)
	}

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	stateChangeEvents := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(sleepTask)
	// wait for task running
	waitForRunningEvents(t, taskEngine.StateChangeEvents())
	// Set the task desired status to be stopped and StopContainer will be called
	updateSleepTask := testdata.LoadTask("sleep5")
	updateSleepTask.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(updateSleepTask)

	// StopContainer timeout error shouldn't cause cantainer/task status change
	// until receive stop event from docker event stream
	select {
	case <-stateChangeEvents:
		t.Error("Should not get task events")
	case <-dockerEventSent:
		t.Logf("Send docker stop event")
		go func() {
			for {
				select {
				case <-dockerEventSent:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// StopContainer was called again and received stop event from docker event stream
	// Expect it to go to stopped
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false)
}

// TestTaskTransitionWhenStopContainerReturnsUnretriableError tests if the task transitions
// to stopped without retrying stopping the container in the task when the initial
// stop container call returns an unretriable error from docker, specifically the
// ContainerNotRunning error
func TestTaskTransitionWhenStopContainerReturnsUnretriableError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)
	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	containerEventsWG := sync.WaitGroup{}
	for _, container := range sleepTask.Containers {
		gomock.InOrder(
			imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes(),
			client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{}),
			imageManager.EXPECT().RecordContainerReference(container),
			imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil),
			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
			// Simulate successful create container
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(x, y, z, timeout interface{}) {
					containerEventsWG.Add(1)
					go func() {
						eventStream <- createDockerEvent(api.ContainerCreated)
						containerEventsWG.Done()
					}()
				}).Return(DockerContainerMetadata{DockerID: containerID}),
			// Simulate successful start container
			client.EXPECT().StartContainer(containerID, startContainerTimeout).Do(
				func(id string, timeout time.Duration) {
					containerEventsWG.Add(1)
					go func() {
						eventStream <- createDockerEvent(api.ContainerRunning)
						containerEventsWG.Done()
					}()
				}).Return(DockerContainerMetadata{DockerID: containerID}),
			// StopContainer errors out. However, since this is a known unretriable error,
			// the task engine should not retry stopping the container and move on.
			// If there's a delay in task engine's processing of the ContainerRunning
			// event, StopContainer will be invoked again as the engine considers it
			// as a stopped container coming back. MinTimes() should guarantee that
			// StopContainer is invoked at least once and in protecting agasint a test
			// failure when there's a delay in task engine processing the ContainerRunning
			// event.
			client.EXPECT().StopContainer(containerID, gomock.Any()).Return(DockerContainerMetadata{
				Error: CannotStopContainerError{&docker.ContainerNotRunning{}},
			}).MinTimes(1),
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
	updateSleepTask.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(updateSleepTask)
	// StopContainer was called again and received stop event from docker event stream
	// Expect it to go to stopped
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false)
}

// TestTaskTransitionWhenStopContainerReturnsTransientErrorBeforeSucceeding tests if the task
// transitions to stopped only after receiving the container stopped event from docker when
// the initial stop container call fails with an unknown error.
func TestTaskTransitionWhenStopContainerReturnsTransientErrorBeforeSucceeding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)
	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	containerStoppingError := DockerContainerMetadata{
		Error: CannotStopContainerError{errors.New("Error stopping container")},
	}
	for _, container := range sleepTask.Containers {
		gomock.InOrder(
			imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes(),
			client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{}),
			imageManager.EXPECT().RecordContainerReference(container),
			imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil),
			// Simulate successful create container
			client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
				DockerContainerMetadata{DockerID: containerID}),
			// Simulate successful start container
			client.EXPECT().StartContainer(containerID, startContainerTimeout).Return(
				DockerContainerMetadata{DockerID: containerID}),
			// StopContainer errors out a couple of times
			client.EXPECT().StopContainer(containerID, gomock.Any()).Return(containerStoppingError).Times(2),
			// Since task is not in steady state, progressContainers causes
			// another invocation of StopContainer. Return the 'succeed' response,
			// which should cause the task engine to stop invoking this again and
			// transition the task to stopped.
			client.EXPECT().StopContainer(containerID, gomock.Any()).Return(DockerContainerMetadata{}),
		)
	}

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	go taskEngine.AddTask(sleepTask)
	// wait for task running
	waitForRunningEvents(t, taskEngine.StateChangeEvents())
	// Set the task desired status to be stopped and StopContainer will be called
	updateSleepTask := testdata.LoadTask("sleep5")
	updateSleepTask.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(updateSleepTask)
	// StopContainer invocation should have caused it to stop eventually.
	waitForStopEvents(t, taskEngine.StateChangeEvents(), false)
}

func TestGetTaskByArn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	// Need a mock client as AddTask not only adds a task to the engine, but
	// also causes the engine to progress the task.
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	client.EXPECT().Version()
	eventStream := make(chan DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).AnyTimes()
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)
	defer taskEngine.Disable()
	sleepTask := testdata.LoadTask("sleep5")
	sleepTask.SetDesiredStatus(api.TaskStopped)
	sleepTaskArn := sleepTask.Arn
	sleepTask.SetDesiredStatus(api.TaskStopped)
	taskEngine.AddTask(sleepTask)

	_, found := taskEngine.GetTaskByArn(sleepTaskArn)
	assert.True(t, found, "Task %s not found", sleepTaskArn)

	_, found = taskEngine.GetTaskByArn(sleepTaskArn + "arn")
	assert.False(t, found, "Task with invalid arn found in the task engine")
}

func TestEngineEnableConcurrentPull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().Version().Return("1.11.1", nil)
	client.EXPECT().ContainerEvents(gomock.Any())

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	dockerTaskEngine, _ := taskEngine.(*DockerTaskEngine)
	assert.True(t, dockerTaskEngine.enableConcurrentPull,
		"Task engine should be able to perform concurrent pulling for docker version >= 1.11.1")
}

func TestEngineDisableConcurrentPull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().Version().Return("1.11.0", nil)
	client.EXPECT().ContainerEvents(gomock.Any())

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	dockerTaskEngine, _ := taskEngine.(*DockerTaskEngine)
	assert.False(t, dockerTaskEngine.enableConcurrentPull,
		"Task engine should not be able to perform concurrent pulling for version < 1.11.1")
}

func TestPauseContainerHappyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	cniClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = cniClient
	eventStream := make(chan DockerContainerChangeEvent)
	sleepTask := testdata.LoadTask("sleep5")

	// Add eni information to the task so the task can add dependency of pause container
	sleepTask.SetTaskENI(&api.ENI{
		ID:         "id",
		MacAddress: "mac",
		IPV4Addresses: []*api.ENIIPV4Address{
			{
				Primary: true,
				Address: "ipv4",
			},
		},
	})

	dockerClient.EXPECT().Version()
	dockerClient.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)

	pauseContainerID := "pauseContainerID"
	// Pause container will be launched first
	gomock.InOrder(
		dockerClient.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		dockerClient.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(config *docker.Config, x, y, z interface{}) {
				name, ok := config.Labels[labelPrefix+"container-name"]
				assert.True(t, ok)
				assert.Equal(t, api.PauseContainerName, name)
			}).Return(DockerContainerMetadata{DockerID: "pauseContainerID"}),
		dockerClient.EXPECT().StartContainer(pauseContainerID, startContainerTimeout).Return(
			DockerContainerMetadata{DockerID: "pauseContainerID"}),
		dockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any()).Return(
			&docker.Container{
				ID:    pauseContainerID,
				State: docker.State{Pid: containerPid},
			}, nil),
		cniClient.EXPECT().SetupNS(gomock.Any()).Return(nsResult, nil),
	)

	// For the other container
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	dockerClient.EXPECT().PullImage(gomock.Any(), nil).Return(DockerContainerMetadata{})
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Return(nil)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil)
	dockerClient.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil)
	dockerClient.EXPECT().CreateContainer(gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any()).Return(DockerContainerMetadata{DockerID: containerID})
	dockerClient.EXPECT().StartContainer(containerID, startContainerTimeout).Return(
		DockerContainerMetadata{DockerID: containerID})

	steadyStateVerify := make(chan time.Time)
	cleanup := make(chan time.Time)
	defer close(cleanup)
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()
	// Expect steady state check once
	mockTime.EXPECT().After(steadyStateTaskVerifyInterval).Return(steadyStateVerify).MinTimes(1)
	dockerClient.EXPECT().DescribeContainer(containerID).AnyTimes()
	dockerClient.EXPECT().DescribeContainer(pauseContainerID).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	taskEngine.AddTask(sleepTask)
	stateChangeEvents := taskEngine.StateChangeEvents()
	verifyTaskIsRunning(stateChangeEvents, sleepTask)
	steadyStateVerify <- time.Now()

	var wg sync.WaitGroup
	wg.Add(1)
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).MinTimes(1)
	dockerClient.EXPECT().InspectContainer(gomock.Any(), gomock.Any()).Return(&docker.Container{
		ID:    pauseContainerID,
		State: docker.State{Pid: containerPid},
	}, nil)
	cniClient.EXPECT().CleanupNS(gomock.Any()).Return(nil)
	dockerClient.EXPECT().StopContainer(pauseContainerID, gomock.Any()).Return(
		DockerContainerMetadata{DockerID: pauseContainerID})
	cniClient.EXPECT().ReleaseIPResource(gomock.Any()).Do(func(cfg *ecscni.Config) {
		wg.Done()
	}).Return(nil)
	dockerClient.EXPECT().RemoveContainer(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any()).Return(nil)

	// Simulate a container stop event from docker
	eventStream <- DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: DockerContainerMetadata{
			DockerID: containerID,
			ExitCode: aws.Int(exitCode),
		},
	}

	verifyTaskIsStopped(stateChangeEvents, sleepTask)
	sleepTask.SetSentStatus(api.TaskStopped)
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

func TestBuildCNIConfigFromTaskContainer(t *testing.T) {
	for _, blockIMDS := range []bool{true, false} {
		t.Run(fmt.Sprintf("When BlockInstanceMetadata is %t", blockIMDS), func(t *testing.T) {
			config := defaultConfig
			config.AWSVPCBlockInstanceMetdata = blockIMDS
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ctrl, dockerClient, _, taskEngine, _, _, _ := mocks(t, ctx, &config)
			defer ctrl.Finish()

			testTask := testdata.LoadTask("sleep5")
			testTask.SetTaskENI(&api.ENI{
				ID: "TestBuildCNIConfigFromTaskContainer",
				IPV4Addresses: []*api.ENIIPV4Address{
					{
						Primary: true,
						Address: ipv4,
					},
				},
				MacAddress: mac,
				IPV6Addresses: []*api.ENIIPV6Address{
					{
						Address: ipv6,
					},
				},
			})
			container := &api.Container{
				Name: "container",
			}
			taskEngine.(*DockerTaskEngine).state.AddContainer(&api.DockerContainer{
				Container:  container,
				DockerName: dockerContainerName,
			}, testTask)

			dockerClient.EXPECT().InspectContainer(dockerContainerName, gomock.Any()).Return(&docker.Container{
				ID:    containerID,
				State: docker.State{Pid: containerPid},
			}, nil)

			cniConfig, err := taskEngine.(*DockerTaskEngine).buildCNIConfigFromTaskContainer(testTask, container)
			assert.NoError(t, err)
			assert.Equal(t, containerID, cniConfig.ContainerID)
			assert.Equal(t, strconv.Itoa(containerPid), cniConfig.ContainerPID)
			assert.Equal(t, mac, cniConfig.ID, "ID should be set to the mac of eni")
			assert.Equal(t, mac, cniConfig.ENIMACAddress)
			assert.Equal(t, ipv4, cniConfig.ENIIPV4Address)
			assert.Equal(t, ipv6, cniConfig.ENIIPV6Address)
			assert.Equal(t, blockIMDS, cniConfig.BlockInstanceMetdata)
		})
	}
}

func TestBuildCNIConfigFromTaskContainerInspectError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, _, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	testTask := testdata.LoadTask("sleep5")
	testTask.SetTaskENI(&api.ENI{})
	container := &api.Container{
		Name: "container",
	}
	taskEngine.(*DockerTaskEngine).state.AddContainer(&api.DockerContainer{
		Container:  container,
		DockerName: dockerContainerName,
	}, testTask)

	dockerClient.EXPECT().InspectContainer(dockerContainerName, gomock.Any()).Return(nil, errors.New("error"))

	_, err := taskEngine.(*DockerTaskEngine).buildCNIConfigFromTaskContainer(testTask, container)
	assert.Error(t, err)
}

// TestStopPauseContainerCleanupCalled tests when stopping the pause container
// its network namespace should be cleaned up first
func TestStopPauseContainerCleanupCalled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, dockerClient, _, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockCNIClient := mock_ecscni.NewMockCNIClient(ctrl)
	taskEngine.(*DockerTaskEngine).cniClient = mockCNIClient
	testTask := testdata.LoadTask("sleep5")
	pauseContainer := &api.Container{
		Name: "pausecontainer",
		Type: api.ContainerCNIPause,
	}
	testTask.Containers = append(testTask.Containers, pauseContainer)
	testTask.SetTaskENI(&api.ENI{
		ID: "TestStopPauseContainerCleanupCalled",
		IPV4Addresses: []*api.ENIIPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		},
		MacAddress: mac,
		IPV6Addresses: []*api.ENIIPV6Address{
			{
				Address: ipv6,
			},
		},
	})
	taskEngine.(*DockerTaskEngine).State().AddTask(testTask)
	taskEngine.(*DockerTaskEngine).State().AddContainer(&api.DockerContainer{
		DockerID:   containerID,
		DockerName: dockerContainerName,
		Container:  pauseContainer,
	}, testTask)

	gomock.InOrder(
		dockerClient.EXPECT().InspectContainer(dockerContainerName, gomock.Any()).Return(&docker.Container{
			ID:    containerID,
			State: docker.State{Pid: containerPid},
		}, nil),
		mockCNIClient.EXPECT().CleanupNS(gomock.Any()).Return(nil),
		dockerClient.EXPECT().StopContainer(containerID, stopContainerTimeout).Return(DockerContainerMetadata{}),
	)

	taskEngine.(*DockerTaskEngine).stopContainer(testTask, pauseContainer)
}

// TestTaskWithCircularDependency tests the task with containers of which the
// dependencies can't be resolved
func TestTaskWithCircularDependency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().Version().Return("1.12.6", nil)
	client.EXPECT().ContainerEvents(gomock.Any())

	task := testdata.LoadTask("circular_dependency")

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	events := taskEngine.StateChangeEvents()
	go taskEngine.AddTask(task)
	event := <-events
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to move to stopped directly")
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
	ctrl, client, _, privateTaskEngine, _, _, _ := mocks(t, ctx, &config.Config{})
	saver := mock_statemanager.NewMockStateManager(ctrl)
	defer ctrl.Finish()

	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	taskEngine.SetSaver(saver)
	state := taskEngine.State()
	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer, _ := sleepTask.ContainerByName("sleep5")
	// Store the generated container name to state
	state.AddContainer(&api.DockerContainer{DockerName: "docker_container_name", Container: sleepContainer}, sleepTask)

	gomock.InOrder(
		client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil),
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), "docker_container_name", gomock.Any()),
	)

	metadata := taskEngine.createContainer(sleepTask, sleepContainer)
	if metadata.Error != nil {
		t.Error("Unexpected error", metadata.Error)
	}
}

func TestPullCNIImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, privateTaskEngine, _, _, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)

	container := &api.Container{
		Type: api.ContainerCNIPause,
	}
	task := &api.Task{
		Containers: []*api.Container{container},
	}
	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, DockerContainerMetadata{}, metadata, "expected empty metadata")
}

func TestPullNormalImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, imageManager, _ := mocks(t, ctx, &config.Config{})
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	saver := mock_statemanager.NewMockStateManager(ctrl)
	taskEngine.SetSaver(saver)
	taskEngine._time = nil
	imageName := "image"
	container := &api.Container{
		Type:  api.ContainerNormal,
		Image: imageName,
	}
	task := &api.Task{
		Containers: []*api.Container{container},
	}
	imageState := &image.ImageState{
		Image: &image.Image{ImageID: "id"},
	}

	client.EXPECT().PullImage(imageName, nil)
	imageManager.EXPECT().RecordContainerReference(container)
	imageManager.EXPECT().GetImageStateFromImageName(imageName).Return(imageState)
	saver.EXPECT().Save()
	metadata := taskEngine.pullContainer(task, container)
	assert.Equal(t, DockerContainerMetadata{}, metadata, "expected empty metadata")
}

// TestMetadataFileUpdatedAgentRestart checks whether metadataManager.Update(...) is
// invoked in the path DockerTaskEngine.Init() -> .synchronizeState() -> .updateMetadataFile(...)
// for the following case:
// agent starts, container created, metadata file created, agent restarted, container recovered
// during task engine init, metadata file updated
func TestMetadataFileUpdatedAgentRestart(t *testing.T) {
	conf := &defaultConfig
	conf.ContainerMetadataEnabled = true
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, imageManager, metadataManager := mocks(t, ctx, conf)
	saver := mock_statemanager.NewMockStateManager(ctrl)
	defer ctrl.Finish()

	var metadataUpdateWG sync.WaitGroup
	metadataUpdateWG.Add(1)
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	assert.True(t, taskEngine.cfg.ContainerMetadataEnabled, "ContainerMetadataEnabled set to false.")

	taskEngine._time = nil
	taskEngine.SetSaver(saver)
	state := taskEngine.State()
	task := testdata.LoadTask("sleep5")
	container, _ := task.ContainerByName("sleep5")
	assert.False(t, container.MetadataFileUpdated)
	container.SetKnownStatus(api.ContainerRunning)
	dockerContainer := &api.DockerContainer{DockerID: containerID, Container: container}
	expectedTaskARN := task.Arn
	expectedDockerID := dockerContainer.DockerID
	expectedContainerName := container.Name

	state.AddTask(task)
	state.AddContainer(dockerContainer, task)
	client.EXPECT().Version()
	eventStream := make(chan DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	client.EXPECT().DescribeContainer(gomock.Any())
	imageManager.EXPECT().RecordContainerReference(gomock.Any())
	saver.EXPECT().Save().AnyTimes()
	saver.EXPECT().ForceSave().AnyTimes()

	metadataManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(dockerID string, taskARN string, containerName string) {
			assert.Equal(t, expectedTaskARN, taskARN)
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
	ctrl, client, mockTime, taskEngine, credentialsManager, imageManager, _ := mocks(
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
	testTask.Containers[0].RegistryAuthentication = &api.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &api.ECRAuthData{
			UseExecutionRole: true,
		},
	}
	container := testTask.Containers[0]

	mockTime.EXPECT().Now().AnyTimes()
	credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(credentials.TaskIAMRoleCredentials{
		ARN:                "",
		IAMRoleCredentials: executionRoleCredentials,
	}, true)
	client.EXPECT().PullImage(gomock.Any(), gomock.Any()).Do(
		func(image string, auth *api.RegistryAuthenticationData) {
			assert.Equal(t, container.Image, image)
			assert.Equal(t, auth.ECRAuthData.GetPullCredentials(), executionRoleCredentials)
		}).Return(DockerContainerMetadata{})
	imageManager.EXPECT().RecordContainerReference(container).Return(nil)
	imageManager.EXPECT().GetImageStateFromImageName(container.Image)

	taskEngine.(*DockerTaskEngine).pullContainer(testTask, container)
}

// TestNewTasktionRoleOnRestart tests the agent will process the task recorded in
// the state file on restart
func TestNewTaskTransitionOnRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, mockTime, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	mockTime.EXPECT().Now().AnyTimes()

	dockerTaskEngine := taskEngine.(*DockerTaskEngine)
	state := dockerTaskEngine.State()
	testTask := testdata.LoadTask("sleep5")
	// add the task to the state to simulate the agent restored the state on restart
	state.AddTask(testTask)
	// Set the task to be stopped so that the process can done quickly
	testTask.SetDesiredStatus(api.TaskStopped)
	dockerTaskEngine.synchronizeState()
	_, ok := dockerTaskEngine.managedTasks[testTask.Arn]
	assert.True(t, ok, "task wasnot started")
}

// TestTaskWaitForHostResourceOnRestart tests task stopped by acs but hasn't
// reached stopped should block the later task to start
func TestTaskWaitForHostResourceOnRestart(t *testing.T) {
	// Task 1 stopped by backend
	taskStoppedByACS := testdata.LoadTask("sleep5")
	taskStoppedByACS.SetDesiredStatus(api.TaskStopped)
	taskStoppedByACS.SetStopSequenceNumber(1)
	taskStoppedByACS.SetKnownStatus(api.TaskRunning)
	// Task 2 has essential container stopped
	taskEssentialContainerStopped := testdata.LoadTask("sleep5")
	taskEssentialContainerStopped.Arn = "task_Essential_Container_Stopped"
	taskEssentialContainerStopped.SetDesiredStatus(api.TaskStopped)
	taskEssentialContainerStopped.SetKnownStatus(api.TaskRunning)
	// Normal task 3 needs to be started
	taskNotStarted := testdata.LoadTask("sleep5")
	taskNotStarted.Arn = "task_Not_started"

	conf := &defaultConfig
	conf.ContainerMetadataEnabled = false
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, privateTaskEngine, _, imageManager, _ := mocks(t, ctx, conf)
	defer ctrl.Finish()
	saver := mock_statemanager.NewMockStateManager(ctrl)

	taskEngine := privateTaskEngine.(*DockerTaskEngine)
	taskEngine.saver = saver
	taskEngine.State().AddTask(taskStoppedByACS)
	taskEngine.State().AddTask(taskNotStarted)
	taskEngine.State().AddTask(taskEssentialContainerStopped)

	taskEngine.State().AddContainer(&api.DockerContainer{
		Container:  taskStoppedByACS.Containers[0],
		DockerID:   containerID + "1",
		DockerName: dockerContainerName + "1",
	}, taskStoppedByACS)
	taskEngine.State().AddContainer(&api.DockerContainer{
		Container:  taskNotStarted.Containers[0],
		DockerID:   containerID + "2",
		DockerName: dockerContainerName + "2",
	}, taskNotStarted)
	taskEngine.State().AddContainer(&api.DockerContainer{
		Container:  taskEssentialContainerStopped.Containers[0],
		DockerID:   containerID + "3",
		DockerName: dockerContainerName + "3",
	}, taskEssentialContainerStopped)

	// these are performed in synchronizeState on restart
	client.EXPECT().DescribeContainer(gomock.Any()).Return(api.ContainerRunning, DockerContainerMetadata{
		DockerID: containerID,
	}).Times(3)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Times(3)

	saver.EXPECT().Save()
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
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	testTask := &api.Task{
		Arn: "taskArn",
	}
	container := &api.Container{
		Image: "image1",
	}
	startTime1 := time.Now()
	startTime2 := startTime1.Add(time.Second)
	startTime3 := startTime2.Add(time.Second)
	stopTime1 := startTime3.Add(time.Second)
	stopTime2 := stopTime1.Add(time.Second)
	stopTime3 := stopTime2.Add(time.Second)

	client.EXPECT().PullImage(gomock.Any(), gomock.Any()).Times(3)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Times(3)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil).Times(3)

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
	ctrl, client, mockTime, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	testTask := &api.Task{
		Arn: "taskArn",
	}
	container := &api.Container{
		Image: "image1",
	}

	startTime1 := time.Now()
	startTime2 := startTime1.Add(time.Second)
	startTime3 := startTime2.Add(time.Second)
	stopTime1 := startTime3.Add(time.Second)
	stopTime2 := stopTime1.Add(time.Second)
	stopTime3 := stopTime2.Add(time.Second)

	gomock.InOrder(
		client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{}),
		client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{}),
		client.EXPECT().PullImage(container.Image, nil).Return(
			DockerContainerMetadata{Error: CannotPullContainerError{fmt.Errorf("error")}}),
	)
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).Times(3)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil).Times(3)
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
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, client, _, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	dockerID := "1234"
	dockerContainer := &api.DockerContainer{
		DockerID:   dockerID,
		DockerName: "c1",
		Container:  &api.Container{},
	}
	labels := map[string]string{
		"name": "metadata",
	}
	created := time.Now()
	gomock.InOrder(
		client.EXPECT().DescribeContainer(dockerID).Return(api.ContainerRunning,
			DockerContainerMetadata{
				Labels:    labels,
				DockerID:  dockerID,
				CreatedAt: created,
			}),
		imageManager.EXPECT().RecordContainerReference(dockerContainer.Container),
	)
	taskEngine.(*DockerTaskEngine).synchronizeContainerStatus(dockerContainer, nil)
	assert.Equal(t, created, dockerContainer.Container.GetCreatedAt())
	assert.Equal(t, labels, dockerContainer.Container.GetLabels())
}

// TestHandleDockerHealthEvent tests the docker health event will only cause the
// container health status change
func TestHandleDockerHealthEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl, _, _, taskEngine, _, _, _ := mocks(t, ctx, &defaultConfig)
	defer ctrl.Finish()

	state := taskEngine.(*DockerTaskEngine).State()
	testTask := testdata.LoadTask("sleep5")
	testContainer := testTask.Containers[0]
	testContainer.HealthCheckType = "docker"

	state.AddTask(testTask)
	state.AddContainer(&api.DockerContainer{DockerID: "id",
		DockerName: "container_name",
		Container:  testContainer,
	}, testTask)

	taskEngine.(*DockerTaskEngine).handleDockerEvent(DockerContainerChangeEvent{
		Status: api.ContainerRunning,
		Type:   api.ContainerHealthEvent,
		DockerContainerMetadata: DockerContainerMetadata{
			DockerID: "id",
			Health: api.HealthStatus{
				Status: api.ContainerHealthy,
			},
		},
	})
	assert.Equal(t, testContainer.Health.Status, api.ContainerHealthy)
}

func TestContainerMetadataUpdatedOnRestart(t *testing.T) {

	dockerID := "dockerID_created"
	labels := map[string]string{
		"name": "metadata",
	}
	testCases := []struct {
		stage        string
		status       api.ContainerStatus
		created      time.Time
		started      time.Time
		finished     time.Time
		portBindings []api.PortBinding
		exitCode     *int
		err          DockerStateError
	}{
		{
			stage:   "created",
			status:  api.ContainerCreated,
			created: time.Now(),
		},
		{
			stage:   "started",
			status:  api.ContainerRunning,
			started: time.Now(),
			portBindings: []api.PortBinding{
				{
					ContainerPort: 80,
					HostPort:      80,
					BindIP:        "0.0.0.0/0",
					Protocol:      api.TransportProtocolTCP,
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
			status:   api.ContainerStopped,
			err:      NewDockerStateError("error"),
			exitCode: aws.Int(1),
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Agent restarted during container: %s", tc.stage), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			ctrl, client, _, taskEngine, _, imageManager, _ := mocks(t, ctx, &defaultConfig)
			defer ctrl.Finish()
			dockerContainer := &api.DockerContainer{
				DockerID:   dockerID,
				DockerName: fmt.Sprintf("docker%s", tc.stage),
				Container:  &api.Container{},
			}
			task := &api.Task{}

			if tc.stage == "created" {
				dockerContainer.Container.MountPoints = []api.MountPoint{
					{
						SourceVolume:  "empty",
						ContainerPath: "container",
					},
				}
				dockerContainer.DockerID = ""
				task.Volumes = []api.TaskVolume{
					{
						Name:   "empty",
						Volume: &api.EmptyHostVolume{},
					},
				}
				client.EXPECT().InspectContainer(dockerContainer.DockerName, gomock.Any()).Return(&docker.Container{
					ID: dockerID,
					Config: &docker.Config{
						Labels: labels,
					},
					Created: tc.created,
					Volumes: map[string]string{
						"container": "tmp",
					},
				}, nil)
				imageManager.EXPECT().RecordContainerReference(dockerContainer.Container).AnyTimes()
			} else {
				client.EXPECT().DescribeContainer(dockerID).Return(tc.status, DockerContainerMetadata{
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
			assert.Equal(t, tc.created, dockerContainer.Container.GetCreatedAt())
			assert.Equal(t, tc.started, dockerContainer.Container.GetStartedAt())
			assert.Equal(t, tc.finished, dockerContainer.Container.GetFinishedAt())
			if tc.stage == "created" {
				assert.Equal(t, "tmp", task.Volumes[0].Volume.SourcePath())
			}
			if tc.stage == "started" {
				assert.Equal(t, uint16(80), dockerContainer.Container.KnownPortBindings[0].ContainerPort)
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
