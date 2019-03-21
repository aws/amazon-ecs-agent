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

package config

import (
	"fmt"
	"os"
	"testing"
)

func TestDockerUnixSocketWithoutDockerHost(t *testing.T) {

	// Make sure that the env variable is not set
	os.Unsetenv("DOCKER_HOST")

	dockerUnixSocketSourcePath, fromEnv := DockerUnixSocket()

	if dockerUnixSocketSourcePath != "/var/run" {
		t.Error("DockerUnixSocket() should be \"/var/run\"")
	}
	if fromEnv {
		t.Error("DockerUnixSocket() should return the default instead of reading from DOCKER_HOST when DOCKER_HOST isn't set")
	}
}

func TestDockerUnixSocketWithDockerHost(t *testing.T) {
	os.Setenv("DOCKER_HOST", "unix:///foo/bar")
	defer os.Unsetenv("DOCKER_HOST")

	dockerUnixSocketSourcePath, fromEnv := DockerUnixSocket()
	if dockerUnixSocketSourcePath != "/foo/bar" {
		t.Error("DockerUnixSocket() should be \"/foo/bar\"")
	}
	if !fromEnv {
		t.Error("DockerUnixSocket() should read from envrionment variable DOCKER_HOST, when DOCKER_HOST is set")
	}
}

func TestGetAgentPartitionBucketRegion(t *testing.T) {
	testCases := []struct {
		region      string
		destination string
		err         error
	}{
		{
			region:      "us-west-2",
			destination: "us-east-1",
		}, {
			region:      "us-gov-west-1",
			destination: "us-gov-west-1",
		}, {
			region:      "cn-north-1",
			destination: "cn-north-1",
		}, {
			region: "invalid",
			err:    fmt.Errorf("Partition not found"),
		},
	}

	for _, testcase := range testCases {
		t.Run(fmt.Sprintf("%s -> %s", testcase.region, testcase.destination),
			func(t *testing.T) {
				region, err := GetAgentPartitionBucketRegion(testcase.region)
				if region != "" && region != testcase.destination && err != nil {
					t.Errorf("GetAgentBucketRegion returned unexpected region: %s, err: %v", region, err)
				}
				if testcase.err != nil && err == nil {
					t.Error("GetAgentBucketRegion should return an error if the destination is not found")
				}
			})
	}
}

func TestAgentDockerLogDriverConfiguration(t *testing.T) {
	resetEnv := func() {
		os.Unsetenv(dockerJSONLogMaxFilesEnvVar)
		os.Unsetenv(dockerJSONLogMaxSizeEnvVar)
	}
	resetEnv()

	testcases := []struct {
		name string

		envFiles string
		envSize  string

		expectedFiles string
		expectedSize  string
	}{

		{
			name: "defaults",

			envFiles: "",
			envSize:  "",

			expectedFiles: dockerJSONLogMaxFiles,
			expectedSize:  dockerJSONLogMaxSize,
		},
		{
			name: "environment set",

			envFiles: "1",
			envSize:  "1m",

			expectedFiles: "1",
			expectedSize:  "1m",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			resetEnv()

			os.Setenv(dockerJSONLogMaxFilesEnvVar, test.envFiles)
			os.Setenv(dockerJSONLogMaxSizeEnvVar, test.envSize)

			result := AgentDockerLogDriverConfiguration()
			if actual := result.Config["max-size"]; actual != test.expectedSize {
				t.Errorf("Configured max-size %q is not the expected %q", actual, test.expectedSize)
			}
			if actual := result.Config["max-file"]; actual != test.expectedFiles {
				t.Errorf("Configured max-files %q is not the expected %q", actual, test.expectedFiles)
			}
		})
	}
}

func TestAgentRemoteTarballKey(t *testing.T) {
	testcases := []struct {
		arch        string
		shouldError bool
		expected    string
	}{
		{
			arch:     "amd64",
			expected: "ecs-agent-" + DefaultAgentVersion + ".tar",
		},
		{
			arch:     "arm64",
			expected: "ecs-agent-arm64-" + DefaultAgentVersion + ".tar",
		},
		{
			arch:        "unknown",
			shouldError: true,
		},
	}

	originalGoarch := goarch
	defer func() { goarch = originalGoarch }()

	for _, test := range testcases {
		t.Run(test.arch, func(t *testing.T) {
			goarch = test.arch

			actual, err := AgentRemoteTarballKey()
			if err == nil && test.shouldError {
				t.Fatal("expected error when trying to get tarball key")
			}
			if err != nil && !test.shouldError {
				t.Fatalf("unexpected error when trying to get tarball key: %s", err)
			}
			if actual != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, actual)
			}
		})
	}
}

func TestAgentPrivileged(t *testing.T) {
	os.Setenv("ECS_AGENT_RUN_PRIVILEGED", "true")
	defer os.Unsetenv("ECS_AGENT_RUN_PRIVILEGED")

	if !RunPrivileged() {
		t.Fatalf("Agent was expected to be running with privileged mode")
	}
}

func TestAgentPrivilegedNotConfigured(t *testing.T) {
	defer os.Unsetenv("ECS_AGENT_RUN_PRIVILEGED")
	cases := []string{
		"false",
		"unrelated_word",
		"1",
		"",
	}

	for _, test := range cases {
		os.Setenv("ECS_AGENT_RUN_PRIVILEGED", test)

		if RunPrivileged() {
			t.Errorf("Agent was expected to be running without privileged mode. Testcase (%s)", test)
		}
	}
}
