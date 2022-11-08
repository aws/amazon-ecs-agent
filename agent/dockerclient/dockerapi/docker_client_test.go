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

package dockerapi

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	mock_sdkclient "github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient/mocks"
	mock_sdkclientfactory "github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory/mocks"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	mock_ecr "github.com/aws/amazon-ecs-agent/agent/ecr/mocks"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	mock_ttime "github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xContainerShortTimeout is a short duration intended to be used by the
// docker client APIs that test if the underlying context gets canceled
// upon the expiration of the timeout duration.
// TODO Remove mock go-dockerclient calls and fields after migration is complete.
const xContainerShortTimeout = 1 * time.Millisecond

// xImgesShortTimeout is a short duration intended to be used by the
// docker client APIs that test if the underlying context gets canceled
// upon the expiration of the timeout duration.
const xImageShortTimeout = 1 * time.Millisecond

const (
	// retry settings for pulling images mock backoff
	xMaximumPullRetries        = 5
	xMinimumPullRetryDelay     = 25 * time.Millisecond
	xMaximumPullRetryDelay     = 100 * time.Microsecond
	xPullRetryDelayMultiplier  = 2
	xPullRetryJitterMultiplier = 0.2
	dockerEventBufferSize      = 100
)

func defaultTestConfig() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
	return cfg
}

func dockerClientSetup(t *testing.T) (
	*mock_sdkclient.MockClient,
	*dockerGoClient,
	*mock_ttime.MockTime,
	*gomock.Controller,
	*mock_ecr.MockECRFactory,
	func()) {
	return dockerClientSetupWithConfig(t, config.DefaultConfig())
}

func dockerClientSetupWithConfig(t *testing.T, conf config.Config) (
	*mock_sdkclient.MockClient,
	*dockerGoClient,
	*mock_ttime.MockTime,
	*gomock.Controller,
	*mock_ecr.MockECRFactory,
	func()) {
	ctrl := gomock.NewController(t)
	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	mockTime := mock_ttime.NewMockTime(ctrl)
	conf.EngineAuthData = config.NewSensitiveRawMessage([]byte{})
	client, _ := NewDockerGoClient(sdkFactory, &conf, ctx)
	goClient, _ := client.(*dockerGoClient)
	ecrClientFactory := mock_ecr.NewMockECRFactory(ctrl)
	goClient.ecrClientFactory = ecrClientFactory
	goClient._time = mockTime
	goClient.imagePullBackoff = retry.NewExponentialBackoff(xMinimumPullRetryDelay, xMaximumPullRetryDelay,
		xPullRetryJitterMultiplier, xPullRetryDelayMultiplier)
	return mockDockerSDK, goClient, mockTime, ctrl, ecrClientFactory, ctrl.Finish
}

func TestPullImageOutputTimeout(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	pullBeginTimeout := make(chan time.Time)
	testTime.EXPECT().After(dockerclient.DockerPullBeginTimeout).Return(pullBeginTimeout).MinTimes(1)

	// multiple invocations will happen due to retries, but all should timeout
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).DoAndReturn(
		func(x, y, z interface{}) (io.ReadCloser, error) {
			pullBeginTimeout <- time.Now()

			reader := &mockReadCloser{
				reader: strings.NewReader(`{"status":"pull in progress"}`),
			}
			return reader, nil
		}).Times(maximumPullRetries) // expected number of retries

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, defaultTestConfig().ImagePullTimeout)
	assert.Error(t, metadata.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
}

func TestImagePullGlobalTimeout(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	pullBeginTimeout := make(chan time.Time, 1)
	testTime.EXPECT().After(dockerclient.DockerPullBeginTimeout).Return(pullBeginTimeout)

	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).Do(func(x, y, z interface{}) {
		wait.Wait()
	}).Return(mockReadCloser{reader: strings.NewReader(`{"status":"pull in progress"}`)}, nil).MaxTimes(1)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, xContainerShortTimeout)
	assert.Error(t, metadata.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestPullImageInactivityTimeout(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	client.config.ImagePullInactivityTimeout = 1 * time.Millisecond

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).DoAndReturn(
		func(x, y, z interface{}) (io.ReadCloser, error) {

			reader := mockReadCloser{
				reader: strings.NewReader(`{"status":"pull in progress"}`),
				delay:  300 * time.Millisecond,
			}
			return reader, nil
		}).Times(maximumPullRetries) // expected number of retries

	client.inactivityTimeoutHandler = func(reader io.ReadCloser, timeout time.Duration, cancelRequest func(), canceled *uint32) (io.ReadCloser, chan<- struct{}) {
		assert.Equal(t, client.config.ImagePullInactivityTimeout, timeout)
		atomic.AddUint32(canceled, 1)
		return reader, make(chan struct{})
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, defaultTestConfig().ImagePullTimeout)
	assert.Error(t, metadata.Error, "Expected error for pull inactivity timeout")
	assert.Equal(t, "CannotPullContainerError", metadata.Error.(apierrors.NamedError).ErrorName(), "Wrong error type")
}

func TestImagePull(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()

	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestImagePullTag(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()
	client.config.ImagePullInactivityTimeout = 10 * time.Second

	testTime.EXPECT().After(gomock.Any()).AnyTimes()

	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:mytag", gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image:mytag", nil, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestImagePullDigest(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb", gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb", nil, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestPullImageECRSuccess(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

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

	imagePullOpts := types.ImagePullOptions{
		All:          false,
		RegistryAuth: "eyJ1c2VybmFtZSI6InVzZXJuYW1lIiwicGFzc3dvcmQiOiJwYXNzd29yZCIsInNlcnZlcmFkZHJlc3MiOiJodHRwczovL3JlZ2lzdHJ5LmVuZHBvaW50In0K",
	}

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
		}, nil)

	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), image, imagePullOpts).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestPullImageECRAuthFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, _ := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
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

	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.Error(t, metadata.Error, "expected pull to fail")
}

func TestPullImageError(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, _ := dockerClientSetup(t)

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).DoAndReturn(
		func(x, y, z interface{}) (io.ReadCloser, error) {

			reader := mockReadCloser{
				reader: strings.NewReader(`{"error":"toomanyrequests: Rate exceeded"}`),
				delay:  300 * time.Millisecond,
			}
			return reader, nil
		}).Times(maximumPullRetries) // expected number of retries

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, defaultTestConfig().ImagePullTimeout)
	assert.Error(t, metadata.Error, "toomanyrequests: Rate exceeded")
	assert.Equal(t, "CannotPullContainerError", metadata.Error.(apierrors.NamedError).ErrorName(), "Wrong error type")
}

type mockReadCloser struct {
	reader io.Reader
	delay  time.Duration
}

func (mr mockReadCloser) Read(data []byte) (n int, err error) {
	time.Sleep(mr.delay)
	return mr.reader.Read(data)
}
func (mr mockReadCloser) Close() error {
	return nil
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

func TestCreateContainerTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	hostConfig := &dockercontainer.HostConfig{Resources: dockercontainer.Resources{Memory: 100}}
	mockDockerSDK.EXPECT().ContainerCreate(gomock.Any(), &dockercontainer.Config{}, hostConfig,
		&network.NetworkingConfig{}, "containerName").Do(func(v, w, x, y, z interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(dockercontainer.ContainerCreateCreatedBody{}, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.CreateContainer(ctx, &dockercontainer.Config{}, hostConfig, "containerName", xContainerShortTimeout)
	assert.Error(t, metadata.Error, "expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestCreateContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	name := "containerName"
	hostConfig := &dockercontainer.HostConfig{Resources: dockercontainer.Resources{Memory: 100}}
	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerCreate(gomock.Any(), gomock.Any(), hostConfig, gomock.Any(), name).
			Do(func(v, w, x, y, z interface{}) {
				assert.True(t, reflect.DeepEqual(x, hostConfig),
					"Mismatch in create container HostConfig, %v != %v", y, hostConfig)
				assert.Equal(t, z, name,
					"Mismatch in create container options, %s != %s", z, name)
			}).Return(dockercontainer.ContainerCreateCreatedBody{ID: "id"}, nil),
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id").
			Return(types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID: "id",
				},
			}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.CreateContainer(ctx, nil, hostConfig, name, defaultTestConfig().ContainerCreateTimeout)
	assert.NoError(t, metadata.Error)
	assert.Equal(t, "id", metadata.DockerID)
	assert.Nil(t, metadata.ExitCode, "Expected a created container to not have an exit code")
}

func TestCreateContainerExecTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	execConfig := types.ExecConfig{
		Privileged:   false,
		AttachStdin:  false,
		AttachStderr: false,
		AttachStdout: false,
		Detach:       true,
		DetachKeys:   "",
		Env:          []string{},
		Cmd:          []string{"ls"},
	}

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerExecCreate(gomock.Any(), gomock.Any(), execConfig).Do(func(v, w, x interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := client.CreateContainerExec(ctx, "id", execConfig, xContainerShortTimeout)
	assert.NotNil(t, err, "Expected error for create container exec")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestCreateContainerExec(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	name := "containerName"
	execEnv := make([]string, 0)
	execCmd := make([]string, 0)
	execCmd = append(execCmd, "ls")
	execConfig := types.ExecConfig{
		Privileged:   false,
		AttachStdin:  false,
		AttachStderr: false,
		AttachStdout: false,
		Detach:       true,
		DetachKeys:   "",
		Env:          execEnv,
		Cmd:          execCmd,
	}

	execCreateResponse := types.IDResponse{ID: "id"}

	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerExecCreate(gomock.Any(), gomock.Any(), execConfig).
			Do(func(v, w, x interface{}) {
				assert.True(t, reflect.DeepEqual(x, execConfig),
					"Mismatch in create container ExecConfig, %v != %v", x, execConfig)
			}).Return(execCreateResponse, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	response, err := client.CreateContainerExec(ctx, name, execConfig, dockerclient.ContainerExecCreateTimeout)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, execCreateResponse, *response)
}

func TestStartContainerExecTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	execStartCheck := types.ExecStartCheck{
		Detach: true,
		Tty:    false,
	}

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerExecStart(gomock.Any(), "id", execStartCheck).Do(func(x, y, z interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1).Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.StartContainerExec(ctx, "id", types.ExecStartCheck{Detach: true, Tty: false}, xContainerShortTimeout)
	assert.NotNil(t, err, "Expected error for start container exec")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestStartContainerExec(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	execStartCheck := types.ExecStartCheck{
		Detach: true,
		Tty:    false,
	}

	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerExecStart(gomock.Any(), "id", execStartCheck).Return(nil),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.StartContainerExec(ctx, "id", types.ExecStartCheck{Detach: true, Tty: false}, dockerclient.ContainerExecStartTimeout)
	assert.NoError(t, err)
}

func TestInspectContainerExecTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerExecInspect(gomock.Any(), "id").Do(func(x, y interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := client.InspectContainerExec(ctx, "id", xContainerShortTimeout)
	assert.NotNil(t, err, "Expected error for inspect container exec")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestInspectContainerExec(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	inspectContainerResponse := types.ContainerExecInspect{
		ExecID:      "id",
		ContainerID: "cont",
		Running:     true,
		ExitCode:    0,
		Pid:         25537,
	}
	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerExecInspect(gomock.Any(), "id").Return(inspectContainerResponse, nil),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	resp, err := client.InspectContainerExec(ctx, "id", dockerclient.ContainerExecInspectTimeout)
	assert.NoError(t, err)
	assert.Equal(t, "id", resp.ExecID)
	assert.Equal(t, "cont", resp.ContainerID)
	assert.Equal(t, true, resp.Running)
	assert.Equal(t, 0, resp.ExitCode)
	assert.Equal(t, 25537, resp.Pid)
}

func TestStartContainerTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerStart(gomock.Any(), "id", types.ContainerStartOptions{}).Do(func(x, y, z interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id").Return(types.ContainerJSON{}, errors.New("test error")).AnyTimes()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StartContainer(ctx, "id", xContainerShortTimeout)
	assert.NotNil(t, metadata.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestStartContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerStart(gomock.Any(), "id", types.ContainerStartOptions{}).Return(nil),
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id").
			Return(types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID: "id",
				}}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StartContainer(ctx, "id", defaultTestConfig().ContainerStartTimeout)
	assert.NoError(t, metadata.Error)
	assert.Equal(t, "id", metadata.DockerID)
}

func TestStopContainerTimeout(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DockerStopTimeout = xContainerShortTimeout
	mockDockerSDK, client, _, _, _, done := dockerClientSetupWithConfig(t, cfg)
	defer done()
	reset := stopContainerTimeoutBuffer
	stopContainerTimeoutBuffer = xContainerShortTimeout
	defer func() {
		stopContainerTimeoutBuffer = reset
	}()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerStop(gomock.Any(), "id", &client.config.DockerStopTimeout).Do(func(x, y, z interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(errors.New("test error"))
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), gomock.Any()).AnyTimes()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StopContainer(ctx, "id", xContainerShortTimeout)
	assert.Error(t, metadata.Error, "Expected error for stop timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestStopContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerStop(gomock.Any(), "id", &client.config.DockerStopTimeout).Return(nil),
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id").
			Return(
				types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						ID: "id",
						State: &types.ContainerState{
							ExitCode: 10,
						},
					},
					Config: &dockercontainer.Config{},
				},
				nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StopContainer(ctx, "id", client.config.DockerStopTimeout)
	assert.NoError(t, metadata.Error)
	assert.Equal(t, "id", metadata.DockerID)
}

func TestRemoveContainerTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerRemove(gomock.Any(), "id",
		types.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   false,
			Force:         false,
		}).Do(func(x, y, z interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	err := client.RemoveContainer(ctx, "id", xContainerShortTimeout)
	assert.Error(t, err, "Expected error for remove timeout")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestRemoveContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().ContainerRemove(gomock.Any(), "id",
		types.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   false,
			Force:         false,
		}).Return(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveContainer(ctx, "id", dockerclient.RemoveContainerTimeout)
	assert.NoError(t, err)
}

func TestInspectContainerTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id").Do(func(ctx, x interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(types.ContainerJSON{}, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := client.InspectContainer(ctx, "id", xContainerShortTimeout)
	assert.Error(t, err, "Expected error for inspect timeout")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestInspectContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	containerOutput := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID: "id",
			State: &types.ContainerState{
				ExitCode: 10,
				Health: &types.Health{
					Status: "healthy",
					Log: []*types.HealthcheckResult{
						{
							ExitCode: 1,
							Output:   "health output",
						},
					},
				},
			}}}
	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id").Return(containerOutput, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	container, err := client.InspectContainer(ctx, "id", dockerclient.InspectContainerTimeout)
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(&containerOutput, container))
}

func TestContainerEvents(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	eventsChan := make(chan events.Message, dockerEventBufferSize)
	errChan := make(chan error)
	mockDockerSDK.EXPECT().Events(gomock.Any(), gomock.Any()).Return(eventsChan, errChan)

	dockerEvents, err := client.ContainerEvents(context.TODO())
	require.NoError(t, err, "Could not get container events")
	go func() {
		eventsChan <- events.Message{Type: "container", ID: "containerId", Status: "create"}
	}()

	event := <-dockerEvents
	assert.Equal(t, event.DockerID, "containerId", "Wrong docker id")
	assert.Equal(t, event.Status, apicontainerstatus.ContainerCreated, "Wrong status")

	container := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID: "cid2",
		},
		NetworkSettings: &types.NetworkSettings{
			NetworkSettingsBase: types.NetworkSettingsBase{
				Ports: nat.PortMap{
					nat.Port("80/tcp"): []nat.PortBinding{{HostPort: "9001"}},
				},
			},
		},
		Config: &dockercontainer.Config{},
		Mounts: []types.MountPoint{
			{Source: "/host/path",
				Destination: "/container/path"},
		},
	}

	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "cid2").Return(container, nil)
	go func() {
		eventsChan <- events.Message{Type: "container", ID: "cid2", Status: "start"}
	}()
	event = <-dockerEvents
	assert.Equal(t, event.DockerID, "cid2", "Wrong docker id")
	assert.Equal(t, event.Status, apicontainerstatus.ContainerRunning, "Wrong status")
	assert.Equal(t, event.PortBindings[0].ContainerPort, aws.Uint16(80), "Incorrect port bindings")
	assert.Equal(t, event.PortBindings[0].HostPort, uint16(9001), "Incorrect port bindings")
	assert.Equal(t, event.Volumes[0].Source, "/host/path", "Incorrect volume mapping")
	assert.Equal(t, event.Volumes[0].Destination, "/container/path", "Incorrect volume mapping")

	for i := 0; i < 2; i++ {
		stoppedContainer := types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID: "cid3" + strconv.Itoa(i),
				State: &types.ContainerState{
					FinishedAt: (time.Now()).Format(time.RFC3339),
					ExitCode:   20,
				},
			},
		}
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "cid3"+strconv.Itoa(i)).Return(stoppedContainer, nil)
	}
	go func() {
		eventsChan <- events.Message{Type: "container", ID: "cid30", Status: "stop"}
		eventsChan <- events.Message{Type: "container", ID: "cid31", Status: "die"}
	}()

	for i := 0; i < 2; i++ {
		anEvent := <-dockerEvents
		assert.True(t, anEvent.DockerID == "cid30" || anEvent.DockerID == "cid31", "Wrong container id: "+anEvent.DockerID)
		assert.Equal(t, anEvent.Status, apicontainerstatus.ContainerStopped, "Should be stopped")
		assert.Equal(t, aws.IntValue(anEvent.ExitCode), 20, "Incorrect exit code")
	}

	containerWithHealthInfo := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID: "container_health",
			State: &types.ContainerState{
				Health: &types.Health{
					Status: "healthy",
					Log: []*types.HealthcheckResult{
						{
							ExitCode: 1,
							Output:   "health output",
						},
					},
				},
			}},
	}
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "container_health").Return(containerWithHealthInfo, nil)
	go func() {
		eventsChan <- events.Message{
			Type:   "container",
			ID:     "container_health",
			Action: "health_status: unhealthy",
			Status: "health_status: unhealthy",
			Actor: events.Actor{
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
		eventsChan <- events.Message{Type: "container", ID: "123", Status: eventStatus}
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
		eventsChan <- events.Message{Type: eventType, ID: "123", Status: eventStatus}
		select {
		case <-dockerEvents:
			t.Errorf("No event should be available for %v", eventType)
		default:
		}
	}
}

func TestContainerEventsError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{
			name: "EOF error",
			err:  io.EOF,
		},
		{
			name: "Unexpected EOF error",
			err:  io.ErrUnexpectedEOF,
		},
		{
			name: "other error",
			err:  errors.New("test error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
			defer done()

			eventsChan := make(chan events.Message, dockerEventBufferSize)
			errChan := make(chan error)
			mockDockerSDK.EXPECT().Events(gomock.Any(), gomock.Any()).Return(eventsChan, errChan).MinTimes(1)

			dockerEvents, err := client.ContainerEvents(context.TODO())
			require.NoError(t, err, "Could not get container events")
			go func() {
				errChan <- tc.err
				eventsChan <- events.Message{Type: "container", ID: "containerId", Status: "create"}
			}()

			event := <-dockerEvents
			assert.Equal(t, event.DockerID, "containerId", "Wrong docker id")
			assert.Equal(t, event.Status, apicontainerstatus.ContainerCreated, "Wrong status")
		})
	}
}

func TestSetExitCodeFromEvent(t *testing.T) {
	var (
		exitCodeInt    = 42
		exitCodeStr    = "42"
		altExitCodeInt = 1
	)

	defaultEvent := &events.Message{
		Status: dockerContainerDieEvent,
		Actor: events.Actor{
			Attributes: map[string]string{
				dockerContainerEventExitCodeAttribute: exitCodeStr,
			},
		},
	}

	testCases := []struct {
		name             string
		event            *events.Message
		metadata         DockerContainerMetadata
		expectedExitCode *int
	}{
		{
			name:             "exit code set from event",
			event:            defaultEvent,
			metadata:         DockerContainerMetadata{},
			expectedExitCode: &exitCodeInt,
		},
		{
			name:  "exit code not set from event when metadata already has it",
			event: defaultEvent,
			metadata: DockerContainerMetadata{
				ExitCode: &altExitCodeInt,
			},
			expectedExitCode: &altExitCodeInt,
		},
		{
			name: "exit code not set from event when event does not has it",
			event: &events.Message{
				Status: dockerContainerDieEvent,
				Actor:  events.Actor{},
			},
			metadata:         DockerContainerMetadata{},
			expectedExitCode: nil,
		},
		{
			name: "exit code not set from event when event has invalid exit code",
			event: &events.Message{
				Status: dockerContainerDieEvent,
				Actor: events.Actor{
					Attributes: map[string]string{
						dockerContainerEventExitCodeAttribute: "invalid",
					},
				},
			},
			metadata:         DockerContainerMetadata{},
			expectedExitCode: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setExitCodeFromEvent(tc.event, &tc.metadata)
			assert.Equal(t, tc.expectedExitCode, tc.metadata.ExitCode)
		})
	}
}

func TestDockerVersion(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().ServerVersion(gomock.Any()).Return(types.Version{Version: "1.6.0"}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	str, err := client.Version(ctx, dockerclient.VersionTimeout)
	assert.NoError(t, err)
	assert.Equal(t, "1.6.0", str, "Got unexpected version string: "+str)
}

func TestSystemPing(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{APIVersion: "test_docker_api"}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	pingResponse := client.SystemPing(ctx, dockerclient.InfoTimeout)

	assert.NoError(t, pingResponse.Error)
	assert.Equal(t, "test_docker_api", pingResponse.Response.APIVersion)
}

func TestSystemPingError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, errors.New("test error"))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	pingResponse := client.SystemPing(ctx, dockerclient.InfoTimeout)

	assert.Error(t, pingResponse.Error)
	assert.Nil(t, pingResponse.Response)
}

func TestDockerInfo(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().Info(gomock.Any()).Return(types.Info{SecurityOptions: []string{"selinux"}}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	info, err := client.Info(ctx, dockerclient.InfoTimeout)

	assert.NoError(t, err)
	assert.Equal(t, []string{"selinux"}, info.SecurityOptions)
}

func TestDockerInfoError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	errorMsg := "Error getting  docker info"

	mockDockerSDK.EXPECT().Info(gomock.Any()).Return(types.Info{}, errors.New(errorMsg))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	info, err := client.Info(ctx, dockerclient.InfoTimeout)

	assert.Error(t, err, errorMsg)
	assert.Equal(t, types.Info{}, info)
}

func TestDockerInfoClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	errorMsg := "Error getting client"

	// Mock SDKFactory
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Return the Docker Go client for the first call
	sdkFactory.EXPECT().GetDefaultClient().Times(1).Return(mockDockerSDK, nil)
	client, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	assert.NoError(t, err)

	// Throw error when `Info` tries to get the client
	sdkFactory.EXPECT().GetDefaultClient().Return(nil, errors.New(errorMsg))
	info, err := client.Info(ctx, dockerclient.InfoTimeout)

	assert.Error(t, err, errorMsg)
	assert.Equal(t, types.Info{}, info)
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
	assert.NoError(t, err)
	assert.Equal(t, "1.6.0", str, "Got unexpected version string: "+str)
}

func TestListContainers(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	containers := []types.Container{{ID: "id"}}
	mockDockerSDK.EXPECT().ContainerList(gomock.Any(), types.ContainerListOptions{All: true}).Return(containers, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListContainers(ctx, true, dockerclient.ListContainersTimeout)
	assert.NoError(t, response.Error)

	containerIds := response.DockerIDs
	assert.Equal(t, 1, len(containerIds), "Unexpected number of containers in list: ", len(containerIds))
	assert.Equal(t, "id", containerIds[0], "Unexpected container id in the list: ", containerIds[0])
}

func TestListContainersTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerList(gomock.Any(), types.ContainerListOptions{All: true}).
		Do(func(x, y interface{}) {
			wait.Wait()
			// Don't return, verify timeout happens
		}).MaxTimes(1).Return(nil, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListContainers(ctx, true, xContainerShortTimeout)
	assert.Error(t, response.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", response.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestListImages(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	images := []types.ImageSummary{{ID: "id"}}
	mockDocker.EXPECT().ImageList(gomock.Any(), gomock.Any()).Return(images, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListImages(ctx, dockerclient.ListImagesTimeout)
	assert.NoError(t, response.Error, "Did not expect error")

	imageIDs := response.ImageIDs
	assert.EqualValues(t, len(imageIDs), 1, "Unexpected number of images in list")
	assert.EqualValues(t, imageIDs[0], "id", "Unexpected id in list of images")
}

func TestListImagesTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().ImageList(gomock.Any(), gomock.Any()).Do(func(x, y interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(nil, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	response := client.ListImages(ctx, xImageShortTimeout)
	assert.Error(t, response.Error, "Expected error for pull timeout")
	assert.Equal(t, response.Error.(apierrors.NamedError).ErrorName(), "DockerTimeoutError", "Wrong error type")

	wait.Done()
}

// Test for constructor fail when Docker SDK Client Ping() fails
func TestPingSdkFailError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, errors.New("test error"))
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	_, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	assert.Error(t, err, "Expected ping error to result in constructor fail")
}

func TestUsesVersionedClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	assert.NoError(t, err)

	vclient := client.WithVersion(dockerclient.DockerVersion("1.20"))

	sdkFactory.EXPECT().GetClient(dockerclient.DockerVersion("1.20")).Times(2).Return(mockDockerSDK, nil)
	mockDockerSDK.EXPECT().ContainerStart(gomock.Any(), gomock.Any(), types.ContainerStartOptions{}).Return(nil)
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), gomock.Any()).Return(types.ContainerJSON{}, errors.New("test error"))
	vclient.StartContainer(ctx, "foo", defaultTestConfig().ContainerStartTimeout)
}

func TestUnavailableVersionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any()).Return(types.Ping{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	assert.NoError(t, err)

	vclient := client.WithVersion(dockerclient.DockerVersion("1.21"))

	sdkFactory.EXPECT().GetClient(dockerclient.DockerVersion("1.21")).Times(1).Return(nil, errors.New("Cannot get client"))
	metadata := vclient.StartContainer(ctx, "foo", defaultTestConfig().ContainerStartTimeout)

	assert.NotNil(t, metadata.Error, "Expected error, didn't get one")
	if namederr, ok := metadata.Error.(apierrors.NamedError); ok {
		if namederr.ErrorName() != "CannotGetDockerclientError" {
			t.Fatal("Wrong error name, expected CannotGetDockerclientError but got " + namederr.ErrorName())
		}
	} else {
		t.Fatal("Error was not a named error")
	}
}

func waitForStatsChanClose(statsChan <-chan *types.StatsJSON) (closed bool) {
	i := 0
	for range statsChan {
		if i == 10 {
			return false
		}
		i++
		time.Sleep(time.Millisecond * 10)
	}
	return true
}

func TestStatsNormalExit(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), true).Return(types.ContainerStats{
		Body: mockStream{
			data:  []byte(`{"memory_stats":{"Usage":50},"cpu_stats":{"system_cpu_usage":100}}`),
			index: 0,
		},
	}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	stats, _ := client.Stats(ctx, "foo", dockerclient.StatsInactivityTimeout)
	newStat := <-stats
	waitForStats(t, newStat)

	assert.Equal(t, uint64(50), newStat.MemoryStats.Usage)
	assert.Equal(t, uint64(100), newStat.CPUStats.SystemUsage)

	// stop container stats
	cancel()
	// verify stats chan was closed to avoid goroutine leaks
	closed := waitForStatsChanClose(stats)
	assert.True(t, closed, "stats channel was not properly closed")
}

func TestStatsErrorReading(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), gomock.Any()).Return(types.ContainerStats{
		Body: mockStream{
			data:  []byte(`{"memory_stats":{"Usage":50},"cpu_stats":{"system_cpu_usage":100}}`),
			index: 0,
		},
	}, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	statsC, errC := client.Stats(ctx, "foo", dockerclient.StatsInactivityTimeout)

	assert.Error(t, <-errC)
	// verify stats chan was closed to avoid goroutine leaks
	closed := waitForStatsChanClose(statsC)
	assert.True(t, closed, "stats channel was not properly closed")
}

func TestStatsErrorDecoding(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), true).Return(types.ContainerStats{
		Body: mockStream{
			data:  []byte(`stuff`),
			index: 0,
		},
	}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	statsC, errC := client.Stats(ctx, "foo", dockerclient.StatsInactivityTimeout)
	assert.Error(t, <-errC)
	// verify stats chan was closed to avoid goroutine leaks
	closed := waitForStatsChanClose(statsC)
	assert.True(t, closed, "stats channel was not properly closed")
}

func TestStatsClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(nil, errors.New("No client"))
	client := &dockerGoClient{
		sdkClientFactory: sdkFactory,
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	statsC, errC := client.Stats(ctx, "foo", dockerclient.StatsInactivityTimeout)
	// should get an error from the channel
	err := <-errC
	// stats channel should be closed
	closed := waitForStatsChanClose(statsC)
	assert.True(t, closed, "stats channel was not properly closed")
	assert.Error(t, err)
}

type mockStream struct {
	data  []byte
	index int64
	delay time.Duration
}

func (ms mockStream) Read(data []byte) (n int, err error) {
	time.Sleep(ms.delay)
	if ms.index >= int64(len(ms.data)) {
		err = io.EOF
		return
	}
	n = copy(data, ms.data[ms.index:])
	ms.index += int64(n)
	return
}
func (ms mockStream) Close() error {
	return nil
}
func waitForStats(t *testing.T, stat *types.StatsJSON) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			t.Error("Timed out waiting for container stats")
		default:
			if stat == nil {
				time.Sleep(time.Second)
				continue
			}
			return
		}
	}
}

func TestStatsInactivityTimeout(t *testing.T) {
	shortInactivityTimeout := 1 * time.Millisecond
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), true).Return(types.ContainerStats{
		Body: mockStream{
			data:  []byte(`{"memory_stats":{"Usage":50},"cpu_stats":{"system_cpu_usage":100}}`),
			index: 0,
			delay: 300 * time.Millisecond,
		},
	}, nil)

	client.inactivityTimeoutHandler = func(reader io.ReadCloser, timeout time.Duration, cancelRequest func(), canceled *uint32) (io.ReadCloser, chan<- struct{}) {
		assert.Equal(t, shortInactivityTimeout, timeout)
		atomic.AddUint32(canceled, 1)
		return reader, make(chan struct{})
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, errC := client.Stats(ctx, "foo", shortInactivityTimeout)
	assert.Error(t, <-errC)
}

func TestPollStatsTimeout(t *testing.T) {
	shortTimeout := 1 * time.Millisecond
	mockDockerSDK, _, _, _, _, done := dockerClientSetup(t)
	defer done()
	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), false).Do(func(x, y, z interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(types.ContainerStats{Body: mockStream{}}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := getContainerStatsNotStreamed(mockDockerSDK, ctx, "", shortTimeout)
	assert.Error(t, err)
	wait.Done()
}

func TestPollStatsError(t *testing.T) {
	shortTimeout := 1 * time.Millisecond
	mockDockerSDK, _, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), false).MaxTimes(1).Return(types.ContainerStats{
		Body: nil},
		errors.New("Container stats error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := getContainerStatsNotStreamed(mockDockerSDK, ctx, "foo", shortTimeout)
	assert.Error(t, err)
}

func TestStatsInactivityTimeoutNoHit(t *testing.T) {
	longInactivityTimeout := 500 * time.Millisecond
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), true).Return(types.ContainerStats{
		Body: mockStream{
			data:  []byte(`{"memory_stats":{"Usage":50},"cpu_stats":{"system_cpu_usage":100}}`),
			index: 0,
			delay: 300 * time.Millisecond,
		},
	}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	stats, _ := client.Stats(ctx, "foo", longInactivityTimeout)
	newStat := <-stats

	waitForStats(t, newStat)
	assert.Equal(t, uint64(50), newStat.MemoryStats.Usage)
	assert.Equal(t, uint64(100), newStat.CPUStats.SystemUsage)
}

func TestRemoveImageTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ImageRemove(gomock.Any(), "image", types.ImageRemoveOptions{}).Do(func(x, y, z interface{}) {
		wait.Wait()
	})
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveImage(ctx, "image", 2*time.Millisecond)
	assert.Error(t, err, "Expected error for remove image timeout")
	wait.Done()
}

func TestRemoveImage(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDockerSDK.EXPECT().ImageRemove(gomock.Any(), "image", types.ImageRemoveOptions{}).Return([]types.ImageDeleteResponseItem{}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveImage(ctx, "image", dockerclient.RemoveImageTimeout)
	assert.NoError(t, err, "Did not expect error, err: %v", err)
}

func TestLoadImageHappyPath(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().ImageLoad(gomock.Any(), gomock.Any(), false).Return(types.ImageLoadResponse{
		Body: ioutil.NopCloser(strings.NewReader("dummy load message")),
	}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.LoadImage(ctx, nil, dockerclient.LoadImageTimeout)
	assert.NoError(t, err)
}

func TestLoadImageTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ImageLoad(gomock.Any(), gomock.Any(), false).Do(func(x, y, z interface{}) {
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
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)

	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
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
	image := imageEndpoint + "myimage:tag"
	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil).Times(4)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"2", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"3", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"4", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheForDifferentRegistry tests the client will call ecr client to get docker
// auth for different registry
func TestECRAuthCacheForDifferentRegistry(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
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

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil).Times(2)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
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
	metadata = client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheWithExecutionRole tests the client will use the cached docker auth
// for ecr when pull from the same registry with same execution role
func TestECRAuthCacheWithSameExecutionRole(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
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
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil).Times(3)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"2", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"3", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheWithDifferentExecutionRole tests client will call ecr client to get
// docker auth credentials for different execution role
func TestECRAuthCacheWithDifferentExecutionRole(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
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

	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecrapi.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil).Times(2)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
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
	metadata = client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestMetadataFromContainer(t *testing.T) {
	ports := nat.PortMap{
		"80/tcp": []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: "80",
			},
		},
	}
	// Representation of Volumes in ContainerJSON
	volumes := []types.MountPoint{
		{Destination: "/foo",
			Source: "/bar",
		},
	}
	labels := map[string]string{
		"name": "metadata",
	}

	created := time.Now().Format(time.RFC3339)
	started := time.Now().Format(time.RFC3339)
	finished := time.Now().Format(time.RFC3339)

	dockerContainer := types.ContainerJSON{
		NetworkSettings: &types.NetworkSettings{
			NetworkSettingsBase: types.NetworkSettingsBase{
				Ports: ports,
			},
			DefaultNetworkSettings: types.DefaultNetworkSettings{
				IPAddress: "17.0.0.3",
			},
		},
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:      "1234",
			Created: created,
			State: &types.ContainerState{
				Running:    true,
				StartedAt:  started,
				FinishedAt: finished,
			},
			HostConfig: &dockercontainer.HostConfig{
				NetworkMode: dockercontainer.NetworkMode("bridge"),
			},
		},
		Config: &dockercontainer.Config{
			Labels: labels,
		},
		Mounts: volumes,
	}

	metadata := MetadataFromContainer(&dockerContainer)
	assert.Equal(t, "1234", metadata.DockerID)
	assert.Equal(t, volumes, metadata.Volumes)
	assert.Equal(t, labels, metadata.Labels)
	assert.Len(t, metadata.PortBindings, 1)
	assert.Equal(t, "bridge", metadata.NetworkMode)
	assert.NotNil(t, metadata.NetworkSettings)
	assert.Equal(t, "17.0.0.3", metadata.NetworkSettings.IPAddress)

	// Need to convert both strings to same format to be able to compare. Parse and Format are not inverses.
	createdTimeSDK, _ := time.Parse(time.RFC3339, dockerContainer.Created)
	startedTimeSDK, _ := time.Parse(time.RFC3339, dockerContainer.State.StartedAt)
	finishedTimeSDK, _ := time.Parse(time.RFC3339, dockerContainer.State.FinishedAt)

	createdTime, _ := time.Parse(time.RFC3339, created)
	startedTime, _ := time.Parse(time.RFC3339, started)
	finishedTime, _ := time.Parse(time.RFC3339, finished)

	assert.True(t, createdTime.Equal(createdTimeSDK))
	assert.True(t, startedTime.Equal(startedTimeSDK))
	assert.True(t, finishedTime.Equal(finishedTimeSDK))
}

func TestMetadataFromContainerHealthCheckWithNoLogs(t *testing.T) {

	dockerContainer := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			State: &types.ContainerState{
				Health: &types.Health{Status: "unhealthy"},
			},
		},
	}

	metadata := MetadataFromContainer(dockerContainer)
	assert.Equal(t, apicontainerstatus.ContainerUnhealthy, metadata.Health.Status)
}

func TestCreateVolumeTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Do(func(ctx context.Context, x interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(types.Volume{}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.CreateVolume(ctx, "name", "driver", nil, nil, xContainerShortTimeout)
	assert.Error(t, volumeResponse.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestCreateVolumeError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Return(types.Volume{}, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.CreateVolume(ctx, "name", "driver", nil, nil, dockerclient.CreateVolumeTimeout)
	assert.Equal(t, "CannotCreateVolumeError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
}

func TestCreateVolume(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"
	mountPoint := "some/mount/point"
	driver := "driver"
	driverOptions := map[string]string{
		"opt1": "val1",
		"opt2": "val2",
	}
	gomock.InOrder(
		mockDockerSDK.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Do(func(ctx context.Context, opts volume.VolumeCreateBody) {
			assert.Equal(t, opts.Name, volumeName)
			assert.Equal(t, opts.Driver, driver)
			assert.EqualValues(t, opts.DriverOpts, driverOptions)
		}).Return(types.Volume{Name: volumeName, Driver: driver, Mountpoint: mountPoint, Labels: nil}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	// This function eventually makes an API call, is that not possible in testing
	volumeResponse := client.CreateVolume(ctx, volumeName, driver, driverOptions, nil, dockerclient.CreateVolumeTimeout)
	assert.NoError(t, volumeResponse.Error)
	assert.Equal(t, volumeResponse.DockerVolume.Name, volumeName)
	assert.Equal(t, volumeResponse.DockerVolume.Driver, driver)
	assert.Equal(t, volumeResponse.DockerVolume.Mountpoint, mountPoint)
	assert.Nil(t, volumeResponse.DockerVolume.Labels)
}

func TestInspectVolumeTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().VolumeInspect(gomock.Any(), gomock.Any()).Do(func(ctx context.Context, x interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(types.Volume{}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, "name", xContainerShortTimeout)
	assert.Error(t, volumeResponse.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestInspectVolumeError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().VolumeInspect(gomock.Any(), gomock.Any()).Return(types.Volume{}, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, "name", dockerclient.InspectVolumeTimeout)
	assert.Equal(t, "CannotInspectVolumeError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
}

func TestInspectVolume(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"

	volumeOutput := types.Volume{
		Name:       volumeName,
		Driver:     "driver",
		Mountpoint: "local/mount/point",
		Labels: map[string]string{
			"label1": "val1",
			"label2": "val2",
		},
	}

	mockDockerSDK.EXPECT().VolumeInspect(gomock.Any(), volumeName).Return(volumeOutput, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, volumeName, dockerclient.InspectVolumeTimeout)
	assert.NoError(t, volumeResponse.Error)
	assert.Equal(t, volumeOutput.Driver, volumeResponse.DockerVolume.Driver)
	assert.Equal(t, volumeOutput.Mountpoint, volumeResponse.DockerVolume.Mountpoint)
	assert.Equal(t, volumeOutput.Labels, volumeResponse.DockerVolume.Labels)
}

func TestRemoveVolumeTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().VolumeRemove(gomock.Any(), "name", false).Do(func(ctx context.Context,
		x interface{}, y bool) {
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
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().VolumeRemove(gomock.Any(), "name", false).Return(errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveVolume(ctx, "name", dockerclient.RemoveVolumeTimeout)
	assert.Equal(t, "CannotRemoveVolumeError", err.(apierrors.NamedError).ErrorName())
	assert.NotNil(t, err.Error(), "Nested error cannot be nil")
}

func TestRemoveVolume(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"

	mockDockerSDK.EXPECT().VolumeRemove(gomock.Any(), volumeName, false).Return(nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveVolume(ctx, volumeName, dockerclient.RemoveVolumeTimeout)
	assert.NoError(t, err)
}

func TestListPluginsTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().PluginList(gomock.Any(), filters.Args{}).Do(func(x, y interface{}) {
		wait.Wait()
	}).MaxTimes(1)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, xContainerShortTimeout, filters.Args{})
	assert.Error(t, response.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", response.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestListPluginsError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().PluginList(gomock.Any(), filters.Args{}).Return(nil, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, dockerclient.ListPluginsTimeout, filters.Args{})
	assert.Equal(t, "CannotListPluginsError", response.Error.(apierrors.NamedError).ErrorName())
}

func TestListPlugins(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	pluginID := "id"
	pluginName := "name"
	plugin := &types.Plugin{
		ID:      pluginID,
		Name:    pluginName,
		Enabled: true,
	}

	mockDockerSDK.EXPECT().PluginList(gomock.Any(), filters.Args{}).Return([]*types.Plugin{plugin}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, dockerclient.ListPluginsTimeout, filters.Args{})
	assert.NoError(t, response.Error)
	assert.Equal(t, plugin, response.Plugins[0])
}

func TestListPluginsWithFilter(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	plugins := []*types.Plugin{
		&types.Plugin{
			ID:      "id1",
			Name:    "name1",
			Enabled: false,
		},
		&types.Plugin{
			ID:      "id2",
			Name:    "name2",
			Enabled: true,
			Config: types.PluginConfig{
				Description: "A sample volume plugin for Docker",
				Interface: types.PluginConfigInterface{
					Types: []types.PluginInterfaceType{
						{Capability: "docker.volumedriver/1.0"},
					},
					Socket: "plugins.sock",
				},
			},
		},
		&types.Plugin{
			ID:      "id3",
			Name:    "name3",
			Enabled: true,
			Config: types.PluginConfig{
				Description: "A sample network plugin for Docker",
				Interface: types.PluginConfigInterface{
					Types: []types.PluginInterfaceType{
						{Capability: "docker.networkdriver/1.0"},
					},
					Socket: "plugins.sock",
				},
			},
		},
	}

	filterList := filters.NewArgs(filters.Arg("enabled", "true"))
	filterList.Add("capability", VolumeDriverType)
	mockDockerSDK.EXPECT().PluginList(gomock.Any(), filterList).Return([]*types.Plugin{plugins[1]}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	pluginNames, error := client.ListPluginsWithFilters(ctx, true, []string{VolumeDriverType}, dockerclient.ListPluginsTimeout)
	assert.NoError(t, error)
	assert.Equal(t, 1, len(pluginNames))
	assert.Equal(t, "name2", pluginNames[0])
}
