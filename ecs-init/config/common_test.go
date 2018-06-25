// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
