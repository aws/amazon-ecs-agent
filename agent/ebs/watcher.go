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

package ebs

import (
	"context"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	log "github.com/cihub/seelog"

	"github.com/pkg/errors"
)

const (
	scanPeriod = 500 * time.Millisecond
)

type EBSWatcher struct {
	ctx        context.Context
	cancel     context.CancelFunc
	scanTicker *time.Ticker
	agentState dockerstate.TaskEngineState
	// TODO: The dataClient will be used to save to agent's data client as well as start the ACK timer. This will be added once the data client functionality have been added
	// dataClient     data.Client
	ebsChangeEvent  chan<- statechange.Event
	discoveryClient apiebs.EBSDiscovery
	mailbox         chan func()
}

// NewWatcher is used to return a new instance of the EBSWatcher struct
func NewWatcher(ctx context.Context,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) (*EBSWatcher, error) {
	derivedContext, cancel := context.WithCancel(ctx)
	discoveryClient := apiebs.NewDiscoveryClient(derivedContext)
	return &EBSWatcher{
		ctx:             derivedContext,
		cancel:          cancel,
		agentState:      state,
		ebsChangeEvent:  stateChangeEvents,
		discoveryClient: discoveryClient,
		mailbox:         make(chan func(), 100),
	}, nil
}

// Start is used to kick off the periodic scanning process of the EBS volume attachments for the EBS watcher.
// If there aren't any initially, the scan ticker will stop.
func (w *EBSWatcher) Start() {
	log.Info("Starting EBS watcher.")

	w.scanTicker = time.NewTicker(scanPeriod)
	if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		w.scanTicker.Stop()
	}

	for {
		select {
		case f := <-w.mailbox:
			f()
		case <-w.scanTicker.C:
			w.scanEBSVolumes()
		case <-w.ctx.Done():
			w.scanTicker.Stop()
			log.Info("EBS Watcher Stopped")
			return
		}
	}
}

// Stop will stop the EBS watcher
func (w *EBSWatcher) Stop() {
	log.Info("Stopping EBS watcher.")
	w.cancel()
}

// HandleResourceAttachment processes the resource attachment message.
func (w *EBSWatcher) HandleResourceAttachment(ebs *apiebs.ResourceAttachment) error {
	var err error
	var wg sync.WaitGroup
	wg.Add(1)
	w.mailbox <- func() {
		defer wg.Done()
		empty := len(w.agentState.GetAllPendingEBSAttachments()) == 0

		err := w.handleEBSAttachment(ebs)
		if err != nil {
			log.Warnf("Failed to handle resource attachment %v", ebs.String())
		}
		if empty && len(w.agentState.GetAllPendingEBSAttachments()) == 1 {
			w.scanTicker.Stop()
			w.scanTicker = time.NewTicker(scanPeriod)
		}
	}
	wg.Wait()
	return err
}

// handleEBSAttachment will handle an EBS attachment via the following:
// 1. Check whether we already have this attachment in state, if so, return
// 2. Otherwise add the attachment to state, start its ack timer, and save the state
func (w *EBSWatcher) handleEBSAttachment(ebs *apiebs.ResourceAttachment) error {
	if ebs.AttachmentProperties[apiebs.ResourceTypeName] != apiebs.ElasticBlockStorage {
		log.Info("Resource type not Elastic Block Storage. Skip handling resource attachment.")
		return nil
	}
	volumeID := ebs.AttachmentProperties[apiebs.VolumeIdName]
	_, ok := w.agentState.GetEBSByVolumeId(volumeID)
	if ok {
		log.Infof("EBS Volume attachment already exists. Skip handling EBS attachment %v.", ebs.String())
		return nil
	}

	if err := w.addEBSAttachmentToState(ebs); err != nil {
		return err
	}
	return nil
}

// notifyFoundEBS will mark it as found within the agent state
func (w *EBSWatcher) notifyFoundEBS(volumeId string) {
	w.mailbox <- func() {
		ebs, ok := w.agentState.GetEBSByVolumeId(volumeId)
		if !ok {
			log.Warnf("Unable to find EBS volume with volume ID: %v.", volumeId)
			return
		}

		if ebs.HasExpired() {
			log.Warnf("EBS status expired, no longer tracking EBS volume: %v.", ebs.String())
			return
		}

		// TODO: This is a placeholder for now until the attachment ACS handler gets implemented
		if ebs.IsSent() {
			log.Warnf("State change event has already been emitted for EBS volume: %v.", ebs.String())
			return
		}

		ebs.StopAckTimer()

		// TODO: This is a placeholder for now until the attachment ACS handler gets implemented
		ebs.SetSentStatus()

		log.Infof("Successfully found attached EBS volume: %v", ebs.String())
		if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
			log.Info("No more attachments to scan for. Stopping scan ticker.")
			w.scanTicker.Stop()
		}
	}
}

// RemoveAttachment will stop tracking an EBS attachment
func (w *EBSWatcher) RemoveAttachment(volumeID string) {
	w.mailbox <- func() {
		w.removeEBSAttachment(volumeID)
	}
}

func (w *EBSWatcher) removeEBSAttachment(volumeID string) {
	// TODO: Remove the EBS volume from the data client.
	w.agentState.RemoveEBSAttachment(volumeID)
	if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		log.Info("No more attachments to scan for. Stopping scan ticker.")
		w.scanTicker.Stop()
	}
}

// scanEBSVolumes will iterate through the entire list of pending EBS volume attachments within the agent state and checks if it's attached on the host.
func (w *EBSWatcher) scanEBSVolumes() {
	for _, ebs := range w.agentState.GetAllPendingEBSAttachments() {
		volumeId := ebs.AttachmentProperties[apiebs.VolumeIdName]
		deviceName := ebs.AttachmentProperties[apiebs.DeviceName]
		err := w.discoveryClient.ConfirmEBSVolumeIsAttached(deviceName, volumeId)
		if err != nil {
			if err == apiebs.ErrInvalidVolumeID || errors.Cause(err) == apiebs.ErrInvalidVolumeID {
				log.Warn("Expected EBS volume with device name: %v and volume ID: %v, Found a different EBS volume attached to the host.", deviceName, volumeId)
			} else {
				log.Warnf("Failed to confirm if EBS volume: %v, is attached to the host.", ebs.String())
			}
			continue
		}
		w.notifyFoundEBS(volumeId)
	}
}

// addEBSAttachmentToState adds an EBS attachment to state, and start its ack timer
func (w *EBSWatcher) addEBSAttachmentToState(ebs *apiebs.ResourceAttachment) error {
	volumeId := string(ebs.AttachmentProperties[apiebs.VolumeIdName])
	err := ebs.StartTimer(func() {
		w.handleEBSAckTimeout(volumeId)
	})
	if err != nil {
		return err
	}

	w.agentState.AddEBSAttachment(ebs)
	return nil
}

// handleEBSAckTimeout removes EBS attachment from agent state after the EBS ack timeout
func (w *EBSWatcher) handleEBSAckTimeout(volumeId string) {
	ebsAttachment, ok := w.agentState.GetEBSByVolumeId(volumeId)
	if !ok {
		log.Warnf("Ignoring unmanaged EBS attachment volume ID=%s", volumeId)
		return
	}
	if !ebsAttachment.IsSent() {
		log.Warnf("Timed out waiting for EBS ack; removing EBS attachment record %v", ebsAttachment.String())
		w.RemoveAttachment(volumeId)
	}
}
