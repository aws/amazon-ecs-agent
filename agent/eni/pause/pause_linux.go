//go:build linux
// +build linux

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
	"os"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

// LoadImage helps load the pause container image for the agent
func (*loader) LoadImage(ctx context.Context, cfg *config.Config, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	log.Debugf("Loading pause container tarball: %s", cfg.PauseContainerTarballPath)
	if err := loadFromFile(ctx, cfg.PauseContainerTarballPath, dockerClient); err != nil {
		return nil, err
	}

	return getPauseContainerImage(
		config.DefaultPauseContainerImageName, config.DefaultPauseContainerTag, dockerClient)
}

func (*loader) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return isImageLoaded(dockerClient)
}

var open = os.Open

func loadFromFile(ctx context.Context, path string, dockerClient dockerapi.DockerClient) error {
	pauseContainerReader, err := open(path)
	if err != nil {
		if err.Error() == noSuchFile {
			return NewNoSuchFileError(errors.Wrapf(err,
				"pause container load: failed to read pause container image: %s", path))
		}
		return errors.Wrapf(err,
			"pause container load: failed to read pause container image: %s", path)
	}
	if err := dockerClient.LoadImage(ctx, pauseContainerReader, dockerclient.LoadImageTimeout); err != nil {
		return errors.Wrapf(err,
			"pause container load: failed to load pause container image: %s", path)
	}

	return nil

}
