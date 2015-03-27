// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
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

const (
	agentIntrospectionPort = "51678"
	logDir                 = "/log"
	dataDir                = "/data"
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
	for _, image := range images {
		for _, repoTag := range image.RepoTags {
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
	containerToRemove, err := c.findAgentContainer()
	if err != nil {
		return err
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

func (c *Client) findAgentContainer() (string, error) {
	// TODO pagination
	containers, err := c.docker.ListContainers(godocker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"status": []string{},
		},
	})
	if err != nil {
		return "", err
	}
	agentContainerName := "/" + config.AgentContainerName
	for _, container := range containers {
		for _, name := range container.Names {
			log.Infof("Container name: %s", name)
			if name == agentContainerName {
				return container.ID, nil
			}
		}
	}
	return "", nil
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
		"ECS_LOGFILE="+logDir+"/"+config.AgentLogFile,
		"ECS_DATADIR="+dataDir,
		"ECS_AGENT_CONFIG_FILE_PATH="+config.AgentJSONConfigFile())

	exposedPorts := map[godocker.Port]struct{}{
		agentIntrospectionPort + "/tcp": struct{}{},
	}

	return &godocker.Config{
		Env:          env,
		ExposedPorts: exposedPorts,
		Image:        config.AgentImageName,
	}
}

func (c *Client) loadEnvVariables() []string {
	file, err := c.fs.ReadFile(config.AgentConfigFile())
	if err != nil {
		return make([]string, 0)
	}
	return strings.Split(strings.TrimSpace(string(file)), "\n")
}

func (c *Client) getHostConfig() *godocker.HostConfig {
	binds := []string{
		defaultDockerEndpoint + ":" + defaultDockerEndpoint,
		config.LogDirectory() + ":" + logDir,
		config.AgentDataDirectory() + ":" + dataDir,
		config.AgentConfigDirectory() + ":" + config.AgentConfigDirectory(),
	}
	portBindings := map[godocker.Port][]godocker.PortBinding{
		agentIntrospectionPort + "/tcp": []godocker.PortBinding{
			godocker.PortBinding{
				HostIP:   "127.0.0.1",
				HostPort: agentIntrospectionPort,
			},
		},
	}
	return &godocker.HostConfig{
		Binds:        binds,
		PortBindings: portBindings,
	}
}

// StopAgent stops the Agent in docker if one is running
func (c *Client) StopAgent() error {
	id, err := c.findAgentContainer()
	if err != nil {
		return err
	}
	if id == "" {
		log.Info("No running Agent to stop")
		return nil
	}
	stopContainerTimeoutSeconds := uint(10)
	return c.docker.StopContainer(id, stopContainerTimeoutSeconds)
}
