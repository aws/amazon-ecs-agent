//go:build windows && unit
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
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/fsxwindowsfileserver"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/golang/mock/gomock"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mock_asm_factory "github.com/aws/amazon-ecs-agent/agent/asm/factory/mocks"
	mock_credentials "github.com/aws/amazon-ecs-agent/agent/credentials/mocks"
	mock_fsx_factory "github.com/aws/amazon-ecs-agent/agent/fsx/factory/mocks"
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
				Name:          "myName",
				TaskARNUnsafe: "myArn",
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
	cfg := config.Config{TaskCPUMemLimit: config.BooleanDefaultTrue{Value: config.ExplicitlyDisabled}}
	task.PostUnmarshalTask(&cfg, nil, nil, nil, nil)

	for _, container := range task.Containers { // remove v3 endpoint from each container because it's randomly generated
		removeEndpointConfigFromEnvironment(container)
	}
	assert.Equal(t, expectedTask.Containers, task.Containers, "Containers should be equal")
	assert.Equal(t, expectedTask.Volumes, task.Volumes, "Volumes should be equal")
}

// removeEndpointConfigFromEnvironment removes the v3 endpoint id and the injected env for a container
// so that checking all other fields can be easier
func removeEndpointConfigFromEnvironment(container *apicontainer.Container) {
	container.SetV3EndpointID("")
	if container.Environment != nil {
		delete(container.Environment, apicontainer.MetadataURIEnvironmentVariableName)
		delete(container.Environment, apicontainer.MetadataURIEnvVarNameV4)
		delete(container.Environment, apicontainer.AgentURIEnvVarName)
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
		cpuUnbounded config.BooleanDefaultFalse
		cpuPercent   int64
	}{
		{
			cpu:          0,
			cpuUnbounded: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			cpuPercent:   0,
		},
		{
			cpu:          1,
			cpuUnbounded: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			cpuPercent:   1,
		},
		{
			cpu:          0,
			cpuUnbounded: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled},
			cpuPercent:   1,
		},
		{
			cpu:          1,
			cpuUnbounded: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled},
			cpuPercent:   1,
		},
		{
			cpu:          100,
			cpuUnbounded: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			cpuPercent:   100 * percentageFactor / int64(cpuShareScaleFactor),
		},
		{
			cpu:          100,
			cpuUnbounded: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled},
			cpuPercent:   100 * percentageFactor / int64(cpuShareScaleFactor),
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("container cpu-%d,cpu unbounded tasks enabled- %t,expected cpu percent-%d",
			tc.cpu, tc.cpuUnbounded.Enabled(), tc.cpuPercent), func(t *testing.T) {
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
			MemoryUnbounded: config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled},
		},
	}

	// With MemoryUnbounded set to false, MemoryReservation is not overridden
	cfg, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask),
		defaultDockerClientAPIVersion, &config.Config{})

	assert.Nil(t, configErr)
	assert.EqualValues(t, nonZeroMemoryReservationValue, cfg.MemoryReservation)

	// With MemoryUnbounded set to true, tasks with no memory hard limit will have their memory reservation set to zero
	testTask.PlatformFields.MemoryUnbounded = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
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
			result := utils.GetCanonicalPath(tc.path)
			assert.Equal(t, result, tc.expectedResult)
		})
	}
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

func TestRequiresFSxWindowsFileServerResource(t *testing.T) {
	task1 := &Task{
		Arn: "test1",
		Volumes: []TaskVolume{
			{
				Name: "fsxWindowsFileServerVolume",
				Type: "fsxWindowsFileServer",
				Volume: &fsxwindowsfileserver.FSxWindowsFileServerVolumeConfig{
					FileSystemID:  "fs-12345678",
					RootDirectory: "root",
					AuthConfig: fsxwindowsfileserver.FSxWindowsFileServerAuthConfig{
						CredentialsParameter: "arn",
						Domain:               "test",
					},
				},
			},
		},
	}

	task2 := &Task{
		Arn:     "test2",
		Volumes: []TaskVolume{},
	}

	testCases := []struct {
		name           string
		task           *Task
		expectedOutput bool
	}{
		{
			name:           "valid_fsxwindowsfileserver",
			task:           task1,
			expectedOutput: true,
		},
		{
			name:           "missing_fsxwindowsfileserver",
			task:           task2,
			expectedOutput: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, tc.task.requiresFSxWindowsFileServerResource())
		})
	}
}

func TestInitializeAndAddFSxWindowsFileServerResource(t *testing.T) {
	task := &Task{
		Arn:                "test1",
		ResourcesMapUnsafe: make(map[string][]taskresource.TaskResource),
		Volumes: []TaskVolume{
			{
				Name: "fsxWindowsFileServerVolume",
				Type: "fsxWindowsFileServer",
				Volume: &fsxwindowsfileserver.FSxWindowsFileServerVolumeConfig{
					FileSystemID:  "fs-12345678",
					RootDirectory: "root",
					AuthConfig: fsxwindowsfileserver.FSxWindowsFileServerAuthConfig{
						CredentialsParameter: "arn",
						Domain:               "test",
					},
				},
			},
		},
		Containers: []*apicontainer.Container{
			{
				Name: "myName",
				MountPoints: []apicontainer.MountPoint{
					{
						SourceVolume:  "fsxWindowsFileServerVolume",
						ContainerPath: "/test",
						ReadOnly:      false,
					},
				},
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &config.Config{
		AWSRegion: "test-aws-region",
	}

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)
	asmClientCreator := mock_asm_factory.NewMockClientCreator(ctrl)
	fsxClientCreator := mock_fsx_factory.NewMockFSxClientCreator(ctrl)

	resFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			ASMClientCreator:   asmClientCreator,
			SSMClientCreator:   ssmClientCreator,
			FSxClientCreator:   fsxClientCreator,
			CredentialsManager: credentialsManager,
		},
	}

	task.initializeFSxWindowsFileServerResource(cfg, credentialsManager, resFields)

	assert.Equal(t, 1, len(task.Containers), "Should match the number of containers as before PostUnmarshalTask")
	assert.Equal(t, 1, len(task.Volumes), "Should have 1 volume")
	assert.Equal(t, 1, len(task.Containers[0].TransitionDependenciesMap), "Should have 1 container volume dependency")
	taskVol := task.Volumes[0]
	assert.Equal(t, "fsxWindowsFileServerVolume", taskVol.Name)
	assert.Equal(t, FSxWindowsFileServerVolumeType, taskVol.Type)

	resources := task.GetResources()
	assert.Len(t, resources, 1)
	_, ok := resources[0].(*fsxwindowsfileserver.FSxWindowsFileServerResource)
	require.True(t, ok)
}

func TestPostUnmarshalTaskWithFSxWindowsFileServerVolumes(t *testing.T) {
	taskFromACS := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			{
				Name: strptr("myName1"),
				MountPoints: []*ecsacs.MountPoint{
					{
						ContainerPath: strptr("\\some\\path"),
						SourceVolume:  strptr("fsxWindowsFileServerVolume"),
					},
				},
			},
		},
		Volumes: []*ecsacs.Volume{
			{
				Name: strptr("fsxWindowsFileServerVolume"),
				Type: strptr("fsxWindowsFileServer"),
				FsxWindowsFileServerVolumeConfiguration: &ecsacs.FSxWindowsFileServerVolumeConfiguration{
					AuthorizationConfig: &ecsacs.FSxWindowsFileServerAuthorizationConfig{
						CredentialsParameter: strptr("arn"),
						Domain:               strptr("test"),
					},
					FileSystemId:  strptr("fs-12345678"),
					RootDirectory: strptr("test"),
				},
			},
		},
	}
	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	assert.Nil(t, err, "Should be able to handle acs task")
	assert.Equal(t, 1, len(task.Containers)) // before PostUnmarshalTask

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := config.Config{
		AWSRegion: "test-aws-region",
	}

	credentialsManager := mock_credentials.NewMockManager(ctrl)
	ssmClientCreator := mock_ssm_factory.NewMockSSMClientCreator(ctrl)
	asmClientCreator := mock_asm_factory.NewMockClientCreator(ctrl)
	fsxClientCreator := mock_fsx_factory.NewMockFSxClientCreator(ctrl)

	resFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator:   ssmClientCreator,
			ASMClientCreator:   asmClientCreator,
			FSxClientCreator:   fsxClientCreator,
			CredentialsManager: credentialsManager,
		},
	}

	task.PostUnmarshalTask(&cfg, credentialsManager, resFields, nil, nil)
	assert.Equal(t, 1, len(task.Containers), "Should match the number of containers as before PostUnmarshalTask")
	assert.Equal(t, 1, len(task.Volumes), "Should have 1 volume")
	taskVol := task.Volumes[0]
	assert.Equal(t, "fsxWindowsFileServerVolume", taskVol.Name)
	assert.Equal(t, FSxWindowsFileServerVolumeType, taskVol.Type)

	resources := task.GetResources()
	assert.Len(t, resources, 1)
	_, ok := resources[0].(*fsxwindowsfileserver.FSxWindowsFileServerResource)
	require.True(t, ok)
	b, err := json.Marshal(resources[0])
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{
		"name": "fsxWindowsFileServerVolume",
		"taskARN": "myArn",
		"executionCredentialsID": "",
		"fsxWindowsFileServerVolumeConfiguration": {
		  	"fileSystemId": "fs-12345678",
		  	"rootDirectory": "test",
		  	"authorizationConfig": {
				"credentialsParameter": "arn",
				"domain": "test"
		  	},
		  "fsxWindowsFileServerHostPath": ""
		},
		"createdAt": "0001-01-01T00:00:00Z",
		"desiredStatus": "NONE",
		"knownStatus": "NONE"
	  }`), string(b))
}

// TestBuildCNIConfig tests if the generated CNI config is correct
func TestBuildCNIConfig(t *testing.T) {
	testTask := &Task{}
	testTask.NetworkMode = AWSVPCNetworkMode
	testTask.AddTaskENI(&apieni.ENI{
		ID:                           "TestBuildCNIConfig",
		MacAddress:                   mac,
		InterfaceAssociationProtocol: apieni.DefaultInterfaceAssociationProtocol,
		SubnetGatewayIPV4Address:     "10.0.1.0/24",
		IPV4Addresses: []*apieni.ENIIPV4Address{
			{
				Primary: true,
				Address: ipv4,
			},
		},
	})

	cniConfig, err := testTask.BuildCNIConfigAwsvpc(true, &ecscni.Config{
		MinSupportedCNIVersion: "latest",
	})
	assert.NoError(t, err)
	// We expect 2 NetworkConfig objects in the cni Config wrapper object:
	// vpc-eni for task ENI setup.
	// vpc-eni for ecs-bridge setup.
	require.Len(t, cniConfig.NetworkConfigs, 2)
	var eniConfig ecscni.VPCENIPluginConfig
	// For the task ns setup.
	err = json.Unmarshal(cniConfig.NetworkConfigs[0].CNINetworkConfig.Bytes, &eniConfig)
	require.NoError(t, err)
	assert.EqualValues(t, ecscni.ECSVPCENIPluginName, eniConfig.Type)
	assert.False(t, eniConfig.UseExistingNetwork)
	// For the ecs-bridge setup.
	err = json.Unmarshal(cniConfig.NetworkConfigs[1].CNINetworkConfig.Bytes, &eniConfig)
	require.NoError(t, err)
	assert.EqualValues(t, ecscni.ECSVPCENIPluginName, eniConfig.Type)
	assert.True(t, eniConfig.UseExistingNetwork)
	assert.EqualValues(t, ecscni.ECSBridgeNetworkName, cniConfig.NetworkConfigs[1].CNINetworkConfig.Network.Name)
}
