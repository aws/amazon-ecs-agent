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
	"fmt"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	apiebs "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

type EBSWatcher struct {
	ctx        context.Context
	cancel     context.CancelFunc
	agentState dockerstate.TaskEngineState
	// TODO: The ebsChangeEvent will be used to send over the state change event for EBS attachments once it's been found and mounted/resize/format.
	ebsChangeEvent chan<- statechange.Event
	// TODO: The dataClient will be used to save to agent's data client as well as start the ACK timer. This will be added once the data client functionality have been added
	// dataClient     data.Client
	discoveryClient      apiebs.EBSDiscovery
	scanTickerController *apiebs.ScanTickerController
}

// NewWatcher is used to return a new instance of the EBSWatcher struct
func NewWatcher(ctx context.Context,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) *EBSWatcher {
	derivedContext, cancel := context.WithCancel(ctx)
	discoveryClient := apiebs.NewDiscoveryClient(derivedContext)
	scanTickerController := apiebs.NewScanTickerController()
	return &EBSWatcher{
		ctx:                  derivedContext,
		cancel:               cancel,
		agentState:           state,
		ebsChangeEvent:       stateChangeEvents,
		discoveryClient:      discoveryClient,
		scanTickerController: scanTickerController,
	}
}

// Start is used to kick off the periodic scanning process of the EBS volume attachments for the EBS watcher.
// It will be start and continue to run whenever there's a pending EBS volume attachment that hasn't been found.
// If there aren't any, the scan ticker will not start up/scan for volumes.
func (w *EBSWatcher) Start() {
	w.scanTickerController.TickerLock.Lock()
	defer w.scanTickerController.TickerLock.Unlock()

	if w.scanTickerController.Running || len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		return
	}

	log.Info("Starting EBS watcher.")
	w.scanTickerController.Running = true
	w.scanTickerController.ScanTicker = time.NewTicker(apiebs.ScanPeriod)
	go func() {
		for {
			select {
			case <-w.scanTickerController.ScanTicker.C:
				pendingEBS := w.agentState.GetAllPendingEBSAttachmentWithKey()
				foundVolumes := apiebs.ScanEBSVolumes(pendingEBS, w.discoveryClient)
				w.NotifyFound(foundVolumes)
			case <-w.scanTickerController.Done:
				w.scanTickerController.Running = false
				w.scanTickerController.ScanTicker.Stop()
				return
			case <-w.ctx.Done():
				w.scanTickerController.StopScanTicker()
				log.Info("EBS Watcher Stopped due to agent stop")
				return
			}
		}
	}()
	log.Info("EBS watcher started.")
}

// Stop will stop the EBS watcher
func (w *EBSWatcher) Stop() {
	log.Info("Stopping EBS watcher.")
	w.cancel()
}

// HandleResourceAttachment processes the resource attachment message. It will:
// 1. Check whether we already have this attachment in state, if so, return
// 2. Otherwise add the attachment to state, start its ack timer, and save to the agent state
// If it's the first pending volume to be added to the agent state, then the EBS watcher will start scanning.
func (w *EBSWatcher) HandleResourceAttachment(ebs *apiebs.ResourceAttachment) error {
	wasEmpty := len(w.agentState.GetAllPendingEBSAttachments()) == 0
	attachmentType := ebs.AttachmentProperties[apiebs.ResourceTypeName]
	if attachmentType != apiebs.ElasticBlockStorage {
		log.Warnf("Resource type not Elastic Block Storage. Skip handling resource attachment with type: %v.", attachmentType)
	}

	volumeId := ebs.AttachmentProperties[apiebs.VolumeIdName]
	_, ok := w.agentState.GetEBSByVolumeId(volumeId)
	if ok {
		log.Infof("EBS Volume attachment already exists. Skip handling EBS attachment %v.", ebs.EBSToString())
		return nil
	}

	if err := w.addEBSAttachmentToState(ebs); err != nil {
		return errors.Wrapf(err, fmt.Sprintf("attach %s message handler: unable to add ebs attachment to engine state: %s",
			attachmentType, ebs.EBSToString()))
	}

	// If it was originally empty and now there's a pending EBS volume to scan for.
	if wasEmpty && len(w.agentState.GetAllPendingEBSAttachments()) == 1 {
		go w.Start()
	}
	return nil
}

// NotifyFound will go through the list of found EBS volumes from the scanning process and mark them as found.
// Afterwards, it stops the EBS watcher if there are no more EBS volumes to find on the host.
func (w *EBSWatcher) NotifyFound(foundVolumes []string) {
	for _, volumeId := range foundVolumes {
		w.notifyFoundEBS(volumeId)
	}
	w.checkPendingEBSVolumes()
	// if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
	// 	w.scanTickerController.StopScanTicker()
	// }
}

// notifyFoundEBS will mark it as found within the agent state
func (w *EBSWatcher) notifyFoundEBS(volumeId string) {
	// TODO: Add the EBS volume to data client
	ebs, ok := w.agentState.GetEBSByVolumeId(volumeId)
	if !ok {
		log.Warnf("Unable to find EBS volume with volume ID: %v.", volumeId)
		return
	}

	if ebs.HasExpired() {
		log.Warnf("EBS status expired, no longer tracking EBS volume: %v.", ebs.EBSToString())
		return
	}

	if ebs.IsSent() {
		log.Warnf("State change event has already been emitted for EBS volume: %v.", ebs.EBSToString())
		return
	}

	if ebs.IsAttached() {
		log.Infof("EBS volume: %v, has been found already.", ebs.EBSToString())
		return
	}

	ebs.StopAckTimer()
	ebs.SetAttachedStatus()

	log.Infof("Successfully found attached EBS volume: %v", ebs.EBSToString())
}

func (w *EBSWatcher) removeEBSAttachment(volumeID string) {
	// TODO: Remove the EBS volume from the data client.
	w.agentState.RemoveEBSAttachment(volumeID)
	w.checkPendingEBSVolumes()
	// if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
	// 	w.scanTickerController.StopScanTicker()
	// }
}

func (w *EBSWatcher) checkPendingEBSVolumes() {
	if len(w.agentState.GetAllPendingEBSAttachments()) == 0 {
		// log.Info("No more attachments to scan for. Stopping scan ticker.")
		w.scanTickerController.StopScanTicker()
	}
}

// addEBSAttachmentToState adds an EBS attachment to state, and start its ack timer
func (w *EBSWatcher) addEBSAttachmentToState(ebs *apiebs.ResourceAttachment) error {
	volumeId := ebs.AttachmentProperties[apiebs.VolumeIdName]
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
		log.Warnf("Timed out waiting for EBS ack; removing EBS attachment record %v", ebsAttachment.EBSToString())
		w.removeEBSAttachment(volumeId)
	}
}
