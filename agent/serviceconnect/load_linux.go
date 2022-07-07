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

package serviceconnect

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"

	"github.com/docker/docker/api/types"
)

var (
	defaultAgentContainerTarballPath = "/managed-agents/serviceconnect/appnet_agent.interface-v1.tar"
)

// LoadImage helps load the AppNetAgent container image for the agent
func (agent *loader) LoadImage(ctx context.Context, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	logger.Debug("Loading appnet agent container tarball:", logger.Fields{
		field.Image: agent.AgentContainerTarballPath,
	})
	if err := loadFromFile(ctx, agent.AgentContainerTarballPath, dockerClient); err != nil {
		return nil, err
	}

	return getAgentContainerImage(
		agent.AgentContainerImageName, agent.AgentContainerTag, dockerClient)
}

func (agent *loader) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	return agent.isImageLoaded(dockerClient)
}

var open = os.Open

func loadFromFile(ctx context.Context, path string, dockerClient dockerapi.DockerClient) error {
	containerReader, err := open(path)
	if err != nil {
		if err.Error() == noSuchFile {
			return NewNoSuchFileError(fmt.Errorf(
				"appnet agent container load: failed to read container image: %s : %w", path, err))
		}
		return fmt.Errorf("appnet agent container load: failed to read container image: %s : %w", path, err)
	}
	if err := dockerClient.LoadImage(ctx, containerReader, dockerclient.LoadImageTimeout); err != nil {
		return fmt.Errorf("appnet agent container load: failed to load container image: %s : %w", path, err)
	}

	return nil

}
