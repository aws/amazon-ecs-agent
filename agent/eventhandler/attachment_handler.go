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
	"errors"
	"fmt"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/cihub/seelog"
)

// AttachmentEventHandler is a handler that is responsible for submitting attachment state change events
// to backend
type AttachmentEventHandler struct {
	// backoff is the backoff object used in submitting attachment state change
	backoff retry.Backoff

	// dataClient is used to save any changes to an attachment's SentStatus
	dataClient data.Client

	// attachmentARNToHandler is a map from attachment ARN to the attachmentHandler that is
	// responsible for handling the attachment
	attachmentARNToHandler map[string]*attachmentHandler

	// lock is used to safely access the attachmentARNToHandler map
	lock sync.Mutex

	client api.ECSClient
	ctx    context.Context
}

// attachmentHandler is responsible for handling a certain attachment
type attachmentHandler struct {
	// attachmentARN is the arn of the attachment that the attachmentHandler is handling
	attachmentARN string

	// dataClient is used to save any changes to an attachment's SentStatus
	dataClient data.Client

	// backoff is the backoff object used in submitting attachment state change
	backoff retry.Backoff

	// lock is used to ensure that the attached status of an attachment won't be sent multiple times
	lock sync.Mutex

	client api.ECSClient
	ctx    context.Context
}

// NewAttachmentEventHandler returns a new AttachmentEventHandler object
func NewAttachmentEventHandler(ctx context.Context,
	dataClient data.Client,
	client api.ECSClient) *AttachmentEventHandler {
	return &AttachmentEventHandler{
		ctx:                    ctx,
		client:                 client,
		dataClient:             dataClient,
		attachmentARNToHandler: make(map[string]*attachmentHandler),
		backoff: retry.NewExponentialBackoff(submitStateBackoffMin, submitStateBackoffMax,
			submitStateBackoffJitterMultiple, submitStateBackoffMultiple),
	}
}

// AddStateChangeEvent adds a state change event to AttachmentEventHandler for it to handle
func (eventHandler *AttachmentEventHandler) AddStateChangeEvent(change statechange.Event) error {
	if change.GetEventType() != statechange.AttachmentEvent {
		return fmt.Errorf("eventhandler: attachment handler received unrecognized event type: %d", change.GetEventType())
	}
	event, ok := change.(api.AttachmentStateChange)
	if !ok {
		return errors.New("eventhandler: unable to get attachment event from state change event")
	}

	if event.Attachment == nil {
		return fmt.Errorf("eventhandler: received malformed attachment state change event: %v", event)
	}

	attachmentARN := event.Attachment.AttachmentARN
	eventHandler.lock.Lock()
	if _, ok := eventHandler.attachmentARNToHandler[attachmentARN]; !ok {
		eventHandler.attachmentARNToHandler[attachmentARN] = &attachmentHandler{
			attachmentARN: attachmentARN,
			dataClient:    eventHandler.dataClient,
			client:        eventHandler.client,
			ctx:           eventHandler.ctx,
			backoff:       eventHandler.backoff,
		}
	}
	eventHandler.lock.Unlock()

	attachmentHandler := eventHandler.attachmentARNToHandler[attachmentARN]
	go attachmentHandler.submitAttachmentEvent(&event)

	return nil
}

// submitAttachmentEvent submits an attachment event to backend
func (handler *attachmentHandler) submitAttachmentEvent(attachmentChange *api.AttachmentStateChange) {
	// we need to lock the attachment handler to avoid sending an attachment state change for an attachment
	// multiple times (this can happen when udev watcher sends multiple attached events for a certain attachment,
	// for example one from udev event and one from reconciliation loop)
	seelog.Debugf("AttachmentHandler: acquiring attachment lock before sending attachment state change for attachment %s", handler.attachmentARN)
	handler.lock.Lock()
	seelog.Debugf("AttachmentHandler: acquired attachment lock for attachment %s", handler.attachmentARN)
	defer handler.lock.Unlock()

	retry.RetryWithBackoffCtx(handler.ctx, handler.backoff, func() error {
		return handler.submitAttachmentEventOnce(attachmentChange)
	})
}

func (handler *attachmentHandler) submitAttachmentEventOnce(attachmentChange *api.AttachmentStateChange) error {
	if !attachmentChangeShouldBeSent(attachmentChange) {
		seelog.Debugf("AttachmentHandler: not sending attachment state change [%s] as it should not be sent", attachmentChange.String())
		// if the attachment state change should not be sent, we don't need to retry anymore so return nil here
		return nil
	}

	seelog.Infof("AttachmentHandler: sending attachment state change: %s", attachmentChange.String())
	if err := handler.client.SubmitAttachmentStateChange(*attachmentChange); err != nil {
		seelog.Errorf("AttachmentHandler: error submitting attachment state change [%s]: %v", attachmentChange.String(), err)
		return err
	}
	seelog.Debugf("AttachmentHandler: submitted attachment state change: %s", attachmentChange.String())

	attachmentChange.Attachment.SetSentStatus()
	attachmentChange.Attachment.StopAckTimer()

	err := handler.dataClient.SaveENIAttachment(attachmentChange.Attachment)
	if err != nil {
		seelog.Errorf("AttachmentHandler: error saving state after submitted attachment state change [%s]: %v", attachmentChange.String(), err)
	}
	return nil
}

// attachmentChangeShouldBeSent checks whether an attachment state change should be sent to backend
func attachmentChangeShouldBeSent(attachmentChange *api.AttachmentStateChange) bool {
	return !attachmentChange.Attachment.HasExpired() && !attachmentChange.Attachment.IsSent()
}
