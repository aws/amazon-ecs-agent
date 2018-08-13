// +build windows,unit

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
	"fmt"
	"runtime"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"

	"github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
)

const (
	expectedMemorySwappinessDefault = memorySwappinessDefault
	minDockerClientAPIVersion       = dockerclient.Version_1_24
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

	assert.Equal(t, expectedTask.Containers, task.Containers, "Containers should be equal")
	assert.Equal(t, expectedTask.Volumes, task.Volumes, "Volumes should be equal")
}

func TestWindowsPlatformHostConfigOverride(t *testing.T) {
	// Testing Windows platform override for HostConfig.
	// Expects MemorySwappiness option to be set to -1

	task := &Task{}

	hostConfig := &docker.HostConfig{CPUShares: int64(1 * cpuSharesPerCore)}

	task.platformHostConfigOverride(hostConfig)
	assert.Equal(t, int64(1*cpuSharesPerCore*percentageFactor)/int64(cpuShareScaleFactor), hostConfig.CPUPercent)
	assert.Equal(t, int64(0), hostConfig.CPUShares)
	assert.EqualValues(t, expectedMemorySwappinessDefault, hostConfig.MemorySwappiness)

	hostConfig = &docker.HostConfig{CPUShares: 10}
	task.platformHostConfigOverride(hostConfig)
	assert.Equal(t, int64(minimumCPUPercent), hostConfig.CPUPercent)
	assert.Empty(t, hostConfig.CPUShares)
}

func TestWindowsMemorySwappinessOption(t *testing.T) {
	// Testing sending a task to windows overriding MemorySwappiness value
	rawHostConfigInput := docker.HostConfig{}

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
	}

	config, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), minDockerClientAPIVersion)
	if configErr != nil {
		t.Fatal(configErr)
	}

	assert.EqualValues(t, expectedMemorySwappinessDefault, config.MemorySwappiness)
}

func TestDockerHostConfigRawConfigMerging(t *testing.T) {
	// Use a struct that will marshal to the actual message we expect; not
	// docker.HostConfig which will include a lot of zero values.
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

	hostConfig, configErr := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), minDockerClientAPIVersion)
	assert.Nil(t, configErr)

	expected := docker.HostConfig{
		Memory:           apicontainer.DockerContainerMinimumMemoryInBytes,
		Privileged:       true,
		SecurityOpt:      []string{"foo", "bar"},
		VolumesFrom:      []string{"dockername-c2"},
		MemorySwappiness: memorySwappinessDefault,
		CPUPercent:       minimumCPUPercent,
	}

	assertSetStructFieldsEqual(t, expected, *hostConfig)
}

// TestSetConfigHostconfigBasedOnAPIVersion tests the docker hostconfig was correctly
// set based on the docker client version
func TestSetConfigHostconfigBasedOnAPIVersion(t *testing.T) {
	memoryMiB := 500
	testTask := &Task{
		Containers: []*apicontainer.Container{
			{
				Name:   "c1",
				CPU:    uint(10),
				Memory: uint(memoryMiB),
			},
		},
	}

	hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), minDockerClientAPIVersion)
	assert.Nil(t, err)

	config, cerr := testTask.DockerConfig(testTask.Containers[0], minDockerClientAPIVersion)
	assert.Nil(t, cerr)
	assert.Equal(t, int64(memoryMiB*1024*1024), hostconfig.Memory)
	assert.Empty(t, hostconfig.CPUShares)
	assert.Equal(t, int64(minimumCPUPercent), hostconfig.CPUPercent)

	assert.Empty(t, config.CPUShares)
	assert.Empty(t, config.Memory)
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
				platformFields: platformFields{
					cpuUnbounded: tc.cpuUnbounded,
				},
			}

			hostconfig, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask), minDockerClientAPIVersion)
			assert.Nil(t, err)
			assert.Empty(t, hostconfig.CPUShares)
			assert.Equal(t, tc.cpuPercent, hostconfig.CPUPercent)
		})
	}
}
