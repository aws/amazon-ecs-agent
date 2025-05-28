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
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	dockertypes "github.com/docker/docker/api/types"
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
			err := scManager.AugmentTaskContainer(scTask, tc.container, hostConfig,
				ipcompatibility.NewIPv4OnlyCompatibility())
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

func TestGetRegionFromContainerInstanceARN(t *testing.T) {
	tests := []struct {
		name                 string
		containerInstanceARN string
		expectedRegion       string
	}{
		{
			name:                 "Valid ARN - US West 2",
			containerInstanceARN: "arn:aws:ecs:us-west-2:123456789012:container-instance/12345678-1234-1234-1234-123456789012",
			expectedRegion:       "us-west-2",
		},
		{
			name:                 "Valid ARN - EU Central 1",
			containerInstanceARN: "arn:aws:ecs:eu-central-1:123456789012:container-instance/87654321-4321-4321-4321-210987654321",
			expectedRegion:       "eu-central-1",
		},
		{
			name:                 "Valid ARN - AP Southeast 1",
			containerInstanceARN: "arn:aws:ecs:ap-southeast-1:123456789012:container-instance/11223344-5566-7788-9900-112233445566",
			expectedRegion:       "ap-southeast-1",
		},
		{
			name:                 "Valid ARN - US Gov West 1",
			containerInstanceARN: "arn:aws-us-gov:ecs:us-gov-west-1:123456789012:container-instance/98765432-1234-5678-9012-123456789012",
			expectedRegion:       "us-gov-west-1",
		},
		{
			name:                 "Valid ARN - CN North 1",
			containerInstanceARN: "arn:aws-cn:ecs:cn-north-1:123456789012:container-instance/11223344-5566-7788-9900-112233445566",
			expectedRegion:       "cn-north-1",
		},
		{
			name:                 "Invalid ARN - Missing Region",
			containerInstanceARN: "arn:aws:ecs::123456789012:container-instance/12345678-1234-1234-1234-123456789012",
			expectedRegion:       "",
		},
		{
			name:                 "Invalid ARN - Wrong Service",
			containerInstanceARN: "arn:aws:ec2:us-west-2:123456789012:instance/i-1234567890abcdef0",
			expectedRegion:       "us-west-2",
		},
		{
			name:                 "Invalid ARN Format",
			containerInstanceARN: "invalid:arn:format",
			expectedRegion:       "",
		},
		{
			name:                 "Empty ARN",
			containerInstanceARN: "",
			expectedRegion:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRegionFromContainerInstanceARN(tt.containerInstanceARN)
			assert.Equal(t, tt.expectedRegion, result, "Unexpected region for ARN: %s", tt.containerInstanceARN)
		})
	}
}

func TestIsIsoRegion(t *testing.T) {
	tests := []struct {
		name           string
		region         string
		expectedResult bool
	}{
		{
			name:           "AWS Standard Region - US West 2",
			region:         "us-west-2",
			expectedResult: false,
		},
		{
			name:           "AWS Standard Region - EU Central 1",
			region:         "eu-central-1",
			expectedResult: false,
		},
		{
			name:           "AWS GovCloud Region - US Gov West 1",
			region:         "us-gov-west-1",
			expectedResult: false,
		},
		{
			name:           "AWS GovCloud Region - US Gov East 1",
			region:         "us-gov-east-1",
			expectedResult: false,
		},
		{
			name:           "AWS China Region - CN North 1",
			region:         "cn-north-1",
			expectedResult: false,
		},
		{
			name:           "AWS China Region - CN Northwest 1",
			region:         "cn-northwest-1",
			expectedResult: false,
		},
		{
			name:           "ISO Region - US ISO East 1",
			region:         "us-iso-east-1",
			expectedResult: true,
		},
		{
			name:           "ISO Region - US ISOB East 1",
			region:         "us-isob-east-1",
			expectedResult: true,
		},
		{
			name:           "Unknown Region",
			region:         "unknown-region",
			expectedResult: true,
		},
		{
			name:           "Empty Region",
			region:         "",
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIsoRegion(tt.region)
			assert.Equal(t, tt.expectedResult, result, "Unexpected result for region: %s", tt.region)
		})
	}
}

// Tests that AugmentTaskContainer returns an error if it fails.
func TestAugmentTaskContainerError(t *testing.T) {
	t.Run("returns an error if container IP mapping could not be generated", func(t *testing.T) {
		// Task containers do not have an IPv6 address
		task := &apitask.Task{
			NetworkMode: apitask.BridgeNetworkMode,
			Containers: []*apicontainer.Container{
				{
					Type: apicontainer.ContainerCNIPause,
					Name: "~internal~ecs~pause-web",
					NetworkSettingsUnsafe: &dockertypes.NetworkSettings{
						DefaultNetworkSettings: dockertypes.DefaultNetworkSettings{
							IPAddress: "1.2.3.4",
						},
					},
				},
				{
					Type: apicontainer.ContainerNormal,
					Name: "web",
				},
				{
					Type: apicontainer.ContainerCNIPause,
					Name: "~internal~ecs~pause-sc-container",
					NetworkSettingsUnsafe: &dockertypes.NetworkSettings{
						DefaultNetworkSettings: dockertypes.DefaultNetworkSettings{
							IPAddress: "1.2.3.5",
						},
					},
				},
				{
					Type: apicontainer.ContainerNormal,
					Name: "sc-container",
				},
			},
			ServiceConnectConfig: &serviceconnect.Config{ContainerName: "sc-container"},
		}
		scManager := &manager{}

		// Instance has IPv6-only compatibility
		err := scManager.AugmentTaskContainer(task, task.Containers[3], nil, ipcompatibility.NewIPv6OnlyCompatibility())
		namedErr, ok := err.(dockerapi.CannotCreateContainerError)
		assert.True(t, ok)
		assert.EqualError(t, namedErr, "instance is IPv6-only but no IPv6 address found for container 'web'")
	})
}
