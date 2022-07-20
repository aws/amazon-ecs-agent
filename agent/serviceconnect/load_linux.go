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

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"
	"github.com/aws/amazon-ecs-agent/agent/utils/loader"

	"github.com/docker/docker/api/types"
)

var (
	defaultAgentContainerTarballPath = "/managed-agents/serviceconnect/appnet_agent.interface-v1.tar"
)

// LoadImage helps load the AppNetAgent container image for the agent
func (agent *agentLoader) LoadImage(ctx context.Context, _ *config.Config, dockerClient dockerapi.DockerClient) (*types.ImageInspect, error) {
	logger.Debug("Loading appnet agent container tarball:", logger.Fields{
		field.Image: agent.AgentContainerTarballPath,
	})
	if err := loader.LoadFromFile(ctx, agent.AgentContainerTarballPath, dockerClient); err != nil {
		return nil, err
	}

	imageName, _ := agent.GetLoadedImageName()
	return loader.GetContainerImage(imageName, dockerClient)
}

func (agent *agentLoader) IsLoaded(dockerClient dockerapi.DockerClient) (bool, error) {
	imageName, _ := agent.GetLoadedImageName()
	return loader.IsImageLoaded(imageName, dockerClient)
}

func (agent *agentLoader) GetLoadedImageName() (string, error) {
	return fmt.Sprintf("%s:%s", agent.AgentContainerImageName, agent.AgentContainerTag), nil
}
