//go:build linux && unit
// +build linux,unit

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
	"path/filepath"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"
	"github.com/aws/aws-sdk-go/aws"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

const (
	ipv4        = "10.0.0.1"
	gatewayIPv4 = "10.0.0.2/20"
	mac         = "1.2.3.4"
	ipv6        = "f0:234:23"
)

var (
	cfg     config.Config
	mockENI = &apieni.ENI{
		ID: "eni-id",
		IPV4Addresses: []*apieni.ENIIPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		},
		MacAddress: mac,
		IPV6Addresses: []*apieni.ENIIPV6Address{
			{
				Address: ipv6,
			},
		},
		SubnetGatewayIPV4Address: gatewayIPv4,
	}
)

func TestDNSConfigToDockerExtraHostsFormat(t *testing.T) {
	tt := []struct {
		dnsConfigs      []task.DNSConfigEntry
		expectedRestult []string
	}{
		{
			dnsConfigs: []task.DNSConfigEntry{
				{
					HostName: "my.test.host",
					Address:  "169.254.1.1",
				},
				{
					HostName: "my.test.host2",
					Address:  "ff06::c3",
				},
			},
			expectedRestult: []string{
				"my.test.host:169.254.1.1",
				"my.test.host2:ff06::c3",
			},
		},
		{
			dnsConfigs:      nil,
			expectedRestult: nil,
		},
	}

	for _, tc := range tt {
		res := DNSConfigToDockerExtraHostsFormat(tc.dnsConfigs)
		assert.Equal(t, tc.expectedRestult, res, "Wrong docker host config ")
	}
}

func getAWSVPCTask(t *testing.T) (*apitask.Task, *apicontainer.Container, *apicontainer.Container) {
	sleepTask := testdata.LoadTask("sleep5TwoContainers")

	sleepTask.ServiceConnectConfig = &apitask.ServiceConnectConfig{
		ContainerName: "service-connect",
		DNSConfig: []apitask.DNSConfigEntry{
			{
				HostName: "host1.my.corp",
				Address:  "169.254.1.1",
			},
			{
				HostName: "host1.my.corp",
				Address:  "ff06::c4",
			},
		},
	}
	dockerConfig := dockercontainer.Config{
		Healthcheck: &dockercontainer.HealthConfig{
			Test:     []string{"echo", "ok"},
			Interval: time.Millisecond,
			Timeout:  time.Second,
			Retries:  1,
		},
	}

	pauseContainer := apicontainer.NewContainerWithSteadyState(apicontainerstatus.ContainerResourcesProvisioned)
	pauseContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	pauseContainer.Name = task.NetworkPauseContainerName
	pauseContainer.Image = fmt.Sprintf("%s:%s", cfg.PauseContainerImageName, cfg.PauseContainerTag)
	pauseContainer.Essential = true
	pauseContainer.Type = apicontainer.ContainerCNIPause

	rawConfig, err := json.Marshal(&dockerConfig)
	if err != nil {
		t.Fatal(err)
	}
	serviceConnectContainer := &apicontainer.Container{
		Name:            sleepTask.ServiceConnectConfig.ContainerName,
		HealthCheckType: apicontainer.DockerHealthCheckType,
		DockerConfig: apicontainer.DockerConfig{
			Config: aws.String(string(rawConfig)),
		},
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}
	sleepTask.Containers = append(sleepTask.Containers, serviceConnectContainer)

	// Add eni information to the task so the task can add dependency of pause container
	sleepTask.AddTaskENI(mockENI)
	return sleepTask, pauseContainer, serviceConnectContainer
}

func TestPauseContainerModificationsForServiceConnect(t *testing.T) {
	scTask, pauseContainer, _ := getAWSVPCTask(t)

	expectedPauseExtraHosts := []string{
		"host1.my.corp:169.254.1.1",
		"host1.my.corp:ff06::c4",
	}

	type testCase struct {
		name               string
		container          *apicontainer.Container
		expectedExtraHosts []string
	}
	testcases := []testCase{
		{
			name:               "Pause container has extra hosts",
			container:          pauseContainer,
			expectedExtraHosts: expectedPauseExtraHosts,
		},
	}
	// Add test cases for other containers expecting no modifications
	for _, container := range scTask.Containers {
		if container != pauseContainer {
			testcases = append(testcases, testCase{name: container.Name, container: container})
		}
	}
	scManager := NewManager()

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			hostConfig := &dockercontainer.HostConfig{}
			err := scManager.AugmentTaskContainer(scTask, tc.container, hostConfig)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.expectedExtraHosts, hostConfig.ExtraHosts)
		})
	}
}

func TestAgentContainerModificationsForServiceConnect(t *testing.T) {
	scTask, _, serviceConnectContainer := getAWSVPCTask(t)

	tempDir := t.TempDir()
	expectedBinds := []string{
		fmt.Sprintf("%s/status/%s:%s", tempDir, scTask.GetID(), "/some/other/run"),
		fmt.Sprintf("%s:%s", tempDir, "/not/var/run"),
	}
	expectedENVs := map[string]string{
		"ReLaYgOeShErE":           "/not/var/run/relay_file_of_holiness",
		"StAtUsGoEsHeRe":          "/some/other/run/status_file_of_holiness",
		"APPNET_AGENT_ADMIN_MODE": "uds",
	}

	type testCase struct {
		name          string
		container     *apicontainer.Container
		expectedENV   map[string]string
		expectedBinds []string
	}
	testcases := []testCase{
		{
			name:          "Service connect container has extra binds/ENV",
			container:     serviceConnectContainer,
			expectedENV:   expectedENVs,
			expectedBinds: expectedBinds,
		},
	}
	// Add test cases for other containers expecting no modifications
	for _, container := range scTask.Containers {
		if container != serviceConnectContainer {
			testcases = append(testcases, testCase{name: container.Name, container: container, expectedENV: map[string]string{}})
		}
	}
	scManager := &manager{
		relayPathContainer:  "/not/var/run",
		relayPathHost:       tempDir,
		relayFileName:       "relay_file_of_holiness",
		relayENV:            "ReLaYgOeShErE",
		statusPathContainer: "/some/other/run",
		statusPathHostRoot:  filepath.Join(tempDir, "status"),
		statusFileName:      "status_file_of_holiness",
		statusENV:           "StAtUsGoEsHeRe",
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			hostConfig := &dockercontainer.HostConfig{}
			err := scManager.AugmentTaskContainer(scTask, tc.container, hostConfig)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.expectedBinds, hostConfig.Binds)
			assert.Equal(t, tc.expectedENV, tc.container.Environment)

		})
	}
	assert.Equal(t, scTask.ServiceConnectConfig.RuntimeConfig.AdminSocketPath, fmt.Sprintf("%s/status/%s/%s", tempDir, scTask.GetID(), "status_file_of_holiness"))

	config := scTask.GetServiceConnectRuntimeConfig()
	assert.Equal(t, config.AdminSocketPath, fmt.Sprintf("%s/status/%s/%s", tempDir, scTask.GetID(), "status_file_of_holiness"))
}
