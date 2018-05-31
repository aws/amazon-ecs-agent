// +build sudo integration

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

package engine

import (
	"context"
	"os"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/clientfactory"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/stretchr/testify/assert"
)

func defaultTestConfigIntegTest() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.TaskCPUMemLimit = config.ExplicitlyDisabled
	return cfg
}

func createTestTask(arn string) *apitask.Task {
	return &apitask.Task{
		Arn:                 arn,
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitask.TaskRunning,
		Containers:          []*apicontainer.Container{createTestContainer()},
	}
}

func verifyContainerRunningStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainer.ContainerRunning,
		"Expected container to be RUNNING")
}

func verifyContainerStoppedStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainer.ContainerStopped,
		"Expected container to be STOPPED")
}

func setup(cfg *config.Config, state dockerstate.TaskEngineState, t *testing.T) (TaskEngine, func(), credentials.Manager) {
	if os.Getenv("ECS_SKIP_ENGINE_INTEG_TEST") != "" {
		t.Skip("ECS_SKIP_ENGINE_INTEG_TEST")
	}
	if !isDockerRunning() {
		t.Skip("Docker not running")
	}
	clientFactory := clientfactory.NewFactory(context.TODO(), dockerEndpoint)
	dockerClient, err := dockerapi.NewDockerGoClient(clientFactory, cfg)
	if err != nil {
		t.Fatalf("Error creating Docker client: %v", err)
	}
	credentialsManager := credentials.NewManager()
	if state == nil {
		state = dockerstate.NewTaskEngineState()
	}
	imageManager := NewImageManager(cfg, dockerClient, state)
	imageManager.SetSaver(statemanager.NewNoopStateManager())
	metadataManager := containermetadata.NewManager(dockerClient, cfg)

	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, state, metadataManager, nil)
	taskEngine.MustInit(context.TODO())
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, credentialsManager
}

func createTestContainerWithImageAndName(image string, name string) *apicontainer.Container {
	return &apicontainer.Container{
		Name:                name,
		Image:               image,
		Command:             []string{},
		Essential:           true,
		DesiredStatusUnsafe: apicontainer.ContainerRunning,
		CPU:                 100,
		Memory:              80,
	}
}
