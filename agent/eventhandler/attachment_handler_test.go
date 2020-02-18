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

package eventhandler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	mock_statemanager "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	// small backoff value used by unit test
	xSubmitStateBackoffMin            = 10 * time.Millisecond
	xSubmitStateBackoffMax            = 30 * time.Millisecond
	xSubmitStateBackoffJitterMultiple = 0.20
	xSubmitStateBackoffMultiple       = 1.3

	attachmentARN = "attachmentARN"
)

func TestSendAttachmentEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	attachmentEvent := attachmentEvent(attachmentARN)

	timeoutFunc := func() {
		t.Error("Timeout sending ENI attach status")
	}
	assert.NoError(t, attachmentEvent.Attachment.StartTimer(timeoutFunc))

	ctx, cancel := context.WithCancel(context.Background())
	handler := NewAttachmentEventHandler(ctx, statemanager.NewNoopStateManager(), client)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	client.EXPECT().SubmitAttachmentStateChange(gomock.Any()).Return(nil).Do(func(change api.AttachmentStateChange) {
		assert.NotNil(t, change.Attachment)
		assert.Equal(t, attachmentARN, change.Attachment.AttachmentARN)
		wg.Done()
	})

	handler.AddStateChangeEvent(attachmentEvent)

	wg.Wait()
}

func TestSendAttachmentEventRetries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	attachmentEvent := attachmentEvent("attachmentARN")

	timeoutFunc := func() {
		t.Error("Timeout sending ENI attach status")
	}
	assert.NoError(t, attachmentEvent.Attachment.StartTimer(timeoutFunc))

	ctx, cancel := context.WithCancel(context.Background())
	handler := NewAttachmentEventHandler(ctx, statemanager.NewNoopStateManager(), client)
	// use smaller backoff value for unit test
	handler.backoff = retry.NewExponentialBackoff(xSubmitStateBackoffMin, xSubmitStateBackoffMax,
		xSubmitStateBackoffJitterMultiple, xSubmitStateBackoffMultiple)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	retriable := apierrors.NewRetriableError(apierrors.NewRetriable(true), errors.New("test"))

	gomock.InOrder(
		client.EXPECT().SubmitAttachmentStateChange(gomock.Any()).Return(retriable).Do(func(interface{}) { wg.Done() }),
		client.EXPECT().SubmitAttachmentStateChange(gomock.Any()).Return(nil).Do(func(change api.AttachmentStateChange) {
			assert.NotNil(t, change.Attachment)
			assert.Equal(t, attachmentARN, change.Attachment.AttachmentARN)
			wg.Done()
		}),
	)

	handler.AddStateChangeEvent(attachmentEvent)

	wg.Wait()
}

func TestSendAttachmentEventRetrySavingState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)
	stateSaver := mock_statemanager.NewMockStateManager(ctrl)

	attachmentEvent := attachmentEvent("attachmentARN")

	timeoutFunc := func() {
		t.Error("Timeout sending ENI attach status")
	}
	assert.NoError(t, attachmentEvent.Attachment.StartTimer(timeoutFunc))

	ctx, cancel := context.WithCancel(context.Background())
	handler := NewAttachmentEventHandler(ctx, stateSaver, client)
	// use smaller backoff value for unit test
	handler.backoff = retry.NewExponentialBackoff(xSubmitStateBackoffMin, xSubmitStateBackoffMax,
		xSubmitStateBackoffJitterMultiple, xSubmitStateBackoffMultiple)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	retriable := apierrors.NewRetriableError(apierrors.NewRetriable(true), errors.New("test"))

	gomock.InOrder(
		client.EXPECT().SubmitAttachmentStateChange(gomock.Any()).Return(nil).Do(func(change api.AttachmentStateChange) {
			assert.NotNil(t, change.Attachment)
			assert.Equal(t, attachmentARN, change.Attachment.AttachmentARN)
		}),
		stateSaver.EXPECT().Save().Return(retriable),
		stateSaver.EXPECT().Save().Return(nil).Do(func() { wg.Done() }),
	)

	handler.AddStateChangeEvent(attachmentEvent)

	wg.Wait()
}

func TestSendMultipleAttachmentEventsDifferentAttachments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	attachmentEvent1 := attachmentEvent("attachmentARN1")
	attachmentEvent2 := attachmentEvent("attachmentARN2")
	attachmentEvent3 := attachmentEvent("attachmentARN3")

	timeoutFunc := func() {
		t.Error("Timeout sending ENI attach status")
	}
	assert.NoError(t, attachmentEvent1.Attachment.StartTimer(timeoutFunc))
	assert.NoError(t, attachmentEvent2.Attachment.StartTimer(timeoutFunc))
	assert.NoError(t, attachmentEvent3.Attachment.StartTimer(timeoutFunc))

	ctx, cancel := context.WithCancel(context.Background())
	handler := NewAttachmentEventHandler(ctx, statemanager.NewNoopStateManager(), client)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(3)

	submittedAttachments := make(map[string]bool) // note down submitted attachments
	mapLock := sync.Mutex{}                       // lock to protect the above map
	client.EXPECT().SubmitAttachmentStateChange(gomock.Any()).Times(3).Return(nil).Do(func(change api.AttachmentStateChange) {
		mapLock.Lock()
		defer mapLock.Unlock()

		submittedAttachments[change.Attachment.AttachmentARN] = true
		wg.Done()
	})

	handler.AddStateChangeEvent(attachmentEvent1)
	handler.AddStateChangeEvent(attachmentEvent2)
	handler.AddStateChangeEvent(attachmentEvent3)

	wg.Wait()
	assert.Equal(t, 3, len(submittedAttachments))
	assert.Contains(t, submittedAttachments, "attachmentARN1")
	assert.Contains(t, submittedAttachments, "attachmentARN2")
	assert.Contains(t, submittedAttachments, "attachmentARN3")
}

func TestSubmitAttachmentEventSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	attachmentEvent := attachmentEvent(attachmentARN)

	timeoutFunc := func() {
		t.Error("Timeout sending ENI attach status")
	}
	assert.NoError(t, attachmentEvent.Attachment.StartTimer(timeoutFunc))

	ctx, cancel := context.WithCancel(context.Background())
	handler := &attachmentHandler{
		stateSaver: statemanager.NewNoopStateManager(),
		client:     client,
		ctx:        ctx,
	}
	defer cancel()

	client.EXPECT().SubmitAttachmentStateChange(gomock.Any()).Return(nil).Do(func(change api.AttachmentStateChange) {
		assert.NotNil(t, change.Attachment)
		assert.Equal(t, attachmentARN, change.Attachment.AttachmentARN)
	})

	handler.submitAttachmentEvent(&attachmentEvent)

	assert.True(t, attachmentEvent.Attachment.AttachStatusSent)
}

func TestSubmitAttachmentEventAttachmentExpired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	attachmentEvent := attachmentEvent(attachmentARN)
	attachmentEvent.Attachment.ExpiresAt = time.Now().Add(100 * time.Millisecond)

	// wait until eni attachment expires
	time.Sleep(200 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	handler := &attachmentHandler{
		stateSaver: statemanager.NewNoopStateManager(),
		client:     client,
		ctx:        ctx,
	}
	defer cancel()

	handler.submitAttachmentEvent(&attachmentEvent)

	// no SubmitAttachmentStateChange should happen and attach status should not be sent
	assert.False(t, attachmentEvent.Attachment.AttachStatusSent)
}

func TestSubmitAttachmentEventAttachmentIsSent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mock_api.NewMockECSClient(ctrl)

	attachmentEvent := attachmentEvent(attachmentARN)
	attachmentEvent.Attachment.SetSentStatus()

	timeoutFunc := func() {
		t.Error("Timeout sending ENI attach status")
	}
	assert.NoError(t, attachmentEvent.Attachment.StartTimer(timeoutFunc))

	ctx, cancel := context.WithCancel(context.Background())
	handler := &attachmentHandler{
		stateSaver: statemanager.NewNoopStateManager(),
		client:     client,
		ctx:        ctx,
	}
	defer cancel()

	handler.submitAttachmentEvent(&attachmentEvent)

	// no SubmitAttachmentStateChange should happen
	attachmentEvent.Attachment.StopAckTimer()
}

func TestAttachmentChangeShouldBeSent(t *testing.T) {
	attachmentEvent := attachmentEvent(attachmentARN)
	assert.True(t, attachmentChangeShouldBeSent(&attachmentEvent))
}

func TestAttachmentChangeShouldBeSentAttachmentExpired(t *testing.T) {
	attachmentEvent := attachmentEvent(attachmentARN)
	attachmentEvent.Attachment.ExpiresAt = time.Now()
	time.Sleep(10 * time.Millisecond)

	assert.False(t, attachmentChangeShouldBeSent(&attachmentEvent))
}

func TestAttachmentChangeShouldBeSentAttachmentIsSent(t *testing.T) {
	attachmentEvent := attachmentEvent(attachmentARN)
	attachmentEvent.Attachment.SetSentStatus()
	assert.False(t, attachmentChangeShouldBeSent(&attachmentEvent))
}

func attachmentEvent(attachmentARN string) api.AttachmentStateChange {
	return api.AttachmentStateChange{
		Attachment: &apieni.ENIAttachment{
			AttachmentType:   apieni.ENIAttachmentTypeInstanceENI,
			AttachmentARN:    attachmentARN,
			AttachStatusSent: false,
			ExpiresAt:        time.Now().Add(time.Second),
		},
	}
}
