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
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

// Loader defines an interface for loading the pause container image. This is mostly
// to facilitate mocking and testing of the LoadImage method
type Loader interface {
	LoadImage(ctx context.Context, cfg *config.Config, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error)
	IsLoaded(dockerClient dockerapi.DockerClient) (bool, error)
}

type loader struct{}

// New creates a new pause image loader
func New() Loader {
	return &loader{}
}

// This function uses the DockerClient to inspect the image with the given name and tag.
func getPauseContainerImage(name string, tag string, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	imageName := fmt.Sprintf("%s:%s", name, tag)
	log.Debugf("Inspecting pause container image: %s", imageName)

	image, err := dockerClient.InspectImage(imageName)
	if err != nil {
		return nil, errors.Wrapf(err,
			"pause container load: failed to inspect image: %s", imageName)
	}

	return image, nil
}

// Common function for linux and windows to check if the container pause image has been loaded
func isImageLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	image, err := getPauseContainerImage(
		config.DefaultPauseContainerImageName, config.DefaultPauseContainerTag, dockerClient)

	if err != nil {
		return false, err
	}

	if image == nil || image.ID == "" {
		return false, nil
	}

	return true, nil
}
