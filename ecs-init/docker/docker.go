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

	"github.com/aws/amazon-ecs-init/ecs-init/backoff"
	"github.com/aws/amazon-ecs-init/ecs-init/config"

	log "github.com/cihub/seelog"
	godocker "github.com/fsouza/go-dockerclient"
)

const (
	// logDir specifies the location of Agent log files in the container
	logDir = "/log"
	// dataDir specifies the location of Agent state file in the container
	dataDir = "/data"
	// readOnly specifies the read-only suffix for mounting host volumes
	// when creating the Agent container
	readOnly = ":ro"
	// hostProcDir binds the host's /proc directory to /host/proc within the
	// ECS Agent container
	// The ECS Agent needs access to host's /proc directory when configuring
	// the network namespace of containers for tasks that are configured
	// with an ENI
	hostProcDir = "/host/proc"
	// defaultDockerEndpoint is set to /var/run instead of /var/run/docker.sock
	// in case /var/run/docker.sock is deleted and recreated outside the container
	defaultDockerEndpoint = "/var/run"
	// dhclientLeasesLocation specifies the location where dhclient leases
	// information is tracked in the Agent container
	dhclientLeasesLocation = "/var/lib/dhclient"
	// dhclientLibDir specifies the location of shared libraries on the
	// host and in the Agent container required for the execution of the dhclient
	// executable
	dhclientLibDir = "/lib64"
	// dhclientExecutableDir specifies the location of the dhclient
	// executable on the  host and in the Agent container
	dhclientExecutableDir = "/sbin"
	// networkMode specifies the networkmode to create the agent container
	networkMode = "host"
	// usernsMode specifies the userns mode to create the agent container
	usernsMode = "host"
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
	// CapNetAdmin to start agent with NET_ADMIN capability
	// For more information on capabilities, please read this manpage:
	// http://man7.org/linux/man-pages/man7/capabilities.7.html
	CapNetAdmin = "NET_ADMIN"
	// CapSysAdmin to start agent with SYS_ADMIN capability
	// This is needed for the ECS Agent to invoke the setns call when
	// configuring the network namespace of the pause container
	// For more information on setns, please read this manpage:
	// http://man7.org/linux/man-pages/man2/setns.2.html
	CapSysAdmin = "SYS_ADMIN"
	// DefaultCgroupMountpoint is the default mount point for the cgroup subsystem
	DefaultCgroupMountpoint = "/sys/fs/cgroup"
	// pluginSocketFilesDir specifies the location of UNIX domain socket files of
	// Docker plugins
	pluginSocketFilesDir = "/run/docker/plugins"
	// pluginSpecFilesEtcDir specifies one of the locations of spec or json files
	// of Docker plugins
	pluginSpecFilesEtcDir = "/etc/docker/plugins"
	// pluginSpecFilesUsrDir specifies one of the locations of spec or json files
	// of Docker plugins
	pluginSpecFilesUsrDir = "/usr/lib/docker/plugins"
)

var pluginDirs = []string{
	pluginSocketFilesDir,
	pluginSpecFilesEtcDir,
	pluginSpecFilesUsrDir,
}

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
	return c.docker.LoadImage(godocker.LoadImageOptions{InputStream: image})
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
	hostConfig := c.getHostConfig()

	container, err := c.docker.CreateContainer(godocker.CreateContainerOptions{
		Name:       config.AgentContainerName,
		Config:     c.getContainerConfig(),
		HostConfig: hostConfig,
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
		"ECS_AVAILABLE_LOGGING_DRIVERS":         `["json-file","syslog","awslogs","none"]`,
		"ECS_ENABLE_TASK_IAM_ROLE":              "true",
		"ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST": "true",
	}

	// for al, al2 add host ssl cert directory envvar if available
	if certDir := config.HostCertsDirPath(); certDir != "" {
		envVariables["SSL_CERT_DIR"] = certDir
	}

	// merge in platform-specific environment variables
	for envKey, envValue := range getPlatformSpecificEnvVariables() {
		envVariables[envKey] = envValue
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
		config.CgroupMountpoint() + ":" + DefaultCgroupMountpoint,
	}

	// for al, al2 add host ssl cert directory mounts
	if pkiDir := config.HostPKIDirPath(); pkiDir != "" {
		certsPath := pkiDir + ":" + pkiDir + readOnly
		binds = append(binds, certsPath)
	}

	binds = append(binds, getDockerPluginDirBinds()...)
	return createHostConfig(binds)
}

func getDockerPluginDirBinds() []string {
	var pluginBinds []string
	for _, pluginDir := range pluginDirs {
		pluginBinds = append(pluginBinds, pluginDir+":"+pluginDir+readOnly)
	}
	return pluginBinds
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
