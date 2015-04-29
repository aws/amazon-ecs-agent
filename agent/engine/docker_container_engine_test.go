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

package engine

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/fsouza/go-dockerclient"

	"code.google.com/p/gomock/gomock"
)

type pullImageOptsMatcher struct {
	image string
}

func (matcher *pullImageOptsMatcher) String() string {
	return "matches " + matcher.image
}

func (matcher *pullImageOptsMatcher) Matches(x interface{}) bool {
	return matcher.image == x.(docker.PullImageOptions).Repository
}

func TestPullImageTimeout(t *testing.T) {
	test_time := ttime.NewTestTime()
	ttime.SetTime(test_time)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image"}, docker.AuthConfiguration{}).Do(func(x, y interface{}) {
		test_time.Warp(3 * time.Hour)
		wait.Wait()
		// Don't return, verify timeout happens
	})

	metadata := client.PullImage("image")
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(NamedError).Name() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	// cleanup
	wait.Done()
}

func TestPullImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)
	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image"}, docker.AuthConfiguration{}).Return(nil)

	metadata := client.PullImage("image")
	if metadata.Error != nil {
		t.Error("Expected pull to succeed")
	}
}

func TestPullEmptyvolumeImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)
	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	// The special emptyvolume image leads to a create, not pull

	gomock.InOrder(
		mockDocker.EXPECT().InspectImage(emptyvolume.Image+":"+emptyvolume.Tag).Return(nil, errors.New("Does not exist")),
		mockDocker.EXPECT().ImportImage(gomock.Any()).Do(func(x interface{}) {
			req := x.(docker.ImportImageOptions)
			if req.Repository != emptyvolume.Image {
				t.Fatal("Expected empty volume repository")
			}
			if req.Tag != emptyvolume.Tag {
				t.Fatal("Expected empty volume repository")
			}
		}),
	)

	metadata := client.PullImage(emptyvolume.Image + ":" + emptyvolume.Tag)
	if metadata.Error != nil {
		t.Error(metadata.Error)
	}
}

func TestPullExistingEmptyvolumeImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)
	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	// The special emptyvolume image leads to a create only if it doesn't exist
	gomock.InOrder(
		mockDocker.EXPECT().InspectImage(emptyvolume.Image+":"+emptyvolume.Tag).Return(&docker.Image{}, nil),
	)

	metadata := client.PullImage(emptyvolume.Image + ":" + emptyvolume.Tag)
	if metadata.Error != nil {
		t.Error(metadata.Error)
	}
}

func TestCreateContainerTimeout(t *testing.T) {
	test_time := ttime.NewTestTime()
	ttime.SetTime(test_time)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	wait := &sync.WaitGroup{}
	wait.Add(1)
	config := docker.CreateContainerOptions{Config: &docker.Config{Memory: 100}, Name: "containerName"}
	mockDocker.EXPECT().CreateContainer(config).Do(func(x interface{}) {
		test_time.Warp(createContainerTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	metadata := client.CreateContainer(config.Config, config.Name)
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(NamedError).Name() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestCreateContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	config := docker.CreateContainerOptions{Config: &docker.Config{Memory: 100}, Name: "containerName"}
	gomock.InOrder(
		mockDocker.EXPECT().CreateContainer(config).Return(&docker.Container{ID: "id"}, nil),
		mockDocker.EXPECT().InspectContainer("id").Return(&docker.Container{ID: "id"}, nil),
	)
	metadata := client.CreateContainer(config.Config, config.Name)
	if metadata.Error != nil {
		t.Error("Did not expect error")
	}
	if metadata.DockerId != "id" {
		t.Error("Wrong id")
	}
}

func TestStartContainerTimeout(t *testing.T) {
	test_time := ttime.NewTestTime()
	ttime.SetTime(test_time)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().StartContainer("id", &docker.HostConfig{}).Do(func(x, y interface{}) {
		test_time.Warp(startContainerTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	metadata := client.StartContainer("id", &docker.HostConfig{})
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(NamedError).Name() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestStartContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	hostConfig := &docker.HostConfig{}
	gomock.InOrder(
		mockDocker.EXPECT().StartContainer("id", hostConfig).Return(nil),
		mockDocker.EXPECT().InspectContainer("id").Return(&docker.Container{ID: "id"}, nil),
	)
	metadata := client.StartContainer("id", hostConfig)
	if metadata.Error != nil {
		t.Error("Did not expect error")
	}
	if metadata.DockerId != "id" {
		t.Error("Wrong id")
	}
}

func TestStopContainerTimeout(t *testing.T) {
	test_time := ttime.NewTestTime()
	ttime.SetTime(test_time)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().StopContainer("id", uint(dockerStopTimeoutSeconds)).Do(func(x, y interface{}) {
		test_time.Warp(stopContainerTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	metadata := client.StopContainer("id")
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(NamedError).Name() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestStopContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	gomock.InOrder(
		mockDocker.EXPECT().StopContainer("id", uint(dockerStopTimeoutSeconds)).Return(nil),
		mockDocker.EXPECT().InspectContainer("id").Return(&docker.Container{ID: "id", State: docker.State{ExitCode: 10}}, nil),
	)
	metadata := client.StopContainer("id")
	if metadata.Error != nil {
		t.Error("Did not expect error")
	}
	if metadata.DockerId != "id" {
		t.Error("Wrong id")
	}
}

func TestInspectContainerTimeout(t *testing.T) {
	test_time := ttime.NewTestTime()
	ttime.SetTime(test_time)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().InspectContainer("id").Do(func(x interface{}) {
		test_time.Warp(inspectContainerTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	_, err := client.InspectContainer("id")
	if err == nil {
		t.Error("Expected error for pull timeout")
	}
	if err.(NamedError).Name() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestInspectContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockerclient.NewMockClient(ctrl)

	client := DockerGoClient{}
	client.SetGoDockerClient(mockDocker)

	containerOutput := docker.Container{ID: "id", State: docker.State{ExitCode: 10}}
	gomock.InOrder(
		mockDocker.EXPECT().InspectContainer("id").Return(&containerOutput, nil),
	)
	container, err := client.InspectContainer("id")
	if err != nil {
		t.Error("Did not expect error")
	}
	if !reflect.DeepEqual(&containerOutput, container) {
		t.Fatal("Did not match expected output")
	}
}
