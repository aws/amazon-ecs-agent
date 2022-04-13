//go:build windows
// +build windows

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

package pause

import (
	"context"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

// In Linux, we use a tar archive to load the pause image. Whereas in Windows, we will cache the image during AMI build.
// Therefore, this functionality is not supported in Windows.
func (*loader) LoadImage(ctx context.Context, cfg *config.Config, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	return nil, errors.New("this functionality is not supported on this platform.")
}

// This method is used to inspect the presence of the pause image. If the image has not been loaded then we return false.
func (*loader) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return isImageLoaded(dockerClient)
}
