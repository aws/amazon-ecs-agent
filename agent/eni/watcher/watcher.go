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
	"fmt"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
)

const (
	// sendENIStateChangeRetryTimeout specifies the timeout before giving up
	// when looking for ENI in agent's state. If for whatever reason, the message
	// from ACS is received after the ENI has been attached to the instance, this
	// timeout duration will be used to wait for ENI message to be sent from ACS
	sendENIStateChangeRetryTimeout = 6 * time.Second

	// sendENIStateChangeBackoffMin specifies minimum value for backoff when
	// waiting for attachment message from ACS
	sendENIStateChangeBackoffMin = 100 * time.Millisecond

	// sendENIStateChangeBackoffMax specifies maximum value for backoff when
	// waiting for attachment message from ACS
	sendENIStateChangeBackoffMax = 250 * time.Millisecond

	// sendENIStateChangeBackoffJitter specifies the jitter multiple percentage
	// when waiting for attachment message from ACS
	sendENIStateChangeBackoffJitter = 0.2

	// sendENIStateChangeBackoffMultiple specifies the backoff duration multipler
	// when waiting for the attachment message from ACS
	sendENIStateChangeBackoffMultiple = 1.5

	// macAddressRetryTimeout specifies the timeout before giving up when
	// looking for an ENI's mac address on the host. It takes a few milliseconds
	// for the host to learn about an ENIs mac address from netlink.LinkList().
	// We are capping off this duration to 1s assuming worst-case behavior
	macAddressRetryTimeout = 2 * time.Second

	// eniStatusSentMsg is the error message to use when trying to send an eni status that's
	// already been sent
	eniStatusSentMsg = "eni status already sent"
)

// New is used to return an instance of the ENIWatcher struct
func New(ctx context.Context, primaryMAC string,
	state dockerstate.TaskEngineState, stateChangeEvents chan<- statechange.Event) (*ENIWatcher, error) {
	return newWatcher(ctx, primaryMAC, state, stateChangeEvents)
}

// unmanagedENIError is used to indicate that the agent found an ENI, but the agent isn't
// aware if this ENI is being managed by ECS
type unmanagedENIError struct {
	mac string
}

// Error returns the error string for the unmanagedENIError type
func (err *unmanagedENIError) Error() string {
	return fmt.Sprintf("eni watcher send ENI state change: eni not managed by ecs: %s", err.mac)
}

// Init initializes a new ENI Watcher
func (eniWatcher *ENIWatcher) Init() error {
	// Retry in the first reconciliation, in case the ENI is attached before we connect to ACS.
	return eniWatcher.reconcileOnce(true)
}

// Start periodically updates the state of ENIs connected to the system
func (eniWatcher *ENIWatcher) Start() {
	// ENI Watcher Event Handler
	go eniWatcher.eventHandler()
	eniWatcher.performPeriodicReconciliation(defaultReconciliationInterval)
}

// Stop is used to invoke the cancellation routine
func (eniWatcher *ENIWatcher) Stop() {
	eniWatcher.cancel()
}

// performPeriodicReconciliation is used to periodically invoke the
// reconciliation process based on a ticker
func (eniWatcher *ENIWatcher) performPeriodicReconciliation(updateInterval time.Duration) {
	eniWatcher.updateIntervalTicker = time.NewTicker(updateInterval)
	for {
		select {
		case <-eniWatcher.updateIntervalTicker.C:
			if err := eniWatcher.reconcileOnce(false); err != nil {
				log.Warnf("ENI watcher reconciliation failed: %v", err)
			}
		case <-eniWatcher.ctx.Done():
			eniWatcher.updateIntervalTicker.Stop()
			return
		}
	}
}

// sendENIStateChange handles the eni event from eni monitoring or reconcile phase
func (eniWatcher *ENIWatcher) sendENIStateChange(mac string) error {
	if mac == "" {
		return errors.New("eni watcher send ENI state change: empty mac address")
	}
	// check if this is an eni required by a task
	eni, ok := eniWatcher.agentState.ENIByMac(mac)
	if !ok {
		return &unmanagedENIError{mac}
	}
	if eni.IsSent() {
		return errors.Errorf("eni watcher send ENI state change: %s: %s", eniStatusSentMsg, eni.String())
	}
	if eni.HasExpired() {
		// Agent is aware of the ENI, but we decide not to ack it
		// as it's ack timeout has expired
		eniWatcher.agentState.RemoveENIAttachment(eni.MACAddress)
		return errors.Errorf(
			"eni watcher send ENI state change: eni status expired, no longer tracking it: %s",
			eni.String())
	}

	// We found an ENI, which has the expiration time set in future and
	// needs to be acknowledged as having been 'attached' to the Instance
	if eni.AttachmentType == ni.ENIAttachmentTypeInstanceENI {
		go eniWatcher.emitInstanceENIAttachedEvent(eni)
	} else {
		go eniWatcher.emitTaskENIAttachedEvent(eni)
	}
	return nil
}

// emitTaskENIChangeEvent sends a state change event for a task ENI attachment to the event channel with eni status as
// attached
func (eniWatcher *ENIWatcher) emitTaskENIAttachedEvent(eni *ni.ENIAttachment) {
	eni.Status = attachment.AttachmentAttached
	log.Infof("Emitting task ENI attached event for: %s", eni.String())
	eniWatcher.eniChangeEvent <- api.TaskStateChange{
		TaskARN:    eni.TaskARN,
		Attachment: eni,
	}
}

// emitInstanceENIChangeEvent sends a state change event for an instance ENI attachment to the event channel with eni
// status as attached
func (eniWatcher *ENIWatcher) emitInstanceENIAttachedEvent(eni *ni.ENIAttachment) {
	eni.Status = attachment.AttachmentAttached
	log.Infof("Emitting instance ENI attached event for: %s", eni.String())
	eniWatcher.eniChangeEvent <- api.NewAttachmentStateChangeEvent(eni)
}

// sendENIStateChangeWithRetries invokes the sendENIStateChange method, with backoff and
// retries. Retries are only effective if sendENIStateChange returns an unmanagedENIError.
// We're effectively waiting for the ENI attachment message from ACS for a network device
// at this point of time.
func (eniWatcher *ENIWatcher) sendENIStateChangeWithRetries(parentCtx context.Context,
	macAddress string,
	timeout time.Duration) error {
	backoff := retry.NewExponentialBackoff(sendENIStateChangeBackoffMin, sendENIStateChangeBackoffMax,
		sendENIStateChangeBackoffJitter, sendENIStateChangeBackoffMultiple)
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	err := retry.RetryWithBackoffCtx(ctx, backoff, func() error {
		sendErr := eniWatcher.sendENIStateChange(macAddress)
		if sendErr != nil {
			if _, ok := sendErr.(*unmanagedENIError); ok {
				// This can happen in two scenarios: (1) the ENI is indeed not managed by ECS (i.e. attached manually
				// by customer); (2) this is an ENI attached by ECS but we have not yet received its information from
				// ACS.
				log.Debugf("Not sending state change because we don't know about the ENI: %v", sendErr)
				return sendErr
			}
			// Not unmanagedENIError. Stop retrying when this happens
			return apierrors.NewRetriableError(apierrors.NewRetriable(false), sendErr)
		}

		return nil
	})

	if err != nil {
		return err
	}
	// RetryWithBackoffCtx returns nil when the context is cancelled. Check if there was
	// a timeout here. TODO: Fix RetryWithBackoffCtx to return ctx.Err() on context Done()
	if err = ctx.Err(); err != nil {
		return errors.Wrapf(err,
			"eni watcher send ENI state change: timed out waiting for eni '%s' in state", macAddress)
	}

	return nil
}
