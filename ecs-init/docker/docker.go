// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package docker

import (
	"io"
	"strings"

	"github.com/aws/amazon-ecs-init/ecs-init/config"

	log "github.com/cihub/seelog"
	godocker "github.com/fsouza/go-dockerclient"
)

// Client enables business logic for running the Agent inside Docker
type Client struct {
	docker dockerclient
	fs     fileSystem
}

// NewClient reutrns a new Client
func NewClient() (*Client, error) {
	client, err := newDockerClient()
	if err != nil {
		return nil, err
	}
	return &Client{
		docker: client,
		fs:     standardFS,
	}, nil
}

// IsAgentImageLoaded returns true if the Agent image is loaded in Docker
func (c *Client) IsAgentImageLoaded() (bool, error) {
	images, err := c.docker.ListImages(godocker.ListImagesOptions{
		All: true,
	})
	if err != nil {
		return false, err
	}
	for i, image := range images {
		for _, repoTag := range image.RepoTags {
			log.Infof("Image %d tag %s", i, repoTag)
			if repoTag == config.AgentImageName {
				return true, nil
			}
		}
	}
	return false, nil
}

// LoadImage loads an io.Reader into Docker
func (c *Client) LoadImage(image io.Reader) error {
	return c.docker.LoadImage(godocker.LoadImageOptions{image})
}

// RemoveExistingAgentContainer remvoes any existing container named
// "ecs-agent" or returns without error if none is found
func (c *Client) RemoveExistingAgentContainer() error {
	// TODO pagination
	containers, err := c.docker.ListContainers(godocker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"status": []string{"exited"},
		},
	})
	if err != nil {
		return err
	}
	var containerToRemove string
	agentContainerName := "/" + config.AgentContainerName
	for _, container := range containers {
		for _, name := range container.Names {
			if name == agentContainerName {
				containerToRemove = container.ID
				break
			}
		}
	}
	if containerToRemove == "" {
		log.Info("No existing agent container to remove.")
		return nil
	}
	log.Infof("Removing existing agent container ID: %s", containerToRemove)
	err = c.docker.RemoveContainer(godocker.RemoveContainerOptions{
		ID:    containerToRemove,
		Force: true,
	})
	return err
}

// StartAgent starts the Agent in Docker and returns the exit code from the container
func (c *Client) StartAgent() (int, error) {
	container, err := c.docker.CreateContainer(godocker.CreateContainerOptions{
		Name:       config.AgentContainerName,
		Config:     c.getContainerConfig(),
		HostConfig: c.getHostConfig(),
	})
	if err != nil {
		return 0, err
	}
	err = c.docker.StartContainer(container.ID, nil)
	if err != nil {
		return 0, err
	}
	return c.docker.WaitContainer(container.ID)
}

func (c *Client) getContainerConfig() *godocker.Config {
	env := append(c.loadEnvVariables(),
		"ECS_LOGFILE=/log/"+config.AgentLogFile,
		"ECS_DATADIR=/data",
		"ECS_AGENT_CONFIG_FILE_PATH="+config.AgentJSONConfigFile)

	exposedPorts := map[godocker.Port]struct{}{
		"51678/tcp": struct{}{},
	}

	volumes := map[string]struct{}{
		defaultDockerEndpoint:       struct{}{},
		"/log":                      struct{}{},
		"/data":                     struct{}{},
		config.AgentConfigDirectory: struct{}{},
	}

	return &godocker.Config{
		Env:          env,
		ExposedPorts: exposedPorts,
		Image:        config.AgentImageName,
		Volumes:      volumes,
	}
}

func (c *Client) loadEnvVariables() []string {
	file, err := c.fs.ReadFile(config.AgentConfigFile)
	if err != nil {
		return make([]string, 0)
	}
	return strings.Split(strings.TrimSpace(string(file)), "\n")
}

func (c *Client) getHostConfig() *godocker.HostConfig {
	binds := []string{
		defaultDockerEndpoint + ":" + defaultDockerEndpoint,
		config.LogDirectory + ":/log",
		config.AgentDataDirectory + ":/data",
		config.AgentConfigDirectory + ":" + config.AgentConfigDirectory,
	}
	portBindings := map[godocker.Port][]godocker.PortBinding{
		"51678/tcp": []godocker.PortBinding{
			godocker.PortBinding{
				HostIP:   "127.0.0.1",
				HostPort: "51678",
			},
		},
	}
	return &godocker.HostConfig{
		Binds:        binds,
		PortBindings: portBindings,
	}
}
