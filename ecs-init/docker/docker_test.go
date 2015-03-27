// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package docker

import (
	"errors"
	"testing"

	"code.google.com/p/gomock/gomock"
	"github.com/aws/amazon-ecs-init/ecs-init/config"
	godocker "github.com/fsouza/go-dockerclient"
)

func TestIsAgentImageLoadedListFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListImages(godocker.ListImagesOptions{All: true}).Return(nil, errors.New("test error"))

	client := &Client{
		docker: mockDocker,
	}
	loaded, err := client.IsAgentImageLoaded()
	if err == nil {
		t.Error("Error should be returned")
	}
	if loaded {
		t.Error("IsImageLoaded should return false")
	}
}

func TestIsAgentImageLoadedNoMatches(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListImages(godocker.ListImagesOptions{All: true}).Return(
		append(make([]godocker.APIImages, 0), godocker.APIImages{
			RepoTags: append(make([]string, 0), ""),
		}), nil)

	client := &Client{
		docker: mockDocker,
	}
	loaded, err := client.IsAgentImageLoaded()
	if err != nil {
		t.Error("Error should not be returned")
	}
	if loaded {
		t.Error("IsImageLoaded should return false")
	}
}

func TestIsImageLoadedMatches(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListImages(godocker.ListImagesOptions{All: true}).Return(
		append(make([]godocker.APIImages, 0), godocker.APIImages{
			RepoTags: append(make([]string, 0), config.AgentImageName),
		}), nil)

	client := &Client{
		docker: mockDocker,
	}
	loaded, err := client.IsAgentImageLoaded()
	if err != nil {
		t.Error("Error should not be returned")
	}
	if !loaded {
		t.Error("IsImageLoaded should return true")
	}
}

func TestLoadImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().LoadImage(godocker.LoadImageOptions{nil})

	client := &Client{
		docker: mockDocker,
	}
	client.LoadImage(nil)
}

func TestRemoveExistingAgentContainerListContainersFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListContainers(godocker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"status": []string{},
		},
	}).Return(nil, errors.New("test error"))

	client := &Client{
		docker: mockDocker,
	}
	err := client.RemoveExistingAgentContainer()
	if err == nil {
		t.Error("Error should be returned")
	}
}

func TestRemoveExistingAgentContainerNoneFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListContainers(godocker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"status": []string{},
		},
	})

	client := &Client{
		docker: mockDocker,
	}
	err := client.RemoveExistingAgentContainer()
	if err != nil {
		t.Error("Error should not be returned")
	}
}

func TestRemoveExistingAgentContainer(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListContainers(godocker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"status": []string{},
		},
	}).Return([]godocker.APIContainers{
		godocker.APIContainers{
			Names: []string{"/" + config.AgentContainerName},
			ID:    "id",
		},
	}, nil)
	mockDocker.EXPECT().RemoveContainer(godocker.RemoveContainerOptions{
		ID:    "id",
		Force: true,
	})

	client := &Client{
		docker: mockDocker,
	}
	err := client.RemoveExistingAgentContainer()
	if err != nil {
		t.Error("Error should not be returned")
	}
}

func TestStartAgentNoEnvFile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	containerID := "container id"

	mockFS := NewMockfileSystem(mockCtrl)
	mockDocker := NewMockdockerclient(mockCtrl)

	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return(nil, errors.New("test error"))
	mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(opts godocker.CreateContainerOptions) {
		validateCommonCreateContainerOptions(opts, t)
	}).Return(&godocker.Container{
		ID: containerID,
	}, nil)
	mockDocker.EXPECT().StartContainer(containerID, nil)
	mockDocker.EXPECT().WaitContainer(containerID)

	client := &Client{
		docker: mockDocker,
		fs:     mockFS,
	}

	_, err := client.StartAgent()
	if err != nil {
		t.Error("Error should not be returned")
	}
}

func validateCommonCreateContainerOptions(opts godocker.CreateContainerOptions, t *testing.T) {
	if opts.Name != "ecs-agent" {
		t.Errorf("Expected container Name to be %s but was %s", "ecs-agent", opts.Name)
	}
	cfg := opts.Config

	if len(cfg.Env) < 3 {
		t.Errorf("Expected at least 3 elements to be in Env, but was %d", len(cfg.Env))
	}
	envVariables := make(map[string]struct{})
	for _, envVar := range cfg.Env {
		envVariables[envVar] = struct{}{}
	}
	expectKey("ECS_DATADIR=/data", envVariables, t)
	expectKey("ECS_LOGFILE=/log/"+config.AgentLogFile, envVariables, t)
	expectKey("ECS_AGENT_CONFIG_FILE_PATH="+config.AgentJSONConfigFile(), envVariables, t)

	if len(cfg.ExposedPorts) != 1 {
		t.Errorf("Expected exactly 1 element to be in ExposedPorts, but was %d", len(cfg.ExposedPorts))
	}
	expectPort("51678/tcp", cfg.ExposedPorts, t)

	if cfg.Image != config.AgentImageName {
		t.Errorf("Expected image to be %s", config.AgentImageName)
	}

	hostCfg := opts.HostConfig

	if len(hostCfg.Binds) != 4 {
		t.Errorf("Expected exactly 4 elements to be in Binds, but was %d", len(hostCfg.Binds))
	}
	binds := make(map[string]struct{})
	for _, binding := range hostCfg.Binds {
		binds[binding] = struct{}{}
	}
	expectKey(defaultDockerEndpoint+":"+defaultDockerEndpoint, binds, t)
	expectKey(config.LogDirectory()+":/log", binds, t)
	expectKey(config.AgentDataDirectory()+":/data", binds, t)
	expectKey(config.AgentConfigDirectory()+":"+config.AgentConfigDirectory(), binds, t)

	if len(hostCfg.PortBindings) != 1 {
		t.Errorf("Expected exactly 1 element to be in PortBindings, but was %d", len(hostCfg.PortBindings))
	}
	if portBindings, ok := hostCfg.PortBindings["51678/tcp"]; ok {
		if len(portBindings) != 1 {
			t.Errorf("Expected exactly 1 element to be in portBindings, but was %d", len(portBindings))
		} else {
			portBinding := portBindings[0]
			if portBinding.HostIP != "127.0.0.1" {
				t.Errorf("Expected HostIP to be 127.0.0.1, but was %s", portBinding.HostIP)
			}
			if portBinding.HostPort != "51678" {
				t.Errorf("Expected HostPort to be 51678, but was %s", portBinding.HostPort)
			}
		}
	} else {
		t.Error("Expected 51678/tcp to be defined")
	}
}

func expectPort(key godocker.Port, input map[godocker.Port]struct{}, t *testing.T) {
	if _, ok := input[key]; !ok {
		t.Errorf("Expected %s to be defined", key)
	}
}
func expectKey(key string, input map[string]struct{}, t *testing.T) {
	if _, ok := input[key]; !ok {
		t.Errorf("Expected %s to be defined", key)
	}
}

func TestStartAgentEnvFile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	envFile := "\nAGENT_TEST_VAR=val\nAGENT_TEST_VAR2=val2\n"
	containerID := "container id"

	mockFS := NewMockfileSystem(mockCtrl)
	mockDocker := NewMockdockerclient(mockCtrl)

	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return([]byte(envFile), nil)
	mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(opts godocker.CreateContainerOptions) {
		validateCommonCreateContainerOptions(opts, t)
		cfg := opts.Config

		envVariables := make(map[string]struct{})
		for _, envVar := range cfg.Env {
			envVariables[envVar] = struct{}{}
		}
		expectKey("AGENT_TEST_VAR=val", envVariables, t)
		expectKey("AGENT_TEST_VAR2=val2", envVariables, t)
	}).Return(&godocker.Container{
		ID: containerID,
	}, nil)
	mockDocker.EXPECT().StartContainer(containerID, nil)
	mockDocker.EXPECT().WaitContainer(containerID)

	client := &Client{
		docker: mockDocker,
		fs:     mockFS,
	}

	_, err := client.StartAgent()
	if err != nil {
		t.Error("Error should not be returned")
	}
}

func TestStopAgentError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListContainers(godocker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"status": []string{},
		},
	}).Return(nil, errors.New("test error"))

	client := &Client{
		docker: mockDocker,
	}

	err := client.StopAgent()
	if err == nil {
		t.Error("Error should be returned")
	}
}

func TestStopAgentNone(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListContainers(godocker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"status": []string{},
		},
	}).Return([]godocker.APIContainers{godocker.APIContainers{}}, nil)

	client := &Client{
		docker: mockDocker,
	}

	err := client.StopAgent()
	if err != nil {
		t.Error("Error should not be returned")
	}
}

func TestStopAgent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListContainers(godocker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"status": []string{},
		},
	}).Return([]godocker.APIContainers{
		godocker.APIContainers{
			Names: []string{"/" + config.AgentContainerName},
			ID:    "id",
		},
	}, nil)
	mockDocker.EXPECT().StopContainer("id", uint(10))

	client := &Client{
		docker: mockDocker,
	}

	err := client.StopAgent()
	if err != nil {
		t.Error("Error should not be returned")
	}
}
