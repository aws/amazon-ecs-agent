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

package handler

import (
	"testing"
	"time"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	attachmentArn = "arn:aws:ecs:us-west-2:1234567890:attachment/abc"
)

// TestTaskENIAckTimeout tests acknowledge timeout for a task eni before submit the state change
func TestTaskENIAckTimeout(t *testing.T) {
	testENIAckTimeout(t, apieni.ENIAttachmentTypeTaskENI)
}

// TestInstanceENIAckTimeout tests acknowledge timeout for an instance level eni before submit the state change
func TestInstanceENIAckTimeout(t *testing.T) {
	testENIAckTimeout(t, apieni.ENIAttachmentTypeInstanceENI)
}

func testENIAckTimeout(t *testing.T, attachmentType string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := newTestDataClient(t)

	expiresAt := time.Now().Add(time.Millisecond * waitTimeoutMillis)
	err := addENIAttachmentToState(attachmentType, attachmentArn, taskArn, randomMAC, expiresAt, taskEngineState, dataClient)
	assert.NoError(t, err)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	res, err := dataClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	for {
		time.Sleep(time.Millisecond * waitTimeoutMillis)
		if len(taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments()) == 0 {
			res, err := dataClient.GetENIAttachments()
			assert.NoError(t, err)
			assert.Len(t, res, 0)
			break
		}
	}
}

// TestTaskENIAckWithinTimeout tests the eni state change was reported before the timeout, for a task eni
func TestTaskENIAckWithinTimeout(t *testing.T) {
	testENIAckWithinTimeout(t, apieni.ENIAttachmentTypeTaskENI)
}

// TestInstanceENIAckWithinTimeout tests the eni state change was reported before the timeout, for an instance eni
func TestInstanceENIAckWithinTimeout(t *testing.T) {
	testENIAckWithinTimeout(t, apieni.ENIAttachmentTypeInstanceENI)
}

func testENIAckWithinTimeout(t *testing.T, attachmentType string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := data.NewNoopClient()
	expiresAt := time.Now().Add(time.Millisecond * waitTimeoutMillis)
	err := addENIAttachmentToState(attachmentType, attachmentArn, taskArn, randomMAC, expiresAt, taskEngineState, dataClient)
	assert.NoError(t, err)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	eniAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).ENIByMac(randomMAC)
	assert.True(t, ok)
	eniAttachment.SetSentStatus()

	time.Sleep(time.Millisecond * waitTimeoutMillis)

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
}

// TestHandleENIAttachmentTaskENI tests handling a new task eni
func TestHandleENIAttachmentTaskENI(t *testing.T) {
	testHandleENIAttachment(t, apieni.ENIAttachmentTypeTaskENI, taskArn)
}

// TestHandleENIAttachmentInstanceENI tests handling a new instance eni
func TestHandleENIAttachmentInstanceENI(t *testing.T) {
	testHandleENIAttachment(t, apieni.ENIAttachmentTypeInstanceENI, "")
}

func testHandleENIAttachment(t *testing.T, attachmentType, taskArn string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dataClient := newTestDataClient(t)

	taskEngineState := dockerstate.NewTaskEngineState()
	expiresAt := time.Now().Add(time.Millisecond * waitTimeoutMillis)
	err := handleENIAttachment(attachmentType, attachmentArn, taskArn, randomMAC, expiresAt, taskEngineState, dataClient)
	assert.NoError(t, err)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	eniAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).ENIByMac(randomMAC)
	assert.True(t, ok)
	eniAttachment.SetSentStatus()

	time.Sleep(time.Millisecond * waitTimeoutMillis)

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	res, err := dataClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 1)
}
