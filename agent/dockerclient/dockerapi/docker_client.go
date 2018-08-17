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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/async"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/clientfactory"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerauth"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockeriface"
	"github.com/aws/amazon-ecs-agent/agent/ecr"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"

	"github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

const (
	dockerDefaultTag = "latest"
	// imageNameFormat is the name of a image may look like: repo:tag
	imageNameFormat = "%s:%s"
	// the buffer size will ensure agent doesn't miss any event from docker
	dockerEventBufferSize = 100
	// healthCheckStarting is the initial status returned from docker container health check
	healthCheckStarting = "starting"
	// healthCheckHealthy is the healthy status returned from docker container health check
	healthCheckHealthy = "healthy"
	// healthCheckUnhealthy is unhealthy status returned from docker container health check
	healthCheckUnhealthy = "unhealthy"
	// maxHealthCheckOutputLength is the maximum length of healthcheck command output that agent will save
	maxHealthCheckOutputLength = 1024
	// VolumeDriverType is one of the plugin capabilities see https://docs.docker.com/engine/reference/commandline/plugin_ls/#filtering
	VolumeDriverType = "volumedriver"
)

// Timelimits for docker operations enforced above docker
// TODO: Make these limits configurable.
const (
	pullImageTimeout = 2 * time.Hour
	// CreateContainerTimeout is the timeout for the CreateContainer API.
	CreateContainerTimeout = 4 * time.Minute
	// StopContainerTimeout is the timeout for the StopContainer API.
	StopContainerTimeout = 30 * time.Second
	// RemoveContainerTimeout is the timeout for the RemoveContainer API.
	RemoveContainerTimeout = 5 * time.Minute
	// InspectContainerTimeout is the timeout for the InspectContainer API.
	InspectContainerTimeout = 30 * time.Second
	// RemoveImageTimeout is the timeout for the RemoveImage API.
	RemoveImageTimeout = 3 * time.Minute
	// ListPluginsTimeout is the timout for ListPlugins API.
	ListPluginsTimeout = 1 * time.Minute
	// CreateVolumeTimeout is the timout for CreateVolume API.
	CreateVolumeTimeout = 5 * time.Minute
	// InspectVolumeTimeout is the timout for InspectVolume API.
	InspectVolumeTimeout = 5 * time.Minute
	// RemoveVolumeTimeout is the timout for RemoveVolume API.
	RemoveVolumeTimeout = 5 * time.Minute
	// Parameters for caching the docker auth for ECR
	tokenCacheSize = 100
	// tokenCacheTTL is the default ttl of the docker auth for ECR
	tokenCacheTTL = 12 * time.Hour

	// dockerPullBeginTimeout is the timeout from when a 'pull' is called to when
	// we expect to see output on the pull progress stream. This is to work
	// around a docker bug which sometimes results in pulls not progressing.
	dockerPullBeginTimeout = 5 * time.Minute

	// dockerPullInactivityTimeout is the amount of time that we will
	// wait when the pulling does not progress
	dockerPullInactivityTimeout = 1 * time.Minute

	// pullStatusSuppressDelay controls the time where pull status progress bar
	// output will be suppressed in debug mode
	pullStatusSuppressDelay = 2 * time.Second

	// StatsInactivityTimeout controls the amount of time we hold open a
	// connection to the Docker daemon waiting for stats data
	StatsInactivityTimeout = 5 * time.Second

	// retry settings for pulling images
	maximumPullRetries        = 10
	minimumPullRetryDelay     = 250 * time.Millisecond
	maximumPullRetryDelay     = 1 * time.Second
	pullRetryDelayMultiplier  = 1.5
	pullRetryJitterMultiplier = 0.2
)

// DockerClient interface to make testing it easier
type DockerClient interface {
	// SupportedVersions returns a slice of the supported docker versions (or at least supposedly supported).
	SupportedVersions() []dockerclient.DockerVersion

	// KnownVersions returns a slice of the Docker API versions known to the Docker daemon.
	KnownVersions() []dockerclient.DockerVersion

	// WithVersion returns a new DockerClient for which all operations will use the given remote api version.
	// A default version will be used for a client not produced via this method.
	WithVersion(dockerclient.DockerVersion) DockerClient

	// ContainerEvents returns a channel of DockerContainerChangeEvents. Events are placed into the channel and should
	// be processed by the listener.
	ContainerEvents(ctx context.Context) (<-chan DockerContainerChangeEvent, error)

	// PullImage pulls an image. authData should contain authentication data provided by the ECS backend.
	PullImage(image string, authData *apicontainer.RegistryAuthenticationData) DockerContainerMetadata

	// CreateContainer creates a container with the provided docker.Config, docker.HostConfig, and name. A timeout value
	// and a context should be provided for the request.
	CreateContainer(context.Context, *docker.Config, *docker.HostConfig, string, time.Duration) DockerContainerMetadata

	// StartContainer starts the container identified by the name provided. A timeout value and a context should be
	// provided for the request.
	StartContainer(context.Context, string, time.Duration) DockerContainerMetadata

	// StopContainer stops the container identified by the name provided. A timeout value and a context should be provided
	// for the request.
	StopContainer(context.Context, string, time.Duration) DockerContainerMetadata

	// DescribeContainer returns status information about the specified container. A context should be provided
	// for the request
	DescribeContainer(context.Context, string) (apicontainerstatus.ContainerStatus, DockerContainerMetadata)

	// RemoveContainer removes a container (typically the rootfs, logs, and associated metadata) identified by the name.
	// A timeout value and a context should be provided for the request.
	RemoveContainer(context.Context, string, time.Duration) error

	// InspectContainer returns information about the specified container. A timeout value and a context should be
	// provided for the request.
	InspectContainer(context.Context, string, time.Duration) (*docker.Container, error)

	// ListContainers returns the set of containers known to the Docker daemon. A timeout value and a context
	// should be provided for the request.
	ListContainers(context.Context, bool, time.Duration) ListContainersResponse

	// CreateVolume creates a docker volume. A timeout value should be provided for the request
	CreateVolume(context.Context, string, string, map[string]string, map[string]string, time.Duration) VolumeResponse

	// InspectVolume returns a volume by its name. A timeout value should be provided for the request
	InspectVolume(context.Context, string, time.Duration) VolumeResponse

	// RemoveVolume removes a volume by its name. A timeout value should be provided for the request
	RemoveVolume(context.Context, string, time.Duration) error

	// ListPluginsWithFilters returns the set of docker plugins installed on the host, filtered by options provided.
	// A timeout value should be provided for the request.
	ListPluginsWithFilters(context.Context, bool, []string, time.Duration) ([]string, error)

	// ListPlugins returns the set of docker plugins installed on the host. A timeout value should be provided for
	// the request.
	ListPlugins(context.Context, time.Duration) ListPluginsResponse

	// Stats returns a channel of stat data for the specified container. A context should be provided so the request can
	// be canceled.
	Stats(string, context.Context) (<-chan *docker.Stats, error)

	// Version returns the version of the Docker daemon.
	Version(context.Context, time.Duration) (string, error)

	// APIVersion returns the api version of the client
	APIVersion() (dockerclient.DockerVersion, error)

	// InspectImage returns information about the specified image.
	InspectImage(string) (*docker.Image, error)

	// RemoveImage removes the metadata associated with an image and may remove the underlying layer data. A timeout
	// value and a context should be provided for the request.
	RemoveImage(context.Context, string, time.Duration) error
	// LoadImage loads an image from an input stream. A timeout value and a context should be provided for the request.
	LoadImage(context.Context, io.Reader, time.Duration) error
}

// DockerGoClient wraps the underlying go-dockerclient library.
// It exists primarily for the following three purposes:
// 1) Provide an abstraction over inputs and outputs,
//    a) Inputs: Trims them down to what we actually need (largely unchanged tbh)
//    b) Outputs: Unifies error handling and the common 'start->inspect'
//       pattern by having a consistent error output. This error output
//       contains error data with a given Name that aims to be presentable as a
//       'reason' in state changes. It also filters out the information about a
//       container that is of interest, such as network bindings, while
//       ignoring the rest.
// 2) Timeouts: It adds timeouts everywhere, mostly as a reaction to
//    pull-related issues in the Docker daemon.
// 3) Versioning: It abstracts over multiple client versions to allow juggling
//    appropriately there.
// Implements DockerClient
type dockerGoClient struct {
	clientFactory    clientfactory.Factory
	version          dockerclient.DockerVersion
	ecrClientFactory ecr.ECRFactory
	auth             dockerauth.DockerAuthProvider
	ecrTokenCache    async.Cache
	config           *config.Config

	_time     ttime.Time
	_timeOnce sync.Once

	daemonVersionUnsafe string
	lock                sync.Mutex
}

func (dg *dockerGoClient) WithVersion(version dockerclient.DockerVersion) DockerClient {
	return &dockerGoClient{
		clientFactory: dg.clientFactory,
		version:       version,
		auth:          dg.auth,
		config:        dg.config,
	}
}

// scratchCreateLock guards against multiple 'scratch' image creations at once
var scratchCreateLock sync.Mutex

// NewDockerGoClient creates a new DockerGoClient
func NewDockerGoClient(clientFactory clientfactory.Factory, cfg *config.Config) (DockerClient, error) {
	client, err := clientFactory.GetDefaultClient()

	if err != nil {
		seelog.Errorf("DockerGoClient: unable to connect to Docker daemon. Ensure Docker is running: %v", err)
		return nil, err
	}

	// Even if we have a dockerclient, the daemon might not be running. Ping it
	// to ensure it's up.
	err = client.Ping()
	if err != nil {
		seelog.Errorf("DockerGoClient: unable to ping Docker daemon. Ensure Docker is running: %v", err)
		return nil, err
	}

	var dockerAuthData json.RawMessage
	if cfg.EngineAuthData != nil {
		dockerAuthData = cfg.EngineAuthData.Contents()
	}
	return &dockerGoClient{
		clientFactory:    clientFactory,
		auth:             dockerauth.NewDockerAuthProvider(cfg.EngineAuthType, dockerAuthData),
		ecrClientFactory: ecr.NewECRFactory(cfg.AcceptInsecureCert),
		ecrTokenCache:    async.NewLRUCache(tokenCacheSize, tokenCacheTTL),
		config:           cfg,
	}, nil
}

func (dg *dockerGoClient) dockerClient() (dockeriface.Client, error) {
	if dg.version == "" {
		return dg.clientFactory.GetDefaultClient()
	}
	return dg.clientFactory.GetClient(dg.version)
}

func (dg *dockerGoClient) time() ttime.Time {
	dg._timeOnce.Do(func() {
		if dg._time == nil {
			dg._time = &ttime.DefaultTime{}
		}
	})
	return dg._time
}

func (dg *dockerGoClient) PullImage(image string, authData *apicontainer.RegistryAuthenticationData) DockerContainerMetadata {
	// TODO Switch to just using context.WithDeadline and get rid of this funky code
	timeout := dg.time().After(pullImageTimeout)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	response := make(chan DockerContainerMetadata, 1)
	go func() {
		imagePullBackoff := utils.NewSimpleBackoff(minimumPullRetryDelay,
			maximumPullRetryDelay, pullRetryJitterMultiplier, pullRetryDelayMultiplier)
		err := utils.RetryNWithBackoffCtx(ctx, imagePullBackoff, maximumPullRetries,
			func() error {
				err := dg.pullImage(image, authData)
				if err != nil {
					seelog.Warnf("DockerGoClient: failed to pull image %s: %s", image, err.Error())
				}
				return err
			})
		response <- DockerContainerMetadata{Error: wrapPullErrorAsNamedError(err)}
	}()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		cancel()
		return DockerContainerMetadata{Error: &DockerTimeoutError{pullImageTimeout, "pulled"}}
	}
}

func wrapPullErrorAsNamedError(err error) apierrors.NamedError {
	var retErr apierrors.NamedError
	if err != nil {
		engErr, ok := err.(apierrors.NamedError)
		if !ok {
			engErr = CannotPullContainerError{err}
		}
		retErr = engErr
	}
	return retErr
}

func (dg *dockerGoClient) pullImage(image string, authData *apicontainer.RegistryAuthenticationData) apierrors.NamedError {
	seelog.Debugf("DockerGoClient: pulling image: %s", image)
	client, err := dg.dockerClient()
	if err != nil {
		return CannotGetDockerClientError{version: dg.version, err: err}
	}

	authConfig, err := dg.getAuthdata(image, authData)
	if err != nil {
		return wrapPullErrorAsNamedError(err)
	}

	pullDebugOut, pullWriter := io.Pipe()
	defer pullWriter.Close()

	repository := getRepository(image)

	opts := docker.PullImageOptions{
		Repository:        repository,
		OutputStream:      pullWriter,
		InactivityTimeout: dockerPullInactivityTimeout,
	}
	timeout := dg.time().After(dockerPullBeginTimeout)
	// pullBegan is a channel indicating that we have seen at least one line of data on the 'OutputStream' above.
	// It is here to guard against a bug wherein Docker never writes anything to that channel and hangs in pulling forever.
	pullBegan := make(chan bool, 1)

	go dg.filterPullDebugOutput(pullDebugOut, pullBegan, image)

	pullFinished := make(chan error, 1)
	go func() {
		pullFinished <- client.PullImage(opts, authConfig)
		seelog.Debugf("DockerGoClient: pulling image complete: %s", image)
	}()

	select {
	case <-pullBegan:
		break
	case pullErr := <-pullFinished:
		if pullErr != nil {
			return CannotPullContainerError{pullErr}
		}
		return nil
	case <-timeout:
		return &DockerTimeoutError{dockerPullBeginTimeout, "pullBegin"}
	}
	seelog.Debugf("DockerGoClient: pull began for image: %s", image)
	defer seelog.Debugf("DockerGoClient: pull completed for image: %s", image)

	err = <-pullFinished
	if err != nil {
		return CannotPullContainerError{err}
	}
	return nil
}

func (dg *dockerGoClient) filterPullDebugOutput(pullDebugOut *io.PipeReader, pullBegan chan<- bool, image string) {
	// pullBeganOnce ensures we only indicate it began once (since our channel will only be read 0 or 1 times)
	pullBeganOnce := sync.Once{}

	reader := bufio.NewReader(pullDebugOut)
	var line string
	var pullErr error
	var statusDisplayed time.Time
	for {
		line, pullErr = reader.ReadString('\n')
		if pullErr != nil {
			break
		}
		pullBeganOnce.Do(func() {
			pullBegan <- true
		})

		now := time.Now()
		if !strings.Contains(line, "[=") || now.After(statusDisplayed.Add(pullStatusSuppressDelay)) {
			// skip most of the progress bar lines, but retain enough for debugging
			seelog.Debugf("DockerGoClient: pulling image %s, status %s", image, line)
			statusDisplayed = now
		}

		if strings.Contains(line, "already being pulled by another client. Waiting.") {
			// This can mean the daemon is 'hung' in pulling status for this image, but we can't be sure.
			seelog.Errorf("DockerGoClient: image 'pull' status marked as already being pulled for image %s, status %s",
				image, line)
		}
	}
	if pullErr != nil && pullErr != io.EOF {
		seelog.Warnf("DockerGoClient: error reading pull image status for image %s: %v", image, pullErr)
	}
}

func getRepository(image string) string {
	repository, tag := parseRepositoryTag(image)
	if tag == "" {
		repository = repository + ":" + dockerDefaultTag
	} else {
		repository = image
	}
	return repository
}

func (dg *dockerGoClient) InspectImage(image string) (*docker.Image, error) {
	client, err := dg.dockerClient()
	if err != nil {
		return nil, err
	}
	return client.InspectImage(image)
}

func (dg *dockerGoClient) getAuthdata(image string, authData *apicontainer.RegistryAuthenticationData) (docker.AuthConfiguration, error) {

	if authData == nil {
		return dg.auth.GetAuthconfig(image, nil)
	}

	switch authData.Type {
	case apicontainer.AuthTypeECR:
		provider := dockerauth.NewECRAuthProvider(dg.ecrClientFactory, dg.ecrTokenCache)
		authConfig, err := provider.GetAuthconfig(image, authData)
		if err != nil {
			return authConfig, CannotPullECRContainerError{err}
		}
		return authConfig, nil

	case apicontainer.AuthTypeASM:
		return authData.ASMAuthData.GetDockerAuthConfig(), nil

	default:
		return dg.auth.GetAuthconfig(image, nil)
	}
}

func (dg *dockerGoClient) CreateContainer(ctx context.Context,
	config *docker.Config,
	hostConfig *docker.HostConfig,
	name string,
	timeout time.Duration) DockerContainerMetadata {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.createContainer(ctx, config, hostConfig, name) }()

	// Wait until we get a response or for the 'done' context channel
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return DockerContainerMetadata{Error: &DockerTimeoutError{timeout, "created"}}
		}
		// Context was canceled even though there was no timeout. Send
		// back an error.
		return DockerContainerMetadata{Error: &CannotCreateContainerError{err}}
	}
}

func (dg *dockerGoClient) createContainer(ctx context.Context,
	config *docker.Config,
	hostConfig *docker.HostConfig,
	name string) DockerContainerMetadata {
	client, err := dg.dockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	containerOptions := docker.CreateContainerOptions{
		Config:     config,
		HostConfig: hostConfig,
		Name:       name,
		Context:    ctx,
	}
	dockerContainer, err := client.CreateContainer(containerOptions)
	if err != nil {
		return DockerContainerMetadata{Error: CannotCreateContainerError{err}}
	}

	return MetadataFromContainer(dockerContainer)
}

func (dg *dockerGoClient) StartContainer(ctx context.Context, id string, timeout time.Duration) DockerContainerMetadata {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.startContainer(ctx, id) }()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return DockerContainerMetadata{Error: &DockerTimeoutError{timeout, "started"}}
		}
		return DockerContainerMetadata{Error: CannotStartContainerError{err}}
	}
}

func (dg *dockerGoClient) startContainer(ctx context.Context, id string) DockerContainerMetadata {
	client, err := dg.dockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	err = client.StartContainerWithContext(id, nil, ctx)
	metadata := dg.containerMetadata(ctx, id)
	if err != nil {
		metadata.Error = CannotStartContainerError{err}
	}

	return metadata
}

// DockerStateToState converts the container status from docker to status recognized by the agent
// Ref: https://github.com/fsouza/go-dockerclient/blob/fd53184a1439b6d7b82ca54c1cd9adac9a5278f2/container.go#L133
func DockerStateToState(state docker.State) apicontainerstatus.ContainerStatus {
	if state.Running {
		return apicontainerstatus.ContainerRunning
	}

	if state.Dead {
		return apicontainerstatus.ContainerStopped
	}

	if state.StartedAt.IsZero() && state.Error == "" {
		return apicontainerstatus.ContainerCreated
	}

	return apicontainerstatus.ContainerStopped
}

func (dg *dockerGoClient) DescribeContainer(ctx context.Context, dockerID string) (apicontainerstatus.ContainerStatus, DockerContainerMetadata) {
	dockerContainer, err := dg.InspectContainer(ctx, dockerID, InspectContainerTimeout)
	if err != nil {
		return apicontainerstatus.ContainerStatusNone, DockerContainerMetadata{Error: CannotDescribeContainerError{err}}
	}
	return DockerStateToState(dockerContainer.State), MetadataFromContainer(dockerContainer)
}

func (dg *dockerGoClient) InspectContainer(ctx context.Context, dockerID string, timeout time.Duration) (*docker.Container, error) {
	type inspectResponse struct {
		container *docker.Container
		err       error
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan inspectResponse, 1)
	go func() {
		container, err := dg.inspectContainer(ctx, dockerID)
		response <- inspectResponse{container, err}
	}()

	// Wait until we get a response or for the 'done' context channel
	select {
	case resp := <-response:
		return resp.container, resp.err
	case <-ctx.Done():
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return nil, &DockerTimeoutError{timeout, "inspecting"}
		}

		return nil, &CannotInspectContainerError{err}
	}
}

func (dg *dockerGoClient) inspectContainer(ctx context.Context, dockerID string) (*docker.Container, error) {
	client, err := dg.dockerClient()
	if err != nil {
		return nil, err
	}
	return client.InspectContainerWithContext(dockerID, ctx)
}

func (dg *dockerGoClient) StopContainer(ctx context.Context, dockerID string, timeout time.Duration) DockerContainerMetadata {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.stopContainer(ctx, dockerID) }()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return DockerContainerMetadata{Error: &DockerTimeoutError{timeout, "stopped"}}
		}
		return DockerContainerMetadata{Error: CannotStopContainerError{err}}
	}
}

func (dg *dockerGoClient) stopContainer(ctx context.Context, dockerID string) DockerContainerMetadata {
	client, err := dg.dockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	err = client.StopContainerWithContext(dockerID, uint(dg.config.DockerStopTimeout/time.Second), ctx)
	metadata := dg.containerMetadata(ctx, dockerID)
	if err != nil {
		seelog.Infof("DockerGoClient: error stopping container %s: %v", dockerID, err)
		if metadata.Error == nil {
			metadata.Error = CannotStopContainerError{err}
		}
	}
	return metadata
}

func (dg *dockerGoClient) RemoveContainer(ctx context.Context, dockerID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan error, 1)
	go func() { response <- dg.removeContainer(ctx, dockerID) }()
	// Wait until we get a response or for the 'done' context channel
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		err := ctx.Err()
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		if err == context.DeadlineExceeded {
			return &DockerTimeoutError{dockerclient.RemoveContainerTimeout, "removing"}
		}
		return &CannotRemoveContainerError{err}
	}
}

func (dg *dockerGoClient) removeContainer(ctx context.Context, dockerID string) error {
	client, err := dg.dockerClient()
	if err != nil {
		return err
	}
	return client.RemoveContainer(docker.RemoveContainerOptions{
		ID:            dockerID,
		RemoveVolumes: true,
		Force:         false,
		Context:       ctx,
	})
}

func (dg *dockerGoClient) containerMetadata(ctx context.Context, id string) DockerContainerMetadata {
	ctx, cancel := context.WithTimeout(ctx, dockerclient.InspectContainerTimeout)
	defer cancel()
	dockerContainer, err := dg.InspectContainer(ctx, id, dockerclient.InspectContainerTimeout)
	if err != nil {
		return DockerContainerMetadata{DockerID: id, Error: CannotInspectContainerError{err}}
	}
	return MetadataFromContainer(dockerContainer)
}

// MetadataFromContainer translates dockerContainer into DockerContainerMetadata
func MetadataFromContainer(dockerContainer *docker.Container) DockerContainerMetadata {
	var bindings []apicontainer.PortBinding
	var err apierrors.NamedError
	if dockerContainer.NetworkSettings != nil {
		// Convert port bindings into the format our container expects
		bindings, err = apicontainer.PortBindingFromDockerPortBinding(dockerContainer.NetworkSettings.Ports)
		if err != nil {
			seelog.Criticalf("DockerGoClient: Docker had network bindings we couldn't understand: %v", err)
			return DockerContainerMetadata{Error: apierrors.NamedError(err)}
		}
	}
	metadata := DockerContainerMetadata{
		DockerID:     dockerContainer.ID,
		PortBindings: bindings,
		Volumes:      dockerContainer.Mounts,
		CreatedAt:    dockerContainer.Created,
		StartedAt:    dockerContainer.State.StartedAt,
		FinishedAt:   dockerContainer.State.FinishedAt,
	}
	if dockerContainer.Config != nil {
		metadata.Labels = dockerContainer.Config.Labels
	}

	if !dockerContainer.State.Running && !dockerContainer.State.FinishedAt.IsZero() {
		// Only record an exitcode if it has exited
		metadata.ExitCode = &dockerContainer.State.ExitCode
	}
	if dockerContainer.State.Error != "" {
		metadata.Error = NewDockerStateError(dockerContainer.State.Error)
	}
	if dockerContainer.State.OOMKilled {
		metadata.Error = OutOfMemoryError{}
	}
	if dockerContainer.State.Health.Status == "" || dockerContainer.State.Health.Status == healthCheckStarting {
		return metadata
	}

	// Record the health check information if exists
	metadata.Health = getMetadataHealthCheck(dockerContainer)
	return metadata
}

func getMetadataHealthCheck(dockerContainer *docker.Container) apicontainer.HealthStatus {
	health := apicontainer.HealthStatus{}
	logLength := len(dockerContainer.State.Health.Log)
	if logLength != 0 {
		// Only save the last log from the health check
		output := dockerContainer.State.Health.Log[logLength-1].Output
		size := len(output)
		if size > maxHealthCheckOutputLength {
			size = maxHealthCheckOutputLength
		}
		health.Output = output[:size]
	}

	switch dockerContainer.State.Health.Status {
	case healthCheckHealthy:
		health.Status = apicontainerstatus.ContainerHealthy
	case healthCheckUnhealthy:
		health.Status = apicontainerstatus.ContainerUnhealthy
		if logLength == 0 {
			seelog.Warn("DockerGoClient: no container healthcheck data returned by Docker")
			break
		}
		health.ExitCode = dockerContainer.State.Health.Log[logLength-1].ExitCode
	default:
		seelog.Debugf("DockerGoClient: unknown healthcheck status event from docker: %s", dockerContainer.State.Health.Status)
	}
	return health
}

// Listen to the docker event stream for container changes and pass them up
func (dg *dockerGoClient) ContainerEvents(ctx context.Context) (<-chan DockerContainerChangeEvent, error) {
	client, err := dg.dockerClient()
	if err != nil {
		return nil, err
	}
	dockerEvents := make(chan *docker.APIEvents, dockerEventBufferSize)
	events := make(chan *docker.APIEvents)
	buffer := NewInfiniteBuffer()

	err = client.AddEventListener(dockerEvents)
	if err != nil {
		seelog.Errorf("DockerGoClient: unable to add a docker event listener: %v", err)
		return nil, err
	}
	go func() {
		<-ctx.Done()
		client.RemoveEventListener(dockerEvents)
	}()

	// Cache the event from go docker client
	go buffer.StartListening(dockerEvents)
	// Read the buffered events and send to task engine
	go buffer.Consume(events)

	changedContainers := make(chan DockerContainerChangeEvent)
	go dg.handleContainerEvents(ctx, events, changedContainers)
	return changedContainers, nil
}

func (dg *dockerGoClient) handleContainerEvents(ctx context.Context,
	events <-chan *docker.APIEvents,
	changedContainers chan<- DockerContainerChangeEvent) {
	for event := range events {
		containerID := event.ID
		seelog.Debugf("DockerGoClient: got event from docker daemon: %v", event)

		var status apicontainerstatus.ContainerStatus
		eventType := apicontainer.ContainerStatusEvent
		switch event.Status {
		case "create":
			status = apicontainerstatus.ContainerCreated
			changedContainers <- DockerContainerChangeEvent{
				Status: status,
				Type:   eventType,
				DockerContainerMetadata: DockerContainerMetadata{
					DockerID: containerID,
				},
			}
			continue
		case "start":
			status = apicontainerstatus.ContainerRunning
		case "stop":
			fallthrough
		case "die":
			status = apicontainerstatus.ContainerStopped
		case "oom":
			containerInfo := event.ID
			// events only contain the container's name in newer Docker API
			// versions (starting with 1.22)
			if containerName, ok := event.Actor.Attributes["name"]; ok {
				containerInfo += fmt.Sprintf(" (name: %q)", containerName)
			}

			seelog.Infof("DockerGoClient: process within container %s died due to OOM", containerInfo)
			// "oom" can either means any process got OOM'd, but doesn't always
			// mean the container dies (non-init processes). If the container also
			// dies, you see a "die" status as well; we'll update suitably there
			continue
		case "health_status: healthy":
			fallthrough
		case "health_status: unhealthy":
			eventType = apicontainer.ContainerHealthEvent
		default:
			// Because docker emits new events even when you use an old event api
			// version, it's not that big a deal
			seelog.Debugf("DockerGoClient: unknown status event from docker: %v", event)
		}

		metadata := dg.containerMetadata(ctx, containerID)

		changedContainers <- DockerContainerChangeEvent{
			Status: status,
			Type:   eventType,
			DockerContainerMetadata: metadata,
		}
	}
}

// ListContainers returns a slice of container IDs.
func (dg *dockerGoClient) ListContainers(ctx context.Context, all bool, timeout time.Duration) ListContainersResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan ListContainersResponse, 1)
	go func() { response <- dg.listContainers(ctx, all) }()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return ListContainersResponse{Error: &DockerTimeoutError{timeout, "listing"}}
		}
		return ListContainersResponse{Error: &CannotListContainersError{err}}
	}
}

func (dg *dockerGoClient) listContainers(ctx context.Context, all bool) ListContainersResponse {
	client, err := dg.dockerClient()
	if err != nil {
		return ListContainersResponse{Error: err}
	}

	containers, err := client.ListContainers(docker.ListContainersOptions{
		All:     all,
		Context: ctx,
	})
	if err != nil {
		return ListContainersResponse{Error: err}
	}

	// We get an empty slice if there are no containers to be listed.
	// Extract container IDs from this list.
	containerIDs := make([]string, len(containers))
	for i, container := range containers {
		containerIDs[i] = container.ID
	}

	return ListContainersResponse{DockerIDs: containerIDs, Error: nil}
}

func (dg *dockerGoClient) SupportedVersions() []dockerclient.DockerVersion {
	return dg.clientFactory.FindSupportedAPIVersions()
}

func (dg *dockerGoClient) KnownVersions() []dockerclient.DockerVersion {
	return dg.clientFactory.FindKnownAPIVersions()
}

func (dg *dockerGoClient) Version(ctx context.Context, timeout time.Duration) (string, error) {
	version := dg.getDaemonVersion()
	if version != "" {
		return version, nil
	}

	derivedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := dg.dockerClient()
	if err != nil {
		return "", err
	}
	info, err := client.VersionWithContext(derivedCtx)
	if err != nil {
		return "", err
	}

	version = info.Get("Version")
	dg.setDaemonVersion(version)
	return version, nil
}

func (dg *dockerGoClient) getDaemonVersion() string {
	dg.lock.Lock()
	defer dg.lock.Unlock()

	return dg.daemonVersionUnsafe
}

func (dg *dockerGoClient) setDaemonVersion(version string) {
	dg.lock.Lock()
	defer dg.lock.Unlock()

	dg.daemonVersionUnsafe = version
}

func (dg *dockerGoClient) CreateVolume(ctx context.Context, name string,
	driver string,
	driverOptions map[string]string,
	labels map[string]string,
	timeout time.Duration) VolumeResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan VolumeResponse, 1)
	go func() { response <- dg.createVolume(ctx, name, driver, driverOptions, labels) }()

	// Wait until we get a response or for the 'done' context channel
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return VolumeResponse{DockerVolume: nil, Error: &DockerTimeoutError{timeout, "creating volume"}}
		}
		// Context was canceled even though there was no timeout. Send
		// back an error.
		return VolumeResponse{DockerVolume: nil, Error: &CannotCreateVolumeError{err}}
	}
}

func (dg *dockerGoClient) createVolume(ctx context.Context,
	name string,
	driver string,
	driverOptions map[string]string,
	labels map[string]string) VolumeResponse {
	client, err := dg.dockerClient()
	if err != nil {
		return VolumeResponse{DockerVolume: nil, Error: &CannotGetDockerClientError{version: dg.version, err: err}}
	}

	volumeOptions := docker.CreateVolumeOptions{
		Name:       name,
		Driver:     driver,
		DriverOpts: driverOptions,
		Context:    ctx,
		Labels:     labels,
	}
	dockerVolume, err := client.CreateVolume(volumeOptions)
	if err != nil {
		return VolumeResponse{DockerVolume: nil, Error: &CannotCreateVolumeError{err}}
	}

	return VolumeResponse{DockerVolume: dockerVolume, Error: nil}
}

func (dg *dockerGoClient) InspectVolume(ctx context.Context, name string, timeout time.Duration) VolumeResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan VolumeResponse, 1)
	go func() { response <- dg.inspectVolume(ctx, name) }()

	// Wait until we get a response or for the 'done' context channel
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return VolumeResponse{DockerVolume: nil, Error: &DockerTimeoutError{timeout, "inspecting volume"}}
		}
		// Context was canceled even though there was no timeout. Send
		// back an error.
		return VolumeResponse{DockerVolume: nil, Error: &CannotInspectVolumeError{err}}
	}
}

func (dg *dockerGoClient) inspectVolume(ctx context.Context, name string) VolumeResponse {
	client, err := dg.dockerClient()
	if err != nil {
		return VolumeResponse{
			DockerVolume: nil,
			Error:        &CannotGetDockerClientError{version: dg.version, err: err}}
	}

	dockerVolume, err := client.InspectVolume(name)
	if err != nil {
		return VolumeResponse{DockerVolume: nil, Error: &CannotInspectVolumeError{err}}
	}

	return VolumeResponse{DockerVolume: dockerVolume, Error: nil}
}

func (dg *dockerGoClient) RemoveVolume(ctx context.Context, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan error, 1)
	go func() { response <- dg.removeVolume(ctx, name) }()

	// Wait until we get a response or for the 'done' context channel
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return &DockerTimeoutError{timeout, "removing volume"}
		}
		// Context was canceled even though there was no timeout. Send
		// back an error.
		return &CannotRemoveVolumeError{err}
	}
}

func (dg *dockerGoClient) removeVolume(ctx context.Context, name string) error {
	client, err := dg.dockerClient()
	if err != nil {
		return &CannotGetDockerClientError{version: dg.version, err: err}
	}

	ok := client.RemoveVolume(name)
	if ok != nil {
		return &CannotRemoveVolumeError{err}
	}

	return nil
}

// ListPluginsWithFilters currently is a convenience method as go-dockerclient doesn't implement fitered list. When we or someone else submits
// PR for the fix we will refactor this to pass in the fiters. See https://docs.docker.com/engine/reference/commandline/plugin_ls/#filtering.
func (dg *dockerGoClient) ListPluginsWithFilters(ctx context.Context, enabled bool, capabilities []string, timeout time.Duration) ([]string, error) {

	var filteredPluginNames []string
	response := dg.ListPlugins(ctx, timeout)

	if response.Error != nil {
		return nil, response.Error
	}

	for _, pluginDetail := range response.Plugins {
		if pluginDetail.Active != enabled {
			continue
		}

		// One plugin might have multiple capabilities, see https://docs.docker.com/engine/reference/commandline/plugin_ls/#filtering
		for _, pluginType := range pluginDetail.Config.Interface.Types {
			for _, capability := range capabilities {
				// capability looks like volumedriver, pluginType looks like docker.volumedriver/1.0 (prefix.capability/version)
				if strings.Contains(pluginType, capability) {
					filteredPluginNames = append(filteredPluginNames, pluginDetail.Name)
					break
				}
			}
		}
	}
	return filteredPluginNames, nil
}

func (dg *dockerGoClient) ListPlugins(ctx context.Context, timeout time.Duration) ListPluginsResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan ListPluginsResponse, 1)
	go func() { response <- dg.listPlugins(ctx) }()

	// Wait until we get a response or for the 'done' context channel
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return ListPluginsResponse{Plugins: nil, Error: &DockerTimeoutError{timeout, "listing plugins"}}
		}
		// Context was canceled even though there was no timeout. Send
		// back an error.
		return ListPluginsResponse{Plugins: nil, Error: &CannotListPluginsError{err}}
	}
}

func (dg *dockerGoClient) listPlugins(ctx context.Context) ListPluginsResponse {
	client, err := dg.dockerClient()
	if err != nil {
		return ListPluginsResponse{Plugins: nil, Error: &CannotGetDockerClientError{version: dg.version, err: err}}
	}

	plugins, err := client.ListPlugins(ctx)
	if err != nil {
		return ListPluginsResponse{Plugins: nil, Error: &CannotListPluginsError{err}}
	}

	return ListPluginsResponse{Plugins: plugins, Error: nil}
}

// APIVersion returns the client api version
func (dg *dockerGoClient) APIVersion() (dockerclient.DockerVersion, error) {
	client, err := dg.dockerClient()
	if err != nil {
		return "", err
	}
	return dg.clientFactory.FindClientAPIVersion(client), nil
}

// Stats returns a channel of *docker.Stats entries for the container.
func (dg *dockerGoClient) Stats(id string, ctx context.Context) (<-chan *docker.Stats, error) {
	client, err := dg.dockerClient()
	if err != nil {
		return nil, err
	}

	stats := make(chan *docker.Stats)
	options := docker.StatsOptions{
		ID:                id,
		Stats:             stats,
		Stream:            true,
		Context:           ctx,
		InactivityTimeout: StatsInactivityTimeout,
	}

	go func() {
		statsErr := client.Stats(options)
		if statsErr != nil {
			seelog.Infof("DockerGoClient: Unable to retrieve stats for container %s: %v",
				id, statsErr)
		}
	}()

	return stats, nil
}

// RemoveImage invokes github.com/fsouza/go-dockerclient.Client's
// RemoveImage API with a timeout
func (dg *dockerGoClient) RemoveImage(ctx context.Context, imageName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response := make(chan error, 1)
	go func() { response <- dg.removeImage(imageName) }()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		return &DockerTimeoutError{timeout, "removing image"}
	}
}

func (dg *dockerGoClient) removeImage(imageName string) error {
	client, err := dg.dockerClient()
	if err != nil {
		return err
	}
	return client.RemoveImage(imageName)
}

// LoadImage invokes loads an image from an input stream, with a specified timeout
func (dg *dockerGoClient) LoadImage(ctx context.Context, inputStream io.Reader, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response := make(chan error, 1)
	go func() {
		response <- dg.loadImage(docker.LoadImageOptions{
			InputStream: inputStream,
			Context:     ctx,
		})
	}()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		return &DockerTimeoutError{timeout, "loading image"}
	}
}

func (dg *dockerGoClient) loadImage(opts docker.LoadImageOptions) error {
	client, err := dg.dockerClient()
	if err != nil {
		return err
	}
	return client.LoadImage(opts)
}
