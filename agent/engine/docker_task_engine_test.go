// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package engine_test

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/golang/mock/gomock"
)

var test_time = ttime.NewTestTime()

func mocks(t *testing.T, cfg *config.Config) (*gomock.Controller, *mock_engine.MockDockerClient, engine.TaskEngine) {
	ctrl := gomock.NewController(t)
	client := mock_engine.NewMockDockerClient(ctrl)
	taskEngine := engine.NewTaskEngine(cfg)
	taskEngine.(*engine.DockerTaskEngine).SetDockerClient(client)
	return ctrl, client, taskEngine
}

func TestBatchContainerHappyPath(t *testing.T) {
	ctrl, client, taskEngine := mocks(t, &config.Config{})
	defer ctrl.Finish()
	ttime.SetTime(test_time)

	sleepTask := testdata.LoadTask("sleep5")

	eventStream := make(chan engine.DockerContainerChangeEvent)

	dockerEvent := func(status api.ContainerStatus) engine.DockerContainerChangeEvent {
		meta := engine.DockerContainerMetadata{
			DockerId: "containerId",
		}
		return engine.DockerContainerChangeEvent{Status: status, DockerContainerMetadata: meta}
	}

	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	for _, container := range sleepTask.Containers {

		client.EXPECT().PullImage(container.Image).Return(engine.DockerContainerMetadata{})

		dockerConfig, err := sleepTask.DockerConfig(container)
		if err != nil {
			t.Fatal(err)
		}
		client.EXPECT().CreateContainer(dockerConfig, gomock.Any(), gomock.Any()).Do(func(x, y, z interface{}) {
			go func() { eventStream <- dockerEvent(api.ContainerCreated) }()
		}).Return(engine.DockerContainerMetadata{DockerId: "containerId"})

		client.EXPECT().StartContainer("containerId").Do(func(id string) {
			go func() { eventStream <- dockerEvent(api.ContainerRunning) }()
		}).Return(engine.DockerContainerMetadata{DockerId: "containerId"})
	}

	err := taskEngine.Init()
	taskEvents, contEvents := taskEngine.TaskEvents()
	if err != nil {
		t.Fatal(err)
	}

	taskEngine.AddTask(sleepTask)

	if (<-contEvents).Status != api.ContainerRunning {
		t.Fatal("Expected container to run first")
	}
	if (<-taskEvents).Status != api.TaskRunning {
		t.Fatal("And then task")
	}
	select {
	case <-taskEvents:
		t.Fatal("Should be out of events")
	case <-contEvents:
		t.Fatal("Should be out of events")
	default:
	}

	exitCode := 0
	// And then docker reports that sleep died, as sleep is wont to do
	eventStream <- engine.DockerContainerChangeEvent{Status: api.ContainerStopped, DockerContainerMetadata: engine.DockerContainerMetadata{DockerId: "containerId", ExitCode: &exitCode}}

	if cont := <-contEvents; cont.Status != api.ContainerStopped {
		t.Fatal("Expected container to stop first")
		if *cont.ExitCode != 0 {
			t.Fatal("Exit code should be present")
		}
	}
	if (<-taskEvents).Status != api.TaskStopped {
		t.Fatal("And then task")
	}

	// Extra events should not block forever; duplicate acs and docker events are possible
	go func() { eventStream <- dockerEvent(api.ContainerStopped) }()
	go func() { eventStream <- dockerEvent(api.ContainerStopped) }()

	sleepTaskStop := testdata.LoadTask("sleep5")
	sleepTaskStop.DesiredStatus = api.TaskStopped
	taskEngine.AddTask(sleepTaskStop)
	// As above, duplicate events should not be a problem
	taskEngine.AddTask(sleepTaskStop)
	taskEngine.AddTask(sleepTaskStop)

	// Expect a bunch of steady state 'poll' describes when we warp 4 hours
	client.EXPECT().DescribeContainer(gomock.Any()).AnyTimes()
	client.EXPECT().RemoveContainer("containerId").Return(nil)

	test_time.Warp(4 * time.Hour)
	go func() { eventStream <- dockerEvent(api.ContainerStopped) }()

	for {
		tasks, _ := taskEngine.(*engine.DockerTaskEngine).ListTasks()
		if len(tasks) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStartTimeoutThenStart(t *testing.T) {
	ctrl, client, taskEngine := mocks(t, &config.Config{})
	defer ctrl.Finish()
	ttime.SetTime(test_time)

	sleepTask := testdata.LoadTask("sleep5")

	eventStream := make(chan engine.DockerContainerChangeEvent)

	dockerEvent := func(status api.ContainerStatus) engine.DockerContainerChangeEvent {
		meta := engine.DockerContainerMetadata{
			DockerId: "containerId",
		}
		return engine.DockerContainerChangeEvent{Status: status, DockerContainerMetadata: meta}
	}

	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	for _, container := range sleepTask.Containers {

		client.EXPECT().PullImage(container.Image).Return(engine.DockerContainerMetadata{})

		dockerConfig, err := sleepTask.DockerConfig(container)
		if err != nil {
			t.Fatal(err)
		}
		client.EXPECT().CreateContainer(dockerConfig, gomock.Any(), gomock.Any()).Do(func(x, y, z interface{}) {
			go func() { eventStream <- dockerEvent(api.ContainerCreated) }()
		}).Return(engine.DockerContainerMetadata{DockerId: "containerId"})

		client.EXPECT().StartContainer("containerId").Return(engine.DockerContainerMetadata{Error: &engine.DockerTimeoutError{}})

		// Expect it to try to stop the container before going on;
		// in the future the agent might optimize to not stop unless the known
		// status is running, at which poitn this can be safeuly removed
		client.EXPECT().StopContainer("containerId").Return(engine.DockerContainerMetadata{Error: errors.New("Cannot start")})
	}

	err := taskEngine.Init()
	taskEvents, contEvents := taskEngine.TaskEvents()
	if err != nil {
		t.Fatal(err)
	}

	taskEngine.AddTask(sleepTask)

	// Expect it to go to stopped
	contEvent := <-contEvents
	if contEvent.Status != api.ContainerStopped {
		t.Fatal("Expected container to timeout on start and stop")
	}
	*contEvent.SentStatus = api.ContainerStopped

	taskEvent := <-taskEvents
	if taskEvent.Status != api.TaskStopped {
		t.Fatal("And then task")
	}
	*taskEvent.SentStatus = api.TaskStopped
	select {
	case <-taskEvents:
		t.Fatal("Should be out of events")
	case <-contEvents:
		t.Fatal("Should be out of events")
	default:
	}

	// Expect it to try to stop it once now
	client.EXPECT().StopContainer("containerId").Return(engine.DockerContainerMetadata{Error: errors.New("Cannot start")})
	// Now surprise surprise, it actually did start!
	eventStream <- dockerEvent(api.ContainerRunning)

	// However, if it starts again, we should not see it be killed; no additional expect
	eventStream <- dockerEvent(api.ContainerRunning)
	eventStream <- dockerEvent(api.ContainerRunning)

	select {
	case <-taskEvents:
		t.Fatal("Should be out of events")
	case ev := <-contEvents:
		t.Fatal("Should be out of events", ev)
	default:
	}
}

func TestSteadyStatePoll(t *testing.T) {
	ctrl, client, taskEngine := mocks(t, &config.Config{})
	defer ctrl.Finish()
	ttime.SetTime(test_time)

	sleepTask := testdata.LoadTask("sleep5")

	eventStream := make(chan engine.DockerContainerChangeEvent)

	dockerEvent := func(status api.ContainerStatus) engine.DockerContainerChangeEvent {
		meta := engine.DockerContainerMetadata{
			DockerId: "containerId",
		}
		return engine.DockerContainerChangeEvent{Status: status, DockerContainerMetadata: meta}
	}

	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	for _, container := range sleepTask.Containers {

		client.EXPECT().PullImage(container.Image).Return(engine.DockerContainerMetadata{})

		dockerConfig, err := sleepTask.DockerConfig(container)
		if err != nil {
			t.Fatal(err)
		}
		client.EXPECT().CreateContainer(dockerConfig, gomock.Any(), gomock.Any()).Do(func(x, y, z interface{}) {
			go func() { eventStream <- dockerEvent(api.ContainerCreated) }()
		}).Return(engine.DockerContainerMetadata{DockerId: "containerId"})

		client.EXPECT().StartContainer("containerId").Do(func(id string) {
			go func() { eventStream <- dockerEvent(api.ContainerRunning) }()
		}).Return(engine.DockerContainerMetadata{DockerId: "containerId"})
	}

	err := taskEngine.Init()
	taskEvents, contEvents := taskEngine.TaskEvents()
	if err != nil {
		t.Fatal(err)
	}

	taskEngine.AddTask(sleepTask)

	if (<-contEvents).Status != api.ContainerRunning {
		t.Fatal("Expected container to run first")
	}
	if (<-taskEvents).Status != api.TaskRunning {
		t.Fatal("And then task")
	}
	select {
	case <-taskEvents:
		t.Fatal("Should be out of events")
	case <-contEvents:
		t.Fatal("Should be out of events")
	default:
	}

	// Two steady state oks, one stop
	gomock.InOrder(
		client.EXPECT().DescribeContainer("containerId").Return(api.ContainerRunning, engine.DockerContainerMetadata{DockerId: "containerId"}).Times(2),
		client.EXPECT().DescribeContainer("containerId").Return(api.ContainerStopped, engine.DockerContainerMetadata{DockerId: "containerId"}),
	)
	// Due to how the mock time works, we actually have to warp 10 minutes per
	// steady-state event. That's 10 per timeout + 10 per describe * 1 container.
	// The reason for this is the '.After' call happens regardless of whether the
	// value gets read, and the test sleep will add that time to elapsed, even
	// though in real-time-units that much time would not have elapsed.
	test_time.Warp(60 * time.Minute)

	contEvent := <-contEvents
	if contEvent.Status != api.ContainerStopped {
		t.Error("Expected container to be stopped")
	}
	if (<-taskEvents).Status != api.TaskStopped {
		t.Fatal("And then task")
	}
	select {
	case <-taskEvents:
		t.Fatal("Should be out of events")
	case <-contEvents:
		t.Fatal("Should be out of events")
	default:
	}
}

func TestStopWithPendingStops(t *testing.T) {
	ctrl, client, taskEngine := mocks(t, &config.Config{})
	defer ctrl.Finish()
	ttime.SetTime(test_time)

	sleepTask1 := testdata.LoadTask("sleep5")
	sleepTask1.StartSequenceNumber = 5
	sleepTask2 := testdata.LoadTask("sleep5")
	sleepTask2.Arn = "arn2"

	eventStream := make(chan engine.DockerContainerChangeEvent)

	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)

	err := taskEngine.Init()
	if err != nil {
		t.Fatal(err)
	}
	taskEvents, contEvents := taskEngine.TaskEvents()
	go func() {
		for {
			<-taskEvents
		}
	}()
	go func() {
		for {
			<-contEvents
		}
	}()

	pulling := make(chan bool)
	client.EXPECT().PullImage(gomock.Any()).Do(func(x interface{}) {
		<-pulling
	})
	taskEngine.AddTask(sleepTask2)
	stopSleep2 := *sleepTask2
	stopSleep2.DesiredStatus = api.TaskStopped
	stopSleep2.StopSequenceNumber = 4
	taskEngine.AddTask(&stopSleep2)

	taskEngine.AddTask(sleepTask1)
	stopSleep1 := *sleepTask1
	stopSleep1.DesiredStatus = api.TaskStopped
	stopSleep1.StopSequenceNumber = 5
	taskEngine.AddTask(&stopSleep1)
	pulling <- true
	// If we get here without deadlocking, we passed the test
}
