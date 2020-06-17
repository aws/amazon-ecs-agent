// +build unit

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

package engine

import (
	"io/ioutil"
	"os"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testContainerName = "test-name"
)

var (
	testContainer = &apicontainer.Container{
		Name:    testContainerName,
		TaskARN: testTaskARN,
	}
	testTask = &apitask.Task{
		Arn:        testTaskARN,
		Containers: []*apicontainer.Container{testContainer},
	}
)

func newTestDataClient(t *testing.T) (data.Client, func()) {
	testDir, err := ioutil.TempDir("", "agent_engine_unit_test")
	require.NoError(t, err)

	testClient, err := data.NewWithSetup(testDir)

	cleanup := func() {
		require.NoError(t, testClient.Close())
		require.NoError(t, os.RemoveAll(testDir))
	}
	return testClient, cleanup
}

func TestSaveAndRemoveTaskData(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	engine := &DockerTaskEngine{
		dataClient: dataClient,
	}
	engine.saveTaskData(testTask)
	tasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	engine.removeTaskData(testTask)
	tasks, err = dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestSaveContainerData(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	engine := &DockerTaskEngine{
		dataClient: dataClient,
	}
	engine.saveContainerData(testContainer)
	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)
}

func TestSaveDockerContainerData(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	engine := &DockerTaskEngine{
		dataClient: dataClient,
	}
	engine.saveDockerContainerData(&apicontainer.DockerContainer{
		DockerName: "test-docker-name",
		Container:  testContainer,
	})
	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)
}
