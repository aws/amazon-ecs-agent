// +build unit

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

package dockerapi

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/clientfactory/mocks"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockeriface/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ecr/mocks"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/amazon-ecs-agent/agent/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"

	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xContainerShortTimeout is a short duration intended to be used by the
// docker client APIs that test if the underlying context gets canceled
// upon the expiration of the timeout duration.
const xContainerShortTimeout = 1 * time.Millisecond

func defaultTestConfig() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
	return cfg
}

func dockerClientSetup(t *testing.T) (
	*mock_dockeriface.MockClient,
	*dockerGoClient,
	*mock_ttime.MockTime,
	*gomock.Controller,
	*mock_ecr.MockECRFactory,
	func()) {
	return dockerClientSetupWithConfig(t, config.DefaultConfig())
}

func dockerClientSetupWithConfig(t *testing.T, conf config.Config) (
	*mock_dockeriface.MockClient,
	*dockerGoClient,
	*mock_ttime.MockTime,
	*gomock.Controller,
	*mock_ecr.MockECRFactory,
	func()) {
	ctrl := gomock.NewController(t)
	mockDocker := mock_dockeriface.NewMockClient(ctrl)
	mockDocker.EXPECT().Ping().AnyTimes().Return(nil)
	factory := mock_clientfactory.NewMockFactory(ctrl)
	factory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDocker, nil)
	mockTime := mock_ttime.NewMockTime(ctrl)

	conf.EngineAuthData = config.NewSensitiveRawMessage([]byte{})
	client, _ := NewDockerGoClient(factory, &conf)
	goClient, _ := client.(*dockerGoClient)
	ecrClientFactory := mock_ecr.NewMockECRFactory(ctrl)
	goClient.ecrClientFactory = ecrClientFactory
	goClient._time = mockTime
	return mockDocker, goClient, mockTime, ctrl, ecrClientFactory, ctrl.Finish
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

func TestPullImageOutputTimeout(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	pullBeginTimeout := make(chan time.Time)
	testTime.EXPECT().After(dockerPullBeginTimeout).Return(pullBeginTimeout).MinTimes(1)
	testTime.EXPECT().After(pullImageTimeout).MinTimes(1)
	wait := sync.WaitGroup{}
	wait.Add(1)
	// multiple invocations will happen due to retries, but all should timeout
	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image:latest"}, gomock.Any()).Do(
		func(x, y interface{}) {
			pullBeginTimeout <- time.Now()
			wait.Wait()
			// Don't return, verify timeout happens
		}).Times(maximumPullRetries) // expected number of retries

	metadata := client.PullImage("image", nil)
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(apierrors.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}

	// cleanup
	wait.Done()
}

func TestPullImageGlobalTimeout(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	pullBeginTimeout := make(chan time.Time, 1)
	testTime.EXPECT().After(dockerPullBeginTimeout).Return(pullBeginTimeout)
	pullTimeout := make(chan time.Time, 1)
	testTime.EXPECT().After(pullImageTimeout).Return(pullTimeout)
	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image:latest"}, gomock.Any()).Do(func(x, y interface{}) {
		opts, ok := x.(docker.PullImageOptions)
		if !ok {
			t.Error("Cannot cast argument to PullImageOptions")
		}
		io.WriteString(opts.OutputStream, "string\n")
		pullBeginTimeout <- time.Now()
		pullTimeout <- time.Now()
		wait.Wait()
		// Don't return, verify timeout happens
	})

	metadata := client.PullImage("image", nil)
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(apierrors.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}

	testTime.EXPECT().After(dockerPullBeginTimeout)
	testTime.EXPECT().After(pullImageTimeout)
	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image2:latest"}, gomock.Any())
	_ = client.PullImage("image2", nil)

	// cleanup
	wait.Done()
}

func TestPullImageInactivityTimeout(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image:latest"}, gomock.Any()).Return(
		docker.ErrInactivityTimeout).Times(maximumPullRetries) // expected number of retries

	metadata := client.PullImage("image", nil)
	assert.Error(t, metadata.Error, "Expected error for pull inactivity timeout")
	assert.Equal(t, "CannotPullContainerError", metadata.Error.(apierrors.NamedError).ErrorName(), "Wrong error type")
}

func TestPullImage(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image:latest"}, gomock.Any()).Return(nil)

	metadata := client.PullImage("image", nil)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestPullImageTag(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDocker.EXPECT().PullImage(&pullImageOptsMatcher{"image:mytag"}, gomock.Any()).Return(nil)

	metadata := client.PullImage("image:mytag", nil)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestPullImageDigest(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDocker.EXPECT().PullImage(
		&pullImageOptsMatcher{"image@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb"},
		gomock.Any(),
	).Return(nil)

	metadata := client.PullImage("image@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb", nil)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestPullImageECRSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockeriface.NewMockClient(ctrl)
	mockDocker.EXPECT().Ping().AnyTimes().Return(nil)
	factory := mock_clientfactory.NewMockFactory(ctrl)
	factory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDocker, nil)
	client, _ := NewDockerGoClient(factory, defaultTestConfig())
	goClient, _ := client.(*dockerGoClient)
	ecrClientFactory := mock_ecr.NewMockECRFactory(ctrl)
	ecrClient := mock_ecr.NewMockECRClient(ctrl)
	mockTime := mock_ttime.NewMockTime(ctrl)
	goClient.ecrClientFactory = ecrClientFactory
	goClient._time = mockTime

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()

	registryID := "123456789012"
	region := "eu-west-1"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	username := "username"
	password := "password"
	dockerAuthConfiguration := docker.AuthConfiguration{
		Username:      username,
		Password:      password,
		ServerAddress: "https://" + imageEndpoint,
	}

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
		}, nil)

	mockDocker.EXPECT().PullImage(
		&pullImageOptsMatcher{image},
		dockerAuthConfiguration,
	).Return(nil)

	metadata := client.PullImage(image, authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestPullImageECRAuthFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockeriface.NewMockClient(ctrl)
	mockDocker.EXPECT().Ping().AnyTimes().Return(nil)
	factory := mock_clientfactory.NewMockFactory(ctrl)
	factory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDocker, nil)
	client, _ := NewDockerGoClient(factory, defaultTestConfig())
	goClient, _ := client.(*dockerGoClient)
	ecrClientFactory := mock_ecr.NewMockECRFactory(ctrl)
	ecrClient := mock_ecr.NewMockECRClient(ctrl)
	mockTime := mock_ttime.NewMockTime(ctrl)
	goClient.ecrClientFactory = ecrClientFactory
	goClient._time = mockTime

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()

	registryID := "123456789012"
	region := "eu-west-1"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"

	// no retries for this error
	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil)
	ecrClient.EXPECT().GetAuthorizationToken(gomock.Any()).Return(nil, errors.New("test error"))

	metadata := client.PullImage(image, authData)
	assert.Error(t, metadata.Error, "expected pull to fail")
}

func TestGetRepositoryWithTaggedImage(t *testing.T) {
	image := "registry.endpoint/myimage:tag"
	repository := getRepository(image)

	assert.Equal(t, image, repository)
}

func TestGetRepositoryWithUntaggedImage(t *testing.T) {
	image := "registry.endpoint/myimage"
	repository := getRepository(image)

	assert.Equal(t, image+":"+dockerDefaultTag, repository)
}

func TestImportLocalEmptyVolumeImage(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	// The special emptyvolume image leads to a create, not pull
	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	gomock.InOrder(
		mockDocker.EXPECT().InspectImage(emptyvolume.Image+":"+emptyvolume.Tag).Return(nil, errors.New("Does not exist")),
		mockDocker.EXPECT().ImportImage(gomock.Any()).Do(func(x interface{}) {
			req := x.(docker.ImportImageOptions)
			require.Equal(t, emptyvolume.Image, req.Repository, "expected empty volume repository")
			require.Equal(t, emptyvolume.Tag, req.Tag, "expected empty volume tag")
		}),
	)

	metadata := client.ImportLocalEmptyVolumeImage()
	assert.NoError(t, metadata.Error, "Expected import to succeed")
}

func TestImportLocalEmptyVolumeImageExisting(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	// The special emptyvolume image leads to a create only if it doesn't exist
	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	gomock.InOrder(
		mockDocker.EXPECT().InspectImage(emptyvolume.Image+":"+emptyvolume.Tag).Return(&docker.Image{}, nil),
	)

	metadata := client.ImportLocalEmptyVolumeImage()
	assert.NoError(t, metadata.Error, "Expected import to succeed")
}

func TestCreateContainerTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	config := docker.CreateContainerOptions{Config: &docker.Config{Memory: 100}, Name: "containerName"}
	mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(x interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(nil, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.CreateContainer(ctx, config.Config, nil, config.Name, xContainerShortTimeout)
	assert.Error(t, metadata.Error, "expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestCreateContainerInspectTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	config := docker.CreateContainerOptions{Config: &docker.Config{Memory: 100}, Name: "containerName"}
	gomock.InOrder(
		mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(opts docker.CreateContainerOptions) {
			if !reflect.DeepEqual(opts.Config, config.Config) {
				t.Errorf("Mismatch in create container config, %v != %v", opts.Config, config.Config)
			}
			if opts.Name != config.Name {
				t.Errorf("Mismatch in create container options, %s != %s", opts.Name, config.Name)
			}
		}).Return(&docker.Container{ID: "id"}, nil),
		mockDocker.EXPECT().InspectContainerWithContext("id", gomock.Any()).Return(nil, &DockerTimeoutError{}),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.CreateContainer(ctx, config.Config, nil, config.Name, 1*time.Second)
	if metadata.DockerID != "id" {
		t.Error("Expected ID to be set even if inspect failed; was " + metadata.DockerID)
	}
	if metadata.Error == nil {
		t.Error("Expected error for inspect timeout")
	}
}

func TestCreateContainer(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	config := docker.CreateContainerOptions{Config: &docker.Config{Memory: 100}, Name: "containerName"}
	gomock.InOrder(
		mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(opts docker.CreateContainerOptions) {
			if !reflect.DeepEqual(opts.Config, config.Config) {
				t.Errorf("Mismatch in create container config, %v != %v", opts.Config, config.Config)
			}
			if opts.Name != config.Name {
				t.Errorf("Mismatch in create container options, %s != %s", opts.Name, config.Name)
			}
		}).Return(&docker.Container{ID: "id"}, nil),
		mockDocker.EXPECT().InspectContainerWithContext("id", gomock.Any()).Return(&docker.Container{ID: "id"}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.CreateContainer(ctx, config.Config, nil, config.Name, 1*time.Second)
	if metadata.Error != nil {
		t.Error("Did not expect error")
	}
	if metadata.DockerID != "id" {
		t.Error("Wrong id")
	}
	if metadata.ExitCode != nil {
		t.Error("Expected a created container to not have an exit code")
	}
}

func TestStartContainerTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().StartContainerWithContext("id", nil, gomock.Any()).Do(func(x, y, z interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)
	mockDocker.EXPECT().InspectContainerWithContext("id", gomock.Any()).Return(nil, errors.New("test error")).AnyTimes()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StartContainer(ctx, "id", xContainerShortTimeout)
	assert.NotNil(t, metadata.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestStartContainer(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	gomock.InOrder(
		mockDocker.EXPECT().StartContainerWithContext("id", nil, gomock.Any()).Return(nil),
		mockDocker.EXPECT().InspectContainerWithContext("id", gomock.Any()).Return(&docker.Container{ID: "id"}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StartContainer(ctx, "id", defaultTestConfig().ContainerStartTimeout)
	if metadata.Error != nil {
		t.Error("Did not expect error")
	}
	if metadata.DockerID != "id" {
		t.Error("Wrong id")
	}
}

func TestStopContainerTimeout(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DockerStopTimeout = xContainerShortTimeout
	mockDocker, client, _, _, _, done := dockerClientSetupWithConfig(t, cfg)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().StopContainerWithContext("id", uint(client.config.DockerStopTimeout/time.Second), gomock.Any()).Do(func(x, y, z interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(errors.New("test error"))
	mockDocker.EXPECT().InspectContainerWithContext(gomock.Any(), gomock.Any()).AnyTimes()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StopContainer(ctx, "id", xContainerShortTimeout)
	if metadata.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if metadata.Error.(apierrors.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestStopContainer(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	gomock.InOrder(
		mockDocker.EXPECT().StopContainerWithContext("id", uint(client.config.DockerStopTimeout/time.Second), gomock.Any()).Return(nil),
		mockDocker.EXPECT().InspectContainerWithContext("id", gomock.Any()).Return(&docker.Container{ID: "id", State: docker.State{ExitCode: 10}}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StopContainer(ctx, "id", dockerclient.StopContainerTimeout)
	if metadata.Error != nil {
		t.Error("Did not expect error")
	}
	if metadata.DockerID != "id" {
		t.Error("Wrong id")
	}
}

func TestInspectContainerTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().InspectContainerWithContext("id", gomock.Any()).Do(func(x, ctx interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(nil, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := client.InspectContainer(ctx, "id", xContainerShortTimeout)
	if err == nil {
		t.Error("Expected error for inspect timeout")
	}
	if err.(apierrors.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestInspectContainer(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	containerOutput := docker.Container{ID: "id",
		State: docker.State{
			ExitCode: 10,
			Health: docker.Health{
				Status: "healthy",
				Log: []docker.HealthCheck{
					{
						ExitCode: 1,
						Output:   "health output",
					},
				},
			}}}
	gomock.InOrder(
		mockDocker.EXPECT().InspectContainerWithContext("id", gomock.Any()).Return(&containerOutput, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	container, err := client.InspectContainer(ctx, "id", dockerclient.InspectContainerTimeout)
	if err != nil {
		t.Error("Did not expect error")
	}
	if !reflect.DeepEqual(&containerOutput, container) {
		t.Fatal("Did not match expected output")
	}
}

func TestContainerEvents(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	var events chan<- *docker.APIEvents
	mockDocker.EXPECT().AddEventListener(gomock.Any()).Do(func(x interface{}) {
		events = x.(chan<- *docker.APIEvents)
	})

	dockerEvents, err := client.ContainerEvents(context.TODO())
	require.NoError(t, err, "Could not get container events")

	mockDocker.EXPECT().InspectContainerWithContext("containerId", gomock.Any()).Return(
		&docker.Container{
			ID: "containerId",
		},
		nil)
	go func() {
		events <- &docker.APIEvents{Type: "container", ID: "containerId", Status: "create"}
	}()

	event := <-dockerEvents
	assert.Equal(t, event.DockerID, "containerId", "Wrong docker id")
	assert.Equal(t, event.Status, apicontainerstatus.ContainerCreated, "Wrong status")

	container := &docker.Container{
		ID: "cid2",
		NetworkSettings: &docker.NetworkSettings{
			Ports: map[docker.Port][]docker.PortBinding{
				"80/tcp": {{HostPort: "9001"}},
			},
		},
		Volumes: map[string]string{"/host/path": "/container/path"},
	}
	mockDocker.EXPECT().InspectContainerWithContext("cid2", gomock.Any()).Return(container, nil)
	go func() {
		events <- &docker.APIEvents{Type: "container", ID: "cid2", Status: "start"}
	}()
	event = <-dockerEvents
	assert.Equal(t, event.DockerID, "cid2", "Wrong docker id")
	assert.Equal(t, event.Status, apicontainerstatus.ContainerRunning, "Wrong status")
	assert.Equal(t, event.PortBindings[0].ContainerPort, uint16(80), "Incorrect port bindings")
	assert.Equal(t, event.PortBindings[0].HostPort, uint16(9001), "Incorrect port bindings")
	assert.Equal(t, event.Volumes["/host/path"], "/container/path", "Incorrect volume mapping")

	for i := 0; i < 2; i++ {
		stoppedContainer := &docker.Container{
			ID: "cid3" + strconv.Itoa(i),
			State: docker.State{
				FinishedAt: time.Now(),
				ExitCode:   20,
			},
		}
		mockDocker.EXPECT().InspectContainerWithContext("cid3"+strconv.Itoa(i), gomock.Any()).Return(stoppedContainer, nil)
	}
	go func() {
		events <- &docker.APIEvents{Type: "container", ID: "cid30", Status: "stop"}
		events <- &docker.APIEvents{Type: "container", ID: "cid31", Status: "die"}
	}()

	for i := 0; i < 2; i++ {
		anEvent := <-dockerEvents
		assert.True(t, anEvent.DockerID == "cid30" || anEvent.DockerID == "cid31", "Wrong container id: "+anEvent.DockerID)
		assert.Equal(t, anEvent.Status, apicontainerstatus.ContainerStopped, "Should be stopped")
		assert.Equal(t, aws.IntValue(anEvent.ExitCode), 20, "Incorrect exit code")
	}

	containerWithHealthInfo := &docker.Container{
		ID: "container_health",
		State: docker.State{
			Health: docker.Health{
				Status: "healthy",
				Log: []docker.HealthCheck{
					{
						ExitCode: 1,
						Output:   "health output",
					},
				},
			},
		},
	}
	mockDocker.EXPECT().InspectContainerWithContext("container_health", gomock.Any()).Return(containerWithHealthInfo, nil)
	go func() {
		events <- &docker.APIEvents{
			Type:   "container",
			ID:     "container_health",
			Action: "health_status: unhealthy",
			Status: "health_status: unhealthy",
			Actor: docker.APIActor{
				ID: "container_health",
			},
		}
	}()

	anEvent := <-dockerEvents
	assert.Equal(t, anEvent.Type, apicontainer.ContainerHealthEvent, "unexpected docker events type received")
	assert.Equal(t, anEvent.Health.Status, apicontainerstatus.ContainerHealthy)
	assert.Equal(t, anEvent.Health.Output, "health output")

	// Verify the following events do not translate into our event stream

	//
	// Docker 1.8.3 sends the full command appended to exec_create and exec_start
	// events. Test that we ignore there as well..
	//
	ignore := []string{
		"pause",
		"exec_create",
		"exec_create: /bin/bash",
		"exec_start",
		"exec_start: /bin/bash",
		"top",
		"attach",
		"export",
		"pull",
		"push",
		"tag",
		"untag",
		"import",
		"delete",
		"oom",
		"kill",
	}
	for _, eventStatus := range ignore {
		events <- &docker.APIEvents{Type: "container", ID: "123", Status: eventStatus}
		select {
		case <-dockerEvents:
			t.Error("No event should be available for " + eventStatus)
		default:
		}
	}

	// Verify only the container type event will translate to our event stream
	// Events type: network, image, volume, daemon, plugins won't be handled
	ignoreEventType := map[string]string{
		"network": "connect",
		"image":   "pull",
		"volume":  "create",
		"plugin":  "install",
		"daemon":  "reload",
	}

	for eventType, eventStatus := range ignoreEventType {
		events <- &docker.APIEvents{Type: eventType, ID: "123", Status: eventStatus}
		select {
		case <-dockerEvents:
			t.Errorf("No event should be available for %v", eventType)
		default:
		}
	}
}

func TestDockerVersion(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDocker.EXPECT().VersionWithContext(gomock.Any()).Return(&docker.Env{"Version=1.6.0"}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	str, err := client.Version(ctx, dockerclient.VersionTimeout)
	if err != nil {
		t.Error(err)
	}
	if str != "1.6.0" {
		t.Error("Got unexpected version string: " + str)
	}
}

func TestDockerVersionCached(t *testing.T) {
	_, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	// Explicitly set daemon version so that mockDocker (the docker client)
	// is not invoked again
	client.setDaemonVersion("1.6.0")
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	str, err := client.Version(ctx, dockerclient.VersionTimeout)
	if err != nil {
		t.Error(err)
	}
	if str != "1.6.0" {
		t.Error("Got unexpected version string: " + str)
	}
}

func TestListContainers(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	containers := []docker.APIContainers{{ID: "id"}}
	mockDocker.EXPECT().ListContainers(gomock.Any()).Return(containers, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListContainers(ctx, true, dockerclient.ListContainersTimeout)
	if response.Error != nil {
		t.Error("Did not expect error")
	}

	containerIds := response.DockerIDs
	if len(containerIds) != 1 {
		t.Error("Unexpected number of containers in list: ", len(containerIds))
	}

	if containerIds[0] != "id" {
		t.Error("Unexpected container id in the list: ", containerIds[0])
	}
}

func TestListContainersTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().ListContainers(gomock.Any()).Do(func(x interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(nil, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListContainers(ctx, true, xContainerShortTimeout)
	if response.Error == nil {
		t.Error("Expected error for pull timeout")
	}
	if response.Error.(apierrors.NamedError).ErrorName() != "DockerTimeoutError" {
		t.Error("Wrong error type")
	}
	wait.Done()
}

func TestPingFailError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockeriface.NewMockClient(ctrl)
	mockDocker.EXPECT().Ping().Return(errors.New("test error"))
	factory := mock_clientfactory.NewMockFactory(ctrl)
	factory.EXPECT().GetDefaultClient().Return(mockDocker, nil)
	_, err := NewDockerGoClient(factory, defaultTestConfig())
	if err == nil {
		t.Fatal("Expected ping error to result in constructor fail")
	}
}

func TestUsesVersionedClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockeriface.NewMockClient(ctrl)
	mockDocker.EXPECT().Ping().Return(nil)
	factory := mock_clientfactory.NewMockFactory(ctrl)
	factory.EXPECT().GetDefaultClient().Return(mockDocker, nil)
	client, err := NewDockerGoClient(factory, defaultTestConfig())
	if err != nil {
		t.Fatal(err)
	}

	vclient := client.WithVersion(dockerclient.DockerVersion("1.20"))

	factory.EXPECT().GetClient(dockerclient.DockerVersion("1.20")).Times(2).Return(mockDocker, nil)
	mockDocker.EXPECT().StartContainerWithContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDocker.EXPECT().InspectContainerWithContext(gomock.Any(), gomock.Any()).Return(nil, errors.New("test error"))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	vclient.StartContainer(ctx, "foo", defaultTestConfig().ContainerStartTimeout)
}

func TestUnavailableVersionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDocker := mock_dockeriface.NewMockClient(ctrl)
	mockDocker.EXPECT().Ping().Return(nil)
	factory := mock_clientfactory.NewMockFactory(ctrl)
	factory.EXPECT().GetDefaultClient().Return(mockDocker, nil)
	client, err := NewDockerGoClient(factory, defaultTestConfig())
	if err != nil {
		t.Fatal(err)
	}

	vclient := client.WithVersion(dockerclient.DockerVersion("1.21"))

	factory.EXPECT().GetClient(dockerclient.DockerVersion("1.21")).Times(1).Return(nil, errors.New("Cannot get client"))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := vclient.StartContainer(ctx, "foo", defaultTestConfig().ContainerStartTimeout)

	if metadata.Error == nil {
		t.Fatal("Expected error, didn't get one")
	}
	if namederr, ok := metadata.Error.(apierrors.NamedError); ok {
		if namederr.ErrorName() != "CannotGetDockerclientError" {
			t.Fatal("Wrong error name, expected CannotGetDockerclientError but got " + namederr.ErrorName())
		}
	} else {
		t.Fatal("Error was not a named error")
	}
}

func TestStatsNormalExit(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	time1 := time.Now()
	time2 := time1.Add(1 * time.Second)
	mockDocker.EXPECT().Stats(gomock.Any()).Do(func(x interface{}) {
		opts := x.(docker.StatsOptions)
		defer close(opts.Stats)
		if opts.ID != "foo" {
			t.Fatalf("Expected ID foo, got %s", opts.ID)
		}
		if opts.Stream != true {
			t.Fatal("Expected stream to be true")
		}
		opts.Stats <- &docker.Stats{
			Read: time1,
		}
		opts.Stats <- &docker.Stats{
			Read: time2,
		}
	})
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	stats, err := client.Stats("foo", ctx)
	if err != nil {
		t.Fatal(err)
	}
	stat := <-stats
	checkStatRead(t, stat, time1)
	stat = <-stats
	checkStatRead(t, stat, time2)
	stat = <-stats
	if stat != nil {
		t.Fatal("Expected stat to be nil")
	}
}

func checkStatRead(t *testing.T, stat *docker.Stats, read time.Time) {
	if stat.Read != read {
		t.Fatalf("Expected %v, but was %v", read, stat.Read)
	}
}

func TestStatsClosed(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	time1 := time.Now()
	mockDocker.EXPECT().Stats(gomock.Any()).Do(func(x interface{}) {
		opts := x.(docker.StatsOptions)
		defer close(opts.Stats)
		if opts.ID != "foo" {
			t.Fatalf("Expected ID foo, got %s", opts.ID)
		}
		if opts.Stream != true {
			t.Fatal("Expected stream to be true")
		}
		for i := 0; true; i++ {
			select {
			case <-opts.Context.Done():
				t.Logf("Received cancel after %d iterations", i)
				return
			default:
				opts.Stats <- &docker.Stats{
					Read: time1.Add(time.Duration(i) * time.Second),
				}
			}
		}
	})
	ctx, cancel := context.WithCancel(context.TODO())
	stats, err := client.Stats("foo", ctx)
	if err != nil {
		t.Fatal(err)
	}
	stat := <-stats
	checkStatRead(t, stat, time1)
	stat = <-stats
	checkStatRead(t, stat, time1.Add(time.Second))
	cancel()
	// drain
	for {
		stat = <-stats
		if stat == nil {
			break
		}
	}
}

func TestStatsErrorReading(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDocker.EXPECT().Stats(gomock.Any()).Do(func(x interface{}) error {
		opts := x.(docker.StatsOptions)
		close(opts.Stats)
		return errors.New("test error")
	})
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	stats, err := client.Stats("foo", ctx)
	if err != nil {
		t.Fatal(err)
	}
	stat := <-stats
	if stat != nil {
		t.Fatal("Expected stat to be nil")
	}
}

func TestStatsClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	factory := mock_clientfactory.NewMockFactory(ctrl)
	factory.EXPECT().GetDefaultClient().Return(nil, errors.New("No client"))
	client := &dockerGoClient{
		clientFactory: factory,
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := client.Stats("foo", ctx)
	if err == nil {
		t.Fatal("Expected error with nil docker client")
	}
}

func TestRemoveImageTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().RemoveImage("image").Do(func(x interface{}) {
		wait.Wait()
	})
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveImage(ctx, "image", 2*time.Millisecond)
	if err == nil {
		t.Errorf("Expected error for remove image timeout")
	}
	wait.Done()
}

func TestRemoveImage(t *testing.T) {
	mockDocker, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDocker.EXPECT().RemoveImage("image").Return(nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveImage(ctx, "image", 2*time.Millisecond)
	if err != nil {
		t.Errorf("Did not expect error, err: %v", err)
	}
}

// TestContainerMetadataWorkaroundIssue27601 tests the workaround for
// issue https://github.com/moby/moby/issues/27601
func TestContainerMetadataWorkaroundIssue27601(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDocker.EXPECT().InspectContainerWithContext("id", gomock.Any()).Return(&docker.Container{
		Mounts: []docker.Mount{{
			Destination: "destination1",
			Source:      "source1",
		}, {
			Destination: "destination2",
			Source:      "source2",
		}},
	}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.containerMetadata(ctx, "id")
	assert.Equal(t, map[string]string{"destination1": "source1", "destination2": "source2"}, metadata.Volumes)
}

func TestLoadImageHappyPath(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDocker.EXPECT().LoadImage(gomock.Any()).Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.LoadImage(ctx, nil, time.Second)
	assert.NoError(t, err)
}

func TestLoadImageTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().LoadImage(gomock.Any()).Do(func(x interface{}) {
		wait.Wait()
	}).MaxTimes(1)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.LoadImage(ctx, nil, time.Millisecond)
	assert.Error(t, err)
	_, ok := err.(*DockerTimeoutError)
	assert.True(t, ok)
	wait.Done()
}

// TestECRAuthCache tests the client will use cached docker auth if pulling
// from same registry on ecr with default instance profile
func TestECRAuthCacheWithoutExecutionRole(t *testing.T) {
	mockDocker, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
	endpointOverride := "my.endpoint"
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "myimage:tag"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}

	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDocker.EXPECT().PullImage(gomock.Any(), gomock.Any()).Return(nil).Times(4)

	metadata := client.PullImage(image, authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(image+"2", authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(image+"3", authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(image+"4", authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheForDifferentRegistry tests the client will call ecr client to get docker
// auth for different registry
func TestECRAuthCacheForDifferentRegistry(t *testing.T) {
	mockDocker, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
	endpointOverride := "my.endpoint"
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}

	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDocker.EXPECT().PullImage(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	metadata := client.PullImage(image, authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the different registry should expect ECR client call
	authData.ECRAuthData.RegistryID = "another"
	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken("another").Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	metadata = client.PullImage(image, authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheWithExecutionRole tests the client will use the cached docker auth
// for ecr when pull from the same registry with same execution role
func TestECRAuthCacheWithSameExecutionRole(t *testing.T) {
	mockDocker, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}
	authData.ECRAuthData.SetPullCredentials(credentials.IAMRoleCredentials{
		RoleArn: "executionRole",
	})

	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDocker.EXPECT().PullImage(gomock.Any(), gomock.Any()).Return(nil).Times(3)

	metadata := client.PullImage(image, authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(image+"2", authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(image+"3", authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheWithDifferentExecutionRole tests client will call ecr client to get
// docker auth credentials for different execution role
func TestECRAuthCacheWithDifferentExecutionRole(t *testing.T) {
	mockDocker, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}
	authData.ECRAuthData.SetPullCredentials(credentials.IAMRoleCredentials{
		RoleArn: "executionRole",
	})

	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDocker.EXPECT().PullImage(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	metadata := client.PullImage(image, authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry but with different role
	authData.ECRAuthData.SetPullCredentials(credentials.IAMRoleCredentials{
		RoleArn: "executionRole2",
	})
	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	metadata = client.PullImage(image, authData)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestMetadataFromContainer(t *testing.T) {
	ports := map[docker.Port][]docker.PortBinding{
		docker.Port("80/tcp"): []docker.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: "80",
			},
		},
	}
	volumes := map[string]string{
		"/foo": "/bar",
	}
	labels := map[string]string{
		"name": "metadata",
	}

	created := time.Now()
	started := time.Now()
	finished := time.Now()

	dockerContainer := &docker.Container{
		NetworkSettings: &docker.NetworkSettings{
			Ports: ports,
		},
		ID:      "1234",
		Volumes: volumes,
		Config: &docker.Config{
			Labels: labels,
		},
		Created: created,
		State: docker.State{
			Running:    true,
			StartedAt:  started,
			FinishedAt: finished,
		},
	}

	metadata := MetadataFromContainer(dockerContainer)
	assert.Equal(t, "1234", metadata.DockerID)
	assert.Equal(t, volumes, metadata.Volumes)
	assert.Equal(t, labels, metadata.Labels)
	assert.Len(t, metadata.PortBindings, 1)
	assert.Equal(t, created, metadata.CreatedAt)
	assert.Equal(t, started, metadata.StartedAt)
	assert.Equal(t, finished, metadata.FinishedAt)
}

func TestMetadataFromContainerHealthCheckWithNoLogs(t *testing.T) {

	dockerContainer := &docker.Container{
		State: docker.State{
			Health: docker.Health{Status: "unhealthy"},
		}}

	metadata := MetadataFromContainer(dockerContainer)
	assert.Equal(t, apicontainerstatus.ContainerUnhealthy, metadata.Health.Status)
}

func TestCreateVolumeTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().CreateVolume(gomock.Any()).Do(func(x interface{}) {
		wait.Wait()
	}).MaxTimes(1)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.CreateVolume(ctx, "name", "driver", nil, nil, xContainerShortTimeout)
	assert.Error(t, volumeResponse.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestCreateVolumeError(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDocker.EXPECT().CreateVolume(gomock.Any()).Return(nil, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.CreateVolume(ctx, "name", "driver", nil, nil, CreateVolumeTimeout)
	assert.Equal(t, "CannotCreateVolumeError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
}

func TestCreateVolume(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"
	mountPoint := "some/mount/point"
	driver := "driver"
	driverOptions := map[string]string{
		"opt1": "val1",
		"opt2": "val2",
	}
	gomock.InOrder(
		mockDocker.EXPECT().CreateVolume(gomock.Any()).Do(func(opts docker.CreateVolumeOptions) {
			assert.Equal(t, opts.Name, volumeName)
			assert.Equal(t, opts.Driver, driver)
			assert.EqualValues(t, opts.DriverOpts, driverOptions)
		}).Return(&docker.Volume{Name: volumeName, Driver: driver, Mountpoint: mountPoint, Labels: nil}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.CreateVolume(ctx, volumeName, driver, driverOptions, nil, CreateVolumeTimeout)
	assert.NoError(t, volumeResponse.Error)
	assert.Equal(t, volumeResponse.DockerVolume.Name, volumeName)
	assert.Equal(t, volumeResponse.DockerVolume.Driver, driver)
	assert.Equal(t, volumeResponse.DockerVolume.Mountpoint, mountPoint)
	assert.Nil(t, volumeResponse.DockerVolume.Labels)
}

func TestInspectVolumeTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().InspectVolume(gomock.Any()).Do(func(x interface{}) {
		wait.Wait()
	}).MaxTimes(1)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, "name", xContainerShortTimeout)
	assert.Error(t, volumeResponse.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestInspectVolumeError(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDocker.EXPECT().InspectVolume(gomock.Any()).Return(nil, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, "name", InspectVolumeTimeout)
	assert.Equal(t, "CannotInspectVolumeError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
}

func TestInspectVolume(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"

	volumeOutput := docker.Volume{
		Name:       volumeName,
		Driver:     "driver",
		Mountpoint: "local/mount/point",
		Labels: map[string]string{
			"label1": "val1",
			"label2": "val2",
		},
	}

	mockDocker.EXPECT().InspectVolume(volumeName).Return(&volumeOutput, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, volumeName, InspectVolumeTimeout)
	assert.NoError(t, volumeResponse.Error)
	assert.Equal(t, volumeOutput.Driver, volumeResponse.DockerVolume.Driver)
	assert.Equal(t, volumeOutput.Mountpoint, volumeResponse.DockerVolume.Mountpoint)
	assert.Equal(t, volumeOutput.Labels, volumeResponse.DockerVolume.Labels)
}

func TestRemoveVolumeTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().RemoveVolume(gomock.Any()).Do(func(x interface{}) {
		wait.Wait()
	}).MaxTimes(1)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveVolume(ctx, "name", xContainerShortTimeout)
	assert.Error(t, err, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestRemoveVolumeError(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDocker.EXPECT().RemoveVolume(gomock.Any()).Return(errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveVolume(ctx, "name", RemoveVolumeTimeout)
	assert.Equal(t, "CannotRemoveVolumeError", err.(apierrors.NamedError).ErrorName())
}

func TestRemoveVolume(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"

	mockDocker.EXPECT().RemoveVolume(volumeName).Return(nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveVolume(ctx, volumeName, RemoveVolumeTimeout)
	assert.NoError(t, err)
}

func TestListPluginsTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().ListPlugins(gomock.Any()).Do(func(x interface{}) {
		wait.Wait()
	}).MaxTimes(1)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, xContainerShortTimeout)
	assert.Error(t, response.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", response.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestListPluginsError(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDocker.EXPECT().ListPlugins(gomock.Any()).Return(nil, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, ListPluginsTimeout)
	assert.Equal(t, "CannotListPluginsError", response.Error.(apierrors.NamedError).ErrorName())
}

func TestListPlugins(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	pluginID := "id"
	pluginName := "name"
	pluginTag := "tag"
	pluginDetail := docker.PluginDetail{
		ID:     pluginID,
		Name:   pluginName,
		Tag:    pluginTag,
		Active: true,
	}

	mockDocker.EXPECT().ListPlugins(gomock.Any()).Return([]docker.PluginDetail{pluginDetail}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, ListPluginsTimeout)
	assert.NoError(t, response.Error)
	assert.Equal(t, pluginDetail, response.Plugins[0])
}

func TestListPluginsWithFilter(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	plugins := []docker.PluginDetail{
		docker.PluginDetail{
			ID:     "id1",
			Name:   "name1",
			Tag:    "tag1",
			Active: false,
		},
		docker.PluginDetail{
			ID:     "id2",
			Name:   "name2",
			Tag:    "tag2",
			Active: true,
			Config: docker.PluginConfig{
				Description: "A sample volume plugin for Docker",
				Interface: docker.PluginInterface{
					Types:  []string{"docker.volumedriver/1.0"},
					Socket: "plugins.sock",
				},
			},
		},
		docker.PluginDetail{
			ID:     "id3",
			Name:   "name3",
			Tag:    "tag3",
			Active: true,
			Config: docker.PluginConfig{
				Description: "A sample network plugin for Docker",
				Interface: docker.PluginInterface{
					Types:  []string{"docker.networkdriver/1.0"},
					Socket: "plugins.sock",
				},
			},
		},
	}

	mockDocker.EXPECT().ListPlugins(gomock.Any()).Return(plugins, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	pluginNames, error := client.ListPluginsWithFilters(ctx, true, []string{VolumeDriverType}, ListPluginsTimeout)
	assert.NoError(t, error)
	assert.Equal(t, 1, len(pluginNames))
	assert.Equal(t, "name2", pluginNames[0])
}
