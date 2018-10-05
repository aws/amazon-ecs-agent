// +build linux
// +build sudo integration

// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package engine

import (
	"os"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
)

const (
	testRegistryImage = "127.0.0.1:51670/amazon/amazon-ecs-netkitten:latest"
	dockerEndpoint    = "unix:///var/run/docker.sock"
)

func createTestContainer() *apicontainer.Container {
	return createTestContainerWithImageAndName(testRegistryImage, "netcat")
}

func isDockerRunning() bool {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		return false
	}
	return true
}
