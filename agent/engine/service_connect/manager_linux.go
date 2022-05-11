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
	"fmt"
	"os"
	"path/filepath"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	defaultRelayPathContainer  = "/var/run/ecs/relay/"
	defaultRelayPathHost       = "/var/run/ecs/instance/relay/"
	defaultRelayFileName       = "envoy_xds.sock"
	defaultRelayENV            = "APPMESH_XDS_ENDPOINT"
	defaultStatusPathContainer = "/var/run/ecs/"
	defaultStatusPathHostRoot  = "/var/run/ecs/"
	defaultStatusFileName      = "appnet_admin.sock"
	defaultStatusENV           = "APPNET_AGENT_UDS_PATH"
)

type manager struct {
	relayPathContainer  string
	relayPathHost       string
	relayFileName       string
	relayENV            string
	statusPathContainer string
	statusPathHostRoot  string
	statusFileName      string
	statusENV           string
}

func NewManager() Manager {
	return &manager{
		relayPathContainer:  defaultRelayPathContainer,
		relayPathHost:       defaultRelayPathHost,
		relayFileName:       defaultRelayFileName,
		relayENV:            defaultRelayENV,
		statusPathContainer: defaultStatusPathContainer,
		statusPathHostRoot:  defaultStatusPathHostRoot,
		statusFileName:      defaultStatusFileName,
		statusENV:           defaultStatusENV,
	}
}

func IsServiceConnectEnabledTask(task *apitask.Task) bool {
	return task.IsServiceConnectEnabled()
}

func (m *manager) initializeAgentContainer(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error {
	return m.initServiceConnectDirectoryMounts(task.GetID(), container, hostConfig)
}

func getBindMountMapping(hostDir, containerDir string) string {
	return hostDir + ":" + containerDir
}

func (m *manager) initServiceConnectDirectoryMounts(taskId string, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error {
	instancePath := filepath.Join(m.statusPathHostRoot, taskId)
	if _, err := os.Stat(instancePath); os.IsNotExist(err) {
		os.MkdirAll(instancePath, 0770)
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(m.relayPathHost); os.IsNotExist(err) {
		os.MkdirAll(m.relayPathHost, 0770)
	} else if err != nil {
		return err
	}

	hostConfig.Binds = append(hostConfig.Binds, getBindMountMapping(instancePath, m.statusPathContainer))
	hostConfig.Binds = append(hostConfig.Binds, getBindMountMapping(m.relayPathHost, m.relayPathContainer))

	scEnv := map[string]string{
		m.relayENV:  filepath.Join(m.relayPathContainer, m.relayFileName),
		m.statusENV: filepath.Join(m.statusPathContainer, m.statusFileName),
	}

	container.MergeEnvironmentVariables(scEnv)
	return nil
}

// DNSConfigToDockerExtraHostsFormat converts a []DNSConfigEntry slice to a list of ExtraHost entries that Docker will
// recognize.
func DNSConfigToDockerExtraHostsFormat(dnsConfigs []apitask.DNSConfigEntry) []string {
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
		m.initializeAgentContainer(task, container, hostConfig)
	}
	return err
}
