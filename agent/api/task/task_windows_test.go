// +build windows,unit

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

package task

import (
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/golang/mock/gomock"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"

	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mock_s3_factory "github.com/aws/amazon-ecs-agent/agent/s3/factory/mocks"
	mock_ssm_factory "github.com/aws/amazon-ecs-agent/agent/ssm/factory/mocks"
)

const (
	minDockerClientAPIVersion = dockerclient.Version_1_24

	nonZeroMemoryReservationValue  = 1
	expectedMemoryReservationValue = 0
)

func TestPostUnmarshalWindowsCanonicalPaths(t *testing.T) {
	// Testing type conversions, bleh. At least the type conversion itself
	// doesn't look this messy.
	taskFromAcs := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			{
				Name: strptr("myName"),
				MountPoints: []*ecsacs.MountPoint{
					{
						ContainerPath: strptr(`C:/Container/Path`),
						SourceVolume:  strptr("sourceVolume"),
					},
				},
			},
		},
		Volumes: []*ecsacs.Volume{
			{
				Name: strptr("sourceVolume"),
				Type: strptr("host"),
				Host: &ecsacs.HostVolumeProperties{
					SourcePath: strptr(`C:/Host/path`),
				},
			},
		},
	}
	expectedTask := &Task{
		Arn:                 "myArn",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Family:              "myFamily",
		Version:             "1",
		Containers: []*apicontainer.Container{
			{
				Name: "myName",
				MountPoints: []apicontainer.MountPoint{
					{
						ContainerPath: `c:\container\path`,
						SourceVolume:  "sourceVolume",
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
		Volumes: []TaskVolume{
			{
				Name: "sourceVolume",
				Type: "host",
				Volume: &taskresourcevolume.FSHostVolume{
					FSSourcePath: `c:\host\path`,
				},
			},
		},
		StartSequenceNumber: 42,
	}

	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromAcs, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	cfg := config.Config{TaskCPUMemLimit: config.ExplicitlyDisabled}
	task.PostUnmarshalTask(&cfg, nil, nil, nil, nil)

	for _, container := range task.Containers { // remove v3 endpoint from each container because it's randomly generated
		removeV3andV4EndpointConfig(container)
	}
	assert.Equal(t, expectedTask.Containers, task.Containers, "Containers should be equal")
	assert.Equal(t, expectedTask.Volumes, task.Volumes, "Volumes should be equal")
}

// removeV3EndpointConfig removes the v3 endpoint id and the injected env for a container
// so that checking all other fields can be easier
func removeV3andV4EndpointConfig(container *apicontainer.Container) {
	container.SetV3EndpointID("")
	if container.Environment != nil {
		delete(container.Environment, apicontainer.MetadataURIEnvironmentVariableName)
		delete(container.Environment, apicontainer.MetadataURIEnvVarNameV4)
	}
	if len(container.Environment) == 0 {
		container.Environment = nil
	}
}

func TestWindowsPlatformHostConfigOverride(t *testing.T) {
	// Testing Windows platform override for HostConfig.
	// Expects MemorySwappiness option to be set to -1

	task := &Task{}

	hostConfig := &dockercontainer.HostConfig{Resources: dockercontainer.Resources{CPUShares: int64(1 * cpuSharesPerCore)}}

	task.platformHostConfigOverride(hostConfig)
	assert.Equal(t, int64(1*cpuSharesPerCore*percentageFactor)/int64(cpuShareScaleFactor), hostConfig.CPUPercent)
	assert.Equal(t, int64(0), hostConfig.CPUShares)

	hostConfig = &dockercontainer.HostConfig{Resources: dockercontainer.Resources{CPUShares: 10}}
	task.platformHostConfigOverride(hostConfig)
	assert.Equal(t, int64(minimumCPUPercent), hostConfig.CPUPercent)
	assert.Empty(t, hostConfig.CPUShares)
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
				CPU:         10,
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

	hostConfig, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask),
		minDockerClientAPIVersion, &config.Config{})
	assert.Nil(t, configErr)

	expected := dockercontainer.HostConfig{
		Resources: dockercontainer.Resources{
			// Convert MB to B and set Memory
			Memory:     apicontainer.DockerContainerMinimumMemoryInBytes,
			CPUPercent: minimumCPUPercent,
		},
		Privileged:  true,
		SecurityOpt: []string{"foo", "bar"},
		VolumesFrom: []string{"dockername-c2"},
	}

	assert.Nil(t, expected.MemorySwappiness, "Expected default memorySwappiness to be nil")
	assertSetStructFieldsEqual(t, expected, *hostConfig)
}

func TestCPUPercentBasedOnUnboundedEnabled(t *testing.T) {
	cpuShareScaleFactor := runtime.NumCPU() * cpuSharesPerCore
	testcases := []struct {
		cpu          int64
		cpuUnbounded bool
		cpuPercent   int64
	}{
		{
			cpu:          0,
			cpuUnbounded: true,
			cpuPercent:   0,
		},
		{
			cpu:          1,
			cpuUnbounded: true,
			cpuPercent:   1,
		},
		{
			cpu:          0,
			cpuUnbounded: false,
			cpuPercent:   1,
		},
		{
			cpu:          1,
			cpuUnbounded: false,
			cpuPercent:   1,
		},
		{
			cpu:          100,
			cpuUnbounded: true,
			cpuPercent:   100 * percentageFactor / int64(cpuShareScaleFactor),
		},
		{
			cpu:          100,
			cpuUnbounded: false,
			cpuPercent:   100 * percentageFactor / int64(cpuShareScaleFactor),
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("container cpu-%d,cpu unbounded tasks enabled- %t,expected cpu percent-%d",
			tc.cpu, tc.cpuUnbounded, tc.cpuPercent), func(t *testing.T) {
			testTask := &Task{
				Containers: []*apicontainer.Container{
					{
						Name: "c1",
						CPU:  uint(tc.cpu),
					},
				},
				PlatformFields: PlatformFields{
					CpuUnbounded: tc.cpuUnbounded,
				},
			}

			hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask),
				minDockerClientAPIVersion, &config.Config{})
			assert.Nil(t, err)
			assert.Empty(t, hostconfig.CPUShares)
			assert.Equal(t, tc.cpuPercent, hostconfig.CPUPercent)
		})
	}
}

func TestWindowsMemoryReservationOption(t *testing.T) {
	// Testing sending a task to windows overriding MemoryReservation value
	rawHostConfigInput := dockercontainer.HostConfig{
		Resources: dockercontainer.Resources{
			MemoryReservation: nonZeroMemoryReservationValue,
		},
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
				Name: "c1",
				DockerConfig: apicontainer.DockerConfig{
					HostConfig: strptr(string(rawHostConfig)),
				},
			},
		},
		PlatformFields: PlatformFields{
			MemoryUnbounded: false,
		},
	}

	// With MemoryUnbounded set to false, MemoryReservation is not overridden
	cfg, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask),
		defaultDockerClientAPIVersion, &config.Config{})

	assert.Nil(t, configErr)
	assert.EqualValues(t, nonZeroMemoryReservationValue, cfg.MemoryReservation)

	// With MemoryUnbounded set to true, tasks with no memory hard limit will have their memory reservation set to zero
	testTask.PlatformFields.MemoryUnbounded = true
	cfg, configErr = testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask),
		defaultDockerClientAPIVersion, &config.Config{})

	assert.Nil(t, configErr)
	assert.EqualValues(t, expectedMemoryReservationValue, cfg.MemoryReservation)
}

func TestGetCanonicalPath(t *testing.T) {
	testcases := []struct {
		name           string
		path           string
		expectedResult string
	}{
		{
			name:           "folderPath",
			path:           `C:\myFile`,
			expectedResult: `c:\myfile`,
		},
		{
			name:           "drivePath",
			path:           `D:`,
			expectedResult: `d:`,
		},
		{
			name:           "pipePath",
			path:           `\\.\pipe\docker_engine`,
			expectedResult: `\\.\pipe\docker_engine`,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			result := getCanonicalPath(tc.path)
			assert.Equal(t, result, tc.expectedResult)
		})
	}
}

func TestRequiresCredentialSpecResource(t *testing.T) {
	container1 := &apicontainer.Container{}
	task1 := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{container1},
	}

	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	container2 := &apicontainer.Container{}
	container2.DockerConfig.HostConfig = &hostConfig
	task2 := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{container2},
	}

	testCases := []struct {
		name           string
		task           *Task
		expectedOutput bool
	}{
		{
			name:           "missing_credentialspec",
			task:           task1,
			expectedOutput: false,
		},
		{
			name:           "valid_credentialspec",
			task:           task2,
			expectedOutput: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, tc.task.requiresCredentialSpecResource())
		})
	}

}

func TestGetAllCredentialSpecRequirements(t *testing.T) {
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	container := &apicontainer.Container{}
	container.DockerConfig.HostConfig = &hostConfig

	task := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{container},
	}

	allCredSpecReq := task.getAllCredentialSpecRequirements()

	credentialspec := "credentialspec:file://gmsa_gmsa-acct.json"
	expectedCredSpecReq := []string{credentialspec}

	assert.EqualValues(t, expectedCredSpecReq, allCredSpecReq)
}

func TestGetAllCredentialSpecRequirementsWithMultipleContainersUsingSameSpec(t *testing.T) {
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	c1 := &apicontainer.Container{}
	c1.DockerConfig.HostConfig = &hostConfig

	c2 := &apicontainer.Container{}
	c2.DockerConfig.HostConfig = &hostConfig

	task := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{c1, c2},
	}

	allCredSpecReq := task.getAllCredentialSpecRequirements()

	credentialspec := "credentialspec:file://gmsa_gmsa-acct.json"
	expectedCredSpecReq := []string{credentialspec}

	assert.Equal(t, len(expectedCredSpecReq), len(allCredSpecReq))
	assert.EqualValues(t, expectedCredSpecReq, allCredSpecReq)
}

func TestGetAllCredentialSpecRequirementsWithMultipleContainers(t *testing.T) {
	hostConfig1 := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct-1.json\"]}"
	hostConfig2 := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct-2.json\"]}"

	c1 := &apicontainer.Container{}
	c1.DockerConfig.HostConfig = &hostConfig1

	c2 := &apicontainer.Container{}
	c2.DockerConfig.HostConfig = &hostConfig1

	c3 := &apicontainer.Container{}
	c3.DockerConfig.HostConfig = &hostConfig2

	task := &Task{
		Arn:        "test",
		Containers: []*apicontainer.Container{c1, c2, c3},
	}

	allCredSpecReq := task.getAllCredentialSpecRequirements()

	credentialspec1 := "credentialspec:file://gmsa_gmsa-acct-1.json"
	credentialspec2 := "credentialspec:file://gmsa_gmsa-acct-2.json"

	expectedCredSpecReq := []string{credentialspec1, credentialspec2}

	assert.EqualValues(t, expectedCredSpecReq, allCredSpecReq)
}

func TestInitializeAndGetCredentialSpecResource(t *testing.T) {
	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://gmsa_gmsa-acct.json\"]}"
	container := &apicontainer.Container{
		Name:                      "myName",
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}
	container.DockerConfig.HostConfig = &hostConfig

	task := &Task{
		Arn:                "test",
		Containers:         []*apicontainer.Container{container},
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &config.Config{
		AWSRegion: "test-aws-region",
	}

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)
	s3ClientCreator := mock_s3_factory.NewMockS3ClientCreator(ctrl)

	resFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			CredentialsManager: credentialsManager,
		},
		S3ClientCreator: s3ClientCreator,
	}

	task.initializeCredentialSpecResource(cfg, credentialsManager, resFields)

	resourceDep := apicontainer.ResourceDependency{
		Name:           credentialspec.ResourceName,
		RequiredStatus: resourcestatus.ResourceStatus(credentialspec.CredentialSpecCreated),
	}

	assert.Equal(t, resourceDep, task.Containers[0].TransitionDependenciesMap[apicontainerstatus.ContainerCreated].ResourceDependencies[0])

	_, ok := task.GetCredentialSpecResource()
	assert.True(t, ok)
}

func TestGetCredentialSpecResource(t *testing.T) {
	credentialspecResource := &credentialspec.CredentialSpecResource{}
	task := &Task{
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
	}
	task.AddResource(credentialspec.ResourceName, credentialspecResource)

	credentialspecTaskResource, ok := task.GetCredentialSpecResource()
	assert.True(t, ok)
	assert.NotEmpty(t, credentialspecTaskResource)
}
