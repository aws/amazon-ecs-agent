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

package serviceconnect

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/pborman/uuid"

	"github.com/aws/aws-sdk-go/aws"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apiserviceconnect "github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/utils/loader"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	defaultRelayPathContainer  = "/var/run/ecs/relay/"
	defaultRelayPathHost       = "/var/run/ecs/service_connect/relay/"
	defaultRelayFileName       = "envoy_xds.sock"
	defaultEndpointENV         = "APPMESH_XDS_ENDPOINT"
	defaultStatusPathContainer = "/var/run/ecs/"
	// Expected to have task.GetID() appended to form actual host path
	defaultStatusPathHostRoot = "/var/run/ecs/service_connect/"
	defaultStatusFileName     = "appnet_admin.sock"
	defaultStatusENV          = "APPNET_AGENT_ADMIN_UDS_PATH"

	// logging
	defaultLogPathHostRoot              = "/var/log/ecs/service_connect/"
	defaultLogPathContainer             = "/var/log/"
	defaultECSAgentLogPathForSC         = "/%s/service_connect/" // %s will be substituted with ECS Agent container log path
	defaultAppnetEnvoyLogDestinationENV = "APPNET_ENVOY_LOG_DESTINATION"

	relayEnableENV = "APPNET_ENABLE_RELAY_MODE_FOR_XDS"
	relayEnableOn  = "1"
	upstreamENV    = "APPNET_RELAY_LISTENER_UDS_PATH"
	regionENV      = "AWS_REGION"
	agentAuthENV   = "ENVOY_ENABLE_IAM_AUTH_FOR_XDS"
	agentAuthOff   = "0"
	agentModeENV   = "APPNET_AGENT_ADMIN_MODE"
	agentModeValue = "uds"
	envoyModeENV   = "ENVOY_ADMIN_MODE"
	envoyModeValue = "uds"

	containerInstanceArnENV = "ECS_CONTAINER_INSTANCE_ARN"

	unixRequestPrefix        = "unix://"
	httpRequestPrefix        = "http://localhost"
	defaultAdminStatsRequest = httpRequestPrefix + "/stats/prometheus?usedonly&filter=metrics_extension&delta"
	defaultAdminDrainRequest = httpRequestPrefix + "/drain_listeners?inboundonly"

	defaultAgentContainerImageName         = "ecs-service-connect-agent"
	defaultAgentContainerTagFormat         = "interface-%s"
	defaultAgentContainerTarballPathFormat = "/managed-agents/serviceconnect/ecs-service-connect-agent.interface-%s.tar"

	ecsAgentLogFileENV              = "ECS_LOGFILE"
	defaultECSAgentLogPathContainer = "/log"
)

type manager struct {
	// Path to where relayFileName exists which Envoy in the container will connect to
	relayPathContainer string
	// Path to where relayFileName exists on Host
	relayPathHost string
	// Filename without Path which Relay will create and Envoy in the container will connect to
	relayFileName string
	// Environment variable to set on Container with contents of relayPathContainer/relayFileName
	endpointENV string
	// Path to where statusFileName exists which Envoy in the container will create for status endpoint
	statusPathContainer string
	// PathRoot to be appended with TaskID statusPathHostRoot/task.GetID() where statusFileName exists on Host
	statusPathHostRoot string
	// Filename without Path which Envoy in container will create for status endpoint
	statusFileName string
	// Environment variable to set on Container with contents of statusPathContainer/statusFileName
	statusENV string
	// Path to where AppNet log file will be written to inside container
	logPathContainer string
	// Path to where AppNet log file will be written to on host
	logPathHostRoot string
	// Path to create logging dir for AppNet, from ECS Agent point of view (b/c "/log" for ECS Agent is "/var/log/ecs" on host)
	logPathECSAgentRoot string

	// Http path + params to make a statistics request of AppNetAgent
	adminStatsRequest string
	// Http path + params to make a drain request of AppNetAgent
	adminDrainRequest string

	agentContainerImageName string
	agentContainerTag       string
	appnetInterfaceVersion  string

	ecsClient            ecs.ECSClient
	containerInstanceARN string
}

func NewManager() Manager {
	return &manager{
		relayPathContainer:  defaultRelayPathContainer,
		relayPathHost:       defaultRelayPathHost,
		relayFileName:       defaultRelayFileName,
		endpointENV:         defaultEndpointENV,
		statusPathContainer: defaultStatusPathContainer,
		statusPathHostRoot:  defaultStatusPathHostRoot,
		statusFileName:      defaultStatusFileName,
		statusENV:           defaultStatusENV,
		adminStatsRequest:   defaultAdminStatsRequest,
		adminDrainRequest:   defaultAdminDrainRequest,
		logPathContainer:    defaultLogPathContainer,
		logPathHostRoot:     defaultLogPathHostRoot,
		logPathECSAgentRoot: fmt.Sprintf(defaultECSAgentLogPathForSC, getECSAgentLogPathContainer()),

		agentContainerImageName: defaultAgentContainerImageName,
	}
}

func (m *manager) SetECSClient(client ecs.ECSClient, containerInstanceARN string) {
	m.ecsClient = client
	m.containerInstanceARN = containerInstanceARN
}

func (m *manager) augmentAgentContainer(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error {
	if task.IsNetworkModeBridge() {
		err := m.initServiceConnectContainerMapping(task, container, hostConfig)
		if err != nil {
			return err
		}
	}
	adminPath, err := m.initAgentDirectoryMounts(task.GetID(), container, hostConfig)
	if err != nil {
		return err
	}
	m.initAgentEnvironment(container)

	// Setup runtime configuration
	var config apiserviceconnect.RuntimeConfig
	config.AdminSocketPath = adminPath
	config.StatsRequest = m.adminStatsRequest
	config.DrainRequest = m.adminDrainRequest

	task.PopulateServiceConnectRuntimeConfig(config)
	container.Image = m.GetLoadedImageName()
	return nil
}

func getBindMountMapping(hostDir, containerDir string) string {
	return hostDir + ":" + containerDir
}

var mkdirAllAndChown = defaultMkdirAllAndChown

func defaultMkdirAllAndChown(path string, perm fs.FileMode, uid, gid int) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, perm)
	}
	if err != nil {
		return fmt.Errorf("failed to mkdir %s: %+v", path, err)
	}
	// AppNet Agent container is going to run as non-root user $AppNetUID.
	// Change directory owner to $AppNetUID so that it has full permission (to create socket file and bind to it etc.)
	if err = os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("failed to chown %s: %+v", path, err)
	}
	return nil
}

func (m *manager) initAgentDirectoryMounts(taskId string, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) (string, error) {
	statusPathHost := filepath.Join(m.statusPathHostRoot, taskId)

	// Create host directories if they don't exist
	for _, path := range []string{statusPathHost, m.relayPathHost} {
		err := mkdirAllAndChown(path, 0700, apiserviceconnect.AppNetUID, os.Getegid())
		if err != nil {
			return "", err
		}
	}

	hostConfig.Binds = append(hostConfig.Binds, getBindMountMapping(statusPathHost, m.statusPathContainer))
	hostConfig.Binds = append(hostConfig.Binds, getBindMountMapping(m.relayPathHost, m.relayPathContainer))

	// create logging directory and bind mount, if customer has not configured a logging driver
	if container.GetLogDriver() == "" {
		logPathHost := filepath.Join(m.logPathHostRoot, taskId)
		logPathECSAgent := filepath.Join(m.logPathECSAgentRoot, taskId)
		err := mkdirAllAndChown(logPathECSAgent, 0700, apiserviceconnect.AppNetUID, os.Getegid())
		if err != nil {
			return "", err
		}
		hostConfig.Binds = append(hostConfig.Binds, getBindMountMapping(logPathHost, m.logPathContainer))
	}

	return filepath.Join(statusPathHost, m.statusFileName), nil
}

func (m *manager) initAgentEnvironment(container *apicontainer.Container) {
	scEnv := map[string]string{
		m.endpointENV:           unixRequestPrefix + filepath.Join(m.relayPathContainer, m.relayFileName),
		m.statusENV:             filepath.Join(m.statusPathContainer, m.statusFileName),
		agentModeENV:            agentModeValue,
		agentAuthENV:            agentAuthOff,
		containerInstanceArnENV: m.containerInstanceARN,
	}
	if container.GetLogDriver() == "" {
		scEnv[defaultAppnetEnvoyLogDestinationENV] = m.logPathContainer
	}

	container.MergeEnvironmentVariables(scEnv)
}

func (m *manager) initRelayEnvironment(config *config.Config, container *apicontainer.Container) {
	endpoint := fmt.Sprintf("https://ecs-sc.%s.api.aws", config.AWSRegion)
	if m.ecsClient != nil {
		discoveredEndpoint, err := m.ecsClient.DiscoverServiceConnectEndpoint(m.containerInstanceARN)
		if err != nil {
			logger.Error("Failed to retrieve service connect endpoint from DiscoverPollEndpoint, failing back to default", logger.Fields{
				field.Error:        err,
				field.ManagedAgent: "service-connect",
				"endpoint":         endpoint,
			})
		} else {
			endpoint = discoveredEndpoint
		}
	}
	scEnv := map[string]string{
		m.statusENV:                         filepath.Join(m.statusPathContainer, m.statusFileName),
		upstreamENV:                         filepath.Join(m.relayPathContainer, m.relayFileName),
		regionENV:                           config.AWSRegion,
		envoyModeENV:                        envoyModeValue,
		agentModeENV:                        agentModeValue,
		relayEnableENV:                      relayEnableOn,
		m.endpointENV:                       endpoint,
		defaultAppnetEnvoyLogDestinationENV: m.logPathContainer,
	}

	container.MergeEnvironmentVariables(scEnv)
}

func (m *manager) initServiceConnectContainerMapping(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error {
	// TODO [SC] - Move the function here
	return task.PopulateServiceConnectContainerMappingEnvVar()
}

// DNSConfigToDockerExtraHostsFormat converts a []DNSConfigEntry slice to a list of ExtraHost entries that Docker will
// recognize.
func DNSConfigToDockerExtraHostsFormat(dnsConfigs []apiserviceconnect.DNSConfigEntry) []string {
	var hosts []string
	for _, dnsConf := range dnsConfigs {
		if len(dnsConf.Address) > 0 {
			hosts = append(hosts,
				fmt.Sprintf("%s:%s", dnsConf.HostName, dnsConf.Address))
		}
	}
	return hosts
}

func (m *manager) AugmentTaskContainer(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error {
	var err error
	// Add SC VIPs to pause container's known hosts
	if container.Type == apicontainer.ContainerCNIPause {
		hostConfig.ExtraHosts = append(hostConfig.ExtraHosts,
			DNSConfigToDockerExtraHostsFormat(task.ServiceConnectConfig.DNSConfig)...)
	}
	if container == task.GetServiceConnectContainer() {
		m.augmentAgentContainer(task, container, hostConfig)
	}
	return err
}

func (m *manager) CreateInstanceTask(cfg *config.Config) (*apitask.Task, error) {
	imageName := m.GetLoadedImageName()
	containerRunning := apicontainerstatus.ContainerRunning
	dockerHostConfig := dockercontainer.HostConfig{
		NetworkMode: apitask.HostNetworkMode,
		// do not restart relay if it's stopped manually.
		// the default value of 0 for MaximumRetryCount means that we will not enforce a maximum count
		RestartPolicy: dockercontainer.RestartPolicy{
			Name:              "on-failure",
			MaximumRetryCount: 0,
		},
	}
	rawHostConfig, err := json.Marshal(&dockerHostConfig)
	if err != nil {
		return nil, err
	}
	// Configure AppNet relay container health check.
	// For AppNet Agent container, the health check configuration is part of task payload,
	// however for relay we need to create it ourselves.
	healthConfig := dockercontainer.HealthConfig{
		Test:     []string{"CMD-SHELL", "/health_check.sh"},
		Interval: 5 * time.Second,
		Timeout:  2 * time.Second,
		Retries:  3,
	}
	rawHealthConfig, err := json.Marshal(&healthConfig)
	if err != nil {
		return nil, err
	}
	// The raw host config needs to be created this way - if we marshal the entire config object
	// directly, and the object only contains healthcheck, all other fields will be written as empty/nil
	// in the result string. This will override the configurations that comes with the container image
	// (CMD for example)
	rawConfig := fmt.Sprintf("{\"Healthcheck\":%s}", string(rawHealthConfig))

	// Create an internal task for AppNet Relay container
	task := &apitask.Task{
		Arn:                 fmt.Sprintf("%s-%s", "arn:::::/service-connect-relay", uuid.NewUUID()),
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{{
			Name:                      "instance-service-connect-relay",
			Image:                     imageName,
			ContainerArn:              "arn:::::/instance-service-connect-relay",
			Type:                      apicontainer.ContainerServiceConnectRelay,
			TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			Essential:                 true,
			SteadyStateStatusUnsafe:   &containerRunning,
			DockerConfig: apicontainer.DockerConfig{
				Config:     aws.String(rawConfig),
				HostConfig: aws.String(string(rawHostConfig)),
			},
			HealthCheckType: "DOCKER",
		}},
		LaunchType:         "EC2",
		NetworkMode:        apitask.HostNetworkMode,
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		IsInternal:         true,
	}
	m.initRelayEnvironment(cfg, task.Containers[0])

	return task, nil
}

func (m *manager) AugmentInstanceContainer(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error {
	adminPath, err := m.initAgentDirectoryMounts("relay", container, hostConfig)
	if err != nil {
		return err
	}

	// Setup runtime configuration
	var config apiserviceconnect.RuntimeConfig
	config.AdminSocketPath = adminPath
	config.StatsRequest = m.adminStatsRequest
	config.DrainRequest = m.adminDrainRequest

	task.PopulateServiceConnectRuntimeConfig(config)
	return nil
}

func (agent *manager) setLoadedAppnetVerion(appnetInterfaceVersion string) {
	agent.appnetInterfaceVersion = appnetInterfaceVersion
}

// LoadImage helps load the AppNetAgent container image for the agent latest supported
// AppNet interface version by looking for the AppNet agent tar name from supported list
// of AppNet versions from highest to lowest version when loading AppNet image
func (agent *manager) LoadImage(ctx context.Context, _ *config.Config, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	var loadErr error
	for _, supportedAppnetInterfaceVersion := range getSupportedAppnetInterfaceVersions() {
		agentContainerTarballPath := fmt.Sprintf(defaultAgentContainerTarballPathFormat, supportedAppnetInterfaceVersion)
		if _, err := os.Stat(agentContainerTarballPath); err != nil {
			logger.Warn(fmt.Sprintf("AppNet agent container tarball unavailable: %s", agentContainerTarballPath), logger.Fields{
				field.Error: err,
			})
			continue
		}
		logger.Debug(fmt.Sprintf("Loading Appnet agent container tarball: %s", agentContainerTarballPath))
		if loadErr = loader.LoadFromFile(ctx, agentContainerTarballPath, dockerClient); loadErr != nil {
			logger.Warn(fmt.Sprintf("Unable to load Appnet agent container tarball: %s", agentContainerTarballPath),
				logger.Fields{
					field.Error: loadErr,
				})
			continue
		}
		agent.setLoadedAppnetVerion(supportedAppnetInterfaceVersion)
		imageName := agent.GetLoadedImageName()
		logger.Info(fmt.Sprintf("Successfully loaded Appnet agent container tarball: %s", agentContainerTarballPath),
			logger.Fields{
				field.Image: imageName,
			})
		return loader.GetContainerImage(imageName, dockerClient)
	}
	return nil, loadErr
}

func (agent *manager) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return loader.IsImageLoaded(agent.GetLoadedImageName(), dockerClient)
}

func (agent *manager) GetLoadedImageName() string {
	agent.agentContainerTag = fmt.Sprintf(defaultAgentContainerTagFormat, agent.appnetInterfaceVersion)
	return fmt.Sprintf("%s:%s", agent.agentContainerImageName, agent.agentContainerTag)
}

func (agent *manager) GetLoadedAppnetVersion() (string, error) {
	return agent.appnetInterfaceVersion, nil
}

// getECSAgentLogPathContainer returns the directory path for ECS_LOGFILE env value if exists, otherwise returns "/log"
func getECSAgentLogPathContainer() string {
	ecsLogFilePath := os.Getenv(ecsAgentLogFileENV)
	if ecsLogFilePath == "" {
		return defaultECSAgentLogPathContainer
	}
	return path.Dir(ecsLogFilePath)
}

// GetCapabilitiesForAppnetInterfaceVersion returns service connect capabilities
// supported by ECS Agent to register for a selected AppNet version.
// Suppose we decide to register ecs.service-connect.v2 capability for new AppNet version (ex: 1.24.0.0),
// now if ecs.service-connect.*v1* is continuously being supported by 1.24.0.0,
// we will then register multiple capabilities.
//
//	{
//	   "v1": ["ecs.capability.service-connect-v1"],
//	   "v2": ["ecs.capability.service-connect-v2", "ecs.capability.service-connect-v2"]
//	}
func (agent *manager) GetCapabilitiesForAppnetInterfaceVersion(appnetVersion string) ([]string, error) {
	return supportedAppnetInterfaceVerToCapability[appnetVersion], nil
}

// GetAppnetContainerTarballDir returns Appnet agent tarball path's directory
func (agent *manager) GetAppnetContainerTarballDir() string {
	return filepath.Dir(defaultAgentContainerTarballPathFormat)
}
