//go:build !linux
// +build !linux

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
	"context"
	"fmt"
	"runtime"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/docker/docker/api/types"
)

var (
	defaultAgentContainerTarballPath = ""
)

// LoadImage returns UnsupportedPlatformError on the unsupported platform
func (*loader) LoadImage(ctx context.Context, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	return nil, NewUnsupportedPlatformError(fmt.Errorf(
		"appnetAgent container load: unsupported platform: %s/%s",
		runtime.GOOS, runtime.GOARCH))
}

func (*loader) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return false, NewUnsupportedPlatformError(fmt.Errorf(
		"appnetAgent container isloaded: unsupported platform: %s/%s",
		runtime.GOOS, runtime.GOARCH))
}

func (*loader) GetLoadedImageName() (string, error) {
	return "", NewUnsupportedPlatformError(fmt.Errorf(
		"appnetAgent container get image name: unsupported platform: %s/%s",
		runtime.GOOS, runtime.GOARCH))
}
