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
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerauth"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	dockerparsers "github.com/docker/docker/pkg/parsers"
	dockerregistry "github.com/docker/docker/registry"

	docker "github.com/fsouza/go-dockerclient"
)

// Interface to make testing it easier
type DockerClient interface {
	ContainerEvents() (<-chan DockerContainerChangeEvent, error)

	PullImage(image string) error
	CreateContainer(*docker.Config, string) (string, error)
	StartContainer(string, *docker.HostConfig) error
	StopContainer(string) error
	RemoveContainer(string) error
	GetContainerName(string) (string, error)

	InspectContainer(string) (*docker.Container, error)
	DescribeContainer(string) (api.ContainerStatus, error)

	client() (*docker.Client, error)
}

// Implements DockerClient
type DockerGoClient struct{}

// dockerClient is a singleton
var dockerclient *docker.Client

// pullLock is a temporary workaround for a devicemapper issue. See: https://github.com/docker/docker/issues/9718
var pullLock sync.Mutex

type DockerImageResponse struct {
	Images []docker.APIImages
}

func NewDockerGoClient() (*DockerGoClient, error) {
	dg := &DockerGoClient{}

	client, err := dg.client()
	if err != nil {
		log.Error("Unable to create a docker client!", "err", err)
		return dg, err
	}

	// Even if we have a dockerclient, the daemon might not be running. Ping it
	// to ensure it's up.
	err = client.Ping()

	return dg, err
}

func (dg *DockerGoClient) PullImage(image string) error {
	log.Info("Pulling image", "image", image)
	client, err := dg.client()
	if err != nil {
		return err
	}
	// The following lines of code are taken, in whole or part, from the docker
	// source code. Please see the NOTICE file in the root of the project for
	// attribution
	// https://github.com/docker/docker/blob/246ec5dd067fc17be5196ae29956e3368b167ccf/api/client/commands.go#L1180
	taglessRemote, tag := dockerparsers.ParseRepositoryTag(image)
	if tag == "" {
		tag = "latest"
	}

	hostname, _, err := dockerregistry.ResolveRepositoryName(taglessRemote)
	if err != nil {
		return err
	}
	// End of docker-attributed code

	authConfig := dockerauth.GetAuthconfig(hostname)

	// Workaround for devicemapper bug. See:
	// https://github.com/docker/docker/issues/9718
	pullLock.Lock()
	defer pullLock.Unlock()

	pullDebugOut, pullWriter := io.Pipe()
	opts := docker.PullImageOptions{
		Repository:   taglessRemote,
		Registry:     hostname,
		Tag:          tag,
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
		if err != nil {
			log.Error("Error reading pull image status", "image", image, "err", err)
		}
	}()
	err = client.PullImage(opts, authConfig)

	return err
}

func (dg *DockerGoClient) CreateContainer(config *docker.Config, name string) (string, error) {
	client, err := dg.client()
	if err != nil {
		return "", err
	}

	// Ensure this image was pulled so this can be a quick operation (taskEngine
	// is blocked on this)
	_, err = client.InspectImage(config.Image)
	if err != nil {
		return "", err
	}
	// TODO, race condition here: images should not be able to be deleted
	// between that inspect and the CreateContainer below

	containerOptions := docker.CreateContainerOptions{Config: config, Name: name}
	dockerContainer, err := client.CreateContainer(containerOptions)

	if err != nil {
		return "", err
	}
	return dockerContainer.ID, nil
}

func (dg *DockerGoClient) StartContainer(id string, hostConfig *docker.HostConfig) error {
	client, err := dg.client()
	if err != nil {
		return err
	}

	err = client.StartContainer(id, hostConfig)
	if err != nil {
		return err
	}

	return nil
}

func dockerStateToState(state docker.State) api.ContainerStatus {
	if state.Running {
		return api.ContainerRunning
	}
	return api.ContainerStopped
}

func (dg *DockerGoClient) DescribeContainer(dockerId string) (api.ContainerStatus, error) {
	client, err := dg.client()
	if err != nil {
		return api.ContainerStatusUnknown, err
	}

	if len(dockerId) == 0 {
		return api.ContainerStatusUnknown, errors.New("Invalid container id: ''")
	}

	dockerContainer, err := client.InspectContainer(dockerId)
	if err != nil {
		return api.ContainerStatusUnknown, err
	}
	return dockerStateToState(dockerContainer.State), nil
}

func (dg *DockerGoClient) InspectContainer(dockerId string) (*docker.Container, error) {
	client, err := dg.client()
	if err != nil {
		return nil, err
	}
	return client.InspectContainer(dockerId)
}

// DescribeDockerImages takes no arguments, and returns a JSON-encoded string of all of the images located on the host
func (dg *DockerGoClient) DescribeDockerImages() (string, error) {
	client, err := dg.client()
	if err != nil {
		return "", err
	}

	imgs, err := client.ListImages(true)
	if err != nil {
		return "", err
	}
	response := DockerImageResponse{Images: imgs}
	output, err := json.Marshal(response)
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func (dg *DockerGoClient) StopContainer(dockerId string) error {
	client, err := dg.client()
	if err != nil {
		return err
	}
	return client.StopContainer(dockerId, DEFAULT_TIMEOUT_SECONDS)
}

func (dg *DockerGoClient) RemoveContainer(dockerId string) error {
	client, err := dg.client()
	if err != nil {
		return err
	}
	return client.RemoveContainer(docker.RemoveContainerOptions{ID: dockerId, RemoveVolumes: true, Force: false})
}

func (dg *DockerGoClient) StopContainerById(id string) error {
	client, err := dg.client()
	if err != nil {
		return err
	}
	return client.StopContainer(id, DEFAULT_TIMEOUT_SECONDS)
}

func (dg *DockerGoClient) GetContainerName(id string) (string, error) {
	client, err := dg.client()
	if err != nil {
		return "", err
	}
	container, err := client.InspectContainer(id)
	if err != nil {
		return "", err
	}
	return container.Name, nil
}

// client returns the last used client if one has worked in the past, or a newly
// created one if one has not been created yet
func (dg *DockerGoClient) client() (*docker.Client, error) {
	if dockerclient != nil {
		return dockerclient, nil
	}

	// Re-read the env in case they corrected it
	endpoint := utils.DefaultIfBlank(os.Getenv(DOCKER_ENDPOINT_ENV_VARIABLE), DOCKER_DEFAULT_ENDPOINT)

	client, err := docker.NewVersionedClient(endpoint, "1.15")
	if err != nil {
		log.Error("Unable to conect to docker client. Ensure daemon is running", "endpoint", endpoint, "err", err)
		return nil, err
	}
	dockerclient = client

	return dockerclient, err
}

// Listen to the docker event stream for container changes and pass them up
func (dg *DockerGoClient) ContainerEvents() (<-chan DockerContainerChangeEvent, error) {
	client, err := dg.client()
	if err != nil {
		log.Error("Unable to communicate with docker daemon", "err", err)
		return nil, err
	}

	events := make(chan *docker.APIEvents)

	err = client.AddEventListener(events)
	if err != nil {
		log.Error("Unable to add a docker event listener", "err", err)
		return nil, err
	}

	changedContainers := make(chan DockerContainerChangeEvent)

	go func() {
		for event := range events {
			log.Debug("Got event from docker daemon", "event", event)
			containerId := event.ID
			image := event.From

			var status api.ContainerStatus
			switch event.Status {
			case "create":
				status = api.ContainerCreated
			case "start":
				status = api.ContainerRunning
			case "stop":
				status = api.ContainerStopped
			case "die":
				status = api.ContainerDead
			case "kill":
				status = api.ContainerDead
			case "destroy":
			case "pause":
			case "unpause":
			case "export":

			// Image events
			case "untag":
			case "delete":
			default:
				log.Warn("Unknown status event! Maybe docker updated? ", "status", event.Status)
			}
			changedContainers <- DockerContainerChangeEvent{DockerId: containerId, Image: image, Status: status}
		}
	}()

	return changedContainers, nil
}
