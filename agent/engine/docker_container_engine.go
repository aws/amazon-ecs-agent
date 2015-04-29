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
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerauth"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"

	docker "github.com/fsouza/go-dockerclient"
)

const dockerStopTimeoutSeconds = 30

// Timelimits for docker operations enforced above docker
const (
	pullImageTimeout        = 2 * time.Hour
	createContainerTimeout  = 1 * time.Minute
	startContainerTimeout   = 1 * time.Minute
	stopContainerTimeout    = 1 * time.Minute
	removeContainerTimeout  = 5 * time.Minute
	inspectContainerTimeout = 10 * time.Second
)

// Interface to make testing it easier
type DockerClient interface {
	ContainerEvents(ctx context.Context) (<-chan DockerContainerChangeEvent, error)

	PullImage(image string) DockerContainerMetadata
	CreateContainer(*docker.Config, string) DockerContainerMetadata
	StartContainer(string, *docker.HostConfig) DockerContainerMetadata
	StopContainer(string) DockerContainerMetadata

	RemoveContainer(string) error

	GetContainerName(string) (string, error)
	InspectContainer(string) (*docker.Container, error)
	DescribeContainer(string) (api.ContainerStatus, error)

	Version() (string, error)
}

// Implements DockerClient
type DockerGoClient struct {
	dockerClient dockerclient.Client
}

func (dg *DockerGoClient) SetGoDockerClient(to dockerclient.Client) {
	dg.dockerClient = to
}

// pullLock is a temporary workaround for a devicemapper issue. See: https://github.com/docker/docker/issues/9718
var pullLock sync.Mutex

// scratchCreateLock guards against multiple 'scratch' image creations at once
var scratchCreateLock sync.Mutex

type DockerImageResponse struct {
	Images []docker.APIImages
}

func NewDockerGoClient() (*DockerGoClient, error) {
	endpoint := utils.DefaultIfBlank(os.Getenv(DOCKER_ENDPOINT_ENV_VARIABLE), DOCKER_DEFAULT_ENDPOINT)

	client, err := docker.NewVersionedClient(endpoint, "1.17")
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

	return &DockerGoClient{
		dockerClient: client,
	}, nil
}

func (dg *DockerGoClient) PullImage(image string) DockerContainerMetadata {
	timeout := ttime.After(pullImageTimeout)

	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.pullImage(image) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		return DockerContainerMetadata{Error: &DockerTimeoutError{pullImageTimeout, "pulled"}}
	}
}

func (dg *DockerGoClient) pullImage(image string) DockerContainerMetadata {
	log.Debug("Pulling image", "image", image)
	client := dg.dockerClient

	// Special case; this image is not one that should be pulled, but rather
	// should be created locally if necessary
	if image == emptyvolume.Image+":"+emptyvolume.Tag {
		err := dg.createScratchImageIfNotExists()
		return DockerContainerMetadata{Error: err}
	}

	authConfig := dockerauth.GetAuthconfig(image)
	// Workaround for devicemapper bug. See:
	// https://github.com/docker/docker/issues/9718
	pullLock.Lock()
	defer pullLock.Unlock()

	pullDebugOut, pullWriter := io.Pipe()
	defer pullWriter.Close()
	opts := docker.PullImageOptions{
		Repository:   image,
		OutputStream: pullWriter,
	}
	go func() {
		reader := bufio.NewReader(pullDebugOut)
		var line []byte
		var err error
		line, _, err = reader.ReadLine()
		for err == nil {
			log.Debug("Pulling image", "image", image, "status", string(line[:]))
			line, _, err = reader.ReadLine()
		}
		if err != nil && err != io.EOF {
			log.Warn("Error reading pull image status", "image", image, "err", err)
		}
	}()
	err := client.PullImage(opts, authConfig)

	return DockerContainerMetadata{Error: err}
}

func (dg *DockerGoClient) createScratchImageIfNotExists() error {
	c := dg.dockerClient

	scratchCreateLock.Lock()
	defer scratchCreateLock.Unlock()

	_, err := c.InspectImage(emptyvolume.Image + ":" + emptyvolume.Tag)
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
	err = c.ImportImage(docker.ImportImageOptions{
		Repository:  emptyvolume.Image,
		Tag:         emptyvolume.Tag,
		Source:      "-",
		InputStream: reader,
	})
	return err
}

func (dg *DockerGoClient) CreateContainer(config *docker.Config, name string) DockerContainerMetadata {
	timeout := ttime.After(createContainerTimeout)

	ctx, cancelFunc := context.WithCancel(context.TODO()) // Could pass one through from engine
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.createContainer(ctx, config, name) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		cancelFunc()
		return DockerContainerMetadata{Error: &DockerTimeoutError{createContainerTimeout, "created"}}
	}
}

func (dg *DockerGoClient) createContainer(ctx context.Context, config *docker.Config, name string) DockerContainerMetadata {
	client := dg.dockerClient

	containerOptions := docker.CreateContainerOptions{Config: config, Name: name}
	dockerContainer, err := client.CreateContainer(containerOptions)
	select {
	case <-ctx.Done():
		// Only inspect if we don't timeout; otherwise no one's listening for our
		// response anyways
		return DockerContainerMetadata{}
	default:
	}
	if err != nil {
		return DockerContainerMetadata{Error: err}
	}
	return dg.containerMetadata(dockerContainer.ID)
}

func (dg *DockerGoClient) StartContainer(id string, hostConfig *docker.HostConfig) DockerContainerMetadata {
	timeout := ttime.After(startContainerTimeout)

	ctx, cancelFunc := context.WithCancel(context.TODO()) // Could pass one through from engine
	response := make(chan DockerContainerMetadata, 1)
	go func() { response <- dg.startContainer(ctx, id, hostConfig) }()
	select {
	case resp := <-response:
		return resp
	case <-timeout:
		cancelFunc()
		return DockerContainerMetadata{Error: &DockerTimeoutError{startContainerTimeout, "started"}}
	}
}

func (dg *DockerGoClient) startContainer(ctx context.Context, id string, hostConfig *docker.HostConfig) DockerContainerMetadata {
	client := dg.dockerClient

	err := client.StartContainer(id, hostConfig)
	select {
	case <-ctx.Done():
		return DockerContainerMetadata{}
	default:
	}
	metadata := dg.containerMetadata(id)
	if err != nil {
		metadata.Error = err
	}

	return metadata
}

func dockerStateToState(state docker.State) api.ContainerStatus {
	if state.Running {
		return api.ContainerRunning
	}
	return api.ContainerStopped
}

func (dg *DockerGoClient) DescribeContainer(dockerId string) (api.ContainerStatus, error) {
	client := dg.dockerClient

	if len(dockerId) == 0 {
		return api.ContainerStatusNone, errors.New("Invalid container id: ''")
	}

	dockerContainer, err := client.InspectContainer(dockerId)
	if err != nil {
		return api.ContainerStatusNone, err
	}
	return dockerStateToState(dockerContainer.State), nil
}

func (dg *DockerGoClient) InspectContainer(dockerId string) (*docker.Container, error) {
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

func (dg *DockerGoClient) inspectContainer(dockerId string) (*docker.Container, error) {
	return dg.dockerClient.InspectContainer(dockerId)
}

func (dg *DockerGoClient) StopContainer(dockerId string) DockerContainerMetadata {
	timeout := ttime.After(stopContainerTimeout)

	ctx, cancelFunc := context.WithCancel(context.TODO()) // Could pass one through from engine
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

func (dg *DockerGoClient) stopContainer(ctx context.Context, dockerId string) DockerContainerMetadata {
	client := dg.dockerClient
	err := client.StopContainer(dockerId, dockerStopTimeoutSeconds)
	select {
	case <-ctx.Done():
		// No one waiting for us anyways
		return DockerContainerMetadata{}
	default:
	}
	metadata := dg.containerMetadata(dockerId)
	if err != nil {
		log.Debug("Error stopping container", "err", err, "id", dockerId)
		if metadata.Error == nil {
			metadata.Error = err
		}
	}
	return metadata
}

func (dg *DockerGoClient) RemoveContainer(dockerId string) error {
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

func (dg *DockerGoClient) removeContainer(dockerId string) error {
	return dg.dockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: dockerId, RemoveVolumes: true, Force: false})
}

func (dg *DockerGoClient) GetContainerName(id string) (string, error) {
	container, err := dg.InspectContainer(id)
	if err != nil {
		return "", err
	}
	return container.Name, nil
}

func (dg *DockerGoClient) containerMetadata(id string) DockerContainerMetadata {
	dockerContainer, err := dg.InspectContainer(id)
	if err != nil {
		return DockerContainerMetadata{Error: err}
	}
	var bindings []api.PortBinding
	if dockerContainer.NetworkSettings != nil {
		// Convert port bindings into the format our container expects
		bindings, err = api.PortBindingFromDockerPortBinding(dockerContainer.NetworkSettings.Ports)
		if err != nil {
			log.Crit("Docker had network bindings we couldn't understand", "err", err)
			return DockerContainerMetadata{Error: err}
		}
	}
	metadata := DockerContainerMetadata{
		DockerId:     id,
		PortBindings: bindings,
		Volumes:      dockerContainer.Volumes,
	}
	if dockerContainer.State.Running == false {
		metadata.ExitCode = &dockerContainer.State.ExitCode
	}
	if dockerContainer.State.Error != "" {
		// TODO type this so that it shows up as 'DockerError: '
		metadata.Error = errors.New(dockerContainer.State.Error)
	}
	if dockerContainer.State.OOMKilled {
		// TODO type this so it shows up as 'OutOfMemoryError: ...'
		metadata.Error = errors.New("Memory limit exceeded; container killed")
	}

	return metadata
}

// Listen to the docker event stream for container changes and pass them up
func (dg *DockerGoClient) ContainerEvents(ctx context.Context) (<-chan DockerContainerChangeEvent, error) {
	client := dg.dockerClient

	events := make(chan *docker.APIEvents)

	err := client.AddEventListener(events)
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
				fallthrough
			case "oom":
				fallthrough
			case "kill":
				status = api.ContainerStopped
			case "destroy":
			case "unpause":
				// These two result in us falling through to inspect the container even
				// though generally it won't cause any change
			case "pause":
				fallthrough
			case "export":
				fallthrough
			// Image events
			case "pull":
				fallthrough
			case "untag":
				fallthrough
			case "delete":
				// No interest in image events
				continue
			default:
				log.Info("Unknown status event! Maybe docker updated? ", "status", event.Status)
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

func (dg *DockerGoClient) Version() (string, error) {
	client := dg.dockerClient
	info, err := client.Version()
	if err != nil {
		return "", err
	}
	return "DockerVersion: " + info.Get("Version"), nil
}
