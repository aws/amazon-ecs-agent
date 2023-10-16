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

package sighandlers

import (
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	imageId          = "test-imageId"
	taskARN          = "arn:aws:ecs:region:account-id:task/task-id"
	eniAttachmentArn = "arn:aws:ecs:us-west-2:1234567890:attachment/abc"
)

func TestFinalSave(t *testing.T) {
	dataClient := newTestDataClient(t)

	state := dockerstate.NewTaskEngineState()
	taskEngine := engine.NewTaskEngine(&config.Config{}, nil, nil,
		nil, nil, nil, state, nil, nil, nil, nil, nil)

	task := &apitask.Task{
		Arn:     taskARN,
		Family:  "test",
		Version: "1",
	}

	dockerContainer := &apicontainer.DockerContainer{
		DockerID:   "docker-id",
		DockerName: "docker-name",
		Container: &apicontainer.Container{
			Name:          "container-name",
			TaskARNUnsafe: taskARN,
		},
	}

	eniAttachment := &ni.ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			TaskARN:          taskARN,
			AttachmentARN:    eniAttachmentArn,
			AttachStatusSent: false,
		},
	}
	imageState := &image.ImageState{
		Image: &image.Image{
			ImageID: imageId,
		},
		PullSucceeded: false,
	}

	state.AddTask(task)
	state.AddContainer(dockerContainer, task)
	state.AddImageState(imageState)
	state.AddENIAttachment(eniAttachment)

	assert.NoError(t, FinalSave(state, dataClient, taskEngine))

	// check if all states are saved to db on FinalSave
	tasks, err := dataClient.GetTasks()
	assert.NoError(t, err)
	assert.Len(t, tasks, 1)

	dockerContainers, err := dataClient.GetContainers()
	assert.NoError(t, err)
	assert.Len(t, dockerContainers, 1)

	attachments, err := dataClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, attachments, 1)

	imageStates, err := dataClient.GetImageStates()
	assert.NoError(t, err)
	assert.Len(t, imageStates, 1)
}

func newTestDataClient(t *testing.T) data.Client {
	testDir := t.TempDir()

	testClient, err := data.NewWithSetup(testDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, testClient.Close())
	})
	return testClient
}
