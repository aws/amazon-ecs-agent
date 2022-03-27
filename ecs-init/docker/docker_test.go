//go:build test
// +build test

// Copyright 2015-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-init/config"
	"github.com/aws/amazon-ecs-agent/ecs-init/gpu"
	godocker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// expectedAgentBinds is the total number of agent host config binds.
// Note: Change this value every time when a new bind mount is added to
// agent for the tests to pass
const (
	expectedAgentBindsUnspecifiedPlatform = 20
	expectedAgentBindsSuseUbuntuPlatform  = 18
)

var expectedAgentBinds = expectedAgentBindsUnspecifiedPlatform

func TestIsAgentImageLoadedListFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListImages(godocker.ListImagesOptions{All: true}).Return(nil, errors.New("test error"))

	client := &client{
		docker: mockDocker,
	}
	loaded, err := client.IsAgentImageLoaded()
	assert.Error(t, err, "error should be returned when list image fails")
	assert.False(t, loaded, "IsImageLoaded should return false if list image fails")
}

func TestIsAgentImageLoadedNoMatches(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListImages(godocker.ListImagesOptions{All: true}).Return(
		append(make([]godocker.APIImages, 0), godocker.APIImages{
			RepoTags: append(make([]string, 0), ""),
		}), nil)

	client := &client{
		docker: mockDocker,
	}
	loaded, err := client.IsAgentImageLoaded()
	assert.NoError(t, err, "error should not be returned when no images match")
	assert.False(t, loaded, "IsImageLoaded should return false if there are no matches")
}

func TestIsImageLoadedMatches(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().ListImages(godocker.ListImagesOptions{All: true}).Return(
		append(make([]godocker.APIImages, 0), godocker.APIImages{
			RepoTags: append(make([]string, 0), config.AgentImageName),
		}), nil)

	client := &client{
		docker: mockDocker,
	}
	loaded, err := client.IsAgentImageLoaded()
	assert.NoError(t, err, "error should not be returned when image match is found")
	assert.True(t, loaded, "IsImageLoaded should return true if there is a match")
}

func TestLoadImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerclient(mockCtrl)

	mockDocker.EXPECT().LoadImage(godocker.LoadImageOptions{})

	client := &client{
		docker: mockDocker,
	}
	err := client.LoadImage(nil)
	assert.NoError(t, err, "no errors should be returned on load image with nil image")
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

	client := &client{
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

	client := &client{
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

	client := &client{
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
	isPathValid = func(path string, isDir bool) bool {
		return false
	}
	defer func() {
		isPathValid = defaultIsPathValid
	}()

	containerID := "container id"

	mockFS := NewMockfileSystem(mockCtrl)
	mockDocker := NewMockdockerclient(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return(nil, errors.New("not found")).AnyTimes()
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return(nil, errors.New("test error")).AnyTimes()
	mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(opts godocker.CreateContainerOptions) {
		validateCommonCreateContainerOptions(opts, t)
	}).Return(&godocker.Container{
		ID: containerID,
	}, nil)
	mockDocker.EXPECT().StartContainer(containerID, nil)
	mockDocker.EXPECT().WaitContainer(containerID)

	client := &client{
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
	expectKey("ECS_UPDATE_DOWNLOAD_DIR="+config.CacheDirectory(), envVariables, t)
	expectKey("ECS_UPDATES_ENABLED=true", envVariables, t)
	expectKey(`ECS_AVAILABLE_LOGGING_DRIVERS=["json-file","syslog","awslogs","fluentd","none"]`,
		envVariables, t)
	expectKey("ECS_ENABLE_TASK_IAM_ROLE=true", envVariables, t)
	expectKey("ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST=true", envVariables, t)
	expectKey("ECS_ENABLE_TASK_ENI=true", envVariables, t)
	expectKey("ECS_ENABLE_AWSLOGS_EXECUTIONROLE_OVERRIDE=true", envVariables, t)
	expectKey(`ECS_VOLUME_PLUGIN_CAPABILITIES=["efsAuth"]`, envVariables, t)
	if cfg.Image != config.AgentImageName {
		t.Errorf("Expected image to be %s", config.AgentImageName)
	}

	hostCfg := opts.HostConfig

	// for hosts that do not have cert directories explicity mounted, ignore
	// host cert directory configuration.
	// TODO (adnxn): ideally, these should be behind build flags.
	// https://github.com/aws/amazon-ecs-init/issues/131
	if certDir := config.HostPKIDirPath(); certDir == "" {
		expectedAgentBinds = expectedAgentBindsSuseUbuntuPlatform
	}

	if len(hostCfg.Binds) != expectedAgentBinds {
		t.Errorf("Expected exactly %d elements to be in Binds, but was %d", expectedAgentBinds, len(hostCfg.Binds))
	}
	binds := make(map[string]struct{})
	for _, binding := range hostCfg.Binds {
		binds[binding] = struct{}{}
	}
	defaultDockerSocket, _ := config.DockerUnixSocket()
	expectKey(defaultDockerSocket+":"+defaultDockerSocket, binds, t)
	expectKey(config.LogDirectory()+":/log", binds, t)
	expectKey(config.AgentDataDirectory()+":/data", binds, t)
	expectKey(config.AgentConfigDirectory()+":"+config.AgentConfigDirectory(), binds, t)
	expectKey(config.CacheDirectory()+":"+config.CacheDirectory(), binds, t)
	expectKey(config.ProcFS+":"+hostProcDir+":ro", binds, t)
	expectKey(iptablesUsrLibDir+":"+iptablesUsrLibDir+":ro", binds, t)
	expectKey(iptablesLibDir+":"+iptablesLibDir+":ro", binds, t)
	expectKey(iptablesUsrLib64Dir+":"+iptablesUsrLib64Dir+":ro", binds, t)
	expectKey(iptablesLib64Dir+":"+iptablesLib64Dir+":ro", binds, t)
	expectKey(iptablesExecutableHostDir+":"+iptablesExecutableContainerDir+":ro", binds, t)
	expectKey(iptablesAltDir+":"+iptablesAltDir+":ro", binds, t)
	expectKey(iptablesLegacyDir+":"+iptablesLegacyDir+":ro", binds, t)
	expectKey(config.LogDirectory()+"/exec:/log/exec", binds, t)
	for _, pluginDir := range pluginDirs {
		expectKey(pluginDir+":"+pluginDir+readOnly, binds, t)
	}

	if hostCfg.NetworkMode != networkMode {
		t.Errorf("Expected network mode to be %s, got %s", networkMode, hostCfg.NetworkMode)
	}

	if len(hostCfg.CapAdd) != 2 {
		t.Error("Mismatch detected in added host config capabilities")
	}

	capNetAdminFound := false
	capSysAdminFound := false
	for _, cap := range hostCfg.CapAdd {
		if cap == CapNetAdmin {
			capNetAdminFound = true
		}
		if cap == CapSysAdmin {
			capSysAdminFound = true
		}
	}
	if !capNetAdminFound {
		t.Errorf("Missing %s from host config capabilities", CapNetAdmin)
	}
	if !capSysAdminFound {
		t.Errorf("Missing %s from host config capabilities", CapSysAdmin)
	}

	if hostCfg.Init != true {
		t.Error("Incorrect host config. Expected Init to be true")
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
	isPathValid = func(path string, isDir bool) bool {
		return false
	}
	defer func() {
		isPathValid = defaultIsPathValid
	}()

	envFile := "\nAGENT_TEST_VAR=val\nAGENT_TEST_VAR2=val2\n"
	containerID := "container id"

	mockFS := NewMockfileSystem(mockCtrl)
	mockDocker := NewMockdockerclient(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return(nil, errors.New("not found")).AnyTimes()
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return([]byte(envFile), nil).AnyTimes()
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

	client := &client{
		docker: mockDocker,
		fs:     mockFS,
	}

	_, err := client.StartAgent()
	if err != nil {
		t.Error("Error should not be returned")
	}
}
func TestStartAgentWithGPUConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	isPathValid = func(path string, isDir bool) bool {
		return false
	}
	defer func() {
		isPathValid = defaultIsPathValid
	}()

	envFile := "\nECS_ENABLE_GPU_SUPPORT=true\n"
	containerID := "container id"
	expectedAgentBinds += 1

	defer func() {
		MatchFilePatternForGPU = FilePatternMatchForGPU
		expectedAgentBinds = expectedAgentBindsUnspecifiedPlatform
	}()
	MatchFilePatternForGPU = func(pattern string) ([]string, error) {
		return []string{"/dev/nvidia0", "/dev/nvidia1"}, nil
	}

	mockFS := NewMockfileSystem(mockCtrl)
	mockDocker := NewMockdockerclient(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return([]byte(envFile), nil).AnyTimes()
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return(nil, errors.New("not found")).AnyTimes()
	mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(opts godocker.CreateContainerOptions) {
		validateCommonCreateContainerOptions(opts, t)
		var found bool
		for _, bind := range opts.HostConfig.Binds {
			if bind == gpu.GPUInfoDirPath+":"+gpu.GPUInfoDirPath {
				found = true
				break
			}
		}
		assert.True(t, found)

		cfg := opts.Config

		envVariables := make(map[string]struct{})
		for _, envVar := range cfg.Env {
			envVariables[envVar] = struct{}{}
		}
	}).Return(&godocker.Container{
		ID: containerID,
	}, nil)
	mockDocker.EXPECT().StartContainer(containerID, nil)
	mockDocker.EXPECT().WaitContainer(containerID)

	client := &client{
		docker: mockDocker,
		fs:     mockFS,
	}

	_, err := client.StartAgent()
	assert.NoError(t, err)
}

func TestStartAgentWithGPUConfigNoDevices(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	isPathValid = func(path string, isDir bool) bool {
		return false
	}
	defer func() {
		isPathValid = defaultIsPathValid
	}()

	envFile := "\nECS_ENABLE_GPU_SUPPORT=true\n"
	containerID := "container id"

	defer func() {
		MatchFilePatternForGPU = FilePatternMatchForGPU
	}()
	MatchFilePatternForGPU = func(pattern string) ([]string, error) {
		// matches is nil
		return nil, nil
	}

	mockFS := NewMockfileSystem(mockCtrl)
	mockDocker := NewMockdockerclient(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return([]byte(envFile), nil).AnyTimes()
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return(nil, errors.New("not found")).AnyTimes()
	mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(opts godocker.CreateContainerOptions) {
		validateCommonCreateContainerOptions(opts, t)
		cfg := opts.Config

		envVariables := make(map[string]struct{})
		for _, envVar := range cfg.Env {
			envVariables[envVar] = struct{}{}
		}
	}).Return(&godocker.Container{
		ID: containerID,
	}, nil)
	mockDocker.EXPECT().StartContainer(containerID, nil)
	mockDocker.EXPECT().WaitContainer(containerID)

	client := &client{
		docker: mockDocker,
		fs:     mockFS,
	}

	_, err := client.StartAgent()
	assert.NoError(t, err)
}

func TestGetContainerConfigWithFileOverrides(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	envFile := "\nECS_UPDATES_ENABLED=false\nAGENT_TEST_VAR2=\"val2\"=val2\n"

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return(nil, errors.New("not found"))
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return([]byte(envFile), nil)

	client := &client{
		fs: mockFS,
	}
	envVarsFromFiles := client.LoadEnvVars()
	cfg := client.getContainerConfig(envVarsFromFiles)

	envVariables := make(map[string]struct{})
	for _, envVar := range cfg.Env {
		envVariables[envVar] = struct{}{}
	}
	expectKey("ECS_UPDATES_ENABLED=false", envVariables, t)
	expectKey("AGENT_TEST_VAR2=\"val2\"=val2", envVariables, t)
	if _, ok := envVariables["ECS_UPDATES_ENABLED=true"]; ok {
		t.Errorf("Did not expect ECS_UPDATES_ENABLED=true to be defined")
	}

}

func TestGetContainerConfigExternal(t *testing.T) {
	os.Setenv(config.ExternalEnvVar, "true")
	defer os.Unsetenv(config.ExternalEnvVar)

	client := &client{}
	cfg := client.getContainerConfig(map[string]string{})
	assert.Contains(t, cfg.Env, "ECS_ENABLE_TASK_ENI=false")
}

func TestGetInstanceConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	envFile := "\nECS_ENABLE_GPU_SUPPORT=true\n"
	defer func() {
		MatchFilePatternForGPU = FilePatternMatchForGPU
	}()
	MatchFilePatternForGPU = func(pattern string) ([]string, error) {
		// GPU device present
		return []string{"/dev/nvidia0"}, nil
	}

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return([]byte(envFile), nil)
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return(nil, errors.New("not found"))

	client := &client{
		fs: mockFS,
	}
	envVarsFromFiles := client.LoadEnvVars()
	cfg := client.getContainerConfig(envVarsFromFiles)

	envVariables := make(map[string]struct{})
	for _, envVar := range cfg.Env {
		envVariables[envVar] = struct{}{}
	}
	expectKey("ECS_ENABLE_GPU_SUPPORT=true", envVariables, t)
}

func TestGetNonGPUInstanceConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	envFile := "\nECS_ENABLE_GPU_SUPPORT=true\n"
	defer func() {
		MatchFilePatternForGPU = FilePatternMatchForGPU
	}()
	MatchFilePatternForGPU = func(pattern string) ([]string, error) {
		// matches is nil
		return nil, nil
	}

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return([]byte(envFile), nil)
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return(nil, errors.New("not found"))

	client := &client{
		fs: mockFS,
	}
	envVarsFromFiles := client.LoadEnvVars()
	cfg := client.getContainerConfig(envVarsFromFiles)

	envVariables := make(map[string]struct{})
	for _, envVar := range cfg.Env {
		envVariables[envVar] = struct{}{}
	}
	if _, ok := envVariables["ECS_ENABLE_GPU_SUPPORT=true"]; ok {
		t.Errorf("Expected ECS_ENABLE_GPU_SUPPORT=true to be not present")
	}
}

func TestGetConfigOverrides(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	instanceEnvFile := "\nECS_ENABLE_GPU_SUPPORT=true\n"
	userEnvFile := "\nECS_ENABLE_GPU_SUPPORT=false\n"

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return([]byte(instanceEnvFile), nil)
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return([]byte(userEnvFile), nil)

	client := &client{
		fs: mockFS,
	}
	envVarsFromFiles := client.LoadEnvVars()
	cfg := client.getContainerConfig(envVarsFromFiles)

	envVariables := make(map[string]struct{})
	for _, envVar := range cfg.Env {
		envVariables[envVar] = struct{}{}
	}
	expectKey("ECS_ENABLE_GPU_SUPPORT=false", envVariables, t)
}

func TestStopAgent(t *testing.T) {
	testCases := []struct {
		name                 string
		listFailed           bool
		listEmpty            bool
		stopFailedNotRunning bool
		stopFailedOther      bool
		expectedError        bool
	}{
		{
			name: "List containers succeeded, stop agent succeeded",
		},
		{
			name:       "List containers failed",
			listFailed: true,
		},
		{
			name:      "List containers no agent present",
			listEmpty: true,
		},
		{
			name:                 "List containers succeeded, stop agent failed on container not running",
			stopFailedNotRunning: true,
		},
		{
			name:            "List containers succeeded, stop agent failed on error other than not running",
			stopFailedOther: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockDocker := NewMockdockerclient(mockCtrl)
			client := &client{
				docker: mockDocker,
			}

			var listInput godocker.ListContainersOptions
			var listOutput []godocker.APIContainers
			var listErr error
			listInput = godocker.ListContainersOptions{
				All: true,
				Filters: map[string][]string{
					"status": {},
				},
			}
			if tc.listEmpty || tc.listFailed {
				listOutput = []godocker.APIContainers{{}}
			} else {
				listOutput = []godocker.APIContainers{
					{
						Names: []string{"/" + config.AgentContainerName},
						ID:    "id",
					},
				}
			}

			if tc.listFailed {
				listErr = errors.New("test error")
			}

			mockDocker.EXPECT().ListContainers(listInput).Return(listOutput, listErr)

			var stopErr error
			if tc.stopFailedNotRunning {
				stopErr = &godocker.ContainerNotRunning{ID: "id"}
			} else if tc.stopFailedOther {
				stopErr = errors.New("test error")
			}

			if !tc.listEmpty && !tc.listFailed {
				mockDocker.EXPECT().StopContainer("id", uint(10)).Return(stopErr)
			}

			if tc.listFailed || tc.stopFailedOther {
				assert.Error(t, client.StopAgent())
			} else {
				assert.NoError(t, client.StopAgent())
			}
		})
	}
}

func TestContainerLabels(t *testing.T) {
	testData := `{"test.label.1":"value1","test.label.2":"value2"}`
	out, err := generateLabelMap(testData)
	if err != nil {
		t.Logf("Got an error while decoding labels, error: %s", err)
		t.Fail()
	}
	if out["test.label.1"] != "value1" {
		t.Logf("Label did not give the correct value out.")
		t.Fail()
	}

	for key, value := range out {
		t.Logf("Key: %s %T | Value: %s %T", key, key, value, value)
	}
}

func TestContainerLabelsNoData(t *testing.T) {
	tests := []struct {
		name     string
		testData string
	}{
		{
			name:     "Blank",
			testData: "",
		},
		{
			name:     "Empty JSON",
			testData: `{}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &godocker.Config{}

			setLabels(cfg, test.testData)

			if cfg.Labels != nil {
				t.Logf("Labels are set but we didn't give any. Current labels: %s", cfg.Labels)
				t.Fail()
			}
		})
	}
}

func TestContainerLabelsBadData(t *testing.T) {
	testData := `{"something":[{"test.label.1":"value1"},{"test.label.2":"value2"}]}`
	_, err := generateLabelMap(testData)
	if err == nil {
		t.Logf("Didn't get a error while getting lables on badly formatted data, error: %s", err)
		t.Fail()
	}
}

func TestGetDockerSocketBind(t *testing.T) {
	testCases := []struct {
		name                     string
		dockerHostFromEnv        string
		dockerHostFromConfigFile string
		expectedBind             string
	}{
		{
			name:                     "No Docker host from env",
			dockerHostFromEnv:        "",
			dockerHostFromConfigFile: "dummy",
			expectedBind:             "/var/run:/var/run",
		},
		{
			name:                     "Invalid Docker host from env",
			dockerHostFromEnv:        "invalid",
			dockerHostFromConfigFile: "dummy",
			expectedBind:             "/var/run:/var/run",
		},
		{
			name:                     "Docker host from env, no Docker host from config file",
			dockerHostFromEnv:        "unix:///var/run/docker.sock",
			dockerHostFromConfigFile: "",
			expectedBind:             "/var/run/docker.sock:/var/run/docker.sock",
		},
		{
			name:                     "Docker host from env, invalid Docker host from config file",
			dockerHostFromEnv:        "unix:///var/run/docker.sock",
			dockerHostFromConfigFile: "invalid",
			expectedBind:             "/var/run/docker.sock:/var/run/docker.sock",
		},
		{
			name:                     "Docker host from env, Docker host from config file",
			dockerHostFromEnv:        "unix:///var/run/docker.sock.1",
			dockerHostFromConfigFile: "unix:///var/run/docker.sock.1",
			expectedBind:             "/var/run/docker.sock.1:/var/run/docker.sock.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("DOCKER_HOST", tc.dockerHostFromEnv)
			defer os.Unsetenv("DOCKER_HOST")

			bind := getDockerSocketBind(map[string]string{"DOCKER_HOST": tc.dockerHostFromConfigFile})
			assert.Equal(t, tc.expectedBind, bind)
		})
	}
}

func TestGetHostConfigExternal(t *testing.T) {
	os.Setenv(config.ExternalEnvVar, "true")
	defer os.Unsetenv(config.ExternalEnvVar)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return(nil, errors.New("not found")).AnyTimes()
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return(nil, errors.New("test error")).AnyTimes()

	credsBind := "/root/.aws:/rotatingcreds:ro"

	client := &client{
		fs: mockFS,
	}
	hostConfig := client.getHostConfig(map[string]string{})
	assert.Contains(t, hostConfig.Binds, credsBind)
	assert.Empty(t, hostConfig.CapAdd)

	os.Setenv(config.ExternalEnvVar, "false")
	hostConfig = client.getHostConfig(map[string]string{})
	assert.NotContains(t, hostConfig.Binds, credsBind)
	assert.NotEmpty(t, hostConfig.CapAdd)
}

func TestStartAgentWithExecBinds(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	containerID := "container id"
	isPathValid = func(path string, isDir bool) bool {
		return true
	}
	hostCapabilityExecResourcesDir := filepath.Join(hostResourcesRootDir, execCapabilityName)
	containerCapabilityExecResourcesDir := filepath.Join(containerResourcesRootDir, execCapabilityName)

	// config
	hostConfigDir := filepath.Join(hostCapabilityExecResourcesDir, execConfigRelativePath)
	containerConfigDir := filepath.Join(containerCapabilityExecResourcesDir, execConfigRelativePath)

	expectedExecBinds := []string{
		hostResourcesRootDir + ":" + containerResourcesRootDir + readOnly,
		hostConfigDir + ":" + containerConfigDir,
	}
	expectedAgentBinds += len(expectedExecBinds)

	// bind mount for the config folder is already included in expectedAgentBinds since it's always added
	expectedExecBinds = append(expectedExecBinds, hostConfigDir+":"+containerConfigDir)
	defer func() {
		expectedAgentBinds = expectedAgentBindsUnspecifiedPlatform
		isPathValid = defaultIsPathValid
	}()

	mockFS := NewMockfileSystem(mockCtrl)
	mockDocker := NewMockdockerclient(mockCtrl)

	mockFS.EXPECT().ReadFile(config.InstanceConfigFile()).Return(nil, errors.New("not found")).AnyTimes()
	mockFS.EXPECT().ReadFile(config.AgentConfigFile()).Return(nil, errors.New("not found")).AnyTimes()
	mockDocker.EXPECT().CreateContainer(gomock.Any()).Do(func(opts godocker.CreateContainerOptions) {
		validateCommonCreateContainerOptions(opts, t)

		// verify that exec binds are added
		assert.Subset(t, opts.HostConfig.Binds, expectedExecBinds)
	}).Return(&godocker.Container{
		ID: containerID,
	}, nil)
	mockDocker.EXPECT().StartContainer(containerID, nil)
	mockDocker.EXPECT().WaitContainer(containerID)

	client := &client{
		docker: mockDocker,
		fs:     mockFS,
	}

	_, err := client.StartAgent()
	assert.NoError(t, err)
}

func TestGetCapabilityExecBinds(t *testing.T) {
	defer func() {
		isPathValid = defaultIsPathValid
	}()
	hostCapabilityExecResourcesDir := filepath.Join(hostResourcesRootDir, execCapabilityName)
	containerCapabilityExecResourcesDir := filepath.Join(containerResourcesRootDir, execCapabilityName)

	// config
	hostConfigDir := filepath.Join(hostCapabilityExecResourcesDir, execConfigRelativePath)
	containerConfigDir := filepath.Join(containerCapabilityExecResourcesDir, execConfigRelativePath)

	testCases := []struct {
		name            string
		testIsPathValid func(string, bool) bool
		expectedBinds   []string
	}{
		{
			name: "all paths valid",
			testIsPathValid: func(path string, isDir bool) bool {
				return true
			},
			expectedBinds: []string{
				hostResourcesRootDir + ":" + containerResourcesRootDir + readOnly,
				hostConfigDir + ":" + containerConfigDir,
			},
		},
		{
			name: "managed-agents path valid, no execute-command",
			testIsPathValid: func(path string, isDir bool) bool {
				return path == hostResourcesRootDir
			},
			expectedBinds: []string{
				hostResourcesRootDir + ":" + containerResourcesRootDir + readOnly,
			},
		},
		{
			name: "no path valid",
			testIsPathValid: func(path string, isDir bool) bool {
				return false
			},
			expectedBinds: []string{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isPathValid = tc.testIsPathValid
			binds := getCapabilityBinds()
			assert.Equal(t, tc.expectedBinds, binds)
		})
	}
}

func TestDefaultIsPathValid(t *testing.T) {
	rootDir := t.TempDir()

	file, err := os.CreateTemp(rootDir, "file")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		assert.NoError(t, file.Close())
	})
	notExistingPath := filepath.Join(rootDir, "not-existing")
	testCases := []struct {
		name              string
		path              string
		shouldBeDirectory bool
		expected          bool
	}{
		{
			name:              "return false if directory does not exist",
			path:              notExistingPath,
			shouldBeDirectory: true,
			expected:          false,
		},
		{
			name:              "return false if false does not exist",
			path:              notExistingPath,
			shouldBeDirectory: false,
			expected:          false,
		},
		{
			name:              "if directory exists, return shouldBeDirectory",
			path:              rootDir,
			shouldBeDirectory: true,
			expected:          true,
		},
		{
			name:              "if directory exists, return shouldBeDirectory",
			path:              rootDir,
			shouldBeDirectory: false,
			expected:          false,
		},
		{
			name:              "if file exists, return !shouldBeDirectory",
			path:              file.Name(),
			shouldBeDirectory: false,
			expected:          true,
		},
		{
			name:              "if file exists, return !shouldBeDirectory",
			path:              file.Name(),
			shouldBeDirectory: true,
			expected:          false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := defaultIsPathValid(tc.path, tc.shouldBeDirectory)
			assert.Equal(t, result, tc.expected)
		})
	}
}

func TestGetCredentialsFetcherSocketBind(t *testing.T) {
	testCases := []struct {
		name                                 string
		credentialsFetcherHostFromEnv        string
		credentialsFetcherHostFromConfigFile string
		expectedBind                         string
	}{
		{
			name:                                 "No Credentials Fetcher host from env",
			credentialsFetcherHostFromEnv:        "",
			credentialsFetcherHostFromConfigFile: "dummy",
			expectedBind:                         "/var/credentials-fetcher/socket/credentials_fetcher.sock:/var/credentials-fetcher/socket/credentials_fetcher.sock",
		},
		{
			name:                                 "Invalid Credentials Fetcher host from env",
			credentialsFetcherHostFromEnv:        "invalid",
			credentialsFetcherHostFromConfigFile: "dummy",
			expectedBind:                         "/var/credentials-fetcher/socket/credentials_fetcher.sock:/var/credentials-fetcher/socket/credentials_fetcher.sock",
		},
		{
			name:                                 "Credentials Fetcher from env, no Credentials Fetcher from config file",
			credentialsFetcherHostFromEnv:        "unix:///var/credentials-fetcher/socket/credentials_fetcher.sock",
			credentialsFetcherHostFromConfigFile: "",
			expectedBind:                         "/var/credentials-fetcher/socket/credentials_fetcher.sock:/var/credentials-fetcher/socket/credentials_fetcher.sock",
		},
		{
			name:                                 "Credentials Fetcher from env, invalid Credentials Fetcher from config file",
			credentialsFetcherHostFromEnv:        "unix:///var/credentials-fetcher/socket/credentials_fetcher.sock",
			credentialsFetcherHostFromConfigFile: "invalid",
			expectedBind:                         "/var/credentials-fetcher/socket/credentials_fetcher.sock:/var/credentials-fetcher/socket/credentials_fetcher.sock",
		},
		{
			name:                                 "Credentials Fetcher host from env, Credentials Fetcher from config file",
			credentialsFetcherHostFromEnv:        "unix:///var/credentials-fetcher/socket/credentials_fetcher.sock.1",
			credentialsFetcherHostFromConfigFile: "unix:///var/credentials-fetcher/socket/credentials_fetcher.sock.1",
			expectedBind:                         "/var/credentials-fetcher/socket/credentials_fetcher.sock.1:/var/credentials-fetcher/socket/credentials_fetcher.sock.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("CREDENTIALS_FETCHER_HOST", tc.credentialsFetcherHostFromEnv)
			defer os.Unsetenv("CREDENTIALS_FETCHER_HOST")

			bind, _ := getCredentialsFetcherSocketBind()
			assert.Equal(t, tc.expectedBind, bind)
		})
	}
}

func TestInstanceDomainJoined(t *testing.T) {
	execCommand = fakeExecCommand
	execLookPath = func(name string) (string, error) {
		return "/usr/sbin/dummy", nil
	}

	defer func() {
		execLookPath = exec.LookPath
		execCommand = exec.Command
	}()

	out := isDomainJoined()
	assert.True(t, out)
}

func TestInstanceDomainJoinedRealmNotFound(t *testing.T) {
	execCommand = fakeExecCommand
	execLookPath = func(name string) (string, error) {
		return "", nil
	}

	defer func() {
		execLookPath = exec.LookPath
		execCommand = exec.Command
	}()

	out := isDomainJoined()
	assert.False(t, out)
}

func TestExecHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	mockRealmList := "contoso.com\n  type: kerberos\n  realm-name: CONTOSO.COM\n  domain-name: contoso.com\n  configured: kerberos-member\n  server-software: active-directory\n  client-software: sssd\n  required-package: oddjob\n  required-package: oddjob-mkhomedir\n  required-package: sssd\n  required-package: adcli\n  required-package: samba-common-tools\n  login-formats: %U@contoso.com\n  login-policy: allow-realm-logins"
	fmt.Fprintf(os.Stdout, mockRealmList)
	os.Exit(0)
}

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestExecHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}
