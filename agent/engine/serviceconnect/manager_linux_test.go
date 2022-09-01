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
	"io/fs"
	"testing"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/engine/testdata"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

func TestDNSConfigToDockerExtraHostsFormat(t *testing.T) {
	tt := []struct {
		dnsConfigs      []serviceconnect.DNSConfigEntry
		expectedRestult []string
	}{
		{
			dnsConfigs: []serviceconnect.DNSConfigEntry{
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

func TestGetSupportedIPFamilies(t *testing.T) {
	tt := []struct {
		task            *apitask.Task
		expectedRestult string
	}{
		{
			task:            getTask(apitask.AWSVPCNetworkMode, true, true),
			expectedRestult: supportDualStack,
		},
		{
			task:            getTask(apitask.AWSVPCNetworkMode, true, false),
			expectedRestult: supportIPv4Only,
		},
		{
			task:            getTask(apitask.AWSVPCNetworkMode, false, true),
			expectedRestult: supportIPv6Only,
		},
		{
			task:            getTask(apitask.BridgeNetworkMode, false, false),
			expectedRestult: supportIPv4Only,
		},
	}

	for _, tc := range tt {
		res := getSupportedIPFamilies(tc.task)
		assert.Equal(t, tc.expectedRestult, res, "Wrong supported IP families ")
	}
}

func TestPauseContainerModificationsForServiceConnect(t *testing.T) {
	scTask, pauseContainer, serviceConnectContainer := getAWSVPCTask(t)

	expectedPauseExtraHosts := []string{
		"host1.my.corp:169.254.1.1",
		"host1.my.corp:ff06::c4",
	}

	type testCase struct {
		name               string
		container          *apicontainer.Container
		expectedExtraHosts []string
		needsImage         bool
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
			testcases = append(testcases, testCase{name: container.Name, container: container, needsImage: container == serviceConnectContainer})
		}
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			origMkdir := mkdirAllAndChown
			if tc.needsImage {
				mkdirAllAndChown = func(path string, perm fs.FileMode, uid, gid int) error {
					return nil
				}
			}

			hostConfig := &dockercontainer.HostConfig{}
			scManager := &manager{
				AgentContainerImageName: "container_image",
				AgentContainerTag:       "tag",
			}
			err := scManager.AugmentTaskContainer(scTask, tc.container, hostConfig)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.expectedExtraHosts, hostConfig.ExtraHosts)
			mkdirAllAndChown = origMkdir
		})
	}
}

func TestAgentContainerModificationsForServiceConnect_NonPrivileged(t *testing.T) {
	testAgentContainerModificationsForServiceConnect(t, false)
}

func getTask(networkMode string, enableIPv4, enableIPv6 bool) *apitask.Task {
	task := testdata.LoadTask("sleep5TwoContainers")
	task.NetworkMode = networkMode
	if task.IsNetworkModeBridge() {
		return task
	}

	task.AddTaskENI(
		&apieni.ENI{
			ID:                       "eni-id",
			MacAddress:               mac,
			SubnetGatewayIPV4Address: gatewayIPv4,
		})

	if enableIPv4 {
		task.ENIs[0].IPV4Addresses = []*apieni.ENIIPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		}
	}

	if enableIPv6 {
		task.ENIs[0].IPV6Addresses = []*apieni.ENIIPV6Address{
			{
				Address: ipv6,
			},
		}
	}

	return task
}
