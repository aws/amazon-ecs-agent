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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/async"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerauth"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/ecr"
	"github.com/aws/amazon-ecs-agent/agent/metrics"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
)

const (
	dockerDefaultTag = "latest"
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
const (
	// Parameters for caching the docker auth for ECR
	tokenCacheSize = 100
	// tokenCacheTTL is the default ttl of the docker auth for ECR
	tokenCacheTTL = 12 * time.Hour

	// pullStatusSuppressDelay controls the time where pull status progress bar
	// output will be suppressed in debug mode
	pullStatusSuppressDelay = 2 * time.Second

	// retry settings for pulling images
	maximumPullRetries        = 5
	minimumPullRetryDelay     = 1100 * time.Millisecond
	maximumPullRetryDelay     = 5 * time.Second
	pullRetryDelayMultiplier  = 2
	pullRetryJitterMultiplier = 0.2

	// pollStatsTimeout is the timeout for polling Docker Stats API;
	// keeping it same as streaming stats inactivity timeout
	pollStatsTimeout = 18 * time.Second
)

// stopContainerTimeoutBuffer is a buffer added to the timeout passed into the docker
// StopContainer api call. The reason for this buffer is that when the regular "stop"
// command fails, the docker api falls back to other kill methods, such as a containerd
// kill and SIGKILL. This buffer adds time onto the context timeout to allow time
// for these backup kill methods to finish.
var stopContainerTimeoutBuffer = 2 * time.Minute

type inactivityTimeoutHandlerFunc func(reader io.ReadCloser, timeout time.Duration, cancelRequest func(), canceled *uint32) (io.ReadCloser, chan<- struct{})

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
	ContainerEvents(context.Context) (<-chan DockerContainerChangeEvent, error)

	// PullImage pulls an image. authData should contain authentication data provided by the ECS backend.
	PullImage(context.Context, string, *apicontainer.RegistryAuthenticationData, time.Duration) DockerContainerMetadata

	// CreateContainer creates a container with the provided Config, HostConfig, and name. A timeout value
	// and a context should be provided for the request.
	CreateContainer(context.Context, *dockercontainer.Config, *dockercontainer.HostConfig, string, time.Duration) DockerContainerMetadata

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
	InspectContainer(context.Context, string, time.Duration) (*types.ContainerJSON, error)

	// TopContainer returns information about the top processes running in the specified container.  A timeout value and a context
	// should be provided for the request. The last argument is an optional parameter for passing in 'ps' arguments
	// as part of the top command.
	TopContainer(context.Context, string, time.Duration, ...string) (*dockercontainer.ContainerTopOKBody, error)

	// CreateContainerExec creates a new exec configuration to run an exec process with the provided Config. A timeout value
	// and a context should be provided for the request.
	CreateContainerExec(ctx context.Context, containerID string, execConfig types.ExecConfig, timeout time.Duration) (*types.IDResponse, error)

	// StartContainerExec starts an exec process already created in the docker host. A timeout value
	// and a context should be provided for the request.
	StartContainerExec(ctx context.Context, execID string, execStartCheck types.ExecStartCheck, timeout time.Duration) error

	// InspectContainerExec returns information about a specific exec process on the docker host. A timeout value
	// and a context should be provided for the request.
	InspectContainerExec(ctx context.Context, execID string, timeout time.Duration) (*types.ContainerExecInspect, error)

	// ListContainers returns the set of containers known to the Docker daemon. A timeout value and a context
	// should be provided for the request.
	ListContainers(context.Context, bool, time.Duration) ListContainersResponse

	// SystemPing returns the Ping response from Docker's SystemPing API
	SystemPing(context.Context, time.Duration) PingResponse

	// ListImages returns the set of the images known to the Docker daemon
	ListImages(context.Context, time.Duration) ListImagesResponse

	// CreateVolume creates a docker volume. A timeout value should be provided for the request
	CreateVolume(context.Context, string, string, map[string]string, map[string]string, time.Duration) SDKVolumeResponse

	// InspectVolume returns a volume by its name. A timeout value should be provided for the request
	InspectVolume(context.Context, string, time.Duration) SDKVolumeResponse

	// RemoveVolume removes a volume by its name. A timeout value should be provided for the request
	RemoveVolume(context.Context, string, time.Duration) error

	// ListPluginsWithFilters returns the set of docker plugins installed on the host, filtered by options provided.
	// A timeout value should be provided for the request.
	// TODO ListPluginsWithFilters can be removed since ListPlugins takes in filters
	ListPluginsWithFilters(context.Context, bool, []string, time.Duration) ([]string, error)

	// ListPlugins returns the set of docker plugins installed on the host. A timeout value should be provided for
	// the request.
	ListPlugins(context.Context, time.Duration, filters.Args) ListPluginsResponse

	// Stats returns a channel of stat data for the specified container. A context should be provided so the request can
	// be canceled.
	Stats(context.Context, string, time.Duration) (<-chan *types.StatsJSON, <-chan error)

	// Version returns the version of the Docker daemon.
	Version(context.Context, time.Duration) (string, error)

	// APIVersion returns the api version of the client
	APIVersion() (dockerclient.DockerVersion, error)

	// InspectImage returns information about the specified image.
	InspectImage(string) (*types.ImageInspect, error)

	// RemoveImage removes the metadata associated with an image and may remove the underlying layer data. A timeout
	// value and a context should be provided for the request.
	RemoveImage(context.Context, string, time.Duration) error

	// LoadImage loads an image from an input stream. A timeout value and a context should be provided for the request.
	LoadImage(context.Context, io.Reader, time.Duration) error

	// Info returns the information of the Docker server.
	Info(context.Context, time.Duration) (types.Info, error)
}

// DockerGoClient wraps the underlying go-dockerclient and docker/docker library.
// It exists primarily for the following four purposes:
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
// 4) Allows for both the go-dockerclient client and Docker SDK client to live
//    side-by-side until migration to the Docker SDK is complete.
// Implements DockerClient
// TODO Remove clientfactory field once all API calls are migrated to sdkclientFactory
type dockerGoClient struct {
	sdkClientFactory         sdkclientfactory.Factory
	version                  dockerclient.DockerVersion
	ecrClientFactory         ecr.ECRFactory
	auth                     dockerauth.DockerAuthProvider
	ecrTokenCache            async.Cache
	config                   *config.Config
	context                  context.Context
	imagePullBackoff         retry.Backoff
	inactivityTimeoutHandler inactivityTimeoutHandlerFunc

	_time     ttime.Time
	_timeOnce sync.Once

	daemonVersionUnsafe string
	lock                sync.Mutex
}

type ImagePullResponse struct {
	Id             string `json:"id,omitempty"`
	Status         string `json:"status,omitempty"`
	ProgressDetail struct {
		Current int64 `json:"current,omitempty"`
		Total   int64 `json:"total,omitempty"`
	} `json:"progressDetail,omitempty"`
	Progress string `json:"progress,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (dg *dockerGoClient) WithVersion(version dockerclient.DockerVersion) DockerClient {
	return &dockerGoClient{
		sdkClientFactory: dg.sdkClientFactory,
		version:          version,
		auth:             dg.auth,
		config:           dg.config,
		context:          dg.context,
	}
}

// NewDockerGoClient creates a new DockerGoClient
// TODO Remove clientfactory parameter once migration to Docker SDK is complete.
func NewDockerGoClient(sdkclientFactory sdkclientfactory.Factory,
	cfg *config.Config, ctx context.Context) (DockerClient, error) {
	// Ensure SDK client can connect to the Docker daemon.
	sdkclient, err := sdkclientFactory.GetDefaultClient()

	if err != nil {
		seelog.Errorf("DockerGoClient: Docker SDK client unable to connect to Docker daemon. "+
			"Ensure Docker is running: %v", err)
		return nil, err
	}

	// Even if we have a DockerClient, the daemon might not be running. Ping from both clients
	// to ensure it's up.
	_, err = sdkclient.Ping(ctx)
	if err != nil {
		seelog.Errorf("DockerGoClient: Docker SDK client unable to ping Docker daemon. "+
			"Ensure Docker is running: %v", err)
		return nil, err
	}

	var dockerAuthData json.RawMessage
	if cfg.EngineAuthData != nil {
		dockerAuthData = cfg.EngineAuthData.Contents()
	}
	return &dockerGoClient{
		sdkClientFactory: sdkclientFactory,
		auth:             dockerauth.NewDockerAuthProvider(cfg.EngineAuthType, dockerAuthData),
		ecrClientFactory: ecr.NewECRFactory(cfg.AcceptInsecureCert),
		ecrTokenCache:    async.NewLRUCache(tokenCacheSize, tokenCacheTTL),
		config:           cfg,
		context:          ctx,
		imagePullBackoff: retry.NewExponentialBackoff(minimumPullRetryDelay, maximumPullRetryDelay,
			pullRetryJitterMultiplier, pullRetryDelayMultiplier),
		inactivityTimeoutHandler: handleInactivityTimeout,
	}, nil
}

// Returns the Docker SDK Client
func (dg *dockerGoClient) sdkDockerClient() (sdkclient.Client, error) {
	if dg.version == "" {
		return dg.sdkClientFactory.GetDefaultClient()
	}
	return dg.sdkClientFactory.GetClient(dg.version)
}

func (dg *dockerGoClient) time() ttime.Time {
	dg._timeOnce.Do(func() {
		if dg._time == nil {
			dg._time = &ttime.DefaultTime{}
		}
	})
	return dg._time
}

func (dg *dockerGoClient) PullImage(ctx context.Context, image string,
	authData *apicontainer.RegistryAuthenticationData, timeout time.Duration) DockerContainerMetadata {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("PULL_IMAGE")()
	response := make(chan DockerContainerMetadata, 1)
	go func() {
		err := retry.RetryNWithBackoffCtx(ctx, dg.imagePullBackoff, maximumPullRetries,
			func() error {
				err := dg.pullImage(ctx, image, authData)
				if err != nil {
					seelog.Errorf("DockerGoClient: failed to pull image %s: [%s] %s", image, err.ErrorName(), err.Error())
				}
				return err
			})
		response <- DockerContainerMetadata{Error: wrapPullErrorAsNamedError(err)}
	}()

	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return DockerContainerMetadata{Error: &DockerTimeoutError{timeout, "pulled"}}
		}
		// Context was canceled even though there was no timeout. Send
		// back an error.
		return DockerContainerMetadata{Error: &CannotPullContainerError{err}}
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

func (dg *dockerGoClient) pullImage(ctx context.Context, image string,
	authData *apicontainer.RegistryAuthenticationData) apierrors.NamedError {
	seelog.Debugf("DockerGoClient: pulling image: %s", image)
	client, err := dg.sdkDockerClient()
	if err != nil {
		return CannotGetDockerClientError{version: dg.version, err: err}
	}

	sdkAuthConfig, err := dg.getAuthdata(image, authData)
	if err != nil {
		return wrapPullErrorAsNamedError(err)
	}
	// encode auth data
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(sdkAuthConfig); err != nil {
		return CannotPullECRContainerError{err}
	}

	imagePullOpts := types.ImagePullOptions{
		All:          false,
		RegistryAuth: base64.URLEncoding.EncodeToString(buf.Bytes()),
	}

	repository := getRepository(image)

	timeout := dg.time().After(dockerclient.DockerPullBeginTimeout)
	// pullBegan is a channel indicating that we have seen at least one line of data on the 'OutputStream' above.
	// It is here to guard against a bug wherein Docker never writes anything to that channel and hangs in pulling forever.
	pullBegan := make(chan bool, 1)
	// pullBeganOnce ensures we only indicate it began once (since our channel will only be read 0 or 1 times)
	pullBeganOnce := sync.Once{}

	pullFinished := make(chan error, 1)
	subCtx, cancelRequest := context.WithCancel(ctx)

	go func() {
		defer cancelRequest()
		reader, err := client.ImagePull(subCtx, repository, imagePullOpts)
		if err != nil {
			pullFinished <- err
			return
		}

		// handle inactivity timeout
		var canceled uint32
		var ch chan<- struct{}
		reader, ch = dg.inactivityTimeoutHandler(reader, dg.config.ImagePullInactivityTimeout, cancelRequest, &canceled)
		defer reader.Close()
		defer close(ch)
		decoder := json.NewDecoder(reader)
		data := new(ImagePullResponse)
		var statusDisplayed time.Time
		for err := decoder.Decode(data); err != io.EOF; err = decoder.Decode(data) {
			if err != nil {
				seelog.Warnf("DockerGoClient: Unable to decode pull event message for image %s: %v", image, err)
				pullFinished <- err
				return
			}
			if data.Error != "" {
				seelog.Warnf("DockerGoClient: Error while pulling image %s: %v", image, data.Error)
				pullFinished <- errors.New(data.Error)
			}
			if atomic.LoadUint32(&canceled) != 0 {
				seelog.Warnf("DockerGoClient: inactivity time exceeded timeout while pulling image %s", image)
				pullErr := errors.New("inactivity time exceeded timeout while pulling image")
				pullFinished <- pullErr
				return
			}

			pullBeganOnce.Do(func() {
				pullBegan <- true
			})

			statusDisplayed = dg.filterPullDebugOutput(data, image, statusDisplayed)

			data = new(ImagePullResponse)
		}
		pullFinished <- nil
	}()

	select {
	case <-pullBegan:
		break
	case pullErr := <-pullFinished:
		if pullErr != nil {
			return CannotPullContainerError{pullErr}
		}
		seelog.Debugf("DockerGoClient: pulling image complete: %s", image)
		return nil
	case <-timeout:
		return &DockerTimeoutError{dockerclient.DockerPullBeginTimeout, "pullBegin"}
	}
	seelog.Debugf("DockerGoClient: pull began for image: %s", image)

	err = <-pullFinished
	if err != nil {
		return CannotPullContainerError{err}
	}

	seelog.Debugf("DockerGoClient: pulling image complete: %s", image)
	return nil
}

func (dg *dockerGoClient) filterPullDebugOutput(data *ImagePullResponse, image string, statusDisplayed time.Time) time.Time {

	now := time.Now()
	if !strings.Contains(data.Progress, "[=") || now.After(statusDisplayed.Add(pullStatusSuppressDelay)) {
		// data.Progress shows the progress bar lines for Status=downlaoding or Extracting, logging data.Status to retain enough for debugging
		seelog.Debugf("DockerGoClient: pulling image %s, status %s", image, data.Status)
	}

	if strings.Contains(data.Status, "already being pulled by another client. Waiting.") {
		// This can mean the daemon is 'hung' in pulling status for this image, but we can't be sure.
		seelog.Errorf("DockerGoClient: image 'pull' status marked as already being pulled for image %s, status %s",
			image, data.Status)
	}
	return now
}

func getRepository(image string) string {
	repository, tag := utils.ParseRepositoryTag(image)
	if tag == "" {
		repository = repository + ":" + dockerDefaultTag
	} else {
		repository = image
	}
	return repository
}

func (dg *dockerGoClient) InspectImage(image string) (*types.ImageInspect, error) {
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("INSPECT_IMAGE")()
	client, err := dg.sdkDockerClient()
	if err != nil {
		return nil, err
	}
	imageData, _, err := client.ImageInspectWithRaw(dg.context, image)
	return &imageData, err
}

func (dg *dockerGoClient) getAuthdata(image string, authData *apicontainer.RegistryAuthenticationData) (types.AuthConfig, error) {

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
	config *dockercontainer.Config,
	hostConfig *dockercontainer.HostConfig,
	name string,
	timeout time.Duration) DockerContainerMetadata {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("CREATE_CONTAINER")()
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
	config *dockercontainer.Config,
	hostConfig *dockercontainer.HostConfig,
	name string) DockerContainerMetadata {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	dockerContainer, err := client.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, name)
	if err != nil {
		return DockerContainerMetadata{Error: CannotCreateContainerError{err}}
	}

	// TODO Remove ContainerInspect call
	return dg.containerMetadata(ctx, dockerContainer.ID)
}

func (dg *dockerGoClient) StartContainer(ctx context.Context, id string, timeout time.Duration) DockerContainerMetadata {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("START_CONTAINER")()
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
	client, err := dg.sdkDockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	err = client.ContainerStart(ctx, id, types.ContainerStartOptions{})
	metadata := dg.containerMetadata(ctx, id)
	if err != nil {
		metadata.Error = CannotStartContainerError{err}
	}

	return metadata
}

// DockerStateToState converts the container status from docker to status recognized by the agent
func DockerStateToState(state *types.ContainerState) apicontainerstatus.ContainerStatus {
	if state.Running {
		return apicontainerstatus.ContainerRunning
	}

	if state.Dead {
		return apicontainerstatus.ContainerStopped
	}

	// StartAt field in ContainerState is a string and need to convert to compare to zero time instant
	startTime, _ := time.Parse(time.RFC3339, state.StartedAt)
	if startTime.IsZero() && state.Error == "" {
		return apicontainerstatus.ContainerCreated
	}

	return apicontainerstatus.ContainerStopped
}

func (dg *dockerGoClient) DescribeContainer(ctx context.Context, dockerID string) (apicontainerstatus.ContainerStatus, DockerContainerMetadata) {
	dockerContainer, err := dg.InspectContainer(ctx, dockerID, dockerclient.InspectContainerTimeout)
	if err != nil {
		return apicontainerstatus.ContainerStatusNone, DockerContainerMetadata{Error: CannotDescribeContainerError{err}}
	}
	return DockerStateToState(dockerContainer.ContainerJSONBase.State), MetadataFromContainer(dockerContainer)
}

func (dg *dockerGoClient) InspectContainer(ctx context.Context, dockerID string, timeout time.Duration) (*types.ContainerJSON, error) {
	type inspectResponse struct {
		container *types.ContainerJSON
		err       error
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("INSPECT_CONTAINER")()
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

func (dg *dockerGoClient) inspectContainer(ctx context.Context, dockerID string) (*types.ContainerJSON, error) {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return nil, err
	}
	containerData, err := client.ContainerInspect(ctx, dockerID)
	return &containerData, err
}

func (dg *dockerGoClient) TopContainer(ctx context.Context, dockerID string, timeout time.Duration, psArgs ...string) (*dockercontainer.ContainerTopOKBody, error) {
	type topResponse struct {
		top *dockercontainer.ContainerTopOKBody
		err error
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("TOP_CONTAINER")()
	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan topResponse, 1)
	go func() {
		top, err := dg.topContainer(ctx, dockerID, psArgs...)
		response <- topResponse{top, err}
	}()

	// Wait until we get a response or for the 'done' context channel
	select {
	case resp := <-response:
		return resp.top, resp.err
	case <-ctx.Done():
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return nil, &DockerTimeoutError{timeout, "listing top"}
		}

		return nil, &CannotGetContainerTopError{err}
	}
}

func (dg *dockerGoClient) topContainer(ctx context.Context, dockerID string, psArgs ...string) (*dockercontainer.ContainerTopOKBody, error) {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return nil, err
	}
	topResponse, err := client.ContainerTop(ctx, dockerID, psArgs)
	return &topResponse, err
}

func (dg *dockerGoClient) StopContainer(ctx context.Context, dockerID string, timeout time.Duration) DockerContainerMetadata {
	ctxTimeout := timeout + stopContainerTimeoutBuffer
	ctx, cancel := context.WithTimeout(ctx, ctxTimeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("STOP_CONTAINER")()
	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.stopContainer(ctx, dockerID, timeout) }()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return DockerContainerMetadata{Error: &DockerTimeoutError{ctxTimeout, "stopped"}}
		}
		return DockerContainerMetadata{Error: CannotStopContainerError{err}}
	}
}

func (dg *dockerGoClient) stopContainer(ctx context.Context, dockerID string, timeout time.Duration) DockerContainerMetadata {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}
	err = client.ContainerStop(ctx, dockerID, &timeout)
	metadata := dg.containerMetadata(ctx, dockerID)
	if err != nil {
		seelog.Errorf("DockerGoClient: error stopping container ID=%s: %v", dockerID, err)
		if metadata.Error != nil {
			// Wrap metadata.Error in CannotStopContainerError in order to make the whole stopContainer operation
			// retryable.
			metadata.Error = CannotStopContainerError{metadata.Error}
		} else {
			if strings.Contains(err.Error(), "No such container") {
				err = NoSuchContainerError{dockerID}
			}
			metadata.Error = CannotStopContainerError{err}
		}
	}
	return metadata
}

func (dg *dockerGoClient) RemoveContainer(ctx context.Context, dockerID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("REMOVE_CONTAINER")()
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
	client, err := dg.sdkDockerClient()
	if err != nil {
		return err
	}
	return client.ContainerRemove(ctx, dockerID,
		types.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   false,
			Force:         false,
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
func MetadataFromContainer(dockerContainer *types.ContainerJSON) DockerContainerMetadata {
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

	createdTime, _ := time.Parse(time.RFC3339, dockerContainer.Created)
	startedTime := time.Time{}
	finishedTime := time.Time{}
	// Need to check for nil to make sure we do not try to access fields of nil pointer
	if dockerContainer.State != nil {
		startedTime, _ = time.Parse(time.RFC3339, dockerContainer.State.StartedAt)
		finishedTime, _ = time.Parse(time.RFC3339, dockerContainer.State.FinishedAt)
	}

	metadata := DockerContainerMetadata{
		DockerID:     dockerContainer.ID,
		PortBindings: bindings,
		Volumes:      dockerContainer.Mounts,
		CreatedAt:    createdTime,
		StartedAt:    startedTime,
		FinishedAt:   finishedTime,
	}

	if dockerContainer.NetworkSettings != nil {
		metadata.NetworkSettings = dockerContainer.NetworkSettings
	}

	if dockerContainer.HostConfig != nil {
		metadata.NetworkMode = string(dockerContainer.HostConfig.NetworkMode)
	}

	if dockerContainer.Config != nil {
		metadata.Labels = dockerContainer.Config.Labels
	}

	if dockerContainer.State == nil {
		return metadata
	}
	if !dockerContainer.State.Running && !finishedTime.IsZero() {
		// Only record an exitcode if it has exited
		metadata.ExitCode = &dockerContainer.State.ExitCode
	}
	if dockerContainer.State.Error != "" {
		metadata.Error = NewDockerStateError(dockerContainer.State.Error)
	}
	if dockerContainer.State.OOMKilled {
		metadata.Error = OutOfMemoryError{}
	}
	// Health field in Docker SDK is a pointer, need to check before not nil before dereference.
	if dockerContainer.State.Health == nil || dockerContainer.State.Health.Status == "" || dockerContainer.State.Health.Status == healthCheckStarting {
		return metadata
	}
	// Record the health check information if exists
	metadata.Health = getMetadataHealthCheck(dockerContainer)
	return metadata
}

func getMetadataHealthCheck(dockerContainer *types.ContainerJSON) apicontainer.HealthStatus {
	health := apicontainer.HealthStatus{}
	if dockerContainer.State == nil || dockerContainer.State.Health == nil {
		return health
	}
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
	client, err := dg.sdkDockerClient()
	if err != nil {
		return nil, err
	}

	events := make(chan *events.Message)
	buffer := NewInfiniteBuffer()

	derivedCtx, cancel := context.WithCancel(ctx)
	dockerEvents, eventErr := client.Events(derivedCtx, types.EventsOptions{})

	// Cache the event from docker client. Channel closes when an error is passed to eventErr.
	go buffer.StartListening(derivedCtx, dockerEvents)
	// Receive errors from channels. If error thrown is not EOF, log and reopen channel.
	// TODO: move the error check into StartListening() to keep event streaming and error handling in one place.
	go func() {
		for {
			select {
			case err := <-eventErr:
				// If parent ctx has been canceled, stop listening and return. Otherwise reopen the stream.
				if ctx.Err() != nil {
					return
				}

				if err == io.EOF {
					seelog.Infof("DockerGoClient: Docker events stream closed with: %v", err)
				} else {
					seelog.Errorf("DockerGoClient: Docker events stream closed with error: %v", err)
				}

				// Reopen a new event stream to continue listening.
				nextCtx, nextCancel := context.WithCancel(ctx)
				dockerEvents, eventErr = client.Events(nextCtx, types.EventsOptions{})
				// Cache the event from docker client.
				go buffer.StartListening(nextCtx, dockerEvents)
				// Close previous stream after starting to listen on new one
				cancel()
				// Reassign cancel variable next Cancel function to setup next iteration of loop.
				cancel = nextCancel
			case <-ctx.Done():
				return
			}
		}
	}()

	// Read the buffered events and send to task engine
	go buffer.Consume(events)
	changedContainers := make(chan DockerContainerChangeEvent)
	go dg.handleContainerEvents(ctx, events, changedContainers)

	return changedContainers, nil
}

func (dg *dockerGoClient) handleContainerEvents(ctx context.Context,
	events <-chan *events.Message,
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
			Status:                  status,
			Type:                    eventType,
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
	client, err := dg.sdkDockerClient()
	if err != nil {
		return ListContainersResponse{Error: err}
	}

	containers, err := client.ContainerList(ctx, types.ContainerListOptions{
		All: all,
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

func (dg *dockerGoClient) ListImages(ctx context.Context, timeout time.Duration) ListImagesResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response := make(chan ListImagesResponse, 1)
	go func() { response <- dg.listImages(ctx) }()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return ListImagesResponse{Error: &DockerTimeoutError{timeout, "listing"}}
		}
		return ListImagesResponse{Error: &CannotListImagesError{err}}
	}
}

func (dg *dockerGoClient) listImages(ctx context.Context) ListImagesResponse {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return ListImagesResponse{Error: err}
	}
	images, err := client.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return ListImagesResponse{Error: err}
	}
	var imageRepoTags []string
	imageIDs := make([]string, len(images))
	for i, image := range images {
		imageIDs[i] = image.ID
		imageRepoTags = append(imageRepoTags, image.RepoTags...)
	}
	return ListImagesResponse{ImageIDs: imageIDs, RepoTags: imageRepoTags, Error: nil}
}

func (dg *dockerGoClient) SystemPing(ctx context.Context, timeout time.Duration) PingResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan PingResponse, 1)
	go func() { response <- dg.systemPing(ctx) }()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		// Context has either expired or canceled. If it has timed out,
		// send back the DockerTimeoutError
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return PingResponse{Error: &DockerTimeoutError{timeout, "listing"}}
		}
		return PingResponse{Error: err}
	}
}

func (dg *dockerGoClient) systemPing(ctx context.Context) PingResponse {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return PingResponse{Error: err}
	}

	pingResponse, err := client.Ping(ctx)
	if err != nil {
		return PingResponse{Error: err}
	}

	return PingResponse{Response: &pingResponse}
}

func (dg *dockerGoClient) SupportedVersions() []dockerclient.DockerVersion {
	return dg.sdkClientFactory.FindSupportedAPIVersions()
}

func (dg *dockerGoClient) KnownVersions() []dockerclient.DockerVersion {
	return dg.sdkClientFactory.FindKnownAPIVersions()
}

func (dg *dockerGoClient) Version(ctx context.Context, timeout time.Duration) (string, error) {
	version := dg.getDaemonVersion()
	if version != "" {
		return version, nil
	}

	derivedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := dg.sdkDockerClient()
	if err != nil {
		return "", err
	}
	info, err := client.ServerVersion(derivedCtx)
	if err != nil {
		return "", err
	}

	version = info.Version
	dg.setDaemonVersion(version)
	return version, nil
}

func (dg *dockerGoClient) Info(ctx context.Context, timeout time.Duration) (types.Info, error) {
	derivedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := dg.sdkDockerClient()
	if err != nil {
		return types.Info{}, err
	}
	info, infoErr := client.Info(derivedCtx)
	if infoErr != nil {
		return types.Info{}, infoErr
	}

	return info, nil
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
	timeout time.Duration) SDKVolumeResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("CREATE_VOLUME")()
	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan SDKVolumeResponse, 1)
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
			return SDKVolumeResponse{DockerVolume: nil, Error: &DockerTimeoutError{timeout, "creating volume"}}
		}
		// Context was canceled even though there was no timeout. Send
		// back an error.
		return SDKVolumeResponse{DockerVolume: nil, Error: &CannotCreateVolumeError{err}}
	}
}

func (dg *dockerGoClient) createVolume(ctx context.Context,
	name string,
	driver string,
	driverOptions map[string]string,
	labels map[string]string) SDKVolumeResponse {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return SDKVolumeResponse{DockerVolume: nil, Error: &CannotGetDockerClientError{version: dg.version, err: err}}
	}

	volumeOptions := volume.VolumeCreateBody{
		Driver:     driver,
		DriverOpts: driverOptions,
		Labels:     labels,
		Name:       name,
	}
	dockerVolume, err := client.VolumeCreate(ctx, volumeOptions)
	if err != nil {
		return SDKVolumeResponse{DockerVolume: nil, Error: &CannotCreateVolumeError{err}}
	}

	return SDKVolumeResponse{DockerVolume: &dockerVolume, Error: nil}
}

func (dg *dockerGoClient) InspectVolume(ctx context.Context, name string, timeout time.Duration) SDKVolumeResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("INSPECT_VOLUME")()
	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan SDKVolumeResponse, 1)
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
			return SDKVolumeResponse{DockerVolume: nil, Error: &DockerTimeoutError{timeout, "inspecting volume"}}
		}
		// Context was canceled even though there was no timeout. Send
		// back an error.
		return SDKVolumeResponse{DockerVolume: nil, Error: &CannotInspectVolumeError{err}}
	}
}

func (dg *dockerGoClient) inspectVolume(ctx context.Context, name string) SDKVolumeResponse {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return SDKVolumeResponse{
			DockerVolume: nil,
			Error:        &CannotGetDockerClientError{version: dg.version, err: err}}
	}

	dockerVolume, err := client.VolumeInspect(ctx, name)
	if err != nil {
		return SDKVolumeResponse{DockerVolume: nil, Error: &CannotInspectVolumeError{err}}
	}

	return SDKVolumeResponse{DockerVolume: &dockerVolume, Error: nil}
}

func (dg *dockerGoClient) RemoveVolume(ctx context.Context, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("REMOVE_VOLUME")()
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
	client, err := dg.sdkDockerClient()
	if err != nil {
		return &CannotGetDockerClientError{version: dg.version, err: err}
	}

	err = client.VolumeRemove(ctx, name, false)
	if err != nil {
		return &CannotRemoveVolumeError{err}
	}

	return nil
}

// ListPluginsWithFilters takes in filter arguments and returns the string of filtered Plugin names
func (dg *dockerGoClient) ListPluginsWithFilters(ctx context.Context, enabled bool, capabilities []string, timeout time.Duration) ([]string, error) {
	// Create filter list
	filterList := filters.NewArgs(filters.Arg("enabled", strconv.FormatBool(enabled)))
	for _, capability := range capabilities {
		filterList.Add("capability", capability)
	}

	var filteredPluginNames []string
	response := dg.ListPlugins(ctx, timeout, filterList)

	if response.Error != nil {
		return nil, response.Error
	}
	// Create a list of the filtered plugin names
	for _, plugin := range response.Plugins {
		filteredPluginNames = append(filteredPluginNames, plugin.Name)
	}
	return filteredPluginNames, nil
}

func (dg *dockerGoClient) ListPlugins(ctx context.Context, timeout time.Duration, filters filters.Args) ListPluginsResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan ListPluginsResponse, 1)
	go func() { response <- dg.listPlugins(ctx, filters) }()

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

func (dg *dockerGoClient) listPlugins(ctx context.Context, filters filters.Args) ListPluginsResponse {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return ListPluginsResponse{Plugins: nil, Error: &CannotGetDockerClientError{version: dg.version, err: err}}
	}

	plugins, err := client.PluginList(ctx, filters)
	if err != nil {
		return ListPluginsResponse{Plugins: nil, Error: &CannotListPluginsError{err}}
	}

	return ListPluginsResponse{Plugins: plugins, Error: nil}
}

// APIVersion returns the client api version
func (dg *dockerGoClient) APIVersion() (dockerclient.DockerVersion, error) {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return "", err
	}
	return dg.sdkClientFactory.FindClientAPIVersion(client), nil
}

// Stats returns a channel of *types.StatsJSON entries for the container.
func (dg *dockerGoClient) Stats(ctx context.Context, id string, inactivityTimeout time.Duration) (<-chan *types.StatsJSON, <-chan error) {
	subCtx, cancelRequest := context.WithCancel(ctx)

	errC := make(chan error)
	statsC := make(chan *types.StatsJSON)
	client, err := dg.sdkDockerClient()
	if err != nil {
		cancelRequest()
		go func() {
			// upstream function should consume error
			errC <- err
			close(statsC)
		}()
		return statsC, errC
	}

	var resp types.ContainerStats
	if !dg.config.PollMetrics.Enabled() {
		// Streaming metrics is the default behavior
		seelog.Infof("DockerGoClient: Starting streaming metrics for container %s", id)
		go func() {
			defer cancelRequest()
			defer close(statsC)
			stream := true
			resp, err = client.ContainerStats(subCtx, id, stream)
			if err != nil {
				errC <- fmt.Errorf("DockerGoClient: Unable to retrieve stats for container %s: %v", id, err)
				return
			}

			// handle inactivity timeout
			var canceled uint32
			var ch chan<- struct{}
			resp.Body, ch = dg.inactivityTimeoutHandler(resp.Body, inactivityTimeout, cancelRequest, &canceled)
			defer resp.Body.Close()
			defer close(ch)

			decoder := json.NewDecoder(resp.Body)
			data := new(types.StatsJSON)
			for err := decoder.Decode(data); err != io.EOF; err = decoder.Decode(data) {
				if err != nil {
					errC <- fmt.Errorf("DockerGoClient: Unable to decode stats for container %s: %v", id, err)
					return
				}
				if atomic.LoadUint32(&canceled) != 0 {
					errC <- fmt.Errorf("DockerGoClient: inactivity time exceeded timeout while retrieving stats for container %s", id)
					return
				}

				statsC <- data
				data = new(types.StatsJSON)
			}
		}()
	} else {
		seelog.Infof("DockerGoClient: Starting to Poll for metrics for container %s", id)

		go func() {
			defer cancelRequest()
			defer close(statsC)
			// we need to start by getting container stats so that the task stats
			// endpoint will be populated immediately.
			stats, err := getContainerStatsNotStreamed(client, subCtx, id, pollStatsTimeout)
			if err != nil {
				errC <- err
				return
			}
			statsC <- stats

			// sleeping here jitters the time at which the ticker is created, so that
			// containers do not synchronize on calling the docker stats api.
			// the max sleep is 80% of the polling interval so that we have a chance to
			// get two stats in the first publishing interval.
			time.Sleep(retry.AddJitter(time.Nanosecond, dg.config.PollingMetricsWaitDuration*8/10))
			statPollTicker := time.NewTicker(dg.config.PollingMetricsWaitDuration)
			defer statPollTicker.Stop()
			for range statPollTicker.C {
				stats, err := getContainerStatsNotStreamed(client, subCtx, id, pollStatsTimeout)
				if err != nil {
					errC <- err
					return
				}
				statsC <- stats
			}
		}()
	}

	return statsC, errC
}

func getContainerStatsNotStreamed(client sdkclient.Client, ctx context.Context, id string, timeout time.Duration) (*types.StatsJSON, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type statsResponse struct {
		stats types.ContainerStats
		err   error
	}
	response := make(chan statsResponse, 1)
	go func() {
		stats, err := client.ContainerStats(ctxWithTimeout, id, false)
		response <- statsResponse{stats, err}
	}()
	select {
	case resp := <-response:
		if resp.err != nil {
			return nil, fmt.Errorf("DockerGoClient: Unable to retrieve stats for container %s: %v", id, resp.err)
		}
		decoder := json.NewDecoder(resp.stats.Body)
		stats := &types.StatsJSON{}
		err := decoder.Decode(stats)
		if err != nil {
			return nil, fmt.Errorf("DockerGoClient: Unable to decode stats for container %s: %v", id, err)
		}
		defer resp.stats.Body.Close()
		return stats, nil
	case <-ctxWithTimeout.Done():
		err := ctxWithTimeout.Err()
		if err == context.DeadlineExceeded {
			return nil, fmt.Errorf("DockerGoClient: timed out retrieving stats for container %s", id)
		}
		return nil, fmt.Errorf("DockerGoClient: Unable to retrieve stats for container %s: %v", id, err)
	}
}

func (dg *dockerGoClient) RemoveImage(ctx context.Context, imageName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response := make(chan error, 1)
	go func() { response <- dg.removeImage(ctx, imageName) }()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		return &DockerTimeoutError{timeout, "removing image"}
	}
}

func (dg *dockerGoClient) removeImage(ctx context.Context, imageName string) error {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return err
	}
	_, err = client.ImageRemove(ctx, imageName, types.ImageRemoveOptions{})
	return err
}

// LoadImage invokes loads an image from an input stream, with a specified timeout
func (dg *dockerGoClient) LoadImage(ctx context.Context, inputStream io.Reader, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("LOAD_IMAGE")()
	response := make(chan error, 1)
	go func() {
		response <- dg.loadImage(ctx, inputStream)
	}()
	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		return &DockerTimeoutError{timeout, "loading image"}
	}
}

func (dg *dockerGoClient) loadImage(ctx context.Context, reader io.Reader) error {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return err
	}
	resp, err := client.ImageLoad(ctx, reader, false)
	if err != nil {
		return err
	}

	// flush and close response reader
	if resp.Body != nil {
		defer resp.Body.Close()
		_, err = io.Copy(ioutil.Discard, resp.Body)
	}
	return err
}

func (dg *dockerGoClient) CreateContainerExec(ctx context.Context, containerID string, execConfig types.ExecConfig, timeout time.Duration) (*types.IDResponse, error) {
	type createContainerExecResponse struct {
		execID *types.IDResponse
		err    error
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("CREATE_CONTAINER_EXEC")()
	response := make(chan createContainerExecResponse, 1)
	go func() {
		execIDresponse, err := dg.createContainerExec(ctx, containerID, execConfig)
		response <- createContainerExecResponse{execIDresponse, err}
	}()

	select {
	case resp := <-response:
		return resp.execID, resp.err
	case <-ctx.Done():
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return nil, &DockerTimeoutError{timeout, "exec command"}
		}
		return nil, &CannotCreateContainerExecError{err}
	}
}

func (dg *dockerGoClient) createContainerExec(ctx context.Context, containerID string, config types.ExecConfig) (*types.IDResponse, error) {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return nil, err
	}

	execIDResponse, err := client.ContainerExecCreate(ctx, containerID, config)
	if err != nil {
		return nil, &CannotCreateContainerExecError{err}
	}
	return &execIDResponse, nil
}

func (dg *dockerGoClient) StartContainerExec(ctx context.Context, execID string, execStartCheck types.ExecStartCheck, timeout time.Duration) error {

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("START_CONTAINER_EXEC")()
	response := make(chan error, 1)
	go func() {
		err := dg.startContainerExec(ctx, execID, execStartCheck)
		response <- err
	}()

	select {
	case resp := <-response:
		return resp
	case <-ctx.Done():
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return &DockerTimeoutError{timeout, "start exec command"}
		}
		return &CannotStartContainerExecError{err}
	}
}

func (dg *dockerGoClient) startContainerExec(ctx context.Context, execID string, execStartCheck types.ExecStartCheck) error {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return err
	}

	err = client.ContainerExecStart(ctx, execID, execStartCheck)
	if err != nil {
		return &CannotStartContainerExecError{err}
	}
	return nil
}

func (dg *dockerGoClient) InspectContainerExec(ctx context.Context, execID string, timeout time.Duration) (*types.ContainerExecInspect, error) {
	type inspectContainerExecResponse struct {
		execInspect *types.ContainerExecInspect
		err         error
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer metrics.MetricsEngineGlobal.RecordDockerMetric("INSPECT_CONTAINER_EXEC")()
	response := make(chan inspectContainerExecResponse, 1)
	go func() {
		execInspectResponse, err := dg.inspectContainerExec(ctx, execID)
		response <- inspectContainerExecResponse{execInspectResponse, err}
	}()

	select {
	case resp := <-response:
		return resp.execInspect, resp.err

	case <-ctx.Done():
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			return nil, &DockerTimeoutError{timeout, "inspect exec command"}
		}
		return nil, &CannotInspectContainerExecError{err}
	}
}

func (dg *dockerGoClient) inspectContainerExec(ctx context.Context, containerID string) (*types.ContainerExecInspect, error) {
	client, err := dg.sdkDockerClient()
	if err != nil {
		return nil, err
	}

	execInspectResponse, err := client.ContainerExecInspect(ctx, containerID)
	if err != nil {
		return nil, &CannotInspectContainerExecError{err}
	}
	return &execInspectResponse, nil
}
