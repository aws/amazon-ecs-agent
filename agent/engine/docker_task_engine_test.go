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
	"reflect"
	"testing"
	"time"

	"code.google.com/p/gomock/gomock"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/fsouza/go-dockerclient"
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
		return engine.DockerContainerChangeEvent{status, meta}
	}

	client.EXPECT().ContainerEvents(gomock.Any()).Return(eventStream, nil)
	for _, container := range sleepTask.Containers {

		client.EXPECT().PullImage(container.Image).Return(engine.DockerContainerMetadata{})

		dockerConfig, err := sleepTask.DockerConfig(container)
		if err != nil {
			t.Fatal(err)
		}
		client.EXPECT().CreateContainer(dockerConfig, gomock.Any()).Do(func(x, y interface{}) {
			go func() { eventStream <- dockerEvent(api.ContainerCreated) }()
		}).Return(engine.DockerContainerMetadata{DockerId: "containerId"})

		client.EXPECT().StartContainer("containerId", gomock.Any()).Do(func(id string, hostConfig *docker.HostConfig) {
			containerMapByArn, _ := taskEngine.(*engine.DockerTaskEngine).State().ContainerMapByArn(sleepTask.Arn)
			computedHostConfig, _ := sleepTask.DockerHostConfig(container, containerMapByArn)
			if !reflect.DeepEqual(hostConfig, computedHostConfig) {
				t.Fatal("Host config mismatch")
			}

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
	eventStream <- engine.DockerContainerChangeEvent{api.ContainerStopped, engine.DockerContainerMetadata{DockerId: "containerId", ExitCode: &exitCode}}

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
