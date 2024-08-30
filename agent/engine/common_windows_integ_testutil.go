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
	"os"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
)

const (
	testBaseImage  = "amazon-ecs-ftest-windows-base:make"
	dockerEndpoint = "npipe:////./pipe/docker_engine"
)

// REGISTRY_IMAGE_NAME is the Windows Server image from Microsoft Container Registry.
// https://github.com/aws/amazon-ecs-agent/blob/78a2bf0c7d3ebd3a13de3ac733af46dfb3816b18/scripts/run-integ-tests.ps1#L45
var (
	testRegistryImage           = os.Getenv("REGISTRY_IMAGE_NAME")
	testRegistryImageWithDigest = os.Getenv("REGISTRY_IMAGE_NAME_WITH_DIGEST")
)

func CreateTestContainer() *apicontainer.Container {
	return createTestContainerWithImageAndName(testBaseImage, "windows")
}

// GetLongRunningCommand returns the command that keeps the container running for the container
// that uses the default integ test image (amazon-ecs-ftest-windows-base:make for windows)
func GetLongRunningCommand() []string {
	return []string{"ping", "-t", "localhost"}
}

func isDockerRunning() bool {
	_, err := os.Stat("//./pipe/docker_engine")
	return err == nil
}
