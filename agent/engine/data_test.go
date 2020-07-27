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
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testContainerName = "test-name"
	testImageId       = "test-imageId"
	testMac           = "test-mac"
	testAttachmentArn = "arn:aws:ecs:us-west-2:1234567890:attachment/abc"
	testDockerID      = "test-docker-id"
	testTaskIP        = "10.1.2.3"
)

var (
	testContainer = &apicontainer.Container{
		Name:          testContainerName,
		TaskARNUnsafe: testTaskARN,
	}
	testDockerContainer = &apicontainer.DockerContainer{
		DockerID:  testDockerID,
		Container: testContainer,
	}
	testTask = &apitask.Task{
		Arn:                  testTaskARN,
		Containers:           []*apicontainer.Container{testContainer},
		LocalIPAddressUnsafe: testTaskIP,
	}

	testImageState = &image.ImageState{
		Image:         testImage,
		PullSucceeded: false,
	}
	testImage = &image.Image{
		ImageID: testImageId,
	}

	testENIAttachment = &eni.ENIAttachment{
		AttachmentARN:    testAttachmentArn,
		AttachStatusSent: false,
		MACAddress:       testMac,
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

func TestLoadState(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	engine := &DockerTaskEngine{
		state:      dockerstate.NewTaskEngineState(),
		dataClient: dataClient,
	}
	require.NoError(t, dataClient.SaveTask(testTask))
	testDockerContainer.Container.SetKnownStatus(apicontainerstatus.ContainerRunning)
	require.NoError(t, dataClient.SaveDockerContainer(testDockerContainer))
	require.NoError(t, dataClient.SaveENIAttachment(testENIAttachment))
	require.NoError(t, dataClient.SaveImageState(testImageState))

	require.NoError(t, engine.LoadState())
	task, ok := engine.state.TaskByArn(testTaskARN)
	assert.True(t, ok)
	// Also check that the container in the task has the updated status from container table.
	assert.Equal(t, apicontainerstatus.ContainerRunning, task.Containers[0].GetKnownStatus())
	_, ok = engine.state.ContainerByID(testDockerID)
	assert.True(t, ok)
	assert.Len(t, engine.state.AllImageStates(), 1)
	assert.Len(t, engine.state.AllENIAttachments(), 1)

	// Check ip <-> task arn mapping is loaded in state.
	ip, ok := engine.state.GetIPAddressByTaskARN(testTaskARN)
	require.True(t, ok)
	assert.Equal(t, testTaskIP, ip)
	arn, ok := engine.state.GetTaskByIPAddress(testTaskIP)
	require.True(t, ok)
	assert.Equal(t, testTaskARN, arn)
}

func TestSaveState(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	engine := &DockerTaskEngine{
		state:      dockerstate.NewTaskEngineState(),
		dataClient: dataClient,
	}
	engine.state.AddTask(testTask)
	engine.state.AddContainer(testDockerContainer, testTask)
	engine.state.AddImageState(testImageState)
	engine.state.AddENIAttachment(testENIAttachment)

	require.NoError(t, engine.SaveState())
	tasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)

	images, err := dataClient.GetImageStates()
	require.NoError(t, err)
	assert.Len(t, images, 1)

	eniAttachments, err := dataClient.GetENIAttachments()
	require.NoError(t, err)
	assert.Len(t, eniAttachments, 1)
}

func TestSaveStateEnsureBoltDBCompatibility(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	engine := &DockerTaskEngine{
		state:      dockerstate.NewTaskEngineState(),
		dataClient: dataClient,
	}

	// Save a container without task ARN populated in taskARNUnsafe field and a task without ip address
	// populated in localIPAddressUnsafe field. Check they are populated upon saving to boltdb.
	testContainer := &apicontainer.Container{
		Name: testContainerName,
	}
	testDockerContainer := &apicontainer.DockerContainer{
		DockerID:  testDockerID,
		Container: testContainer,
	}
	testTask := &apitask.Task{
		Arn:        testTaskARN,
		Containers: []*apicontainer.Container{testContainer},
	}
	engine.state.AddTask(testTask)
	engine.state.AddContainer(testDockerContainer, testTask)
	engine.state.AddTaskIPAddress(testTaskIP, testTaskARN)

	require.NoError(t, engine.SaveState())
	tasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, testTaskIP, tasks[0].GetLocalIPAddress())

	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)
	assert.Equal(t, testTaskARN, containers[0].Container.GetTaskARN())
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

func TestSaveAndRemoveImageStateData(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	imageManager := &dockerImageManager{
		dataClient: dataClient,
	}
	imageManager.saveImageStateData(testImageState)
	res, err := dataClient.GetImageStates()
	require.NoError(t, err)
	assert.Len(t, res, 1)

	imageManager.removeImageStateData(testImageId)
	res, err = dataClient.GetImageStates()
	require.NoError(t, err)
	assert.Len(t, res, 0)
}

func TestRemoveENIAttachmentData(t *testing.T) {
	dataClient, cleanup := newTestDataClient(t)
	defer cleanup()

	engine := &DockerTaskEngine{
		state:      dockerstate.NewTaskEngineState(),
		dataClient: dataClient,
	}

	engine.state.AddENIAttachment(testENIAttachment)
	dataClient.SaveENIAttachment(testENIAttachment)
	res, err := dataClient.GetENIAttachments()
	require.NoError(t, err)
	assert.Len(t, res, 1)

	engine.removeENIAttachmentData(testMac)
	res, err = dataClient.GetENIAttachments()
	require.NoError(t, err)
	assert.Len(t, res, 0)
}
