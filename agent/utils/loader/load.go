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

package loader

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/docker/docker/api/types"
)

// Loader defines an interface for loading the container images. This is mostly
// to facilitate mocking and testing
type Loader interface {
	LoadImage(ctx context.Context, cfg *config.Config, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error)
	IsLoaded(dockerClient dockerapi.DockerClient) (bool, error)
}

// GetContainerImage This function uses the DockerClient to inspect the image with the given name and tag.
func GetContainerImage(imageName string, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	logger.Debug("Inspecting container image: ", logger.Fields{
		imageName: imageName,
	})

	image, err := dockerClient.InspectImage(imageName)
	if err != nil {
		return nil, fmt.Errorf(
			"container load: failed to inspect image: %s, : %w", imageName, err)
	}

	return image, nil
}

// IsImageLoaded Common function for to check if a container image has been loaded
func IsImageLoaded(imageName string, dockerClient dockerapi.DockerClient) (bool, error) {
	image, err := GetContainerImage(imageName, dockerClient)

	if err != nil {
		return false, err
	}

	if image == nil || image.ID == "" {
		return false, nil
	}

	return true, nil
}

var open = os.Open

// LoadFromFile This function supports loading a container from a local file into docker
func LoadFromFile(ctx context.Context, path string, dockerClient dockerapi.DockerClient) error {
	containerReader, err := open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewNoSuchFileError(fmt.Errorf(
				"container load: failed to read container image: %s : %w", path, err))
		}
		return fmt.Errorf(
			"container load: failed to read container image: %s : %w", path, err)
	}
	if err := dockerClient.LoadImage(ctx, containerReader, dockerclient.LoadImageTimeout); err != nil {
		return fmt.Errorf(
			"container load: failed to load container image: %s : %w", path, err)
	}

	return nil

}
