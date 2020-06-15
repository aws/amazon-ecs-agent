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
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/cihub/seelog"
)

const (
	// saveAttachmentStateRetryNum is the number of retries we have when we successfully submitted an attachment
	// state change event but failed to save the state for the attachment
	saveAttachmentStateRetryNum = 3
)

// AttachmentEventHandler is a handler that is responsible for submitting attachment state change events
// to backend
type AttachmentEventHandler struct {
	// stateSaver is a statemanager which may be used to save any
	// changes to an attachment's SentStatus
	stateSaver statemanager.Saver

	// backoff is the backoff object used in submitting attachment state change
	backoff retry.Backoff

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

	// stateSaver is a statemanager which may be used to save any
	// changes to an attachment's SentStatus
	stateSaver statemanager.Saver

	// backoff is the backoff object used in submitting attachment state change
	backoff retry.Backoff

	// lock is used to ensure that the attached status of an attachment won't be sent multiple times
	lock sync.Mutex

	client api.ECSClient
	ctx    context.Context
}

// NewAttachmentEventHandler returns a new AttachmentEventHandler object
func NewAttachmentEventHandler(ctx context.Context,
	stateSaver statemanager.Saver,
	client api.ECSClient) *AttachmentEventHandler {
	return &AttachmentEventHandler{
		ctx:                    ctx,
		stateSaver:             stateSaver,
		client:                 client,
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
			stateSaver:    eventHandler.stateSaver,
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

	err := handler.stateSaver.Save()
	if err != nil {
		// saving state error more often than not is caused by running out of disk space and it's unlikely to succeed by
		// retry, so just retry a few times and give up. and we don't need to hold the attachment lock here, so retry in
		// a separate go routine and return nil for the outer retry loop
		seelog.Errorf("AttachmentHandler: error saving state after submitted attachment state change [%s]: %v", attachmentChange.String(), err)
		go handler.retrySavingState(attachmentChange)
	}
	return nil
}

func (handler *attachmentHandler) retrySavingState(attachmentChange *api.AttachmentStateChange) {
	err := retry.RetryNWithBackoffCtx(handler.ctx, handler.backoff, saveAttachmentStateRetryNum, handler.stateSaver.Save)
	if err != nil {
		seelog.Errorf("AttachmentHandler: failed to save state after submitted attachment state change [%s]: %v", attachmentChange.String(), err)
	}
}

// attachmentChangeShouldBeSent checks whether an attachment state change should be sent to backend
func attachmentChangeShouldBeSent(attachmentChange *api.AttachmentStateChange) bool {
	return !attachmentChange.Attachment.HasExpired() && !attachmentChange.Attachment.IsSent()
}
