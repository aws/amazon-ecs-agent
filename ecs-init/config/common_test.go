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
	"os"
	"testing"
)

func TestDockerUnixSocketWithoutDockerHost(t *testing.T) {

	// Make sure that the env variable is not set
	os.Unsetenv("DOCKER_HOST")

	if DockerUnixSocket() != "/var/run/docker.sock" {
		t.Error("DockerUnixSocket() should be \"/var/run/docker.sock\"")
	}
}

func TestDockerUnixSocketWithDockerHost(t *testing.T) {

	os.Setenv("DOCKER_HOST", "unix:///foo/bar")

	if DockerUnixSocket() != "/foo/bar" {
		t.Error("DockerUnixSocket() should be \"/foo/bar\"")
	}
}
