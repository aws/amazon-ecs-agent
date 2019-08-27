// +build linux,unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package task

import (
	"encoding/json"
	"testing"
	"time"

	apiappmesh "github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmsecret"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control/mock_control"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/ssmsecret"
	mock_ioutilwrapper "github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper/mocks"
	"github.com/golang/mock/gomock"

	"github.com/aws/aws-sdk-go/aws"
	dockercontainer "github.com/docker/docker/api/types/container"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	validTaskArn   = "arn:aws:ecs:region:account-id:task/task-id"
	invalidTaskArn = "invalid:task::arn"

	expectedCgroupRoot = "/ecs/task-id"

	taskVCPULimit             = 2.0
	taskMemoryLimit           = 512
	minDockerClientAPIVersion = dockerclient.Version_1_17

	proxyName = "envoy"

	testCluster        = "testCluster"
	testDataDir        = "testDataDir"
	testDataDirOnHost  = "testDataDirOnHost"
	testInstanceID     = "testInstanceID"
	testTaskDefFamily  = "testFamily"
	testTaskDefVersion = "1"
)

func TestAddNetworkResourceProvisioningDependencyNop(t *testing.T) {
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
			},
		},
	}
	testTask.addNetworkResourceProvisioningDependency(nil)
	assert.Equal(t, 1, len(testTask.Containers))
}

func TestAddNetworkResourceProvisioningDependencyWithENI(t *testing.T) {
	testTask := &Task{
		ENIs: []*apieni.ENI{{}},
		Containers: []*apicontainer.Container{
			{
				Name:                      "c1",
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}
	cfg := &config.Config{
		PauseContainerImageName: "pause-container-image-name",
		PauseContainerTag:       "pause-container-tag",
	}
	testTask.addNetworkResourceProvisioningDependency(cfg)
	assert.Equal(t, 2, len(testTask.Containers),
		"addNetworkResourceProvisioningDependency should add another container")
	pauseContainer, ok := testTask.ContainerByName(NetworkPauseContainerName)
	require.True(t, ok, "Expected to find pause container")
	assert.Equal(t, apicontainer.ContainerCNIPause, pauseContainer.Type, "pause container should have correct type")
	assert.True(t, pauseContainer.Essential, "pause container should be essential")
	assert.Equal(t, cfg.PauseContainerImageName+":"+cfg.PauseContainerTag, pauseContainer.Image,
		"pause container should use configured image")
}

func TestAddNetworkResourceProvisioningDependencyWithAppMesh(t *testing.T) {
	pauseConfig := dockercontainer.Config{
		User: "1337:35",
	}

	bytes, _ := json.Marshal(pauseConfig)
	serializedConfig := string(bytes)

	testTask := &Task{
		AppMesh: &apiappmesh.AppMesh{
			ContainerName: proxyName,
		},
		ENIs: []*apieni.ENI{{}},
		Containers: []*apicontainer.Container{
			{
				Name:                      "c1",
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
			{
				Name: proxyName,
				DockerConfig: apicontainer.DockerConfig{
					Config: &serializedConfig,
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}
	cfg := &config.Config{
		PauseContainerImageName: "pause-container-image-name",
		PauseContainerTag:       "pause-container-tag",
	}

	testTask.addNetworkResourceProvisioningDependency(cfg)
	assert.Equal(t, 3, len(testTask.Containers),
		"addNetworkResourceProvisioningDependency should add another container")
	pauseContainer, ok := testTask.ContainerByName(NetworkPauseContainerName)
	require.True(t, ok, "Expected to find pause container")
	containerConfig := &dockercontainer.Config{}
	json.Unmarshal([]byte(aws.StringValue(pauseContainer.DockerConfig.Config)), &containerConfig)
	assert.Equal(t, "1337:35", containerConfig.User, "pause container should have correct user")
	assert.Equal(t, apicontainer.ContainerCNIPause, pauseContainer.Type, "pause container should have correct type")
	assert.True(t, pauseContainer.Essential, "pause container should be essential")
	assert.Equal(t, cfg.PauseContainerImageName+":"+cfg.PauseContainerTag, pauseContainer.Image,
		"pause container should use configured image")
}

func TestAddNetworkResourceProvisioningDependencyWithAppMeshDefaultImage(t *testing.T) {
	pauseConfig := dockercontainer.Config{
		User: "1337:35",
	}

	bytes, _ := json.Marshal(pauseConfig)
	serializedConfig := string(bytes)

	testTask := &Task{
		AppMesh: &apiappmesh.AppMesh{
			ContainerName: proxyName,
		},
		ENIs: []*apieni.ENI{{}},
		Containers: []*apicontainer.Container{
			{
				Name:                      "c1",
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
			{
				Name: proxyName,
				DockerConfig: apicontainer.DockerConfig{
					Config: &serializedConfig,
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}
	cfg := &config.Config{
		PauseContainerImageName: "",
		PauseContainerTag:       "pause-container-tag",
	}
	testTask.addNetworkResourceProvisioningDependency(cfg)
	assert.Equal(t, 3, len(testTask.Containers),
		"addNetworkResourceProvisioningDependency should add another container")
	pauseContainer, ok := testTask.ContainerByName(NetworkPauseContainerName)
	require.True(t, ok, "Expected to find pause container")
	assert.Equal(t, apicontainer.DockerConfig{}, pauseContainer.DockerConfig, "pause container should not have user")
	assert.Equal(t, apicontainer.ContainerCNIPause, pauseContainer.Type, "pause container should have correct type")
	assert.True(t, pauseContainer.Essential, "pause container should be essential")
	assert.Equal(t, cfg.PauseContainerImageName+":"+cfg.PauseContainerTag, pauseContainer.Image,
		"pause container should use configured image")
}

func TestAddNetworkResourceProvisioningDependencyWithAppMeshError(t *testing.T) {
	testTask := &Task{
		AppMesh: &apiappmesh.AppMesh{
			ContainerName: proxyName,
		},
		ENIs: []*apieni.ENI{{}},
		Containers: []*apicontainer.Container{
			{
				Name:                      "c1",
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
			{
				Name:                      proxyName,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}
	cfg := &config.Config{
		PauseContainerImageName: "pause-container-image-name",
		PauseContainerTag:       "pause-container-tag",
	}
	err := testTask.addNetworkResourceProvisioningDependency(cfg)
	assert.Error(t, err,
		"addNetworkResourceProvisioningDependency should throw error when no user in proxy container")
}

// TestBuildCgroupRootHappyPath builds cgroup root from valid taskARN
func TestBuildCgroupRootHappyPath(t *testing.T) {
	task := Task{
		Arn: validTaskArn,
	}

	cgroupRoot, err := task.BuildCgroupRoot()

	assert.NoError(t, err)
	assert.Equal(t, expectedCgroupRoot, cgroupRoot)
}

// TestBuildCgroupRootErrorPath validates the cgroup path build error path
func TestBuildCgroupRootErrorPath(t *testing.T) {
	task := Task{
		Arn: invalidTaskArn,
	}

	cgroupRoot, err := task.BuildCgroupRoot()

	assert.Error(t, err)
	assert.Empty(t, cgroupRoot)
}

// TestBuildLinuxResourceSpecCPUMem validates the linux resource spec builder
func TestBuildLinuxResourceSpecCPUMem(t *testing.T) {
	taskMemoryLimit := int64(taskMemoryLimit)

	task := &Task{
		Arn:    validTaskArn,
		CPU:    float64(taskVCPULimit),
		Memory: taskMemoryLimit,
	}

	expectedTaskCPUPeriod := uint64(defaultCPUPeriod / time.Microsecond)
	expectedTaskCPUQuota := int64(taskVCPULimit * float64(expectedTaskCPUPeriod))
	expectedTaskMemory := taskMemoryLimit * bytesPerMegabyte
	expectedLinuxResourceSpec := specs.LinuxResources{
		CPU: &specs.LinuxCPU{
			Quota:  &expectedTaskCPUQuota,
			Period: &expectedTaskCPUPeriod,
		},
		Memory: &specs.LinuxMemory{
			Limit: &expectedTaskMemory,
		},
	}

	linuxResourceSpec, err := task.BuildLinuxResourceSpec(defaultCPUPeriod)

	assert.NoError(t, err)
	assert.EqualValues(t, expectedLinuxResourceSpec, linuxResourceSpec)
}

// TestBuildLinuxResourceSpecCPU validates the linux resource spec builder
func TestBuildLinuxResourceSpecCPU(t *testing.T) {
	task := &Task{
		Arn: validTaskArn,
		CPU: float64(taskVCPULimit),
	}

	expectedTaskCPUPeriod := uint64(defaultCPUPeriod / time.Microsecond)
	expectedTaskCPUQuota := int64(taskVCPULimit * float64(expectedTaskCPUPeriod))
	expectedLinuxResourceSpec := specs.LinuxResources{
		CPU: &specs.LinuxCPU{
			Quota:  &expectedTaskCPUQuota,
			Period: &expectedTaskCPUPeriod,
		},
	}

	linuxResourceSpec, err := task.BuildLinuxResourceSpec(defaultCPUPeriod)

	assert.NoError(t, err)
	assert.EqualValues(t, expectedLinuxResourceSpec, linuxResourceSpec)
}

// TestBuildLinuxResourceSpecWithoutTaskCPULimits validates behavior of CPU Shares
func TestBuildLinuxResourceSpecWithoutTaskCPULimits(t *testing.T) {
	task := &Task{
		Arn: validTaskArn,
		Containers: []*apicontainer.Container{
			{
				Name: "C1",
			},
		},
	}
	expectedCPUShares := uint64(minimumCPUShare)
	expectedLinuxResourceSpec := specs.LinuxResources{
		CPU: &specs.LinuxCPU{
			Shares: &expectedCPUShares,
		},
	}

	linuxResourceSpec, err := task.BuildLinuxResourceSpec(defaultCPUPeriod)

	assert.NoError(t, err)
	assert.EqualValues(t, expectedLinuxResourceSpec, linuxResourceSpec)
}

// TestBuildLinuxResourceSpecWithoutTaskCPUWithContainerCPULimits validates behavior of CPU Shares
func TestBuildLinuxResourceSpecWithoutTaskCPUWithContainerCPULimits(t *testing.T) {
	task := &Task{
		Arn: validTaskArn,
		Containers: []*apicontainer.Container{
			{
				Name: "C1",
				CPU:  uint(512),
			},
		},
	}
	expectedCPUShares := uint64(512)
	expectedLinuxResourceSpec := specs.LinuxResources{
		CPU: &specs.LinuxCPU{
			Shares: &expectedCPUShares,
		},
	}

	linuxResourceSpec, err := task.BuildLinuxResourceSpec(defaultCPUPeriod)

	assert.NoError(t, err)
	assert.EqualValues(t, expectedLinuxResourceSpec, linuxResourceSpec)
}

// TestBuildLinuxResourceSpecInvalidMem validates the linux resource spec builder
func TestBuildLinuxResourceSpecInvalidMem(t *testing.T) {
	taskMemoryLimit := int64(taskMemoryLimit)

	task := &Task{
		Arn:    validTaskArn,
		CPU:    float64(taskVCPULimit),
		Memory: taskMemoryLimit,
		Containers: []*apicontainer.Container{
			{
				Name:   "C1",
				Memory: uint(2048),
			},
		},
	}

	expectedLinuxResourceSpec := specs.LinuxResources{}
	linuxResourceSpec, err := task.BuildLinuxResourceSpec(defaultCPUPeriod)

	assert.Error(t, err)
	assert.EqualValues(t, expectedLinuxResourceSpec, linuxResourceSpec)
}

// TestOverrideCgroupParent validates the cgroup parent override
func TestOverrideCgroupParentHappyPath(t *testing.T) {
	task := &Task{
		Arn:                    validTaskArn,
		CPU:                    float64(taskVCPULimit),
		Memory:                 int64(taskMemoryLimit),
		MemoryCPULimitsEnabled: true,
	}

	hostConfig := &dockercontainer.HostConfig{}

	assert.NoError(t, task.overrideCgroupParent(hostConfig))
	assert.NotEmpty(t, hostConfig)
	assert.Equal(t, expectedCgroupRoot, hostConfig.CgroupParent)
}

// TestOverrideCgroupParentErrorPath validates the error path for
// cgroup parent update
func TestOverrideCgroupParentErrorPath(t *testing.T) {
	task := &Task{
		Arn:                    invalidTaskArn,
		CPU:                    float64(taskVCPULimit),
		Memory:                 int64(taskMemoryLimit),
		MemoryCPULimitsEnabled: true,
	}

	hostConfig := &dockercontainer.HostConfig{}

	assert.Error(t, task.overrideCgroupParent(hostConfig))
	assert.Empty(t, hostConfig.CgroupParent)
}

// TestPlatformHostConfigOverride validates the platform host config overrides
func TestPlatformHostConfigOverride(t *testing.T) {
	task := &Task{
		Arn:                    validTaskArn,
		CPU:                    float64(taskVCPULimit),
		Memory:                 int64(taskMemoryLimit),
		MemoryCPULimitsEnabled: true,
	}

	hostConfig := &dockercontainer.HostConfig{}

	assert.NoError(t, task.platformHostConfigOverride(hostConfig))
	assert.NotEmpty(t, hostConfig)
	assert.Equal(t, expectedCgroupRoot, hostConfig.CgroupParent)
}

// TestPlatformHostConfigOverride validates the platform host config overrides
func TestPlatformHostConfigOverrideErrorPath(t *testing.T) {
	task := &Task{
		Arn:                    invalidTaskArn,
		CPU:                    float64(taskVCPULimit),
		Memory:                 int64(taskMemoryLimit),
		MemoryCPULimitsEnabled: true,
		Containers: []*apicontainer.Container{
			{
				Name: "c1",
			},
		},
	}

	dockerHostConfig, err := task.DockerHostConfig(task.Containers[0], dockerMap(task), defaultDockerClientAPIVersion)
	assert.Error(t, err)
	assert.Empty(t, dockerHostConfig)
}

func TestDockerHostConfigRawConfigMerging(t *testing.T) {
	// Use a struct that will marshal to the actual message we expect; not
	// dockercontainer.HostConfig which will include a lot of zero values.
	rawHostConfigInput := struct {
		Privileged  bool     `json:"Privileged,omitempty" yaml:"Privileged,omitempty"`
		SecurityOpt []string `json:"SecurityOpt,omitempty" yaml:"SecurityOpt,omitempty"`
	}{
		Privileged:  true,
		SecurityOpt: []string{"foo", "bar"},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	if err != nil {
		t.Fatal(err)
	}

	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name:        "c1",
				Image:       "image",
				CPU:         50,
				Memory:      100,
				VolumesFrom: []apicontainer.VolumeFrom{{SourceContainer: "c2"}},
				DockerConfig: apicontainer.DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
			},
			{
				Name: "c2",
			},
		},
	}

	hostConfig, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), minDockerClientAPIVersion)
	assert.Nil(t, configErr)

	expected := dockercontainer.HostConfig{
		Privileged:  true,
		SecurityOpt: []string{"foo", "bar"},
		VolumesFrom: []string{"dockername-c2"},
		Resources: dockercontainer.Resources{
			// Convert MB to B and set Memory
			Memory:     int64(100 * 1024 * 1024),
			CPUShares:  50,
			CPUPercent: minimumCPUPercent,
		},
	}
	assertSetStructFieldsEqual(t, expected, *hostConfig)
}

func TestInitCgroupResourceSpecHappyPath(t *testing.T) {
	taskMemoryLimit := int64(taskMemoryLimit)
	task := &Task{
		Arn:    validTaskArn,
		CPU:    float64(taskVCPULimit),
		Memory: taskMemoryLimit,
		Containers: []*apicontainer.Container{
			{
				Name:                      "c1",
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		MemoryCPULimitsEnabled: true,
		ResourcesMapUnsafe:     make(map[string][]taskresource.TaskResource),
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockControl := mock_control.NewMockControl(ctrl)
	mockIO := mock_ioutilwrapper.NewMockIOUtil(ctrl)
	assert.NoError(t, task.initializeCgroupResourceSpec("cgroupPath", defaultCPUPeriod, &taskresource.ResourceFields{
		Control: mockControl,
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			IOUtil: mockIO,
		},
	}))
	assert.Equal(t, 1, len(task.GetResources()))
	assert.Equal(t, 1, len(task.Containers[0].TransitionDependenciesMap))
}

func TestInitCgroupResourceSpecInvalidARN(t *testing.T) {
	task := &Task{
		Arn:     "arn", // malformed arn
		Family:  "testFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name:                      "c1",
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		MemoryCPULimitsEnabled: true,
		ResourcesMapUnsafe:     make(map[string][]taskresource.TaskResource),
	}
	assert.Error(t, task.initializeCgroupResourceSpec("", time.Millisecond, nil))
	assert.Equal(t, 0, len(task.GetResources()))
	assert.Equal(t, 0, len(task.Containers[0].TransitionDependenciesMap))
}

func TestInitCgroupResourceSpecInvalidMem(t *testing.T) {
	taskMemoryLimit := int64(taskMemoryLimit)
	task := &Task{
		Arn:    validTaskArn,
		CPU:    float64(taskVCPULimit),
		Memory: taskMemoryLimit,
		Containers: []*apicontainer.Container{
			{
				Name:                      "C1",
				Memory:                    uint(2048), // container memory > task memory
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		MemoryCPULimitsEnabled: true,
		ResourcesMapUnsafe:     make(map[string][]taskresource.TaskResource),
	}
	assert.Error(t, task.initializeCgroupResourceSpec("", time.Millisecond, nil))
	assert.Equal(t, 0, len(task.GetResources()))
	assert.Equal(t, 0, len(task.Containers[0].TransitionDependenciesMap))
}

func TestPostUnmarshalWithCPULimitsFail(t *testing.T) {
	task := &Task{
		Arn:     "arn", // malformed arn
		Family:  "testFamily",
		Version: "1",
		Containers: []*apicontainer.Container{
			{
				Name:                      "c1",
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
	}
	cfg := config.Config{
		TaskCPUMemLimit: config.ExplicitlyEnabled,
	}
	assert.Error(t, task.PostUnmarshalTask(&cfg, nil, nil, nil, nil))
	assert.Equal(t, 0, len(task.GetResources()))
	assert.Equal(t, 0, len(task.Containers[0].TransitionDependenciesMap))
}

func TestPostUnmarshalWithFirelensContainer(t *testing.T) {
	task := getFirelensTask(t)

	resourceFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			EC2InstanceID: testInstanceID,
		},
	}
	cfg := &config.Config{
		DataDir: testDataDir,
		Cluster: testCluster,
	}
	assert.NoError(t, task.PostUnmarshalTask(cfg, nil, resourceFields, nil, nil))
	resources := task.GetResources()
	assert.Len(t, resources, 2)
	assert.Len(t, task.Containers[1].TransitionDependenciesMap, 1)
	assert.Len(t, task.Containers[1].TransitionDependenciesMap[apicontainerstatus.ContainerCreated].ResourceDependencies, 2)
	var firelensResource *firelens.FirelensResource
	var secretResource *ssmsecret.SSMSecretResource
	for _, resource := range resources {
		if resource.GetName() == firelens.ResourceName {
			firelensResource = resource.(*firelens.FirelensResource)
		} else if resource.GetName() == ssmsecret.ResourceName {
			secretResource = resource.(*ssmsecret.SSMSecretResource)
		}
	}

	assert.NotNil(t, firelensResource)
	assert.NotNil(t, secretResource)

	assert.Equal(t, testCluster, firelensResource.GetCluster())
	assert.Equal(t, validTaskArn, firelensResource.GetTaskARN())
	assert.Equal(t, testTaskDefFamily+":"+testTaskDefVersion, firelensResource.GetTaskDefinition())
	assert.Equal(t, testInstanceID, firelensResource.GetEC2InstanceID())
	assert.Equal(t, testDataDir+"/firelens/task-id", firelensResource.GetResourceDir())
	assert.NotNil(t, firelensResource.GetContainerToLogOptions())
	assert.Equal(t, "value1", firelensResource.GetContainerToLogOptions()["logsender"]["key1"])
	assert.Equal(t, "value2", firelensResource.GetContainerToLogOptions()["logsender"]["key2"])
	assert.Contains(t, task.Containers[0].DependsOnUnsafe, apicontainer.DependsOn{
		ContainerName: task.Containers[1].Name,
		Condition:     ContainerOrderingStartCondition,
	})
}

func TestPostUnmarshalWithFirelensContainerError(t *testing.T) {
	task := getFirelensTask(t)
	task.Containers[0].DockerConfig.HostConfig = strptr(string("invalid"))

	resourceFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			EC2InstanceID: testInstanceID,
		},
	}
	cfg := &config.Config{
		DataDir: testDataDir,
		Cluster: testCluster,
	}
	assert.Error(t, task.PostUnmarshalTask(cfg, nil, resourceFields, nil, nil))
}

func TestGetFirelensContainer(t *testing.T) {
	firelensContainer := &apicontainer.Container{
		Name: "c",
		FirelensConfig: &apicontainer.FirelensConfig{
			Type: firelens.FirelensConfigTypeFluentd,
		},
	}

	testCases := []struct {
		name              string
		task              *Task
		firelensContainer *apicontainer.Container
	}{
		{
			name: "task has firelens container",
			task: &Task{
				Containers: []*apicontainer.Container{
					firelensContainer,
				},
			},
			firelensContainer: firelensContainer,
		},
		{
			name: "task doesn't have firelens container",
			task: &Task{
				Containers: []*apicontainer.Container{
					{
						Name: "c",
					},
				},
			},
			firelensContainer: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.firelensContainer, tc.task.getFirelensContainer())
		})
	}
}

func TestInitializeFirelensResource(t *testing.T) {
	cfg := &config.Config{
		DataDir: testDataDir,
		Cluster: testCluster,
	}
	resourceFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			EC2InstanceID: testInstanceID,
		},
	}

	testCases := []struct {
		name                  string
		task                  *Task
		shouldFail            bool
		shouldHaveInstanceID  bool
		shouldDisableMetadata bool
		expectedLogOptions    map[string]map[string]string
	}{
		{
			name:                 "test initialize firelens resource fluentd",
			task:                 getFirelensTask(t),
			shouldHaveInstanceID: true,
			expectedLogOptions: map[string]map[string]string{
				"logsender": {
					"key1":        "value1",
					"key2":        "value2",
					"secret-name": "\"#{ENV['secret-name_0']}\"",
				},
			},
		},
		{
			name: "test initialize firelens resource fluentbit",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[1].FirelensConfig.Type = firelens.FirelensConfigTypeFluentbit
				return task
			}(),
			shouldHaveInstanceID: true,
			expectedLogOptions: map[string]map[string]string{
				"logsender": {
					"key1":        "value1",
					"key2":        "value2",
					"secret-name": "${secret-name_0}",
				},
			},
		},
		{
			name: "test initialize firelens resource without ec2 instance id",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[1].Environment = nil
				return task
			}(),
			expectedLogOptions: map[string]map[string]string{
				"logsender": {
					"key1":        "value1",
					"key2":        "value2",
					"secret-name": "\"#{ENV['secret-name_0']}\"",
				},
			},
		},
		{
			name: "test initialize firelens resource disables ecs log metadata",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[1].FirelensConfig.Options["enable-ecs-log-metadata"] = "false"
				return task
			}(),
			shouldHaveInstanceID:  true,
			shouldDisableMetadata: true,
			expectedLogOptions: map[string]map[string]string{
				"logsender": {
					"key1":        "value1",
					"key2":        "value2",
					"secret-name": "\"#{ENV['secret-name_0']}\"",
				},
			},
		},
		{
			name: "test initialize firelens resource invalid host config",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[0].DockerConfig.HostConfig = strptr(string("invalid"))
				return task
			}(),
			shouldFail: true,
		},
		{
			name: "test initialize firelens resource no firelens container",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[1].FirelensConfig = nil
				return task
			}(),
			shouldFail: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.task.initializeFirelensResource(cfg, resourceFields, tc.task.Containers[1])
			if tc.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				resources := tc.task.GetResources()
				assert.Equal(t, 1, len(resources))
				assert.Equal(t, 1, len(tc.task.Containers[1].TransitionDependenciesMap))

				firelensResource := resources[0].(*firelens.FirelensResource)
				assert.Equal(t, testCluster, firelensResource.GetCluster())
				assert.Equal(t, validTaskArn, firelensResource.GetTaskARN())
				assert.Equal(t, testTaskDefFamily+":"+testTaskDefVersion, firelensResource.GetTaskDefinition())
				assert.Equal(t, testDataDir+"/firelens/task-id", firelensResource.GetResourceDir())
				assert.NotNil(t, firelensResource.GetContainerToLogOptions())
				assert.Equal(t, tc.expectedLogOptions, firelensResource.GetContainerToLogOptions())
				assert.Equal(t, !tc.shouldDisableMetadata, firelensResource.GetECSMetadataEnabled())

				if tc.shouldHaveInstanceID {
					assert.Equal(t, testInstanceID, firelensResource.GetEC2InstanceID())
				} else {
					assert.Empty(t, firelensResource.GetEC2InstanceID())
				}
			}
		})
	}
}

func TestCollectFirelensLogOptions(t *testing.T) {
	task := getFirelensTask(t)

	containerToLogOptions := make(map[string]map[string]string)
	err := task.collectFirelensLogOptions(containerToLogOptions)
	assert.NoError(t, err)
	assert.Equal(t, "value1", containerToLogOptions["logsender"]["key1"])
	assert.Equal(t, "value2", containerToLogOptions["logsender"]["key2"])
}

func TestCollectFirelensLogOptionsInvalidOptions(t *testing.T) {
	task := getFirelensTask(t)
	task.Containers[0].DockerConfig.HostConfig = strptr(string("invalid"))

	containerToLogOptions := make(map[string]map[string]string)
	err := task.collectFirelensLogOptions(containerToLogOptions)
	assert.Error(t, err)
}

func TestCollectFirelensLogEnvOptions(t *testing.T) {
	task := getFirelensTask(t)

	containerToLogOptions := make(map[string]map[string]string)
	err := task.collectFirelensLogEnvOptions(containerToLogOptions, "fluentd")
	assert.NoError(t, err)
	assert.Equal(t, "\"#{ENV['secret-name_0']}\"", containerToLogOptions["logsender"]["secret-name"])
}

func TestAddFirelensContainerDependency(t *testing.T) {
	testCases := []struct {
		name                string
		task                *Task
		shouldAddDependency bool
	}{
		{
			name:                "test adding firelens container dependency",
			task:                getFirelensTask(t),
			shouldAddDependency: true,
		},
		{
			name: "test not adding firelens container dependency case 1",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[0].FirelensConfig = task.Containers[1].FirelensConfig
				task.Containers = task.Containers[:1]
				return task
			}(),
			shouldAddDependency: false,
		},
		{
			name: "test not adding firelens container dependency case 2",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers = append(task.Containers, &apicontainer.Container{
					Name: "container2",
				})
				task.Containers[1].DependsOnUnsafe = append(task.Containers[1].DependsOnUnsafe, apicontainer.DependsOn{
					ContainerName: "container2",
					Condition:     ContainerOrderingStartCondition,
				})
				return task
			}(),
			shouldAddDependency: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.task.addFirelensContainerDependency()
			assert.NoError(t, err)

			if tc.shouldAddDependency {
				assert.Equal(t, 1, len(tc.task.Containers[0].DependsOnUnsafe))
				assert.Equal(t, tc.task.Containers[1].Name, tc.task.Containers[0].DependsOnUnsafe[0].ContainerName)
				assert.Equal(t, ContainerOrderingStartCondition, tc.task.Containers[0].DependsOnUnsafe[0].Condition)
			} else {
				assert.Empty(t, tc.task.Containers[0].DependsOnUnsafe)
			}
		})
	}
}

func TestAddFirelensContainerBindMounts(t *testing.T) {
	cfg := &config.Config{
		DataDirOnHost: testDataDirOnHost,
	}

	testCases := []struct {
		name               string
		task               *Task
		firelensConfigType string
		hostCfg            *dockercontainer.HostConfig
		cfg                *config.Config
		shouldFail         bool
		expectedBindMounts []string
	}{
		{
			name:               "test add bind mounts for fluentd firelens container",
			task:               getFirelensTask(t),
			firelensConfigType: firelens.FirelensConfigTypeFluentd,
			hostCfg:            &dockercontainer.HostConfig{},
			cfg:                cfg,
			shouldFail:         false,
			expectedBindMounts: []string{
				"testDataDirOnHost/data/firelens/task-id/config/fluent.conf:/fluentd/etc/fluent.conf",
				"testDataDirOnHost/data/firelens/task-id/socket/:/var/run/",
			},
		},
		{
			name: "test add bind mounts for fluentbit firelens container",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[1].FirelensConfig.Type = firelens.FirelensConfigTypeFluentbit
				return task
			}(),
			firelensConfigType: firelens.FirelensConfigTypeFluentbit,
			hostCfg:            &dockercontainer.HostConfig{},
			cfg:                cfg,
			shouldFail:         false,
			expectedBindMounts: []string{
				"testDataDirOnHost/data/firelens/task-id/config/fluent.conf:/fluent-bit/etc/fluent-bit.conf",
				"testDataDirOnHost/data/firelens/task-id/socket/:/var/run/",
			},
		},
		{
			name: "test add bind mounts invalid firelens configuration type",
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[1].FirelensConfig.Type = "invalid"
				return task
			}(),
			firelensConfigType: "invalid",
			hostCfg:            &dockercontainer.HostConfig{},
			cfg:                cfg,
			shouldFail:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.task.AddFirelensContainerBindMounts(tc.firelensConfigType, tc.hostCfg, tc.cfg)
			if tc.shouldFail {
				// assert.Error doesn't work with *apierrors.HostConfigError.
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedBindMounts, tc.hostCfg.Binds)
			}
		})
	}
}

func TestFirelensDependsOnSecretResource(t *testing.T) {
	testCases := []struct {
		name     string
		provider string
		task     *Task
		res      bool
	}{
		{
			name:     "depends on ssm",
			provider: apicontainer.SecretProviderSSM,
			task:     getFirelensTask(t),
			res:      true,
		},
		{
			name:     "depends on asm",
			provider: apicontainer.SecretProviderASM,
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[0].Secrets[0].Provider = apicontainer.SecretProviderASM
				return task
			}(),
			res: true,
		},
		{
			name:     "no dependency",
			provider: apicontainer.SecretProviderSSM,
			task: func() *Task {
				task := getFirelensTask(t)
				task.Containers[0].Secrets = []apicontainer.Secret{}
				return task
			}(),
			res: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.res, tc.task.firelensDependsOnSecretResource(tc.provider))
		})
	}
}

func TestPopulateSecretLogOptionsToFirelensContainer(t *testing.T) {
	task := getFirelensTask(t)
	ssmRes := &ssmsecret.SSMSecretResource{}
	ssmRes.SetCachedSecretValue("secret-value-from_us-west-2", "secret-val")
	task.AddResource(ssmsecret.ResourceName, ssmRes)

	assert.Nil(t, task.PopulateSecretLogOptionsToFirelensContainer(task.Containers[1]))
	assert.Len(t, task.Containers[1].Environment, 2)
	assert.Equal(t, "secret-val", task.Containers[1].Environment["secret-name_0"])
}

func TestCollectLogDriverSecretData(t *testing.T) {
	ssmRes := &ssmsecret.SSMSecretResource{}
	ssmRes.SetCachedSecretValue("secret-value-from_us-west-2", "secret-val")

	asmRes := &asmsecret.ASMSecretResource{}
	asmRes.SetCachedSecretValue("secret-value-from-asm_us-west-2", "secret-val-asm")

	secrets := []apicontainer.Secret{
		{
			Name:      "secret-name",
			Provider:  apicontainer.SecretProviderSSM,
			Target:    apicontainer.SecretTargetLogDriver,
			ValueFrom: "secret-value-from",
			Region:    "us-west-2",
		},
		{
			Name:      "secret-name-asm",
			Provider:  apicontainer.SecretProviderASM,
			Target:    apicontainer.SecretTargetLogDriver,
			ValueFrom: "secret-value-from-asm",
			Region:    "us-west-2",
		},
	}

	secretData, err := collectLogDriverSecretData(secrets, ssmRes, asmRes)
	assert.NoError(t, err)
	assert.Len(t, secretData, 2)
	assert.Equal(t, "secret-val", secretData["secret-name"])
	assert.Equal(t, "secret-val-asm", secretData["secret-name-asm"])
}

// getFirelensTask returns a sample firelens task.
func getFirelensTask(t *testing.T) *Task {
	rawHostConfigInput := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
			Type: firelensDriverName,
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	rawHostConfig, err := json.Marshal(&rawHostConfigInput)
	require.NoError(t, err)

	return &Task{
		Arn:                validTaskArn,
		Family:             testTaskDefFamily,
		Version:            testTaskDefVersion,
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Containers: []*apicontainer.Container{
			{
				Name: "logsender",
				DockerConfig: apicontainer.DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
				Secrets: []apicontainer.Secret{
					{
						Name:      "secret-name",
						ValueFrom: "secret-value-from",
						Region:    "us-west-2",

						Target:   apicontainer.SecretTargetLogDriver,
						Provider: apicontainer.SecretProviderSSM,
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
			{
				Name: "firelenscontainer",
				FirelensConfig: &apicontainer.FirelensConfig{
					Type: firelens.FirelensConfigTypeFluentd,
					Options: map[string]string{
						"enable-ecs-log-metadata": "true",
					},
				},
				Environment: map[string]string{
					"AWS_EXECUTION_ENV": "AWS_ECS_EC2",
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}
}
