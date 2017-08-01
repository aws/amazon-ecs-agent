// +build !integration
// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"golang.org/x/net/context"
)

const credentialsID = "credsid"

var defaultConfig = config.DefaultConfig()

func mocks(t *testing.T, cfg *config.Config) (*gomock.Controller, *MockDockerClient, *mock_ttime.MockTime, TaskEngine, *mock_credentials.MockManager, *MockImageManager) {
	ctrl := gomock.NewController(t)
	client := NewMockDockerClient(ctrl)
	mockTime := mock_ttime.NewMockTime(ctrl)
	credentialsManager := mock_credentials.NewMockManager(ctrl)

	containerChangeEventStream := eventstream.NewEventStream("TESTTASKENGINE", context.Background())
	containerChangeEventStream.StartListening()
	imageManager := NewMockImageManager(ctrl)
	taskEngine := NewTaskEngine(cfg, client, credentialsManager, containerChangeEventStream, imageManager, dockerstate.NewTaskEngineState())
	taskEngine.(*DockerTaskEngine)._time = mockTime
	return ctrl, client, mockTime, taskEngine, credentialsManager, imageManager
}

func createDockerEvent(status api.ContainerStatus) DockerContainerChangeEvent {
	meta := DockerContainerMetadata{
		DockerID: "containerId",
	}
	return DockerContainerChangeEvent{Status: status, DockerContainerMetadata: meta}
}

func TestBatchContainerHappyPath(t *testing.T) {
	ctrl, client, mockTime, taskEngine, credentialsManager, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	roleCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{CredentialsID: "credsid"},
	}
	credentialsManager.EXPECT().GetTaskCredentials(credentialsID).Return(roleCredentials, true).AnyTimes()
	credentialsManager.EXPECT().RemoveCredentials(credentialsID)

	sleepTask := testdata.LoadTask("sleep5")
	sleepTask.SetCredentialsID(credentialsID)

	eventStream := make(chan DockerContainerChangeEvent)
	// createStartEventsReported is used to force the test to wait until the container created and started
	// events are processed
	createStartEventsReported := sync.WaitGroup{}

	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	var createdContainerName string
	for _, container := range sleepTask.Containers {
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
		client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{})
		imageManager.EXPECT().RecordContainerReference(container).Return(nil)
		imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil)
		dockerConfig, err := sleepTask.DockerConfig(container)
		if err != nil {
			t.Fatal(err)
		}
		// Container config should get updated with this during PostUnmarshalTask
		credentialsEndpointEnvValue := roleCredentials.IAMRoleCredentials.GenerateCredentialsEndpointRelativeURI()
		dockerConfig.Env = append(dockerConfig.Env, "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI="+credentialsEndpointEnvValue)
		// Container config should get updated with this during CreateContainer
		dockerConfig.Labels["com.amazonaws.ecs.task-arn"] = sleepTask.Arn
		dockerConfig.Labels["com.amazonaws.ecs.container-name"] = container.Name
		dockerConfig.Labels["com.amazonaws.ecs.task-definition-family"] = sleepTask.Family
		dockerConfig.Labels["com.amazonaws.ecs.task-definition-version"] = sleepTask.Version
		dockerConfig.Labels["com.amazonaws.ecs.cluster"] = ""
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(config *docker.Config, y interface{}, containerName string, z time.Duration) {

				if !reflect.DeepEqual(dockerConfig, config) {
					t.Errorf("Mismatch in container config; expected: %v, got: %v", dockerConfig, config)
				}
				// sleep5 task contains only one container. Just assign
				// the containerName to createdContainerName
				createdContainerName = containerName
				createStartEventsReported.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerCreated)
					createStartEventsReported.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: "containerId"})

		client.EXPECT().StartContainer("containerId", startContainerTimeout).Do(
			func(id string, timeout time.Duration) {
				createStartEventsReported.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerRunning)
					createStartEventsReported.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: "containerId"})
	}

	// steadyStateCheckWait is used to force the test to wait until the steady-state check
	// has been invoked at least once
	steadyStateCheckWait := sync.WaitGroup{}
	steadyStateVerify := make(chan time.Time, 1)
	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().Now().Do(func() time.Time { return time.Now() }).AnyTimes()
	gomock.InOrder(
		mockTime.EXPECT().After(steadyStateTaskVerifyInterval).Do(func(d time.Duration) {
			steadyStateCheckWait.Done()
		}).Return(steadyStateVerify),
		mockTime.EXPECT().After(steadyStateTaskVerifyInterval).Return(steadyStateVerify).AnyTimes(),
	)

	err := taskEngine.Init(context.TODO())
	assert.NoError(t, err)

	stateChangeEvents := taskEngine.StateChangeEvents()

	steadyStateCheckWait.Add(1)
	taskEngine.AddTask(sleepTask)

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerRunning, "Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskRunning, "Expected task to be RUNNING")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}

	// Wait for container create and start events to be processed
	createStartEventsReported.Wait()
	// Wait for steady state check to be invoked
	steadyStateCheckWait.Wait()
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()
	client.EXPECT().DescribeContainer(gomock.Any()).AnyTimes()

	// Wait for all events to be consumed prior to moving it towards stopped; we
	// don't want to race the below with these or we'll end up with the "going
	// backwards in state" stop and we haven't 'expect'd for that

	exitCode := 0
	// And then docker reports that sleep died, as sleep is wont to do
	eventStream <- DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: DockerContainerMetadata{
			DockerID: "containerId",
			ExitCode: &exitCode,
		},
	}

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerStopped, "Expected container to be STOPPED")

	// hold on to container event to verify exit code
	contEvent := event.(api.ContainerStateChange)
	assert.Equal(t, *contEvent.ExitCode, 0, "Exit code should be present")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to be STOPPED")
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

	// Expect a bunch of steady state 'poll' describes when we trigger cleanup
	client.EXPECT().RemoveContainer(gomock.Any(), gomock.Any()).Do(
		func(removedContainerName string, timeout time.Duration) {
			assert.Equal(t, createdContainerName, removedContainerName, "Container name mismatch")
		}).Return(nil)

	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any())
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
}

// TestRemoveEvents tests if the task engine can handle task events while the task is being
// cleaned up. This test ensures that there's no regression in the task engine and ensures
// there's no deadlock as seen in #313
func TestRemoveEvents(t *testing.T) {
	ctrl, client, mockTime, taskEngine, _, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)

	// createStartEventsReported is used to force the test to wait until the container created and started
	// events are processed
	createStartEventsReported := sync.WaitGroup{}
	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	var createdContainerName string
	for _, container := range sleepTask.Containers {
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
		client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{})
		imageManager.EXPECT().RecordContainerReference(container).Return(nil)
		imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil)
		client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(config *docker.Config, y interface{}, containerName string, z time.Duration) {
				createdContainerName = containerName
				createStartEventsReported.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerCreated)
					createStartEventsReported.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: "containerId"})

		client.EXPECT().StartContainer("containerId", startContainerTimeout).Do(
			func(id string, timeout time.Duration) {
				createStartEventsReported.Add(1)
				go func() {
					eventStream <- createDockerEvent(api.ContainerRunning)
					createStartEventsReported.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: "containerId"})
	}

	// steadyStateCheckWait is used to force the test to wait until the steady-state check
	// has been invoked at least once
	steadyStateCheckWait := sync.WaitGroup{}
	steadyStateVerify := make(chan time.Time, 1)
	cleanup := make(chan time.Time, 1)
	mockTime.EXPECT().Now().Do(func() time.Time { return time.Now() }).AnyTimes()
	gomock.InOrder(
		mockTime.EXPECT().After(steadyStateTaskVerifyInterval).Do(func(d time.Duration) {
			steadyStateCheckWait.Done()
		}).Return(steadyStateVerify),
		mockTime.EXPECT().After(steadyStateTaskVerifyInterval).Return(steadyStateVerify).AnyTimes(),
	)

	err := taskEngine.Init(context.TODO())
	assert.NoError(t, err)

	stateChangeEvents := taskEngine.StateChangeEvents()
	steadyStateCheckWait.Add(1)
	taskEngine.AddTask(sleepTask)

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerRunning, "Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskRunning, "Expected task to be RUNNING")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}

	// Wait for container create and start events to be processed
	createStartEventsReported.Wait()
	// Wait for steady state check to be invoked
	steadyStateCheckWait.Wait()
	mockTime.EXPECT().After(gomock.Any()).Return(cleanup).AnyTimes()
	client.EXPECT().DescribeContainer(gomock.Any()).AnyTimes()

	// Wait for all events to be consumed prior to moving it towards stopped; we
	// don't want to race the below with these or we'll end up with the "going
	// backwards in state" stop and we haven't 'expect'd for that

	exitCode := 0
	// And then docker reports that sleep died, as sleep is wont to do
	eventStream <- DockerContainerChangeEvent{
		Status: api.ContainerStopped,
		DockerContainerMetadata: DockerContainerMetadata{
			DockerID: "containerId",
			ExitCode: &exitCode,
		},
	}

	event = <-stateChangeEvents
	if cont := event.(api.ContainerStateChange); cont.Status != api.ContainerStopped {
		t.Fatal("Expected container to stop first")
		assert.Equal(t, *cont.ExitCode, 0, "Exit code should be present")
	}

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to be STOPPED")

	sleepTaskStop := testdata.LoadTask("sleep5")
	sleepTaskStop.SetDesiredStatus(api.TaskStopped)
	taskEngine.AddTask(sleepTaskStop)

	client.EXPECT().RemoveContainer(gomock.Any(), gomock.Any()).Do(
		func(removedContainerName string, timeout time.Duration) {
			assert.Equal(t, createdContainerName, removedContainerName, "Container name mismatch")

			// Emit a couple of events for the task before cleanup finishes. This forces
			// discardEventsUntil to be invoked and should test the code path that
			// caused the deadlock, which was fixed with #320
			eventStream <- createDockerEvent(api.ContainerStopped)
			eventStream <- createDockerEvent(api.ContainerStopped)
		}).Return(nil)

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
	ctrl, client, testTime, taskEngine, _, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")

	eventStream := make(chan DockerContainerChangeEvent)
	testTime.EXPECT().After(gomock.Any())

	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	for _, container := range sleepTask.Containers {
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
		client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{})

		imageManager.EXPECT().RecordContainerReference(container)
		imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil)
		dockerConfig, err := sleepTask.DockerConfig(container)
		if err != nil {
			t.Fatal(err)
		}

		// Container config should get updated with this during CreateContainer
		dockerConfig.Labels["com.amazonaws.ecs.task-arn"] = sleepTask.Arn
		dockerConfig.Labels["com.amazonaws.ecs.container-name"] = container.Name
		dockerConfig.Labels["com.amazonaws.ecs.task-definition-family"] = sleepTask.Family
		dockerConfig.Labels["com.amazonaws.ecs.task-definition-version"] = sleepTask.Version
		dockerConfig.Labels["com.amazonaws.ecs.cluster"] = ""

		client.EXPECT().CreateContainer(dockerConfig, gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(x, y, z, timeout interface{}) {
				go func() { eventStream <- createDockerEvent(api.ContainerCreated) }()
			}).Return(DockerContainerMetadata{DockerID: "containerId"})

		client.EXPECT().StartContainer("containerId", startContainerTimeout).Return(DockerContainerMetadata{
			Error: &DockerTimeoutError{},
		})
	}

	err := taskEngine.Init(context.TODO())
	assert.NoError(t, err)

	stateChangeEvents := taskEngine.StateChangeEvents()
	taskEngine.AddTask(sleepTask)

	// Expect it to go to stopped
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerStopped, "Expected container to timeout on start and stop")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to be STOPPED")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}

	// Expect it to try to stop it once now
	client.EXPECT().StopContainer("containerId", gomock.Any()).Return(DockerContainerMetadata{
		Error: CannotStartContainerError{fmt.Errorf("cannot start container")},
	}).AnyTimes()
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
	ctrl, client, testTime, taskEngine, _, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	wait := &sync.WaitGroup{}
	sleepTask := testdata.LoadTask("sleep5")

	eventStream := make(chan DockerContainerChangeEvent)

	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	// set up expectations for each container in the task calling create + start
	for _, container := range sleepTask.Containers {
		imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
		client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{})
		imageManager.EXPECT().RecordContainerReference(container)
		imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil)
		dockerConfig, err := sleepTask.DockerConfig(container)
		assert.Nil(t, err)

		// Container config should get updated with this during CreateContainer
		dockerConfig.Labels["com.amazonaws.ecs.task-arn"] = sleepTask.Arn
		dockerConfig.Labels["com.amazonaws.ecs.container-name"] = container.Name
		dockerConfig.Labels["com.amazonaws.ecs.task-definition-family"] = sleepTask.Family
		dockerConfig.Labels["com.amazonaws.ecs.task-definition-version"] = sleepTask.Version
		dockerConfig.Labels["com.amazonaws.ecs.cluster"] = ""

		wait.Add(1)
		client.EXPECT().CreateContainer(dockerConfig, gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(x, y, z, timeout interface{}) {
				go func() {
					eventStream <- createDockerEvent(api.ContainerCreated)
					wait.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: "containerId"})

		wait.Add(1)
		client.EXPECT().StartContainer("containerId", startContainerTimeout).Do(
			func(id string, timeout time.Duration) {
				go func() {
					eventStream <- createDockerEvent(api.ContainerRunning)
					wait.Done()
				}()
			}).Return(DockerContainerMetadata{DockerID: "containerId"})
	}

	steadyStateVerify := make(chan time.Time, 10) // channel to trigger a "steady state verify" action
	testTime.EXPECT().After(steadyStateTaskVerifyInterval).Return(steadyStateVerify).AnyTimes()
	err := taskEngine.Init(context.TODO()) // start the task engine
	assert.Nil(t, err)

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskEngine.AddTask(sleepTask) // actually add the task we created

	// verify that we get events for the container and task starting, but no other events
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerRunning, "Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskRunning, "Expected task to be RUNNING")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}

	containerMap, ok := taskEngine.(*DockerTaskEngine).State().ContainerMapByArn(sleepTask.Arn)
	assert.True(t, ok)
	dockerContainer, ok := containerMap[sleepTask.Containers[0].Name]
	assert.True(t, ok)

	// Two steady state oks, one stop
	gomock.InOrder(
		client.EXPECT().DescribeContainer("containerId").Return(
			api.ContainerRunning,
			DockerContainerMetadata{
				DockerID: "containerId",
			}).Times(2),
		client.EXPECT().DescribeContainer("containerId").Return(
			api.ContainerStopped,
			DockerContainerMetadata{
				DockerID: "containerId",
			}).MinTimes(1),
		// the engine *may* call StopContainer even though it's already stopped
		client.EXPECT().StopContainer("containerId", stopContainerTimeout).AnyTimes(),
	)
	wait.Wait()

	cleanupChan := make(chan time.Time)
	testTime.EXPECT().After(gomock.Any()).Return(cleanupChan).AnyTimes()
	client.EXPECT().RemoveContainer(dockerContainer.DockerName, removeContainerTimeout).Return(nil)
	imageManager.EXPECT().RemoveContainerReferenceFromImageState(gomock.Any()).Return(nil)

	// trigger steady state verification
	for i := 0; i < 10; i++ {
		steadyStateVerify <- time.Now()
	}

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerStopped, "Expected container to be STOPPED")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to be STOPPED")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}

	close(steadyStateVerify)
	// trigger cleanup, this ensures all the goroutines were finished
	sleepTask.SetSentStatus(api.TaskStopped)
	cleanupChan <- time.Now()

	for {
		tasks, _ := taskEngine.(*DockerTaskEngine).ListTasks()
		if len(tasks) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStopWithPendingStops(t *testing.T) {
	ctrl, client, testTime, taskEngine, _, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()
	testTime.EXPECT().Now().AnyTimes()
	testTime.EXPECT().After(gomock.Any()).AnyTimes()

	sleepTask1 := testdata.LoadTask("sleep5")
	sleepTask1.StartSequenceNumber = 5
	sleepTask2 := testdata.LoadTask("sleep5")
	sleepTask2.Arn = "arn2"

	eventStream := make(chan DockerContainerChangeEvent)

	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	err := taskEngine.Init(context.TODO())
	assert.NoError(t, err)
	stateChangeEvents := taskEngine.StateChangeEvents()
	go func() {
		for {
			<-stateChangeEvents
		}
	}()

	pullDone := make(chan bool)
	pullInvoked := make(chan bool)
	client.EXPECT().PullImage(gomock.Any(), nil).Do(func(x, y interface{}) {
		pullInvoked <- true
		<-pullDone
	})

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
	ctrl, client, _, privateTaskEngine, _, _ := mocks(t, &config.Config{})
	saver := mock_statemanager.NewMockStateManager(ctrl)
	defer ctrl.Finish()
	taskEngine, _ := privateTaskEngine.(*DockerTaskEngine)
	taskEngine.SetSaver(saver)

	sleepTask := testdata.LoadTask("sleep5")
	sleepContainer, _ := sleepTask.ContainerByName("sleep5")

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
	ctrl, client, _, taskEngine, _, _ := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	testTask := &api.Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
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
	expectedConfig, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Fatal(err)
	}
	expectedConfig.Labels = map[string]string{
		"com.amazonaws.ecs.task-arn":                "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		"com.amazonaws.ecs.container-name":          "c1",
		"com.amazonaws.ecs.task-definition-family":  "myFamily",
		"com.amazonaws.ecs.task-definition-version": "1",
		"com.amazonaws.ecs.cluster":                 "",
		"key": "value",
	}
	client.EXPECT().CreateContainer(expectedConfig, gomock.Any(), gomock.Any(), gomock.Any())
	taskEngine.(*DockerTaskEngine).createContainer(testTask, testTask.Containers[0])
}

// TestTaskTransitionWhenStopContainerTimesout tests that task transitions to stopped
// only when terminal events are recieved from docker event stream when
// StopContainer times out
func TestTaskTransitionWhenStopContainerTimesout(t *testing.T) {
	ctrl, client, mockTime, taskEngine, _, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")

	eventStream := make(chan DockerContainerChangeEvent)

	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
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
		dockerConfig, err := sleepTask.DockerConfig(container)
		if err != nil {
			t.Fatal(err)
		}
		// Container config should get updated with this during CreateContainer
		dockerConfig.Labels["com.amazonaws.ecs.task-arn"] = sleepTask.Arn
		dockerConfig.Labels["com.amazonaws.ecs.container-name"] = container.Name
		dockerConfig.Labels["com.amazonaws.ecs.task-definition-family"] = sleepTask.Family
		dockerConfig.Labels["com.amazonaws.ecs.task-definition-version"] = sleepTask.Version
		dockerConfig.Labels["com.amazonaws.ecs.cluster"] = ""

		client.EXPECT().CreateContainer(dockerConfig, gomock.Any(), gomock.Any(), gomock.Any()).Do(
			func(x, y, z, timeout interface{}) {
				go func() { eventStream <- createDockerEvent(api.ContainerCreated) }()
			}).Return(DockerContainerMetadata{DockerID: "containerId"})

		gomock.InOrder(
			client.EXPECT().StartContainer("containerId", startContainerTimeout).Do(
				func(id string, timeout time.Duration) {
					go func() {
						eventStream <- createDockerEvent(api.ContainerRunning)
					}()
				}).Return(DockerContainerMetadata{DockerID: "containerId"}),

			// StopContainer times out
			client.EXPECT().StopContainer("containerId", gomock.Any()).Return(containerStopTimeoutError),
			// Since task is not in steady state, progressContainers causes
			// another invocation of StopContainer. Return a timeout error
			// for that as well.
			client.EXPECT().StopContainer("containerId", gomock.Any()).Do(
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

	err := taskEngine.Init(context.TODO())
	assert.NoError(t, err)
	stateChangeEvents := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(sleepTask)
	// wait for task running
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerRunning, "Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskRunning, "Expected task to be RUNNING")

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
				<-dockerEventSent
			}
		}()
	}

	// StopContainer was called again and received stop event from docker event stream
	// Expect it to go to stopped
	event = <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerStopped, "Expected container to timeout on start and stop")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to be STOPPED")

	select {
	case <-stateChangeEvents:
		t.Error("Should be out of events")
	default:
	}
}

// TestTaskTransitionWhenStopContainerReturnsUnretriableError tests if the task transitions
// to stopped without retrying stopping the container in the task when the initial
// stop container call returns an unretriable error from docker, specifically the
// ContainerNotRunning error
func TestTaskTransitionWhenStopContainerReturnsUnretriableError(t *testing.T) {
	ctrl, client, mockTime, taskEngine, _, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)
	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	eventsReported := sync.WaitGroup{}
	for _, container := range sleepTask.Containers {
		gomock.InOrder(
			imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes(),
			client.EXPECT().PullImage(container.Image, nil).Return(DockerContainerMetadata{}),
			imageManager.EXPECT().RecordContainerReference(container),
			imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil),
			// Simulate successful create container
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(x, y, z, timeout interface{}) {
					eventsReported.Add(1)
					go func() {
						eventStream <- createDockerEvent(api.ContainerCreated)
						eventsReported.Done()
					}()
				}).Return(DockerContainerMetadata{DockerID: "containerId"}),
			// Simulate successful start container
			client.EXPECT().StartContainer("containerId", startContainerTimeout).Do(
				func(id string, timeout time.Duration) {
					eventsReported.Add(1)
					go func() {
						eventStream <- createDockerEvent(api.ContainerRunning)
						eventsReported.Done()
					}()
				}).Return(DockerContainerMetadata{DockerID: "containerId"}),
			// StopContainer errors out. However, since this is a known unretriable error,
			// the task engine should not retry stopping the container and move on.
			// If there's a delay in task engine's processing of the ContainerRunning
			// event, StopContainer will be invoked again as the engine considers it
			// as a stopped container coming back. MinTimes() should guarantee that
			// StopContainer is invoked at least once and in protecting agasint a test
			// failure when there's a delay in task engine processing the ContainerRunning
			// event.
			client.EXPECT().StopContainer("containerId", gomock.Any()).Return(DockerContainerMetadata{
				Error: CannotStopContainerError{&docker.ContainerNotRunning{}},
			}).MinTimes(1),
		)
	}

	err := taskEngine.Init(context.TODO())
	assert.NoError(t, err, "Error getting event streams from engine")
	stateChangeEvents := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(sleepTask)
	// wait for task running
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerRunning, "Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskRunning, "Expected task to be RUNNING")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}
	eventsReported.Wait()

	// Set the task desired status to be stopped and StopContainer will be called
	updateSleepTask := testdata.LoadTask("sleep5")
	updateSleepTask.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(updateSleepTask)

	// StopContainer was called again and received stop event from docker event stream
	// Expect it to go to stopped
	event = <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerStopped, "Expected container to be STOPPED")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to be STOPPED")

	select {
	case <-stateChangeEvents:
		t.Error("Should be out of events")
	default:
	}
}

// TestTaskTransitionWhenStopContainerReturnsTransientErrorBeforeSucceeding tests if the task
// transitions to stopped only after receiving the container stopped event from docker when
// the initial stop container call fails with an unknown error.
func TestTaskTransitionWhenStopContainerReturnsTransientErrorBeforeSucceeding(t *testing.T) {
	ctrl, client, mockTime, taskEngine, _, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	sleepTask := testdata.LoadTask("sleep5")
	eventStream := make(chan DockerContainerChangeEvent)

	client.EXPECT().Version()
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
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
			client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
				DockerContainerMetadata{DockerID: "containerId"}),
			// Simulate successful start container
			client.EXPECT().StartContainer("containerId", startContainerTimeout).Return(
				DockerContainerMetadata{DockerID: "containerId"}),
			// StopContainer errors out a couple of times
			client.EXPECT().StopContainer("containerId", gomock.Any()).Return(containerStoppingError).Times(2),
			// Since task is not in steady state, progressContainers causes
			// another invocation of StopContainer. Return the 'succeed' response,
			// which should cause the task engine to stop invoking this again and
			// transition the task to stopped.
			client.EXPECT().StopContainer("containerId", gomock.Any()).Return(DockerContainerMetadata{}),
		)
	}

	err := taskEngine.Init(context.TODO())
	assert.NoError(t, err, "Error getting event streams from engine")
	stateChangeEvents := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(sleepTask)
	// wait for task running

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerRunning, "Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskRunning, "Expected task to be RUNNING")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}

	// Set the task desired status to be stopped and StopContainer will be called
	updateSleepTask := testdata.LoadTask("sleep5")
	updateSleepTask.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(updateSleepTask)

	// StopContainer invocation should have caused it to stop eventually.
	event = <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, api.ContainerStopped, "Expected container to be STOPPED")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to be STOPPED")

	select {
	case <-stateChangeEvents:
		t.Error("Should be out of events")
	default:
	}
}

func TestCapabilities(t *testing.T) {
	conf := &config.Config{
		AvailableLoggingDrivers: []dockerclient.LoggingDriver{
			dockerclient.JSONFileDriver,
			dockerclient.SyslogDriver,
			dockerclient.JournaldDriver,
			dockerclient.GelfDriver,
			dockerclient.FluentdDriver,
		},
		PrivilegedDisabled:      false,
		SELinuxCapable:          true,
		AppArmorCapable:         true,
		TaskCleanupWaitDuration: config.DefaultConfig().TaskCleanupWaitDuration,
	}
	ctrl, client, _, taskEngine, _, _ := mocks(t, conf)
	defer ctrl.Finish()

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_17,
		dockerclient.Version_1_18,
	})

	client.EXPECT().KnownVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_17,
		dockerclient.Version_1_18,
		dockerclient.Version_1_19,
	})

	capabilities := taskEngine.Capabilities()

	expectedCapabilities := []string{
		"com.amazonaws.ecs.capability.privileged-container",
		"com.amazonaws.ecs.capability.docker-remote-api.1.17",
		"com.amazonaws.ecs.capability.docker-remote-api.1.18",
		"com.amazonaws.ecs.capability.logging-driver.json-file",
		"com.amazonaws.ecs.capability.logging-driver.syslog",
		"com.amazonaws.ecs.capability.logging-driver.journald",
		"com.amazonaws.ecs.capability.selinux",
		"com.amazonaws.ecs.capability.apparmor",
	}

	if !reflect.DeepEqual(capabilities, expectedCapabilities) {
		t.Errorf("Expected capabilities %v, but got capabilities %v", expectedCapabilities, capabilities)
	}
}

func TestCapabilitiesECR(t *testing.T) {
	conf := &config.Config{}
	ctrl, client, _, taskEngine, _, _ := mocks(t, conf)
	defer ctrl.Finish()

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)

	capabilities := taskEngine.Capabilities()

	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[capability] = true
	}

	_, ok := capMap["com.amazonaws.ecs.capability.ecr-auth"]
	assert.True(t, ok, "Could not find ECR capability when expected; got capabilities %v", capabilities)

}

func TestCapabilitiesTaskIAMRoleForSupportedDockerVersion(t *testing.T) {
	conf := &config.Config{
		TaskIAMRoleEnabled: true,
	}
	ctrl, client, _, taskEngine, _, _ := mocks(t, conf)
	defer ctrl.Finish()

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)

	capabilities := taskEngine.Capabilities()
	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[capability] = true
	}

	ok := capMap["com.amazonaws.ecs.capability.task-iam-role"]
	assert.True(t, ok, "Could not find iam capability when expected; got capabilities %v", capabilities)
}

func TestCapabilitiesTaskIAMRoleForUnSupportedDockerVersion(t *testing.T) {
	conf := &config.Config{
		TaskIAMRoleEnabled: true,
	}
	ctrl, client, _, taskEngine, _, _ := mocks(t, conf)
	defer ctrl.Finish()

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_18,
	})
	client.EXPECT().KnownVersions().Return(nil)

	capabilities := taskEngine.Capabilities()
	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[capability] = true
	}

	_, ok := capMap["com.amazonaws.ecs.capability.task-iam-role"]
	assert.False(t, ok, "task-iam-role capability set for unsupported docker version")
}

func TestCapabilitiesTaskIAMRoleNetworkHostForSupportedDockerVersion(t *testing.T) {
	conf := &config.Config{
		TaskIAMRoleEnabledForNetworkHost: true,
	}
	ctrl, client, _, taskEngine, _, _ := mocks(t, conf)
	defer ctrl.Finish()

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_19,
	})
	client.EXPECT().KnownVersions().Return(nil)

	capabilities := taskEngine.Capabilities()
	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[capability] = true
	}

	_, ok := capMap["com.amazonaws.ecs.capability.task-iam-role-network-host"]
	assert.True(t, ok, "Could not find iam capability when expected; got capabilities %v", capabilities)
}

func TestCapabilitiesTaskIAMRoleNetworkHostForUnSupportedDockerVersion(t *testing.T) {
	conf := &config.Config{
		TaskIAMRoleEnabledForNetworkHost: true,
	}
	ctrl, client, _, taskEngine, _, _ := mocks(t, conf)
	defer ctrl.Finish()

	client.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{
		dockerclient.Version_1_18,
	})
	client.EXPECT().KnownVersions().Return(nil)

	capabilities := taskEngine.Capabilities()
	capMap := make(map[string]bool)
	for _, capability := range capabilities {
		capMap[capability] = true
	}

	_, ok := capMap["com.amazonaws.ecs.capability.task-iam-role-network-host"]
	assert.False(t, ok, "task-iam-role capability set for unsupported docker version")
}

func TestGetTaskByArn(t *testing.T) {
	// Need a mock client as AddTask not only adds a task to the engine, but
	// also causes the engine to progress the task.

	ctrl, client, _, taskEngine, _, imageManager := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().Version()
	eventStream := make(chan DockerContainerChangeEvent)
	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	imageManager.EXPECT().RecordContainerReference(gomock.Any()).AnyTimes()
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).AnyTimes()
	client.EXPECT().PullImage(gomock.Any(), gomock.Any()).AnyTimes() // TODO change to MaxTimes(1)
	err := taskEngine.Init(context.TODO())
	assert.Nil(t, err)
	defer taskEngine.Disable()

	sleepTask := testdata.LoadTask("sleep5")
	sleepTaskArn := sleepTask.Arn
	taskEngine.AddTask(sleepTask)

	_, found := taskEngine.GetTaskByArn(sleepTaskArn)
	assert.True(t, found, "Task %s not found", sleepTaskArn)

	_, found = taskEngine.GetTaskByArn(sleepTaskArn + "arn")
	assert.False(t, found, "Task with invalid arn found in the task engine")
}

func TestEngineEnableConcurrentPull(t *testing.T) {
	ctrl, client, _, taskEngine, _, _ := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().Version().Return("1.11.1", nil)
	client.EXPECT().ContainerEvents(gomock.Any())
	err := taskEngine.Init(context.TODO())
	assert.Nil(t, err)

	dockerTaskEngine, _ := taskEngine.(*DockerTaskEngine)
	assert.True(t, dockerTaskEngine.enableConcurrentPull,
		"Task engine should be able to perform concurrent pulling for docker version >= 1.11.1")
}

func TestEngineDisableConcurrentPull(t *testing.T) {
	ctrl, client, _, taskEngine, _, _ := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().Version().Return("1.11.0", nil)
	client.EXPECT().ContainerEvents(gomock.Any())
	err := taskEngine.Init(context.TODO())
	if err != nil {
		t.Fatal(err)

	}

	dockerTaskEngine, _ := taskEngine.(*DockerTaskEngine)
	assert.False(t, dockerTaskEngine.enableConcurrentPull,
		"Task engine should not be able to perform concurrent pulling for version < 1.11.1")
}

// TestTaskWithCircularDependency tests the task with containers of which the
// dependencies can't be resolved
func TestTaskWithCircularDependency(t *testing.T) {
	ctrl, client, _, taskEngine, _, _ := mocks(t, &defaultConfig)
	defer ctrl.Finish()

	client.EXPECT().Version().Return("1.12.6", nil)
	client.EXPECT().ContainerEvents(gomock.Any())

	task := testdata.LoadTask("circular_dependency")

	taskEngine.Init(context.TODO())
	events := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(task)

	event := <-events
	assert.Equal(t, event.(api.TaskStateChange).Status, api.TaskStopped, "Expected task to move to stopped directly")
	_, ok := taskEngine.(*DockerTaskEngine).state.TaskByArn(task.Arn)
	assert.True(t, ok, "Task state should be added to the agent state")

	_, ok = taskEngine.(*DockerTaskEngine).managedTasks[task.Arn]
	assert.False(t, ok, "Task should not be added to task manager for processing")
}
