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

package watcher

import (
	"context"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	expirationTimeAddition    = time.Second * 5
	expirationTimeSubtraction = time.Second * -1

	primaryMAC = "02:22:ea:8c:80:ae"
	randomMAC  = "00:0a:95:9d:68:16"
)

// setupWatcher sets up the watcher with the required fields
func setupWatcher(ctx context.Context, cancel context.CancelFunc, agentState dockerstate.TaskEngineState,
	eniChangeEvent chan<- statechange.Event, primaryMAC string) *ENIWatcher {

	return &ENIWatcher{
		ctx:            ctx,
		cancel:         cancel,
		agentState:     agentState,
		eniChangeEvent: eniChangeEvent,
		primaryMAC:     primaryMAC,
	}
}

// Test for SendENIStateChange. We send a StateChange notification for an ECS managed ENI which is yet to expire.
func TestSendENIStateChange(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(&apieni.ENIAttachment{
			ExpiresAt: time.Now().Add(expirationTimeAddition),
		}, true),
	)

	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel, primaryMAC)

	go watcher.sendENIStateChange(randomMAC)

	eniChangeEvent := <-eventChannel
	taskStateChange, ok := eniChangeEvent.(api.TaskStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, taskStateChange.Attachment.Status)
}

// Test for SendENIStateChange. We call the method for an Unmanaged ENI. Therefore we get an error.
func TestSendENIStateChangeUnmanaged(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(nil, false),
	)

	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel, primaryMAC)

	assert.Error(t, watcher.sendENIStateChange(randomMAC))
}

// Test for SendENIStateChange. We call the method for an ENI whose state change message has already been sent.
// Therefore, we will not send another message
func TestSendENIStateChangeAlreadySent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(&apieni.ENIAttachment{
			AttachStatusSent: true,
			ExpiresAt:        time.Now().Add(expirationTimeAddition),
			MACAddress:       randomMAC,
		}, true),
	)

	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel, primaryMAC)

	assert.Error(t, watcher.sendENIStateChange(randomMAC))
}

// Test for SendENIStateChange. We call the method for an expired ENI.
// The ENI is removed from the state and no new notification is sent
func TestSendENIStateChangeExpired(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(
			&apieni.ENIAttachment{
				AttachStatusSent: false,
				ExpiresAt:        time.Now().Add(expirationTimeSubtraction),
				MACAddress:       randomMAC,
			}, true),
		mockStateManager.EXPECT().RemoveENIAttachment(randomMAC),
	)

	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel, primaryMAC)

	assert.Error(t, watcher.sendENIStateChange(randomMAC))
}

// Test for SendENIStateChangeWithRetries. We send a notification after retry in this case.
func TestSendENIStateChangeWithRetries(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(nil, false),
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(&apieni.ENIAttachment{
			ExpiresAt:  time.Now().Add(expirationTimeAddition),
			MACAddress: randomMAC,
		}, true),
	)

	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel, primaryMAC)

	go watcher.sendENIStateChangeWithRetries(ctx, randomMAC, sendENIStateChangeRetryTimeout)

	eniChangeEvent := <-eventChannel
	taskStateChange, ok := eniChangeEvent.(api.TaskStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, taskStateChange.Attachment.Status)
}

// Test for SendENIStateChangeWithRetries. We call this method for an expired ENI.
// Therefore, we remove the ENI from state and return without any message on channel
func TestSendENIStateChangeWithRetriesDoesNotRetryExpiredENI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	ctx := context.TODO()

	gomock.InOrder(
		// ENIByMAC returns an error for exipred ENI attachment, which should
		// mean that it doesn't get retried.
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(
			&apieni.ENIAttachment{
				AttachStatusSent: false,
				ExpiresAt:        time.Now().Add(expirationTimeSubtraction),
				MACAddress:       randomMAC,
			}, true),
		mockStateManager.EXPECT().RemoveENIAttachment(randomMAC),
	)

	watcher := setupWatcher(ctx, nil, mockStateManager, nil, primaryMAC)

	assert.Error(t, watcher.sendENIStateChangeWithRetries(
		ctx, randomMAC, sendENIStateChangeRetryTimeout))
}

// TestSendENIStateChangeWithAttachmentTypeInstanceENI tests that we send the attachment state change
// of an instance level eni as an attachment state change
func TestSendENIStateChangeWithAttachmentTypeInstanceENI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(&apieni.ENIAttachment{
			AttachmentType: apieni.ENIAttachmentTypeInstanceENI,
			ExpiresAt:      time.Now().Add(expirationTimeAddition),
		}, true),
	)

	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel, primaryMAC)

	go watcher.sendENIStateChange(randomMAC)

	eniChangeEvent := <-eventChannel
	attachmentStateChange, ok := eniChangeEvent.(api.AttachmentStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, attachmentStateChange.Attachment.Status)
}

// TestSendENIStateChangeWithAttachmentTypeTaskENI tests that we send the attachment state change
// of a regular eni as a task state change
func TestSendENIStateChangeWithAttachmentTypeTaskENI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(&apieni.ENIAttachment{
			AttachmentType: apieni.ENIAttachmentTypeTaskENI,
			ExpiresAt:      time.Now().Add(expirationTimeAddition),
		}, true),
	)

	watcher := setupWatcher(ctx, nil, mockStateManager, eventChannel, primaryMAC)

	go watcher.sendENIStateChange(randomMAC)

	eniChangeEvent := <-eventChannel
	taskStateChange, ok := eniChangeEvent.(api.TaskStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, taskStateChange.Attachment.Status)
}
