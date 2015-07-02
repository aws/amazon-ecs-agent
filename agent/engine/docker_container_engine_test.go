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
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	docker "github.com/fsouza/go-dockerclient"

	"github.com/golang/mock/gomock"
)

func dockerclientSetup(t *testing.T) (*mock_dockerclient.MockClient, *DockerGoClient, *ttime.TestTime, func()) {
	ctrl := gomock.NewController(t)
	mockDocker := mock_dockerclient.NewMockClient(ctrl)
	client := &DockerGoClient{}
	client.SetGoDockerClient(mockDocker)
	testTime := ttime.NewTestTime()
	ttime.SetTime(testTime)
	return mockDocker, client, testTime, ctrl.Finish
}

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
	mockDocker, client, testTime, done := dockerclientSetup(t)
	defer done()

	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image:latest"}, gomock.Any()).Do(func(x, y interface{}) {
		testTime.Warp(3 * time.Hour)
		wait.Wait()
		// Don't return, verify timeout happens
	})

	metadata := client.PullImage("image")
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(api.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	// cleanup
	wait.Done()
}

func TestPullImage(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image:latest"}, gomock.Any()).Return(nil)

	metadata := client.PullImage("image")
	if metadata.Error != nil {
		t.Error("Expected pull to succeed")
	}
}

func TestPullEmptyvolumeImage(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

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
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

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
	mockDocker, client, testTime, done := dockerclientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	config := docker.CreateContainerOptions{Config: &docker.Config{Memory: 100}, Name: "containerName"}
	mockDocker.EXPECT().CreateContainer(config).Do(func(x interface{}) {
		testTime.Warp(createContainerTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	metadata := client.CreateContainer(config.Config, nil, config.Name)
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(api.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestCreateContainer(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

	config := docker.CreateContainerOptions{Config: &docker.Config{Memory: 100}, Name: "containerName"}
	gomock.InOrder(
		mockDocker.EXPECT().CreateContainer(config).Return(&docker.Container{ID: "id"}, nil),
		mockDocker.EXPECT().InspectContainer("id").Return(&docker.Container{ID: "id"}, nil),
	)
	metadata := client.CreateContainer(config.Config, nil, config.Name)
	if metadata.Error != nil {
		t.Error("Did not expect error")
	}
	if metadata.DockerId != "id" {
		t.Error("Wrong id")
	}
}

func TestStartContainerTimeout(t *testing.T) {
	mockDocker, client, testTime, done := dockerclientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().StartContainer("id", nil).Do(func(x, y interface{}) {
		testTime.Warp(startContainerTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	metadata := client.StartContainer("id")
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(api.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestStartContainer(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

	gomock.InOrder(
		mockDocker.EXPECT().StartContainer("id", nil).Return(nil),
		mockDocker.EXPECT().InspectContainer("id").Return(&docker.Container{ID: "id"}, nil),
	)
	metadata := client.StartContainer("id")
	if metadata.Error != nil {
		t.Error("Did not expect error")
	}
	if metadata.DockerId != "id" {
		t.Error("Wrong id")
	}
}

func TestStopContainerTimeout(t *testing.T) {
	mockDocker, client, testTime, done := dockerclientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().StopContainer("id", uint(dockerStopTimeoutSeconds)).Do(func(x, y interface{}) {
		testTime.Warp(stopContainerTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	metadata := client.StopContainer("id")
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(api.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestStopContainer(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

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
	mockDocker, client, testTime, done := dockerclientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().InspectContainer("id").Do(func(x interface{}) {
		testTime.Warp(inspectContainerTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	_, err := client.InspectContainer("id")
	if err == nil {
		t.Error("Expected error for pull timeout")
	}
	if err.(api.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestInspectContainer(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

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

func TestContainerEvents(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

	var events chan<- *docker.APIEvents
	mockDocker.EXPECT().AddEventListener(gomock.Any()).Do(func(x interface{}) {
		events = x.(chan<- *docker.APIEvents)
	})

	dockerEvents, err := client.ContainerEvents(context.TODO())
	if err != nil {
		t.Fatal("Could not get container events")
	}

	mockDocker.EXPECT().InspectContainer("containerId").Return(&docker.Container{ID: "containerId"}, nil)
	go func() {
		events <- &docker.APIEvents{ID: "containerId", Status: "create"}
	}()

	event := <-dockerEvents
	if event.DockerId != "containerId" {
		t.Error("Wrong docker id")
	}
	if event.Status != api.ContainerCreated {
		t.Error("Wrong status")
	}

	container := &docker.Container{
		ID: "cid2",
		NetworkSettings: &docker.NetworkSettings{
			Ports: map[docker.Port][]docker.PortBinding{
				"80/tcp": []docker.PortBinding{docker.PortBinding{HostPort: "9001"}},
			},
		},
		Volumes: map[string]string{"/host/path": "/container/path"},
	}
	mockDocker.EXPECT().InspectContainer("cid2").Return(container, nil)
	go func() {
		events <- &docker.APIEvents{ID: "cid2", Status: "start"}
	}()
	event = <-dockerEvents
	if event.DockerId != "cid2" {
		t.Error("Wrong docker id")
	}
	if event.Status != api.ContainerRunning {
		t.Error("Wrong status")
	}
	if event.PortBindings[0].ContainerPort != 80 || event.PortBindings[0].HostPort != 9001 {
		t.Error("Incorrect port bindings")
	}
	if event.Volumes["/host/path"] != "/container/path" {
		t.Error("Incorrect volume mapping")
	}

	for i := 0; i < 3; i++ {
		stoppedContainer := &docker.Container{
			ID: "cid3" + strconv.Itoa(i),
			State: docker.State{
				ExitCode: 20,
			},
		}
		mockDocker.EXPECT().InspectContainer("cid3"+strconv.Itoa(i)).Return(stoppedContainer, nil)
	}
	go func() {
		events <- &docker.APIEvents{ID: "cid30", Status: "stop"}
		events <- &docker.APIEvents{ID: "cid31", Status: "die"}
		events <- &docker.APIEvents{ID: "cid32", Status: "kill"}
	}()

	for i := 0; i < 3; i++ {
		anEvent := <-dockerEvents
		if anEvent.DockerId != "cid3"+strconv.Itoa(i) {
			t.Error("Wrong container id: " + anEvent.DockerId)
		}
		if anEvent.Status != api.ContainerStopped {
			t.Error("Should be stopped")
		}
		if *anEvent.ExitCode != 20 {
			t.Error("Incorrect exit code")
		}
	}

	// Verify the following events do not translate into our event stream
	for _, eventStatus := range []string{"pause", "export", "pull", "untag", "delete"} {
		events <- &docker.APIEvents{ID: "123", Status: eventStatus}
		select {
		case <-dockerEvents:
			t.Error("No event should be available for " + eventStatus)
		default:
		}
	}
}

func TestDockerVersion(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

	mockDocker.EXPECT().Version().Return(&docker.Env{"Version=1.6.0"}, nil)

	str, err := client.Version()
	if err != nil {
		t.Error(err)
	}
	if str != "DockerVersion: 1.6.0" {
		t.Error("Got unexpected version string: " + str)
	}
}

func TestListImages(t *testing.T) {
	mockDocker, client, _, done := dockerclientSetup(t)
	defer done()

	containers := []docker.APIContainers{docker.APIContainers{ID: "id"}}
	mockDocker.EXPECT().ListContainers(gomock.Any()).Return(containers, nil)
	response := client.ListContainers(true)
	if response.Error != nil {
		t.Error("Did not expect error")
	}

	containerIds := response.DockerIds
	if len(containerIds) != 1 {
		t.Error("Unexpected number of containers in list: ", len(containerIds))
	}

	if containerIds[0] != "id" {
		t.Error("Unexpected container id in the list: ", containerIds[0])
	}
}

func TestListImagesTimeout(t *testing.T) {
	mockDocker, client, testTime, done := dockerclientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().ListContainers(gomock.Any()).Do(func(x interface{}) {
		testTime.Warp(listContainersTimeout)
		wait.Wait()
		// Don't return, verify timeout happens
	})
	response := client.ListContainers(true)
	if response.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if response.Error.(api.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}
