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
	"archive/tar"
	"bufio"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ecr"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerauth"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockeriface"
	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/cihub/seelog"
	"github.com/docker/docker/pkg/parsers"

	docker "github.com/fsouza/go-dockerclient"
)

const (
	dockerStopTimeoutSeconds = 30
	dockerDefaultTag         = "latest"
)

// Timelimits for docker operations enforced above docker
const (
	pullImageTimeout        = 2 * time.Hour
	createContainerTimeout  = 3 * time.Minute
	startContainerTimeout   = 1*time.Minute + 30*time.Second
	stopContainerTimeout    = 1 * time.Minute
	removeContainerTimeout  = 5 * time.Minute
	inspectContainerTimeout = 30 * time.Second
	listContainersTimeout   = 10 * time.Minute

	// dockerPullBeginTimeout is the timeout from when a 'pull' is called to when
	// we expect to see output on the pull progress stream. This is to work
	// around a docker bug which sometimes results in pulls not progressing.
	dockerPullBeginTimeout = 5 * time.Minute
)

// Interface to make testing it easier
type DockerClient interface {
	// SupportedVersions returns a slice of the supported docker versions (or at least supposedly supported).
	SupportedVersions() []dockerclient.DockerVersion
	// WithVersion returns a new DockerClient for which all operations will use the given remote api version.
	// A default version will be used for a client not produced via this method.
	WithVersion(dockerclient.DockerVersion) DockerClient
	ContainerEvents(ctx context.Context) (<-chan DockerContainerChangeEvent, error)

	PullImage(image string, authData *api.RegistryAuthenticationData) DockerContainerMetadata
	CreateContainer(*docker.Config, *docker.HostConfig, string) DockerContainerMetadata
	StartContainer(string) DockerContainerMetadata
	StopContainer(string) DockerContainerMetadata
	DescribeContainer(string) (api.ContainerStatus, DockerContainerMetadata)

	RemoveContainer(string) error

	GetContainerName(string) (string, error)
	InspectContainer(string) (*docker.Container, error)

	ListContainers(bool) ListContainersResponse

	Version() (string, error)
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
	clientFactory    dockerclient.Factory
	version          dockerclient.DockerVersion
	auth             dockerauth.DockerAuthProvider
	ecrClientFactory ecr.ECRFactory
}

func (dg *dockerGoClient) WithVersion(version dockerclient.DockerVersion) DockerClient {
	return &dockerGoClient{
		clientFactory: dg.clientFactory,
		version:       version,
		auth:          dg.auth,
	}
}

// pullLock is a temporary workaround for a devicemapper issue. See: https://github.com/docker/docker/issues/9718
var pullLock sync.Mutex

// scratchCreateLock guards against multiple 'scratch' image creations at once
var scratchCreateLock sync.Mutex

type DockerImageResponse struct {
	Images []docker.APIImages
}

// NewDockerGoClient creates a new DockerGoClient
func NewDockerGoClient(clientFactory dockerclient.Factory, authType string, authData *config.SensitiveRawMessage, acceptInsecureCert bool) (DockerClient, error) {
	endpoint := utils.DefaultIfBlank(os.Getenv(DOCKER_ENDPOINT_ENV_VARIABLE), DOCKER_DEFAULT_ENDPOINT)
	if clientFactory == nil {
		clientFactory = dockerclient.NewFactory(endpoint)
	}

	client, err := clientFactory.GetDefaultClient()
	if err != nil {
		log.Error("Unable to connect to docker daemon . Ensure docker is running", "endpoint", endpoint, "err", err)
		return nil, err
	}

	// Even if we have a dockerclient, the daemon might not be running. Ping it
	// to ensure it's up.
	err = client.Ping()
	if err != nil {
		log.Error("Unable to ping docker daemon. Ensure docker is running", "endpoint", endpoint, "err", err)
		return nil, err
	}

	return &dockerGoClient{
		clientFactory:    clientFactory,
		auth:             dockerauth.NewDockerAuthProvider(authType, authData.Contents()),
		ecrClientFactory: ecr.NewECRFactory(acceptInsecureCert),
	}, nil
}

func (dg *dockerGoClient) dockerClient() (dockeriface.Client, error) {
	if dg.version == "" {
		return dg.clientFactory.GetDefaultClient()
	}
	return dg.clientFactory.GetClient(dg.version)
}

func (dg *dockerGoClient) PullImage(image string, authData *api.RegistryAuthenticationData) DockerContainerMetadata {
	timeout := ttime.After(pullImageTimeout)

	// Workaround for devicemapper bug. See:
	// https://github.com/docker/docker/issues/9718
	pullLock.Lock()
	defer pullLock.Unlock()

	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.pullImage(image, authData) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		return DockerContainerMetadata{Error: &DockerTimeoutError{pullImageTimeout, "pulled"}}
	}
}

func (dg *dockerGoClient) pullImage(image string, authData *api.RegistryAuthenticationData) DockerContainerMetadata {
	log.Debug("Pulling image", "image", image)
	client, err := dg.dockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	// Special case; this image is not one that should be pulled, but rather
	// should be created locally if necessary
	if image == emptyvolume.Image+":"+emptyvolume.Tag {
		err := dg.createScratchImageIfNotExists()
		if err != nil {
			return DockerContainerMetadata{Error: &api.DefaultNamedError{Name: "CreateEmptyVolumeError", Err: "Could not create empty volume " + err.Error()}}
		}
		return DockerContainerMetadata{}
	}

	authConfig, err := dg.getAuthdata(image, authData)
	if err != nil {
		return DockerContainerMetadata{Error: err}
	}

	pullDebugOut, pullWriter := io.Pipe()
	defer pullWriter.Close()

	repository, tag := parsers.ParseRepositoryTag(image)
	if tag == "" {
		repository = repository + ":" + dockerDefaultTag
	} else {
		repository = image
	}

	opts := docker.PullImageOptions{
		Repository:   repository,
		OutputStream: pullWriter,
	}
	timeout := ttime.After(dockerPullBeginTimeout)
	// pullBegan is a channel indicating that we have seen at least one line of data on the 'OutputStream' above.
	// It is here to guard against a bug wherin docker never writes anything to that channel and hangs in pulling forever.
	pullBegan := make(chan bool, 1)
	// pullBeganOnce ensures we only indicate it began once (since our channel will only be read 0 or 1 times)
	pullBeganOnce := sync.Once{}

	go func() {
		reader := bufio.NewReader(pullDebugOut)
		var line string
		var err error
		for err == nil {
			line, err = reader.ReadString('\n')
			if err != nil {
				break
			}
			pullBeganOnce.Do(func() {
				pullBegan <- true
			})
			log.Debug("Pulling image", "image", image, "status", line)
			if strings.Contains(line, "already being pulled by another client. Waiting.") {
				// This can mean the deamon is 'hung' in pulling status for this image, but we can't be sure.
				log.Error("Image 'pull' status marked as already being pulled", "image", image, "status", line)
			}
		}
		if err != nil && err != io.EOF {
			log.Warn("Error reading pull image status", "image", image, "err", err)
		}
	}()
	pullFinished := make(chan error, 1)
	go func() {
		pullFinished <- client.PullImage(opts, authConfig)
		log.Debug("Pulling image complete", "image", image)
	}()

	select {
	case <-pullBegan:
		break
	case err := <-pullFinished:
		if err != nil {
			return DockerContainerMetadata{Error: CannotXContainerError{"Pull", err.Error()}}
		}
		return DockerContainerMetadata{}
	case <-timeout:
		return DockerContainerMetadata{Error: &DockerTimeoutError{dockerPullBeginTimeout, "pullBegin"}}
	}
	log.Debug("Pull began for image", "image", image)
	defer log.Debug("Pull completed for image", "image", image)

	err = <-pullFinished
	if err != nil {
		return DockerContainerMetadata{Error: CannotXContainerError{"Pull", err.Error()}}
	}
	return DockerContainerMetadata{}
}

func (dg *dockerGoClient) createScratchImageIfNotExists() error {
	client, err := dg.dockerClient()
	if err != nil {
		return err
	}

	scratchCreateLock.Lock()
	defer scratchCreateLock.Unlock()

	_, err = client.InspectImage(emptyvolume.Image + ":" + emptyvolume.Tag)
	if err == nil {
		// Already exists; assume that it's okay to use it
		return nil
	}

	reader, writer := io.Pipe()

	emptytarball := tar.NewWriter(writer)
	go func() {
		emptytarball.Close()
		writer.Close()
	}()

	// Create it from an empty tarball
	err = client.ImportImage(docker.ImportImageOptions{
		Repository:  emptyvolume.Image,
		Tag:         emptyvolume.Tag,
		Source:      "-",
		InputStream: reader,
	})
	return err
}

func (dg *dockerGoClient) getAuthdata(image string, authData *api.RegistryAuthenticationData) (docker.AuthConfiguration, error) {
	if authData == nil || authData.Type != "ecr" {
		return dg.auth.GetAuthconfig(image)
	}
	provider := dockerauth.NewECRAuthProvider(authData.ECRAuthData, dg.ecrClientFactory)
	authConfig, err := provider.GetAuthconfig(image)
	if err != nil {
		return authConfig, CannotXContainerError{"PullECR", err.Error()}
	}
	return authConfig, nil
}

func (dg *dockerGoClient) CreateContainer(config *docker.Config, hostConfig *docker.HostConfig, name string) DockerContainerMetadata {
	timeout := ttime.After(createContainerTimeout)

	ctx, cancelFunc := context.WithCancel(context.TODO()) // Could pass one through from engine
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.createContainer(ctx, config, hostConfig, name) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		cancelFunc()
		return DockerContainerMetadata{Error: &DockerTimeoutError{createContainerTimeout, "created"}}
	}
}

func (dg *dockerGoClient) createContainer(ctx context.Context, config *docker.Config, hostConfig *docker.HostConfig, name string) DockerContainerMetadata {
	client, err := dg.dockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	containerOptions := docker.CreateContainerOptions{Config: config, HostConfig: hostConfig, Name: name}
	dockerContainer, err := client.CreateContainer(containerOptions)
	select {
	case <-ctx.Done():
		// Parent function already timed out; no need to get container metadata
		return DockerContainerMetadata{}
	default:
	}
	if err != nil {
		return DockerContainerMetadata{Error: CannotXContainerError{"Create", err.Error()}}
	}
	return dg.containerMetadata(dockerContainer.ID)
}

func (dg *dockerGoClient) StartContainer(id string) DockerContainerMetadata {
	timeout := ttime.After(startContainerTimeout)

	ctx, cancelFunc := context.WithCancel(context.TODO()) // Could pass one through from engine
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.startContainer(ctx, id) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		cancelFunc()
		return DockerContainerMetadata{Error: &DockerTimeoutError{startContainerTimeout, "started"}}
	}
}

func (dg *dockerGoClient) startContainer(ctx context.Context, id string) DockerContainerMetadata {
	client, err := dg.dockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	err = client.StartContainer(id, nil)
	select {
	case <-ctx.Done():
		// Parent function already timed out; no need to get container metadata
		return DockerContainerMetadata{}
	default:
	}
	metadata := dg.containerMetadata(id)
	if err != nil {
		metadata.Error = CannotXContainerError{"Start", err.Error()}
	}

	return metadata
}

func dockerStateToState(state docker.State) api.ContainerStatus {
	if state.Running {
		return api.ContainerRunning
	}
	return api.ContainerStopped
}

func (dg *dockerGoClient) DescribeContainer(dockerId string) (api.ContainerStatus, DockerContainerMetadata) {
	dockerContainer, err := dg.InspectContainer(dockerId)
	if err != nil {
		return api.ContainerStatusNone, DockerContainerMetadata{Error: CannotXContainerError{"Describe", err.Error()}}
	}
	return dockerStateToState(dockerContainer.State), metadataFromContainer(dockerContainer)
}

func (dg *dockerGoClient) InspectContainer(dockerId string) (*docker.Container, error) {
	timeout := ttime.After(inspectContainerTimeout)

	type inspectResponse struct {
		container *docker.Container
		err       error
	}
	response := make(chan inspectResponse, 1)
	go func() {
		container, err := dg.inspectContainer(dockerId)
		response <- inspectResponse{container, err}
	}()
	select {
	case resp := <-response:
		return resp.container, resp.err
	case <-timeout:
		return nil, &DockerTimeoutError{inspectContainerTimeout, "inspecting"}
	}
}

func (dg *dockerGoClient) inspectContainer(dockerId string) (*docker.Container, error) {
	client, err := dg.dockerClient()
	if err != nil {
		return nil, err
	}
	return client.InspectContainer(dockerId)
}

func (dg *dockerGoClient) StopContainer(dockerId string) DockerContainerMetadata {
	timeout := ttime.After(stopContainerTimeout)

	ctx, cancelFunc := context.WithCancel(context.TODO()) // Could pass one through from engine
	// Buffered channel so in the case of timeout it takes one write, never gets
	// read, and can still be GC'd
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.stopContainer(ctx, dockerId) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		cancelFunc()
		return DockerContainerMetadata{Error: &DockerTimeoutError{stopContainerTimeout, "stopped"}}
	}
}

func (dg *dockerGoClient) stopContainer(ctx context.Context, dockerId string) DockerContainerMetadata {
	client, err := dg.dockerClient()
	if err != nil {
		return DockerContainerMetadata{Error: CannotGetDockerClientError{version: dg.version, err: err}}
	}

	err = client.StopContainer(dockerId, dockerStopTimeoutSeconds)
	select {
	case <-ctx.Done():
		// parent function has already timed out and returned; we're writing to a
		// buffered channel that will never be read
		return DockerContainerMetadata{}
	default:
	}
	metadata := dg.containerMetadata(dockerId)
	if err != nil {
		log.Debug("Error stopping container", "err", err, "id", dockerId)
		if metadata.Error == nil {
			metadata.Error = CannotXContainerError{"Stop", err.Error()}
		}
	}
	return metadata
}

func (dg *dockerGoClient) RemoveContainer(dockerId string) error {
	timeout := ttime.After(removeContainerTimeout)

	response := make(chan error, 1)
	go func() { response <- dg.removeContainer(dockerId) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		return &DockerTimeoutError{removeContainerTimeout, "removing"}
	}
}

func (dg *dockerGoClient) removeContainer(dockerId string) error {
	client, err := dg.dockerClient()
	if err != nil {
		return err
	}
	return client.RemoveContainer(docker.RemoveContainerOptions{ID: dockerId, RemoveVolumes: true, Force: false})
}

func (dg *dockerGoClient) GetContainerName(id string) (string, error) {
	container, err := dg.InspectContainer(id)
	if err != nil {
		return "", err
	}
	return container.Name, nil
}

func (dg *dockerGoClient) containerMetadata(id string) DockerContainerMetadata {
	dockerContainer, err := dg.InspectContainer(id)
	if err != nil {
		return DockerContainerMetadata{DockerId: id, Error: CannotXContainerError{"Inspect", err.Error()}}
	}
	return metadataFromContainer(dockerContainer)
}

func metadataFromContainer(dockerContainer *docker.Container) DockerContainerMetadata {
	var bindings []api.PortBinding
	var err api.NamedError
	if dockerContainer.NetworkSettings != nil {
		// Convert port bindings into the format our container expects
		bindings, err = api.PortBindingFromDockerPortBinding(dockerContainer.NetworkSettings.Ports)
		if err != nil {
			log.Crit("Docker had network bindings we couldn't understand", "err", err)
			return DockerContainerMetadata{Error: api.NamedError(err)}
		}
	}
	metadata := DockerContainerMetadata{
		DockerId:     dockerContainer.ID,
		PortBindings: bindings,
		Volumes:      dockerContainer.Volumes,
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

	return metadata
}

// Listen to the docker event stream for container changes and pass them up
func (dg *dockerGoClient) ContainerEvents(ctx context.Context) (<-chan DockerContainerChangeEvent, error) {
	client, err := dg.dockerClient()
	if err != nil {
		return nil, err
	}
	events := make(chan *docker.APIEvents)

	err = client.AddEventListener(events)
	if err != nil {
		log.Error("Unable to add a docker event listener", "err", err)
		return nil, err
	}
	go func() {
		<-ctx.Done()
		client.RemoveEventListener(events)
	}()

	changedContainers := make(chan DockerContainerChangeEvent)

	go func() {
		for event := range events {
			containerId := event.ID
			if containerId == "" {
				continue
			}
			log.Debug("Got event from docker daemon", "event", event)

			var status api.ContainerStatus
			switch event.Status {
			case "create":
				status = api.ContainerCreated
			case "start":
				status = api.ContainerRunning
			case "stop":
				fallthrough
			case "die":
				status = api.ContainerStopped
			case "kill":
				fallthrough
			case "rename":
				// TODO, ensure this wasn't one of our containers. This isn't critical
				// because we typically have the docker id stored too and a wrong name
				// won't be fatal once we do
				continue
			case "restart":
			case "resize":
			case "destroy":
			case "unpause":
				// These result in us falling through to inspect the container, some
				// out of caution, some because it's a form of state change

			case "oom":
				seelog.Infof("process within container %v died due to OOM", event.ID)
				// "oom" can either means any process got OOM'd, but doesn't always
				// mean the container dies (non-init processes). If the container also
				// dies, you see a "die" status as well; we'll update suitably there
				fallthrough
			case "pause":
				// non image events that aren't of interest currently
				fallthrough
			case "exec_create":
				fallthrough
			case "exec_start":
				fallthrough
			case "top":
				fallthrough
			case "attach":
				fallthrough
			// image events
			case "export":
				fallthrough
			case "pull":
				fallthrough
			case "push":
				fallthrough
			case "tag":
				fallthrough
			case "untag":
				fallthrough
			case "import":
				fallthrough
			case "delete":
				// No interest in image events
				continue
			default:
				if strings.HasPrefix(event.Status, "exec_create:") || strings.HasPrefix(event.Status, "exec_start:") {
					continue
				}

				// Because docker emits new events even when you use an old event api
				// version, it's not that big a deal
				seelog.Debugf("Unknown status event from docker: %s", event.Status)
			}

			metadata := dg.containerMetadata(containerId)

			changedContainers <- DockerContainerChangeEvent{
				Status:                  status,
				DockerContainerMetadata: metadata,
			}
		}
	}()

	return changedContainers, nil
}

// ListContainers returns a slice of container IDs.
func (dg *dockerGoClient) ListContainers(all bool) ListContainersResponse {
	timeout := ttime.After(listContainersTimeout)

	response := make(chan ListContainersResponse, 1)
	go func() { response <- dg.listContainers(all) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		return ListContainersResponse{Error: &DockerTimeoutError{listContainersTimeout, "listing"}}
	}
}

func (dg *dockerGoClient) listContainers(all bool) ListContainersResponse {
	client, err := dg.dockerClient()
	if err != nil {
		return ListContainersResponse{Error: err}
	}

	containers, err := client.ListContainers(docker.ListContainersOptions{All: all})
	if err != nil {
		return ListContainersResponse{Error: err}
	}

	// We get an empty slice if there are no containers to be listed.
	// Extract container IDs from this list.
	containerIDs := make([]string, len(containers))
	for i, container := range containers {
		containerIDs[i] = container.ID
	}

	return ListContainersResponse{DockerIds: containerIDs, Error: nil}
}

func (dg *dockerGoClient) SupportedVersions() []dockerclient.DockerVersion {
	return dg.clientFactory.FindAvailableVersions()
}

func (dg *dockerGoClient) Version() (string, error) {
	client, err := dg.dockerClient()
	if err != nil {
		return "", err
	}
	info, err := client.Version()
	if err != nil {
		return "", err
	}
	return "DockerVersion: " + info.Get("Version"), nil
}
