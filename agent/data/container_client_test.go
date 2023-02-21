//go:build unit
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

package data

import (
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDockerID       = "test-docker-id"
	testDockerName     = "test-docker-name"
	testContainerName  = "test-name"
	testContainerName2 = "test-name-2"
)

func TestManageContainers(t *testing.T) {
	testClient := newTestClient(t)

	// Test saving a container with SaveDockerContainer and updating it with SaveContainer.
	testDockerContainer := &apicontainer.DockerContainer{
		DockerID:   testDockerID,
		DockerName: testDockerName,
		Container: &apicontainer.Container{
			Name:          testContainerName,
			TaskARNUnsafe: testTaskArn,
		},
	}
	require.NoError(t, testClient.SaveDockerContainer(testDockerContainer))
	testDockerContainer.Container.SetKnownStatus(apicontainerstatus.ContainerRunning)
	require.NoError(t, testClient.SaveContainer(testDockerContainer.Container))
	res, err := testClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, apicontainerstatus.ContainerRunning, res[0].Container.GetKnownStatus())
	assert.Equal(t, testDockerID, res[0].DockerID)
	assert.Equal(t, testDockerName, res[0].DockerName)

	// Test saving a container with SaveContainer.
	testContainer := &apicontainer.Container{
		Name:          testContainerName2,
		TaskARNUnsafe: testTaskArn,
	}
	require.NoError(t, testClient.SaveContainer(testContainer))
	res, err = testClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, res, 2)

	// Test deleting containers.
	require.NoError(t, testClient.DeleteContainer("abc-test-name"))
	require.NoError(t, testClient.DeleteContainer("abc-test-name-2"))
	res, err = testClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, res, 0)
}

func TestSaveContainerInvalidID(t *testing.T) {
	testClient := newTestClient(t)

	testDockerContainer := &apicontainer.DockerContainer{
		DockerID:   testDockerID,
		DockerName: testDockerName,
		Container: &apicontainer.Container{
			Name:          testContainerName,
			TaskARNUnsafe: "invalid-arn",
		},
	}
	assert.Error(t, testClient.SaveDockerContainer(testDockerContainer))
	assert.Error(t, testClient.SaveContainer(testDockerContainer.Container))
}

func TestGetContainerID(t *testing.T) {
	c := &apicontainer.Container{
		TaskARNUnsafe: testTaskArn,
		Name:          testContainerName,
	}
	id, err := GetContainerID(c)
	require.NoError(t, err)
	assert.Equal(t, "abc-test-name", id)

	c.TaskARNUnsafe = "invalid"
	_, err = GetContainerID(c)
	assert.Error(t, err)
}
