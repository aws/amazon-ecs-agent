// +build linux,unit

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
package execcmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
)

func strptr(s string) *string { return &s }

func TestInitializeTask(t *testing.T) {
	const containerNameOnlyHyphens = "--"
	var tt = []struct {
		taskName            string
		execEnabled         bool
		expectedVolumes     int
		expectedMountPoints int
		errorExpected       bool
	}{
		{
			taskName:            "arn:aws:iam::123456789012:user/Test",
			execEnabled:         true,
			expectedVolumes:     8,
			expectedMountPoints: 5,
			errorExpected:       false,
		},
		{
			taskName:            "arn:aws:iam::123456789012:user/Test",
			execEnabled:         false,
			expectedVolumes:     0,
			expectedMountPoints: 0,
			errorExpected:       false,
		},
		{
			taskName:            "BAD TASK NAME",
			execEnabled:         true,
			expectedVolumes:     0,
			expectedMountPoints: 0,
			errorExpected:       true,
		},
	}
	defer func() {
		GetExecAgentConfigFileName = getAgentConfigFileName
	}()
	for _, test := range tt {
		// Constants used here are defined in task_unix_test.go and task_windows_test.go
		taskFromACS := ecsacs.Task{
			Arn:           strptr(test.taskName),
			DesiredStatus: strptr("RUNNING"),
			Family:        strptr("myFamily"),
			Version:       strptr("1"),
			Containers: []*ecsacs.Container{
				{
					Name: strptr("myName1"),
				},
				{
					Name: strptr("myName2"),
				},
				{
					Name: strptr("--container-with-trailing-hyphens"),
				},
				{
					Name: strptr(containerNameOnlyHyphens), // Container that will end up in an empty directory name (trailing hyphens are removed)
				},
			},
		}
		seqNum := int64(42)
		task, err := apitask.TaskFromACS(&taskFromACS, &ecsacs.PayloadMessage{SeqNum: &seqNum})
		require.Nil(t, err, "Should be able to handle acs task")

		assert.Equal(t, 4, len(task.Containers))              // before PostUnmarshalTask
		task.ExecCommandAgentEnabledUnsafe = test.execEnabled // TODO: [ecs-exec] remove this statement once ecsacs model is complete with ecs exec stuff
		require.Equal(t, test.execEnabled, task.IsExecCommandAgentEnabled(), "task.IsExecCommandAgentEnabled() returned an unexpected value")

		if !test.execEnabled {
			return
		}

		execCmdMgr := newTestManager()
		GetExecAgentConfigFileName = func(s int) (string, error) {
			return ConfigFileName, nil
		}
		err = execCmdMgr.InitializeTask(task)
		if test.errorExpected {
			require.NotNil(t, err, "An error should be returned for a task with bad name")
			return
		} else {
			require.Nil(t, err, "No error was expected for PostUnmarshalTask")
		}

		assert.Equal(t, 4, len(task.Containers), "Should match the number of containers as before PostUnmarshalTask")
		taskVolumes := task.Volumes
		// 3 for the exec agent binaries + 1 for tls certs + 2 log volumes (1 per container) = 6
		assert.Equal(t, test.expectedVolumes, len(taskVolumes), "Should have created 6 task volumes")

		// Check agent binary volumes
		expectedBinaryNames := []string{BinName, SessionWorkerBinName}
		for _, ebn := range expectedBinaryNames {
			bvn := fmt.Sprintf("%s-%s", internalNamePrefix, ebn)
			assertTaskVolume(t, task, bvn, filepath.Join(HostBinDir, ebn))
		}

		// check tls cert volume
		assertTaskVolume(t, task, certVolumeName, HostCertFile)

		// check config file volume
		assertTaskVolume(t, task, configVolumeName, filepath.Join(HostExecConfigDir, ConfigFileName))

		// Check exec agent log volumes
		for _, c := range task.Containers {
			tID, _ := task.GetID()
			lvn := fmt.Sprintf("%s-%s-%s", logVolumeNamePrefix, tID, c.Name)

			// Check special case where container name contains only hyphens
			if c.Name == containerNameOnlyHyphens {
				assertLogVolumeWithPrefix(t, task, lvn, filepath.Join(HostLogDir, tID, namelessContainerPrefix))
			} else {
				assertTaskVolume(t, task, lvn, filepath.Join(HostLogDir, tID, strings.TrimLeft(c.Name, "-")))
			}
			// Check mount points were added for this container (1 for log, 1 for tls cert, 3 for binaries)
			require.Equal(t, test.expectedMountPoints, len(c.MountPoints))
			// Check exec agent log mount point
			assertMountPoint(t, c, lvn, ContainerLogDir, false)
			// Check exec agent tls cert mount point
			assertMountPoint(t, c, certVolumeName, ContainerCertFile, true)
			// Check exec agent config file mount point
			assertMountPoint(t, c, configVolumeName, ContainerConfigFile, true)

			// Check exec agent binary mount points
			for _, ebn := range expectedBinaryNames {
				assertMountPoint(t, c, fmt.Sprintf("%s-%s", internalNamePrefix, ebn), filepath.Join(ContainerBinDir, ebn), true)
			}
		}
	}
}

func findTaskVolumeByName(t *apitask.Task, name string) *apitask.TaskVolume {
	for _, tv := range t.Volumes {
		if tv.Name == name {
			return &tv
		}
	}
	return nil
}

func findMountPointBySourceVolume(c *apicontainer.Container, source string) *apicontainer.MountPoint {
	for _, mp := range c.MountPoints {
		if mp.SourceVolume == source {
			return &mp
		}
	}
	return nil
}

func assertLogVolumeWithPrefix(t *testing.T, task *apitask.Task, expectedVolName, expectedSourceDirPrefix string) {
	vol := findTaskVolumeByName(task, expectedVolName)
	require.NotNil(t, vol, "Exec agent volume (%s) was not added to task volumes", expectedVolName)
	assert.Equal(t, apitask.HostVolumeType, vol.Type, "exec agent volume (%s) is of the wrong type", expectedVolName)
	fshv, ok := vol.Volume.(*taskresourcevolume.FSHostVolume)
	assert.True(t, ok, "Exec agent volume (%s) is not of the correct type (FSHostVolume)", expectedVolName)
	assert.True(t, strings.HasPrefix(fshv.FSSourcePath, expectedSourceDirPrefix),
		"Exec agent volume (%s) has the wrong path prefix (expected: %s, actual: %s)", expectedVolName, expectedSourceDirPrefix, fshv.FSSourcePath)
}

func assertTaskVolume(t *testing.T, task *apitask.Task, expectedVolName, expectedSourceDir string) {
	vol := findTaskVolumeByName(task, expectedVolName)
	require.NotNil(t, vol, "Exec agent volume (%s) was not added to task volumes", expectedVolName)
	assert.Equal(t, apitask.HostVolumeType, vol.Type, "exec agent volume (%s) is of the wrong type", expectedVolName)
	fshv, ok := vol.Volume.(*taskresourcevolume.FSHostVolume)
	assert.True(t, ok, "Exec agent volume (%s) is not of the correct type (FSHostVolume)", expectedVolName)
	assert.Equal(t, expectedSourceDir, fshv.FSSourcePath, "Exec agent volume (%s) points to the wrong source path", expectedVolName)
}

func assertMountPoint(t *testing.T, c *apicontainer.Container, source, containerPath string, readonly bool) {
	found := findMountPointBySourceVolume(c, source)
	require.NotNil(t, found, "Mount point for volume (%s) not found in container (%s)", source, c.Name)
	assert.Equal(t, readonly, found.ReadOnly, "Mount point for volume (%s) for container (%s) has the wrong write permissions", source, c.Name)
	assert.Equal(t, containerPath, found.ContainerPath, "Mount point for volume (%s) points to the wrong container (%s) path", source, c.Name)
}

func TestGetExecAgentConfigFileName(t *testing.T) {
	execAgentConfig := `
	"Agent": {
	  "Region": "",
	  "OrchestrationRootDir": "",
	  "ContainerMode": true,
	  "Mgs": {
		"Region": "",
		"Endpoint": "",
		"StopTimeoutMillis": 20000,
		"SessionWorkersLimit": 2
	  }
	}`
	sha := getExecAgentConfigHash(execAgentConfig)
	configFileName := fmt.Sprintf("amazon-ssm-agent-%s.json", sha)
	var tests = []struct {
		fileExists             bool
		createConfigFileErr    error
		expectedConfigFileName string
		expectedError          error
	}{
		{
			fileExists:             true,
			createConfigFileErr:    nil,
			expectedConfigFileName: configFileName,
			expectedError:          nil,
		},
		{
			fileExists:             false,
			createConfigFileErr:    nil,
			expectedConfigFileName: configFileName,
			expectedError:          nil,
		},
		{
			fileExists:             false,
			createConfigFileErr:    errors.New("cannot create config"),
			expectedConfigFileName: "",
			expectedError:          errors.New("cannot create config"),
		},
	}
	defer func() {
		execAgentConfigFileExists = configFileExists
		createNewExecAgentConfigFile = createNewConfigFile
	}()
	for _, tc := range tests {
		execAgentConfigFileExists = func(file string) bool {
			return tc.fileExists
		}
		createNewExecAgentConfigFile = func(c, f string) error {
			return tc.createConfigFileErr
		}
		fileName, err := GetExecAgentConfigFileName(2)
		assert.Equal(t, tc.expectedConfigFileName, fileName, "incorrect config file name")
		assert.Equal(t, tc.expectedError, err)
	}
}
