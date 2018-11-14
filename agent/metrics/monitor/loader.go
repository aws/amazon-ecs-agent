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

package monitor

import (
	"context"
	"fmt"
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler/os"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

const noSuchFile = "no such file or directory"

type NoSuchFileError struct {
	error
}

// This function loads the Docker image from a tarball packaged with Agent.
// It is adapted from the LoadImage(...) function from the Paues container
// TODO: Use a common utils LoadImage(...) call for both Prometheus and Pause
// containers
func LoadImage(ctx context.Context, cfg *config.Config, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	log.Debugf("Loading prometheus monitor tarball: %s", cfg.PrometheusMonitorContainerTarballPath)
	if err := loadFromFile(ctx, cfg.PrometheusMonitorContainerTarballPath, dockerClient, os.Default); err != nil {
		return nil, err
	}

	return getPrometheusContainerImage(
		cfg.PrometheusMonitorContainerImageName, cfg.PrometheusMonitorContainerTag, dockerClient)
}

func loadFromFile(ctx context.Context, path string, dockerClient dockerapi.DockerClient, fs os.FileSystem) error {
	prometheusMonitorContainerReader, err := fs.Open(path)
	if err != nil {
		if err.Error() == noSuchFile {
			return errors.Wrapf(err, "Prometheus Monitor container  load: failed to read Prometheus Monitor container  image: %s", path)
		}
		return errors.Wrapf(err, "Prometheus Monitor container  load: failed to read Prometheus Monitor container  image: %s", path)
	}
	if err := dockerClient.LoadImage(ctx, prometheusMonitorContainerReader, dockerclient.LoadImageTimeout); err != nil {
		return errors.Wrapf(err,
			"Prometheus Monitor container  load: failed to load Prometheus Monitor container  image: %s", path)
	}

	return nil

}

func getPrometheusContainerImage(name string, tag string, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	imageName := fmt.Sprintf("%s:%s", name, tag)
	log.Debugf("Inspecting Prometheus Monitor container  image: %s", imageName)

	image, err := dockerClient.InspectImage(imageName)
	if err != nil {
		return nil, errors.Wrapf(err,
			"Prometheus Monitor container load: failed to inspect image: %s", imageName)
	}

	return image, nil
}
