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
	"os"
	"testing"

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
				agentContainerImageName: "container_image",
				agentContainerTag:       "tag",
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

func TestGetECSAgentLogPathContainer(t *testing.T) {
	oldVal := os.Getenv(ecsAgentLogFileENV)
	defer func() {
		if oldVal != "" {
			os.Setenv(ecsAgentLogFileENV, oldVal)
		}
	}()

	type testCase struct {
		envVal   string
		expected string
	}
	testcases := []testCase{
		{
			envVal:   "/log/ecs-agent.log",
			expected: "/log",
		},
		{
			envVal:   "/some/path/to/log/ecs-agent.log",
			expected: "/some/path/to/log",
		},
		{
			envVal:   "",
			expected: "/log",
		},
	}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			if tc.envVal == "" {
				os.Unsetenv(ecsAgentLogFileENV)
			} else {
				os.Setenv(ecsAgentLogFileENV, tc.envVal)
			}
			actualPath := getECSAgentLogPathContainer()
			assert.Equal(t, tc.expected, actualPath)

		})
	}
}

func TestGetSupportedAppnetInterfaceVerToCapabilities(t *testing.T) {
	testCases := []struct {
		name                 string
		appNetAgentVersion   string
		expectedCapabilities []string
	}{
		{
			name:                 "test supported service connect capabilities for AppNet agent version v1",
			appNetAgentVersion:   "",
			expectedCapabilities: nil,
		},
		{
			name:                 "test supported service connect capabilities for AppNet agent version v1",
			appNetAgentVersion:   "v1",
			expectedCapabilities: []string{"ecs.capability.service-connect-v1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scManager := &manager{}
			scCapabilities, err := scManager.GetCapabilitiesForAppnetInterfaceVersion(tc.appNetAgentVersion)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedCapabilities, scCapabilities)
		})
	}
}
