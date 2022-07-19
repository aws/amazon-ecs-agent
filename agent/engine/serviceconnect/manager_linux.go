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
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"

	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apiserviceconnect "github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/serviceconnect"
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

	relayEnableENV = "APPNET_ENABLE_RELAY_MODE_FOR_XDS"
	relayEnableOn  = "1"
	upstreamENV    = "APPNET_RELAY_LISTENER_UDS_PATH"
	regionENV      = "AWS_REGION"
	agentAuthENV   = "ENVOY_ENABLE_IAM_AUTH_FOR_XDS"
	agentAuthOff   = "0"
	agentModeENV   = "APPNET_AGENT_ADMIN_MODE"
	agentModeValue = "uds"

	unixRequestPrefix        = "unix://"
	httpRequestPrefix        = "http://localhost"
	defaultAdminStatsRequest = httpRequestPrefix + "/stats/prometheus?usedonly&filter=metrics_extension&delta"
	defaultAdminDrainRequest = httpRequestPrefix + "/drain_listeners?inboundonly"
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

	// Http path + params to make a statistics request of AppNetAgent
	adminStatsRequest string
	// Http path + params to make a drain request of AppNetAgent
	adminDrainRequest string

	// Loader for the AppNet container
	serviceConnectLoader serviceconnect.Loader
}

func NewManager(serviceConnectLoader serviceconnect.Loader) Manager {
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

		serviceConnectLoader: serviceConnectLoader,
	}
}

func (m *manager) augmentAgentContainer(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig, serviceConnectLoader serviceconnect.Loader) error {
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
	container.Image, _ = serviceConnectLoader.GetLoadedImageName()
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
	return filepath.Join(statusPathHost, m.statusFileName), nil
}

func (m *manager) initAgentEnvironment(container *apicontainer.Container) {
	scEnv := map[string]string{
		m.endpointENV: unixRequestPrefix + filepath.Join(m.relayPathContainer, m.relayFileName),
		m.statusENV:   filepath.Join(m.statusPathContainer, m.statusFileName),
		agentModeENV:  agentModeValue,
		agentAuthENV:  agentAuthOff,
	}

	container.MergeEnvironmentVariables(scEnv)
}

func (m *manager) initRelayEnvironment(config *config.Config, container *apicontainer.Container) {
	scEnv := map[string]string{
		m.statusENV:    filepath.Join(m.statusPathContainer, m.statusFileName),
		upstreamENV:    filepath.Join(m.relayPathContainer, m.relayFileName),
		regionENV:      config.AWSRegion,
		agentModeENV:   agentModeValue,
		relayEnableENV: relayEnableOn,
		// TODO: get programmatically when available
		m.endpointENV: "https://ecs-sc-gamma.us-west-2.api.aws",
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
		m.augmentAgentContainer(task, container, hostConfig, m.serviceConnectLoader)
	}
	return err
}

func (m *manager) CreateInstanceTask(cfg *config.Config) (*apitask.Task, error) {
	imageName, err := m.serviceConnectLoader.GetLoadedImageName()
	if err != nil {
		return nil, err
	}
	containerRunning := apicontainerstatus.ContainerRunning
	dockerHostConfig := dockercontainer.HostConfig{NetworkMode: apitask.HostNetworkMode}
	rawHostConfig, err := json.Marshal(&dockerHostConfig)
	if err != nil {
		return nil, err
	}
	task := &apitask.Task{
		Arn:                 "arn:::::/service-connect-relay",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{{
			Name:                      "instance-service-connect-relay",
			Image:                     imageName,
			ContainerArn:              "arn:::::/instance-service-connect-relay",
			Type:                      apicontainer.ContainerServiceConnectRelay,
			TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			Essential:                 true,
			SteadyStateStatusUnsafe:   &containerRunning,
			DockerConfig:              apicontainer.DockerConfig{HostConfig: aws.String(string(rawHostConfig))},
		}},
		LaunchType:         "EC2",
		NetworkMode:        apitask.HostNetworkMode,
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
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
