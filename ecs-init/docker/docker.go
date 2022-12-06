// Copyright 2015-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-init/backoff"
	"github.com/aws/amazon-ecs-agent/ecs-init/config"
	"github.com/aws/amazon-ecs-agent/ecs-init/gpu"

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
	defaultDockerEndpoint   = "/var/run"
	defaultDockerSocketPath = "/var/run/docker.sock"
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
	// iptablesExecutableHostDir specifies the location of the iptable
	// executable on the host
	iptablesExecutableHostDir = "/sbin"
	// iptablesExecutableHostDir specifies the location of the iptable
	// executable inside container.
	iptablesExecutableContainerDir = "/host/sbin"
	// iptablesAltDir specifies the location of iptables alternatives
	iptablesAltDir = "/etc/alternatives"
	// legacyDir holds the location of legacy iptables
	iptablesLegacyDir = "/usr/sbin"
	// externalEnvCredsHostDir specifies the location of the credentials on host when running in external environment.
	externalEnvCredsHostDir = "/root/.aws"
	// externalEnvCredsContainerDir specifies the location of the credentials that will be mounted in agent container.
	externalEnvCredsContainerDir = "/rotatingcreds"

	// the following libDirs  specify the location of shared libraries on the
	// host and in the Agent container required for the execution of the iptables
	// executable. Some OS like AL2 moved lib64 to /usr/lib64 (and lib to /usr/lib)
	iptablesLibDir      = "/lib"
	iptablesUsrLibDir   = "/usr/lib"
	iptablesLib64Dir    = "/lib64"
	iptablesUsrLib64Dir = "/usr/lib64"

	hostResourcesRootDir      = "/var/lib/ecs/deps"
	containerResourcesRootDir = "/managed-agents"

	execCapabilityName     = "execute-command"
	execConfigRelativePath = "config"

	execAgentLogRelativePath = "/exec"
)

// Do NOT include "CAP_" in capability string
const (
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
)

var pluginDirs = []string{
	pluginSocketFilesDir,
	pluginSpecFilesEtcDir,
	pluginSpecFilesUsrDir,
}

var (
	dockerOnce      sync.Once
	dockerClient    *client
	dockerClientErr error
	isPathValid     = defaultIsPathValid
	execCommand     = exec.Command
	execLookPath    = exec.LookPath
)

// client enables business logic for running the Agent inside Docker
type client struct {
	docker dockerclient
	fs     fileSystem
}

// Client returns the global docker client.
func Client() (*client, error) {
	dockerOnce.Do(func() {
		// Create a backoff for pinging the docker socket. This should result in 17-19
		// seconds of delay in the worst-case between different actions that depend on
		// docker
		pingBackoff := backoff.NewBackoff(minBackoffDuration, maxBackoffDuration, backoffJitterMultiple,
			backoffMultiple, maxRetries)
		cl, err := newDockerClient(godockerClientFactory{}, pingBackoff)
		if err != nil {
			dockerClientErr = err
			return
		}
		dockerClient = &client{
			docker: cl,
			fs:     standardFS,
		}
	})
	return dockerClient, dockerClientErr
}

// IsAgentImageLoaded returns true if the Agent image is loaded in Docker
func (c *client) IsAgentImageLoaded() (bool, error) {
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
func (c *client) LoadImage(image io.Reader) error {
	return c.docker.LoadImage(godocker.LoadImageOptions{InputStream: image})
}

// RemoveExistingAgentContainer remvoes any existing container named
// "ecs-agent" or returns without error if none is found
func (c *client) RemoveExistingAgentContainer() error {
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

func (c *client) findAgentContainer() (string, error) {
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
func (c *client) StartAgent() (int, error) {
	envVarsFromFiles := c.LoadEnvVars()

	hostConfig := c.getHostConfig(envVarsFromFiles)

	container, err := c.docker.CreateContainer(godocker.CreateContainerOptions{
		Name:       config.AgentContainerName,
		Config:     c.getContainerConfig(envVarsFromFiles),
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

// GetContainerLogTail will return the last logWindowSize lines of logs for
// the Agent Container.
func (c *client) GetContainerLogTail(logWindowSize string) string {
	containerToLog, _ := c.findAgentContainer()
	if containerToLog == "" {
		log.Info("No existing container to take logs from.")
		return ""
	}
	// we want to capture some logs from our removed containers in case of failure
	var containerLogBuf bytes.Buffer
	err := c.docker.Logs(godocker.LogsOptions{
		Container:    containerToLog,
		OutputStream: &containerLogBuf,
		Stdout:       true,
		Stderr:       true,
		Tail:         logWindowSize,
		Timestamps:   true,
	})
	// we're ok if grabbing the container's logs fails
	if err != nil {
		log.Infof("Unable to tail logs for container ID: %s", containerToLog)
	}
	return containerLogBuf.String()
}

func (c *client) getContainerConfig(envVarsFromFiles map[string]string) *godocker.Config {
	// default environment variables
	envVariables := map[string]string{
		"ECS_LOGFILE":                           logDir + "/" + config.AgentLogFile,
		"ECS_DATADIR":                           dataDir,
		"ECS_AGENT_CONFIG_FILE_PATH":            config.AgentJSONConfigFile(),
		"ECS_UPDATE_DOWNLOAD_DIR":               config.CacheDirectory(),
		"ECS_UPDATES_ENABLED":                   "true",
		"ECS_AVAILABLE_LOGGING_DRIVERS":         `["json-file","syslog","awslogs","fluentd","none"]`,
		"ECS_ENABLE_TASK_IAM_ROLE":              "true",
		"ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST": "true",
		"ECS_AGENT_LABELS":                      "",
		"ECS_VOLUME_PLUGIN_CAPABILITIES":        `["efsAuth"]`,
	}

	// for al, al2 add host ssl cert directory envvar if available
	if certDir := config.HostCertsDirPath(); certDir != "" {
		envVariables["SSL_CERT_DIR"] = certDir
	}

	// env variable is only available if the gmsa is enabled on linux
	credentialsFetcherHost, ok := config.HostCredentialsFetcherPath()
	if ok && credentialsFetcherHost != "" {
		envVariables["CREDENTIALS_FETCHER_HOST_DIR"] = credentialsFetcherHost
	}

	// merge in platform-specific environment variables
	for envKey, envValue := range getPlatformSpecificEnvVariables() {
		envVariables[envKey] = envValue
	}

	envVar := os.Getenv(config.ECSGMSASupportEnvVar)
	// check for the domain join only if ECS_GMSA_SUPPORTED environment variable is set to true
	if envVar == "true" {
		if isDomainJoined() {
			// set the environment variable to true if the container instance is domain joined
			envVariables["ECS_DOMAIN_JOINED_LINUX_INSTANCE"] = "true"
		}
	}

	for key, val := range envVarsFromFiles {
		envVariables[key] = val
	}
	if config.RunningInExternal() {
		// Task networking is not supported when not running on EC2. Explicitly disable since it's enabled by default.
		envVariables["ECS_ENABLE_TASK_ENI"] = "false"
	}

	var env []string
	for envKey, envValue := range envVariables {
		env = append(env, envKey+"="+envValue)
	}
	cfg := &godocker.Config{
		Env:   env,
		Image: config.AgentImageName,
	}
	setLabels(cfg, envVariables["ECS_AGENT_LABELS"])
	return cfg
}

func setLabels(cfg *godocker.Config, labelsStringRaw string) {
	// Is there labels to add?
	if len(labelsStringRaw) > 0 {
		labels, err := generateLabelMap(labelsStringRaw)
		if err != nil {
			// Are the labels valid?
			log.Errorf("Failed to decode the container labels, skipping labels. Error: %s", err)
			return
		}
		// Stops `{}` from being valid
		if len(labels) > 0 {
			cfg.Labels = labels
		}
	}
}

func (c *client) LoadEnvVars() map[string]string {
	envVariables := make(map[string]string)
	// merge in instance-specific environment variables
	for envKey, envValue := range c.loadCustomInstanceEnvVars() {
		if envKey == config.GPUSupportEnvVar && envValue == "true" {
			if !nvidiaGPUDevicesPresent() {
				continue
			}
		}
		envVariables[envKey] = envValue
	}

	// merge in user-supplied environment variables
	for envKey, envValue := range c.loadUsrEnvVars() {
		if val, ok := envVariables[envKey]; ok {
			log.Debugf("Overriding instance config %s of value %s to %s", envKey, val, envValue)
		}
		envVariables[envKey] = envValue
	}
	return envVariables
}

// loadUsrEnvVars gets user-supplied environment variables
func (c *client) loadUsrEnvVars() map[string]string {
	return c.getEnvVars(config.AgentConfigFile())
}

// loadCustomInstanceEnvVars gets custom config set in the instance by Amazon
func (c *client) loadCustomInstanceEnvVars() map[string]string {
	return c.getEnvVars(config.InstanceConfigFile())
}

func (c *client) getEnvVars(filename string) map[string]string {
	envVariables := make(map[string]string)

	file, err := c.fs.ReadFile(filename)
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

func generateLabelMap(jsonBlock string) (map[string]string, error) {
	out := map[string]string{}
	err := json.Unmarshal([]byte(jsonBlock), &out)
	return out, err
}

func (c *client) getHostConfig(envVarsFromFiles map[string]string) *godocker.HostConfig {
	dockerSocketBind := getDockerSocketBind(envVarsFromFiles)

	binds := []string{
		dockerSocketBind,
		config.LogDirectory() + ":" + logDir,
		config.AgentDataDirectory() + ":" + dataDir,
		config.AgentConfigDirectory() + ":" + config.AgentConfigDirectory(),
		config.CacheDirectory() + ":" + config.CacheDirectory(),
		config.CgroupMountpoint() + ":" + DefaultCgroupMountpoint,
		// bind mount instance config dir
		config.InstanceConfigDirectory() + ":" + config.InstanceConfigDirectory(),
		filepath.Join(config.LogDirectory(), execAgentLogRelativePath) + ":" + filepath.Join(logDir, execAgentLogRelativePath),
	}

	// for al, al2 add host ssl cert directory mounts
	if pkiDir := config.HostPKIDirPath(); pkiDir != "" {
		certsPath := pkiDir + ":" + pkiDir + readOnly
		binds = append(binds, certsPath)
	}

	if config.RunningInExternal() {
		credsPath := externalEnvCredsHostDir + ":" + externalEnvCredsContainerDir + readOnly
		binds = append(binds, credsPath)
	}

	for key, val := range c.LoadEnvVars() {
		if key == config.GPUSupportEnvVar && val == "true" {
			if nvidiaGPUDevicesPresent() {
				// bind mount gpu info dir
				binds = append(binds, gpu.GPUInfoDirPath+":"+gpu.GPUInfoDirPath)
			}
		}

		if key == config.ECSGMSASupportEnvVar && val == "true" {
			// only bind if the env variable gmsa support is set to true and the path to bind exists
			credentialsfetcherSocketBind, volumeExists := getCredentialsFetcherSocketBind()
			if volumeExists {
				binds = append(binds, credentialsfetcherSocketBind)
			}
		}
	}

	binds = append(binds, getDockerPluginDirBinds()...)

	// only add bind mounts when the src file/directory exists on host; otherwise docker API create an empty directory on host
	binds = append(binds, getCapabilityBinds()...)

	return createHostConfig(binds)
}

// getCredentialsFetcherSocketBind returns the corresponding bind for credentials fetcher socket.
func getCredentialsFetcherSocketBind() (string, bool) {
	credentialsFetcherUnixSocketHostPath, ok := config.HostCredentialsFetcherPath()
	if ok && credentialsFetcherUnixSocketHostPath != "" {
		// check whether the path to the credentials fetcher socket exists
		_, err := os.Stat(credentialsFetcherUnixSocketHostPath)
		if err != nil {
			if os.IsNotExist(err) {
				return "", false
			}
		}

		return credentialsFetcherUnixSocketHostPath + ":" + credentialsFetcherUnixSocketHostPath, true
	}
	return "", false
}

// getDockerSocketBind returns the bind for Docker socket.
// Value for the bind is as follow:
//  1. DOCKER_HOST (as in os.Getenv) not set: source /var/run, dest /var/run
//  2. DOCKER_HOST (as in os.Getenv) set: source DOCKER_HOST (as in os.Getenv, trim unix:// prefix),
//     dest DOCKER_HOST (as in /etc/ecs/ecs.config, trim unix:// prefix)
//
// On AL2, the value from os.Getenv is the same as the one from /etc/ecs/ecs.config, but on AL1 they might be different, which
// is why I distinguish the two.
func getDockerSocketBind(envVarsFromFiles map[string]string) string {
	dockerEndpointAgent := defaultDockerEndpoint
	dockerUnixSocketSourcePath, fromEnv := config.DockerUnixSocket()
	if fromEnv {
		if dockerEndpointFromConfig, ok := envVarsFromFiles[config.DockerHostEnvVar]; ok && strings.HasPrefix(dockerEndpointFromConfig, config.UnixSocketPrefix) {
			dockerEndpointAgent = strings.TrimPrefix(dockerEndpointFromConfig, config.UnixSocketPrefix)
		} else {
			dockerEndpointAgent = defaultDockerSocketPath
		}
	}

	return dockerUnixSocketSourcePath + ":" + dockerEndpointAgent
}

// getDockerPluginDirBinds returns the binds for Docker plugin directories.
func getDockerPluginDirBinds() []string {
	var pluginBinds []string
	for _, pluginDir := range pluginDirs {
		pluginBinds = append(pluginBinds, pluginDir+":"+pluginDir+readOnly)
	}
	return pluginBinds
}

func getCapabilityBinds() []string {
	var binds = []string{}

	// bind mount the entire /host/dependency/path/ folder
	// as readonly to support all managed dependencies
	if isPathValid(hostResourcesRootDir, true) {
		binds = append(binds,
			hostResourcesRootDir+":"+containerResourcesRootDir+readOnly)
	}

	// bind mount the entire /host/dependency/path/execute-command/config folder
	// in read-write mode to allow ecs-agent to write config files to host file system
	// (docker will) create the config folder if it does not exist
	hostConfigDir := filepath.Join(hostResourcesRootDir, execCapabilityName, execConfigRelativePath)
	// Check that execute-command folder is present not config folder
	if isPathValid(filepath.Dir(hostConfigDir), true) {
		binds = append(binds,
			hostConfigDir+":"+filepath.Join(containerResourcesRootDir, execCapabilityName, execConfigRelativePath))
	}

	return binds
}

func defaultIsPathValid(path string, shouldBeDirectory bool) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}

	isDirectory := fileInfo.IsDir()
	return (isDirectory && shouldBeDirectory) || (!isDirectory && !shouldBeDirectory)
}

// nvidiaGPUDevicesPresent checks if nvidia GPU devices are present in the instance
func nvidiaGPUDevicesPresent() bool {
	matches, err := MatchFilePatternForGPU(gpu.NvidiaGPUDeviceFilePattern)
	if err != nil {
		log.Errorf("Detecting Nvidia GPU devices failed")
		return false
	}
	if matches == nil {
		return false
	}
	return true
}

var MatchFilePatternForGPU = FilePatternMatchForGPU

func FilePatternMatchForGPU(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// StopAgent stops the Agent in docker if one is running
func (c *client) StopAgent() error {
	id, err := c.findAgentContainer()
	if err != nil {
		return err
	}
	if id == "" {
		log.Info("No running Agent to stop")
		return nil
	}
	stopContainerTimeoutSeconds := uint(10)
	err = c.docker.StopContainer(id, stopContainerTimeoutSeconds)
	if _, ok := err.(*godocker.ContainerNotRunning); ok {
		log.Info("Agent is already stopped")
		return nil
	}
	return err
}

// isDomainJoined is used to validate if container instance is part of a valid active directory.
func isDomainJoined() bool {
	realmPath, err := execLookPath("realm")
	if err != nil {
		log.Error("realmd is not installed on the instance")
		return false
	}

	if len(realmPath) == 0 {
		log.Error("could not path of realm on the instance")
		return false
	}

	realmCommmand := realmPath + " list"
	realmCmdOutput, err := execCommand("bash", "-c", realmCommmand).Output()
	if err != nil {
		log.Error("failed to read realm info")
		return false
	}

	realmOutputStr := string(realmCmdOutput)
	if !strings.Contains(realmOutputStr, "realm-name") {
		log.Error("couldn't not find realm name")
		return false
	}

	return true
}
