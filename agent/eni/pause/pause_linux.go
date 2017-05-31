// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler/os"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"

	log "github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

// Load helps load the pause container image for the agent
func Load(cfg *config.Config, dockerClient engine.DockerClient) error {
	log.Debugf("Loading pause container tarball: %s:%s", cfg.PauseContainerTarballPath, cfg.PauseContainerTag)
	return _load(cfg, dockerClient, os.Default)
}

func _load(cfg *config.Config, dockerClient engine.DockerClient, osi os.FileSystem) error {
	pauseContainerReader, err := osi.Open(cfg.PauseContainerTarballPath)
	if err != nil {
		return errors.Wrapf(err, "pause container load: reading pause container image %s failed with error %v", cfg.PauseContainerTarballPath, err)
	}
	err = dockerClient.LoadImage(
		docker.LoadImageOptions{
			InputStream: pauseContainerReader,
		},
	)
	return err
}
