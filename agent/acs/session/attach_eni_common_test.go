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

package session

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
)

const (
	attachmentArn = "arn:aws:ecs:us-west-2:1234567890:attachment/abc"
)

// TestTaskENIAckTimeout tests acknowledge timeout for a task eni before submit the state change
func TestTaskENIAckTimeout(t *testing.T) {
	testENIAckTimeout(t, ni.ENIAttachmentTypeTaskENI)
}

// TestInstanceENIAckTimeout tests acknowledge timeout for an instance level eni before submit the state change
func TestInstanceENIAckTimeout(t *testing.T) {
	testENIAckTimeout(t, ni.ENIAttachmentTypeInstanceENI)
}

func testENIAckTimeout(t *testing.T, attachmentType string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := newTestDataClient(t)

	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)
	eniAttachment := &ni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:          testconst.TaskARN,
			AttachmentARN:    attachmentArn,
			ExpiresAt:        expiresAt,
			AttachStatusSent: false,
		},
		AttachmentType: attachmentType,
		MACAddress:     testconst.RandomMAC,
	}
	eniHandler := NewENIHandler(taskEngineState, dataClient)

	err := eniHandler.addENIAttachmentToState(eniAttachment)
	assert.NoError(t, err)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	res, err := dataClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	for {
		time.Sleep(time.Millisecond * testconst.WaitTimeoutMillis)
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
	testENIAckWithinTimeout(t, ni.ENIAttachmentTypeTaskENI)
}

// TestInstanceENIAckWithinTimeout tests the eni state change was reported before the timeout, for an instance eni
func TestInstanceENIAckWithinTimeout(t *testing.T) {
	testENIAckWithinTimeout(t, ni.ENIAttachmentTypeInstanceENI)
}

func testENIAckWithinTimeout(t *testing.T, attachmentType string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := data.NewNoopClient()
	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)
	eniAttachment := &ni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:          testconst.TaskARN,
			AttachmentARN:    attachmentArn,
			ExpiresAt:        expiresAt,
			AttachStatusSent: false,
		},
		AttachmentType: attachmentType,
		MACAddress:     testconst.RandomMAC,
	}
	eniHandler := NewENIHandler(taskEngineState, dataClient)

	err := eniHandler.addENIAttachmentToState(eniAttachment)
	assert.NoError(t, err)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	eniAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).ENIByMac(testconst.RandomMAC)
	assert.True(t, ok)
	eniAttachment.SetSentStatus()

	time.Sleep(time.Millisecond * testconst.WaitTimeoutMillis)

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
}

// TestHandleENIAttachmentTaskENI tests handling a new task eni
func TestHandleENIAttachmentTaskENI(t *testing.T) {
	testHandleENIAttachment(t, ni.ENIAttachmentTypeTaskENI, testconst.TaskARN)
}

// TestHandleENIAttachmentInstanceENI tests handling a new instance eni
func TestHandleENIAttachmentInstanceENI(t *testing.T) {
	testHandleENIAttachment(t, ni.ENIAttachmentTypeInstanceENI, "")
}

func testHandleENIAttachment(t *testing.T, attachmentType, taskArn string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dataClient := newTestDataClient(t)

	taskEngineState := dockerstate.NewTaskEngineState()
	expiresAt := time.Now().Add(time.Millisecond * testconst.WaitTimeoutMillis)
	eniAttachment := &ni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:          taskArn,
			AttachmentARN:    attachmentArn,
			ExpiresAt:        expiresAt,
			AttachStatusSent: false,
		},
		AttachmentType: attachmentType,
		MACAddress:     testconst.RandomMAC,
	}
	eniHandler := NewENIHandler(taskEngineState, dataClient)

	err := eniHandler.HandleENIAttachment(eniAttachment)
	assert.NoError(t, err)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	eniAttachment, ok := taskEngineState.(*dockerstate.DockerTaskEngineState).ENIByMac(testconst.RandomMAC)
	assert.True(t, ok)
	eniAttachment.SetSentStatus()

	time.Sleep(time.Millisecond * testconst.WaitTimeoutMillis)

	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	res, err := dataClient.GetENIAttachments()
	assert.NoError(t, err)
	assert.Len(t, res, 1)
}

// TestHandleExpiredENIAttachmentTaskENI tests handling an expired task eni
func TestHandleExpiredENIAttachmentTaskENI(t *testing.T) {
	testHandleExpiredENIAttachment(t, ni.ENIAttachmentTypeTaskENI, testconst.TaskARN)
}

// TestHandleExpiredENIAttachmentInstanceENI tests handling an expired instance eni
func TestHandleExpiredENIAttachmentInstanceENI(t *testing.T) {
	testHandleExpiredENIAttachment(t, ni.ENIAttachmentTypeInstanceENI, "")
}

func testHandleExpiredENIAttachment(t *testing.T, attachmentType, taskArn string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Set expiresAt to a value in the past.
	expiresAt := time.Unix(time.Now().Unix()-1, 0)

	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := data.NewNoopClient()

	eniAttachment := &ni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:       taskArn,
			AttachmentARN: attachmentArn,
			ExpiresAt:     expiresAt,
		},
		AttachmentType: attachmentType,
		MACAddress:     testconst.RandomMAC,
	}
	eniHandler := NewENIHandler(taskEngineState, dataClient)

	// Expect an error starting the timer because of <=0 duration.
	err := eniHandler.HandleENIAttachment(eniAttachment)
	assert.Error(t, err)
	assert.Equal(t, true, eniAttachment.HasExpired())
}
