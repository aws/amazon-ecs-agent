// Copyright 2015-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"time"

	"golang.org/x/net/context"

	"github.com/aws/amazon-ecs-init/ecs-init/backoff"
	"github.com/aws/amazon-ecs-init/ecs-init/config"

	log "github.com/cihub/seelog"
	godocker "github.com/fsouza/go-dockerclient"
)

const (
	logDir   = "/log"
	dataDir  = "/data"
	readOnly = ":ro"
	// set default to /var/run instead of /var/run/docker.sock in case
	// /var/run/docker.sock is deleted and recreated outside the container
	defaultDockerEndpoint = "/var/run"
	// networkMode specifies the networkmode to create the agent container
	networkMode = "host"
	// minBackoffDuration specifies the minimum backoff duration for ping to
	// return a success response from the docker socket
	minBackoffDuration = time.Second
	// maxBackoffDuration specifies the maximum backoff duration for ping to
	// return a success response from docker socket
	maxBackoffDuration = 5 * time.Second
	// backoffJitterMultiple specifies the backoff jitter multiplier
	// coefficient when pinging the docker socket
	backoffJitterMultiple = 0.2
	// backoffMultiple specifies the backoff multiplier coefficient when
	// pinging the docker socket
	backoffMultiple = 2
	// maxRetries specifies the maximum number of retries for ping to return
	// a successful response from the docker socket
	maxRetries = 5
)

// Client enables business logic for running the Agent inside Docker
type Client struct {
	docker dockerclient
	fs     fileSystem
}

// NewClient reutrns a new Client
func NewClient() (*Client, error) {
	// Create a backoff for pinging the docker socker. This should result in 17-19
	// seconds of delay in the worst-case between different actions that depend on
	// docker
	pingBackoff := backoff.NewBackoff(minBackoffDuration, maxBackoffDuration, backoffJitterMultiple,
		backoffMultiple, maxRetries)
	client, err := newDockerClient(godockerClientFactory{}, pingBackoff)
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
	return c.docker.LoadImage(godocker.LoadImageOptions{image, context.TODO()})
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

	// default environment variables
	envVariables := map[string]string{
		"ECS_LOGFILE":                           logDir + "/" + config.AgentLogFile,
		"ECS_DATADIR":                           dataDir,
		"ECS_AGENT_CONFIG_FILE_PATH":            config.AgentJSONConfigFile(),
		"ECS_UPDATE_DOWNLOAD_DIR":               config.CacheDirectory(),
		"ECS_UPDATES_ENABLED":                   "true",
		"ECS_AVAILABLE_LOGGING_DRIVERS":         `["json-file","syslog","awslogs"]`,
		"ECS_ENABLE_TASK_IAM_ROLE":              "true",
		"ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST": "true",
	}

	// merge in user-supplied environment variables
	for envKey, envValue := range c.loadEnvVariables() {
		envVariables[envKey] = envValue
	}

	var env []string
	for envKey, envValue := range envVariables {
		env = append(env, envKey+"="+envValue)
	}

	return &godocker.Config{
		Env:   env,
		Image: config.AgentImageName,
	}
}

func (c *Client) loadEnvVariables() map[string]string {
	envVariables := make(map[string]string)

	file, err := c.fs.ReadFile(config.AgentConfigFile())
	if err != nil {
		return envVariables
	}

	lines := strings.Split(strings.TrimSpace(string(file)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 {
			continue
		}
		envVariables[parts[0]] = parts[1]
	}

	return envVariables
}

func (c *Client) getHostConfig() *godocker.HostConfig {
	dockerEndpointAgent := defaultDockerEndpoint
	dockerUnixSocketSourcePath, fromEnv := config.DockerUnixSocket()
	if fromEnv {
		dockerEndpointAgent = "/var/run/docker.sock"
	}

	binds := []string{
		dockerUnixSocketSourcePath + ":" + dockerEndpointAgent,
		config.LogDirectory() + ":" + logDir,
		config.AgentDataDirectory() + ":" + dataDir,
		config.AgentConfigDirectory() + ":" + config.AgentConfigDirectory(),
		config.CacheDirectory() + ":" + config.CacheDirectory(),
	}
	return &godocker.HostConfig{
		Binds:       binds,
		NetworkMode: networkMode,
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
