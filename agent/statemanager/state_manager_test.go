// +build unit

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

package statemanager_test

import (
	"path/filepath"
	"testing"
	"time"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateManagerNonexistantDirectory(t *testing.T) {
	cfg := &config.Config{DataDir: "/path/to/directory/that/doesnt/exist"}
	_, err := statemanager.NewStateManager(cfg)
	assert.Error(t, err)
}

func TestLoadsV1DataCorrectly(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v1", "1", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v1", "1")}

	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(),
		nil, nil)
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
	assert.Equal(t, cluster, "test")
	assert.True(t, sequenceNumber == 0)
	tasks, err := taskEngine.ListTasks()
	assert.NoError(t, err)
	var deadTask *apitask.Task
	for _, task := range tasks {
		if task.Arn == "arn:aws:ecs:us-west-2:1234567890:task/f44b4fc9-adb0-4f4f-9dff-871512310588" {
			deadTask = task
		}
	}

	require.NotNil(t, deadTask)
	assert.Equal(t, deadTask.GetSentStatus(), apitaskstatus.TaskStopped)
	assert.Equal(t, deadTask.Containers[0].SentStatusUnsafe, apicontainerstatus.ContainerStopped)
	assert.Equal(t, deadTask.Containers[0].DesiredStatusUnsafe, apicontainerstatus.ContainerStopped)
	assert.Equal(t, deadTask.Containers[0].KnownStatusUnsafe, apicontainerstatus.ContainerStopped)

	exitCode := deadTask.Containers[0].KnownExitCodeUnsafe
	require.NotNil(t, exitCode)
	assert.Equal(t, *exitCode, 128)

	expected, _ := time.Parse(time.RFC3339, "2015-04-28T17:29:48.129140193Z")
	assert.Equal(t, deadTask.GetKnownStatusTime(), expected)
}

func TestLoadsV13DataCorrectly(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v13", "1", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v13", "1")}

	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil)
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

	assert.Equal(t, "test", cluster)
	assert.EqualValues(t, 0, sequenceNumber)
	tasks, err := taskEngine.ListTasks()
	assert.NoError(t, err)
	var deadTask *apitask.Task
	for _, task := range tasks {
		if task.Arn == "arn:aws:ecs:us-west-2:694464167470:task/5e9f6adb-2a02-48db-860f-41e12c4ced32" {
			deadTask = task
		}
	}
	require.NotNil(t, deadTask)
	assert.Equal(t, deadTask.GetSentStatus(), apitaskstatus.TaskRunning)
	assert.Equal(t, deadTask.Containers[0].SentStatusUnsafe, apicontainerstatus.ContainerRunning)
	assert.Equal(t, deadTask.Containers[0].DesiredStatusUnsafe, apicontainerstatus.ContainerRunning)
	assert.Equal(t, deadTask.Containers[0].KnownStatusUnsafe, apicontainerstatus.ContainerRunning)

	exitCode := deadTask.Containers[0].KnownExitCodeUnsafe
	require.NotNil(t, exitCode)
	assert.Equal(t, *exitCode, 128)

	expected, _ := time.Parse(time.RFC3339, "2015-04-28T17:29:48.129140193Z")
	assert.Equal(t, deadTask.GetKnownStatusTime(), expected)

}

// verify that the state manager correctly loads the existing container health check related fields in state file.
// if we change those fields in the future, we should modify this test to test the new fields
func TestLoadsDataForContainerHealthCheckTask(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v10", "container-health-check", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v10", "container-health-check")}

	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil)
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
	assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:task/e4e6c98c-aa44-4146-baf9-431b04c0d162", task.Arn)
	assert.Equal(t, "chc-state", task.Family)
	assert.Equal(t, 1, len(task.Containers))

	container := task.Containers[0]
	assert.Equal(t, "container_1", container.Name)
	assert.Equal(t, "docker", container.HealthCheckType)
	assert.NotNil(t, container.DockerConfig)
	assert.Equal(t, "{\"HealthCheck\":{\"Test\":[\"CMD\",\"echo\",\"hello\"],\"Interval\":30000000000,\"Timeout\":5000000000,\"Retries\":3}}", *container.DockerConfig.Config)
}

// verify that the state manager correctly loads the existing private registry related fields in state file.
// if we change those fields in the future, we should modify this test to test the new fields
func TestLoadsDataForPrivateRegistryTask(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v14", "private-registry", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v14", "private-registry")}

	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil)
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
	assert.Equal(t, "private-registry-state", task.Family)
	assert.Equal(t, 1, len(task.Containers))

	container := task.Containers[0]
	assert.Equal(t, "container_1", container.Name)
	assert.NotNil(t, container.RegistryAuthentication)

	registryAuth := container.RegistryAuthentication
	assert.Equal(t, "asm", registryAuth.Type)
	assert.NotNil(t, registryAuth.ASMAuthData)

	asmAuthData := registryAuth.ASMAuthData
	assert.Equal(t, "arn:aws:secretsmanager:us-west-2:1234567890:secret:FunctionalTest-PrivateRegistryAuth-I0nqxs", asmAuthData.CredentialsParameter)
	assert.Equal(t, "us-west-2", asmAuthData.Region)
}

// verify that the state manager correctly loads ssm secrets related fields in state file
func TestLoadsDataForSecretsTask(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v17", "secrets", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v17", "secrets")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil)
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
	assert.Equal(t, "secrets-state", task.Family)
	assert.Equal(t, 1, len(task.Containers))
	container := task.Containers[0]
	assert.Equal(t, "container_1", container.Name)
	assert.NotNil(t, container.Secrets)
	secret := container.Secrets[0]
	assert.Equal(t, "ENVIRONMENT_VARIABLE", secret.Type)
	assert.Equal(t, "ssm-secret", secret.Name)
	assert.Equal(t, "us-west-2", secret.Region)
	assert.Equal(t, "secret-value-from", secret.ValueFrom)
	assert.Equal(t, "ssm", secret.Provider)
}

func TestLoadsDataForAddingAvailabilityZoneInTask(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v18", "availabilityZone", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v18", "availabilityZone")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil)
	var containerInstanceArn, cluster, savedInstanceID, availabilityZone string
	var sequenceNumber int64
	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		statemanager.AddSaveable("Cluster", &cluster),
		statemanager.AddSaveable("EC2InstanceID", &savedInstanceID),
		statemanager.AddSaveable("SeqNum", &sequenceNumber),
		statemanager.AddSaveable("AvailabilityZone", &availabilityZone),
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
	assert.Equal(t, 1, len(task.Containers))
	assert.Equal(t, "us-west-2c", availabilityZone)
}

// verify that the state manager correctly loads asm secrets related fields in state file
func TestLoadsDataForASMSecretsTask(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v18", "secrets", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v18", "secrets")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil)
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
	assert.Equal(t, "secrets-state", task.Family)
	assert.Equal(t, 1, len(task.Containers))
	container := task.Containers[0]
	assert.Equal(t, "container_1", container.Name)
	assert.NotNil(t, container.Secrets)
	secret := container.Secrets[1]
	assert.Equal(t, "ENVIRONMENT_VARIABLE", secret.Type)
	assert.Equal(t, "asm-secret", secret.Name)
	assert.Equal(t, "us-west-2", secret.Region)
	assert.Equal(t, "secret-value-from", secret.ValueFrom)
	assert.Equal(t, "asm", secret.Provider)
}

// verify that the state manager correctly loads container ordering related fields in state file
func TestLoadsDataForContainerOrdering(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v20", "containerOrdering", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v20", "containerOrdering")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil)
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
	assert.Equal(t, "container-ordering-state", task.Family)
	assert.Equal(t, 2, len(task.Containers))

	dependsOn := task.Containers[1].DependsOn
	assert.Equal(t, "container_1", dependsOn[0].ContainerName)
	assert.Equal(t, "START", dependsOn[0].Condition)
}

func TestLoadsDataForPerContainerTimeouts(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v20", "perContainerTimeouts", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()
	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v20", "perContainerTimeouts")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil)
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
	assert.Equal(t, "per-container-timeouts", task.Family)
	assert.Equal(t, 1, len(task.Containers))

	c1 := task.Containers[0]
	assert.Equal(t, uint(10), c1.StartTimeout)
	assert.Equal(t, uint(10), c1.StopTimeout)
}
