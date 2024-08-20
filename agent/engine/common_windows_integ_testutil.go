//go:build windows && (sudo || integration)
// +build windows
// +build sudo integration

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

package engine

import (
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
)

const (
	testBaseImage     = "amazon-ecs-ftest-windows-base:make"
	testRegistryImage = "amazon/amazon-ecs-netkitten:make"
	dockerEndpoint    = "npipe:////./pipe/docker_engine"
)

func CreateTestContainer() *apicontainer.Container {
	return createTestContainerWithImageAndName(testBaseImage, "windows")
}

// GetLongRunningCommand returns the command that keeps the container running for the container
// that uses the default integ test image (amazon-ecs-ftest-windows-base:make for windows)
func GetLongRunningCommand() []string {
	return []string{"ping", "-t", "localhost"}
}

// TODO: implement this
func isDockerRunning() bool {
	return true
}
