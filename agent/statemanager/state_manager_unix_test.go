//go:build linux && unit
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
package statemanager_test

import (
	"os"
	"path/filepath"
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	engine_testutils "github.com/aws/amazon-ecs-agent/agent/engine/testutils"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateManager(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{DataDir: tmpDir}
	manager, err := statemanager.NewStateManager(cfg)
	assert.Nil(t, err, "Error loading manager")

	err = manager.Load()
	assert.Nil(t, err, "Expected loading a non-existent file to not be an error")

	// Now let's make some state to save
	containerInstanceArn := ""
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, dockerstate.NewTaskEngineState(),
		nil, nil, nil, nil, nil)

	manager, err = statemanager.NewStateManager(cfg, statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn))
	require.Nil(t, err)

	containerInstanceArn = "containerInstanceArn"

	testTask := &apitask.Task{Arn: "test-arn"}
	taskEngine.(*engine.DockerTaskEngine).State().AddTask(testTask)

	err = manager.Save()
	require.Nil(t, err, "Error saving state")

	assertFileMode(t, filepath.Join(tmpDir, "ecs_agent_data.json"))

	// Now make sure we can load that state sanely
	loadedTaskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, dockerstate.NewTaskEngineState(),
		nil, nil, nil, nil, nil)
	var loadedContainerInstanceArn string

	manager, err = statemanager.NewStateManager(cfg, statemanager.AddSaveable("TaskEngine", &loadedTaskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &loadedContainerInstanceArn))
	require.Nil(t, err)

	err = manager.Load()
	require.Nil(t, err, "Error loading state")

	assert.Equal(t, containerInstanceArn, loadedContainerInstanceArn, "Did not load containerInstanceArn correctly")

	if !engine_testutils.DockerTaskEnginesEqual(loadedTaskEngine.(*engine.DockerTaskEngine), (taskEngine.(*engine.DockerTaskEngine))) {
		t.Error("Did not load taskEngine correctly")
	}

	// I'd rather double check .Equal there; let's make sure ListTasks agrees.
	tasks, err := loadedTaskEngine.ListTasks()
	assert.Nil(t, err, "Error listing tasks")
	require.Equal(t, 1, len(tasks), "Should have a task!")
	assert.Equal(t, "test-arn", tasks[0].Arn, "Wrong arn")
}

func assertFileMode(t *testing.T, path string) {
	info, err := os.Stat(path)
	assert.Nil(t, err)

	mode := info.Mode()
	assert.Equal(t, os.FileMode(0600), mode, "Wrong file mode")
}

// verify that the state manager correctly loads the existing task networking related fields in state file.
// if we change those fields in the future, we should modify this test to test the new fields
func TestLoadsDataForAWSVPCTask(t *testing.T) {
	testCases := []struct {
		dir  string
		name string
	}{
		{"task-networking", "original_v11_encoding_scheme"},
		{"task-networking-refactor", "refactored_v11_encoding_scheme"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v11", tc.dir)}

			taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil, nil, nil, nil)
			var containerInstanceArn, cluster, savedInstanceID string

			stateManager, err := statemanager.NewStateManager(cfg,
				statemanager.AddSaveable("TaskEngine", taskEngine),
				statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn),
				statemanager.AddSaveable("Cluster", &cluster),
				statemanager.AddSaveable("EC2InstanceID", &savedInstanceID),
			)
			assert.NoError(t, err)
			err = stateManager.Load()
			assert.NoError(t, err)

			assert.Equal(t, "state-file", cluster)

			tasks, err := taskEngine.ListTasks()
			assert.NoError(t, err)
			assert.Equal(t, 1, len(tasks))

			task := tasks[0]
			assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:task/fad405be-8705-4175-877b-db50109a15f2", task.Arn)
			assert.Equal(t, "task-networking-state", task.Family)
			assert.NotNil(t, task.ENIs)

			eni := task.ENIs[0]
			assert.Equal(t, "eni-089ba8329b8e3f6ec", eni.ID)
			assert.Equal(t, "ip-172-31-10-246.us-west-2.compute.internal", eni.GetHostname())

			ipv4Addresses := eni.GetIPV4Addresses()
			assert.Equal(t, 1, len(ipv4Addresses))
			assert.Equal(t, "172.31.10.246", ipv4Addresses[0])
		})
	}

}

// verify that the state manager correctly loads gpu related fields in state file
func TestLoadsDataForGPU(t *testing.T) {
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v18", "gpu")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil, nil, nil, nil)
	var containerInstanceArn, cluster, savedInstanceID string
	var sequenceNumber int64
	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		statemanager.AddSaveable("Cluster", &cluster),
		statemanager.AddSaveable("EC2InstanceID", &savedInstanceID),
		statemanager.AddSaveable("SeqNum", &sequenceNumber),
	)
	assert.NoError(t, err)

	err = stateManager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "state-file", cluster)
	assert.EqualValues(t, 0, sequenceNumber)

	tasks, err := taskEngine.ListTasks()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(tasks))

	task := tasks[0]
	assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:task/33425c99-5db7-45fb-8244-bc94d00661e4", task.Arn)
	assert.Equal(t, "gpu-state", task.Family)
	assert.Equal(t, 1, len(task.Containers))
	assert.Equal(t, 2, len(task.Associations))

	association1 := task.Associations[0]
	assert.Equal(t, "container_1", association1.Containers[0])
	assert.Equal(t, "gpu", association1.Type)
	assert.Equal(t, "0", association1.Name)

	association2 := task.Associations[1]
	assert.Equal(t, "container_1", association2.Containers[0])
	assert.Equal(t, "gpu", association2.Type)
	assert.Equal(t, "1", association2.Name)

	container := task.Containers[0]
	assert.Equal(t, "container_1", container.Name)
	assert.Equal(t, []string{"0", "1"}, container.GPUIDs)
	assert.Equal(t, "0,1", container.Environment["NVIDIA_VISIBLE_DEVICES"])
}

func TestLoadsDataForFirelensTask(t *testing.T) {
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v23", "firelens")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil, nil, nil, nil)
	var containerInstanceArn, cluster, savedInstanceID string
	var sequenceNumber int64
	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		statemanager.AddSaveable("Cluster", &cluster),
		statemanager.AddSaveable("EC2InstanceID", &savedInstanceID),
		statemanager.AddSaveable("SeqNum", &sequenceNumber),
	)
	assert.NoError(t, err)
	err = stateManager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "test-logrouter-p2", cluster)
	assert.EqualValues(t, 0, sequenceNumber)
	tasks, err := taskEngine.ListTasks()
	assert.NoError(t, err)
	assert.Len(t, tasks, 1)
	task := tasks[0]
	assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:task/test-logrouter-p2/265f161270634aeaab7d0e88c29389c3", task.Arn)
	assert.Len(t, task.Containers, 2)
	foundFirelensContainer := false
	for _, container := range task.Containers {
		if container.FirelensConfig != nil {
			foundFirelensContainer = true
			assert.Equal(t, "fluentd", container.FirelensConfig.Type)
			assert.Equal(t, "true", container.FirelensConfig.Options["enable-ecs-log-metadata"])
		}
	}
	assert.True(t, foundFirelensContainer)

	assert.Len(t, task.ResourcesMapUnsafe, 2)
	assert.Len(t, task.ResourcesMapUnsafe["firelens"], 1)
	firelensResource, ok := task.ResourcesMapUnsafe["firelens"][0].(*firelens.FirelensResource)
	assert.True(t, ok)
	assert.Equal(t, "test-logrouter-p2", firelensResource.GetCluster())
	assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:task/test-logrouter-p2/265f161270634aeaab7d0e88c29389c3", firelensResource.GetTaskARN())
	assert.Equal(t, "test-firelens-beta-2-bridge:1", firelensResource.GetTaskDefinition())
	assert.Contains(t, firelensResource.GetContainerToLogOptions(), "app")
	assert.True(t, firelensResource.GetECSMetadataEnabled())
	assert.Equal(t, "i-1234567890", firelensResource.GetEC2InstanceID())
	assert.Equal(t, "/data/firelens/265f161270634aeaab7d0e88c29389c3", firelensResource.GetResourceDir())
	assert.Equal(t, resourcestatus.ResourceCreated, firelensResource.GetKnownStatus())
	assert.Equal(t, resourcestatus.ResourceCreated, firelensResource.GetDesiredStatus())
}

func TestLoadsDataForFirelensTaskWithExternalConfig(t *testing.T) {
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v24", "firelens")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil, nil, nil, nil)
	var containerInstanceArn, cluster, savedInstanceID string
	var sequenceNumber int64
	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		statemanager.AddSaveable("Cluster", &cluster),
		statemanager.AddSaveable("EC2InstanceID", &savedInstanceID),
		statemanager.AddSaveable("SeqNum", &sequenceNumber),
	)
	assert.NoError(t, err)
	err = stateManager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "test-logrouter-p2", cluster)
	assert.EqualValues(t, 0, sequenceNumber)
	tasks, err := taskEngine.ListTasks()
	assert.NoError(t, err)
	assert.Len(t, tasks, 1)
	task := tasks[0]
	assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:task/test-logrouter-p2/027002862d4b485ba3da5e935e0bc10c", task.Arn)
	assert.Len(t, task.Containers, 2)
	foundFirelensContainer := false
	for _, container := range task.Containers {
		if container.FirelensConfig != nil {
			foundFirelensContainer = true
			assert.Equal(t, "fluentbit", container.FirelensConfig.Type)
			assert.Equal(t, "file", container.FirelensConfig.Options["config-file-type"])
			assert.Equal(t, "/tmp/dummy.conf", container.FirelensConfig.Options["config-file-value"])
		}
	}
	assert.True(t, foundFirelensContainer)

	assert.Len(t, task.ResourcesMapUnsafe, 2)
	assert.Len(t, task.ResourcesMapUnsafe["firelens"], 1)
	firelensResource, ok := task.ResourcesMapUnsafe["firelens"][0].(*firelens.FirelensResource)
	assert.True(t, ok)
	assert.Equal(t, "test-logrouter-p2", firelensResource.GetCluster())
	assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:task/test-logrouter-p2/027002862d4b485ba3da5e935e0bc10c", firelensResource.GetTaskARN())
	assert.Equal(t, "test-firelens-local-bit:2", firelensResource.GetTaskDefinition())
	assert.Contains(t, firelensResource.GetContainerToLogOptions(), "app")
	assert.True(t, firelensResource.GetECSMetadataEnabled())
	assert.Equal(t, "i-1234567890", firelensResource.GetEC2InstanceID())
	assert.Equal(t, "/data/firelens/027002862d4b485ba3da5e935e0bc10c", firelensResource.GetResourceDir())
	assert.Equal(t, "file", firelensResource.GetExternalConfigType())
	assert.Equal(t, "/tmp/dummy.conf", firelensResource.GetExternalConfigValue())
	assert.Equal(t, "xxx", firelensResource.GetExecutionCredentialsID())
	assert.Equal(t, "us-west-2", firelensResource.GetRegion())
	assert.Equal(t, "bridge", firelensResource.GetNetworkMode())
	assert.Equal(t, resourcestatus.ResourceCreated, firelensResource.GetKnownStatus())
	assert.Equal(t, resourcestatus.ResourceCreated, firelensResource.GetDesiredStatus())
}

func TestLoadsDataForEFSGATask(t *testing.T) {
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v27", "efs")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil, nil, nil, nil)
	var containerInstanceArn, cluster, savedInstanceID string
	var sequenceNumber int64
	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		statemanager.AddSaveable("Cluster", &cluster),
		statemanager.AddSaveable("EC2InstanceID", &savedInstanceID),
		statemanager.AddSaveable("SeqNum", &sequenceNumber),
	)
	require.NoError(t, err)
	err = stateManager.Load()
	require.NoError(t, err)
	tasks, err := taskEngine.ListTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	task := tasks[0]

	taskVolumes := task.Volumes
	require.Len(t, taskVolumes, 1)
	require.Len(t, task.ResourcesMapUnsafe, 2)
	require.Len(t, task.ResourcesMapUnsafe["dockerVolume"], 1)
	volumeResource, ok := task.ResourcesMapUnsafe["dockerVolume"][0].(*taskresourcevolume.VolumeResource)
	require.True(t, ok)
	assert.Equal(t, "efs-html", volumeResource.Name)
	assert.Equal(t, "123", volumeResource.GetPauseContainerPID())
	assert.Equal(t, "ecs-test-efs-creds-ap-awsvpc-2-efs-html-xxx", volumeResource.VolumeConfig.DockerVolumeName)
	assert.Equal(t, "amazon-ecs-volume-plugin", volumeResource.VolumeConfig.Driver)
	require.NotNil(t, volumeResource.VolumeConfig.DriverOpts)
	assert.Equal(t, "fs-xxx:/", volumeResource.VolumeConfig.DriverOpts["device"])
	assert.Equal(t, "tls,tlsport=20050,iam,awscredsuri=/v2/credentials/xxx,accesspoint=fsap-xxx,netns=/proc/123/ns/net", volumeResource.VolumeConfig.DriverOpts["o"])
	assert.Equal(t, "efs", volumeResource.VolumeConfig.DriverOpts["type"])
}

func TestLoadsDataForGMSATask(t *testing.T) {
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v31", "gmsalinux")}
	taskEngineState := dockerstate.NewTaskEngineState()
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, taskEngineState, nil, nil, nil, nil, nil)
	var containerInstanceArn, cluster, savedInstanceID string
	var sequenceNumber int64

	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		statemanager.AddSaveable("Cluster", &cluster),
		statemanager.AddSaveable("EC2InstanceID", &savedInstanceID),
		statemanager.AddSaveable("SeqNum", &sequenceNumber),
	)

	assert.NoError(t, err)
	err = stateManager.Load()
	assert.NoError(t, err)
	assert.Equal(t, "gmsa-test", cluster)
	assert.EqualValues(t, 0, sequenceNumber)
	tasks, err := taskEngine.ListTasks()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(tasks))
	task := tasks[0]

	assert.Equal(t, "arn:aws:ecs:ap-northeast-1:1234567890:task/b8e2bd3c-c82a-4b43-9bde-199ae05b49a5", task.Arn)
	assert.Equal(t, "gmsa-test", task.Family)
	assert.Equal(t, 1, len(task.Containers))
	container := task.Containers[0]
	assert.Equal(t, "linux_sample_app", container.Name)

	resource, ok := task.GetCredentialSpecResource()
	assert.True(t, ok)
	assert.NotEmpty(t, resource)

	credSpecResource := resource[0].(*credentialspec.CredentialSpecResource)

	assert.Equal(t, status.ResourceCreated, credSpecResource.GetDesiredStatus())
	assert.Equal(t, status.ResourceCreated, credSpecResource.GetKnownStatus())

	credSpecMap := credSpecResource.CredSpecMap
	assert.NotEmpty(t, credSpecMap)

	testCredSpec := "credentialspec:arn:aws:s3:::gmsacredspec/contoso_webapp01.json"
	expectedKerberosTicketPath := "/var/credentials-fetcher/krbdir/123456/webapp01"

	actualKerberosTicketPath, err := credSpecResource.GetTargetMapping(testCredSpec)
	assert.NoError(t, err)
	assert.Equal(t, expectedKerberosTicketPath, actualKerberosTicketPath)
}
