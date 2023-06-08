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

package statemanager_test

import (
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/credentialspec"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadsDataForGMSATask(t *testing.T) {
	cleanup, err := setupWindowsTest(filepath.Join(".", "testdata", "v26", "gmsa", "ecs_agent_data.json"))
	require.Nil(t, err, "Failed to set up test")
	defer cleanup()

	cfg := &config.Config{DataDir: filepath.Join(".", "testdata", "v26", "gmsa")}
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil, nil, nil, nil, dockerstate.NewTaskEngineState(), nil, nil, nil, nil)
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
	assert.Equal(t, "windows_sample_app", container.Name)
	assert.NotNil(t, container.DockerConfig.HostConfig)

	resource, ok := task.GetCredentialSpecResource()
	assert.True(t, ok)
	assert.NotEmpty(t, resource)

	credSpecResource := resource[0].(*credentialspec.CredentialSpecResource)

	assert.Equal(t, status.ResourceCreated, credSpecResource.GetDesiredStatus())
	assert.Equal(t, status.ResourceCreated, credSpecResource.GetKnownStatus())

	credSpecMap := credSpecResource.CredSpecMap
	assert.NotEmpty(t, credSpecMap)

	testCredSpec := "credentialspec:file://WebApp01.json"
	expectedCredSpec := "credentialspec=file://WebApp01.json"

	targetCredSpec, err := credSpecResource.GetTargetMapping(testCredSpec)
	assert.NoError(t, err)
	assert.Equal(t, expectedCredSpec, targetCredSpec)
}
