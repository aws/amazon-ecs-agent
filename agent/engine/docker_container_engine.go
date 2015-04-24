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

	"golang.org/x/net/context"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerauth"
	"github.com/aws/amazon-ecs-agent/agent/engine/emptyvolume"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	docker "github.com/fsouza/go-dockerclient"
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
	dockerClient *docker.Client
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
		log.Error("Unable to connect to docker deamon. Ensure docker is running", "endpoint", endpoint, "err", err)
		return nil, err
	}

	// Even if we have a dockerclient, the daemon might not be running. Ping it
	// to ensure it's up.
	err = client.Ping()
	if err != nil {
		log.Error("Unable to ping docker deamon. Ensure docker is running", "endpoint", endpoint, "err", err)
		return nil, err
	}

	return &DockerGoClient{
		dockerClient: client,
	}, nil
}

func (dg *DockerGoClient) PullImage(image string) DockerContainerMetadata {
	log.Debug("Pulling image", "image", image)
	client := dg.client()

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
	c := dg.client()

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
	client := dg.client()

	containerOptions := docker.CreateContainerOptions{Config: config, Name: name}
	dockerContainer, err := client.CreateContainer(containerOptions)
	if err != nil {
		return DockerContainerMetadata{Error: err}
	}
	return dg.containerMetadata(dockerContainer.ID)
}

func (dg *DockerGoClient) StartContainer(id string, hostConfig *docker.HostConfig) DockerContainerMetadata {
	client := dg.client()

	err := client.StartContainer(id, hostConfig)
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
	client := dg.client()

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
	return dg.client().InspectContainer(dockerId)
}

func (dg *DockerGoClient) StopContainer(dockerId string) DockerContainerMetadata {
	client := dg.client()
	err := client.StopContainer(dockerId, DEFAULT_TIMEOUT_SECONDS)
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
	return dg.client().RemoveContainer(docker.RemoveContainerOptions{ID: dockerId, RemoveVolumes: true, Force: false})
}

func (dg *DockerGoClient) StopContainerById(id string) DockerContainerMetadata {
	client := dg.client()
	err := client.StopContainer(id, DEFAULT_TIMEOUT_SECONDS)
	if err != nil {
		return DockerContainerMetadata{Error: err}
	}
	return dg.containerMetadata(id)
}

func (dg *DockerGoClient) GetContainerName(id string) (string, error) {
	client := dg.client()
	container, err := client.InspectContainer(id)
	if err != nil {
		return "", err
	}
	return container.Name, nil
}

// client returns the underlying docker client
func (dg *DockerGoClient) client() *docker.Client {
	return dg.dockerClient
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
	client := dg.client()

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
			case "pause":
			case "unpause":
			case "export":

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
	client := dg.client()
	info, err := client.Version()
	if err != nil {
		return "", err
	}
	return "DockerVersion: " + info.Get("Version"), nil
}
